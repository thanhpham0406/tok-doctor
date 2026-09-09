package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/config"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

// --- Profile validation tests -----------------------------------------------

func TestValidateProfiles_DuplicateListen(t *testing.T) {
	raw := map[string]config.GatewayProfile{
		"a": {Enabled: true, Listen: "127.0.0.1:9001", Protocol: "anthropic_messages", Source: "claude", Upstream: "http://127.0.0.1:20128"},
		"b": {Enabled: true, Listen: "127.0.0.1:9001", Protocol: "openai_responses", Source: "codex", Upstream: "http://127.0.0.1:20128"},
	}
	set := ValidateProfiles(raw)
	if len(set.Profiles) != 1 {
		t.Fatalf("expected 1 valid profile, got %d", len(set.Profiles))
	}
	if set.Profiles[0].Name != "a" {
		t.Fatalf("unexpected survivor: %s", set.Profiles[0].Name)
	}
	if len(set.Errors) != 1 || !strings.Contains(set.Errors[0].Reason, "duplicate listen") {
		t.Fatalf("expected duplicate-listen error, got %v", set.Errors)
	}
}

func TestValidateProfiles_NonLoopback(t *testing.T) {
	raw := map[string]config.GatewayProfile{
		"bad": {Enabled: true, Listen: "0.0.0.0:9001", Protocol: "anthropic_messages", Source: "claude", Upstream: "http://127.0.0.1:20128"},
	}
	set := ValidateProfiles(raw)
	if len(set.Profiles) != 0 {
		t.Fatalf("non-loopback bind must be rejected")
	}
}

func TestValidateProfiles_UnsupportedProtocol(t *testing.T) {
	raw := map[string]config.GatewayProfile{
		"x": {Enabled: true, Listen: "127.0.0.1:9001", Protocol: "gibberish", Source: "codex", Upstream: "http://127.0.0.1:20128"},
	}
	set := ValidateProfiles(raw)
	if len(set.Profiles) != 0 {
		t.Fatalf("unsupported protocol must be rejected")
	}
}

func TestValidateProfiles_DisabledIgnored(t *testing.T) {
	raw := map[string]config.GatewayProfile{
		"off": {Enabled: false, Listen: "127.0.0.1:9001", Protocol: "anthropic_messages", Source: "claude", Upstream: "http://127.0.0.1:20128"},
	}
	set := ValidateProfiles(raw)
	if len(set.Profiles) != 0 {
		t.Fatalf("disabled profiles must not be returned")
	}
}

func TestValidateProfiles_EmptyUpstream(t *testing.T) {
	raw := map[string]config.GatewayProfile{
		"x": {Enabled: true, Listen: "127.0.0.1:9001", Protocol: "anthropic_messages", Source: "claude", Upstream: ""},
	}
	set := ValidateProfiles(raw)
	if len(set.Profiles) != 0 {
		t.Fatalf("empty upstream must be rejected")
	}
}

// --- Multi-profile runtime --------------------------------------------------

func freeLoopback(t *testing.T) (string, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen loopback: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr, func() {}
}

func TestRuntime_MultipleProfiles(t *testing.T) {
	listen1, _ := freeLoopback(t)
	listen2, _ := freeLoopback(t)

	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream1.Close()
	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream2.Close()

	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()
	rt := NewRuntime(rec)
	profiles := []Profile{
		{Name: "alpha", Enabled: true, Listen: listen1, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream1.URL},
		{Name: "beta", Enabled: true, Listen: listen2, Protocol: "openai_responses", Source: "codex", Upstream: upstream2.URL},
	}
	if err := rt.Start(context.Background(), profiles); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

	for _, p := range profiles {
		resp, err := http.Get("http://" + p.Listen + "/v1/anything?x=1")
		if err != nil {
			t.Fatalf("GET %s: %v", p.Name, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status for %s = %d", p.Name, resp.StatusCode)
		}
	}
}

func TestRuntime_SingleProfileFlag(t *testing.T) {
	listen1, _ := freeLoopback(t)
	listen2, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	set := ProfileSet{Profiles: []Profile{
		{Name: "alpha", Enabled: true, Listen: listen1, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		{Name: "beta", Enabled: true, Listen: listen2, Protocol: "openai_responses", Source: "codex", Upstream: upstream.URL},
	}}
	filtered := set.FilterByName("beta")
	if len(filtered.Profiles) != 1 || filtered.Profiles[0].Name != "beta" {
		t.Fatalf("FilterByName failed: %+v", filtered.Profiles)
	}
}

func TestRuntime_GracefulShutdown(t *testing.T) {
	listen, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	dir := t.TempDir()
	rec, _ := NewFileRecorder(dir)
	defer func() { _ = rec.Close() }()
	rt := NewRuntime(rec)
	if err := rt.Start(context.Background(), []Profile{
		{Name: "x", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rt.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// --- Transparent forwarding -------------------------------------------------

type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

func TestProxy_ForwardsMethodPathQuery(t *testing.T) {
	captured := make(chan capturedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Header: r.Header.Clone(),
			Body:   body,
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	proxy, err := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	bodyIn := []byte(`{"model":"claude-x","messages":[]}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages?x=1&y=two", bytes.NewReader(bodyIn))
	req.Header.Set("X-Test", "yes")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()

	got := <-captured
	if got.Method != http.MethodPost {
		t.Fatalf("method = %q", got.Method)
	}
	if got.Path != "/v1/messages" {
		t.Fatalf("path = %q", got.Path)
	}
	if got.Query != "x=1&y=two" {
		t.Fatalf("query = %q", got.Query)
	}
	if !bytes.Equal(got.Body, bodyIn) {
		t.Fatalf("body mismatch: got %q", got.Body)
	}
}

func TestProxy_AuthHeaderForwardedNotPersisted(t *testing.T) {
	got := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	dir := t.TempDir()
	rec, _ := NewFileRecorder(dir)
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", bytes.NewReader([]byte(`{"model":"x"}`)))
	req.Header.Set("Authorization", "Bearer sk-secret-value")
	resp, _ := http.DefaultClient.Do(req)
	_ = resp.Body.Close()

	if v := <-got; v != "Bearer sk-secret-value" {
		t.Fatalf("auth header was not forwarded: %q", v)
	}

	persisted, err := Replay(rec, "p")
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	for _, ex := range persisted {
		for _, component := range ex.Request.Components {
			for _, ev := range component.Evidence {
				if strings.Contains(ev.Field, "Authorization") {
					t.Fatalf("evidence captured Authorization: %+v", ev)
				}
			}
		}
		raw, _ := json.Marshal(ex)
		if strings.Contains(string(raw), "sk-secret-value") {
			t.Fatalf("persisted capture leaks auth: %s", raw)
		}
	}
}

func TestProxy_UpstreamFailureSurfaced(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close() // listener immediately dies; any request fails to connect

	listen, _ := freeLoopback(t)
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: dead.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 from dead upstream, got %d", resp.StatusCode)
	}
}

func TestProxy_ParserFailureContinues(t *testing.T) {
	got := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	junk := []byte(`{"messages":[not json`)
	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader(junk))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxy should still forward on parser failure, got %d", resp.StatusCode)
	}
	gotBody := <-got
	if !bytes.Equal(gotBody, junk) {
		t.Fatalf("forwarded body mismatch")
	}
}

func TestProxy_StreamingPassesThrough(t *testing.T) {
	chunks := []string{"first", "second", "third"}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", c)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/messages")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected SSE Content-Type, got %q", resp.Header.Get("Content-Type"))
	}

	got := readAllChunks(t, resp.Body)
	if !strings.Contains(got, "first") || !strings.Contains(got, "third") {
		t.Fatalf("missing chunks: %q", got)
	}
}

func readAllChunks(t *testing.T, r io.Reader) string {
	t.Helper()
	var sb strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestProxy_BodyHashDeterministic(t *testing.T) {
	body := []byte(`{"model":"x","messages":[]}`)
	if got := contentHash(body); got != contentHash(body) {
		t.Fatalf("hash not deterministic")
	}
	other := []byte(`{"model":"y","messages":[]}`)
	if got := contentHash(body); got == contentHash(other) {
		t.Fatalf("hash collisions on distinct bodies")
	}
}

func TestProxy_RawContentNeverPersisted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	dir := t.TempDir()
	rec, _ := NewFileRecorder(dir)
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	secret := "SECRET_PROMPT_LITERAL_DO_NOT_PERSIST"
	body := []byte(fmt.Sprintf(`{"model":"claude-x","system":"%s","messages":[{"role":"user","content":"hi"}]}`, secret))
	resp, _ := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader(body))
	_ = resp.Body.Close()

	data, err := os.ReadFile(filepath.Join(dir, "p.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("persisted capture contains raw secret content: %s", data)
	}
}

func TestProxy_CaptureIncludesProfileSourceProtocol(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "alpha", Enabled: true, Listen: listen, Protocol: "openai_responses", Source: "codex", Upstream: upstream.URL},
		OpenAIResponsesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	body := []byte(`{"model":"gpt-x","instructions":"be terse","input":[{"role":"user","content":"hi"}]}`)
	resp, _ := http.Post(srv.URL+"/v1/responses", "application/json", bytes.NewReader(body))
	_ = resp.Body.Close()

	persisted, _ := Replay(rec, "alpha")
	if len(persisted) != 1 {
		t.Fatalf("expected 1 exchange, got %d", len(persisted))
	}
	ex := persisted[0]
	if ex.Profile != "alpha" || ex.SourceHint != "codex" || string(ex.Protocol) != "openai_responses" {
		t.Fatalf("missing metadata: %+v", ex)
	}
	if ex.Request.Bytes != int64(len(body)) {
		t.Fatalf("bytes = %d, want %d", ex.Request.Bytes, len(body))
	}
}

// --- Protocol observers -----------------------------------------------------

func TestAnthropic_ClassifiesComponents(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"system":"You are terse.",
		"messages":[
			{"role":"user","content":"hi"},
			{"role":"assistant","content":"hello"}
		],
		"tools":[{"name":"a"},{"name":"b"}]
	}`)
	components := AnthropicMessagesObserver{}.Parse(body)
	if len(components) == 0 {
		t.Fatalf("expected components")
	}
	kinds := map[model.ContextComponentKind]int{}
	for _, c := range components {
		kinds[c.Kind]++
		if c.Observation != ObservationScope {
			t.Fatalf("component observation = %q, want %q", c.Observation, ObservationScope)
		}
		if c.ContentHash == "" {
			t.Fatalf("component missing content hash")
		}
	}
	if kinds[model.ContextInstructions] == 0 {
		t.Fatalf("instructions not classified: %+v", kinds)
	}
	if kinds[model.ContextUserPrompt] == 0 {
		t.Fatalf("user prompt not classified: %+v", kinds)
	}
	if kinds[model.ContextHistory] == 0 {
		t.Fatalf("assistant history not classified: %+v", kinds)
	}
	if kinds[model.ContextToolDefinition] == 0 {
		t.Fatalf("tools not classified: %+v", kinds)
	}
}

func TestAnthropic_UnsupportedDetailsDegrade(t *testing.T) {
	obs := AnthropicMessagesObserver{}
	body := []byte(`{"weird":[1,2,3]}`)
	if comps := obs.Parse(body); len(comps) != 0 {
		t.Fatalf("expected empty components for unsupported payload, got %+v", comps)
	}
	body = []byte(`not json at all`)
	if comps := obs.Parse(body); len(comps) != 0 {
		t.Fatalf("expected empty components for non-JSON, got %+v", comps)
	}
}

func TestOpenAI_ClassifiesComponents(t *testing.T) {
	body := []byte(`{
		"model":"gpt-x",
		"instructions":"stay calm",
		"input":[
			{"type":"message","role":"user","content":[{"text":"hi"}]},
			{"type":"message","role":"assistant","content":[{"text":"hello"}]},
			{"type":"function_call_output","output":"ok"}
		],
		"tools":[{"name":"a"}]
	}`)
	obs := OpenAIResponsesObserver{}
	components := obs.Parse(body)
	kinds := map[model.ContextComponentKind]int{}
	for _, c := range components {
		kinds[c.Kind]++
		if c.Observation != ObservationScope {
			t.Fatalf("observation scope = %q", c.Observation)
		}
	}
	if kinds[model.ContextInstructions] == 0 {
		t.Fatalf("instructions missing: %+v", kinds)
	}
	if kinds[model.ContextUserPrompt] == 0 {
		t.Fatalf("user prompt missing: %+v", kinds)
	}
	if kinds[model.ContextHistory] == 0 {
		t.Fatalf("history missing: %+v", kinds)
	}
	if kinds[model.ContextToolResult] == 0 {
		t.Fatalf("tool result missing: %+v", kinds)
	}
	if kinds[model.ContextToolDefinition] == 0 {
		t.Fatalf("tool definitions missing: %+v", kinds)
	}
}

func TestOpenAI_UnsupportedDetailsDegrade(t *testing.T) {
	obs := OpenAIResponsesObserver{}
	if comps := obs.Parse([]byte(`garbage`)); len(comps) != 0 {
		t.Fatalf("expected empty components for garbage input")
	}
	if comps := obs.Parse([]byte(`{"weird":{}}`)); len(comps) != 0 {
		t.Fatalf("expected empty components for unsupported payload")
	}
}

func TestDedupeComponents(t *testing.T) {
	in := []model.ContextComponent{
		{Kind: model.ContextUserPrompt, Position: 0, ContentHash: "h1"},
		{Kind: model.ContextUserPrompt, Position: 1, ContentHash: "h1"},
		{Kind: model.ContextUserPrompt, Position: 2, ContentHash: "h2"},
	}
	out := dedupeComponents(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 components after dedupe, got %d", len(out))
	}
}

// --- Loopback binding -------------------------------------------------------

func TestIsLoopbackAddress(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8000", true},
		{"127.0.0.1", true},
		{":8000", true},
		{"0.0.0.0:8000", false},
		{"192.168.0.1:8000", false},
	}
	for _, c := range cases {
		if got := IsLoopbackAddress(c.addr); got != c.want {
			t.Fatalf("IsLoopbackAddress(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

// --- Sanitised upstream -----------------------------------------------------

func TestSanitisedUpstream(t *testing.T) {
	parsed, _ := url.Parse("http://user:pass@127.0.0.1:20128")
	got := sanitisedUpstream(parsed)
	if strings.Contains(got, "pass") || strings.Contains(got, "user") {
		t.Fatalf("credentials leaked: %s", got)
	}
}

// --- Concurrency -------------------------------------------------------------

type recordingRecorder struct {
	mu       sync.Mutex
	received []Exchange
}

func (r *recordingRecorder) Record(e Exchange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.received = append(r.received, e)
}

func (r *recordingRecorder) Summary(string) (Summary, error) { return Summary{}, nil }

func TestProxy_NoBlockingOnRecorder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	slow := &recordingRecorder{}
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, slow,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader([]byte(`{"model":"x"}`)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	_ = resp.Body.Close()
}

var _ Recorder = (*FileRecorder)(nil)
var _ Recorder = (*recordingRecorder)(nil)

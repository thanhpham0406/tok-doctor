package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProxy_AnthropicStreamingObservesUsageAndPreservesBytes(t *testing.T) {
	original := []byte(
		"event: message_start\n" +
			"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":120,\"output_tokens\":0,\"cache_read_input_tokens\":50}}}\n\n" +
			"event: content_block_delta\n" +
			"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
			"event: message_delta\n" +
			"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":13}}\n\n" +
			"event: message_stop\n" +
			"data: {\"type\":\"message_stop\"}\n\n",
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(original)
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
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(body, original) {
		t.Fatalf("downstream bytes do not match upstream\nwant=%q\ngot =%q", original, body)
	}

	persisted, err := Replay(rec, "p")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("recorded %d exchanges, want 1", len(persisted))
	}
	pu := persisted[0].Response.ProviderUsage
	if pu == nil {
		t.Fatalf("provider usage nil")
	}
	if pu.InputTokens == nil || *pu.InputTokens != 120 {
		t.Fatalf("input_tokens = %+v", pu.InputTokens)
	}
	if pu.OutputTokens == nil || *pu.OutputTokens != 13 {
		t.Fatalf("output_tokens = %+v", pu.OutputTokens)
	}
	if pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 50 {
		t.Fatalf("cache_read = %+v", pu.CacheReadInputTokens)
	}
}

func TestProxy_AnthropicStreamingWithoutMessageStopMarksTruncated(t *testing.T) {
	original := []byte(
		"event: message_start\n" +
			"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":7,\"output_tokens\":0}}}\n\n" +
			"event: message_delta\n" +
			"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":4}}\n\n",
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(original)
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

	resp, _ := http.Get(srv.URL + "/v1/messages")
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	persisted, _ := Replay(rec, "p")
	if len(persisted) != 1 {
		t.Fatalf("recorded %d, want 1", len(persisted))
	}
	ex := persisted[0]
	if !ex.Response.Usage.Truncated {
		t.Fatalf("expected truncated=true for missing message_stop, got %+v", ex.Response.Usage)
	}
	if ex.Response.ProviderUsage == nil || ex.Response.ProviderUsage.OutputTokens == nil || *ex.Response.ProviderUsage.OutputTokens != 4 {
		t.Fatalf("last snapshot usage not preserved: %+v", ex.Response.ProviderUsage)
	}
	if ex.Outcome != OutcomeStreamTruncated {
		t.Fatalf("Outcome = %q, want %q", ex.Outcome, OutcomeStreamTruncated)
	}
}

func TestProxy_AnthropicStreamingParallelExchangesDoNotShareUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		var input, output int64
		if strings.Contains(r.URL.RawQuery, "a=") {
			input = 10
			output = 1
		} else {
			input = 20
			output = 2
		}
		_, _ = fmt.Fprintf(w,
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":%d,\"output_tokens\":0}}}\n\n"+
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":%d}}\n\n"+
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			input, output)
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

	makeReq := func(query string) *http.Response {
		resp, err := http.Get(srv.URL + "/v1/messages?" + query)
		if err != nil {
			t.Fatalf("get %s: %v", query, err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp
	}
	makeReq("a=1")
	makeReq("b=2")

	persisted, _ := Replay(rec, "p")
	if len(persisted) != 2 {
		t.Fatalf("recorded %d, want 2", len(persisted))
	}
	seen := map[int64]bool{}
	for _, ex := range persisted {
		pu := ex.Response.ProviderUsage
		if pu == nil || pu.InputTokens == nil || pu.OutputTokens == nil {
			t.Fatalf("usage missing: %+v", pu)
		}
		key := *pu.InputTokens*1000 + *pu.OutputTokens
		seen[key] = true
	}
	if !seen[10001] || !seen[20002] {
		t.Fatalf("usage values leaked across concurrent streams: %v", seen)
	}
}

func TestProxy_AnthropicStreamingMalformedFrameDoesNotKillExchange(t *testing.T) {
	original := []byte(
		"event: message_start\ndata: {not json\n\n" +
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":99,\"output_tokens\":0}}}\n\n" +
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":11}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(original)
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

	resp, _ := http.Get(srv.URL + "/v1/messages")
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	persisted, _ := Replay(rec, "p")
	if len(persisted) != 1 {
		t.Fatalf("recorded %d, want 1", len(persisted))
	}
	pu := persisted[0].Response.ProviderUsage
	if pu == nil || pu.InputTokens == nil || *pu.InputTokens != 99 || pu.OutputTokens == nil || *pu.OutputTokens != 11 {
		t.Fatalf("usage lost after malformed frame: %+v", pu)
	}
}

func TestProxy_AnthropicJSONNonStream(t *testing.T) {
	body := []byte(`{"id":"msg_1","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":300,"output_tokens":7,"cache_read_input_tokens":50,"cache_creation_input_tokens":12}}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
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

	resp, _ := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader([]byte(`{"model":"c","messages":[{"role":"user","content":"hi"}]}`)))
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	persisted, _ := Replay(rec, "p")
	if len(persisted) != 1 {
		t.Fatalf("recorded %d, want 1", len(persisted))
	}
	pu := persisted[0].Response.ProviderUsage
	if pu == nil || pu.InputTokens == nil || *pu.InputTokens != 300 || pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 50 || pu.CacheCreationInputTokens == nil || *pu.CacheCreationInputTokens != 12 {
		t.Fatalf("anthropic json usage missing: %+v", pu)
	}
	if persisted[0].Outcome != OutcomeUpstreamOK {
		t.Fatalf("Outcome = %q, want %q", persisted[0].Outcome, OutcomeUpstreamOK)
	}
}

func TestProxy_AnthropicNonStreamLargeBodyForwardedUnchangedAndUsageUnavailable(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), MaxCaptureBytes+1024)
	body := append([]byte(`{"id":"msg_huge","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":300,"output_tokens":7}}`), huge...)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
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

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader([]byte(`{"model":"c","messages":[{"role":"user","content":"hi"}]}`)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, body) {
		t.Fatalf("downstream body bytes do not match upstream, length got=%d want=%d", len(got), len(body))
	}

	persisted, _ := Replay(rec, "p")
	if len(persisted) != 1 {
		t.Fatalf("recorded %d, want 1", len(persisted))
	}
	if persisted[0].Response.ProviderUsage != nil {
		t.Fatalf("usage from truncated prefix should not be trusted; got %+v", persisted[0].Response.ProviderUsage)
	}
	if persisted[0].Response.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", persisted[0].Response.Status)
	}
}

func TestProxy_AnthropicNonStreamBodyPreservedOnUpstreamFourXX(t *testing.T) {
	body := []byte(`{"error":{"type":"validation","message":"bad"}}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
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

	resp, _ := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader([]byte(`{"model":"c","messages":[]}`)))
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, body) {
		t.Fatalf("upstream 4xx body must reach client unchanged: %s vs %s", got, body)
	}
	persisted, _ := Replay(rec, "p")
	if len(persisted) != 1 || persisted[0].Response.Status != 400 || persisted[0].Outcome != OutcomeUpstreamHTTPError {
		t.Fatalf("expected single exchange with upstream_http_error outcome, got %+v", persisted)
	}
}

func TestProxy_RecorderFailureNotSwallowed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	r := &failingRecorder{failOn: 1}
	defer func() { _ = r.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, r,
	)
	proxy.Sink = &sinkCollector{}
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader([]byte(`{"model":"x","messages":[]}`)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(got, []byte(`"usage"`)) {
		t.Fatalf("client must still receive upstream body even when recorder fails, got %s", got)
	}
	if r.calls != 1 {
		t.Fatalf("recorder saw %d calls, want 1", r.calls)
	}
}

func TestProxy_RecorderReceivesExactlyOneCall(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name: "stream_eof",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n"))
			},
		},
		{
			name: "non_stream_ok",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"usage":{"input_tokens":1,"output_tokens":1}}`))
			},
		},
		{
			name: "upstream_4xx",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"bad"}`))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listen, _ := freeLoopback(t)
			rr := &countingRecorder{}
			proxy, _ := NewProxy(
				Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "openai_responses", Source: "codex", Upstream: upstreamURL(tc.handler)},
				OpenAIResponsesObserver{}, rr,
			)
			srv := httptest.NewServer(proxy.Handler())
			defer srv.Close()
			resp, _ := http.Post(srv.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"x","input":[]}`))
			_, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if rr.calls != 1 {
				t.Fatalf("recorded %d, want 1", rr.calls)
			}
		})
	}
}

func upstreamURL(handler http.HandlerFunc) string {
	srv := httptest.NewServer(handler)
	return srv.URL
}

type countingRecorder struct {
	mu    sync.Mutex
	calls int
}

func (r *countingRecorder) Record(Exchange) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return nil
}

func (r *countingRecorder) Summary(string) (Summary, error) { return Summary{}, nil }

func (r *countingRecorder) Close() error { return nil }

type failingRecorder struct {
	mu     sync.Mutex
	calls  int
	failOn int
}

func (r *failingRecorder) Record(Exchange) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.calls == r.failOn {
		return &RecorderError{Profile: "p", Op: "open", Err: errFake}
	}
	return nil
}

func (r *failingRecorder) Summary(string) (Summary, error) { return Summary{}, nil }

func (r *failingRecorder) Close() error { return nil }

var errFake = bytes.ErrTooLarge

type sinkCollector struct {
	mu   sync.Mutex
	hits int
	last *RecorderError
}

func (s *sinkCollector) RecordRecorderFailure(_ string, err *RecorderError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits++
	s.last = err
}

func TestAccountProfile_AnthropicTokensAreCanonicalized(t *testing.T) {
	in := int64(100)
	out := int64(30)
	cc := int64(20)
	cr := int64(800)
	exchange := Exchange{
		ID:        "gw-canon",
		Profile:   "p",
		Protocol:  ProtocolAnthropicMessages,
		StartedAt: time.Unix(0, 0),
		Request: ExchangeRequest{
			Method:   http.MethodPost,
			Endpoint: "/v1/messages",
		},
		Response: ExchangeResponse{ProviderUsage: &ProviderUsage{
			Source:                   "anthropic_messages",
			InputTokens:              &in,
			CacheCreationInputTokens: &cc,
			CacheReadInputTokens:     &cr,
			OutputTokens:             &out,
		}},
	}
	account := AccountProfile("p", []Exchange{exchange}, ChainBuildResult{}, RecorderFailureRead{})
	if account.Observed.TotalInput.Sum != 920 {
		t.Fatalf("observed.TotalInput = %d, want 920", account.Observed.TotalInput.Sum)
	}
	if account.Observed.Total.Sum != 950 {
		t.Fatalf("observed.Total = %d, want 950", account.Observed.Total.Sum)
	}
}

func TestAccountProfile_PerFieldCompletenessForMissingFields(t *testing.T) {
	inOne := int64(100)
	outOne := int64(20)
	inTwo := int64(50)
	exchanges := []Exchange{
		{
			ID: "gw-a", Profile: "p", Protocol: ProtocolAnthropicMessages, StartedAt: time.Unix(0, 0),
			Kind: RequestKindModel, Outcome: OutcomeUpstreamOK,
			Request:  ExchangeRequest{Method: http.MethodPost, Endpoint: "/v1/messages"},
			Response: ExchangeResponse{ProviderUsage: &ProviderUsage{Source: string(ProtocolAnthropicMessages), InputTokens: &inOne, OutputTokens: &outOne}},
		},
		{
			ID: "gw-b", Profile: "p", Protocol: ProtocolAnthropicMessages, StartedAt: time.Unix(1, 0),
			Kind: RequestKindModel, Outcome: OutcomeUpstreamOK,
			Request:  ExchangeRequest{Method: http.MethodPost, Endpoint: "/v1/messages"},
			Response: ExchangeResponse{ProviderUsage: &ProviderUsage{Source: string(ProtocolAnthropicMessages), InputTokens: &inTwo}},
		},
	}
	account := AccountProfile("p", exchanges, ChainBuildResult{}, RecorderFailureRead{})
	if account.Observed.Complete {
		t.Fatalf("expected aggregate.Observed.Complete=false when output missing on one request")
	}
	if account.Completeness != "partial" {
		t.Fatalf("completeness = %q, want partial", account.Completeness)
	}
	if account.Observed.RawInput.Count != 2 {
		t.Fatalf("RawInput.Count = %d, want 2", account.Observed.RawInput.Count)
	}
	if account.Observed.Output.Count != 1 {
		t.Fatalf("Output.Count = %d, want 1", account.Observed.Output.Count)
	}
}

func TestAccountProfile_ChainedCompleteUncorrelatedMissingIsPartial(t *testing.T) {
	in := int64(50)
	out := int64(20)
	a := Exchange{
		ID: "gw-a", Profile: "p", Protocol: ProtocolAnthropicMessages, StartedAt: time.Unix(0, 0),
		Kind: RequestKindModel, Outcome: OutcomeUpstreamOK,
		Request:  ExchangeRequest{Method: http.MethodPost, Endpoint: "/v1/messages"},
		Response: ExchangeResponse{ProviderUsage: &ProviderUsage{Source: string(ProtocolAnthropicMessages), InputTokens: &in, OutputTokens: &out}},
	}
	b := Exchange{
		ID: "gw-b", Profile: "p", Protocol: ProtocolAnthropicMessages, StartedAt: time.Unix(1, 0),
		Kind: RequestKindModel, Outcome: OutcomeUpstreamOK,
		Request:  ExchangeRequest{Method: http.MethodPost, Endpoint: "/v1/messages"},
		Response: ExchangeResponse{ProviderUsage: &ProviderUsage{OutputTokens: &out}},
	}
	chain := Chain{
		ID: "gwc-a", Profile: "p", Protocol: ProtocolAnthropicMessages,
		Exchanges: []ChainExchange{{ID: "gw-a", StartedAt: "1970-01-01T00:00:00Z"}},
	}
	build := ChainBuildResult{Chains: []Chain{chain}}
	account := AccountProfile("p", []Exchange{a, b}, build, RecorderFailureRead{})
	if !account.Chained.Complete {
		t.Fatalf("Chained aggregate covers only a and should be complete: %+v", account.Chained)
	}
	if account.Uncorrelated.Complete {
		t.Fatalf("Uncorrelated aggregate should be incomplete: missing request b input")
	}
	if account.Completeness != "partial" {
		t.Fatalf("Completeness = %q, want partial", account.Completeness)
	}
}

func TestAccountProfile_TruncatedStreamMarksPartialEvenWithSnapshot(t *testing.T) {
	in := int64(10)
	out := int64(2)
	ex := Exchange{
		ID: "gw-trunc", Profile: "p", Protocol: ProtocolAnthropicMessages, StartedAt: time.Unix(0, 0),
		Kind: RequestKindModel, Outcome: OutcomeStreamTruncated,
		Request: ExchangeRequest{Method: http.MethodPost, Endpoint: "/v1/messages"},
		Response: ExchangeResponse{
			ProviderUsage: &ProviderUsage{Source: string(ProtocolAnthropicMessages), InputTokens: &in, OutputTokens: &out},
			Usage:         &ObservedUsage{Truncated: true},
		},
	}
	account := AccountProfile("p", []Exchange{ex}, ChainBuildResult{}, RecorderFailureRead{})
	if account.Completeness != "partial" {
		t.Fatalf("Completeness = %q, want partial (truncated)", account.Completeness)
	}
	if account.Outcomes.OutcomeStreamTruncated != 1 {
		t.Fatalf("stream_truncated outcome count = %d, want 1", account.Outcomes.OutcomeStreamTruncated)
	}
}

func TestAccountProfile_ExchangesNoModelRequestsIsComplete(t *testing.T) {
	account := AccountProfile("p", nil, ChainBuildResult{}, RecorderFailureRead{})
	if !account.Observed.Complete {
		t.Fatalf("expected complete when no model requests")
	}
	if account.Completeness != "complete" {
		t.Fatalf("Completeness = %q, want complete", account.Completeness)
	}
}

func TestAccountProfile_OutcomesAggregated(t *testing.T) {
	make := func(id string, outcome RequestOutcome) Exchange {
		return Exchange{
			ID: id, Profile: "p", Protocol: ProtocolAnthropicMessages, StartedAt: time.Unix(0, 0),
			Kind: RequestKindModel, Outcome: outcome,
			Request: ExchangeRequest{Method: http.MethodPost, Endpoint: "/v1/messages"},
		}
	}
	exs := []Exchange{
		make("gw-a", OutcomeUpstreamOK),
		make("gw-b", OutcomeUpstreamHTTPError),
		make("gw-c", OutcomeTransportFailure),
		make("gw-d", OutcomeClientCanceled),
		make("gw-e", OutcomeStreamTruncated),
		make("gw-f", OutcomeUnknown),
	}
	account := AccountProfile("p", exs, ChainBuildResult{}, RecorderFailureRead{})
	if account.Outcomes.OutcomeUpstreamOK != 1 || account.Outcomes.OutcomeUpstreamHTTPError != 1 ||
		account.Outcomes.OutcomeTransportFailure != 1 || account.Outcomes.OutcomeClientCanceled != 1 ||
		account.Outcomes.OutcomeStreamTruncated != 1 || account.Outcomes.OutcomeUnknown != 1 {
		t.Fatalf("outcome buckets not aggregated: %+v", account.Outcomes)
	}
}

func TestRender_ReportJSONContainsBucketCountsOutcomesAndCompleteness(t *testing.T) {
	in := int64(100)
	out := int64(30)
	ex := Exchange{
		ID: "gw-rend", Profile: "p", Protocol: ProtocolAnthropicMessages, StartedAt: time.Unix(0, 0),
		Kind: RequestKindModel, Outcome: OutcomeUpstreamOK,
		Request:  ExchangeRequest{Method: http.MethodPost, Endpoint: "/v1/messages"},
		Response: ExchangeResponse{ProviderUsage: &ProviderUsage{Source: string(ProtocolAnthropicMessages), InputTokens: &in, OutputTokens: &out}},
	}
	nonModel := Exchange{
		ID: "gw-rend2", Profile: "p", Protocol: ProtocolAnthropicMessages, StartedAt: time.Unix(1, 0),
		Kind: RequestKindNonModel, Outcome: OutcomeUpstreamOK,
		Request: ExchangeRequest{Method: http.MethodGet, Endpoint: "/v1/models"},
	}
	account := AccountProfile("p", []Exchange{ex, nonModel}, ChainBuildResult{}, RecorderFailureRead{})
	raw, err := json.Marshal(account)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	compact := strings.Join(strings.Fields(string(raw)), "")
	for _, want := range []string{`"httpRequests":2`, `"modelRequests":1`, `"nonModelRequests":1`, `"upstream_ok":2`, `"completeness":"complete"`} {
		if !strings.Contains(compact, want) {
			t.Fatalf("json missing %s: %s", want, compact)
		}
	}
}

func TestProxy_RecorderFailureKeepsClientResponseAndBuffersFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_1","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	// Force every Record to fail by pre-creating the capture JSONL as a
	// directory. The failure journal path is unaffected, so each Record
	// must append exactly one entry there.
	if err := os.Mkdir(filepath.Join(dir, "p.jsonl"), 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	rec, _ := NewFileRecorder(dir)
	defer func() { _ = rec.Close() }()

	listen, _ := freeLoopback(t)
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"x","messages":[]}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(got, []byte(`"usage"`)) {
		t.Fatalf("client must still receive upstream body, got %s", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	failures, err := rec.RecorderFailures("p")
	if err != nil {
		t.Fatalf("read failures: %v", err)
	}
	if len(failures.Failures) != 1 {
		t.Fatalf("recorder failures len = %d, want 1 (no duplicate)", len(failures.Failures))
	}
	if failures.Failures[0].Operation != "open" {
		t.Fatalf("operation = %q, want open", failures.Failures[0].Operation)
	}
	if failures.Failures[0].ExchangeID == "" {
		t.Fatalf("exchange id must be set")
	}
}

func TestProxy_StillReturns200OnRecorderFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	r := &failingRecorder{failOn: 1}
	defer func() { _ = r.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, r,
	)
	proxy.Sink = &sinkCollector{}
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"x","messages":[]}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 even when recorder fails", resp.StatusCode)
	}
}

func TestProxy_StreamRecorderFailureNotifiedExactlyOnce(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_f\",\"usage\":{\"input_tokens\":1}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	r := &failingRecorder{failOn: 1}
	defer func() { _ = r.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, r,
	)
	var hits int32
	var mu sync.Mutex
	proxy.Sink = sinkFunc(func(_ string, _ *RecorderError) {
		mu.Lock()
		defer mu.Unlock()
		hits = 1
	})
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"x","messages":[]}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	mu.Lock()
	got := hits
	mu.Unlock()
	if got != 1 {
		t.Fatalf("stream record failure sink hits = %d, want 1", got)
	}
}

type sinkFunc func(string, *RecorderError)

func (f sinkFunc) RecordRecorderFailure(profile string, err *RecorderError) { f(profile, err) }

var _ = time.Unix

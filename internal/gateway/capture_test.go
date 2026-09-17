package gateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func newCaptureProxy(t *testing.T, observer Observer, upstream string) (*Proxy, string) {
	t.Helper()
	listen, _ := freeLoopback(t)
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("NewFileRecorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })
	proxy, err := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream},
		observer, rec,
	)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	return proxy, dir
}

const captureSecret = "SECRET_REQUEST_LITERAL_DO_NOT_FORWARD_TWICE_OR_PERSIST"

// A request above the capture limit must still reach upstream byte for byte.
// The limit only bounds the observation, never the forwarded body.
func TestProxy_RequestAboveCaptureLimitReachesUpstreamUnchanged(t *testing.T) {
	body := []byte(`{"model":"claude-x","messages":[{"role":"user","content":"` +
		captureSecret + strings.Repeat("a", MaxCaptureBytes) + `"}]}`)
	if len(body) <= MaxCaptureBytes {
		t.Fatalf("fixture body is %d bytes, want more than the capture limit", len(body))
	}

	received := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		received <- got
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	proxy, dir := newCaptureProxy(t, AnthropicMessagesObserver{}, upstream.URL)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the oversized request to be forwarded", resp.StatusCode)
	}
	if got := <-received; !bytes.Equal(got, body) {
		t.Fatalf("upstream body differs: got %d bytes want %d", len(got), len(body))
	}

	persisted, err := Replay(proxy.Recorder.(*FileRecorder), "p")
	if err != nil || len(persisted) != 1 {
		t.Fatalf("persisted = %d (err %v), want 1", len(persisted), err)
	}
	exchange := persisted[0]
	capture := exchange.Request.Capture
	if capture == nil || capture.State != model.ContextCompletenessTruncated {
		t.Fatalf("capture = %+v, want truncated", capture)
	}
	if capture.CapturedBytes != MaxCaptureBytes || capture.TotalBytes != int64(len(body)) || capture.LimitBytes != MaxCaptureBytes {
		t.Fatalf("capture sizes = %+v, want captured=%d total=%d limit=%d", capture, MaxCaptureBytes, len(body), MaxCaptureBytes)
	}
	if len(exchange.Request.Components) != 0 {
		t.Fatalf("components = %+v, want none parsed from a partial body", exchange.Request.Components)
	}
	if exchange.Request.Bytes != int64(len(body)) {
		t.Fatalf("request bytes = %d, want %d measured while forwarding", exchange.Request.Bytes, len(body))
	}
	if exchange.Request.BodyHash != contentHash(body) {
		t.Fatalf("body hash = %q, want the hash of the forwarded body", exchange.Request.BodyHash)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "p.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), captureSecret) {
		t.Fatal("persisted capture contains raw request content")
	}
}

// A request with no declared length is forwarded as a stream. The capture is
// still bounded, and the streamed size replaces the unknown declared one.
func TestProxy_RequestWithoutDeclaredLengthIsForwardedAndMeasured(t *testing.T) {
	body := []byte(`{"model":"claude-x","messages":[{"role":"user","content":"` +
		strings.Repeat("a", MaxCaptureBytes) + `"}]}`)

	received := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		received <- got
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	proxy, _ := newCaptureProxy(t, AnthropicMessagesObserver{}, upstream.URL)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.ContentLength = -1
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()

	if got := <-received; !bytes.Equal(got, body) {
		t.Fatalf("upstream body differs: got %d bytes want %d", len(got), len(body))
	}
	persisted, err := Replay(proxy.Recorder.(*FileRecorder), "p")
	if err != nil || len(persisted) != 1 {
		t.Fatalf("persisted = %d (err %v), want 1", len(persisted), err)
	}
	capture := persisted[0].Request.Capture
	if capture == nil || capture.State != model.ContextCompletenessTruncated {
		t.Fatalf("capture = %+v, want truncated", capture)
	}
	if capture.TotalBytes != 0 {
		t.Fatalf("total bytes = %d, want unset when the client declared no length", capture.TotalBytes)
	}
	if persisted[0].Request.Bytes != int64(len(body)) {
		t.Fatalf("request bytes = %d, want %d measured while forwarding", persisted[0].Request.Bytes, len(body))
	}
	if persisted[0].Request.BodyHash != contentHash(body) {
		t.Fatalf("body hash = %q, want the hash of the forwarded body", persisted[0].Request.BodyHash)
	}
}

func TestProxy_RequestWithinCaptureLimitIsObservedCompletely(t *testing.T) {
	body := []byte(`{"model":"claude-x","messages":[{"role":"user","content":"hi"}]}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	proxy, _ := newCaptureProxy(t, AnthropicMessagesObserver{}, upstream.URL)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()

	persisted, err := Replay(proxy.Recorder.(*FileRecorder), "p")
	if err != nil || len(persisted) != 1 {
		t.Fatalf("persisted = %d (err %v), want 1", len(persisted), err)
	}
	capture := persisted[0].Request.Capture
	if capture == nil || capture.State != model.ContextCompletenessComplete {
		t.Fatalf("capture = %+v, want complete", capture)
	}
	if capture.CapturedBytes != int64(len(body)) || capture.TotalBytes != int64(len(body)) {
		t.Fatalf("capture sizes = %+v, want the whole body captured", capture)
	}
	if len(persisted[0].Request.Components) == 0 {
		t.Fatal("components = none, want the captured body parsed")
	}
}

func TestProxy_CaptureIsUnavailableWithoutObserver(t *testing.T) {
	body := []byte(`{"model":"claude-x","messages":[{"role":"user","content":"hi"}]}`)
	received := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		received <- got
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	proxy, _ := newCaptureProxy(t, nil, upstream.URL)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()

	if got := <-received; !bytes.Equal(got, body) {
		t.Fatalf("upstream body differs: got %q", got)
	}
	persisted, err := Replay(proxy.Recorder.(*FileRecorder), "p")
	if err != nil || len(persisted) != 1 {
		t.Fatalf("persisted = %d (err %v), want 1", len(persisted), err)
	}
	capture := persisted[0].Request.Capture
	if capture == nil || capture.State != model.ContextCompletenessUnavailable {
		t.Fatalf("capture = %+v, want unavailable without an observer", capture)
	}
}

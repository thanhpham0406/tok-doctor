package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxy_AnthropicBodyIDInResponseObjectIDNotOpenAIResponses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_x","content":[{"text":"ok"}]}`))
	}))
	defer upstream.Close()
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	listen, _ := freeLoopback(t)
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()
	_, _ = http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"x","messages":[]}`))
	persisted, _ := Replay(rec, "p")
	if persisted[0].Response.ResponseObjectID != "msg_x" {
		t.Fatalf("ResponseObjectID = %q, want msg_x", persisted[0].Response.ResponseObjectID)
	}
	if persisted[0].Response.OpenAIResponses != nil {
		t.Fatalf("Anthropic body must NOT populate OpenAIResponses, got %+v", persisted[0].Response.OpenAIResponses)
	}
}

func TestProxy_OpenAIBodyIDPopulatesBoth(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_42","output":[{"type":"function_call","call_id":"c1"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	listen, _ := freeLoopback(t)
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "openai_responses", Source: "codex", Upstream: upstream.URL},
		OpenAIResponsesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()
	_, _ = http.Post(srv.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"m","input":[]}`))
	persisted, _ := Replay(rec, "p")
	if persisted[0].Response.ResponseObjectID != "resp_42" {
		t.Fatalf("ResponseObjectID = %q, want resp_42", persisted[0].Response.ResponseObjectID)
	}
	if persisted[0].Response.OpenAIResponses == nil {
		t.Fatalf("OpenAIResponses must be populated for OpenAI")
	}
	if persisted[0].Response.OpenAIResponses.ResponseID != "resp_42" {
		t.Fatalf("OpenAIResponses.ResponseID = %q, want resp_42", persisted[0].Response.OpenAIResponses.ResponseID)
	}
	if len(persisted[0].Response.OpenAIResponses.OutputItemCallIDs) != 1 ||
		persisted[0].Response.OpenAIResponses.OutputItemCallIDs[0] != "c1" {
		t.Fatalf("OpenAIResponses.OutputItemCallIDs = %+v", persisted[0].Response.OpenAIResponses.OutputItemCallIDs)
	}
}

func TestStreamObserver_AnthropicSSECapturesMessageID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`event: message_start
data: {"type":"message_start","message":{"id":"msg_stream","usage":{"input_tokens":1,"output_tokens":0}}}

event: message_delta
data: {"type":"message_delta","usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}

`))
	}))
	defer upstream.Close()
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	listen, _ := freeLoopback(t)
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"x","messages":[]}`))
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "message_stop") {
		t.Fatalf("client did not receive full stream: %s", body)
	}
	persisted, _ := Replay(rec, "p")
	if persisted[0].Response.ResponseObjectID != "msg_stream" {
		t.Fatalf("ResponseObjectID = %q, want msg_stream", persisted[0].Response.ResponseObjectID)
	}
	if persisted[0].Response.ProviderUsage == nil ||
		persisted[0].Response.ProviderUsage.OutputTokens == nil ||
		*persisted[0].Response.ProviderUsage.OutputTokens != 7 {
		t.Fatalf("usage from delta not merged: %+v", persisted[0].Response.ProviderUsage)
	}
}

func TestStreamObserver_OpenAIResponseIDAcrossFrames(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`event: response.created
data: {"type":"response.created","response":{"id":"resp_stream"},"usage":{"input_tokens":1,"output_tokens":0}}

event: response.usage
data: {"type":"response.usage","response":{"id":"resp_stream"},"usage":{"input_tokens":1,"output_tokens":11}}

`))
	}))
	defer upstream.Close()
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	listen, _ := freeLoopback(t)
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "openai_responses", Source: "codex", Upstream: upstream.URL},
		OpenAIResponsesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"m","input":[]}`))
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	persisted, _ := Replay(rec, "p")
	if persisted[0].Response.ResponseObjectID != "resp_stream" {
		t.Fatalf("ResponseObjectID = %q, want resp_stream", persisted[0].Response.ResponseObjectID)
	}
	if persisted[0].Response.ProviderUsage == nil ||
		persisted[0].Response.ProviderUsage.OutputTokens == nil ||
		*persisted[0].Response.ProviderUsage.OutputTokens != 11 {
		t.Fatalf("usage from later frame not merged: %+v", persisted[0].Response.ProviderUsage)
	}
}

func TestStreamObserver_ConcurrentStreamsDoNotShareID(t *testing.T) {
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`event: message_start
data: {"type":"message_start","message":{"id":"msg_a"}}

`))
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`event: message_start
data: {"type":"message_start","message":{"id":"msg_b"}}

`))
	}))
	defer upstreamB.Close()

	recA, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = recA.Close() }()
	listenA, _ := freeLoopback(t)
	proxyA, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listenA, Protocol: "anthropic_messages", Source: "claude", Upstream: upstreamA.URL},
		AnthropicMessagesObserver{}, recA,
	)
	srvA := httptest.NewServer(proxyA.Handler())
	defer srvA.Close()

	recB, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = recB.Close() }()
	listenB, _ := freeLoopback(t)
	proxyB, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listenB, Protocol: "anthropic_messages", Source: "claude", Upstream: upstreamB.URL},
		AnthropicMessagesObserver{}, recB,
	)
	srvB := httptest.NewServer(proxyB.Handler())
	defer srvB.Close()

	respA, err := http.Post(srvA.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"x","messages":[]}`))
	if err != nil {
		t.Fatalf("post a: %v", err)
	}
	_, _ = io.ReadAll(respA.Body)
	_ = respA.Body.Close()
	respB, err := http.Post(srvB.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"x","messages":[]}`))
	if err != nil {
		t.Fatalf("post b: %v", err)
	}
	_, _ = io.ReadAll(respB.Body)
	_ = respB.Body.Close()

	pa, _ := Replay(recA, "p")
	pb, _ := Replay(recB, "p")
	if pa[0].Response.ResponseObjectID == pb[0].Response.ResponseObjectID {
		t.Fatalf("streams shared id %q", pa[0].Response.ResponseObjectID)
	}
	if pa[0].Response.ResponseObjectID != "msg_a" || pb[0].Response.ResponseObjectID != "msg_b" {
		t.Fatalf("ids crossed: a=%q b=%q", pa[0].Response.ResponseObjectID, pb[0].Response.ResponseObjectID)
	}
}

func TestAnthropicParseStreamFrame_MalformedKeepsEarlierID(t *testing.T) {
	state := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}
	obs.ParseStreamFrame(state, []byte(`{"type":"message_start","message":{"id":"msg_keep","usage":{"input_tokens":1,"output_tokens":2}}}`))
	obs.ParseStreamFrame(state, []byte(`{not json`))
	obs.ParseStreamFrame(state, []byte(`{"type":"message_delta","usage":{"output_tokens":3}}`))
	if state.messageID != "msg_keep" {
		t.Fatalf("malformed frame cleared message.id: %q", state.messageID)
	}
}

func TestStreamObserver_BytesForwardedUnchanged(t *testing.T) {
	upstreamBody := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_bytes\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":4}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamBody))
	}))
	defer upstream.Close()
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	listen, _ := freeLoopback(t)
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "anthropic_messages", Source: "claude", Upstream: upstream.URL},
		AnthropicMessagesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"x","messages":[]}`))
	got, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(got) != upstreamBody {
		t.Fatalf("client body differs from upstream\nclient:\n%s\nupstream:\n%s", got, upstreamBody)
	}
}

func TestExchange_SchemaVersionRoundTrip(t *testing.T) {
	ex := Exchange{
		SchemaVersion: ExchangeSchemaVersion,
		ID:            "gw-schema",
		Profile:       "p",
		Protocol:      ProtocolOpenAIResponses,
		StartedAt:     time.Unix(0, 0),
	}
	raw, err := json.Marshal(ex)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"schemaVersion":1`) {
		t.Fatalf("missing schemaVersion:1 in %s", raw)
	}
	var decoded Exchange
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.SchemaVersion != 1 {
		t.Fatalf("schemaVersion round-trip lost: %d", decoded.SchemaVersion)
	}
}

func TestExchange_BothIdentityAbsent(t *testing.T) {
	ex := Exchange{
		ID:        "gw-empty",
		Profile:   "p",
		Protocol:  ProtocolOpenAIResponses,
		StartedAt: time.Unix(0, 0),
	}
	if ex.Response.ProviderRequestID != "" || ex.Response.ResponseObjectID != "" {
		t.Fatalf("expected empty identity fields, got %+v", ex.Response)
	}
}

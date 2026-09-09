package gateway

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func readStreamThroughObserver(t *testing.T, upstream io.Reader, bufferSize int) []byte {
	t.Helper()
	streamer := newStreamObserver(OpenAIResponsesObserver{}, io.NopCloser(upstream), nil, nil)
	var got []byte
	buf := make([]byte, bufferSize)
	for {
		n, err := streamer.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return got
}

func readStreamAndCapture(t *testing.T, upstream io.Reader, bufferSize int) (*streamObserver, []byte) {
	t.Helper()
	ex := Exchange{ID: "gw-x", Profile: "p", Protocol: ProtocolOpenAIResponses}
	streamer := newStreamObserver(OpenAIResponsesObserver{}, io.NopCloser(upstream), nil, &ex)
	var got []byte
	buf := make([]byte, bufferSize)
	for {
		n, err := streamer.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return streamer, got
}

func TestStreamObserver_ExactBytePreservation_MultipleEvents(t *testing.T) {
	parts := []string{
		"event: response.created\ndata: {\"type\":\"response.created\"}\n\n",
		"data: hello world\n\n",
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n",
		"data: [DONE]\n\n",
	}
	original := []byte(strings.Join(parts, ""))
	got := readStreamThroughObserver(t, bytes.NewReader(original), 64*1024)
	if !bytes.Equal(got, original) {
		t.Fatalf("downstream bytes do not match upstream\nwant=%q\ngot =%q", original, got)
	}
}

func TestStreamObserver_SSEDelimitersPreserved_CRLF(t *testing.T) {
	original := []byte("event: response.created\r\ndata: {\"type\":\"response.created\"}\r\n\r\ndata: hello\r\n\r\n")
	got := readStreamThroughObserver(t, bytes.NewReader(original), 4096)
	if !bytes.Equal(got, original) {
		t.Fatalf("CRLF delimiters not preserved\nwant=%q\ngot =%q", original, got)
	}
}

func TestStreamObserver_SmallCallerBuffer_LargeEvent(t *testing.T) {
	payload := strings.Repeat("x", 200*1024)
	frame := "data: " + payload + "\n\n"
	terminal := "event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":5,\"output_tokens\":7}}\n\n"
	original := []byte(frame + terminal)
	got := readStreamThroughObserver(t, bytes.NewReader(original), 4096)
	if !bytes.Equal(got, original) {
		t.Fatalf("large event was truncated\nwant_len=%d got_len=%d", len(original), len(got))
	}
}

func TestStreamObserver_OversizedEventContinuesStream(t *testing.T) {
	big := strings.Repeat("a", int(float64(maxSSEResponseEventBytes)*1.5))
	hugeFrame := []byte("data: " + big + "\n\n")
	terminal := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":42,\"output_tokens\":3}}\n\n")
	original := append(append([]byte{}, hugeFrame...), terminal...)
	got := readStreamThroughObserver(t, bytes.NewReader(original), 16*1024)
	if !bytes.Equal(got, original) {
		t.Fatalf("oversized event broke transport\nwant_len=%d got_len=%d", len(original), len(got))
	}
}

func TestStreamObserver_MalformedSSEDoesNotBreakTransport(t *testing.T) {
	original := []byte("this is not sse at all\n\n" +
		"event: response.completed\ndata: {not valid json\n\n" +
		"data: [DONE]\n\n")
	got := readStreamThroughObserver(t, bytes.NewReader(original), 4096)
	if !bytes.Equal(got, original) {
		t.Fatalf("malformed SSE altered bytes\nwant=%q\ngot =%q", original, got)
	}
}

func TestStreamObserver_ValidTerminalUsage(t *testing.T) {
	original := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":111,\"output_tokens\":22,\"input_tokens_details\":{\"cached_tokens\":80},\"output_tokens_details\":{\"reasoning_tokens\":5}}}\n\n")
	streamer, _ := readStreamAndCapture(t, bytes.NewReader(original), 4096)
	usage := streamer.Usage()
	if usage == nil || usage.Input == nil || *usage.Input != 111 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.Output == nil || *usage.Output != 22 {
		t.Fatalf("output = %+v", usage.Output)
	}
}

func TestStreamObserver_DelimiterSplitAcrossChunks(t *testing.T) {
	chunk1 := []byte("data: hello\n")
	chunk2 := []byte("\n")
	chunk3 := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":7,\"output_tokens\":3}}\n\n")
	original := append(append(append([]byte{}, chunk1...), chunk2...), chunk3...)
	upstream := &splitReader{chunks: [][]byte{chunk1, chunk2, chunk3}}
	streamer, got := readStreamAndCapture(t, upstream, 4096)
	if !bytes.Equal(got, original) {
		t.Fatalf("delimiter-split chunks altered bytes\nwant=%q\ngot =%q", original, got)
	}
	usage := streamer.Usage()
	if usage == nil || usage.Input == nil || *usage.Input != 7 {
		t.Fatalf("usage after split = %+v", usage)
	}
}

func TestStreamObserver_JsonSplitAcrossManyChunks(t *testing.T) {
	frame := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":11,\"output_tokens\":22}}\n\n")
	var chunks [][]byte
	for _, b := range frame {
		chunks = append(chunks, []byte{b})
	}
	upstream := &splitReader{chunks: chunks}
	streamer, _ := readStreamAndCapture(t, upstream, 4096)
	usage := streamer.Usage()
	if usage == nil || usage.Input == nil || *usage.Input != 11 {
		t.Fatalf("usage missing after JSON split: %+v", usage)
	}
	if usage.Output == nil || *usage.Output != 22 {
		t.Fatalf("output wrong after split: %+v", usage)
	}
}

func TestStreamObserver_MultipleEventsOneRead(t *testing.T) {
	a := []byte("event: response.created\ndata: {\"type\":\"response.created\"}\n\n")
	b := []byte("data: hello\n\n")
	c := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":4}}\n\n")
	original := append(append(a, b...), c...)
	upstream := &splitReader{chunks: [][]byte{original}}
	streamer, _ := readStreamAndCapture(t, upstream, 64*1024)
	usage := streamer.Usage()
	if usage == nil || usage.Input == nil || *usage.Input != 3 {
		t.Fatalf("usage from multi-event read = %+v", usage)
	}
}

func TestStreamObserver_PartialFinalFrameAtEOF(t *testing.T) {
	original := []byte("event: response.created\ndata: {\"type\":\"response.created\"}\n\ndata: partial without delimiter")
	got := readStreamThroughObserver(t, bytes.NewReader(original), 4096)
	if !bytes.Equal(got, original) {
		t.Fatalf("partial trailing frame altered\nwant=%q\ngot =%q", original, got)
	}
}

func TestStreamObserver_CloseBeforeEOFIsIdempotent(t *testing.T) {
	terminal := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}\n\n")
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()
	ex := Exchange{ID: "gw-close", Profile: "p", Protocol: ProtocolOpenAIResponses}
	streamer := newStreamObserver(OpenAIResponsesObserver{}, io.NopCloser(bytes.NewReader(terminal)), rec, &ex)
	buf := make([]byte, 4096)
	if _, err := streamer.Read(buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	persisted, err := Replay(rec, "p")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("recorded %d exchanges, want 1", len(persisted))
	}
	if persisted[0].Response.ProviderUsage == nil || persisted[0].Response.ProviderUsage.Input == nil || *persisted[0].Response.ProviderUsage.Input != 1 {
		t.Fatalf("usage missing after close: %+v", persisted[0])
	}
}

func TestStreamObserver_UpstreamEOFPropagated(t *testing.T) {
	ex := Exchange{ID: "gw-eof", Profile: "p", Protocol: ProtocolOpenAIResponses}
	streamer := newStreamObserver(OpenAIResponsesObserver{}, io.NopCloser(&emptyEOFReader{}), nil, &ex)
	buf := make([]byte, 16)
	n, err := streamer.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("expected (0, EOF), got (%d, %v)", n, err)
	}
}

func TestStreamObserver_RecordingExactlyOnce(t *testing.T) {
	terminal := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":9,\"output_tokens\":1}}\n\n")
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()
	ex := Exchange{ID: "gw-once", Profile: "p", Protocol: ProtocolOpenAIResponses}
	streamer := newStreamObserver(OpenAIResponsesObserver{}, io.NopCloser(bytes.NewReader(terminal)), rec, &ex)
	buf := make([]byte, 4096)
	for {
		_, err := streamer.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	persisted, err := Replay(rec, "p")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("recorded %d, want 1", len(persisted))
	}
}

func TestStreamObserver_UpstreamReadErrorPropagated(t *testing.T) {
	expected := errors.New("upstream boom")
	ex := Exchange{ID: "gw-err", Profile: "p", Protocol: ProtocolOpenAIResponses}
	streamer := newStreamObserver(OpenAIResponsesObserver{}, io.NopCloser(&erringReader{err: expected}), nil, &ex)
	buf := make([]byte, 16)
	_, err := streamer.Read(buf)
	if !errors.Is(err, expected) {
		t.Fatalf("expected upstream error, got %v", err)
	}
}

func TestStreamObserver_Proxy_EndToEnd_BytesPreserved(t *testing.T) {
	parts := []string{
		"event: response.created\ndata: {\"type\":\"response.created\"}\n\n",
		"data: hello\n\n",
		"data: world\n\n",
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":13,\"output_tokens\":7}}\n\n",
	}
	original := strings.Join(parts, "")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, original)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	rec, _ := NewFileRecorder(t.TempDir())
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "openai_responses", Source: "codex", Upstream: upstream.URL},
		OpenAIResponsesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/responses")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != original {
		t.Fatalf("end-to-end bytes differ\nwant=%q\ngot =%q", original, got)
	}
}

type splitReader struct {
	chunks [][]byte
	pos    int
	off    int
}

func (r *splitReader) Read(p []byte) (int, error) {
	for r.pos < len(r.chunks) {
		c := r.chunks[r.pos]
		if r.off < len(c) {
			n := copy(p, c[r.off:])
			r.off += n
			return n, nil
		}
		r.pos++
		r.off = 0
	}
	return 0, io.EOF
}

type emptyEOFReader struct{}

func (*emptyEOFReader) Read(p []byte) (int, error) { return 0, io.EOF }

type erringReader struct{ err error }

func (r *erringReader) Read(p []byte) (int, error) { return 0, r.err }

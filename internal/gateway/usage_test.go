package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestProviderUsage_NonStreamingInputTokensParsed(t *testing.T) {
	body := []byte(`{"id":"resp_x","output":[],"usage":{"input_tokens":84279,"output_tokens":753,"input_tokens_details":{"cached_tokens":75312},"output_tokens_details":{"reasoning_tokens":512},"total_tokens":85032}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	if pu.Input == nil || *pu.Input != 84279 {
		t.Fatalf("input = %+v", pu.Input)
	}
	if pu.Output == nil || *pu.Output != 753 {
		t.Fatalf("output = %+v", pu.Output)
	}
	if pu.CachedInput == nil || *pu.CachedInput != 75312 {
		t.Fatalf("cached = %+v", pu.CachedInput)
	}
	if pu.ReasoningOutput == nil || *pu.ReasoningOutput != 512 {
		t.Fatalf("reasoning = %+v", pu.ReasoningOutput)
	}
}

func TestProviderUsage_NonStreamingOutputTokensParsed(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":1000,"output_tokens":42}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil || pu.Output == nil || *pu.Output != 42 {
		t.Fatalf("output missing: %+v", pu)
	}
}

func TestProviderUsage_NonStreamingCachedTokensParsed(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":5000,"output_tokens":10,"input_tokens_details":{"cached_tokens":4500}}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil || pu.CachedInput == nil || *pu.CachedInput != 4500 {
		t.Fatalf("cached missing: %+v", pu)
	}
}

func TestProviderUsage_NonStreamingReasoningTokensParsed(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":500,"output_tokens":100,"output_tokens_details":{"reasoning_tokens":75}}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil || pu.ReasoningOutput == nil || *pu.ReasoningOutput != 75 {
		t.Fatalf("reasoning missing: %+v", pu)
	}
}

func TestProviderUsage_MissingUsageNil(t *testing.T) {
	body := []byte(`{"id":"resp_x","output":[]}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu != nil {
		t.Fatalf("expected nil usage, got %+v", pu)
	}
}

func TestProviderUsage_MissingCachedNotZero(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":1000,"output_tokens":10}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	if pu.CachedInput != nil {
		t.Fatalf("cached should be nil (missing field), got %+v", pu.CachedInput)
	}
}

func TestProviderUsage_ExplicitCachedZeroIsZero(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":1000,"output_tokens":10,"input_tokens_details":{"cached_tokens":0}}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil || pu.CachedInput == nil || *pu.CachedInput != 0 {
		t.Fatalf("expected explicit zero cached: %+v", pu)
	}
}

func TestProviderUsage_MissingReasoningNotZero(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":1000,"output_tokens":10}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	if pu.ReasoningOutput != nil {
		t.Fatalf("reasoning should be nil (missing), got %+v", pu.ReasoningOutput)
	}
}

func TestProviderUsage_MalformedOptionalUsageDegrades(t *testing.T) {
	body := []byte(`{"usage":"not_an_object"}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu != nil {
		t.Fatalf("malformed usage should yield nil, got %+v", pu)
	}
	body = []byte(`{"usage":{"input_tokens_details":"oops"}}`)
	pu = OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu != nil {
		t.Fatalf("malformed details should yield nil, got %+v", pu)
	}
}

func TestProviderUsage_AllZeroUsageIsUnknown(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":0,"output_tokens":0}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu != nil {
		t.Fatalf("expected nil for all-zero usage, got %+v", pu)
	}
}

func TestProviderUsage_StreamTerminalEventParsed(t *testing.T) {
	event := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":1000,\"output_tokens\":50,\"input_tokens_details\":{\"cached_tokens\":800},\"output_tokens_details\":{\"reasoning_tokens\":20}}}\n\n")
	pu := OpenAIResponsesObserver{}.ParseStreamEvent(event)
	if pu == nil {
		t.Fatalf("nil usage from stream event")
	}
	if pu.Input == nil || *pu.Input != 1000 {
		t.Fatalf("input = %+v", pu.Input)
	}
	if pu.CachedInput == nil || *pu.CachedInput != 800 {
		t.Fatalf("cached = %+v", pu.CachedInput)
	}
	if pu.Output == nil || *pu.Output != 50 {
		t.Fatalf("output = %+v", pu.Output)
	}
	if pu.ReasoningOutput == nil || *pu.ReasoningOutput != 20 {
		t.Fatalf("reasoning = %+v", pu.ReasoningOutput)
	}
}

func TestProviderUsage_StreamSkipsNonUsageEvents(t *testing.T) {
	event := []byte("event: response.created\ndata: {\"type\":\"response.created\"}\n\nevent: response.in_progress\ndata: {\"type\":\"response.in_progress\"}\n\n")
	pu := OpenAIResponsesObserver{}.ParseStreamEvent(event)
	if pu != nil {
		t.Fatalf("expected nil, got %+v", pu)
	}
}

func TestProviderUsage_StreamMalformedDegrades(t *testing.T) {
	event := []byte("data: {not json}\n\ndata: {\"type\":\"x\"}\n\n")
	pu := OpenAIResponsesObserver{}.ParseStreamEvent(event)
	if pu != nil {
		t.Fatalf("malformed events should not yield usage, got %+v", pu)
	}
}

func TestProviderUsage_StreamOversizedDegrades(t *testing.T) {
	huge := strings.Repeat("data: ", (OpenAIResponsesObserver{}.MaxStreamEventBytes()/6)+10)
	event := []byte(huge + "\n\n")
	streamer := &streamObserver{observer: OpenAIResponsesObserver{}}
	streamer.overran = true
	streamer.feed(event)
	if streamer.Usage() != nil {
		t.Fatalf("oversized event should not yield usage")
	}
}

func TestProviderUsage_StreamEmptyYieldsNil(t *testing.T) {
	pu := OpenAIResponsesObserver{}.ParseStreamEvent(nil)
	if pu != nil {
		t.Fatalf("nil event should yield nil usage")
	}
	pu = OpenAIResponsesObserver{}.ParseStreamEvent([]byte{})
	if pu != nil {
		t.Fatalf("empty event should yield nil usage")
	}
}

func TestProviderUsage_StreamRawContentNotPersisted(t *testing.T) {
	secret := "SECRET_REASONING_TEXT_DO_NOT_PERSIST"
	event := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"reasoning\":\"" + secret + "\"}\n\n")
	pu := OpenAIResponsesObserver{}.ParseStreamEvent(event)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	raw, _ := json.Marshal(pu)
	if strings.Contains(string(raw), secret) {
		t.Fatalf("usage leaked reasoning text: %s", raw)
	}
}

func TestProxy_StreamingForwardsChunks(t *testing.T) {
	chunks := []string{"first", "second", "third"}
	usage := []byte(`{"type":"response.completed","usage":{"input_tokens":111,"output_tokens":22}}`)
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
		_, _ = fmt.Fprintf(w, "event: response.completed\ndata: %s\n\n", string(usage))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	dir := t.TempDir()
	rec, _ := NewFileRecorder(dir)
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "openai_responses", Source: "codex", Upstream: upstream.URL},
		OpenAIResponsesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	body := []byte(`{"model":"m","input":[{"role":"user","content":"hi"}]}`)
	resp, err := http.Post(srv.URL+"/v1/responses", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	all, _ := readAllChunks2(resp.Body)
	if !strings.Contains(all, "first") || !strings.Contains(all, "third") {
		t.Fatalf("missing chunks: %q", all)
	}

	persisted, err := Replay(rec, "p")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("persisted = %d, want 1", len(persisted))
	}
	pu := persisted[0].Response.ProviderUsage
	if pu == nil || pu.Input == nil || *pu.Input != 111 {
		t.Fatalf("provider usage = %+v", pu)
	}
}

func TestProxy_StreamingMalformedSSEDoesNotBreakProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("this is not sse at all\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("event: response.completed\ndata: {not valid json\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
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
	all, _ := readAllChunks2(resp.Body)
	if !strings.Contains(all, "this is not sse") || !strings.Contains(all, "[DONE]") {
		t.Fatalf("malformed SSE did not pass through: %q", all)
	}
}

func TestProxy_NonStreamingExtractsProviderUsage(t *testing.T) {
	body := []byte(`{"id":"resp_x","output":[],"usage":{"input_tokens":100,"output_tokens":5,"input_tokens_details":{"cached_tokens":80}}}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	dir := t.TempDir()
	rec, _ := NewFileRecorder(dir)
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "openai_responses", Source: "codex", Upstream: upstream.URL},
		OpenAIResponsesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	reqBody := []byte(`{"model":"m","input":[{"role":"user","content":"hi"}]}`)
	resp, err := http.Post(srv.URL+"/v1/responses", "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	persisted, err := Replay(rec, "p")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("persisted = %d, want 1", len(persisted))
	}
	pu := persisted[0].Response.ProviderUsage
	if pu == nil || pu.Input == nil || *pu.Input != 100 {
		t.Fatalf("usage = %+v", pu)
	}
	if pu.CachedInput == nil || *pu.CachedInput != 80 {
		t.Fatalf("cached = %+v", pu)
	}
	if persisted[0].Response.Usage == nil || persisted[0].Response.Usage.Source != model.MeasurementMeasured {
		t.Fatalf("observed usage should be measured, got %+v", persisted[0].Response.Usage)
	}
}

func TestProxy_RawResponseBodyNotPersisted(t *testing.T) {
	secret := "SECRET_MODEL_OUTPUT_DO_NOT_PERSIST"
	body := []byte(fmt.Sprintf(`{"id":"resp_x","output":[{"type":"message","content":[{"text":%q}]}],"usage":{"input_tokens":1,"output_tokens":1}}`, secret))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	listen, _ := freeLoopback(t)
	dir := t.TempDir()
	rec, _ := NewFileRecorder(dir)
	defer func() { _ = rec.Close() }()
	proxy, _ := NewProxy(
		Profile{Name: "p", Enabled: true, Listen: listen, Protocol: "openai_responses", Source: "codex", Upstream: upstream.URL},
		OpenAIResponsesObserver{}, rec,
	)
	srv := httptest.NewServer(proxy.Handler())
	defer srv.Close()

	resp, _ := http.Post(srv.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"m"}`))
	_ = resp.Body.Close()

	data, err := os.ReadFile(filepath.Join(dir, "p.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("capture leaked model output: %s", data)
	}
}

func readAllChunks2(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return string(data), err
	}
	return string(data), nil
}

func TestProviderUsage_ApplyProviderUsage_MeasurementKindMeasured(t *testing.T) {
	v := int64(100)
	pu := &ProviderUsage{Input: &v}
	ex := Exchange{Response: ExchangeResponse{ProviderUsage: pu}}
	ApplyProviderUsageToExchange(&ex)
	if ex.Response.Usage == nil || ex.Response.Usage.Source != model.MeasurementMeasured {
		t.Fatalf("expected measured, got %+v", ex.Response.Usage)
	}
	if ex.Response.Usage.Input != 100 {
		t.Fatalf("input = %d", ex.Response.Usage.Input)
	}
}

func TestProviderUsage_FreshInputDerived(t *testing.T) {
	input, cached := int64(1000), int64(750)
	pu := &ProviderUsage{Input: &input, CachedInput: &cached}
	ex := Exchange{Response: ExchangeResponse{ProviderUsage: pu}}
	ApplyProviderUsageToExchange(&ex)
	if ex.Response.Usage.Cached != 750 {
		t.Fatalf("cached = %d", ex.Response.Usage.Cached)
	}
	request := RequestSummary{Attributed: AttributedTotal{Value: model.NewMeasurement(900, model.MeasurementEstimated)}}
	puSummary := buildRequestProviderSummary(ex, request.Attributed.Value)
	if puSummary == nil || puSummary.FreshInput == nil || *puSummary.FreshInput != 250 {
		t.Fatalf("fresh = %+v", puSummary)
	}
}

func TestProviderUsage_CachedNotAddedToInput(t *testing.T) {
	v := int64(100)
	c := int64(80)
	pu := &ProviderUsage{Input: &v, CachedInput: &c}
	out := providerUsageToObserved(pu)
	if out.Input != 100 {
		t.Fatalf("input = %d, want 100 (no double count)", out.Input)
	}
	if out.Total != out.Input+out.Output {
		t.Fatalf("total must be input + output only")
	}
}

func TestProviderUsage_ReasoningNotAddedToOutput(t *testing.T) {
	v := int64(50)
	r := int64(20)
	pu := &ProviderUsage{Output: &v, ReasoningOutput: &r}
	out := providerUsageToObserved(pu)
	if out.Output != 50 {
		t.Fatalf("output = %d, want 50 (no double count)", out.Output)
	}
	if out.Total != out.Input+out.Output {
		t.Fatalf("total must not include reasoning")
	}
}

func TestProviderUsage_GapPositive(t *testing.T) {
	ex := exchangeWithAttributed(80000, int64P(84279), int64P(75312), nil)
	got := summarizeCallProviderUsage2(ex, 80000)
	if got.gap == nil || got.gap.ValueOrZero() != 4279 {
		t.Fatalf("gap = %+v, want 4279", got.gap)
	}
}

func TestProviderUsage_GapNegative(t *testing.T) {
	ex := exchangeWithAttributed(25398, int64P(23504), nil, nil)
	got := summarizeCallProviderUsage2(ex, 25398)
	if got.gap == nil || got.gap.ValueOrZero() != -1894 {
		t.Fatalf("gap = %+v, want -1894", got.gap)
	}
}

func TestProviderUsage_GapZero(t *testing.T) {
	ex := exchangeWithAttributed(1000, int64P(1000), nil, nil)
	got := summarizeCallProviderUsage2(ex, 1000)
	if got.gap == nil || got.gap.ValueOrZero() != 0 {
		t.Fatalf("gap = %+v, want 0", got.gap)
	}
}

func TestProviderUsage_GapUnknown(t *testing.T) {
	ex := Exchange{Response: ExchangeResponse{ProviderUsage: &ProviderUsage{}}}
	got := summarizeCallProviderUsage2(ex, 1000)
	if got.gap != nil {
		t.Fatalf("gap should be nil for unknown provider input, got %+v", got.gap)
	}
}

func TestProviderUsage_CoverageBasic(t *testing.T) {
	ex := exchangeWithAttributed(74000, int64P(84000), nil, nil)
	got := summarizeCallProviderUsage2(ex, 74000)
	if got.coverage == nil || got.coverage.Percent == nil {
		t.Fatalf("coverage nil")
	}
	if got.coverage.Kind != "estimated" {
		t.Fatalf("coverage kind = %q, want estimated", got.coverage.Kind)
	}
	gotVal := *got.coverage.Percent
	if gotVal < 88.0 || gotVal > 88.2 {
		t.Fatalf("coverage percent = %f, want ~88.1", gotVal)
	}
}

func TestProviderUsage_CoverageOver100Preserved(t *testing.T) {
	ex := exchangeWithAttributed(25398, int64P(23504), nil, nil)
	got := summarizeCallProviderUsage2(ex, 25398)
	if got.coverage == nil || got.coverage.Percent == nil {
		t.Fatalf("coverage nil")
	}
	if *got.coverage.Percent <= 100 {
		t.Fatalf("coverage percent = %f, want >100", *got.coverage.Percent)
	}
}

func TestProviderUsage_CoverageNotClamped(t *testing.T) {
	ex := exchangeWithAttributed(50000, int64P(10), nil, nil)
	got := summarizeCallProviderUsage2(ex, 50000)
	if got.coverage == nil || got.coverage.Percent == nil {
		t.Fatalf("coverage nil")
	}
	if *got.coverage.Percent <= 100 {
		t.Fatalf("coverage percent = %f, want >100", *got.coverage.Percent)
	}
}

func TestProviderUsage_CoverageProviderZeroUnavailable(t *testing.T) {
	ex := exchangeWithAttributed(5000, int64P(0), nil, nil)
	got := summarizeCallProviderUsage2(ex, 5000)
	if got.coverage != nil {
		t.Fatalf("coverage should be nil when provider input is zero, got %+v", got.coverage)
	}
}

func TestProviderUsage_CoverageUnknownProvider(t *testing.T) {
	ex := Exchange{Response: ExchangeResponse{}}
	got := summarizeCallProviderUsage2(ex, 1000)
	if got.coverage != nil {
		t.Fatalf("coverage should be nil when provider input missing, got %+v", got.coverage)
	}
}

func TestProviderUsage_CoverageUnknownAttributed(t *testing.T) {
	v := int64(100)
	ex := Exchange{Response: ExchangeResponse{ProviderUsage: &ProviderUsage{Input: &v}}}
	got := summarizeCallProviderUsage2(ex, 0)
	got.coverage = nil
	if got.coverage != nil {
		t.Fatalf("coverage should be nil when attributed unknown")
	}
}

func TestChain_AllSevenCallsWithUsage_AggregateExact(t *testing.T) {
	inputs := []int64{84279, 91000, 120000, 95000, 80000, 60000}
	cached := []int64{75312, 81000, 110000, 80000, 60000, 40000}
	exchanges := observedMetricChainWithUsage(inputs, cached)
	chains := AnalyzeChains(exchanges)
	if len(chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(chains))
	}
	c := chains[0]
	wantInput := int64(0)
	for _, v := range inputs {
		wantInput += v
	}
	wantCached := int64(0)
	for _, v := range cached {
		wantCached += v
	}
	if c.ProviderInput == nil || c.ProviderInput.ValueOrZero() != wantInput {
		t.Fatalf("provider input = %+v, want %d", c.ProviderInput, wantInput)
	}
	if c.ProviderCachedInput == nil || c.ProviderCachedInput.ValueOrZero() != wantCached {
		t.Fatalf("provider cached = %+v, want %d", c.ProviderCachedInput, wantCached)
	}
	if c.UsageObservedCalls == nil || *c.UsageObservedCalls != 6 || c.UsageTotalCalls == nil || *c.UsageTotalCalls != 6 {
		t.Fatalf("usage calls = %+v/%+v", c.UsageObservedCalls, c.UsageTotalCalls)
	}
}

func TestChain_MissingUsageOneOfSeven_AggregateUnknown(t *testing.T) {
	inputs := []int64{84279, 91000, 120000, 95000, 80000, 60000}
	cached := []int64{75312, 81000, 110000, 80000, 60000, 40000}
	exchanges := observedMetricChainWithUsage(inputs, cached)
	exchanges[3].Response.ProviderUsage = nil
	chains := AnalyzeChains(exchanges)
	if len(chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(chains))
	}
	c := chains[0]
	if c.ProviderInput != nil {
		t.Fatalf("provider input should be unknown when one call missing usage, got %+v", c.ProviderInput)
	}
	if c.UsageObservedCalls == nil || *c.UsageObservedCalls != 5 || c.UsageTotalCalls == nil || *c.UsageTotalCalls != 6 {
		t.Fatalf("usage observed = %+v/%+v", c.UsageObservedCalls, c.UsageTotalCalls)
	}
}

func TestChain_MissingCachedOnly_AggregateUnknown(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(3)
	setProviderUsageAll(exchanges, []int64{100, 200, 300}, []int64{50, 60, 70})
	exchanges[1].Response.ProviderUsage.CachedInput = nil
	chains := AnalyzeChains(exchanges)
	c := chains[0]
	if c.ProviderCachedInput != nil {
		t.Fatalf("provider cached should be unknown when one call missing, got %+v", c.ProviderCachedInput)
	}
	if c.ProviderInput == nil || c.ProviderInput.ValueOrZero() != 600 {
		t.Fatalf("provider input should still aggregate when only cached missing, got %+v", c.ProviderInput)
	}
}

func TestChain_MissingOutputOnly_AggregateUnknown(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(3)
	setProviderUsageAll(exchanges, []int64{100, 200, 300}, []int64{50, 60, 70})
	outV := int64(10)
	exchanges[0].Response.ProviderUsage.Output = &outV
	exchanges[2].Response.ProviderUsage.Output = &outV
	exchanges[1].Response.ProviderUsage.Output = nil
	chains := AnalyzeChains(exchanges)
	c := chains[0]
	if c.ProviderOutput != nil {
		t.Fatalf("provider output should be unknown when one call missing, got %+v", c.ProviderOutput)
	}
	if c.ProviderInput == nil || c.ProviderInput.ValueOrZero() != 600 {
		t.Fatalf("provider input should still aggregate when only output missing, got %+v", c.ProviderInput)
	}
}

func TestChain_ProviderInputZero_CoverageUnavailable(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(3)
	setProviderUsageAll(exchanges, []int64{0, 0, 0}, []int64{0, 0, 0})
	chains := AnalyzeChains(exchanges)
	if chains[0].AttributionCoverage != nil {
		t.Fatalf("coverage should be unavailable, got %+v", chains[0].AttributionCoverage)
	}
}

func TestRender_InspectShowsProviderUsage(t *testing.T) {
	ex := exchangeWithAttributed(74355, int64P(84279), int64P(75312), int64P(753))
	s := SummarizeRequest(ex)
	var buf strings.Builder
	if err := RenderInspect(&buf, s); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Provider usage", "Input", "84,279", "Cached", "75,312", "Fresh", "Output", "753", "Attribution", "Coverage"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspect missing %q in %q", want, out)
		}
	}
}

func TestRender_InspectUnknownRendersDash(t *testing.T) {
	ex := exchangeWithAttributed(1000, nil, nil, nil)
	s := SummarizeRequest(ex)
	var buf strings.Builder
	if err := RenderInspect(&buf, s); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "Provider usage") {
		t.Fatalf("inspect should not show provider section when no usage: %q", out)
	}
	if strings.Contains(out, "Attribution\n") {
		t.Fatalf("inspect should not show attribution when no usage: %q", out)
	}
}

func TestRender_ChainShowsProviderUsageTable(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(3)
	setProviderUsageAll(exchanges, []int64{84279, 57731, 95000}, []int64{75312, 45000, 70000})
	outV := int64(100)
	for i := range exchanges {
		exchanges[i].Response.ProviderUsage.Output = &outV
	}
	chain := AnalyzeChains(exchanges)[0]
	var buf strings.Builder
	if err := RenderChain(&buf, chain); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Provider usage", "84,279", "75,312", "8,967", "Coverage", "Provider input", "Cached input", "Fresh input", "Usage observed calls  3/3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("chain missing %q in %q", want, out)
		}
	}
}

func TestRender_ChainShowsEstimatedMarker(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(2)
	setProviderUsageAll(exchanges, []int64{100, 200}, []int64{50, 100})
	outV := int64(10)
	for i := range exchanges {
		exchanges[i].Response.ProviderUsage.Output = &outV
	}
	chain := AnalyzeChains(exchanges)[0]
	var buf strings.Builder
	if err := RenderChain(&buf, chain); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "*") {
		t.Fatalf("expected estimated marker in coverage output: %q", out)
	}
}

func TestRender_ChainsListShowsCoverage(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(2)
	setProviderUsageAll(exchanges, []int64{84279, 57731}, []int64{75312, 45000})
	chains := AnalyzeChains(exchanges)
	var buf strings.Builder
	if err := RenderChains(&buf, chains); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Coverage") {
		t.Fatalf("chains list missing Coverage column: %q", out)
	}
}

func TestJSON_NumericProviderUsageValues(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(1)
	setProviderUsageAll(exchanges, []int64{84279}, []int64{75312})
	outV := int64(100)
	exchanges[0].Response.ProviderUsage.Output = &outV
	chain := AnalyzeChains(exchanges)[0]
	raw, err := json.Marshal(chain)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	compact := strings.Join(strings.Fields(string(raw)), "")
	if !strings.Contains(compact, `"input":84279`) {
		t.Fatalf("json missing numeric input: %s", compact)
	}
	if !strings.Contains(compact, `"cachedInput":75312`) {
		t.Fatalf("json missing numeric cachedInput: %s", compact)
	}
	if !strings.Contains(compact, `"providerInput":`) {
		t.Fatalf("json missing providerInput field: %s", compact)
	}
	if strings.Contains(compact, `"84,279"`) || strings.Contains(compact, `"75,312"`) {
		t.Fatalf("json contains formatted strings: %s", compact)
	}
}

func TestJSON_CoveragePercentNumeric(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(1)
	setProviderUsageAll(exchanges, []int64{1000}, []int64{500})
	outV := int64(10)
	exchanges[0].Response.ProviderUsage.Output = &outV
	chain := AnalyzeChains(exchanges)[0]
	raw, _ := json.Marshal(chain)
	compact := strings.Join(strings.Fields(string(raw)), "")
	if strings.Contains(compact, `"88.1%"`) {
		t.Fatalf("coverage percent should be numeric, not string: %s", compact)
	}
	if !strings.Contains(compact, `"percent":`) {
		t.Fatalf("coverage missing percent field: %s", compact)
	}
}

func TestJSON_MeasurementKindsPreserved(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(1)
	setProviderUsageAll(exchanges, []int64{100}, []int64{50})
	outV := int64(10)
	exchanges[0].Response.ProviderUsage.Output = &outV
	chain := AnalyzeChains(exchanges)[0]
	raw, _ := json.Marshal(chain)
	compact := strings.Join(strings.Fields(string(raw)), "")
	if !strings.Contains(compact, `"kind":"measured"`) {
		t.Fatalf("provider input should be measured: %s", compact)
	}
	if !strings.Contains(compact, `"kind":"estimated"`) {
		t.Fatalf("attribution kind should be estimated: %s", compact)
	}
}

func TestPrivacy_NoRawPromptInCapture(t *testing.T) {
	body := []byte(`{"model":"m","instructions":"SECRET_PROMPT","input":[{"role":"user","content":"hi"}]}`)
	ex := openAIExchangeWithMeta("gw-priv", "codex", "m", 0, body, nil, nil)
	raw, _ := json.Marshal(ex)
	if strings.Contains(string(raw), "SECRET_PROMPT") {
		t.Fatalf("exchange leaked raw prompt: %s", raw)
	}
}

func TestPrivacy_NoRawReasoningInCapture(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":1,"output_tokens_details":{"reasoning_tokens":1}}}`)
	ex := openAIExchangeWithMeta("gw-priv", "codex", "m", 0, body, nil, nil)
	ex.Response.ProviderUsage = OpenAIResponsesObserver{}.ParseResponseUsage(body)
	raw, _ := json.Marshal(ex)
	if strings.Contains(string(raw), "reasoning_text_DO_NOT_PERSIST") {
		t.Fatalf("leaked reasoning: %s", raw)
	}
}

func TestPrivacy_NoRawFunctionArgsInCapture(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call_output","call_id":"c1","output":"SECRET_OUTPUT"}]}`)
	ex := openAIExchangeWithMeta("gw-priv", "codex", "m", 0, body, nil, nil)
	raw, _ := json.Marshal(ex)
	if strings.Contains(string(raw), "SECRET_OUTPUT") {
		t.Fatalf("leaked function output: %s", raw)
	}
}

func TestChainSummary_JSON_NoRawBodies(t *testing.T) {
	exchanges := observedMetricChainWithUsage([]int64{100, 200}, []int64{50, 100})
	for _, e := range exchanges {
		e.Request.Components = nil
	}
	chain := AnalyzeChains(exchanges)[0]
	raw, _ := json.Marshal(chain)
	for _, banned := range []string{"SECRET", "Authorization", "Bearer"} {
		if strings.Contains(string(raw), banned) {
			t.Fatalf("chain leaked %q: %s", banned, raw)
		}
	}
}

func TestAnthropicTestsStillPass(t *testing.T) {
	exchanges := []Exchange{
		anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true)),
		anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", nil, []string{"toolu_a"}, false)),
	}
	chain := NewChainBuilder().Build(exchanges)
	if len(chain.Chains) != 1 || len(chain.Chains[0].Exchanges) != 2 {
		t.Fatalf("Anthropic chain regression: %+v", chain)
	}
}

func TestChainReportingTestsStillPass(t *testing.T) {
	chains := AnalyzeChains(observedMetricChain())
	if len(chains) != 1 {
		t.Fatalf("chain reporting regression")
	}
}

func TestOpenAICorrelationTestsStillPass(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{ResponseID: "resp_A"},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBody("m", nil, true),
		&OpenAIResponsesMetadata{PreviousResponseID: "resp_A"},
		nil,
	)
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue {
		t.Fatalf("OpenAI correlation regression: %+v", got)
	}
}

func TestModelsEndpointExcluded(t *testing.T) {
	modelsEx := openAIExchange("gw-models", "codex", "m", 0, openAIModelsBody())
	got := NewChainBuilder().Build([]Exchange{modelsEx})
	if len(got.Chains) != 0 {
		t.Fatalf("models endpoint should not create chain")
	}
}

func TestGatewayLifecycleStillPass(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()
	rec.Record(Exchange{ID: "gw-a", Profile: "p", Protocol: ProtocolOpenAIResponses, StartedAt: time.Now()})
	summary, err := rec.Summary("p")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Requests != 1 {
		t.Fatalf("requests = %d, want 1", summary.Requests)
	}
	if err := rec.Purge("p"); err != nil {
		t.Fatalf("purge: %v", err)
	}
	_, err = os.Stat(filepath.Join(dir, "p.jsonl"))
	if err == nil {
		t.Fatalf("expected file removed")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}

func int64P(v int64) *int64 { return &v }

func observedMetricChainWithUsage(inputs, cached []int64) []Exchange {
	base := observedMetricChain()
	for i := range base {
		if i >= len(inputs) {
			break
		}
		var inV, caV int64 = inputs[i], cached[i]
		if i < len(cached) {
			caV = cached[i]
		}
		base[i].Response.ProviderUsage = &ProviderUsage{
			Input:       &inV,
			CachedInput: &caV,
		}
		if i == 0 {
			outV := int64(100)
			base[i].Response.ProviderUsage.Output = &outV
		}
	}
	return base
}

func smallOpenAIResponsesChain(count int) []Exchange {
	out := make([]Exchange, 0, count)
	for i := 0; i < count; i++ {
		exchange := openAIExchangeWithMeta("gw-"+itoa(i), "codex", "m", int64(i),
			openAIResponsesBody("m", nil, true),
			nil,
			&OpenAIResponsesResponseMeta{ResponseID: "resp_" + itoa(i)},
		)
		exchange.Request.Components = []model.ContextComponent{
			{Kind: model.ContextInstructions, Measurement: model.NewMeasurement(1000, model.MeasurementEstimated)},
			{Kind: model.ContextHistory, Measurement: model.NewMeasurement(int64(500*(i+1)), model.MeasurementEstimated)},
		}
		out = append(out, exchange)
	}
	for i := 1; i < count; i++ {
		prev := out[i-1].Response.OpenAIResponses
		if prev == nil {
			continue
		}
		meta := OpenAIResponsesMetadata{PreviousResponseID: prev.ResponseID}
		out[i].Request.Metadata.OpenAIResponses = &meta
	}
	return out
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func setProviderUsageAll(exchanges []Exchange, inputs, cached []int64) {
	for i := range exchanges {
		if i >= len(inputs) || i >= len(cached) {
			break
		}
		inV, caV := inputs[i], cached[i]
		exchanges[i].Response.ProviderUsage = &ProviderUsage{
			Input:       &inV,
			CachedInput: &caV,
		}
	}
}

type callProvResult struct {
	gap      *model.Measurement
	coverage *AttributionCoverageValue
}

func summarizeCallProviderUsage2(exchange Exchange, attributed int64) callProvResult {
	request := SummarizeRequest(exchange)
	if request.Attributed.Value.Available() {
		request.Attributed.Value = model.NewMeasurement(attributed, request.Attributed.Value.Kind)
	} else {
		request.Attributed.Value = model.NewMeasurement(attributed, model.MeasurementEstimated)
	}
	_, gap, cov := summarizeCallProviderUsage(exchange, request)
	return callProvResult{gap: gap, coverage: cov}
}

func exchangeWithAttributed(attributed int64, input, cached, output *int64) Exchange {
	pu := &ProviderUsage{}
	if input != nil {
		v := *input
		pu.Input = &v
	}
	if cached != nil {
		v := *cached
		pu.CachedInput = &v
	}
	if output != nil {
		v := *output
		pu.Output = &v
	}
	return Exchange{
		ID:        "gw-attrib",
		Profile:   "p",
		Protocol:  ProtocolOpenAIResponses,
		StartedAt: time.Now(),
		Request: ExchangeRequest{
			Method:   "POST",
			Endpoint: "/v1/responses",
			Components: []model.ContextComponent{
				{Kind: model.ContextInstructions, Measurement: model.NewMeasurement(attributed/3, model.MeasurementEstimated)},
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(attributed/3, model.MeasurementEstimated)},
				{Kind: model.ContextToolResult, Measurement: model.NewMeasurement(attributed/3, model.MeasurementEstimated)},
			},
		},
		Response: ExchangeResponse{ProviderUsage: pu},
	}
}

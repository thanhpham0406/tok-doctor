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
	if pu.InputTokens == nil || *pu.InputTokens != 84279 {
		t.Fatalf("input = %+v", pu.InputTokens)
	}
	if pu.OutputTokens == nil || *pu.OutputTokens != 753 {
		t.Fatalf("output = %+v", pu.OutputTokens)
	}
	if pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 75312 {
		t.Fatalf("cached = %+v", pu.CacheReadInputTokens)
	}
	if pu.ReasoningOutputTokens == nil || *pu.ReasoningOutputTokens != 512 {
		t.Fatalf("reasoning = %+v", pu.ReasoningOutputTokens)
	}
}

func TestProviderUsage_NonStreamingOutputTokensParsed(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":1000,"output_tokens":42}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil || pu.OutputTokens == nil || *pu.OutputTokens != 42 {
		t.Fatalf("output missing: %+v", pu)
	}
}

func TestProviderUsage_NonStreamingCachedTokensParsed(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":5000,"output_tokens":10,"input_tokens_details":{"cached_tokens":4500}}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil || pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 4500 {
		t.Fatalf("cached missing: %+v", pu)
	}
}

func TestProviderUsage_NonStreamingReasoningTokensParsed(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":500,"output_tokens":100,"output_tokens_details":{"reasoning_tokens":75}}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil || pu.ReasoningOutputTokens == nil || *pu.ReasoningOutputTokens != 75 {
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
	if pu.CacheReadInputTokens != nil {
		t.Fatalf("cached should be nil (missing field), got %+v", pu.CacheReadInputTokens)
	}
}

func TestProviderUsage_ExplicitCachedZeroIsZero(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":1000,"output_tokens":10,"input_tokens_details":{"cached_tokens":0}}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil || pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 0 {
		t.Fatalf("expected explicit zero cached: %+v", pu)
	}
}

func TestProviderUsage_MissingReasoningNotZero(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":1000,"output_tokens":10}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	if pu.ReasoningOutputTokens != nil {
		t.Fatalf("reasoning should be nil (missing), got %+v", pu.ReasoningOutputTokens)
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

func TestProviderUsage_AllZeroUsageIsObservedZero(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":0,"output_tokens":0}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("zero usage should still be observed; nil means upstream sent a structured empty usage block, not zero values")
	}
	if pu.InputTokens == nil || *pu.InputTokens != 0 {
		t.Fatalf("input_tokens should be explicit zero pointer, got %+v", pu.InputTokens)
	}
	if pu.OutputTokens == nil || *pu.OutputTokens != 0 {
		t.Fatalf("output_tokens should be explicit zero pointer, got %+v", pu.OutputTokens)
	}
}

func TestProviderUsage_StreamTerminalEventParsed(t *testing.T) {
	state := newOpenAIStreamState()
	obs := OpenAIResponsesObserver{}
	got := obs.ParseStreamFrame(state, []byte(`{"type":"response.completed","response":{"id":"resp_term"},"usage":{"input_tokens":1000,"output_tokens":50,"input_tokens_details":{"cached_tokens":800},"output_tokens_details":{"reasoning_tokens":20}}}`))
	pu := got.Usage
	if pu == nil || !got.Terminal {
		t.Fatalf("usage=%+v terminal=%v", pu, got.Terminal)
	}
	if pu.InputTokens == nil || *pu.InputTokens != 1000 {
		t.Fatalf("input = %+v", pu.InputTokens)
	}
	if pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 800 {
		t.Fatalf("cached = %+v", pu.CacheReadInputTokens)
	}
	if pu.OutputTokens == nil || *pu.OutputTokens != 50 {
		t.Fatalf("output = %+v", pu.OutputTokens)
	}
	if pu.ReasoningOutputTokens == nil || *pu.ReasoningOutputTokens != 20 {
		t.Fatalf("reasoning = %+v", pu.ReasoningOutputTokens)
	}
	if got.ResponseObjectID != "resp_term" {
		t.Fatalf("response id = %q, want resp_term", got.ResponseObjectID)
	}
}

func TestProviderUsage_StreamSkipsNonUsageEvents(t *testing.T) {
	state := newOpenAIStreamState()
	obs := OpenAIResponsesObserver{}
	if got := obs.ParseStreamFrame(state, []byte(`{"type":"response.created","response":{"id":"resp_created"}}`)); got.Usage != nil {
		t.Fatalf("expected nil usage from response.created, got %+v", got.Usage)
	}
	if got := obs.ParseStreamFrame(state, []byte(`{"type":"response.in_progress"}`)); got.Usage != nil {
		t.Fatalf("expected nil usage from response.in_progress, got %+v", got.Usage)
	}
}

func TestProviderUsage_StreamMalformedDegrades(t *testing.T) {
	state := newOpenAIStreamState()
	obs := OpenAIResponsesObserver{}
	if got := obs.ParseStreamFrame(state, []byte(`{not json`)); got.Usage != nil {
		t.Fatalf("malformed events should not yield usage, got %+v", got.Usage)
	}
}

func TestProviderUsage_StreamEmptyYieldsNil(t *testing.T) {
	obs := OpenAIResponsesObserver{}
	state := newOpenAIStreamState()
	if got := obs.ParseStreamFrame(state, nil); got.Usage != nil {
		t.Fatalf("nil event should yield nil usage")
	}
	if got := obs.ParseStreamFrame(state, []byte{}); got.Usage != nil {
		t.Fatalf("empty event should yield nil usage")
	}
}

func TestProviderUsage_StreamRawContentNotPersisted(t *testing.T) {
	secret := "SECRET_REASONING_TEXT_DO_NOT_PERSIST"
	state := newOpenAIStreamState()
	obs := OpenAIResponsesObserver{}
	got := obs.ParseStreamFrame(state, []byte(fmt.Sprintf(`{"type":"response.completed","usage":{"input_tokens":1,"output_tokens":1},"reasoning":%q}`, secret)))
	pu := got.Usage
	if pu == nil {
		t.Fatalf("usage nil")
	}
	raw, _ := json.Marshal(pu)
	if strings.Contains(string(raw), secret) {
		t.Fatalf("usage leaked reasoning text: %s", raw)
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
	if pu == nil || pu.InputTokens == nil || *pu.InputTokens != 100 {
		t.Fatalf("input_tokens = %+v", pu)
	}
	if pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 80 {
		t.Fatalf("cached = %+v", pu.CacheReadInputTokens)
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
	pu := &ProviderUsage{Source: string(ProtocolOpenAIResponses), InputTokens: &v}
	ex := Exchange{Response: ExchangeResponse{ProviderUsage: pu}}
	ProviderUsageToObservedApply(&ex)
	if ex.Response.Usage == nil || ex.Response.Usage.Source != model.MeasurementMeasured {
		t.Fatalf("expected measured, got %+v", ex.Response.Usage)
	}
	if ex.Response.Usage.TotalInput != 100 {
		t.Fatalf("TotalInput = %d, want 100", ex.Response.Usage.TotalInput)
	}
}

func TestProviderUsage_FreshInputDerived(t *testing.T) {
	input, cached := int64(1000), int64(750)
	pu := &ProviderUsage{Source: "anthropic_messages", InputTokens: &input, CacheReadInputTokens: &cached}
	observed := ProviderUsageToObserved(pu)
	if observed.Cached != 750 {
		t.Fatalf("cached = %d, want 750", observed.Cached)
	}
	if observed.RawInput != 1000 {
		t.Fatalf("RawInput = %d, want 1000", observed.RawInput)
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
	if c.ProviderTotalInput == nil || c.ProviderTotalInput.ValueOrZero() != wantInput {
		t.Fatalf("provider total input = %+v, want %d", c.ProviderTotalInput, wantInput)
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
	if c.ProviderTotalInput != nil {
		t.Fatalf("provider total input should be unknown when one call missing usage, got %+v", c.ProviderTotalInput)
	}
	if c.UsageObservedCalls == nil || *c.UsageObservedCalls != 5 || c.UsageTotalCalls == nil || *c.UsageTotalCalls != 6 {
		t.Fatalf("usage observed = %+v/%+v", c.UsageObservedCalls, c.UsageTotalCalls)
	}
}

func TestChain_MissingCachedOnly_AggregateUnknown(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(3)
	setProviderUsageAll(exchanges, []int64{100, 200, 300}, []int64{50, 60, 70})
	exchanges[1].Response.ProviderUsage.CacheReadInputTokens = nil
	chains := AnalyzeChains(exchanges)
	c := chains[0]
	if c.ProviderCachedInput != nil {
		t.Fatalf("provider cached should be unknown when one call missing, got %+v", c.ProviderCachedInput)
	}
	if c.ProviderTotalInput == nil || c.ProviderTotalInput.ValueOrZero() != 600 {
		t.Fatalf("provider total input should still aggregate when only cached missing, got %+v", c.ProviderTotalInput)
	}
}

func TestChain_MissingOutputOnly_AggregateUnknown(t *testing.T) {
	exchanges := smallOpenAIResponsesChain(3)
	setProviderUsageAll(exchanges, []int64{100, 200, 300}, []int64{50, 60, 70})
	outV := int64(10)
	exchanges[0].Response.ProviderUsage.OutputTokens = &outV
	exchanges[2].Response.ProviderUsage.OutputTokens = &outV
	exchanges[1].Response.ProviderUsage.OutputTokens = nil
	chains := AnalyzeChains(exchanges)
	c := chains[0]
	if c.ProviderOutput != nil {
		t.Fatalf("provider output should be unknown when one call missing, got %+v", c.ProviderOutput)
	}
	if c.ProviderTotalInput == nil || c.ProviderTotalInput.ValueOrZero() != 600 {
		t.Fatalf("provider total input should still aggregate when only output missing, got %+v", c.ProviderTotalInput)
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
	for i := range exchanges {
		exchanges[i].Request.Components = nil
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
			Source:               string(ProtocolOpenAIResponses),
			InputTokens:          &inV,
			CacheReadInputTokens: &caV,
		}
		if i == 0 {
			outV := int64(100)
			base[i].Response.ProviderUsage.OutputTokens = &outV
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
			Source:               string(ProtocolOpenAIResponses),
			InputTokens:          &inV,
			CacheReadInputTokens: &caV,
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
		pu.InputTokens = &v
	}
	if cached != nil {
		v := *cached
		pu.CacheReadInputTokens = &v
	}
	if output != nil {
		v := *output
		pu.OutputTokens = &v
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

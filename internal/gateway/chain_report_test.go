package gateway

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestAnalyzeChains_SixCallMetrics(t *testing.T) {
	exchanges := observedMetricChain()
	chains := AnalyzeChains(exchanges)
	if len(chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(chains))
	}
	chain := chains[0]
	if chain.ModelCalls != 6 {
		t.Fatalf("model calls = %d, want 6", chain.ModelCalls)
	}
	if chain.TotalContextSent.ValueOrZero() != 338452 {
		t.Fatalf("total context = %d, want 338452", chain.TotalContextSent.ValueOrZero())
	}
	if chain.InitialContext.ValueOrZero() != 38181 {
		t.Fatalf("initial context = %d, want 38181", chain.InitialContext.ValueOrZero())
	}
	if chain.PeakContext.ValueOrZero() != 75701 {
		t.Fatalf("peak context = %d, want 75701", chain.PeakContext.ValueOrZero())
	}
	if chain.FinalContext.ValueOrZero() != 75701 {
		t.Fatalf("final context = %d, want 75701", chain.FinalContext.ValueOrZero())
	}
	if chain.ContextGrowth.ValueOrZero() != 37520 {
		t.Fatalf("context growth = %d, want 37520", chain.ContextGrowth.ValueOrZero())
	}
	if chain.Calls[1].ContextDelta.ValueOrZero() != 7885 {
		t.Fatalf("call 2 delta = %d, want 7885", chain.Calls[1].ContextDelta.ValueOrZero())
	}
	if chain.LargestGrowth.ValueOrZero() != 18898 || chain.LargestGrowthCall != 5 {
		t.Fatalf("largest growth = %+v at %d, want 18898 at 5", chain.LargestGrowth, chain.LargestGrowthCall)
	}
	if chain.ToolResultsGrowth.ValueOrZero() != 36606 {
		t.Fatalf("tool-result growth = %d, want 36606", chain.ToolResultsGrowth.ValueOrZero())
	}
	if chain.HistoryGrowth.ValueOrZero() != 914 {
		t.Fatalf("history growth = %d, want 914", chain.HistoryGrowth.ValueOrZero())
	}
	if chain.ToolsGrowth.ValueOrZero() != 0 {
		t.Fatalf("tools growth = %d, want 0", chain.ToolsGrowth.ValueOrZero())
	}
}

func TestAnalyzeChains_DeltaPreservesNegativeZeroUnknown(t *testing.T) {
	calls := []int64{100, 80, 80}
	exchanges := metricChain("gw-delta", calls, nil)
	chains := AnalyzeChains(exchanges)
	if len(chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(chains))
	}
	got := chains[0]
	if got.Calls[1].ContextDelta.ValueOrZero() != -20 {
		t.Fatalf("negative delta = %d, want -20", got.Calls[1].ContextDelta.ValueOrZero())
	}
	if got.Calls[2].ContextDelta.ValueOrZero() != 0 {
		t.Fatalf("zero delta = %d, want 0", got.Calls[2].ContextDelta.ValueOrZero())
	}
	if got.LargestGrowth.ValueOrZero() != 0 || got.LargestGrowthCall != 0 {
		t.Fatalf("largest growth = %+v at %d, want zero without call", got.LargestGrowth, got.LargestGrowthCall)
	}

	unknown := metricChain("gw-unknown", []int64{100, 120}, map[int]bool{1: true})
	got = AnalyzeChains(unknown)[0]
	if got.Calls[1].ContextDelta.Available() {
		t.Fatalf("delta = %+v, want unknown", got.Calls[1].ContextDelta)
	}
	if got.TotalContextSent.Available() {
		t.Fatalf("total = %+v, want unknown", got.TotalContextSent)
	}
}

func TestAnalyzeChains_MeasurementKinds(t *testing.T) {
	exchanges := metricChain("gw-kind", []int64{10, 20}, nil)
	exchanges[0].Request.Components = []model.ContextComponent{
		measuredComponent(model.ContextHistory, 4, model.MeasurementCounted),
		measuredComponent(model.ContextToolResult, 6, model.MeasurementEstimated),
	}
	chains := AnalyzeChains(exchanges)
	got := chains[0]
	if got.Calls[0].Context.Kind != model.MeasurementEstimated {
		t.Fatalf("mixed context kind = %q, want estimated", got.Calls[0].Context.Kind)
	}
	if got.ContextGrowth.Kind != model.MeasurementEstimated {
		t.Fatalf("growth kind = %q, want estimated", got.ContextGrowth.Kind)
	}
}

func TestAnalyzeChains_UnknownNotConvertedToZero(t *testing.T) {
	exchanges := metricChain("gw-u", []int64{0}, map[int]bool{0: true})
	chains := AnalyzeChains(exchanges)
	if len(chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(chains))
	}
	if chains[0].InitialContext.Available() {
		t.Fatalf("initial = %+v, want unknown", chains[0].InitialContext)
	}
}

func TestAnalyzeChains_CountTokensExcludedFromCalls(t *testing.T) {
	exchanges := metricChain("gw-count", []int64{10, 20}, nil)
	exchanges[1].Request.Endpoint = "/v1/messages/count_tokens"
	chains := AnalyzeChains(exchanges)
	if len(chains) != 1 || chains[0].ModelCalls != 1 {
		t.Fatalf("chains = %+v, want one model call", chains)
	}
}

func TestRenderChains_ShowsSummaryColumns(t *testing.T) {
	chains := AnalyzeChains(observedMetricChain())
	var buf strings.Builder
	if err := RenderChains(&buf, chains); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"gwc-report0", "6", "338,452*", "75,701*", "mnm/MiniMax-M3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("chains output = %q, want %q", out, want)
		}
	}
}

func TestRenderChain_MeasurementMarkers(t *testing.T) {
	chain := ChainSummary{
		ID:         "gwc-markers",
		Profile:    "codex",
		Protocol:   ProtocolAnthropicMessages,
		Model:      "m",
		StartedAt:  "2026-09-09T12:00:00Z",
		ModelCalls: 1,
		Calls: []ChainCallSummary{{
			Index:      1,
			ExchangeID: "gw-marker",
			StartedAt:  "2026-09-09T12:00:00Z",
			Context:    model.NewMeasurement(1234, model.MeasurementCounted),
		}},
		TotalContextSent: model.NewMeasurement(1234, model.MeasurementCounted),
		PeakContext:      model.NewMeasurement(1234, model.MeasurementCounted),
		InitialContext:   model.NewMeasurement(1234, model.MeasurementCounted),
		FinalContext:     model.NewMeasurement(1234, model.MeasurementCounted),
		ContextGrowth:    model.NewMeasurement(0, model.MeasurementCounted),
		LargestGrowth:    model.Measurement{Kind: model.MeasurementUnknown},
	}
	var buf strings.Builder
	if err := RenderChain(&buf, chain); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "1,234*") {
		t.Fatalf("counted measurement was starred: %q", out)
	}
	if !strings.Contains(out, "1,234") || !strings.Contains(out, "—") {
		t.Fatalf("output missing counted or unknown marker: %q", out)
	}
}

func TestRenderChain_ShowsCallsInOrder(t *testing.T) {
	chain := AnalyzeChains(observedMetricChain())[0]
	var buf strings.Builder
	if err := RenderChain(&buf, chain); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	first := strings.Index(out, "gw-report0")
	last := strings.Index(out, "gw-report5")
	if first < 0 || last < 0 || first > last {
		t.Fatalf("detail output order = %q", out)
	}
	if !strings.Contains(out, "Tool-result growth") || !strings.Contains(out, "+36,606*") {
		t.Fatalf("detail output missing growth: %q", out)
	}
}

func TestFindChainExactPrefixAmbiguousMissing(t *testing.T) {
	chains := []ChainSummary{
		{ID: "gwc-aaaa1111"},
		{ID: "gwc-aaaa2222"},
		{ID: "gwc-bbbb1111"},
	}
	if got, err := FindChain(chains, "gwc-bbbb1111"); err != nil || got.ID != "gwc-bbbb1111" {
		t.Fatalf("exact lookup = %+v err=%v", got, err)
	}
	if got, err := FindChain(chains, "gwc-bbbb"); err != nil || got.ID != "gwc-bbbb1111" {
		t.Fatalf("prefix lookup = %+v err=%v", got, err)
	}
	if _, err := FindChain(chains, "gwc-aaaa"); !errors.Is(err, ErrChainAmbiguous) {
		t.Fatalf("ambiguous error = %v", err)
	}
	if _, err := FindChain(chains, "gwc-missing"); !errors.Is(err, ErrChainNotFound) {
		t.Fatalf("missing error = %v", err)
	}
}

func TestChainJSONNumericValuesAndKind(t *testing.T) {
	chain := AnalyzeChains(observedMetricChain())[0]
	raw, err := json.Marshal(chain)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	compact := strings.Join(strings.Fields(string(raw)), "")
	if !strings.Contains(compact, `"modelCalls":6`) || !strings.Contains(compact, `"value":338452`) {
		t.Fatalf("json values not numeric: %s", compact)
	}
	if !strings.Contains(compact, `"kind":"estimated"`) {
		t.Fatalf("json missing kind: %s", compact)
	}
	if strings.Contains(compact, `"338,452"`) {
		t.Fatalf("json contains formatted number: %s", compact)
	}
}

func TestRenderChainDoesNotPrintRawPromptOrToolResult(t *testing.T) {
	chain := AnalyzeChains(observedMetricChain())[0]
	var buf strings.Builder
	if err := RenderChain(&buf, chain); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, banned := range []string{"SECRET_PROMPT", "SECRET_TOOL_RESULT"} {
		if strings.Contains(out, banned) {
			t.Fatalf("output leaked %q: %q", banned, out)
		}
	}
}

func TestAnalyzeChains_UncorrelatedNotForced(t *testing.T) {
	exchanges := []Exchange{
		reportExchange("gw-a", 0, []string{"toolu_a"}, nil, true, estimatedBreakdown(10, 0, 0, 0)),
		reportExchange("gw-b", 1, nil, []string{"toolu_missing"}, false, estimatedBreakdown(20, 0, 0, 0)),
	}
	chains := AnalyzeChains(exchanges)
	if len(chains) != 1 || chains[0].ModelCalls != 1 {
		t.Fatalf("chains = %+v, want one unextended chain", chains)
	}
}

func observedMetricChain() []Exchange {
	rows := []struct {
		history      int64
		instructions int64
		tools        int64
		toolResults  int64
	}{
		{906, 1908, 28639, 6728},
		{1116, 1908, 28639, 14403},
		{1272, 1908, 28639, 14443},
		{1482, 1908, 28639, 24643},
		{1689, 1908, 28639, 43334},
		{1820, 1908, 28639, 43334},
	}
	var out []Exchange
	for i, row := range rows {
		var uses []string
		var results []string
		if i < len(rows)-1 {
			uses = []string{"toolu_report_" + strconv.Itoa(i)}
		}
		if i > 0 {
			results = []string{"toolu_report_" + strconv.Itoa(i-1)}
		}
		out = append(out, reportExchange("gw-report"+strconv.Itoa(i), int64(i), uses, results, i == 0, []model.ContextComponent{
			measuredComponent(model.ContextHistory, row.history, model.MeasurementEstimated),
			measuredComponent(model.ContextInstructions, row.instructions, model.MeasurementEstimated),
			measuredComponent(model.ContextToolDefinition, row.tools, model.MeasurementEstimated),
			measuredComponent(model.ContextToolResult, row.toolResults, model.MeasurementEstimated),
		}))
	}
	return out
}

func metricChain(prefix string, contexts []int64, unknown map[int]bool) []Exchange {
	var out []Exchange
	for i, context := range contexts {
		var uses []string
		var results []string
		if i < len(contexts)-1 {
			uses = []string{"toolu_" + prefix + "_" + strconv.Itoa(i)}
		}
		if i > 0 {
			results = []string{"toolu_" + prefix + "_" + strconv.Itoa(i-1)}
		}
		components := []model.ContextComponent{measuredComponent(model.ContextHistory, context, model.MeasurementEstimated)}
		if unknown[i] {
			components = []model.ContextComponent{{Kind: model.ContextHistory, Measurement: model.Measurement{Kind: model.MeasurementUnknown}}}
		}
		out = append(out, reportExchange(prefix+strconv.Itoa(i), int64(i), uses, results, i == 0, components))
	}
	return out
}

func estimatedBreakdown(history, instructions, tools, toolResults int64) []model.ContextComponent {
	return []model.ContextComponent{
		measuredComponent(model.ContextHistory, history, model.MeasurementEstimated),
		measuredComponent(model.ContextInstructions, instructions, model.MeasurementEstimated),
		measuredComponent(model.ContextToolDefinition, tools, model.MeasurementEstimated),
		measuredComponent(model.ContextToolResult, toolResults, model.MeasurementEstimated),
	}
}

func reportExchange(id string, seconds int64, toolUses, toolResults []string, fresh bool, components []model.ContextComponent) Exchange {
	exchange := anthropicExchange(id, "codex", "mnm/MiniMax-M3", seconds, anthropicBody("mnm/MiniMax-M3", toolUses, toolResults, fresh))
	exchange.Request.Components = components
	return exchange
}

func measuredComponent(kind model.ContextComponentKind, value int64, measurementKind model.MeasurementKind) model.ContextComponent {
	return model.ContextComponent{
		Kind:        kind,
		Observation: ObservationScope,
		Measurement: model.NewMeasurement(value, measurementKind),
	}
}

package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func int64p(v int64) *int64 { p := v; return &p }

func TestProviderUsageToObserved_Anthropic_FullContract(t *testing.T) {
	pu := &ProviderUsage{
		Source:                   "anthropic_messages",
		InputTokens:              int64p(100),
		CacheReadInputTokens:     int64p(800),
		CacheCreationInputTokens: int64p(20),
		OutputTokens:             int64p(30),
	}
	obs := ProviderUsageToObserved(pu)
	if obs.RawInput != 100 {
		t.Fatalf("RawInput = %d, want 100", obs.RawInput)
	}
	if obs.Cached != 800 {
		t.Fatalf("Cached = %d, want 800", obs.Cached)
	}
	if obs.CacheCreation != 20 {
		t.Fatalf("CacheCreation = %d, want 20", obs.CacheCreation)
	}
	if obs.TotalInput != 920 {
		t.Fatalf("TotalInput = %d, want 920", obs.TotalInput)
	}
	if obs.Output != 30 {
		t.Fatalf("Output = %d, want 30", obs.Output)
	}
	if obs.Reasoning != 0 {
		t.Fatalf("Reasoning = %d, want 0", obs.Reasoning)
	}
	if obs.Total != 950 {
		t.Fatalf("Total = %d, want 950", obs.Total)
	}
	if obs.Source != model.MeasurementMeasured {
		t.Fatalf("Source = %q, want measured", obs.Source)
	}
}

func TestProviderUsageToObserved_OpenAI_FullContract(t *testing.T) {
	pu := &ProviderUsage{
		Source:                "openai_responses",
		InputTokens:           int64p(900),
		CacheReadInputTokens:  int64p(800),
		OutputTokens:          int64p(30),
		ReasoningOutputTokens: int64p(10),
		TotalTokens:           int64p(930),
	}
	obs := ProviderUsageToObserved(pu)
	if obs.TotalInput != 900 {
		t.Fatalf("TotalInput = %d, want 900", obs.TotalInput)
	}
	if obs.Cached != 800 {
		t.Fatalf("Cached = %d, want 800", obs.Cached)
	}
	if obs.RawInput != 100 {
		t.Fatalf("RawInput = %d, want 100", obs.RawInput)
	}
	if obs.Output != 30 {
		t.Fatalf("Output = %d, want 30", obs.Output)
	}
	if obs.Reasoning != 10 {
		t.Fatalf("Reasoning = %d, want 10", obs.Reasoning)
	}
	if obs.Total != 930 {
		t.Fatalf("Total = %d, want 930", obs.Total)
	}
}

func TestProviderUsageToObserved_UnknownSourceNotMeasured(t *testing.T) {
	pu := &ProviderUsage{
		Source:       "future_provider",
		OutputTokens: int64p(42),
		TotalTokens:  int64p(99),
	}
	obs := ProviderUsageToObserved(pu)
	if obs.Source != model.MeasurementDerived {
		t.Fatalf("Source = %q, want derived", obs.Source)
	}
	if obs.Output != 42 {
		t.Fatalf("Output = %d, want 42", obs.Output)
	}
	if obs.Total != 99 {
		t.Fatalf("Total = %d, want 99", obs.Total)
	}
	if obs.RawInput != 0 || obs.Cached != 0 || obs.CacheCreation != 0 || obs.TotalInput != 0 {
		t.Fatalf("unknown source must not invent fields: %+v", obs)
	}
}

func TestProviderUsageToObserved_EmptySourceDerived(t *testing.T) {
	obs := ProviderUsageToObserved(&ProviderUsage{})
	if obs.Source != model.MeasurementDerived {
		t.Fatalf("Source = %q, want derived for empty source", obs.Source)
	}
}

func TestProviderUsageToObserved_NilReturnsDerived(t *testing.T) {
	obs := ProviderUsageToObserved(nil)
	if obs.Source != model.MeasurementDerived {
		t.Fatalf("Source = %q, want derived for nil", obs.Source)
	}
}

func TestProviderUsageToObserved_RawProviderUsageUnchanged(t *testing.T) {
	input := int64(900)
	cached := int64(800)
	pu := &ProviderUsage{
		Source:               "openai_responses",
		InputTokens:          &input,
		CacheReadInputTokens: &cached,
	}
	_ = ProviderUsageToObserved(pu)
	if pu.InputTokens == nil || *pu.InputTokens != 900 {
		t.Fatalf("provider input_tokens mutated: %+v", pu.InputTokens)
	}
	if pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 800 {
		t.Fatalf("provider cached mutated: %+v", pu.CacheReadInputTokens)
	}
}

func TestProviderUsageToObserved_OpenAIReasoningDoesNotDoubleCount(t *testing.T) {
	pu := &ProviderUsage{
		Source:                "openai_responses",
		InputTokens:           int64p(900),
		CacheReadInputTokens:  int64p(800),
		OutputTokens:          int64p(30),
		ReasoningOutputTokens: int64p(10),
		TotalTokens:           int64p(930),
	}
	obs := ProviderUsageToObserved(pu)
	if obs.Output != 30 {
		t.Fatalf("Output = %d, want 30 (reasoning is sub-bucket)", obs.Output)
	}
	if obs.Reasoning != 10 {
		t.Fatalf("Reasoning = %d, want 10", obs.Reasoning)
	}
	if obs.Total != 930 {
		t.Fatalf("Total = %d, want 930 (reasoning must not fold into total)", obs.Total)
	}
}

func TestProviderUsageToObserved_AnthropicMissingVsExplicitZeroCacheCreation(t *testing.T) {
	missing := &ProviderUsage{
		Source:               "anthropic_messages",
		InputTokens:          int64p(100),
		CacheReadInputTokens: int64p(800),
		OutputTokens:         int64p(30),
	}
	zero := &ProviderUsage{
		Source:                   "anthropic_messages",
		InputTokens:              int64p(100),
		CacheReadInputTokens:     int64p(800),
		CacheCreationInputTokens: int64p(0),
		OutputTokens:             int64p(30),
	}
	if ProviderUsageToObserved(missing).CacheCreation != 0 {
		t.Fatalf("missing cache_creation should observe 0")
	}
	if ProviderUsageToObserved(zero).CacheCreation != 0 {
		t.Fatalf("explicit zero cache_creation should observe 0")
	}
	missingJSON, _ := json.Marshal(missing)
	zeroJSON, _ := json.Marshal(zero)
	if strings.Contains(string(missingJSON), "cacheCreationInputTokens") {
		t.Fatalf("missing field must be absent from JSON, got %s", missingJSON)
	}
	if !strings.Contains(string(zeroJSON), `"cacheCreationInputTokens":0`) {
		t.Fatalf("explicit zero must round-trip in JSON, got %s", zeroJSON)
	}
}

func TestProviderUsageToObserved_NegativeFreshImpossible(t *testing.T) {
	cases := []struct {
		name string
		pu   *ProviderUsage
	}{
		{"anthropic_cached_greater_than_input", &ProviderUsage{Source: "anthropic_messages", InputTokens: int64p(10), CacheReadInputTokens: int64p(20)}},
		{"openai_cached_greater_than_input", &ProviderUsage{Source: "openai_responses", InputTokens: int64p(10), CacheReadInputTokens: int64p(2000)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := ProviderUsageToObserved(tc.pu)
			if obs.RawInput < 0 {
				t.Fatalf("RawInput = %d, want >= 0", obs.RawInput)
			}
			if obs.Cached < 0 {
				t.Fatalf("Cached = %d, want >= 0", obs.Cached)
			}
			if obs.TotalInput < 0 {
				t.Fatalf("TotalInput = %d, want >= 0", obs.TotalInput)
			}
		})
	}
}

func TestProvidersAgreeAcrossExchangePipeline_AnthropicFull(t *testing.T) {
	in := int64(100)
	cc := int64(20)
	cr := int64(800)
	out := int64(30)
	pu := &ProviderUsage{
		Source:                   "anthropic_messages",
		InputTokens:              &in,
		CacheCreationInputTokens: &cc,
		CacheReadInputTokens:     &cr,
		OutputTokens:             &out,
	}
	ex := Exchange{
		ID:        "gw-agree-anthropic",
		Profile:   "p",
		Protocol:  ProtocolAnthropicMessages,
		StartedAt: time.Unix(0, 0),
		Kind:      RequestKindModel,
		Request: ExchangeRequest{
			Method:   http.MethodPost,
			Endpoint: "/v1/messages",
		},
		Response: ExchangeResponse{ProviderUsage: pu},
	}
	observed := ProviderUsageToObserved(pu)
	request := SummarizeRequest(ex)
	call, _, _ := summarizeCallProviderUsage(ex, request)
	account := AccountProfile("p", []Exchange{ex}, ChainBuildResult{}, RecorderFailureRead{})

	if observed.RawInput != 100 || observed.Cached != 800 || observed.TotalInput != 920 ||
		observed.Output != 30 || observed.Total != 950 {
		t.Fatalf("observed mismatch: %+v", observed)
	}
	if call == nil || call.FreshInput == nil || *call.FreshInput != 100 {
		t.Fatalf("call FreshInput = %+v, want 100", call)
	}
	if call.CachedInput == nil || *call.CachedInput != 800 {
		t.Fatalf("call CachedInput = %+v, want 800", call.CachedInput)
	}
	if call.CacheCreationInput == nil || *call.CacheCreationInput != 20 {
		t.Fatalf("call CacheCreationInput = %+v, want 20", call.CacheCreationInput)
	}
	if call.TotalInput == nil || *call.TotalInput != 920 {
		t.Fatalf("call TotalInput = %+v, want 920", call.TotalInput)
	}
	if call.Output == nil || *call.Output != 30 {
		t.Fatalf("call Output = %+v, want 30", call.Output)
	}
	if account.Observed.TotalInput.Sum != 920 {
		t.Fatalf("profile TotalInput = %d, want 920", account.Observed.TotalInput.Sum)
	}
	if account.Observed.Total.Sum != 950 {
		t.Fatalf("profile Total = %d, want 950", account.Observed.Total.Sum)
	}
	if request.Provider == nil || request.Provider.CachedInput == nil || *request.Provider.CachedInput != 800 {
		t.Fatalf("request CachedInput = %+v, want 800", request.Provider)
	}
}

func TestProvidersAgreeAcrossExchangePipeline_OpenAIWithReasoning(t *testing.T) {
	in := int64(900)
	cr := int64(800)
	out := int64(30)
	reasoning := int64(10)
	total := int64(930)
	pu := &ProviderUsage{
		Source:                "openai_responses",
		InputTokens:           &in,
		CacheReadInputTokens:  &cr,
		OutputTokens:          &out,
		ReasoningOutputTokens: &reasoning,
		TotalTokens:           &total,
	}
	ex := Exchange{
		ID:        "gw-agree-openai",
		Profile:   "p",
		Protocol:  ProtocolOpenAIResponses,
		StartedAt: time.Unix(0, 0),
		Kind:      RequestKindModel,
		Request: ExchangeRequest{
			Method:   http.MethodPost,
			Endpoint: "/v1/responses",
		},
		Response: ExchangeResponse{ProviderUsage: pu},
	}
	observed := ProviderUsageToObserved(pu)
	request := SummarizeRequest(ex)
	call, _, _ := summarizeCallProviderUsage(ex, request)
	account := AccountProfile("p", []Exchange{ex}, ChainBuildResult{}, RecorderFailureRead{})

	if observed.RawInput != 100 || observed.Reasoning != 10 || observed.Total != 930 {
		t.Fatalf("observed mismatch: %+v", observed)
	}
	if call == nil || call.Output == nil || *call.Output != 30 {
		t.Fatalf("call Output = %+v, want 30", call.Output)
	}
	if call.ReasoningOutput == nil || *call.ReasoningOutput != 10 {
		t.Fatalf("call ReasoningOutput = %+v, want 10", call.ReasoningOutput)
	}
	if account.Observed.Reasoning.Sum != 10 {
		t.Fatalf("profile Reasoning = %d, want 10", account.Observed.Reasoning.Sum)
	}
	if account.Observed.Output.Sum != 30 {
		t.Fatalf("profile Output = %d, want 30 (reasoning must not double-count)", account.Observed.Output.Sum)
	}
	if request.Provider == nil || request.Provider.ReasoningOutput == nil || *request.Provider.ReasoningOutput != 10 {
		t.Fatalf("request ReasoningOutput = %+v, want 10", request.Provider)
	}
}

func TestProviderUsageToObservedApply_ReplacesCanonicalAndKeepsTruncated(t *testing.T) {
	in := int64(100)
	out := int64(30)
	cached := int64(80)
	cases := []struct {
		name       string
		existing   *ObservedUsage
		pu         *ProviderUsage
		wantIn     int64
		wantOut    int64
		wantCached int64
		wantSource model.MeasurementKind
	}{
		{
			name:       "explicit_zero_provider_value_is_preserved",
			existing:   &ObservedUsage{RawInput: 999, Output: 444, Source: model.MeasurementMeasured},
			pu:         &ProviderUsage{Source: "anthropic_messages", InputTokens: int64p(0), OutputTokens: int64p(0), CacheReadInputTokens: int64p(0)},
			wantIn:     0,
			wantOut:    0,
			wantCached: 0,
			wantSource: model.MeasurementMeasured,
		},
		{
			name:       "known_values_replaced_once",
			existing:   &ObservedUsage{RawInput: 7, Cached: 7, CacheCreation: 7, TotalInput: 21, Output: 7, Reasoning: 7, Total: 28, Source: model.MeasurementUnknown},
			pu:         &ProviderUsage{Source: "anthropic_messages", InputTokens: &in, CacheReadInputTokens: &cached, OutputTokens: &out},
			wantIn:     100,
			wantOut:    30,
			wantCached: 80,
			wantSource: model.MeasurementMeasured,
		},
		{
			name:       "unknown_schema_stays_derived",
			existing:   nil,
			pu:         &ProviderUsage{Source: "future_provider", OutputTokens: int64p(42)},
			wantOut:    42,
			wantSource: model.MeasurementDerived,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := Exchange{Response: ExchangeResponse{ProviderUsage: tc.pu, Usage: tc.existing}}
			if tc.existing != nil {
				ex.Response.Usage.Truncated = true
			}
			ProviderUsageToObservedApply(&ex)
			if ex.Response.Usage.RawInput != tc.wantIn || ex.Response.Usage.Output != tc.wantOut || ex.Response.Usage.Cached != tc.wantCached {
				t.Fatalf("counts = %+v", ex.Response.Usage)
			}
			if ex.Response.Usage.Source != tc.wantSource {
				t.Fatalf("source = %q, want %q", ex.Response.Usage.Source, tc.wantSource)
			}
			if tc.existing != nil && !ex.Response.Usage.Truncated {
				t.Fatalf("truncated state must be preserved: %+v", ex.Response.Usage)
			}
		})
	}
}

func TestAccountProfile_UnknownSchemaKeepsOnlyProvableFields(t *testing.T) {
	input := int64(500)
	output := int64(20)
	ex := mkBucketEx("gw-u", ProtocolOpenAIResponses, http.MethodPost, "/responses", RequestKindModel,
		&ProviderUsage{Source: "future_provider", InputTokens: &input, OutputTokens: &output})
	acc := AccountProfile("p", []Exchange{ex}, ChainBuildResult{}, RecorderFailureRead{})
	if acc.Observed.Complete {
		t.Fatalf("unknown usage schema must not flip aggregate to complete: %+v", acc.Observed)
	}
	if acc.Completeness != "partial" {
		t.Fatalf("completeness = %q, want partial", acc.Completeness)
	}
	if acc.Observed.Output.Sum != 20 {
		t.Fatalf("directly reported output must be kept: %+v", acc.Observed.Output)
	}
	if acc.Observed.TotalInput.Sum != 0 || acc.Observed.RawInput.Sum != 0 || acc.Observed.Cached.Sum != 0 {
		t.Fatalf("unknown schema must not invent input fields: %+v", acc.Observed)
	}
}

func TestAccountProfile_AnthropicDerivedFieldsKeepDerivedKind(t *testing.T) {
	in := int64(100)
	out := int64(30)
	ex := mkBucketEx("gw-a", ProtocolAnthropicMessages, http.MethodPost, "/v1/messages", RequestKindModel,
		&ProviderUsage{Source: "anthropic_messages", InputTokens: &in, OutputTokens: &out})
	acc := AccountProfile("p", []Exchange{ex}, ChainBuildResult{}, RecorderFailureRead{})
	if acc.Observed.RawInput.Kind != model.MeasurementMeasured {
		t.Fatalf("rawInput kind = %q, want measured", acc.Observed.RawInput.Kind)
	}
	if acc.Observed.TotalInput.Kind != model.MeasurementDerived {
		t.Fatalf("totalInput kind = %q, want derived (computed sum)", acc.Observed.TotalInput.Kind)
	}
	if acc.Observed.Total.Kind != model.MeasurementDerived {
		t.Fatalf("total kind = %q, want derived (computed sum)", acc.Observed.Total.Kind)
	}
	if acc.Observed.Output.Kind != model.MeasurementMeasured {
		t.Fatalf("output kind = %q, want measured", acc.Observed.Output.Kind)
	}
}

func TestProfileUsageAggregate_TotalCountedOnce(t *testing.T) {
	anthropic := string(ProtocolAnthropicMessages)
	openai := string(ProtocolOpenAIResponses)
	cases := []struct {
		name      string
		usage     *ProviderUsage
		wantCount int
		wantSum   int64
		wantKind  model.MeasurementKind
	}{
		{
			name: "anthropic_reported_total_is_measured_once",
			usage: &ProviderUsage{
				Source: anthropic, InputTokens: int64p(100), CacheReadInputTokens: int64p(800),
				CacheCreationInputTokens: int64p(20), OutputTokens: int64p(30), TotalTokens: int64p(950),
			},
			wantCount: 1, wantSum: 950, wantKind: model.MeasurementMeasured,
		},
		{
			name: "anthropic_derived_total_requires_input_and_output",
			usage: &ProviderUsage{
				Source: anthropic, InputTokens: int64p(100), CacheReadInputTokens: int64p(800),
				CacheCreationInputTokens: int64p(20), OutputTokens: int64p(30),
			},
			wantCount: 1, wantSum: 950, wantKind: model.MeasurementDerived,
		},
		{
			name: "openai_reported_total_is_measured_once",
			usage: &ProviderUsage{
				Source: openai, InputTokens: int64p(900), CacheReadInputTokens: int64p(800),
				OutputTokens: int64p(30), TotalTokens: int64p(930),
			},
			wantCount: 1, wantSum: 930, wantKind: model.MeasurementMeasured,
		},
		{
			name: "openai_derived_total_requires_input_and_output",
			usage: &ProviderUsage{
				Source: openai, InputTokens: int64p(900), CacheReadInputTokens: int64p(800),
				OutputTokens: int64p(30),
			},
			wantCount: 1, wantSum: 930, wantKind: model.MeasurementDerived,
		},
		{
			name:      "anthropic_missing_output_does_not_invent_total",
			usage:     &ProviderUsage{Source: anthropic, InputTokens: int64p(100)},
			wantCount: 0, wantSum: 0,
		},
		{
			name:      "anthropic_missing_input_does_not_invent_total",
			usage:     &ProviderUsage{Source: anthropic, OutputTokens: int64p(30)},
			wantCount: 0, wantSum: 0,
		},
		{
			name:      "openai_missing_output_does_not_invent_total",
			usage:     &ProviderUsage{Source: openai, InputTokens: int64p(900)},
			wantCount: 0, wantSum: 0,
		},
		{
			name:      "openai_missing_input_does_not_invent_total",
			usage:     &ProviderUsage{Source: openai, OutputTokens: int64p(30)},
			wantCount: 0, wantSum: 0,
		},
		{
			name:      "anthropic_explicit_zero_total_is_observed",
			usage:     &ProviderUsage{Source: anthropic, InputTokens: int64p(0), OutputTokens: int64p(0), TotalTokens: int64p(0)},
			wantCount: 1, wantSum: 0, wantKind: model.MeasurementMeasured,
		},
		{
			name:      "openai_explicit_zero_total_is_observed",
			usage:     &ProviderUsage{Source: openai, InputTokens: int64p(0), OutputTokens: int64p(0), TotalTokens: int64p(0)},
			wantCount: 1, wantSum: 0, wantKind: model.MeasurementMeasured,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var agg ProfileUsageAggregate
			agg.record(tc.usage)
			if agg.Total.Count != tc.wantCount {
				t.Fatalf("Total.Count = %d, want %d", agg.Total.Count, tc.wantCount)
			}
			if agg.Total.Sum != tc.wantSum {
				t.Fatalf("Total.Sum = %d, want %d", agg.Total.Sum, tc.wantSum)
			}
			if tc.wantCount > 0 && agg.Total.Kind != tc.wantKind {
				t.Fatalf("Total.Kind = %q, want %q", agg.Total.Kind, tc.wantKind)
			}
		})
	}
}

func TestProfileUsageAggregate_TotalCountsOncePerRequest(t *testing.T) {
	usage := &ProviderUsage{
		Source: string(ProtocolAnthropicMessages), InputTokens: int64p(100),
		OutputTokens: int64p(30), TotalTokens: int64p(130),
	}
	var agg ProfileUsageAggregate
	agg.record(usage)
	agg.record(usage)
	if agg.Total.Count != 2 {
		t.Fatalf("Total.Count = %d, want 2 (one per request)", agg.Total.Count)
	}
	if agg.Total.Sum != 260 {
		t.Fatalf("Total.Sum = %d, want 260", agg.Total.Sum)
	}
}

func TestProfileUsageAggregate_OpenAITotalInputTokens(t *testing.T) {
	openai := string(ProtocolOpenAIResponses)
	cases := []struct {
		name               string
		usage              *ProviderUsage
		wantTotalInput     int
		wantTotalInputSum  int64
		wantTotalInputKind model.MeasurementKind
		wantCached         int
		wantCachedSum      int64
		wantFresh          int
		wantFreshSum       int64
		wantTotal          int
		wantTotalSum       int64
		wantTotalKind      model.MeasurementKind
	}{
		{
			name:               "total_input_tokens_only",
			usage:              &ProviderUsage{Source: openai, TotalInputTokens: int64p(900), OutputTokens: int64p(30)},
			wantTotalInput:     1,
			wantTotalInputSum:  900,
			wantTotalInputKind: model.MeasurementMeasured,
			wantTotal:          1,
			wantTotalSum:       930,
			wantTotalKind:      model.MeasurementDerived,
		},
		{
			name:               "input_tokens_and_total_input_tokens_prefer_total",
			usage:              &ProviderUsage{Source: openai, InputTokens: int64p(800), TotalInputTokens: int64p(900), CacheReadInputTokens: int64p(100), OutputTokens: int64p(30)},
			wantTotalInput:     1,
			wantTotalInputSum:  900,
			wantTotalInputKind: model.MeasurementMeasured,
			wantCached:         1,
			wantCachedSum:      100,
			wantFresh:          1,
			wantFreshSum:       800,
			wantTotal:          1,
			wantTotalSum:       930,
			wantTotalKind:      model.MeasurementDerived,
		},
		{
			name:               "explicit_zero_total_input_tokens",
			usage:              &ProviderUsage{Source: openai, TotalInputTokens: int64p(0), OutputTokens: int64p(0)},
			wantTotalInput:     1,
			wantTotalInputSum:  0,
			wantTotalInputKind: model.MeasurementMeasured,
			wantTotal:          1,
			wantTotalSum:       0,
			wantTotalKind:      model.MeasurementDerived,
		},
		{
			name:           "both_input_fields_missing",
			usage:          &ProviderUsage{Source: openai, OutputTokens: int64p(30)},
			wantTotalInput: 0,
			wantTotal:      0,
		},
		{
			name:               "total_input_tokens_without_cached_has_no_fresh",
			usage:              &ProviderUsage{Source: openai, TotalInputTokens: int64p(900), OutputTokens: int64p(30), ReasoningOutputTokens: int64p(5)},
			wantTotalInput:     1,
			wantTotalInputSum:  900,
			wantTotalInputKind: model.MeasurementMeasured,
			wantFresh:          0,
			wantTotal:          1,
			wantTotalSum:       930,
			wantTotalKind:      model.MeasurementDerived,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var agg ProfileUsageAggregate
			agg.record(tc.usage)
			if agg.TotalInput.Count != tc.wantTotalInput {
				t.Fatalf("TotalInput.Count = %d, want %d", agg.TotalInput.Count, tc.wantTotalInput)
			}
			if agg.TotalInput.Sum != tc.wantTotalInputSum {
				t.Fatalf("TotalInput.Sum = %d, want %d", agg.TotalInput.Sum, tc.wantTotalInputSum)
			}
			if tc.wantTotalInput > 0 && agg.TotalInput.Kind != tc.wantTotalInputKind {
				t.Fatalf("TotalInput.Kind = %q, want %q", agg.TotalInput.Kind, tc.wantTotalInputKind)
			}
			if agg.Cached.Count != tc.wantCached || agg.Cached.Sum != tc.wantCachedSum {
				t.Fatalf("Cached = %+v, want count=%d sum=%d", agg.Cached, tc.wantCached, tc.wantCachedSum)
			}
			if agg.RawInput.Count != tc.wantFresh || agg.RawInput.Sum != tc.wantFreshSum {
				t.Fatalf("RawInput = %+v, want count=%d sum=%d", agg.RawInput, tc.wantFresh, tc.wantFreshSum)
			}
			if agg.Total.Count != tc.wantTotal {
				t.Fatalf("Total.Count = %d, want %d", agg.Total.Count, tc.wantTotal)
			}
			if agg.Total.Sum != tc.wantTotalSum {
				t.Fatalf("Total.Sum = %d, want %d", agg.Total.Sum, tc.wantTotalSum)
			}
			if tc.wantTotal > 0 && agg.Total.Kind != tc.wantTotalKind {
				t.Fatalf("Total.Kind = %q, want %q", agg.Total.Kind, tc.wantTotalKind)
			}
		})
	}
}

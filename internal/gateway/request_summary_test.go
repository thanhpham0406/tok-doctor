package gateway

import (
	"net/http"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func mkReqSummaryEx(pu *ProviderUsage, attributed int64) Exchange {
	ex := Exchange{
		ID:        "gw-rs",
		Profile:   "p",
		Protocol:  ProtocolAnthropicMessages,
		Kind:      RequestKindModel,
		StartedAt: time.Unix(0, 0),
		Request: ExchangeRequest{
			Method: http.MethodPost,
			Components: []model.ContextComponent{
				{Kind: model.ContextInstructions, Measurement: model.NewMeasurement(attributed, model.MeasurementEstimated)},
			},
		},
		Response: ExchangeResponse{ProviderUsage: pu},
	}
	return ex
}

func TestRequestProviderSummary_AnthropicExposesAllBuckets(t *testing.T) {
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
	s := buildRequestProviderSummary(mkReqSummaryEx(pu, 0), model.Measurement{})
	if s == nil {
		t.Fatalf("summary nil")
	}
	if s.FreshInput == nil || *s.FreshInput != 100 {
		t.Fatalf("FreshInput = %+v, want 100", s.FreshInput)
	}
	if s.CachedInput == nil || *s.CachedInput != 800 {
		t.Fatalf("CachedInput = %+v, want 800", s.CachedInput)
	}
	if s.CacheCreationInput == nil || *s.CacheCreationInput != 20 {
		t.Fatalf("CacheCreationInput = %+v, want 20", s.CacheCreationInput)
	}
	if s.TotalInput == nil || *s.TotalInput != 920 {
		t.Fatalf("TotalInput = %+v, want 920", s.TotalInput)
	}
	if s.Output == nil || *s.Output != 30 {
		t.Fatalf("Output = %+v, want 30", s.Output)
	}
	if s.Total == nil || *s.Total != 950 {
		t.Fatalf("Total = %+v, want 950", s.Total)
	}
	if s.ReasoningOutput != nil {
		t.Fatalf("ReasoningOutput = %+v, want nil", s.ReasoningOutput)
	}
	if s.Kind != "measured" {
		t.Fatalf("Kind = %q, want measured", s.Kind)
	}
}

func TestRequestProviderSummary_OpenAIExposesAllBuckets(t *testing.T) {
	in := int64(900)
	cr := int64(800)
	out := int64(30)
	reasoning := int64(10)
	total := int64(930)
	ex := mkReqSummaryEx(nil, 0)
	ex.Protocol = ProtocolOpenAIResponses
	pu := &ProviderUsage{
		Source:                "openai_responses",
		InputTokens:           &in,
		CacheReadInputTokens:  &cr,
		OutputTokens:          &out,
		ReasoningOutputTokens: &reasoning,
		TotalTokens:           &total,
	}
	ex.Response.ProviderUsage = pu
	s := buildRequestProviderSummary(ex, model.Measurement{})
	if s.TotalInput == nil || *s.TotalInput != 900 {
		t.Fatalf("TotalInput = %+v, want 900", s.TotalInput)
	}
	if s.CachedInput == nil || *s.CachedInput != 800 {
		t.Fatalf("CachedInput = %+v, want 800", s.CachedInput)
	}
	if s.FreshInput == nil || *s.FreshInput != 100 {
		t.Fatalf("FreshInput = %+v, want 100", s.FreshInput)
	}
	if s.Output == nil || *s.Output != 30 {
		t.Fatalf("Output = %+v, want 30", s.Output)
	}
	if s.ReasoningOutput == nil || *s.ReasoningOutput != 10 {
		t.Fatalf("ReasoningOutput = %+v, want 10", s.ReasoningOutput)
	}
	if s.Total == nil || *s.Total != 930 {
		t.Fatalf("Total = %+v, want 930", s.Total)
	}
	if s.CacheCreationInput != nil {
		t.Fatalf("CacheCreationInput must stay nil for OpenAI, got %+v", s.CacheCreationInput)
	}
}

func TestRequestProviderSummary_ExplicitZeroFreshPreserved(t *testing.T) {
	in := int64(0)
	cr := int64(0)
	out := int64(30)
	pu := &ProviderUsage{
		Source:               "anthropic_messages",
		InputTokens:          &in,
		CacheReadInputTokens: &cr,
		OutputTokens:         &out,
	}
	s := buildRequestProviderSummary(mkReqSummaryEx(pu, 0), model.Measurement{})
	if s.FreshInput == nil || *s.FreshInput != 0 {
		t.Fatalf("FreshInput = %+v, want non-nil pointer to 0", s.FreshInput)
	}
}

func TestRequestProviderSummary_OpenAIMissingCachedStaysNoFresh(t *testing.T) {
	in := int64(900)
	out := int64(30)
	pu := &ProviderUsage{
		Source:       "openai_responses",
		InputTokens:  &in,
		OutputTokens: &out,
	}
	s := buildRequestProviderSummary(mkReqSummaryEx(pu, 0), model.Measurement{})
	if s.TotalInput == nil || *s.TotalInput != 900 {
		t.Fatalf("TotalInput = %+v, want 900", s.TotalInput)
	}
	if s.FreshInput != nil {
		t.Fatalf("FreshInput must be nil when cached not reported, got %+v", s.FreshInput)
	}
	if s.CachedInput != nil {
		t.Fatalf("CachedInput must be nil, got %+v", s.CachedInput)
	}
}

func TestRequestProviderSummary_MissingFieldStaysNil(t *testing.T) {
	in := int64(100)
	pu := &ProviderUsage{
		Source:      "anthropic_messages",
		InputTokens: &in,
	}
	s := buildRequestProviderSummary(mkReqSummaryEx(pu, 0), model.Measurement{})
	if s.Output != nil {
		t.Fatalf("Output must be nil, got %+v", s.Output)
	}
	if s.CacheCreationInput != nil {
		t.Fatalf("CacheCreationInput must be nil, got %+v", s.CacheCreationInput)
	}
	if s.CachedInput != nil {
		t.Fatalf("CachedInput must be nil, got %+v", s.CachedInput)
	}
	if s.TotalInput == nil || *s.TotalInput != 100 {
		t.Fatalf("TotalInput = %+v, want 100", s.TotalInput)
	}
	if s.FreshInput == nil || *s.FreshInput != 100 {
		t.Fatalf("FreshInput = %+v, want 100", s.FreshInput)
	}
}

func TestRequestProviderSummary_AttributionGapUsesTotalInput(t *testing.T) {
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
	attributed := model.NewMeasurement(500, model.MeasurementEstimated)
	s := buildRequestProviderSummary(mkReqSummaryEx(pu, 500), attributed)
	if s.Gap == nil {
		t.Fatalf("Gap nil")
	}
	if s.Gap.ValueOrZero() != 420 {
		t.Fatalf("Gap = %d, want 420 (920 - 500)", s.Gap.ValueOrZero())
	}
	if s.Coverage == nil || s.Coverage.Percent == nil {
		t.Fatalf("Coverage nil")
	}
	pct := *s.Coverage.Percent
	if pct < 54.0 || pct > 54.5 {
		t.Fatalf("Coverage = %.2f, want ~54.3", pct)
	}
}

func TestRequestProviderSummary_UnknownSourceKindDerived(t *testing.T) {
	out := int64(30)
	total := int64(99)
	pu := &ProviderUsage{
		Source:       "future_provider",
		OutputTokens: &out,
		TotalTokens:  &total,
	}
	s := buildRequestProviderSummary(mkReqSummaryEx(pu, 0), model.Measurement{})
	if s.Kind != "derived" {
		t.Fatalf("Kind = %q, want derived", s.Kind)
	}
	if s.Output == nil || *s.Output != 30 {
		t.Fatalf("Output = %+v, want 30", s.Output)
	}
	if s.Total == nil || *s.Total != 99 {
		t.Fatalf("Total = %+v, want 99", s.Total)
	}
	if s.FreshInput != nil || s.CachedInput != nil || s.CacheCreationInput != nil {
		t.Fatalf("unknown source must not split fresh/cached: %+v", s)
	}
}

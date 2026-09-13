package gateway

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func observationExchange() Exchange {
	return Exchange{
		SchemaVersion: ExchangeSchemaVersion,
		ID:            "ex-1",
		Profile:       "default",
		SourceHint:    "codex",
		Protocol:      ProtocolAnthropicMessages,
		StartedAt:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Upstream:      "https://upstream.example",
		Model:         "request-model",
		Kind:          RequestKindModel,
		Outcome:       OutcomeUpstreamOK,
		Request: ExchangeRequest{
			Method:   "POST",
			Endpoint: "/v1/messages",
		},
		Response: ExchangeResponse{
			Status: 200,
			Model:  "response-model",
		},
	}
}

func anthropicProviderUsage(input, cacheRead, cacheCreation, output, reasoning, totalInput, total *int64) *ProviderUsage {
	return &ProviderUsage{
		InputTokens:              input,
		CacheReadInputTokens:     cacheRead,
		CacheCreationInputTokens: cacheCreation,
		OutputTokens:             output,
		ReasoningOutputTokens:    reasoning,
		TotalInputTokens:         totalInput,
		TotalTokens:              total,
		Source:                   string(ProtocolAnthropicMessages),
	}
}

func openAIProviderUsage(input, cacheRead, output, reasoning, totalInput, total *int64) *ProviderUsage {
	return &ProviderUsage{
		InputTokens:           input,
		CacheReadInputTokens:  cacheRead,
		OutputTokens:          output,
		ReasoningOutputTokens: reasoning,
		TotalInputTokens:      totalInput,
		TotalTokens:           total,
		Source:                string(ProtocolOpenAIResponses),
	}
}

func projectExchange(t *testing.T, e Exchange) model.Observation {
	t.Helper()
	observation, err := ObservationFromExchange(e)
	if err != nil {
		t.Fatalf("ObservationFromExchange: %v", err)
	}
	if err := observation.Validate(); err != nil {
		t.Fatalf("projected observation invalid: %v", err)
	}
	return observation
}

func assertObservationUsage(t *testing.T, got, want model.ObservationUsage) {
	t.Helper()
	for _, field := range []struct {
		name string
		got  model.Measurement
		want model.Measurement
	}{
		{"freshInput", got.FreshInput, want.FreshInput},
		{"cachedInput", got.CachedInput, want.CachedInput},
		{"cacheCreationInput", got.CacheCreationInput, want.CacheCreationInput},
		{"totalInput", got.TotalInput, want.TotalInput},
		{"output", got.Output, want.Output},
		{"reasoningOutput", got.ReasoningOutput, want.ReasoningOutput},
		{"total", got.Total, want.Total},
	} {
		if (field.got.Value == nil) != (field.want.Value == nil) {
			t.Fatalf("%s value presence = %v, want %v", field.name, field.got.Value, field.want.Value)
		}
		if field.got.Value != nil && *field.got.Value != *field.want.Value {
			t.Fatalf("%s value = %d, want %d", field.name, *field.got.Value, *field.want.Value)
		}
		if field.got.Kind != field.want.Kind {
			t.Fatalf("%s kind = %q, want %q", field.name, field.got.Kind, field.want.Kind)
		}
	}
}

func assertSourceValueEvidence(t *testing.T, m model.Measurement, record, field string) {
	t.Helper()
	want := model.Evidence{
		Kind:   model.EvidenceSourceValue,
		Source: observationEvidenceSource,
		Record: record,
		Field:  field,
	}
	assertSingleEvidence(t, m, want)
}

func assertAggregateEvidence(t *testing.T, m model.Measurement, record string, count int) {
	t.Helper()
	want := model.Evidence{
		Kind:      model.EvidenceAggregate,
		Source:    observationEvidenceSource,
		Record:    record,
		Operation: model.AggregateSum,
		Count:     count,
	}
	assertSingleEvidence(t, m, want)
}

func assertProvenanceEvidence(t *testing.T, m model.Measurement, record, field string) {
	t.Helper()
	want := model.Evidence{
		Kind:   model.EvidenceProvenance,
		Source: observationEvidenceSource,
		Record: record,
		Field:  field,
	}
	assertSingleEvidence(t, m, want)
}

func assertSingleEvidence(t *testing.T, m model.Measurement, want model.Evidence) {
	t.Helper()
	if len(m.Evidence) != 1 {
		t.Fatalf("evidence = %+v, want exactly one entry", m.Evidence)
	}
	if !reflect.DeepEqual(m.Evidence[0], want) {
		t.Fatalf("evidence = %+v, want %+v", m.Evidence[0], want)
	}
}

func TestObservationFromExchangeStructuralMapping(t *testing.T) {
	e := observationExchange()
	e.Response.ProviderUsage = anthropicProviderUsage(int64p(100), int64p(40), int64p(10), int64p(25), int64p(5), nil, nil)
	observation := projectExchange(t, e)

	if observation.SchemaVersion != model.ObservationSchemaVersion {
		t.Fatalf("schema version = %d, want %d", observation.SchemaVersion, model.ObservationSchemaVersion)
	}
	if observation.ID != "gateway:default:ex-1" {
		t.Fatalf("id = %q, want gateway:default:ex-1", observation.ID)
	}
	if observation.Channel != model.ObservationChannelGateway {
		t.Fatalf("channel = %q, want gateway", observation.Channel)
	}
	if observation.Scope != model.ObservationScopeRequest {
		t.Fatalf("scope = %q, want request", observation.Scope)
	}
	if observation.Source != "codex" {
		t.Fatalf("source = %q, want codex", observation.Source)
	}
	if observation.Model != "response-model" {
		t.Fatalf("model = %q, want response-model", observation.Model)
	}
	if observation.StartedAt == nil || !observation.StartedAt.Equal(e.StartedAt) {
		t.Fatalf("startedAt = %v, want %v", observation.StartedAt, e.StartedAt)
	}
	if observation.FinishedAt != nil {
		t.Fatalf("finishedAt = %v, want nil", observation.FinishedAt)
	}
	if observation.Outcome != model.ObservationOutcomeSucceeded {
		t.Fatalf("outcome = %q, want succeeded", observation.Outcome)
	}
}

func TestObservationFromExchangeSourcePreference(t *testing.T) {
	cases := []struct {
		name    string
		hint    string
		profile string
		want    string
	}{
		{"source hint preferred", "claude", "default", "claude"},
		{"profile fallback", "", "default", "default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := observationExchange()
			e.SourceHint = tc.hint
			e.Profile = tc.profile
			if got := projectExchange(t, e).Source; got != tc.want {
				t.Fatalf("source = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestObservationFromExchangeModelPreference(t *testing.T) {
	e := observationExchange()
	e.Model = "request-model"
	e.Response.Model = "response-model"
	if got := projectExchange(t, e).Model; got != "response-model" {
		t.Fatalf("model = %q, want response-model", got)
	}
	e.Response.Model = ""
	if got := projectExchange(t, e).Model; got != "request-model" {
		t.Fatalf("model fallback = %q, want request-model", got)
	}
}

func TestObservationFromExchangeTimestamps(t *testing.T) {
	e := observationExchange()
	e.StartedAt = time.Time{}
	observation := projectExchange(t, e)
	if observation.StartedAt != nil {
		t.Fatalf("startedAt = %v, want nil", observation.StartedAt)
	}
	if observation.FinishedAt != nil {
		t.Fatalf("finishedAt = %v, want nil", observation.FinishedAt)
	}
}

func TestObservationFromExchangeDeterministicAndPure(t *testing.T) {
	e := observationExchange()
	e.Response.ProviderUsage = anthropicProviderUsage(int64p(100), int64p(40), nil, int64p(25), nil, nil, nil)
	snapshot := e

	first := projectExchange(t, e)
	second := projectExchange(t, e)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("projection is not deterministic:\nfirst  = %+v\nsecond = %+v", first, second)
	}
	if !reflect.DeepEqual(e, snapshot) {
		t.Fatalf("exchange was mutated: %+v", e)
	}
}

func TestObservationFromExchangeIdentityNamespaces(t *testing.T) {
	e := observationExchange()
	e.Response.ProviderRequestID = "req-1"
	e.Response.ResponseObjectID = "resp-1"
	e.Request.Metadata.OpenAIResponses = &OpenAIResponsesMetadata{PreviousResponseID: "prev-1"}

	observation := projectExchange(t, e)
	want := model.ObservationIdentity{
		ExchangeID:             "ex-1",
		ProviderRequestID:      "req-1",
		ResponseObjectID:       "resp-1",
		ParentResponseObjectID: "prev-1",
	}
	if !reflect.DeepEqual(observation.Identity, want) {
		t.Fatalf("identity = %+v, want %+v", observation.Identity, want)
	}
}

func TestObservationFromExchangeIdentityWithoutOpenAIMetadata(t *testing.T) {
	e := observationExchange()
	e.Response.ProviderRequestID = "req-1"
	e.Response.ResponseObjectID = "resp-1"
	observation := projectExchange(t, e)
	if observation.Identity.ParentResponseObjectID != "" {
		t.Fatalf("parentResponseObjectId = %q, want empty", observation.Identity.ParentResponseObjectID)
	}
	if observation.Identity.AgentRequestID != "" || observation.Identity.ChainID != "" {
		t.Fatalf("unexpected synthesized identity: %+v", observation.Identity)
	}
}

func TestAnthropicObservationUsageMapping(t *testing.T) {
	cases := []struct {
		name string
		pu   *ProviderUsage
		want model.ObservationUsage
	}{
		{
			name: "full usage derives totals",
			pu:   anthropicProviderUsage(int64p(100), int64p(40), int64p(10), int64p(25), int64p(5), nil, nil),
			want: model.ObservationUsage{
				FreshInput:         model.NewMeasurement(100, model.MeasurementMeasured),
				CachedInput:        model.NewMeasurement(40, model.MeasurementMeasured),
				CacheCreationInput: model.NewMeasurement(10, model.MeasurementMeasured),
				TotalInput:         model.NewMeasurement(150, model.MeasurementDerived),
				Output:             model.NewMeasurement(25, model.MeasurementMeasured),
				ReasoningOutput:    model.NewMeasurement(5, model.MeasurementMeasured),
				Total:              model.NewMeasurement(175, model.MeasurementDerived),
			},
		},
		{
			name: "missing cache fields stay missing",
			pu:   anthropicProviderUsage(int64p(100), nil, nil, int64p(25), nil, nil, nil),
			want: model.ObservationUsage{
				FreshInput: model.NewMeasurement(100, model.MeasurementMeasured),
				TotalInput: model.NewMeasurement(100, model.MeasurementDerived),
				Output:     model.NewMeasurement(25, model.MeasurementMeasured),
				Total:      model.NewMeasurement(125, model.MeasurementDerived),
			},
		},
		{
			name: "explicit zero kept measured",
			pu:   anthropicProviderUsage(int64p(0), int64p(0), int64p(0), int64p(0), int64p(0), nil, nil),
			want: model.ObservationUsage{
				FreshInput:         model.NewMeasurement(0, model.MeasurementMeasured),
				CachedInput:        model.NewMeasurement(0, model.MeasurementMeasured),
				CacheCreationInput: model.NewMeasurement(0, model.MeasurementMeasured),
				TotalInput:         model.NewMeasurement(0, model.MeasurementDerived),
				Output:             model.NewMeasurement(0, model.MeasurementMeasured),
				ReasoningOutput:    model.NewMeasurement(0, model.MeasurementMeasured),
				Total:              model.NewMeasurement(0, model.MeasurementDerived),
			},
		},
		{
			name: "direct total input wins over derived",
			pu:   anthropicProviderUsage(int64p(100), int64p(40), int64p(10), nil, nil, int64p(200), nil),
			want: model.ObservationUsage{
				FreshInput:         model.NewMeasurement(100, model.MeasurementMeasured),
				CachedInput:        model.NewMeasurement(40, model.MeasurementMeasured),
				CacheCreationInput: model.NewMeasurement(10, model.MeasurementMeasured),
				TotalInput:         model.NewMeasurement(200, model.MeasurementMeasured),
			},
		},
		{
			name: "direct total wins over derived",
			pu:   anthropicProviderUsage(int64p(100), int64p(40), int64p(10), int64p(25), nil, nil, int64p(500)),
			want: model.ObservationUsage{
				FreshInput:         model.NewMeasurement(100, model.MeasurementMeasured),
				CachedInput:        model.NewMeasurement(40, model.MeasurementMeasured),
				CacheCreationInput: model.NewMeasurement(10, model.MeasurementMeasured),
				TotalInput:         model.NewMeasurement(150, model.MeasurementDerived),
				Output:             model.NewMeasurement(25, model.MeasurementMeasured),
				Total:              model.NewMeasurement(500, model.MeasurementMeasured),
			},
		},
		{
			name: "missing output does not invent total",
			pu:   anthropicProviderUsage(int64p(100), int64p(40), nil, nil, nil, nil, nil),
			want: model.ObservationUsage{
				FreshInput:  model.NewMeasurement(100, model.MeasurementMeasured),
				CachedInput: model.NewMeasurement(40, model.MeasurementMeasured),
				TotalInput:  model.NewMeasurement(140, model.MeasurementDerived),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := observationExchange()
			e.Response.ProviderUsage = tc.pu
			assertObservationUsage(t, projectExchange(t, e).Usage, tc.want)
		})
	}
}

func TestOpenAIResponsesObservationUsageMapping(t *testing.T) {
	cases := []struct {
		name string
		pu   *ProviderUsage
		want model.ObservationUsage
	}{
		{
			name: "full usage derives totals",
			pu:   openAIProviderUsage(nil, int64p(40), int64p(25), int64p(5), int64p(150), nil),
			want: model.ObservationUsage{
				FreshInput:      model.NewMeasurement(110, model.MeasurementDerived),
				CachedInput:     model.NewMeasurement(40, model.MeasurementMeasured),
				TotalInput:      model.NewMeasurement(150, model.MeasurementMeasured),
				Output:          model.NewMeasurement(25, model.MeasurementMeasured),
				ReasoningOutput: model.NewMeasurement(5, model.MeasurementMeasured),
				Total:           model.NewMeasurement(175, model.MeasurementDerived),
			},
		},
		{
			name: "total input tokens wins input tokens",
			pu:   openAIProviderUsage(int64p(100), int64p(40), int64p(25), nil, int64p(150), nil),
			want: model.ObservationUsage{
				FreshInput:  model.NewMeasurement(110, model.MeasurementDerived),
				CachedInput: model.NewMeasurement(40, model.MeasurementMeasured),
				TotalInput:  model.NewMeasurement(150, model.MeasurementMeasured),
				Output:      model.NewMeasurement(25, model.MeasurementMeasured),
				Total:       model.NewMeasurement(175, model.MeasurementDerived),
			},
		},
		{
			name: "input tokens only",
			pu:   openAIProviderUsage(int64p(100), int64p(40), int64p(25), nil, nil, nil),
			want: model.ObservationUsage{
				FreshInput:  model.NewMeasurement(60, model.MeasurementDerived),
				CachedInput: model.NewMeasurement(40, model.MeasurementMeasured),
				TotalInput:  model.NewMeasurement(100, model.MeasurementMeasured),
				Output:      model.NewMeasurement(25, model.MeasurementMeasured),
				Total:       model.NewMeasurement(125, model.MeasurementDerived),
			},
		},
		{
			name: "explicit zero kept",
			pu:   openAIProviderUsage(nil, int64p(0), int64p(0), int64p(0), int64p(0), nil),
			want: model.ObservationUsage{
				FreshInput:      model.NewMeasurement(0, model.MeasurementDerived),
				CachedInput:     model.NewMeasurement(0, model.MeasurementMeasured),
				TotalInput:      model.NewMeasurement(0, model.MeasurementMeasured),
				Output:          model.NewMeasurement(0, model.MeasurementMeasured),
				ReasoningOutput: model.NewMeasurement(0, model.MeasurementMeasured),
				Total:           model.NewMeasurement(0, model.MeasurementDerived),
			},
		},
		{
			name: "cached missing keeps fresh missing",
			pu:   openAIProviderUsage(nil, nil, int64p(25), nil, int64p(150), nil),
			want: model.ObservationUsage{
				TotalInput: model.NewMeasurement(150, model.MeasurementMeasured),
				Output:     model.NewMeasurement(25, model.MeasurementMeasured),
				Total:      model.NewMeasurement(175, model.MeasurementDerived),
			},
		},
		{
			name: "cached present zero derives fresh",
			pu:   openAIProviderUsage(nil, int64p(0), int64p(25), nil, int64p(150), nil),
			want: model.ObservationUsage{
				FreshInput:  model.NewMeasurement(150, model.MeasurementDerived),
				CachedInput: model.NewMeasurement(0, model.MeasurementMeasured),
				TotalInput:  model.NewMeasurement(150, model.MeasurementMeasured),
				Output:      model.NewMeasurement(25, model.MeasurementMeasured),
				Total:       model.NewMeasurement(175, model.MeasurementDerived),
			},
		},
		{
			name: "cached greater than total clamps fresh",
			pu:   openAIProviderUsage(nil, int64p(150), int64p(25), nil, int64p(100), nil),
			want: model.ObservationUsage{
				FreshInput:  model.NewMeasurement(0, model.MeasurementDerived),
				CachedInput: model.NewMeasurement(150, model.MeasurementMeasured),
				TotalInput:  model.NewMeasurement(100, model.MeasurementMeasured),
				Output:      model.NewMeasurement(25, model.MeasurementMeasured),
				Total:       model.NewMeasurement(125, model.MeasurementDerived),
			},
		},
		{
			name: "reasoning output mapped",
			pu:   openAIProviderUsage(nil, nil, int64p(25), int64p(7), int64p(100), nil),
			want: model.ObservationUsage{
				ReasoningOutput: model.NewMeasurement(7, model.MeasurementMeasured),
				TotalInput:      model.NewMeasurement(100, model.MeasurementMeasured),
				Output:          model.NewMeasurement(25, model.MeasurementMeasured),
				Total:           model.NewMeasurement(125, model.MeasurementDerived),
			},
		},
		{
			name: "direct total wins over derived",
			pu:   openAIProviderUsage(nil, int64p(40), int64p(25), nil, int64p(100), int64p(999)),
			want: model.ObservationUsage{
				FreshInput:  model.NewMeasurement(60, model.MeasurementDerived),
				CachedInput: model.NewMeasurement(40, model.MeasurementMeasured),
				TotalInput:  model.NewMeasurement(100, model.MeasurementMeasured),
				Output:      model.NewMeasurement(25, model.MeasurementMeasured),
				Total:       model.NewMeasurement(999, model.MeasurementMeasured),
			},
		},
		{
			name: "missing output does not invent total",
			pu:   openAIProviderUsage(nil, int64p(40), nil, nil, int64p(100), nil),
			want: model.ObservationUsage{
				FreshInput:  model.NewMeasurement(60, model.MeasurementDerived),
				CachedInput: model.NewMeasurement(40, model.MeasurementMeasured),
				TotalInput:  model.NewMeasurement(100, model.MeasurementMeasured),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := observationExchange()
			e.Response.ProviderUsage = tc.pu
			assertObservationUsage(t, projectExchange(t, e).Usage, tc.want)
		})
	}
}

func TestAnthropicObservationUsageEvidence(t *testing.T) {
	e := observationExchange()
	e.Response.ProviderUsage = anthropicProviderUsage(int64p(100), int64p(40), int64p(10), int64p(25), int64p(5), nil, nil)
	usage := projectExchange(t, e).Usage

	assertSourceValueEvidence(t, usage.FreshInput, "ex-1", observationFieldInputTokens)
	assertSourceValueEvidence(t, usage.CachedInput, "ex-1", observationFieldCacheReadInputTokens)
	assertSourceValueEvidence(t, usage.CacheCreationInput, "ex-1", observationFieldCacheCreationInputTokens)
	assertSourceValueEvidence(t, usage.Output, "ex-1", observationFieldOutputTokens)
	assertSourceValueEvidence(t, usage.ReasoningOutput, "ex-1", observationFieldReasoningOutputTokens)
	assertAggregateEvidence(t, usage.TotalInput, "ex-1", 3)
	assertAggregateEvidence(t, usage.Total, "ex-1", 2)
}

func TestOpenAIResponsesObservationUsageEvidence(t *testing.T) {
	e := observationExchange()
	e.Response.ProviderUsage = openAIProviderUsage(nil, int64p(40), int64p(25), int64p(5), int64p(150), nil)
	usage := projectExchange(t, e).Usage

	assertSourceValueEvidence(t, usage.TotalInput, "ex-1", observationFieldTotalInputTokens)
	assertSourceValueEvidence(t, usage.CachedInput, "ex-1", observationFieldCacheReadInputTokens)
	assertSourceValueEvidence(t, usage.Output, "ex-1", observationFieldOutputTokens)
	assertSourceValueEvidence(t, usage.ReasoningOutput, "ex-1", observationFieldReasoningOutputTokens)
	assertProvenanceEvidence(t, usage.FreshInput, "ex-1", observationFieldDerivedFreshInput)
	assertAggregateEvidence(t, usage.Total, "ex-1", 2)
}

func TestUnknownSourceObservationUsage(t *testing.T) {
	e := observationExchange()
	e.Response.ProviderUsage = &ProviderUsage{
		InputTokens:              int64p(70),
		CacheReadInputTokens:     int64p(10),
		CacheCreationInputTokens: int64p(3),
		OutputTokens:             int64p(25),
		ReasoningOutputTokens:    int64p(5),
		TotalInputTokens:         int64p(100),
		TotalTokens:              int64p(130),
		Source:                   "acme_gateway",
	}
	observation := projectExchange(t, e)

	assertObservationUsage(t, observation.Usage, model.ObservationUsage{
		TotalInput:      model.NewMeasurement(100, model.MeasurementDerived),
		Output:          model.NewMeasurement(25, model.MeasurementDerived),
		ReasoningOutput: model.NewMeasurement(5, model.MeasurementDerived),
		Total:           model.NewMeasurement(130, model.MeasurementDerived),
	})
	if observation.Usage.FreshInput.Value != nil ||
		observation.Usage.CachedInput.Value != nil ||
		observation.Usage.CacheCreationInput.Value != nil {
		t.Fatalf("unknown source invented input fields: %+v", observation.Usage)
	}
	if observation.Completeness != model.ObservationCompletenessPartial {
		t.Fatalf("completeness = %q, want partial", observation.Completeness)
	}
	for name, m := range map[string]model.Measurement{
		"totalInput":      observation.Usage.TotalInput,
		"output":          observation.Usage.Output,
		"reasoningOutput": observation.Usage.ReasoningOutput,
		"total":           observation.Usage.Total,
	} {
		if m.Kind == model.MeasurementMeasured {
			t.Fatalf("%s promoted to measured for unknown source", name)
		}
	}
}

func TestObservationOutcomeAndCompleteness(t *testing.T) {
	completeAnthropic := anthropicProviderUsage(int64p(100), nil, nil, int64p(25), nil, nil, nil)
	incompleteAnthropic := anthropicProviderUsage(int64p(100), nil, nil, nil, nil, nil, nil)
	completeOpenAI := openAIProviderUsage(int64p(100), nil, int64p(25), nil, nil, nil)
	incompleteOpenAI := openAIProviderUsage(int64p(100), nil, nil, nil, nil, nil)

	cases := []struct {
		name             string
		outcome          RequestOutcome
		providerUsage    *ProviderUsage
		wantOutcome      model.ObservationOutcome
		wantCompleteness model.ObservationCompleteness
	}{
		{"successful complete anthropic", OutcomeUpstreamOK, completeAnthropic, model.ObservationOutcomeSucceeded, model.ObservationCompletenessComplete},
		{"successful incomplete anthropic", OutcomeUpstreamOK, incompleteAnthropic, model.ObservationOutcomeSucceeded, model.ObservationCompletenessPartial},
		{"successful complete openai", OutcomeUpstreamOK, completeOpenAI, model.ObservationOutcomeSucceeded, model.ObservationCompletenessComplete},
		{"successful incomplete openai", OutcomeUpstreamOK, incompleteOpenAI, model.ObservationOutcomeSucceeded, model.ObservationCompletenessPartial},
		{"successful without provider usage", OutcomeUpstreamOK, nil, model.ObservationOutcomeSucceeded, model.ObservationCompletenessPartial},
		{"successful unknown source", OutcomeUpstreamOK, &ProviderUsage{OutputTokens: int64p(25), Source: "acme"}, model.ObservationOutcomeSucceeded, model.ObservationCompletenessPartial},
		{"provider http error", OutcomeUpstreamHTTPError, nil, model.ObservationOutcomeProviderError, model.ObservationCompletenessComplete},
		{"transport failure", OutcomeTransportFailure, nil, model.ObservationOutcomeTransportFailure, model.ObservationCompletenessComplete},
		{"client canceled", OutcomeClientCanceled, nil, model.ObservationOutcomeCanceled, model.ObservationCompletenessComplete},
		{"stream truncated with usage", OutcomeStreamTruncated, completeAnthropic, model.ObservationOutcomeTruncated, model.ObservationCompletenessPartial},
		{"unknown outcome", OutcomeUnknown, completeAnthropic, model.ObservationOutcomeUnknown, model.ObservationCompletenessUnknown},
		{"unrecognized outcome", RequestOutcome("surprise"), completeAnthropic, model.ObservationOutcomeUnknown, model.ObservationCompletenessUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := observationExchange()
			e.Outcome = tc.outcome
			e.Response.ProviderUsage = tc.providerUsage
			observation := projectExchange(t, e)
			if observation.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q, want %q", observation.Outcome, tc.wantOutcome)
			}
			if observation.Completeness != tc.wantCompleteness {
				t.Fatalf("completeness = %q, want %q", observation.Completeness, tc.wantCompleteness)
			}
		})
	}
}

func TestObservationFromExchangeRejectsUnsupportedKind(t *testing.T) {
	for _, kind := range []RequestKind{RequestKindNonModel, RequestKindUnknown, RequestKindUnclassified, RequestKind("other")} {
		e := observationExchange()
		e.Kind = kind
		_, err := ObservationFromExchange(e)
		if err == nil {
			t.Fatalf("kind %q: expected error", kind)
		}
		if !strings.Contains(err.Error(), "unsupported request kind") {
			t.Fatalf("kind %q: error = %q", kind, err)
		}
	}
}

func TestObservationFromExchangeRejectsEmptyID(t *testing.T) {
	e := observationExchange()
	e.ID = ""
	_, err := ObservationFromExchange(e)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Fatalf("error = %q, want id requirement", err)
	}
}

func TestObservationFromExchangeRejectsEmptyProfileAndSource(t *testing.T) {
	e := observationExchange()
	e.SourceHint = ""
	e.Profile = ""
	_, err := ObservationFromExchange(e)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "profile is required") {
		t.Fatalf("error = %q, want profile requirement", err)
	}
}

func TestProjectedObservationValidationWrapsExchangeContext(t *testing.T) {
	err := validateProjectedObservation(Exchange{ID: "ex-9"}, model.Observation{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "project gateway exchange ex-9") {
		t.Fatalf("error = %q, want wrapped exchange context", err)
	}
	if !strings.Contains(err.Error(), "validate observation") {
		t.Fatalf("error = %q, want validation cause", err)
	}
}

func TestObservationFromExchangePrivacy(t *testing.T) {
	e := observationExchange()
	e.Upstream = "https://secret-upstream.example/v1"
	e.Request.Endpoint = "/v1/messages"
	e.Request.BodyHash = "sha256-secret-body-hash"
	e.Request.Components = []model.ContextComponent{{
		Kind:        model.ContextUserPrompt,
		Path:        "/private/secret/project",
		ContentHash: "content-secret",
		Measurement: model.NewMeasurement(12, model.MeasurementMeasured),
	}}
	e.Response.ProviderUsage = anthropicProviderUsage(int64p(100), nil, nil, int64p(25), nil, nil, nil)

	observation := projectExchange(t, e)
	data, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{
		"prompt", "payload", "rawInput", "rawResponse", "apiKey", "credential",
		"components", "bodyHash", "endpoint", "upstream", "secret-upstream",
		"sha256-secret-body-hash", "content-secret", "/private/secret/project",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("observation JSON contains forbidden data %q: %s", forbidden, text)
		}
	}
}

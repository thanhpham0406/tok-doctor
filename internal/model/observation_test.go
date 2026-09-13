package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func validObservation() Observation {
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	finished := started.Add(1500 * time.Millisecond)
	return Observation{
		SchemaVersion: ObservationSchemaVersion,
		ID:            "obs-1",
		Channel:       ObservationChannelGateway,
		Scope:         ObservationScopeRequest,
		Source:        "codex",
		Identity: ObservationIdentity{
			ExchangeID:        "ex-1",
			ProviderRequestID: "req-1",
		},
		StartedAt:  &started,
		FinishedAt: &finished,
		Model:      "test-model",
		Usage: ObservationUsage{
			FreshInput:         NewMeasurement(100, MeasurementMeasured),
			CachedInput:        NewMeasurement(40, MeasurementMeasured),
			CacheCreationInput: NewMeasurement(10, MeasurementMeasured),
			TotalInput:         NewMeasurement(150, MeasurementDerived),
			Output:             NewMeasurement(25, MeasurementMeasured),
			ReasoningOutput:    NewMeasurement(5, MeasurementEstimated),
			Total:              NewMeasurement(175, MeasurementDerived),
		},
		Completeness: ObservationCompletenessComplete,
		Outcome:      ObservationOutcomeSucceeded,
		Evidence: []Evidence{
			{Kind: EvidenceSourceValue, Source: "gateway", Record: "ex-1"},
		},
	}
}

type observationUsageField struct {
	name string
	set  func(*ObservationUsage, Measurement)
}

func observationUsageFields() []observationUsageField {
	return []observationUsageField{
		{"usage.freshInput", func(u *ObservationUsage, m Measurement) { u.FreshInput = m }},
		{"usage.cachedInput", func(u *ObservationUsage, m Measurement) { u.CachedInput = m }},
		{"usage.cacheCreationInput", func(u *ObservationUsage, m Measurement) { u.CacheCreationInput = m }},
		{"usage.totalInput", func(u *ObservationUsage, m Measurement) { u.TotalInput = m }},
		{"usage.output", func(u *ObservationUsage, m Measurement) { u.Output = m }},
		{"usage.reasoningOutput", func(u *ObservationUsage, m Measurement) { u.ReasoningOutput = m }},
		{"usage.total", func(u *ObservationUsage, m Measurement) { u.Total = m }},
	}
}

func TestValidObservationChannel(t *testing.T) {
	for _, channel := range []ObservationChannel{
		ObservationChannelGateway,
		ObservationChannelAgentTelemetry,
		ObservationChannelSessionTranscript,
	} {
		if !ValidObservationChannel(channel) {
			t.Fatalf("channel %q should be valid", channel)
		}
	}
	for _, channel := range []ObservationChannel{"", "client", "telemetry"} {
		if ValidObservationChannel(channel) {
			t.Fatalf("channel %q should be rejected", channel)
		}
	}
}

func TestValidObservationScope(t *testing.T) {
	for _, scope := range []ObservationScope{
		ObservationScopeRequest,
		ObservationScopeTurn,
		ObservationScopeSession,
	} {
		if !ValidObservationScope(scope) {
			t.Fatalf("scope %q should be valid", scope)
		}
	}
	for _, scope := range []ObservationScope{"", "exchange", "run"} {
		if ValidObservationScope(scope) {
			t.Fatalf("scope %q should be rejected", scope)
		}
	}
}

func TestValidObservationCompleteness(t *testing.T) {
	for _, value := range []ObservationCompleteness{
		ObservationCompletenessComplete,
		ObservationCompletenessPartial,
		ObservationCompletenessUnknown,
	} {
		if !ValidObservationCompleteness(value) {
			t.Fatalf("completeness %q should be valid", value)
		}
	}
	for _, value := range []ObservationCompleteness{"", "done"} {
		if ValidObservationCompleteness(value) {
			t.Fatalf("completeness %q should be rejected", value)
		}
	}
}

func TestValidObservationOutcome(t *testing.T) {
	valid := []ObservationOutcome{
		ObservationOutcomeUnknown,
		ObservationOutcomeSucceeded,
		ObservationOutcomeProviderError,
		ObservationOutcomeTransportFailure,
		ObservationOutcomeCanceled,
		ObservationOutcomeTruncated,
	}
	for _, outcome := range valid {
		if !ValidObservationOutcome(outcome) {
			t.Fatalf("outcome %q should be valid", outcome)
		}
	}
	for _, outcome := range []ObservationOutcome{"", "ok", "failed", "succeeded "} {
		if ValidObservationOutcome(outcome) {
			t.Fatalf("outcome %q should be rejected", outcome)
		}
	}
}

func TestObservationValidateAcceptsValid(t *testing.T) {
	if err := validObservation().Validate(); err != nil {
		t.Fatalf("valid observation rejected: %v", err)
	}
}

func TestObservationValidateEmptyIdentityIsValid(t *testing.T) {
	obs := validObservation()
	obs.Identity = ObservationIdentity{}
	if err := obs.Validate(); err != nil {
		t.Fatalf("empty identity should be valid: %v", err)
	}
}

func TestObservationValidateChannelScopeIndependent(t *testing.T) {
	channels := []ObservationChannel{
		ObservationChannelGateway,
		ObservationChannelAgentTelemetry,
		ObservationChannelSessionTranscript,
	}
	scopes := []ObservationScope{
		ObservationScopeRequest,
		ObservationScopeTurn,
		ObservationScopeSession,
	}
	for _, channel := range channels {
		for _, scope := range scopes {
			obs := validObservation()
			obs.Channel = channel
			obs.Scope = scope
			if err := obs.Validate(); err != nil {
				t.Fatalf("channel %q scope %q: %v", channel, scope, err)
			}
		}
	}
}

func TestObservationValidateStructuralRules(t *testing.T) {
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	earlier := started.Add(-time.Second)
	cases := []struct {
		name   string
		mutate func(*Observation)
		want   string
	}{
		{"schema version", func(o *Observation) { o.SchemaVersion = 2 }, "schema version"},
		{"missing id", func(o *Observation) { o.ID = "" }, "id is required"},
		{"invalid channel", func(o *Observation) { o.Channel = "client" }, "invalid channel"},
		{"invalid scope", func(o *Observation) { o.Scope = "run" }, "invalid scope"},
		{"missing source", func(o *Observation) { o.Source = "" }, "source is required"},
		{"invalid completeness", func(o *Observation) { o.Completeness = "done" }, "invalid completeness"},
		{"missing outcome", func(o *Observation) { o.Outcome = "" }, "invalid outcome"},
		{"invalid outcome", func(o *Observation) { o.Outcome = "failed" }, "invalid outcome"},
		{"finished before started", func(o *Observation) {
			o.StartedAt = &started
			o.FinishedAt = &earlier
		}, "finishedAt precedes startedAt"},
		{"invalid observation evidence", func(o *Observation) {
			o.Evidence = []Evidence{{Kind: "surprise"}}
		}, "invalid evidence kind"},
	}
	for _, tc := range cases {
		obs := validObservation()
		tc.mutate(&obs)
		err := obs.Validate()
		if err == nil {
			t.Fatalf("case %s: expected error", tc.name)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("case %s: error = %q, want substring %q", tc.name, err, tc.want)
		}
	}
}

func TestObservationValidateAllowsMissingTimes(t *testing.T) {
	obs := validObservation()
	obs.StartedAt = nil
	obs.FinishedAt = nil
	if err := obs.Validate(); err != nil {
		t.Fatalf("missing timestamps should be valid: %v", err)
	}
}

func TestObservationValidateUsageFieldContract(t *testing.T) {
	valid := []struct {
		name        string
		measurement Measurement
	}{
		{"missing", Measurement{}},
		{"missing unknown", Measurement{Kind: MeasurementUnknown}},
		{"explicit zero measured", NewMeasurement(0, MeasurementMeasured)},
		{"measured", NewMeasurement(12, MeasurementMeasured)},
		{"derived", NewMeasurement(12, MeasurementDerived)},
		{"counted", NewMeasurement(12, MeasurementCounted)},
		{"estimated", NewMeasurement(12, MeasurementEstimated)},
	}
	invalid := []struct {
		name        string
		measurement Measurement
		want        string
	}{
		{"value without kind", Measurement{Value: Int64(1)}, "value requires a kind"},
		{"value with unknown kind", Measurement{Value: Int64(1), Kind: MeasurementUnknown}, "usable kind"},
		{"measured without value", Measurement{Kind: MeasurementMeasured}, "requires a value"},
		{"unrecognized kind without value", Measurement{Kind: "surprise"}, "unrecognized measurement kind"},
		{"unrecognized kind with value", Measurement{Value: Int64(1), Kind: "surprise"}, "unrecognized measurement kind"},
		{"invalid measurement evidence", MeasurementWithEvidence(1, MeasurementMeasured, Evidence{Kind: "surprise"}), "invalid evidence kind"},
	}
	for _, field := range observationUsageFields() {
		for _, tc := range valid {
			obs := validObservation()
			field.set(&obs.Usage, tc.measurement)
			if err := obs.Validate(); err != nil {
				t.Fatalf("field %s case %s: unexpected error: %v", field.name, tc.name, err)
			}
		}
		for _, tc := range invalid {
			obs := validObservation()
			field.set(&obs.Usage, tc.measurement)
			err := obs.Validate()
			if err == nil {
				t.Fatalf("field %s case %s: expected error", field.name, tc.name)
			}
			if !strings.Contains(err.Error(), field.name) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("field %s case %s: error = %q, want field and %q", field.name, tc.name, err, tc.want)
			}
		}
	}
}

func TestObservationJSONRoundTrip(t *testing.T) {
	original := validObservation()
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Observation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.SchemaVersion != original.SchemaVersion ||
		decoded.ID != original.ID ||
		decoded.Channel != original.Channel ||
		decoded.Scope != original.Scope ||
		decoded.Source != original.Source ||
		decoded.Model != original.Model ||
		decoded.Outcome != original.Outcome ||
		decoded.Completeness != original.Completeness {
		t.Fatalf("scalar fields changed: %+v", decoded)
	}
	if !reflect.DeepEqual(decoded.Identity, original.Identity) {
		t.Fatalf("identity = %+v, want %+v", decoded.Identity, original.Identity)
	}
	if !reflect.DeepEqual(decoded.Usage, original.Usage) {
		t.Fatalf("usage = %+v, want %+v", decoded.Usage, original.Usage)
	}
	if !reflect.DeepEqual(decoded.Evidence, original.Evidence) {
		t.Fatalf("evidence = %+v, want %+v", decoded.Evidence, original.Evidence)
	}
	if decoded.StartedAt == nil || !decoded.StartedAt.Equal(*original.StartedAt) {
		t.Fatalf("startedAt = %v, want %v", decoded.StartedAt, original.StartedAt)
	}
	if decoded.FinishedAt == nil || !decoded.FinishedAt.Equal(*original.FinishedAt) {
		t.Fatalf("finishedAt = %v, want %v", decoded.FinishedAt, original.FinishedAt)
	}
}

func TestObservationJSONDistinguishesExplicitZeroFromMissing(t *testing.T) {
	obs := validObservation()
	obs.Usage.TotalInput = NewMeasurement(0, MeasurementMeasured)
	obs.Usage.CachedInput = Measurement{}
	obs.Usage.ReasoningOutput = Measurement{Kind: MeasurementUnknown}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Observation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Usage.TotalInput.Value == nil || *decoded.Usage.TotalInput.Value != 0 {
		t.Fatalf("explicit zero value = %v, want pointer to 0", decoded.Usage.TotalInput.Value)
	}
	if decoded.Usage.TotalInput.Kind != MeasurementMeasured {
		t.Fatalf("explicit zero kind = %q, want measured", decoded.Usage.TotalInput.Kind)
	}
	if decoded.Usage.CachedInput.Value != nil {
		t.Fatalf("missing value = %v, want nil", decoded.Usage.CachedInput.Value)
	}
	if decoded.Usage.CachedInput.Kind != "" {
		t.Fatalf("missing kind = %q, want empty", decoded.Usage.CachedInput.Kind)
	}
	if decoded.Usage.ReasoningOutput.Value != nil || decoded.Usage.ReasoningOutput.Kind != MeasurementUnknown {
		t.Fatalf("unknown measurement = %+v, want nil value and unknown kind", decoded.Usage.ReasoningOutput)
	}
}

func TestObservationJSONKeepsPerFieldKinds(t *testing.T) {
	obs := validObservation()
	obs.Usage = ObservationUsage{
		FreshInput:         NewMeasurement(11, MeasurementMeasured),
		CachedInput:        NewMeasurement(22, MeasurementDerived),
		CacheCreationInput: NewMeasurement(33, MeasurementCounted),
		TotalInput:         NewMeasurement(44, MeasurementEstimated),
		Output:             NewMeasurement(55, MeasurementMeasured),
		ReasoningOutput:    NewMeasurement(66, MeasurementCounted),
		Total:              NewMeasurement(77, MeasurementDerived),
	}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Observation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(decoded.Usage, obs.Usage) {
		t.Fatalf("usage = %+v, want %+v", decoded.Usage, obs.Usage)
	}
}

func TestObservationJSONIdentityNamespacesRoundTripIndependently(t *testing.T) {
	obs := validObservation()
	obs.Identity = ObservationIdentity{
		ExchangeID:             "exchange",
		AgentRequestID:         "agent",
		ProviderRequestID:      "provider",
		ResponseObjectID:       "response",
		ParentResponseObjectID: "parent",
		SessionID:              "session",
		TurnID:                 "turn",
		InvocationID:           "invocation",
		ChainID:                "chain",
	}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Observation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Identity != obs.Identity {
		t.Fatalf("identity = %+v, want %+v", decoded.Identity, obs.Identity)
	}
}

func TestObservationJSONEvidenceRoundTrip(t *testing.T) {
	obs := validObservation()
	obs.Evidence = []Evidence{
		{Kind: EvidenceSourceValue, Source: "gateway", Record: "r1", Field: "usage.output"},
		{Kind: EvidenceCumulativeDelta, Source: "gateway", Previous: "p1", Current: "c1"},
		{Kind: EvidenceAggregate, Source: "gateway", Operation: AggregateSum, Count: 3},
		{Kind: EvidenceProvenance, Source: "gateway"},
	}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Observation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(decoded.Evidence, obs.Evidence) {
		t.Fatalf("evidence = %+v, want %+v", decoded.Evidence, obs.Evidence)
	}
}

func TestObservationJSONEmitsSchemaVersion(t *testing.T) {
	data, err := json.Marshal(validObservation())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	value, ok := raw["schemaVersion"]
	if !ok {
		t.Fatal("schemaVersion key missing from JSON")
	}
	if string(value) != "1" {
		t.Fatalf("schemaVersion = %s, want 1", value)
	}
}

func TestObservationJSONHasOnlyContractFields(t *testing.T) {
	obs := validObservation()
	obs.Usage.CachedInput = Measurement{Kind: MeasurementUnknown}
	obs.Evidence = nil
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	allowedTop := map[string]bool{
		"schemaVersion": true, "id": true, "channel": true, "scope": true,
		"source": true, "identity": true, "startedAt": true, "finishedAt": true,
		"model": true, "usage": true, "outcome": true, "completeness": true, "evidence": true,
	}
	for key := range raw {
		if !allowedTop[key] {
			t.Fatalf("unexpected top-level field %q", key)
		}
	}
	var usage map[string]json.RawMessage
	if err := json.Unmarshal(raw["usage"], &usage); err != nil {
		t.Fatalf("unmarshal usage: %v", err)
	}
	allowedUsage := map[string]bool{
		"freshInput": true, "cachedInput": true, "cacheCreationInput": true,
		"totalInput": true, "output": true, "reasoningOutput": true, "total": true,
	}
	for key := range usage {
		if !allowedUsage[key] {
			t.Fatalf("unexpected usage field %q", key)
		}
	}
	var identity map[string]json.RawMessage
	if err := json.Unmarshal(raw["identity"], &identity); err != nil {
		t.Fatalf("unmarshal identity: %v", err)
	}
	allowedIdentity := map[string]bool{
		"exchangeId": true, "agentRequestId": true, "providerRequestId": true,
		"responseObjectId": true, "parentResponseObjectId": true, "sessionId": true,
		"turnId": true, "invocationId": true, "chainId": true,
	}
	for key := range identity {
		if !allowedIdentity[key] {
			t.Fatalf("unexpected identity field %q", key)
		}
	}
	text := string(data)
	for _, forbidden := range []string{
		`"prompt"`, `"payload"`, `"rawInput"`, `"rawResponse"`, `"apiKey"`,
		`"credential"`, `"anthropicMessages"`, `"openaiResponses"`, `"previousResponseId"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("JSON contains forbidden field %s", forbidden)
		}
	}
}

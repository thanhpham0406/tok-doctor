package codex

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func obsMeasured(value int64, evidence ...model.Evidence) model.Measurement {
	measurement := model.NewMeasurement(value, model.MeasurementMeasured)
	measurement.Evidence = append(measurement.Evidence, evidence...)
	return measurement
}

func obsDerived(value int64, evidence ...model.Evidence) model.Measurement {
	measurement := model.NewMeasurement(value, model.MeasurementDerived)
	measurement.Evidence = append(measurement.Evidence, evidence...)
	return measurement
}

func obsSourceValue(record, field string) model.Evidence {
	return model.Evidence{Kind: model.EvidenceSourceValue, Source: "codex_rollout", Record: record, Field: field}
}

func obsCumulativeDelta(previous, current, field string) model.Evidence {
	return model.Evidence{Kind: model.EvidenceCumulativeDelta, Source: "codex_rollout", Previous: previous, Current: current, Field: field}
}

func obsSession() model.Session {
	started := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	return model.Session{
		ID:        "sess-1",
		Source:    "codex",
		Agent:     model.AgentCodex,
		StartedAt: &started,
		Model:     "gpt-5-codex",
		Usage: model.Usage{
			Input:     obsMeasured(1000, obsSourceValue("codex_rollout#snap:3", "total_token_usage.input_tokens")),
			Cached:    obsMeasured(400, obsSourceValue("codex_rollout#snap:3", "total_token_usage.cached_input_tokens")),
			Output:    obsMeasured(200, obsSourceValue("codex_rollout#snap:3", "total_token_usage.output_tokens")),
			Reasoning: obsMeasured(50, obsSourceValue("codex_rollout#snap:3", "total_token_usage.reasoning_output_tokens")),
			Total:     obsMeasured(1200, obsSourceValue("codex_rollout#snap:3", "total_token_usage.total_tokens")),
		},
		Turns: []model.Turn{
			{
				ID:       "sess-1#1",
				Sequence: 1,
				Model:    "gpt-5-codex-turn",
				Usage: model.Usage{
					Input:     obsDerived(700, obsCumulativeDelta("", "codex_rollout#snap:1", "input_tokens")),
					Cached:    obsDerived(200, obsCumulativeDelta("", "codex_rollout#snap:1", "cached_input_tokens")),
					Output:    obsDerived(150, obsCumulativeDelta("", "codex_rollout#snap:1", "output_tokens")),
					Reasoning: obsDerived(20, obsCumulativeDelta("", "codex_rollout#snap:1", "reasoning_output_tokens")),
					Total:     obsDerived(850, obsCumulativeDelta("", "codex_rollout#snap:1", "total_tokens")),
				},
			},
			{
				ID:       "sess-1#2",
				Sequence: 2,
				Usage: model.Usage{
					Input:  obsDerived(300, obsCumulativeDelta("codex_rollout#snap:1", "codex_rollout#snap:2", "input_tokens")),
					Cached: obsDerived(200, obsCumulativeDelta("codex_rollout#snap:1", "codex_rollout#snap:2", "cached_input_tokens")),
					Output: obsDerived(50, obsCumulativeDelta("codex_rollout#snap:1", "codex_rollout#snap:2", "output_tokens")),
					Total:  obsDerived(350, obsCumulativeDelta("codex_rollout#snap:1", "codex_rollout#snap:2", "total_tokens")),
				},
			},
		},
	}
}

func obsProject(t *testing.T, session model.Session) []model.Observation {
	t.Helper()
	observations, err := ObservationsFromSession(session)
	if err != nil {
		t.Fatalf("ObservationsFromSession: %v", err)
	}
	for i, observation := range observations {
		if err := observation.Validate(); err != nil {
			t.Fatalf("observation[%d] invalid: %v", i, err)
		}
	}
	return observations
}

func TestObservationsFromSessionSessionMapping(t *testing.T) {
	session := obsSession()
	observation := obsProject(t, session)[0]

	if observation.ID != "codex:session:sess-1" {
		t.Fatalf("id = %q, want codex:session:sess-1", observation.ID)
	}
	if observation.Channel != model.ObservationChannelSessionTranscript {
		t.Fatalf("channel = %q, want session_transcript", observation.Channel)
	}
	if observation.Scope != model.ObservationScopeSession {
		t.Fatalf("scope = %q, want session", observation.Scope)
	}
	if observation.Source != "codex" {
		t.Fatalf("source = %q, want codex", observation.Source)
	}
	if observation.Identity != (model.ObservationIdentity{SessionID: "sess-1"}) {
		t.Fatalf("identity = %+v, want session id only", observation.Identity)
	}
	if observation.Model != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", observation.Model)
	}
	if observation.StartedAt == nil || !observation.StartedAt.Equal(*session.StartedAt) {
		t.Fatalf("startedAt = %v, want %v", observation.StartedAt, session.StartedAt)
	}
	if observation.FinishedAt != nil {
		t.Fatalf("finishedAt = %v, want nil", observation.FinishedAt)
	}
	if observation.Outcome != model.ObservationOutcomeUnknown {
		t.Fatalf("outcome = %q, want unknown", observation.Outcome)
	}
	if observation.Completeness != model.ObservationCompletenessComplete {
		t.Fatalf("completeness = %q, want complete", observation.Completeness)
	}
}

func TestObservationsFromSessionTurnOrder(t *testing.T) {
	observations := obsProject(t, obsSession())
	if len(observations) != 3 {
		t.Fatalf("observations = %d, want 3", len(observations))
	}
	wantIDs := []string{"codex:session:sess-1", "codex:turn:sess-1#1", "codex:turn:sess-1#2"}
	for i, want := range wantIDs {
		if observations[i].ID != want {
			t.Fatalf("observations[%d].id = %q, want %q", i, observations[i].ID, want)
		}
	}
	for _, observation := range observations[1:] {
		if observation.Scope != model.ObservationScopeTurn {
			t.Fatalf("turn scope = %q, want turn", observation.Scope)
		}
	}
}

func TestObservationsFromSessionTurnModelPreference(t *testing.T) {
	observations := obsProject(t, obsSession())
	if got := observations[1].Model; got != "gpt-5-codex-turn" {
		t.Fatalf("turn model = %q, want turn override", got)
	}
	if got := observations[2].Model; got != "gpt-5-codex" {
		t.Fatalf("turn model fallback = %q, want session model", got)
	}
}

func TestObservationsFromSessionIdentityNamespaces(t *testing.T) {
	observations := obsProject(t, obsSession())

	sessionIdentity := observations[0].Identity
	if sessionIdentity.SessionID != "sess-1" || sessionIdentity.TurnID != "" {
		t.Fatalf("session identity = %+v, want session id only", sessionIdentity)
	}
	turnIdentity := observations[1].Identity
	if turnIdentity.SessionID != "sess-1" || turnIdentity.TurnID != "sess-1#1" {
		t.Fatalf("turn identity = %+v, want session and turn ids", turnIdentity)
	}
	for _, identity := range []model.ObservationIdentity{sessionIdentity, turnIdentity} {
		if identity.AgentRequestID != "" || identity.ProviderRequestID != "" || identity.ResponseObjectID != "" ||
			identity.ParentResponseObjectID != "" || identity.InvocationID != "" || identity.ChainID != "" || identity.ExchangeID != "" {
			t.Fatalf("unexpected synthesized identity: %+v", identity)
		}
	}
	if turnIdentity.TurnID == observations[1].ID {
		t.Fatal("observation id was reused as correlation identity")
	}
}

func TestObservationsFromSessionUsageMapping(t *testing.T) {
	usage := model.Usage{
		Input:     obsMeasured(100, obsSourceValue("rec", "input_tokens")),
		Cached:    obsMeasured(30, obsSourceValue("rec", "cached_input_tokens")),
		Output:    obsDerived(40, obsCumulativeDelta("prev", "curr", "output_tokens")),
		Reasoning: model.MeasurementWithEvidence(10, model.MeasurementEstimated, model.Evidence{Kind: model.EvidenceProvenance, Source: "codex_rollout"}),
		Total:     model.MeasurementWithEvidence(150, model.MeasurementCounted, model.Evidence{Kind: model.EvidenceAggregate, Source: "codex_rollout", Operation: model.AggregateSum, Count: 2}),
	}
	observation := obsProject(t, model.Session{ID: "sess-usage", Source: "codex", Agent: model.AgentCodex, Usage: usage})[0]

	cases := []struct {
		field string
		got   model.Measurement
		want  model.Measurement
	}{
		{"cachedInput", observation.Usage.CachedInput, usage.Cached},
		{"totalInput", observation.Usage.TotalInput, usage.Input},
		{"output", observation.Usage.Output, usage.Output},
		{"reasoningOutput", observation.Usage.ReasoningOutput, usage.Reasoning},
		{"total", observation.Usage.Total, usage.Total},
	}
	for _, tc := range cases {
		if !reflect.DeepEqual(tc.got, tc.want) {
			t.Fatalf("%s = %+v, want %+v", tc.field, tc.got, tc.want)
		}
	}
	if observation.Usage.CacheCreationInput.Value != nil || observation.Usage.CacheCreationInput.Kind != "" {
		t.Fatalf("cacheCreationInput = %+v, want missing", observation.Usage.CacheCreationInput)
	}
}

func TestObservationsFromSessionFreshInputDerived(t *testing.T) {
	observation := obsProject(t, obsSession())[0]
	fresh := observation.Usage.FreshInput
	if fresh.Value == nil || *fresh.Value != 600 {
		t.Fatalf("freshInput = %v, want 600", fresh.Value)
	}
	if fresh.Kind != model.MeasurementDerived {
		t.Fatalf("freshInput kind = %q, want derived", fresh.Kind)
	}
	if len(fresh.Evidence) != 1 || fresh.Evidence[0].Kind != model.EvidenceProvenance {
		t.Fatalf("freshInput evidence = %+v, want one provenance entry", fresh.Evidence)
	}
}

func TestObservationsFromSessionExplicitZero(t *testing.T) {
	usage := model.Usage{
		Input:     model.NewMeasurement(0, model.MeasurementMeasured),
		Cached:    model.NewMeasurement(0, model.MeasurementMeasured),
		Output:    model.NewMeasurement(0, model.MeasurementMeasured),
		Reasoning: model.NewMeasurement(0, model.MeasurementMeasured),
		Total:     model.NewMeasurement(0, model.MeasurementMeasured),
	}
	observation := obsProject(t, model.Session{ID: "s", Source: "codex", Usage: usage})[0]

	for name, measurement := range map[string]model.Measurement{
		"totalInput":      observation.Usage.TotalInput,
		"cachedInput":     observation.Usage.CachedInput,
		"output":          observation.Usage.Output,
		"reasoningOutput": observation.Usage.ReasoningOutput,
		"total":           observation.Usage.Total,
	} {
		if measurement.Value == nil || *measurement.Value != 0 {
			t.Fatalf("%s = %v, want explicit zero", name, measurement.Value)
		}
	}
	if observation.Usage.FreshInput.Value == nil || *observation.Usage.FreshInput.Value != 0 {
		t.Fatalf("freshInput = %v, want explicit derived zero", observation.Usage.FreshInput.Value)
	}
}

func TestObservationsFromSessionMissingReasoningStaysMissing(t *testing.T) {
	usage := model.Usage{
		Input:  obsMeasured(100, obsSourceValue("rec", "input_tokens")),
		Cached: obsMeasured(40, obsSourceValue("rec", "cached_input_tokens")),
		Output: obsMeasured(20, obsSourceValue("rec", "output_tokens")),
		Total:  obsMeasured(120, obsSourceValue("rec", "total_tokens")),
	}
	observation := obsProject(t, model.Session{ID: "s", Source: "codex", Usage: usage})[0]
	if observation.Usage.ReasoningOutput.Value != nil || observation.Usage.ReasoningOutput.Kind != "" {
		t.Fatalf("reasoningOutput = %+v, want missing", observation.Usage.ReasoningOutput)
	}
	if observation.Completeness != model.ObservationCompletenessComplete {
		t.Fatalf("completeness = %q, want complete without reasoning", observation.Completeness)
	}
}

func TestObservationsFromSessionMissingCachedKeepsFreshMissing(t *testing.T) {
	usage := model.Usage{
		Input:  obsMeasured(100, obsSourceValue("rec", "input_tokens")),
		Output: obsMeasured(20, obsSourceValue("rec", "output_tokens")),
		Total:  obsMeasured(120, obsSourceValue("rec", "total_tokens")),
	}
	observation := obsProject(t, model.Session{ID: "s", Source: "codex", Usage: usage})[0]
	if observation.Usage.FreshInput.Value != nil || observation.Usage.FreshInput.Kind != "" {
		t.Fatalf("freshInput = %+v, want missing when cached missing", observation.Usage.FreshInput)
	}
}

func TestObservationsFromSessionNegativeFreshIsUnknown(t *testing.T) {
	usage := model.Usage{
		Input:  obsMeasured(100, obsSourceValue("rec", "input_tokens")),
		Cached: obsMeasured(150, obsSourceValue("rec", "cached_input_tokens")),
		Output: obsMeasured(20, obsSourceValue("rec", "output_tokens")),
		Total:  obsMeasured(120, obsSourceValue("rec", "total_tokens")),
	}
	observation := obsProject(t, model.Session{ID: "s", Source: "codex", Usage: usage})[0]
	fresh := observation.Usage.FreshInput
	if fresh.Value != nil {
		t.Fatalf("freshInput value = %v, want nil", *fresh.Value)
	}
	if fresh.Kind != model.MeasurementUnknown {
		t.Fatalf("freshInput kind = %q, want unknown", fresh.Kind)
	}
	if len(fresh.Evidence) != 0 {
		t.Fatalf("freshInput evidence = %+v, want none", fresh.Evidence)
	}
	if observation.Usage.TotalInput.ValueOrZero() != 100 {
		t.Fatalf("totalInput = %d, want 100 preserved", observation.Usage.TotalInput.ValueOrZero())
	}
	if observation.Usage.CachedInput.ValueOrZero() != 150 {
		t.Fatalf("cachedInput = %d, want 150 preserved", observation.Usage.CachedInput.ValueOrZero())
	}
}

func TestObservationsFromSessionTurnCumulativeDeltaEvidence(t *testing.T) {
	turn := obsProject(t, obsSession())[1]
	metric := turn.Usage.TotalInput
	if metric.Kind != model.MeasurementDerived {
		t.Fatalf("turn totalInput kind = %q, want derived", metric.Kind)
	}
	if len(metric.Evidence) != 1 {
		t.Fatalf("turn totalInput evidence = %+v, want one entry", metric.Evidence)
	}
	want := obsCumulativeDelta("", "codex_rollout#snap:1", "input_tokens")
	if !reflect.DeepEqual(metric.Evidence[0], want) {
		t.Fatalf("turn evidence = %+v, want %+v", metric.Evidence[0], want)
	}
}

func TestObservationsFromSessionTurnUsageUnavailable(t *testing.T) {
	session := obsSession()
	session.Turns = append(session.Turns, model.Turn{ID: "sess-1#3", Sequence: 3})
	observation := obsProject(t, session)[3]

	for name, measurement := range map[string]model.Measurement{
		"freshInput":      observation.Usage.FreshInput,
		"cachedInput":     observation.Usage.CachedInput,
		"totalInput":      observation.Usage.TotalInput,
		"output":          observation.Usage.Output,
		"reasoningOutput": observation.Usage.ReasoningOutput,
		"total":           observation.Usage.Total,
	} {
		if measurement.Value != nil || measurement.Kind != "" || len(measurement.Evidence) != 0 {
			t.Fatalf("%s = %+v, want missing without fake evidence", name, measurement)
		}
	}
	if observation.Completeness != model.ObservationCompletenessUnknown {
		t.Fatalf("completeness = %q, want unknown", observation.Completeness)
	}
}

func TestObservationsFromSessionOutcomeUnknownWithCompleteCompleteness(t *testing.T) {
	observation := obsProject(t, obsSession())[0]
	if observation.Outcome != model.ObservationOutcomeUnknown {
		t.Fatalf("outcome = %q, want unknown", observation.Outcome)
	}
	if observation.Completeness != model.ObservationCompletenessComplete {
		t.Fatalf("completeness = %q, want complete", observation.Completeness)
	}
}

func TestObservationsFromSessionCompleteness(t *testing.T) {
	cases := []struct {
		name  string
		usage model.Usage
		want  model.ObservationCompleteness
	}{
		{
			name:  "no usage",
			usage: model.Usage{},
			want:  model.ObservationCompletenessUnknown,
		},
		{
			name: "partial missing total",
			usage: model.Usage{
				Input:  obsMeasured(100, obsSourceValue("rec", "input_tokens")),
				Output: obsMeasured(20, obsSourceValue("rec", "output_tokens")),
			},
			want: model.ObservationCompletenessPartial,
		},
		{
			name: "partial reasoning only",
			usage: model.Usage{
				Reasoning: obsMeasured(5, obsSourceValue("rec", "reasoning_output_tokens")),
			},
			want: model.ObservationCompletenessPartial,
		},
		{
			name: "complete without reasoning",
			usage: model.Usage{
				Input:  obsMeasured(100, obsSourceValue("rec", "input_tokens")),
				Output: obsMeasured(20, obsSourceValue("rec", "output_tokens")),
				Total:  obsMeasured(120, obsSourceValue("rec", "total_tokens")),
			},
			want: model.ObservationCompletenessComplete,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := obsProject(t, model.Session{ID: "s", Source: "codex", Usage: tc.usage})[0].Completeness; got != tc.want {
				t.Fatalf("completeness = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestObservationsFromSessionRejectsInvalidSession(t *testing.T) {
	cases := []struct {
		name    string
		session model.Session
		want    string
	}{
		{
			name:    "empty id",
			session: model.Session{},
			want:    "id is required",
		},
		{
			name:    "non-codex agent",
			session: model.Session{ID: "s", Agent: model.AgentClaude},
			want:    "unexpected agent",
		},
		{
			name:    "non-codex source",
			session: model.Session{ID: "s", Source: "claude"},
			want:    "unexpected source",
		},
		{
			name:    "empty turn id",
			session: model.Session{ID: "s", Turns: []model.Turn{{Usage: model.Usage{}}}},
			want:    "turn id is required",
		},
		{
			name:    "duplicate turn id",
			session: model.Session{ID: "s", Turns: []model.Turn{{ID: "t"}, {ID: "t"}}},
			want:    "duplicate turn id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ObservationsFromSession(tc.session)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
		})
	}
}

func TestObservationsFromSessionAcceptsEmptyAgentAndSource(t *testing.T) {
	session := obsSession()
	session.Agent = ""
	session.Source = ""
	if _, err := ObservationsFromSession(session); err != nil {
		t.Fatalf("empty agent and source should be accepted: %v", err)
	}
}

func TestObservationsFromSessionDeterministicAndPure(t *testing.T) {
	session := obsSession()
	want := obsSession()

	first := obsProject(t, session)
	second := obsProject(t, session)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("projection is not deterministic:\nfirst  = %+v\nsecond = %+v", first, second)
	}
	if !reflect.DeepEqual(session, want) {
		t.Fatalf("session was mutated:\n got = %+v\nwant = %+v", session, want)
	}

	first[0].Usage.TotalInput.Value = model.Int64(9999)
	first[0].Usage.TotalInput.Evidence[0].Record = "mutated"
	if *session.Usage.Input.Value == 9999 || session.Usage.Input.Evidence[0].Record == "mutated" {
		t.Fatal("observation shares mutable state with the session")
	}
}

func TestObservationsFromSessionPrivacy(t *testing.T) {
	session := obsSession()
	session.Turns[0].ContextAttribution = model.ContextAttribution{
		Components: []model.ContextComponent{{
			Kind:        model.ContextUserPrompt,
			Path:        "/private/secret/project",
			ContentHash: "content-secret-hash",
			Measurement: model.NewMeasurement(12, model.MeasurementEstimated),
		}},
	}
	observations := obsProject(t, session)
	data, err := json.Marshal(observations)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{
		"prompt", "payload", "contextAttribution", "components", "secret",
		"content-secret-hash", "/private/secret/project", "apiKey", "credential", "sk-live",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("observation JSON contains forbidden data %q: %s", forbidden, text)
		}
	}
}

func TestObservationsFromSessionAllValidate(t *testing.T) {
	observations, err := ObservationsFromSession(obsSession())
	if err != nil {
		t.Fatalf("ObservationsFromSession: %v", err)
	}
	for i, observation := range observations {
		if err := observation.Validate(); err != nil {
			t.Fatalf("observation[%d] invalid: %v", i, err)
		}
	}
}

func TestObservationsFromNormalizedSessionMultiSnapshot(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	observations, err := ObservationsFromSession(sessions[0])
	if err != nil {
		t.Fatalf("ObservationsFromSession: %v", err)
	}
	if len(observations) != 4 {
		t.Fatalf("observations = %d, want session plus three turns", len(observations))
	}
	sessionObservation := observations[0]
	if sessionObservation.Completeness != model.ObservationCompletenessComplete {
		t.Fatalf("session completeness = %q, want complete", sessionObservation.Completeness)
	}
	if sessionObservation.Outcome != model.ObservationOutcomeUnknown {
		t.Fatalf("session outcome = %q, want unknown", sessionObservation.Outcome)
	}
	if sessionObservation.Usage.FreshInput.Value == nil || *sessionObservation.Usage.FreshInput.Value != 1600 {
		t.Fatalf("session freshInput = %v, want 1600", sessionObservation.Usage.FreshInput.Value)
	}
	for i, turnObservation := range observations[1:] {
		if turnObservation.Usage.Total.Kind != model.MeasurementDerived {
			t.Fatalf("turn %d total kind = %q, want derived", i+1, turnObservation.Usage.Total.Kind)
		}
		if len(turnObservation.Usage.Total.Evidence) != 1 || turnObservation.Usage.Total.Evidence[0].Kind != model.EvidenceCumulativeDelta {
			t.Fatalf("turn %d total evidence = %+v, want cumulative delta", i+1, turnObservation.Usage.Total.Evidence)
		}
	}
}

func TestObservationsFromNormalizedSessionAnomalyTurn(t *testing.T) {
	withFixtureHome(t, "anomaly-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 || len(sessions[0].Turns) != 2 {
		t.Fatalf("sessions = %+v, want one session with two turns", sessions)
	}
	observations, err := ObservationsFromSession(sessions[0])
	if err != nil {
		t.Fatalf("ObservationsFromSession: %v", err)
	}
	anomaly := observations[2]
	if anomaly.Usage.Total.Value != nil || anomaly.Usage.Total.HasEvidence() {
		t.Fatalf("anomaly turn total = %+v, want unavailable without fake evidence", anomaly.Usage.Total)
	}
	if anomaly.Completeness != model.ObservationCompletenessUnknown {
		t.Fatalf("anomaly completeness = %q, want unknown", anomaly.Completeness)
	}
}

package claude

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func claudeObsTurnUsage(input, cacheRead, cacheWrite, output int64) model.Usage {
	cached := cacheRead + cacheWrite
	return model.Usage{
		Input:  model.NewMeasurement(input, model.MeasurementMeasured),
		Cached: model.NewMeasurement(cached, model.MeasurementDerived),
		Output: model.NewMeasurement(output, model.MeasurementMeasured),
		Total:  model.NewMeasurement(input+cached+output, model.MeasurementDerived),
		Billable: &model.BillableUsage{
			Input:      model.NewMeasurement(input+cacheRead+cacheWrite, model.MeasurementDerived),
			CacheRead:  model.NewMeasurement(cacheRead, model.MeasurementMeasured),
			CacheWrite: model.NewMeasurement(cacheWrite, model.MeasurementMeasured),
			Output:     model.NewMeasurement(output, model.MeasurementMeasured),
		},
	}
}

func claudeObsSession() model.Session {
	started := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	turnStamp := time.Date(2026, 4, 5, 6, 7, 9, 0, time.UTC)
	return model.Session{
		ID:        "sess-1",
		Source:    "claude",
		Agent:     model.AgentClaude,
		StartedAt: &started,
		Model:     "claude-sonnet-4",
		Usage: model.Usage{
			Input:  model.NewMeasurement(1250, model.MeasurementDerived),
			Cached: model.NewMeasurement(550, model.MeasurementDerived),
			Output: model.NewMeasurement(275, model.MeasurementDerived),
			Total:  model.NewMeasurement(2075, model.MeasurementDerived),
			Billable: &model.BillableUsage{
				Input:      model.NewMeasurement(1800, model.MeasurementDerived),
				CacheRead:  model.NewMeasurement(400, model.MeasurementMeasured),
				CacheWrite: model.NewMeasurement(150, model.MeasurementMeasured),
				Output:     model.NewMeasurement(275, model.MeasurementMeasured),
			},
		},
		Turns: []model.Turn{
			{ID: "sess-1#1", Sequence: 1, Timestamp: &turnStamp, Model: "claude-sonnet-4-turn", Usage: claudeObsTurnUsage(1000, 300, 100, 200)},
			{ID: "sess-1#2", Sequence: 2, Usage: claudeObsTurnUsage(250, 100, 50, 75)},
		},
	}
}

func claudeObsProject(t *testing.T, session model.Session) []model.Observation {
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

func claudeObsFromFixture(t *testing.T, fixture string) model.Session {
	t.Helper()
	withFixtureHome(t, fixture)
	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1 for %s", len(sessions), fixture)
	}
	return sessions[0]
}

func claudeObsFromParsedFixture(t *testing.T, fixture, id string) []model.Observation {
	t.Helper()
	parsed, err := ParseSession(fixturePath(t, fixture))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	session := model.Session{
		ID:     id,
		Source: "claude",
		Agent:  model.AgentClaude,
		Model:  parsed.Model,
		Usage:  parsed.Usage.toModelUsage(model.MeasurementDerived),
		Turns:  claudeTurns(id, parsed),
	}
	return claudeObsProject(t, session)
}

func TestObservationsFromSessionStructure(t *testing.T) {
	observations := claudeObsProject(t, claudeObsSession())
	if len(observations) != 3 {
		t.Fatalf("observations = %d, want 3", len(observations))
	}
	sessionObservation := observations[0]
	if sessionObservation.SchemaVersion != model.ObservationSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", sessionObservation.SchemaVersion, model.ObservationSchemaVersion)
	}
	if sessionObservation.ID != "claude:session:sess-1" {
		t.Fatalf("session id = %q, want claude:session:sess-1", sessionObservation.ID)
	}
	if sessionObservation.Channel != model.ObservationChannelSessionTranscript {
		t.Fatalf("channel = %q, want session_transcript", sessionObservation.Channel)
	}
	if sessionObservation.Scope != model.ObservationScopeSession {
		t.Fatalf("scope = %q, want session", sessionObservation.Scope)
	}
	if sessionObservation.Source != "claude" {
		t.Fatalf("source = %q, want claude", sessionObservation.Source)
	}
	wantTurnIDs := []string{"claude:turn:sess-1#1", "claude:turn:sess-1#2"}
	for i, want := range wantTurnIDs {
		if observations[i+1].ID != want {
			t.Fatalf("observations[%d].id = %q, want %q", i+1, observations[i+1].ID, want)
		}
		if observations[i+1].Scope != model.ObservationScopeTurn {
			t.Fatalf("turn scope = %q, want turn", observations[i+1].Scope)
		}
	}
}

func TestObservationsFromSessionDeterministicAndPure(t *testing.T) {
	session := claudeObsSession()
	want := claudeObsSession()

	first := claudeObsProject(t, session)
	second := claudeObsProject(t, session)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("projection is not deterministic:\nfirst  = %+v\nsecond = %+v", first, second)
	}
	if !reflect.DeepEqual(session, want) {
		t.Fatalf("session was mutated:\n got = %+v\nwant = %+v", session, want)
	}

	first[0].Usage.FreshInput.Value = model.Int64(9999)
	if session.Usage.Input.ValueOrZero() == 9999 {
		t.Fatal("observation shares mutable state with the session")
	}
}

func TestObservationsFromSessionIdentity(t *testing.T) {
	observations := claudeObsProject(t, claudeObsSession())
	sessionIdentity := observations[0].Identity
	if sessionIdentity != (model.ObservationIdentity{SessionID: "sess-1"}) {
		t.Fatalf("session identity = %+v, want session id only", sessionIdentity)
	}
	turnIdentity := observations[1].Identity
	if turnIdentity != (model.ObservationIdentity{SessionID: "sess-1", TurnID: "sess-1#1"}) {
		t.Fatalf("turn identity = %+v, want session and turn ids", turnIdentity)
	}
	for _, identity := range []model.ObservationIdentity{sessionIdentity, turnIdentity} {
		if identity.ExchangeID != "" || identity.AgentRequestID != "" || identity.ProviderRequestID != "" ||
			identity.ResponseObjectID != "" || identity.ParentResponseObjectID != "" ||
			identity.InvocationID != "" || identity.ChainID != "" {
			t.Fatalf("unexpected synthesized identity: %+v", identity)
		}
	}
	if turnIdentity.TurnID == observations[1].ID {
		t.Fatal("observation id was reused as correlation identity")
	}
}

func TestObservationsFromSessionTimeAndModel(t *testing.T) {
	session := claudeObsSession()
	observations := claudeObsProject(t, session)

	sessionObservation := observations[0]
	if sessionObservation.StartedAt == nil || !sessionObservation.StartedAt.Equal(*session.StartedAt) {
		t.Fatalf("session startedAt = %v, want %v", sessionObservation.StartedAt, session.StartedAt)
	}
	if sessionObservation.FinishedAt != nil {
		t.Fatalf("session finishedAt = %v, want nil", sessionObservation.FinishedAt)
	}
	if sessionObservation.Model != "claude-sonnet-4" {
		t.Fatalf("session model = %q, want session model", sessionObservation.Model)
	}

	firstTurn := observations[1]
	if firstTurn.StartedAt == nil || !firstTurn.StartedAt.Equal(*session.Turns[0].Timestamp) {
		t.Fatalf("turn startedAt = %v, want %v", firstTurn.StartedAt, session.Turns[0].Timestamp)
	}
	if firstTurn.FinishedAt != nil {
		t.Fatalf("turn finishedAt = %v, want nil", firstTurn.FinishedAt)
	}
	if firstTurn.Model != "claude-sonnet-4-turn" {
		t.Fatalf("turn model = %q, want turn override", firstTurn.Model)
	}
	if observations[2].Model != "claude-sonnet-4" {
		t.Fatalf("turn model fallback = %q, want session model", observations[2].Model)
	}
}

func TestObservationsFromSessionNilTimestamps(t *testing.T) {
	session := model.Session{
		ID:     "nil-time",
		Source: "claude",
		Turns:  []model.Turn{{ID: "nil-time#1", Usage: claudeObsTurnUsage(1, 0, 0, 1)}},
	}
	observations := claudeObsProject(t, session)
	for i, observation := range observations {
		if observation.StartedAt != nil {
			t.Fatalf("observation[%d] startedAt = %v, want nil", i, observation.StartedAt)
		}
	}
}

func TestObservationsFromSessionUsageMapping(t *testing.T) {
	observations := claudeObsProject(t, claudeObsSession())
	turn := observations[1].Usage

	if turn.FreshInput.ValueOrZero() != 1000 || turn.FreshInput.Kind != model.MeasurementMeasured {
		t.Fatalf("freshInput = %+v, want measured 1000", turn.FreshInput)
	}
	if turn.CachedInput.ValueOrZero() != 300 || turn.CachedInput.Kind != model.MeasurementMeasured {
		t.Fatalf("cachedInput = %+v, want measured cache read only", turn.CachedInput)
	}
	if turn.CacheCreationInput.ValueOrZero() != 100 || turn.CacheCreationInput.Kind != model.MeasurementMeasured {
		t.Fatalf("cacheCreationInput = %+v, want measured cache creation only", turn.CacheCreationInput)
	}
	if turn.Output.ValueOrZero() != 200 || turn.Output.Kind != model.MeasurementMeasured {
		t.Fatalf("output = %+v, want measured 200", turn.Output)
	}
	if turn.TotalInput.ValueOrZero() != 1400 || turn.TotalInput.Kind != model.MeasurementDerived {
		t.Fatalf("totalInput = %+v, want derived 1400", turn.TotalInput)
	}
	if turn.Total.ValueOrZero() != 1600 || turn.Total.Kind != model.MeasurementDerived {
		t.Fatalf("total = %+v, want derived 1600", turn.Total)
	}
	if turn.ReasoningOutput.Value != nil || turn.ReasoningOutput.Kind != "" {
		t.Fatalf("reasoningOutput = %+v, want missing", turn.ReasoningOutput)
	}
}

func TestObservationsFromSessionCachedInputIsNotMerged(t *testing.T) {
	observations := claudeObsProject(t, claudeObsSession())
	turn := observations[1].Usage
	if turn.CachedInput.ValueOrZero() == turn.CacheCreationInput.ValueOrZero() {
		t.Fatal("cache read and cache creation must stay independent")
	}
	if turn.CachedInput.ValueOrZero() != 300 || turn.CacheCreationInput.ValueOrZero() != 100 {
		t.Fatalf("cachedInput/cacheCreationInput = %d/%d, want 300/100", turn.CachedInput.ValueOrZero(), turn.CacheCreationInput.ValueOrZero())
	}
}

func TestObservationsFromSessionPerFieldEvidence(t *testing.T) {
	observations := claudeObsProject(t, claudeObsSession())
	turn := observations[1].Usage
	cases := []struct {
		name   string
		metric model.Measurement
		field  string
	}{
		{"freshInput", turn.FreshInput, claudeFieldInputTokens},
		{"cachedInput", turn.CachedInput, claudeFieldCacheRead},
		{"cacheCreationInput", turn.CacheCreationInput, claudeFieldCacheCreation},
		{"output", turn.Output, claudeFieldOutputTokens},
	}
	for _, tc := range cases {
		want := model.Evidence{Kind: model.EvidenceSourceValue, Source: "claude_session", Record: "sess-1#1", Field: tc.field}
		if len(tc.metric.Evidence) != 1 || !reflect.DeepEqual(tc.metric.Evidence[0], want) {
			t.Fatalf("%s evidence = %+v, want %+v", tc.name, tc.metric.Evidence, want)
		}
	}
}

func TestObservationsFromSessionDerivedEvidence(t *testing.T) {
	observations := claudeObsProject(t, claudeObsSession())
	turn := observations[1].Usage

	totalInput := turn.TotalInput.Evidence
	if len(totalInput) != 1 {
		t.Fatalf("totalInput evidence = %+v, want one entry", totalInput)
	}
	if totalInput[0].Kind != model.EvidenceAggregate || totalInput[0].Source != "claude_session" ||
		totalInput[0].Operation != model.AggregateSum || totalInput[0].Count != 3 {
		t.Fatalf("totalInput evidence = %+v, want aggregate count 3", totalInput[0])
	}

	total := turn.Total.Evidence
	if len(total) != 1 {
		t.Fatalf("total evidence = %+v, want one entry", total)
	}
	if total[0].Kind != model.EvidenceAggregate || total[0].Operation != model.AggregateSum || total[0].Count != 2 {
		t.Fatalf("total evidence = %+v, want aggregate count 2", total[0])
	}
}

func TestObservationsFromSessionMissingOperandSkipsDerived(t *testing.T) {
	observations := claudeObsFromParsedFixture(t, "input-field-only-session.jsonl", "input-only")
	turn := observations[1].Usage
	if turn.FreshInput.ValueOrZero() != 7 {
		t.Fatalf("freshInput = %+v, want 7", turn.FreshInput)
	}
	for name, metric := range map[string]model.Measurement{
		"cachedInput":        turn.CachedInput,
		"cacheCreationInput": turn.CacheCreationInput,
		"totalInput":         turn.TotalInput,
		"output":             turn.Output,
		"total":              turn.Total,
	} {
		if metric.Value != nil || metric.Kind != "" {
			t.Fatalf("%s = %+v, want missing", name, metric)
		}
	}
}

func TestObservationsFromSessionOutputOnly(t *testing.T) {
	observations := claudeObsFromParsedFixture(t, "output-field-only-session.jsonl", "output-only")
	turn := observations[1].Usage
	if turn.Output.ValueOrZero() != 9 || turn.Output.Kind != model.MeasurementMeasured {
		t.Fatalf("output = %+v, want measured 9", turn.Output)
	}
	for name, metric := range map[string]model.Measurement{
		"freshInput":  turn.FreshInput,
		"cachedInput": turn.CachedInput,
		"totalInput":  turn.TotalInput,
		"total":       turn.Total,
	} {
		if metric.Value != nil || metric.Kind != "" {
			t.Fatalf("%s = %+v, want missing", name, metric)
		}
	}
}

func TestObservationsFromSessionCacheReadOnly(t *testing.T) {
	observations := claudeObsFromParsedFixture(t, "cache-read-field-only-session.jsonl", "cache-read-only")
	turn := observations[1].Usage
	if turn.CachedInput.ValueOrZero() != 11 || turn.CachedInput.Kind != model.MeasurementMeasured {
		t.Fatalf("cachedInput = %+v, want measured 11", turn.CachedInput)
	}
	if turn.CacheCreationInput.Value != nil || turn.CacheCreationInput.Kind != "" {
		t.Fatalf("cacheCreationInput = %+v, want missing", turn.CacheCreationInput)
	}
	if turn.TotalInput.Value != nil || turn.Total.Value != nil {
		t.Fatalf("derived totals = %+v/%+v, want missing", turn.TotalInput, turn.Total)
	}
}

func TestObservationsFromSessionExplicitZero(t *testing.T) {
	observations := claudeObsFromParsedFixture(t, "explicit-zero-usage-session.jsonl", "explicit-zero")
	turn := observations[1].Usage
	for name, metric := range map[string]model.Measurement{
		"freshInput":         turn.FreshInput,
		"cachedInput":        turn.CachedInput,
		"cacheCreationInput": turn.CacheCreationInput,
		"output":             turn.Output,
		"totalInput":         turn.TotalInput,
		"total":              turn.Total,
	} {
		if metric.Value == nil || *metric.Value != 0 {
			t.Fatalf("%s = %v, want explicit zero value", name, metric.Value)
		}
	}
	if observations[1].Completeness != model.ObservationCompletenessComplete {
		t.Fatalf("completeness = %q, want complete", observations[1].Completeness)
	}
}

func TestObservationsFromSessionNegativeUsage(t *testing.T) {
	observations := claudeObsFromParsedFixture(t, "negative-usage-session.jsonl", "negative")
	turn := observations[1].Usage
	for name, metric := range map[string]model.Measurement{
		"freshInput":         turn.FreshInput,
		"cachedInput":        turn.CachedInput,
		"cacheCreationInput": turn.CacheCreationInput,
		"output":             turn.Output,
		"totalInput":         turn.TotalInput,
		"total":              turn.Total,
	} {
		if metric.Value != nil {
			t.Fatalf("%s = %+v, want no negative value emitted", name, metric)
		}
	}
	if observations[1].Completeness == model.ObservationCompletenessComplete {
		t.Fatal("negative usage must not report complete completeness")
	}
}

func TestObservationsFromSessionAggregatesAcceptedTurns(t *testing.T) {
	observations := claudeObsProject(t, claudeObsFromFixture(t, "basic-session.jsonl"))
	session := observations[0].Usage
	if session.FreshInput.ValueOrZero() != 720 {
		t.Fatalf("freshInput = %d, want 720", session.FreshInput.ValueOrZero())
	}
	if session.CachedInput.ValueOrZero() != 0 || session.CacheCreationInput.ValueOrZero() != 0 {
		t.Fatalf("cachedInput/cacheCreationInput = %d/%d, want 0/0", session.CachedInput.ValueOrZero(), session.CacheCreationInput.ValueOrZero())
	}
	if session.Output.ValueOrZero() != 200 {
		t.Fatalf("output = %d, want 200", session.Output.ValueOrZero())
	}
	if session.TotalInput.ValueOrZero() != 720 || session.Total.ValueOrZero() != 920 {
		t.Fatalf("totalInput/total = %d/%d, want 720/920", session.TotalInput.ValueOrZero(), session.Total.ValueOrZero())
	}
}

func TestObservationsFromSessionMixedTurnModelsKeepUsage(t *testing.T) {
	observations := claudeObsProject(t, claudeObsFromFixture(t, "multiple-models-session.jsonl"))
	if len(observations) != 3 {
		t.Fatalf("observations = %d, want session plus two turns", len(observations))
	}
	models := map[string]string{observations[1].ID: observations[1].Model, observations[2].ID: observations[2].Model}
	if models["claude:turn:multiple-models-session#1"] != "MiniMax-M3" || models["claude:turn:multiple-models-session#2"] != "claude-sonnet-4" {
		t.Fatalf("turn models = %+v, want per-turn models", models)
	}
	if observations[1].Usage.FreshInput.ValueOrZero() != 50 || observations[2].Usage.FreshInput.ValueOrZero() != 60 {
		t.Fatalf("turn usage = %d/%d, want 50/60", observations[1].Usage.FreshInput.ValueOrZero(), observations[2].Usage.FreshInput.ValueOrZero())
	}
}

func TestObservationsFromSessionCompletenessAndOutcome(t *testing.T) {
	cases := []struct {
		name      string
		session   model.Session
		completes model.ObservationCompleteness
	}{
		{
			name:      "full usage",
			session:   claudeObsSession(),
			completes: model.ObservationCompletenessComplete,
		},
		{
			name:      "no usage",
			session:   model.Session{ID: "none", Source: "claude"},
			completes: model.ObservationCompletenessUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observations := claudeObsProject(t, tc.session)
			for _, observation := range observations {
				if observation.Outcome != model.ObservationOutcomeUnknown {
					t.Fatalf("outcome = %q, want unknown", observation.Outcome)
				}
				if observation.Completeness != tc.completes {
					t.Fatalf("completeness = %q, want %q", observation.Completeness, tc.completes)
				}
			}
		})
	}
}

func TestObservationsFromSessionPartialUsageCompleteness(t *testing.T) {
	session := model.Session{
		ID:     "partial",
		Source: "claude",
		Turns: []model.Turn{{
			ID: "partial#1",
			Usage: model.Usage{
				Input:  model.NewMeasurement(5, model.MeasurementMeasured),
				Output: model.NewMeasurement(2, model.MeasurementMeasured),
				Billable: &model.BillableUsage{
					CacheRead: model.NewMeasurement(1, model.MeasurementMeasured),
				},
			},
		}},
	}
	observations := claudeObsProject(t, session)
	if observations[1].Completeness != model.ObservationCompletenessPartial {
		t.Fatalf("turn completeness = %q, want partial", observations[1].Completeness)
	}
}

func TestObservationsFromSessionConflictDoesNotPublishAuthority(t *testing.T) {
	observations := claudeObsProject(t, claudeObsFromFixture(t, "conflict-mixed-session.jsonl"))
	if len(observations) != 3 {
		t.Fatalf("observations = %d, want session plus two turns", len(observations))
	}
	conflict := observations[1]
	if conflict.ID != "claude:turn:msg_dup" {
		t.Fatalf("conflict turn id = %q, want msg_dup turn", conflict.ID)
	}
	for name, metric := range map[string]model.Measurement{
		"freshInput":         conflict.Usage.FreshInput,
		"cachedInput":        conflict.Usage.CachedInput,
		"cacheCreationInput": conflict.Usage.CacheCreationInput,
		"output":             conflict.Usage.Output,
		"totalInput":         conflict.Usage.TotalInput,
		"total":              conflict.Usage.Total,
	} {
		if metric.Available() {
			t.Fatalf("%s = %+v, want non-authoritative usage", name, metric)
		}
	}
	if conflict.Completeness != model.ObservationCompletenessPartial {
		t.Fatalf("conflict completeness = %q, want partial", conflict.Completeness)
	}
	if observations[2].Completeness != model.ObservationCompletenessComplete {
		t.Fatalf("accepted turn completeness = %q, want complete", observations[2].Completeness)
	}
	session := observations[0].Usage
	if session.FreshInput.ValueOrZero() != 100 || session.Total.ValueOrZero() != 130 {
		t.Fatalf("session aggregate = %d/%d, want conflict excluded (100/130)", session.FreshInput.ValueOrZero(), session.Total.ValueOrZero())
	}
}

func TestObservationsFromSessionRejectsInvalidSession(t *testing.T) {
	cases := []struct {
		name    string
		session model.Session
		want    string
	}{
		{name: "empty id", session: model.Session{}, want: "id is required"},
		{name: "unexpected agent", session: model.Session{ID: "s", Agent: model.AgentCodex}, want: "unexpected agent"},
		{name: "unexpected source", session: model.Session{ID: "s", Source: "codex"}, want: "unexpected source"},
		{name: "empty turn id", session: model.Session{ID: "s", Turns: []model.Turn{{}}}, want: "turn id is required"},
		{name: "duplicate turn id", session: model.Session{ID: "s", Turns: []model.Turn{{ID: "t"}, {ID: "t"}}}, want: "duplicate turn id"},
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
			if !strings.Contains(err.Error(), "project claude session") {
				t.Fatalf("error = %q, want session context", err)
			}
		})
	}
}

func TestObservationsFromSessionAcceptsEmptyAgentAndSource(t *testing.T) {
	session := claudeObsSession()
	session.Agent = ""
	session.Source = ""
	if _, err := ObservationsFromSession(session); err != nil {
		t.Fatalf("empty agent and source should be accepted: %v", err)
	}
}

func TestObservationsFromSessionPrivacy(t *testing.T) {
	session := claudeObsFromFixture(t, "basic-session.jsonl")
	session.Turns[0].ContextAttribution = model.ContextAttribution{
		Components: []model.ContextComponent{{
			Kind:        model.ContextUserPrompt,
			Path:        "/private/secret/project",
			ContentHash: "content-secret-hash",
			Measurement: model.NewMeasurement(12, model.MeasurementEstimated),
		}},
	}
	observations := claudeObsProject(t, session)
	data, err := json.Marshal(observations)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{
		"contextAttribution", "components", "content-secret-hash", "/private/secret/project",
		"apiKey", "credential", "summary", "sourceFilePath",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("observation JSON contains forbidden data %q: %s", forbidden, text)
		}
	}
}

func TestObservationsFromSessionPartialTurnsStayPartial(t *testing.T) {
	observations := claudeObsProject(t, claudeObsFromFixture(t, "partial-turns-session.jsonl"))
	if len(observations) != 3 {
		t.Fatalf("observations = %d, want session plus two turns", len(observations))
	}
	session := observations[0]
	if session.Completeness != model.ObservationCompletenessPartial {
		t.Fatalf("session completeness = %q, want partial", session.Completeness)
	}
	for name, metric := range map[string]model.Measurement{
		"freshInput":         session.Usage.FreshInput,
		"cachedInput":        session.Usage.CachedInput,
		"cacheCreationInput": session.Usage.CacheCreationInput,
		"output":             session.Usage.Output,
		"totalInput":         session.Usage.TotalInput,
		"total":              session.Usage.Total,
	} {
		if metric.Value != nil {
			t.Fatalf("session %s = %+v, want no full numeric value from union of partial turns", name, metric)
		}
	}
}

func TestObservationsFromSessionAggregateRequiresEveryTurn(t *testing.T) {
	cases := []struct {
		fixture        string
		wantInputValue bool
	}{
		{fixture: "missing-output-turn-session.jsonl", wantInputValue: true},
		{fixture: "negative-input-turn-session.jsonl", wantInputValue: false},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			session := claudeObsProject(t, claudeObsFromFixture(t, tc.fixture))[0]
			if session.Completeness != model.ObservationCompletenessPartial {
				t.Fatalf("session completeness = %q, want partial", session.Completeness)
			}
			if session.Usage.Total.Value != nil {
				t.Fatalf("session total = %+v, want missing", session.Usage.Total)
			}
			if (session.Usage.FreshInput.Value != nil) != tc.wantInputValue {
				t.Fatalf("session freshInput = %+v, want numeric=%v", session.Usage.FreshInput, tc.wantInputValue)
			}
		})
	}
}

func TestObservationsFromSessionDoesNotPromoteNonMeasuredDirectFields(t *testing.T) {
	session := model.Session{
		ID:     "non-measured",
		Source: "claude",
		Turns: []model.Turn{{
			ID: "non-measured#1",
			Usage: model.Usage{
				Input:  model.NewMeasurement(10, model.MeasurementDerived),
				Output: model.NewMeasurement(2, model.MeasurementEstimated),
				Billable: &model.BillableUsage{
					CacheRead:  model.NewMeasurement(1, model.MeasurementCounted),
					CacheWrite: model.NewMeasurement(1, model.MeasurementEstimated),
				},
			},
		}},
	}
	observations := claudeObsProject(t, session)
	turn := observations[1]
	for name, metric := range map[string]model.Measurement{
		"freshInput":         turn.Usage.FreshInput,
		"cachedInput":        turn.Usage.CachedInput,
		"cacheCreationInput": turn.Usage.CacheCreationInput,
		"output":             turn.Usage.Output,
		"totalInput":         turn.Usage.TotalInput,
		"total":              turn.Usage.Total,
	} {
		if metric.Kind == model.MeasurementMeasured {
			t.Fatalf("%s = %+v, want non-measured not promoted to measured", name, metric)
		}
		if metric.Value != nil {
			t.Fatalf("%s = %+v, want no authoritative numeric value", name, metric)
		}
	}
	if turn.Completeness != model.ObservationCompletenessPartial {
		t.Fatalf("completeness = %q, want partial", turn.Completeness)
	}
}

func TestObservationsFromSessionAllValidate(t *testing.T) {
	for _, fixture := range []string{"basic-session.jsonl", "cache-session.jsonl", "conflict-mixed-session.jsonl"} {
		t.Run(fixture, func(t *testing.T) {
			observations := claudeObsProject(t, claudeObsFromFixture(t, fixture))
			for i, observation := range observations {
				if err := observation.Validate(); err != nil {
					t.Fatalf("observation[%d] invalid: %v", i, err)
				}
			}
		})
	}
}

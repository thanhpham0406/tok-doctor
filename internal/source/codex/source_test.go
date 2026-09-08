package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestDetectMissingSource(t *testing.T) {
	result := source.DetectAutoPathSource("codex", "Codex", nil, []source.PathCandidate{
		{Kind: "filesystem", Path: t.TempDir() + "/missing"},
	}, nil, nil, hasCodexSessionData)
	if result.Status != source.StatusUnavailable {
		t.Fatalf("Status = %q, want unavailable", result.Status)
	}
}

func TestDetectMissingConfiguredSource(t *testing.T) {
	result := New().Detect(context.Background(), source.Override{
		Path:   t.TempDir() + "/configured-missing",
		Origin: source.OriginConfig,
	})
	if result.Status != source.StatusBroken {
		t.Fatalf("Status = %q, want broken", result.Status)
	}
}

func TestDetectReadySource(t *testing.T) {
	dir := t.TempDir()
	touch := dir + "/session.jsonl"
	if err := os.WriteFile(touch, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := New().Detect(context.Background(), source.Override{
		Path:   dir,
		Origin: source.OriginCLI,
	})

	if result.Status != source.StatusReady {
		t.Fatalf("Status = %q, want ready", result.Status)
	}
	if result.Origin != source.OriginCLI {
		t.Fatalf("Origin = %q, want cli", result.Origin)
	}
}

func TestEmptySession(t *testing.T) {
	session := New().EmptySession(context.Background())

	if session.Agent != "codex" {
		t.Fatalf("Agent = %q, want codex", session.Agent)
	}
	if session.ID == "" {
		t.Fatal("ID is empty")
	}
}

func withFixtureHome(t *testing.T, names ...string) {
	t.Helper()
	home := t.TempDir()
	sessions := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	for _, name := range names {
		src := filepath.Join("..", "..", "..", "fixtures", "codex", name)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		dst := filepath.Join(sessions, name)
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			t.Fatalf("copy fixture %s: %v", name, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestSourceUsageAggregatesAcrossFixtures(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "multi-snapshot-session.jsonl")

	src := New()
	refs, err := src.Sessions(context.Background())
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("expected sessions to be discovered")
	}

	usage, err := src.Usage(context.Background(), refs)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}

	wantInput := int64(3600)
	wantCached := int64(1100)
	wantOutput := int64(1550)
	wantReasoning := int64(150)
	wantTotal := int64(6400)
	if usage.Input.ValueOrZero() != wantInput || usage.Cached.ValueOrZero() != wantCached || usage.Output.ValueOrZero() != wantOutput ||
		usage.Total.ValueOrZero() != wantTotal {
		t.Fatalf("usage = %+v, want input=%d cached=%d output=%d total=%d",
			usage, wantInput, wantCached, wantOutput, wantTotal)
	}
	if usage.Reasoning.Value == nil || usage.Reasoning.ValueOrZero() != wantReasoning {
		t.Fatalf("reasoning = %v, want pointer to %d", usage.Reasoning, wantReasoning)
	}
	if usage.Total.Kind != model.MeasurementDerived {
		t.Fatalf("total kind = %q, want derived aggregate", usage.Total.Kind)
	}
}

func TestSourceUsageNoDataIsZero(t *testing.T) {
	withFixtureHome(t, "no-usage-session.jsonl")

	src := New()
	refs, err := src.Sessions(context.Background())
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}

	usage, err := src.Usage(context.Background(), refs)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.HasUsage() {
		t.Fatalf("usage = %+v, want unavailable", usage)
	}
	if usage.Total.ValueOrZero() != 0 {
		t.Fatalf("total = %d, want 0", usage.Total.ValueOrZero())
	}
}

func TestReadSessionsUsesStableIdentityAndFinalUsage(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	session := sessions[0]
	if session.ID != "sess-2" {
		t.Fatalf("ID = %q, want source session id", session.ID)
	}
	if session.Usage.Total.ValueOrZero() != 4450 {
		t.Fatalf("total = %d, want final cumulative total 4450", session.Usage.Total.ValueOrZero())
	}
	if session.Usage.Total.Kind != model.MeasurementMeasured {
		t.Fatalf("total kind = %q, want measured", session.Usage.Total.Kind)
	}
	if session.Usage.Reasoning.Value == nil || session.Usage.Reasoning.ValueOrZero() != 150 {
		t.Fatalf("reasoning = %v, want final cumulative reasoning 150", session.Usage.Reasoning)
	}
}

func TestReadSessionsAggregateReconcilesWithUsage(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "multi-snapshot-session.jsonl")

	src := New()
	refs, err := src.Sessions(context.Background())
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	usage, err := src.Usage(context.Background(), refs)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	sessions, err := src.ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}

	var snaps []UsageSnapshot
	for _, session := range sessions {
		snaps = append(snaps, UsageSnapshot{
			Input:     session.Usage.Input.ValueOrZero(),
			Cached:    session.Usage.Cached.ValueOrZero(),
			Output:    session.Usage.Output.ValueOrZero(),
			Reasoning: session.Usage.Reasoning.Value,
			Total:     session.Usage.Total.ValueOrZero(),
			HasUsage:  session.Usage.HasUsage(),
		})
	}
	reconciled := SumSnapshots(snaps)
	if reconciled.Input.ValueOrZero() != usage.Input.ValueOrZero() || reconciled.Cached.ValueOrZero() != usage.Cached.ValueOrZero() ||
		reconciled.Output.ValueOrZero() != usage.Output.ValueOrZero() || reconciled.Total.ValueOrZero() != usage.Total.ValueOrZero() {
		t.Fatalf("session aggregate = %+v, usage = %+v", reconciled, usage)
	}
}

func TestReadSessionsMissingModelStaysUnavailable(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if sessions[0].Model != "" {
		t.Fatalf("model = %q, want unavailable", sessions[0].Model)
	}
}

func TestReadSessionsTurnsFromCumulativeSnapshotsAreDerived(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	turns := sessions[0].Turns
	if len(turns) != 3 {
		t.Fatalf("turns = %d, want 3", len(turns))
	}

	cases := []struct {
		index int
		input int64
		cache int64
		out   int64
		reas  int64
		total int64
	}{
		{0, 800, 200, 300, 0, 1300},
		{1, 900, 300, 450, 0, 1650},
		{2, 700, 300, 350, 150, 1500},
	}
	for _, want := range cases {
		turn := turns[want.index]
		if turn.Sequence != want.index+1 {
			t.Fatalf("turn %d sequence = %d, want %d", want.index, turn.Sequence, want.index+1)
		}
		if turn.Usage.Input.ValueOrZero() != want.input ||
			turn.Usage.Cached.ValueOrZero() != want.cache ||
			turn.Usage.Output.ValueOrZero() != want.out ||
			turn.Usage.Total.ValueOrZero() != want.total {
			t.Fatalf("turn %d usage = %+v, want input=%d cache=%d out=%d total=%d",
				want.index, turn.Usage, want.input, want.cache, want.out, want.total)
		}
		if turn.Usage.Total.Kind != model.MeasurementDerived {
			t.Fatalf("turn %d total kind = %q, want derived", want.index, turn.Usage.Total.Kind)
		}
		if turn.Usage.Reasoning.Value == nil || turn.Usage.Reasoning.ValueOrZero() != want.reas {
			t.Fatalf("turn %d reasoning = %v, want %d (source reported explicit value)", want.index, turn.Usage.Reasoning, want.reas)
		}
	}
}

func TestReadSessionsTurnsReconcileWithSessionUsage(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	for _, sess := range sessions {
		if cmp := model.ReconcileUsage(sess); !cmp.Matches() {
			t.Fatalf("session %s: turns do not reconcile with session usage, comparison = %+v (session=%+v turns=%+v)",
				sess.ID, cmp, sess.Usage, sess.Turns)
		}
	}
}

func TestReadSessionsTurnsSingleSnapshotHasFirstTurn(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 || len(sessions[0].Turns) != 1 {
		t.Fatalf("expected single turn, got %d turns", len(sessions[0].Turns))
	}
	turn := sessions[0].Turns[0]
	if turn.Usage.Total.ValueOrZero() != 1950 {
		t.Fatalf("first turn total = %d, want 1950", turn.Usage.Total.ValueOrZero())
	}
	if turn.Usage.Total.Kind != model.MeasurementDerived {
		t.Fatalf("first turn total kind = %q, want derived", turn.Usage.Total.Kind)
	}
}

func TestReadSessionsUnchangedSnapshotHasDerivedZeroTurn(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	path := filepath.Join(sessionsDir, "unchanged.jsonl")
	data := []byte(
		`{"type":"session_meta","payload":{"id":"sess-unchanged"}}` + "\n" +
			`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":10,"reasoning_output_tokens":0,"total_tokens":110}}}}` + "\n" +
			`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":10,"reasoning_output_tokens":0,"total_tokens":110}}}}` + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 || len(sessions[0].Turns) != 2 {
		t.Fatalf("sessions = %+v, want one session with two turns", sessions)
	}
	second := sessions[0].Turns[1]
	if second.Usage.Total.ValueOrZero() != 0 {
		t.Fatalf("second turn total = %d, want derived zero", second.Usage.Total.ValueOrZero())
	}
	if second.Usage.Total.Kind != model.MeasurementDerived {
		t.Fatalf("second turn total kind = %q, want derived", second.Usage.Total.Kind)
	}
}

func TestReadSessionsTurnsMissingReasoningPreservesUnavailable(t *testing.T) {
	withFixtureHome(t, "missing-reasoning-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(sessions[0].Turns))
	}
	turn := sessions[0].Turns[0]
	if turn.Usage.Reasoning.Value != nil {
		t.Fatalf("reasoning = %v, want unavailable for missing reasoning field", turn.Usage.Reasoning.ValueOrZero())
	}
}

func TestReadSessionsTurnsDoNotDoubleCountCumulative(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	var sum int64
	for _, turn := range sessions[0].Turns {
		sum += turn.Usage.Total.ValueOrZero()
	}
	if sum != 4450 {
		t.Fatalf("Σ turn totals = %d, want 4450 (cumulative max), not 1300+2950+4450=8700", sum)
	}
}

func TestReadSessionsTurnsSequentialIDs(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	prev := ""
	for _, turn := range sessions[0].Turns {
		if prev != "" && turn.ID <= prev {
			t.Fatalf("turn ids not strictly increasing: %q <= %q", turn.ID, prev)
		}
		prev = turn.ID
	}
}

func TestReconcileUsageHandlesNoUsageSessionGracefully(t *testing.T) {
	withFixtureHome(t, "no-usage-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1 (source reader preserves parsed artifact)", len(sessions))
	}
	if len(sessions[0].Turns) != 0 {
		t.Fatalf("turns = %d, want 0 (no snapshots in fixture)", len(sessions[0].Turns))
	}
}

func TestParseSessionTurnCount(t *testing.T) {
	session, err := ParseSession(fixturePath(t, "multi-snapshot-session.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if len(session.Snapshots) != 3 {
		t.Fatalf("snapshots = %d, want 3", len(session.Snapshots))
	}
}

func TestReadSessionsAnomalyMarkedUnknownNotSilentlyClamped(t *testing.T) {
	withFixtureHome(t, "anomaly-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 || len(sessions[0].Turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(sessions[0].Turns))
	}
	second := sessions[0].Turns[1]
	if second.Usage.Total.Available() {
		t.Fatalf("anomaly turn total = %+v, want unavailable (not silently negative-clamped)", second.Usage.Total)
	}
}

func TestReadSessionsSessionEvidenceIsSourceValue(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	session := sessions[0]
	if session.Usage.Total.Kind != model.MeasurementMeasured {
		t.Fatalf("total kind = %q, want measured", session.Usage.Total.Kind)
	}
	for _, metric := range []model.Measurement{session.Usage.Input, session.Usage.Cached, session.Usage.Output, session.Usage.Total} {
		if len(metric.Evidence) == 0 {
			t.Fatalf("metric %+v missing evidence", metric)
		}
		ev := metric.Evidence[0]
		if ev.Kind != model.EvidenceSourceValue {
			t.Fatalf("evidence kind = %q, want source_value", ev.Kind)
		}
		if ev.Source != "codex_rollout" {
			t.Fatalf("evidence source = %q, want codex_rollout", ev.Source)
		}
		if ev.Record == "" {
			t.Fatalf("evidence record = empty, want codex snapshot id")
		}
		if ev.Field == "" {
			t.Fatalf("evidence field = empty, want total_token_usage.<field>")
		}
		if !ev.ConsistentWithKind(metric.Kind) {
			t.Fatalf("evidence inconsistent with measurement kind %q", metric.Kind)
		}
	}
	if session.Usage.Reasoning.Available() {
		ev := session.Usage.Reasoning.Evidence[0]
		if !strings.Contains(ev.Field, "reasoning") {
			t.Fatalf("reasoning evidence field = %q, want reasoning", ev.Field)
		}
	}
}

func TestReadSessionsTurnsCarryCumulativeDeltaEvidence(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	turns := sessions[0].Turns
	if len(turns) != 3 {
		t.Fatalf("turns = %d, want 3", len(turns))
	}

	for i, turn := range turns {
		for _, metric := range []model.Measurement{turn.Usage.Input, turn.Usage.Cached, turn.Usage.Output, turn.Usage.Total} {
			if !metric.Available() {
				continue
			}
			if len(metric.Evidence) == 0 {
				t.Fatalf("turn %d metric missing evidence", i)
			}
			ev := metric.Evidence[0]
			if ev.Kind != model.EvidenceCumulativeDelta {
				t.Fatalf("turn %d evidence kind = %q, want cumulative_delta", i, ev.Kind)
			}
			if ev.Source != "codex_rollout" {
				t.Fatalf("turn %d evidence source = %q, want codex_rollout", i, ev.Source)
			}
			if !ev.ConsistentWithKind(metric.Kind) {
				t.Fatalf("turn %d evidence inconsistent with measurement kind %q", i, metric.Kind)
			}
		}
	}
}

func TestReadSessionsFirstTurnHasNoPreviousRecord(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	first := sessions[0].Turns[0]
	ev := first.Usage.Input.Evidence[0]
	if ev.Previous != "" {
		t.Fatalf("first turn previous = %q, want empty (no fake prior record)", ev.Previous)
	}
	if ev.Current == "" {
		t.Fatalf("first turn current = empty, want snap:1")
	}
}

func TestReadSessionsSubsequentTurnsReferencePriorRecord(t *testing.T) {
	withFixtureHome(t, "multi-snapshot-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	second := sessions[0].Turns[1]
	ev := second.Usage.Input.Evidence[0]
	if ev.Previous == "" {
		t.Fatalf("second turn previous = empty, want snap:1 reference")
	}
	if ev.Current == "" {
		t.Fatalf("second turn current = empty, want snap:2 reference")
	}
	if ev.Previous == ev.Current {
		t.Fatalf("second turn previous = current = %q, must differ", ev.Previous)
	}
}

func TestReadSessionsExplicitZeroSnapshotRetainsEvidence(t *testing.T) {
	withFixtureHome(t, "explicit-zero-usage-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Usage.Total.ValueOrZero() != 0 {
		t.Fatalf("session total = %d, want explicit zero", sessions[0].Usage.Total.ValueOrZero())
	}
	if !sessions[0].Usage.Total.HasEvidence() {
		t.Fatalf("explicit zero session total should retain evidence")
	}
	if sessions[0].Usage.Total.Kind != model.MeasurementMeasured {
		t.Fatalf("session total kind = %q, want measured for explicit zero", sessions[0].Usage.Total.Kind)
	}
}

func TestReadSessionsUnavailableMetricHasNoEvidence(t *testing.T) {
	withFixtureHome(t, "missing-reasoning-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if sessions[0].Usage.Reasoning.Available() {
		t.Fatalf("reasoning should be unavailable for missing-reasoning session")
	}
	if sessions[0].Usage.Reasoning.HasEvidence() {
		t.Fatalf("unavailable reasoning should not carry evidence: %+v", sessions[0].Usage.Reasoning.Evidence)
	}
	if !sessions[0].Usage.Total.HasEvidence() {
		t.Fatalf("total should carry evidence: %+v", sessions[0].Usage.Total.Evidence)
	}
}

func TestReadSessionsAnomalyTurnHasNoFakeEvidence(t *testing.T) {
	withFixtureHome(t, "anomaly-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	second := sessions[0].Turns[1]
	if second.Usage.Total.HasEvidence() {
		t.Fatalf("unavailable anomaly turn should not carry fake evidence: %+v", second.Usage.Total.Evidence)
	}
}

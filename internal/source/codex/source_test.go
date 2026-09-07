package codex

import (
	"context"
	"os"
	"path/filepath"
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
	if usage.Input != wantInput || usage.Cached != wantCached || usage.Output != wantOutput ||
		usage.Total != wantTotal {
		t.Fatalf("usage = %+v, want input=%d cached=%d output=%d total=%d",
			usage, wantInput, wantCached, wantOutput, wantTotal)
	}
	if usage.Reasoning == nil || *usage.Reasoning != wantReasoning {
		t.Fatalf("reasoning = %v, want pointer to %d", usage.Reasoning, wantReasoning)
	}
	if usage.Confidence != model.ConfidenceMeasured {
		t.Fatalf("confidence = %q, want measured", usage.Confidence)
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
	if usage.Confidence != "" {
		t.Fatalf("confidence = %q, want empty", usage.Confidence)
	}
	if usage.Total != 0 {
		t.Fatalf("total = %d, want 0", usage.Total)
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
	if session.Usage.Total != 4450 {
		t.Fatalf("total = %d, want final cumulative total 4450", session.Usage.Total)
	}
	if session.Usage.Reasoning == nil || *session.Usage.Reasoning != 150 {
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
			Input:     session.Usage.Input,
			Cached:    session.Usage.Cached,
			Output:    session.Usage.Output,
			Reasoning: session.Usage.Reasoning,
			Total:     session.Usage.Total,
		})
	}
	reconciled := SumSnapshots(snaps)
	if reconciled.Input != usage.Input || reconciled.Cached != usage.Cached ||
		reconciled.Output != usage.Output || reconciled.Total != usage.Total {
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
		if turn.Usage.Input != want.input ||
			turn.Usage.Cached != want.cache ||
			turn.Usage.Output != want.out ||
			turn.Usage.Total != want.total {
			t.Fatalf("turn %d usage = %+v, want input=%d cache=%d out=%d total=%d",
				want.index, turn.Usage, want.input, want.cache, want.out, want.total)
		}
		if turn.Measurement != model.MeasurementDerived {
			t.Fatalf("turn %d measurement = %q, want derived", want.index, turn.Measurement)
		}
		if turn.Confidence != model.ConfidenceHigh {
			t.Fatalf("turn %d confidence = %q, want high", want.index, turn.Confidence)
		}
		if turn.Usage.Reasoning == nil || *turn.Usage.Reasoning != want.reas {
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
	if turn.Usage.Total != 1950 {
		t.Fatalf("first turn total = %d, want 1950", turn.Usage.Total)
	}
	if turn.Measurement != model.MeasurementDerived {
		t.Fatalf("first turn measurement = %q, want derived", turn.Measurement)
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
	if turn.Usage.Reasoning != nil {
		t.Fatalf("reasoning = %v, want nil for missing reasoning field", *turn.Usage.Reasoning)
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
		sum += turn.Usage.Total
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
		t.Fatalf("sessions = %d, want 1 (no-usage session still listed)", len(sessions))
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
	if second.Measurement != model.MeasurementUnknown {
		t.Fatalf("anomaly turn measurement = %q, want unknown", second.Measurement)
	}
	if second.Usage.Total != 0 {
		t.Fatalf("anomaly turn total = %d, want 0 (not silently negative-clamped)", second.Usage.Total)
	}
}

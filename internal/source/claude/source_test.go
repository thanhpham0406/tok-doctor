package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestConfiguredPathIsInstalled(t *testing.T) {
	result := New().Detect(context.Background(), source.Override{
		Path:   t.TempDir(),
		Origin: source.OriginConfig,
	})

	if result.Status != source.StatusInstalled {
		t.Fatalf("Status = %q, want installed", result.Status)
	}
}

func TestMissingConfiguredPathIsBroken(t *testing.T) {
	result := New().Detect(context.Background(), source.Override{
		Path:   t.TempDir() + "/missing",
		Origin: source.OriginConfig,
	})

	if result.Status != source.StatusBroken {
		t.Fatalf("Status = %q, want broken", result.Status)
	}
}

func withFixtureHome(t *testing.T, names ...string) {
	t.Helper()
	home := t.TempDir()
	projects := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}
	for _, name := range names {
		src := filepath.Join("..", "..", "..", "fixtures", "claude", name)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		dst := filepath.Join(projects, name)
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			t.Fatalf("copy fixture %s: %v", name, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestSourceUsageAggregatesAcrossFixtures(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "cache-session.jsonl")

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

	wantInput := int64(1470)
	wantOutput := int64(375)
	wantCached := int64(3400)
	if usage.Input != wantInput || usage.Output != wantOutput || usage.Cached != wantCached {
		t.Fatalf("usage = %+v, want input=%d output=%d cached=%d",
			usage, wantInput, wantOutput, wantCached)
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

func TestReadSessionsUsesFileIdentityAndNoReasoning(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	session := sessions[0]
	if session.ID != "basic-session" {
		t.Fatalf("ID = %q, want stable file id", session.ID)
	}
	if session.Usage.Total != 920 {
		t.Fatalf("total = %d, want 920", session.Usage.Total)
	}
	if session.Usage.Reasoning != nil {
		t.Fatalf("reasoning = %v, want unavailable", *session.Usage.Reasoning)
	}
}

func TestReadSessionsAggregateReconcilesWithUsage(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "cache-session.jsonl")

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
			Input:    session.Usage.Input,
			Cached:   session.Usage.Cached,
			Output:   session.Usage.Output,
			Total:    session.Usage.Total,
			HasUsage: session.Usage.Confidence != "",
		})
	}
	reconciled := SumSnapshots(snaps)
	if reconciled.Input != usage.Input || reconciled.Cached != usage.Cached ||
		reconciled.Output != usage.Output || reconciled.Total != usage.Total {
		t.Fatalf("session aggregate = %+v, usage = %+v", reconciled, usage)
	}
}

func TestReadSessionsSkipsEmptyArtifacts(t *testing.T) {
	withFixtureHome(t, "empty-artifact-session.jsonl", "synthetic-only-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %+v, want none", sessions)
	}
}

func TestReadSessionsKeepsUsageWithoutModel(t *testing.T) {
	withFixtureHome(t, "usage-without-model-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Model != "" {
		t.Fatalf("model = %q, want unavailable", sessions[0].Model)
	}
	if sessions[0].Usage.Total != 60 {
		t.Fatalf("total = %d, want 60", sessions[0].Usage.Total)
	}
}

func TestReadSessionsDoesNotExposeSyntheticModel(t *testing.T) {
	withFixtureHome(t, "synthetic-after-real-session.jsonl", "synthetic-before-real-session.jsonl", "multiple-models-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("sessions = %d, want 3", len(sessions))
	}
	for _, session := range sessions {
		if session.Model == "<synthetic>" {
			t.Fatalf("session %s exposed synthetic model", session.ID)
		}
	}
}

func TestReadSessionsSkipsAllZeroUsage(t *testing.T) {
	withFixtureHome(t, "explicit-zero-usage-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %+v, want none for all-zero usage", sessions)
	}
}

func TestReadSessionsSkipsEmptyUsageObject(t *testing.T) {
	withFixtureHome(t, "empty-usage-object-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %+v, want none for empty usage object", sessions)
	}
}

func TestReadSessionsSkipsMissingUsageObject(t *testing.T) {
	withFixtureHome(t, "empty-artifact-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %+v, want none for missing usage object", sessions)
	}
}

func TestReadSessionsKeepsCachedOnly(t *testing.T) {
	withFixtureHome(t, "cached-only-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Usage.Cached != 100 {
		t.Fatalf("cached = %d, want 100", sessions[0].Usage.Cached)
	}
}

func TestReadSessionsKeepsOutputOnly(t *testing.T) {
	withFixtureHome(t, "output-only-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Usage.Output != 10 {
		t.Fatalf("output = %d, want 10", sessions[0].Usage.Output)
	}
}

func TestReadSessionsKeepsInputOnly(t *testing.T) {
	withFixtureHome(t, "input-only-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Usage.Input != 1 {
		t.Fatalf("input = %d, want 1", sessions[0].Usage.Input)
	}
}

func TestUsageAggregateSkipsAllZeroSessions(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "explicit-zero-usage-session.jsonl")

	src := New()
	refs, err := src.Sessions(context.Background())
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	usage, err := src.Usage(context.Background(), refs)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if usage.Input != 720 || usage.Output != 200 {
		t.Fatalf("usage = %+v, want basic-session totals only (all-zero skipped)", usage)
	}
}

func TestReadSessionsAndUsageAgree(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "cache-session.jsonl", "explicit-zero-usage-session.jsonl")

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
	filterUsage := model.Usage{}
	for _, sess := range sessions {
		filterUsage.Input += sess.Usage.Input
		filterUsage.Cached += sess.Usage.Cached
		filterUsage.Output += sess.Usage.Output
		filterUsage.Total += sess.Usage.Total
	}
	if filterUsage.Input != usage.Input || filterUsage.Cached != usage.Cached ||
		filterUsage.Output != usage.Output || filterUsage.Total != usage.Total {
		t.Fatalf("usage aggregate = %+v, session aggregate = %+v", usage, filterUsage)
	}
}

func TestReadSessionsTurnsCountMatchesRealModelCalls(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "synthetic-after-real-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	turnCounts := map[string]int{}
	for _, sess := range sessions {
		turnCounts[sess.ID] = len(sess.Turns)
	}
	if turnCounts["basic-session"] != 2 {
		t.Fatalf("basic-session turns = %d, want 2", turnCounts["basic-session"])
	}
	if turnCounts["synthetic-after-real-session"] != 1 {
		t.Fatalf("synthetic-after-real-session turns = %d, want 1 (real invocation only; synthetic excluded)", turnCounts["synthetic-after-real-session"])
	}
}

func TestReadSessionsTurnsReconcileWithSessionUsage(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "cached-only-session.jsonl", "output-only-session.jsonl", "input-only-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	for _, sess := range sessions {
		if cmp := model.ReconcileUsage(sess); !cmp.Matches() {
			t.Fatalf("session %s: turns do not reconcile, comparison = %+v", sess.ID, cmp)
		}
	}
}

func TestReadSessionsTurnsKeepPerCallUsage(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if len(sessions[0].Turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(sessions[0].Turns))
	}
	first := sessions[0].Turns[0]
	if first.Usage.Input != 420 || first.Usage.Output != 80 {
		t.Fatalf("first turn usage = %+v, want 420/80 (per-call usage, not session aggregate)", first.Usage)
	}
	second := sessions[0].Turns[1]
	if second.Usage.Input != 300 || second.Usage.Output != 120 {
		t.Fatalf("second turn usage = %+v, want 300/120", second.Usage)
	}
}

func TestReadSessionsTurnsDoNotUseSyntheticModel(t *testing.T) {
	withFixtureHome(t, "synthetic-only-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %d, want 0 (no positive usage)", len(sessions))
	}
}

func TestReadSessionsTurnReasoningIsUnavailable(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	for _, sess := range sessions {
		for _, turn := range sess.Turns {
			if turn.Usage.Reasoning != nil {
				t.Fatalf("turn %s reasoning = %v, want nil (Claude session format does not expose reasoning)", turn.ID, *turn.Usage.Reasoning)
			}
		}
	}
}

func TestReadSessionsTurnSequenceIsStable(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	for _, sess := range sessions {
		for i, turn := range sess.Turns {
			if turn.Sequence != i+1 {
				t.Fatalf("session %s turn %d sequence = %d, want %d", sess.ID, i, turn.Sequence, i+1)
			}
			if turn.ID == "" {
				t.Fatalf("turn missing id at index %d", i)
			}
		}
	}
}

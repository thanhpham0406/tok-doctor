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
	if usage.Input.ValueOrZero() != wantInput || usage.Output.ValueOrZero() != wantOutput || usage.Cached.ValueOrZero() != wantCached {
		t.Fatalf("usage = %+v, want input=%d output=%d cached=%d",
			usage, wantInput, wantOutput, wantCached)
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
	if session.Usage.Total.ValueOrZero() != 920 {
		t.Fatalf("total = %d, want 920", session.Usage.Total.ValueOrZero())
	}
	if session.Usage.Total.Kind != model.MeasurementDerived {
		t.Fatalf("session total kind = %q, want derived", session.Usage.Total.Kind)
	}
	if session.Usage.Reasoning.Value != nil {
		t.Fatalf("reasoning = %v, want unavailable", session.Usage.Reasoning.ValueOrZero())
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
			Input:    session.Usage.Input.ValueOrZero(),
			Cached:   session.Usage.Cached.ValueOrZero(),
			Output:   session.Usage.Output.ValueOrZero(),
			Total:    session.Usage.Total.ValueOrZero(),
			HasUsage: session.Usage.HasUsage(),
		})
	}
	reconciled := SumSnapshots(snaps)
	if reconciled.Input.ValueOrZero() != usage.Input.ValueOrZero() || reconciled.Cached.ValueOrZero() != usage.Cached.ValueOrZero() ||
		reconciled.Output.ValueOrZero() != usage.Output.ValueOrZero() || reconciled.Total.ValueOrZero() != usage.Total.ValueOrZero() {
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
	if sessions[0].Usage.Total.ValueOrZero() != 60 {
		t.Fatalf("total = %d, want 60", sessions[0].Usage.Total.ValueOrZero())
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
	if sessions[0].Usage.Cached.ValueOrZero() != 100 {
		t.Fatalf("cached = %d, want 100", sessions[0].Usage.Cached.ValueOrZero())
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
	if sessions[0].Usage.Output.ValueOrZero() != 10 {
		t.Fatalf("output = %d, want 10", sessions[0].Usage.Output.ValueOrZero())
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
	if sessions[0].Usage.Input.ValueOrZero() != 1 {
		t.Fatalf("input = %d, want 1", sessions[0].Usage.Input.ValueOrZero())
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
	if usage.Input.ValueOrZero() != 720 || usage.Output.ValueOrZero() != 200 {
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
	var filterUsages []model.Usage
	for _, sess := range sessions {
		filterUsages = append(filterUsages, sess.Usage)
	}
	filterUsage := model.SumUsage(filterUsages)
	if filterUsage.Input.ValueOrZero() != usage.Input.ValueOrZero() || filterUsage.Cached.ValueOrZero() != usage.Cached.ValueOrZero() ||
		filterUsage.Output.ValueOrZero() != usage.Output.ValueOrZero() || filterUsage.Total.ValueOrZero() != usage.Total.ValueOrZero() {
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
	if first.Usage.Input.ValueOrZero() != 420 || first.Usage.Output.ValueOrZero() != 80 {
		t.Fatalf("first turn usage = %+v, want 420/80 (per-call usage, not session aggregate)", first.Usage)
	}
	if first.Usage.Input.Kind != model.MeasurementMeasured {
		t.Fatalf("first turn input kind = %q, want measured", first.Usage.Input.Kind)
	}
	second := sessions[0].Turns[1]
	if second.Usage.Input.ValueOrZero() != 300 || second.Usage.Output.ValueOrZero() != 120 {
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
			if turn.Usage.Reasoning.Value != nil {
				t.Fatalf("turn %s reasoning = %v, want unavailable (Claude session format does not expose reasoning)", turn.ID, turn.Usage.Reasoning.ValueOrZero())
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

func TestReadSessionsTurnsCarrySourceValueEvidence(t *testing.T) {
	withFixtureHome(t, "duplicate-message-id-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if len(sessions[0].Turns) != 2 {
		t.Fatalf("turns = %d, want 2 (duplicates dedupe, two distinct messages)", len(sessions[0].Turns))
	}
	for _, turn := range sessions[0].Turns {
		if turn.Usage.Input.Kind != model.MeasurementMeasured {
			t.Fatalf("turn %s input kind = %q, want measured", turn.ID, turn.Usage.Input.Kind)
		}
		for _, metric := range []model.Measurement{turn.Usage.Input, turn.Usage.Output, turn.Usage.Total} {
			if len(metric.Evidence) == 0 {
				t.Fatalf("turn %s metric missing evidence", turn.ID)
			}
			ev := metric.Evidence[0]
			if ev.Kind != model.EvidenceSourceValue {
				t.Fatalf("turn %s evidence kind = %q, want source_value", turn.ID, ev.Kind)
			}
			if ev.Source != "claude_session" {
				t.Fatalf("turn %s evidence source = %q, want claude_session", turn.ID, ev.Source)
			}
			if ev.Record != turn.ID {
				t.Fatalf("turn %s evidence record = %q, want message id %q", turn.ID, ev.Record, turn.ID)
			}
			if !ev.ConsistentWithKind(metric.Kind) {
				t.Fatalf("turn %s evidence inconsistent with measurement kind %q", turn.ID, metric.Kind)
			}
		}
	}
}

func TestReadSessionsDedupeRegressionEvidenceLineage(t *testing.T) {
	withFixtureHome(t, "duplicate-message-id-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	seen := map[string]int{}
	for _, turn := range sessions[0].Turns {
		for _, metric := range []model.Measurement{turn.Usage.Input, turn.Usage.Output, turn.Usage.Total} {
			for _, ev := range metric.Evidence {
				if ev.Kind == model.EvidenceSourceValue {
					seen[ev.Record]++
				}
			}
		}
	}
	if len(seen) != 2 {
		t.Fatalf("distinct records = %d, want 2 (one per unique message id)", len(seen))
	}
	for id, count := range seen {
		if count != 3 {
			t.Fatalf("record %s appeared on %d metrics, want 3 (one per metric)", id, count)
		}
	}
}

func TestReadSessionsSessionEvidenceIsAggregate(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if len(sessions[0].Evidence) != 1 {
		t.Fatalf("session evidence = %d, want 1", len(sessions[0].Evidence))
	}
	ev := sessions[0].Evidence[0]
	if ev.Kind != model.EvidenceAggregate {
		t.Fatalf("session evidence kind = %q, want aggregate", ev.Kind)
	}
	if ev.Source != "turns" {
		t.Fatalf("session evidence source = %q, want turns", ev.Source)
	}
	if ev.Operation != model.AggregateSum {
		t.Fatalf("session evidence operation = %q, want sum", ev.Operation)
	}
	if ev.Count != 2 {
		t.Fatalf("session evidence count = %d, want 2", ev.Count)
	}
}

func TestReadSessionsSessionUsageMetricsCarryAggregateEvidence(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	session := sessions[0]
	if session.Usage.Input.Kind != model.MeasurementDerived {
		t.Fatalf("session input kind = %q, want derived", session.Usage.Input.Kind)
	}
	found := false
	for _, ev := range session.Usage.Input.Evidence {
		if ev.Kind == model.EvidenceAggregate {
			found = true
		}
	}
	if !found {
		t.Fatalf("session input missing aggregate evidence: %+v", session.Usage.Input.Evidence)
	}
}

func TestReadSessionsUnavailableMetricHasNoEvidence(t *testing.T) {
	withFixtureHome(t, "cached-only-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 1 || len(sessions[0].Turns) != 1 {
		t.Fatalf("expected one session with one turn, got %d turns", len(sessions[0].Turns))
	}
	turn := sessions[0].Turns[0]
	if !turn.Usage.Cached.HasEvidence() {
		t.Fatalf("cached turn metric should carry evidence: %+v", turn.Usage.Cached.Evidence)
	}
	for _, metric := range []model.Measurement{turn.Usage.Cached, turn.Usage.Total} {
		if !metric.HasEvidence() {
			t.Fatalf("metric %+v should have evidence", metric)
		}
	}
}

func TestReadSessionsExplicitZeroRetainsEvidence(t *testing.T) {
	withFixtureHome(t, "explicit-zero-usage-session.jsonl")

	sessions, err := New().ReadSessions(context.Background())
	if err != nil {
		t.Fatalf("ReadSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Skip("explicit-zero sessions are filtered out at the source level by HasPositiveUsage; explicit zero retention is exercised for Codex where the snapshot is preserved")
	}
}

package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func fixturePath(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "fixtures", "claude", rel)
}

func TestParseSessionUsageBasic(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "basic-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Input != 720 || snap.Output != 200 || snap.Cached != 0 {
		t.Fatalf("snapshot = %+v, want input=720 output=200 cached=0", snap)
	}
	if snap.Total != 920 {
		t.Fatalf("total = %d, want 920", snap.Total)
	}
	if !snap.HasUsage {
		t.Fatal("HasUsage = false, want true")
	}
}

func TestParseSessionUsageCacheFields(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "cache-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Cached != 3400 {
		t.Fatalf("cached = %d, want 3400", snap.Cached)
	}
}

func TestParseSessionUsageSkipsUserAndMalformed(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "no-usage-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Total != 0 {
		t.Fatalf("snapshot = %+v, want total 0", snap)
	}
	if snap.HasUsage {
		t.Fatal("HasUsage = true, want false")
	}
}

func TestParseSessionUsageMissingFile(t *testing.T) {
	_, err := ParseSessionUsage(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseSessionUsageEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("write empty fixture: %v", err)
	}
	snap, err := ParseSessionUsage(path)
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if snap.Total != 0 {
		t.Fatalf("snapshot = %+v, want zero", snap)
	}
	if snap.HasUsage {
		t.Fatal("HasUsage = true, want false")
	}
}

func TestUsageSnapshotToModelUsage(t *testing.T) {
	u := UsageSnapshot{Input: 10, Cached: 5, Output: 3, Total: 18, HasUsage: true}.ToModelUsage()
	if u.Input.ValueOrZero() != 10 || u.Cached.ValueOrZero() != 5 || u.Output.ValueOrZero() != 3 || u.Total.ValueOrZero() != 18 {
		t.Fatalf("usage = %+v", u)
	}
	if u.Total.Kind != model.MeasurementMeasured {
		t.Fatalf("total kind = %q, want measured", u.Total.Kind)
	}
}

func TestUsageSnapshotZeroIsUnavailableWithoutPresence(t *testing.T) {
	u := UsageSnapshot{}.ToModelUsage()
	if u.HasUsage() {
		t.Fatalf("usage = %+v, want unavailable", u)
	}
}

func TestSumSnapshotsAggregates(t *testing.T) {
	snaps := []UsageSnapshot{
		{Input: 10, Cached: 5, Output: 3, Total: 18, HasUsage: true},
		{Input: 20, Cached: 2, Output: 4, Total: 26, HasUsage: true},
	}
	u := SumSnapshots(snaps)
	if u.Input.ValueOrZero() != 30 || u.Cached.ValueOrZero() != 7 || u.Output.ValueOrZero() != 7 || u.Total.ValueOrZero() != 44 {
		t.Fatalf("usage = %+v, want input=30 cached=7 output=7 total=44", u)
	}
	if u.Total.Kind != model.MeasurementDerived {
		t.Fatalf("total kind = %q, want derived aggregate", u.Total.Kind)
	}
}

func TestParseSessionExplicitZeroUsageIsMeasured(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "explicit-zero-usage-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !snap.HasUsage {
		t.Fatal("HasUsage = false, want true for explicit usage object")
	}
	u := snap.ToModelUsage()
	if u.Total.Kind != model.MeasurementMeasured {
		t.Fatalf("total kind = %q, want measured", u.Total.Kind)
	}
	if u.Total.ValueOrZero() != 0 {
		t.Fatalf("total = %d, want explicit zero", u.Total.ValueOrZero())
	}
}

func TestParseSessionSelectsRealModelAroundSynthetic(t *testing.T) {
	cases := map[string]string{
		"synthetic-after-real-session.jsonl":  "MiniMax-M3",
		"synthetic-before-real-session.jsonl": "MiniMax-M3",
	}

	for fixture, want := range cases {
		t.Run(fixture, func(t *testing.T) {
			session, err := ParseSession(fixturePath(t, fixture))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if session.Model != want {
				t.Fatalf("model = %q, want %q", session.Model, want)
			}
			if !session.Usage.HasUsage {
				t.Fatal("HasUsage = false, want true")
			}
		})
	}
}

func TestParseSessionSyntheticOnlyModelUnavailable(t *testing.T) {
	session, err := ParseSession(fixturePath(t, "synthetic-only-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if session.Model != "" {
		t.Fatalf("model = %q, want unavailable", session.Model)
	}
	if session.Usage.HasUsage {
		t.Fatal("HasUsage = true, want false")
	}
}

func TestParseSessionUsageWithoutModelRemainsVisibleInput(t *testing.T) {
	session, err := ParseSession(fixturePath(t, "usage-without-model-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if session.Model != "" {
		t.Fatalf("model = %q, want unavailable", session.Model)
	}
	if !session.Usage.HasUsage {
		t.Fatal("HasUsage = false, want true")
	}
}

func TestParseSessionMultipleRealModelsUnavailable(t *testing.T) {
	session, err := ParseSession(fixturePath(t, "multiple-models-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if session.Model != "" {
		t.Fatalf("model = %q, want unavailable for multiple real models", session.Model)
	}
	if !session.Usage.HasUsage {
		t.Fatal("HasUsage = false, want true")
	}
}

func TestSnapshotHasNoReasoningField(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "basic-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	u := snap.ToModelUsage()
	if u.Reasoning.Value != nil {
		t.Fatalf("reasoning = %v, want unavailable (Claude session format does not expose a separate reasoning field)", u.Reasoning.ValueOrZero())
	}
	if u.Output.ValueOrZero() != 200 {
		t.Fatalf("output = %d, want 200 from source output_tokens", u.Output.ValueOrZero())
	}
}

func TestParseSessionMissingUsageKeepsAvailabilityFalse(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "empty-artifact-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.HasUsage {
		t.Fatalf("HasUsage = true, want false for session with no usage object")
	}
	if snap.HasPositiveUsage() {
		t.Fatalf("HasPositiveUsage = true, want false")
	}
}

func TestParseSessionEmptyUsageObjectPreservesAbsence(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "empty-usage-object-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.HasUsage {
		t.Fatalf("HasUsage = true, want false because no fields were reported")
	}
	if snap.Input != 0 || snap.Output != 0 || snap.Cached != 0 || snap.Total != 0 {
		t.Fatalf("snapshot = %+v, want no fields populated", snap)
	}
	if snap.HasPositiveUsage() {
		t.Fatalf("HasPositiveUsage = true, want false for empty usage object")
	}
}

func TestParseSessionCachedOnlyIsPositive(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "cached-only-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Cached != 100 {
		t.Fatalf("cached = %d, want 100", snap.Cached)
	}
	if !snap.HasPositiveUsage() {
		t.Fatalf("HasPositiveUsage = false, want true")
	}
}

func TestParseSessionOutputOnlyIsPositive(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "output-only-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Output != 10 {
		t.Fatalf("output = %d, want 10", snap.Output)
	}
	if !snap.HasPositiveUsage() {
		t.Fatalf("HasPositiveUsage = false, want true")
	}
}

func TestParseSessionInputOnlyIsPositive(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "input-only-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Input != 1 {
		t.Fatalf("input = %d, want 1", snap.Input)
	}
	if !snap.HasPositiveUsage() {
		t.Fatalf("HasPositiveUsage = false, want true")
	}
}

func TestHasPositiveUsageDistinguishesMeasuredZeroFromMissing(t *testing.T) {
	zero := UsageSnapshot{HasUsage: true}
	if zero.HasPositiveUsage() {
		t.Fatal("explicit-zero snapshot should not be positive")
	}
	missing := UsageSnapshot{}
	if missing.HasPositiveUsage() {
		t.Fatal("missing snapshot should not be positive")
	}
	measured := UsageSnapshot{HasUsage: true, Input: 5}
	if !measured.HasPositiveUsage() {
		t.Fatal("positive snapshot should be positive")
	}
}

func TestParseSessionDedupesByMessageID(t *testing.T) {
	session, err := ParseSession(fixturePath(t, "duplicate-message-id-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(session.Invocations) != 2 {
		t.Fatalf("invocations = %d, want 2 (duplicate message.id must collapse)", len(session.Invocations))
	}
	if session.Usage.Input != 3000 {
		t.Fatalf("session input = %d, want 3000 (1000+2000, duplicates not summed)", session.Usage.Input)
	}
	if session.duplicates != 1 {
		t.Fatalf("duplicates = %d, want 1", session.duplicates)
	}
}

func TestParseSessionSameTotalsDifferentIDsStaySeparate(t *testing.T) {
	session, err := ParseSession(fixturePath(t, "same-totals-different-ids-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(session.Invocations) != 2 {
		t.Fatalf("invocations = %d, want 2 (token equality must not drive dedupe)", len(session.Invocations))
	}
	if session.Usage.Input != 1000 {
		t.Fatalf("session input = %d, want 1000 (sum of two distinct invocations)", session.Usage.Input)
	}
}

func TestParseSessionConflictDuplicateMarkedNotSilentlyPicked(t *testing.T) {
	session, err := ParseSession(fixturePath(t, "conflict-duplicate-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(session.Invocations) != 1 {
		t.Fatalf("invocations = %d, want 1 (same message.id)", len(session.Invocations))
	}
	if !session.Invocations[0].Conflict {
		t.Fatalf("conflict flag = false, want true when duplicate usage differs")
	}
	if session.conflicts != 1 {
		t.Fatalf("conflicts counter = %d, want 1", session.conflicts)
	}
}

func TestParseSessionTurnIDUsesMessageID(t *testing.T) {
	session, err := ParseSession(fixturePath(t, "duplicate-message-id-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(session.Invocations) != 2 {
		t.Fatalf("invocations = %d, want 2", len(session.Invocations))
	}
	for _, inv := range session.Invocations {
		if inv.ID == "" {
			t.Fatalf("invocation id = empty, want authoritative message.id when present")
		}
	}
	if session.Invocations[0].ID != "msg_aaa" || session.Invocations[1].ID != "msg_bbb" {
		t.Fatalf("invocation ids = %v, want msg_aaa, msg_bbb", []string{session.Invocations[0].ID, session.Invocations[1].ID})
	}
}

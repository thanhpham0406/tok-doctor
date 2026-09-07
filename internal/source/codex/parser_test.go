package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func fixturePath(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "fixtures", "codex", rel)
}

func TestParseSessionUsageBasic(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "basic-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Input != 1200 || snap.Cached != 300 || snap.Output != 450 || snap.Total != 1950 {
		t.Fatalf("snapshot = %+v, want input=1200 cached=300 output=450 total=1950", snap)
	}
}

func TestParseSessionUsageTakesFinalSnapshot(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "multi-snapshot-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Input != 2400 || snap.Cached != 800 || snap.Output != 1100 || snap.Reasoning != 150 || snap.Total != 4450 {
		t.Fatalf("snapshot = %+v, want final cumulative totals", snap)
	}
}

func TestParseSessionUsageNoUsage(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "no-usage-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Total != 0 || snap.Input != 0 {
		t.Fatalf("snapshot = %+v, want zero", snap)
	}
}

func TestParseSessionUsageSkipsMalformedLines(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "malformed-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Total != 1200 || snap.Input != 900 {
		t.Fatalf("snapshot = %+v, want input=900 total=1200", snap)
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
}

func TestUsageSnapshotToModelUsage(t *testing.T) {
	u := UsageSnapshot{Input: 10, Cached: 5, Output: 3, Total: 18}.ToModelUsage()
	if u.Input != 10 || u.Cached != 5 || u.Output != 3 || u.Total != 18 {
		t.Fatalf("usage = %+v", u)
	}
	if u.Confidence != model.ConfidenceMeasured {
		t.Fatalf("confidence = %q, want measured", u.Confidence)
	}
}

func TestUsageSnapshotZeroHasEmptyConfidence(t *testing.T) {
	u := UsageSnapshot{}.ToModelUsage()
	if u.Confidence != "" {
		t.Fatalf("confidence = %q, want empty for zero snapshot", u.Confidence)
	}
}

func TestSumSnapshotsAggregates(t *testing.T) {
	snaps := []UsageSnapshot{
		{Input: 10, Cached: 5, Output: 3, Total: 18},
		{Input: 20, Cached: 2, Output: 4, Total: 26},
	}
	u := SumSnapshots(snaps)
	if u.Input != 30 || u.Cached != 7 || u.Output != 7 || u.Total != 44 {
		t.Fatalf("usage = %+v, want input=30 cached=7 output=7 total=44", u)
	}
}

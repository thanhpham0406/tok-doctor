package codex

import (
	"os"
	"path/filepath"
	"strings"
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
	if !snap.HasUsage {
		t.Fatal("HasUsage = false, want true")
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
	if snap.Input != 2400 || snap.Cached != 800 || snap.Output != 1100 || snap.Total != 4450 {
		t.Fatalf("snapshot = %+v, want final cumulative totals", snap)
	}
	if snap.Reasoning == nil || *snap.Reasoning != 150 {
		t.Fatalf("reasoning = %v, want pointer to 150", snap.Reasoning)
	}
}

func TestParseSessionUsageCumulativeReasoningNotDoubled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	lines := []string{
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":100,"reasoning_output_tokens":100,"total_tokens":200},"last_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":100,"reasoning_output_tokens":100,"total_tokens":200}}}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":300,"cached_input_tokens":0,"output_tokens":300,"reasoning_output_tokens":250,"total_tokens":600},"last_token_usage":{"input_tokens":200,"cached_input_tokens":0,"output_tokens":200,"reasoning_output_tokens":150,"total_tokens":400}}}}`,
	}
	if err := os.WriteFile(path, []byte(lines[0]+"\n"+lines[1]+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	snap, err := ParseSessionUsage(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Reasoning == nil || *snap.Reasoning != 250 {
		t.Fatalf("reasoning = %v, want pointer to 250 (final cumulative, not sum 350)", snap.Reasoning)
	}
	if snap.Total != 600 {
		t.Fatalf("total = %d, want 600 (final cumulative)", snap.Total)
	}
}

func TestParseSessionUsageMissingReasoningIsNil(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "missing-reasoning-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Reasoning != nil {
		t.Fatalf("reasoning = %v, want nil when source did not expose reasoning_output_tokens", *snap.Reasoning)
	}
	if snap.Total != 800 {
		t.Fatalf("total = %d, want 800", snap.Total)
	}
}

func TestParseSessionUsageExplicitZeroReasoning(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "basic-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.Reasoning == nil {
		t.Fatal("reasoning = nil, want pointer to 0 (source reported explicit 0)")
	}
	if *snap.Reasoning != 0 {
		t.Fatalf("reasoning = %d, want 0", *snap.Reasoning)
	}
}

func TestParseSessionUsageNoUsage(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "no-usage-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.HasUsage {
		t.Fatal("HasUsage = true, want false")
	}
	if snap.Total != 0 || snap.Input != 0 {
		t.Fatalf("snapshot = %+v, want zero", snap)
	}
}

func TestParseSessionUsageExplicitZero(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "explicit-zero-usage-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !snap.HasUsage {
		t.Fatal("HasUsage = false, want true for explicit token_count")
	}
	u := snap.ToModelUsage()
	if !u.HasAuthoritativeUsage() {
		t.Fatal("model usage should be authoritative for explicit zero token_count")
	}
	if u.Input.ValueOrZero() != 0 || u.Cached.ValueOrZero() != 0 || u.Output.ValueOrZero() != 0 || u.Total.ValueOrZero() != 0 {
		t.Fatalf("usage = %+v, want explicit zero values", u)
	}
	if u.Reasoning.Value == nil || u.Reasoning.ValueOrZero() != 0 {
		t.Fatalf("reasoning = %v, want pointer to explicit zero", u.Reasoning)
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
	r := int64(10)
	u := UsageSnapshot{Input: 10, Cached: 5, Output: 3, Reasoning: &r, Total: 18}.ToModelUsage()
	if u.Input.ValueOrZero() != 10 || u.Cached.ValueOrZero() != 5 || u.Output.ValueOrZero() != 3 || u.Total.ValueOrZero() != 18 {
		t.Fatalf("usage = %+v", u)
	}
	if u.Reasoning.Value == nil || u.Reasoning.ValueOrZero() != 10 {
		t.Fatalf("reasoning = %v, want pointer to 10", u.Reasoning)
	}
	if u.Total.Kind != model.MeasurementMeasured {
		t.Fatalf("total kind = %q, want measured", u.Total.Kind)
	}
	if u.Billable == nil || u.Billable.Input.ValueOrZero() != 10 || u.Billable.CacheRead.ValueOrZero() != 5 || u.Billable.Output.ValueOrZero() != 3 {
		t.Fatalf("billable = %+v, want codex input/cache/output", u.Billable)
	}
	if u.Billable.CacheWrite.Available() {
		t.Fatalf("cache write = %+v, want unavailable for Codex snapshot", u.Billable.CacheWrite)
	}
}

func TestUsageSnapshotZeroIsUnavailableWithoutPresence(t *testing.T) {
	u := UsageSnapshot{}.ToModelUsage()
	if u.HasUsage() {
		t.Fatalf("usage = %+v, want unavailable for zero snapshot without presence", u)
	}
}

func TestUsageSnapshotExplicitZeroIsMeasured(t *testing.T) {
	zero := int64(0)
	u := UsageSnapshot{Reasoning: &zero, HasUsage: true}.ToModelUsage()
	if u.Total.Kind != model.MeasurementMeasured {
		t.Fatalf("total kind = %q, want measured", u.Total.Kind)
	}
	if !u.HasAuthoritativeUsage() {
		t.Fatal("explicit zero usage should be authoritative")
	}
}

func TestUsageSnapshotMissingReasoningStaysMissing(t *testing.T) {
	u := UsageSnapshot{Input: 10, Cached: 5, Output: 3, Total: 18}.ToModelUsage()
	if u.Reasoning.Value != nil {
		t.Fatalf("reasoning = %v, want unavailable when snapshot did not expose reasoning", u.Reasoning.ValueOrZero())
	}
}

func TestUsageSnapshotExplicitZeroReasoningPreserved(t *testing.T) {
	zero := int64(0)
	u := UsageSnapshot{Input: 10, Cached: 5, Output: 3, Reasoning: &zero, Total: 18}.ToModelUsage()
	if u.Reasoning.Value == nil || u.Reasoning.ValueOrZero() != 0 {
		t.Fatalf("reasoning = %v, want pointer to 0 (explicit zero)", u.Reasoning)
	}
}

func TestSumSnapshotsAggregates(t *testing.T) {
	snaps := []UsageSnapshot{
		{Input: 10, Cached: 5, Output: 3, Total: 18},
		{Input: 20, Cached: 2, Output: 4, Total: 26},
	}
	u := SumSnapshots(snaps)
	if u.Input.ValueOrZero() != 30 || u.Cached.ValueOrZero() != 7 || u.Output.ValueOrZero() != 7 || u.Total.ValueOrZero() != 44 {
		t.Fatalf("usage = %+v, want input=30 cached=7 output=7 total=44", u)
	}
	if u.Total.Kind != model.MeasurementDerived {
		t.Fatalf("total kind = %q, want derived", u.Total.Kind)
	}
}

func TestSumSnapshotsAggregatesReasoning(t *testing.T) {
	r1 := int64(4)
	r2 := int64(6)
	snaps := []UsageSnapshot{
		{Input: 10, Cached: 5, Output: 3, Reasoning: &r1, Total: 22},
		{Input: 20, Cached: 2, Output: 4, Reasoning: &r2, Total: 32},
	}
	u := SumSnapshots(snaps)
	if u.Reasoning.Value == nil || u.Reasoning.ValueOrZero() != 10 {
		t.Fatalf("reasoning = %v, want pointer to 10", u.Reasoning)
	}
	if u.Total.ValueOrZero() != 54 {
		t.Fatalf("total = %d, want 54", u.Total.ValueOrZero())
	}
}

func TestSumSnapshotsPreservesReasoningAvailability(t *testing.T) {
	r := int64(50)
	snaps := []UsageSnapshot{
		{Input: 10, Cached: 5, Output: 3, Total: 18},
		{Input: 20, Cached: 2, Output: 4, Reasoning: &r, Total: 26},
	}
	u := SumSnapshots(snaps)
	if u.Reasoning.Value == nil || u.Reasoning.ValueOrZero() != 50 {
		t.Fatalf("reasoning = %v, want pointer to 50 (single source reported it)", u.Reasoning)
	}
}

func TestParseSessionClassifiesContextProvenance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	lines := []string{
		`{"type":"session_meta","payload":{"id":"ctx"}}`,
		`{"type":"response_item","payload":{"type":"message","id":"dev","role":"developer","content":[{"type":"input_text","text":"Follow instructions."}]}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"Read a file."}}`,
		`{"type":"response_item","payload":{"type":"function_call","id":"call","call_id":"call","name":"functions.exec_command","arguments":"{\"cmd\":\"cat internal/model/model.go\"}"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","id":"out","call_id":"call","output":"synthetic tool result"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":10,"reasoning_output_tokens":0,"total_tokens":110}}}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(parsed.Snapshots))
	}
	components := parsed.Snapshots[0].Context
	wantKinds := map[model.ContextComponentKind]bool{
		model.ContextInstructions: false,
		model.ContextUserPrompt:   false,
		model.ContextFile:         false,
		model.ContextToolResult:   false,
	}
	for _, component := range components {
		wantKinds[component.Kind] = true
		if component.Observation != model.ContextObservedByAgent {
			t.Fatalf("component observation = %q, want agent", component.Observation)
		}
		if component.Kind != model.ContextFile && component.Measurement.Kind != model.MeasurementEstimated {
			t.Fatalf("component = %+v, want estimated non-file measurement", component)
		}
		if component.Kind == model.ContextFile && (component.Path != "internal/model/model.go" || component.Measurement.DisplayKind() != model.MeasurementUnknown) {
			t.Fatalf("file component = %+v, want path with unknown tokens", component)
		}
	}
	for kind, seen := range wantKinds {
		if !seen {
			t.Fatalf("missing context kind %q in %+v", kind, components)
		}
	}
}

func TestParseSessionContextDedupesStableRecordWithinTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	lines := []string{
		`{"type":"response_item","payload":{"type":"message","id":"same","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
		`{"type":"response_item","payload":{"type":"message","id":"same","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"cached_input_tokens":0,"output_tokens":1,"reasoning_output_tokens":0,"total_tokens":11}}}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := len(parsed.Snapshots[0].Context); got != 1 {
		t.Fatalf("components = %d, want duplicate stable record collapsed", got)
	}
}

func TestParseSessionRepeatedFileAcrossTurnsRemainsVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	lines := []string{
		`{"type":"response_item","payload":{"type":"function_call","id":"call-1","call_id":"call-1","name":"functions.exec_command","arguments":"{\"cmd\":\"cat AGENTS.md\"}"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"cached_input_tokens":0,"output_tokens":1,"reasoning_output_tokens":0,"total_tokens":11}}}}`,
		`{"type":"response_item","payload":{"type":"function_call","id":"call-2","call_id":"call-2","name":"functions.exec_command","arguments":"{\"cmd\":\"cat AGENTS.md\"}"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":20,"cached_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":0,"total_tokens":22}}}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for i, snap := range parsed.Snapshots {
		if len(snap.Context) != 1 || snap.Context[0].Path != "AGENTS.md" {
			t.Fatalf("turn %d context = %+v, want repeated AGENTS.md file component", i+1, snap.Context)
		}
	}
}

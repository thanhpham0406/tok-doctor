package claude

import (
	"reflect"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func parseInlineSession(t *testing.T, lines ...string) ParsedSession {
	t.Helper()
	session, err := parseSession(strings.NewReader(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatalf("parse inline session: %v", err)
	}
	return session
}

func inlineAssistant(usage string) string {
	return `{"type":"assistant","message":{"id":"msg_inline","role":"assistant","model":"m","content":[{"type":"text","text":"synthetic"}],"usage":` + usage + `}}`
}

func TestSnapshotFullUsagePresence(t *testing.T) {
	session := parseInlineSession(t, inlineAssistant(`{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}`))
	snap := session.Usage
	if !snap.InputPresent || !snap.OutputPresent || !snap.CacheReadPresent || !snap.CacheWritePresent {
		t.Fatalf("presence = %+v, want all four fields present", snap)
	}
	if snap.Input != 1 || snap.Output != 2 || snap.CacheRead != 3 || snap.CacheWrite != 4 {
		t.Fatalf("snapshot = %+v, want values preserved", snap)
	}
}

func TestSnapshotExplicitZeroStaysPresent(t *testing.T) {
	session := parseInlineSession(t, inlineAssistant(`{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`))
	snap := session.Usage
	if !snap.InputPresent || !snap.OutputPresent || !snap.CacheReadPresent || !snap.CacheWritePresent {
		t.Fatalf("presence = %+v, want explicit zero to keep presence", snap)
	}
	usage := snap.ToModelUsage()
	for name, metric := range map[string]model.Measurement{
		"input":  usage.Input,
		"output": usage.Output,
	} {
		if metric.Value == nil || *metric.Value != 0 || metric.Kind != model.MeasurementMeasured {
			t.Fatalf("%s = %+v, want measured explicit zero", name, metric)
		}
	}
}

func TestSnapshotEmptyUsageObjectIsAbsent(t *testing.T) {
	session := parseInlineSession(t, inlineAssistant(`{}`))
	if session.Usage.HasUsage {
		t.Fatalf("HasUsage = true, want false for empty usage object")
	}
	if session.Usage.InputPresent || session.Usage.OutputPresent || session.Usage.CacheReadPresent || session.Usage.CacheWritePresent {
		t.Fatalf("presence = %+v, want no fields present", session.Usage)
	}
}

func TestSnapshotNilUsageIsAbsent(t *testing.T) {
	session := parseInlineSession(t, `{"type":"assistant","message":{"id":"msg_nil","role":"assistant","model":"m","content":[{"type":"text","text":"synthetic"}]}}`)
	if session.Usage.HasUsage {
		t.Fatalf("HasUsage = true, want false for nil usage")
	}
}

func TestSnapshotInputOnlyDoesNotInventTotals(t *testing.T) {
	session := parseInlineSession(t, inlineAssistant(`{"input_tokens":7}`))
	snap := session.Usage
	if !snap.InputPresent || snap.OutputPresent || snap.CacheReadPresent || snap.CacheWritePresent {
		t.Fatalf("presence = %+v, want input only", snap)
	}
	if _, ok := snap.TotalValue(); ok {
		t.Fatal("total was invented without all operands")
	}
	usage := snap.ToModelUsage()
	if usage.Input.ValueOrZero() != 7 || usage.Input.Kind != model.MeasurementMeasured {
		t.Fatalf("input = %+v, want measured 7", usage.Input)
	}
	if usage.Cached.Value != nil || usage.Output.Value != nil || usage.Total.Value != nil {
		t.Fatalf("usage = %+v, want missing cache/output/total", usage)
	}
}

func TestSnapshotOutputOnlyDoesNotInventTotals(t *testing.T) {
	session := parseInlineSession(t, inlineAssistant(`{"output_tokens":9}`))
	snap := session.Usage
	if snap.InputPresent || !snap.OutputPresent || snap.CacheReadPresent || snap.CacheWritePresent {
		t.Fatalf("presence = %+v, want output only", snap)
	}
	if _, ok := snap.TotalValue(); ok {
		t.Fatal("total was invented without all operands")
	}
	usage := snap.ToModelUsage()
	if usage.Output.ValueOrZero() != 9 || usage.Output.Kind != model.MeasurementMeasured {
		t.Fatalf("output = %+v, want measured 9", usage.Output)
	}
	if usage.Input.Value != nil || usage.Cached.Value != nil || usage.Total.Value != nil {
		t.Fatalf("usage = %+v, want missing input/cache/total", usage)
	}
}

func TestSnapshotCacheReadOnlyKeepsSplitAvailability(t *testing.T) {
	session := parseInlineSession(t, inlineAssistant(`{"cache_read_input_tokens":11}`))
	snap := session.Usage
	if snap.InputPresent || snap.OutputPresent || !snap.CacheReadPresent || snap.CacheWritePresent {
		t.Fatalf("presence = %+v, want cache read only", snap)
	}
	if _, ok := snap.CachedValue(); ok {
		t.Fatal("combined cached was invented without cache creation")
	}
	usage := snap.ToModelUsageWithEvidence("msg_cache")
	if usage.Billable == nil || usage.Billable.CacheRead.ValueOrZero() != 11 {
		t.Fatalf("billable = %+v, want cache read 11", usage.Billable)
	}
	if usage.Billable.CacheWrite.Value != nil {
		t.Fatalf("billable cache write = %+v, want missing", usage.Billable.CacheWrite)
	}
}

func TestSnapshotDirectFieldEvidence(t *testing.T) {
	session := parseInlineSession(t, inlineAssistant(`{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}`))
	usage := session.Invocations[0].Snapshot.ToModelUsageWithEvidence("msg_inline")

	cases := []struct {
		metric model.Measurement
		field  string
	}{
		{usage.Input, claudeFieldInputTokens},
		{usage.Output, claudeFieldOutputTokens},
		{usage.Billable.CacheRead, claudeFieldCacheRead},
		{usage.Billable.CacheWrite, claudeFieldCacheCreation},
	}
	for _, tc := range cases {
		if len(tc.metric.Evidence) != 1 {
			t.Fatalf("%s evidence = %+v, want one source_value", tc.field, tc.metric.Evidence)
		}
		want := model.Evidence{Kind: model.EvidenceSourceValue, Source: "claude_session", Record: "msg_inline", Field: tc.field}
		if !reflect.DeepEqual(tc.metric.Evidence[0], want) {
			t.Fatalf("evidence = %+v, want %+v", tc.metric.Evidence[0], want)
		}
		if !tc.metric.Evidence[0].ConsistentWithKind(tc.metric.Kind) {
			t.Fatalf("%s evidence inconsistent with kind %q", tc.field, tc.metric.Kind)
		}
	}
}

func TestSnapshotComputedSumsAreDerived(t *testing.T) {
	session := parseInlineSession(t, inlineAssistant(`{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}`))
	usage := session.Invocations[0].Snapshot.ToModelUsageWithEvidence("msg_inline")

	for name, metric := range map[string]model.Measurement{
		"cached":        usage.Cached,
		"total":         usage.Total,
		"billableInput": usage.Billable.Input,
	} {
		if metric.Kind != model.MeasurementDerived {
			t.Fatalf("%s kind = %q, want derived", name, metric.Kind)
		}
		if len(metric.Evidence) != 1 || metric.Evidence[0].Kind != model.EvidenceAggregate {
			t.Fatalf("%s evidence = %+v, want aggregate", name, metric.Evidence)
		}
	}
}

func TestSumSnapshotsPreservesPresence(t *testing.T) {
	snaps := []UsageSnapshot{
		{Input: 5, InputPresent: true, Output: 3, OutputPresent: true, HasUsage: true},
		{Input: 0, InputPresent: true, Output: 0, OutputPresent: true, HasUsage: true},
	}
	u := SumSnapshots(snaps)
	if u.Input.ValueOrZero() != 5 || u.Input.Kind != model.MeasurementDerived {
		t.Fatalf("input = %+v, want derived 5", u.Input)
	}
	if u.Output.ValueOrZero() != 3 || u.Output.Kind != model.MeasurementDerived {
		t.Fatalf("output = %+v, want derived 3", u.Output)
	}
	if u.Cached.Value != nil {
		t.Fatalf("cached = %+v, want missing when no snapshot reported cache fields", u.Cached)
	}
	if u.Total.Value != nil {
		t.Fatalf("total = %+v, want missing when components are absent", u.Total)
	}
	for name, metric := range map[string]model.Measurement{"input": u.Input, "output": u.Output} {
		if len(metric.Evidence) != 1 || metric.Evidence[0].Kind != model.EvidenceAggregate {
			t.Fatalf("%s evidence = %+v, want aggregate", name, metric.Evidence)
		}
	}
}

func TestSumSnapshotsExplicitZeroRemainsPresent(t *testing.T) {
	snaps := []UsageSnapshot{
		{Input: 0, InputPresent: true, Output: 0, OutputPresent: true, CacheRead: 0, CacheReadPresent: true, CacheWrite: 0, CacheWritePresent: true, HasUsage: true},
	}
	u := SumSnapshots(snaps)
	if u.Input.Value == nil || *u.Input.Value != 0 {
		t.Fatalf("input = %+v, want explicit zero", u.Input)
	}
	if u.Total.Value == nil || *u.Total.Value != 0 {
		t.Fatalf("total = %+v, want explicit zero", u.Total)
	}
}

func TestSumSnapshotsEmptyInputStaysMissing(t *testing.T) {
	u := SumSnapshots([]UsageSnapshot{{HasUsage: true}})
	if u.HasUsage() {
		t.Fatalf("usage = %+v, want no usable fields", u)
	}
}

func TestNegativeRawUsageNeverEmitsNegativeTokens(t *testing.T) {
	snaps := []UsageSnapshot{
		{Input: -1, InputPresent: true, Output: -2, OutputPresent: true, CacheRead: -3, CacheReadPresent: true, CacheWrite: -4, CacheWritePresent: true, HasUsage: true},
	}
	usage := snaps[0].ToModelUsage()
	for name, metric := range map[string]model.Measurement{
		"input":  usage.Input,
		"output": usage.Output,
		"cached": usage.Cached,
		"total":  usage.Total,
	} {
		if metric.Value != nil {
			t.Fatalf("%s = %+v, want no negative value", name, metric)
		}
	}
	sum := SumSnapshots(snaps)
	if sum.Input.Value != nil || sum.Output.Value != nil || sum.Cached.Value != nil || sum.Total.Value != nil {
		t.Fatalf("sum = %+v, want no numeric usage from negative input", sum)
	}
}

func TestParseNegativeUsageFixture(t *testing.T) {
	snap, err := ParseSessionUsage(fixturePath(t, "negative-usage-session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !snap.HasUsage {
		t.Fatal("HasUsage = false, want true because fields were reported")
	}
	if snap.HasPositiveUsage() {
		t.Fatal("HasPositiveUsage = true, want false for negative raw usage")
	}
	usage := snap.ToModelUsage()
	for name, metric := range map[string]model.Measurement{
		"input":  usage.Input,
		"output": usage.Output,
		"cached": usage.Cached,
		"total":  usage.Total,
	} {
		if metric.Value != nil {
			t.Fatalf("%s = %+v, want nil value", name, metric)
		}
	}
}

func fullSnapshot(input, cacheRead, cacheWrite, output int64) UsageSnapshot {
	return UsageSnapshot{
		Input: input, InputPresent: true,
		CacheRead: cacheRead, CacheReadPresent: true,
		CacheWrite: cacheWrite, CacheWritePresent: true,
		Output: output, OutputPresent: true,
		HasUsage: true,
	}
}

func TestSumSnapshotsRequiresFieldInEverySnapshot(t *testing.T) {
	snaps := []UsageSnapshot{
		{Input: 5, InputPresent: true, HasUsage: true},
		{InputPresent: false, CacheRead: 2, CacheReadPresent: true, CacheWrite: 3, CacheWritePresent: true, Output: 4, OutputPresent: true, HasUsage: true},
	}
	u := SumSnapshots(snaps)
	if u.Input.Value != nil {
		t.Fatalf("input = %+v, want no full numeric input from partial turns", u.Input)
	}
	if u.Cached.Value != nil {
		t.Fatalf("cached = %+v, want missing", u.Cached)
	}
	if u.Total.Value != nil {
		t.Fatalf("total = %+v, want missing", u.Total)
	}
}

func TestSumSnapshotsFullSnapshotsAggregate(t *testing.T) {
	snaps := []UsageSnapshot{
		fullSnapshot(10, 1, 2, 3),
		fullSnapshot(20, 4, 5, 6),
	}
	u := SumSnapshots(snaps)
	if u.Input.ValueOrZero() != 30 || u.Cached.ValueOrZero() != 12 || u.Output.ValueOrZero() != 9 || u.Total.ValueOrZero() != 51 {
		t.Fatalf("usage = %+v, want input=30 cached=12 output=9 total=51", u)
	}
	if u.Total.Kind != model.MeasurementDerived {
		t.Fatalf("total kind = %q, want derived", u.Total.Kind)
	}
}

func TestSumSnapshotsMissingOutputKeepsTotalMissing(t *testing.T) {
	snaps := []UsageSnapshot{
		fullSnapshot(10, 1, 2, 3),
		{Input: 3, InputPresent: true, CacheRead: 1, CacheReadPresent: true, CacheWrite: 1, CacheWritePresent: true, HasUsage: true},
	}
	u := SumSnapshots(snaps)
	if u.Output.Value != nil {
		t.Fatalf("output = %+v, want unavailable", u.Output)
	}
	if u.Total.Value != nil {
		t.Fatalf("total = %+v, want missing", u.Total)
	}
}

func TestSumSnapshotsNegativeInputIsNotAggregated(t *testing.T) {
	snaps := []UsageSnapshot{
		fullSnapshot(10, 1, 2, 3),
		{Input: -1, InputPresent: true, CacheRead: 1, CacheReadPresent: true, CacheWrite: 1, CacheWritePresent: true, Output: 2, OutputPresent: true, HasUsage: true},
	}
	u := SumSnapshots(snaps)
	if u.Input.Value != nil {
		t.Fatalf("input = %+v, want no aggregate from mixed negative input", u.Input)
	}
	if u.Total.Value != nil {
		t.Fatalf("total = %+v, want missing", u.Total)
	}
}

package coverage

import (
	"strconv"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func measuredTurn(sequence int, components ...model.ContextComponent) model.Turn {
	usage := model.MeasuredUsage(1000, 400, 150, model.Int64(0), 1150)
	return model.Turn{
		ID:       "sess#" + strconv.Itoa(sequence),
		Sequence: sequence,
		Usage:    usage,
		ContextAttribution: model.ContextAttribution{
			Components: components,
			Input:      model.CodexInputAccounting(usage),
		},
	}
}

func toolOutput(component model.ContextComponent) model.ContextComponent {
	component.Kind = model.ContextToolResult
	return component
}

func fullToolOutput(bytes int64, tokens int64) model.ContextComponent {
	return toolOutput(model.ContextComponent{
		ContentBytes: model.Int64(bytes),
		ToolCallID:   "call-1",
		ToolName:     "exec_command",
		Measurement:  model.NewMeasurement(tokens, model.MeasurementEstimated),
		Completeness: model.ContextCompletenessComplete,
	})
}

func TestScanSessionWithoutToolOutputKeepsRatiosUndefined(t *testing.T) {
	summary := Scan([]model.Session{{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1, model.ContextComponent{
			Kind:        model.ContextUserPrompt,
			Measurement: model.NewMeasurement(10, model.MeasurementEstimated),
		})},
	}})

	if summary.Sessions != 1 || summary.Turns != 1 {
		t.Fatalf("scanned = %d sessions, %d turns, want 1 and 1", summary.Sessions, summary.Turns)
	}
	if summary.ToolOutputs != 0 {
		t.Fatalf("tool outputs = %d, want 0", summary.ToolOutputs)
	}
	ratios := map[string]Ratio{
		"bytes":       summary.WithContentBytes,
		"tokens":      summary.WithEstimatedTokens,
		"call ids":    summary.WithToolCallID,
		"tool names":  summary.WithToolName,
		"fresh input": summary.WithFreshInput,
	}
	for name, ratio := range ratios {
		if ratio.Value != nil || ratio.Total != 0 || ratio.Count != 0 {
			t.Fatalf("%s ratio = %+v, want undefined over zero", name, ratio)
		}
	}
	if summary.ContentBytes.Samples != 0 || summary.ContentBytes.Min != nil || summary.ContentBytes.P50 != nil {
		t.Fatalf("content bytes = %+v, want no samples", summary.ContentBytes)
	}
	if summary.EstimatedTokens.Kind != model.MeasurementUnknown || summary.EstimatedTokens.Value != nil {
		t.Fatalf("estimated tokens = %+v, want unknown", summary.EstimatedTokens)
	}
}

func TestScanCountsCompleteMetadata(t *testing.T) {
	summary := Scan([]model.Session{{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1,
			fullToolOutput(100, 25),
			fullToolOutput(300, 75),
		)},
	}})

	if summary.ToolOutputs != 2 {
		t.Fatalf("tool outputs = %d, want 2", summary.ToolOutputs)
	}
	for name, ratio := range map[string]Ratio{
		"bytes":       summary.WithContentBytes,
		"tokens":      summary.WithEstimatedTokens,
		"call ids":    summary.WithToolCallID,
		"tool names":  summary.WithToolName,
		"fresh input": summary.WithFreshInput,
	} {
		if ratio.Value == nil || *ratio.Value != 1 {
			t.Fatalf("%s ratio = %+v, want fully covered", name, ratio)
		}
	}
	if summary.RecognizedToolOutputCompleteness.Complete != 2 {
		t.Fatalf("complete = %d, want 2", summary.RecognizedToolOutputCompleteness.Complete)
	}
	if summary.ContentBytes.Samples != 2 || summary.ContentBytes.Total != 400 {
		t.Fatalf("content bytes = %+v, want 2 samples over 400 bytes", summary.ContentBytes)
	}
	if summary.EstimatedTokens.Kind != model.MeasurementEstimated || summary.EstimatedTokens.ValueOrZero() != 100 {
		t.Fatalf("estimated tokens = %+v, want 100 estimated", summary.EstimatedTokens)
	}
}

func TestScanCountsMissingMetadata(t *testing.T) {
	summary := Scan([]model.Session{{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1,
			fullToolOutput(100, 25),
			toolOutput(model.ContextComponent{
				Measurement:  model.NewMeasurement(10, model.MeasurementEstimated),
				Completeness: model.ContextCompletenessComplete,
			}),
			toolOutput(model.ContextComponent{
				ToolCallID:   "call-3",
				Measurement:  model.Measurement{Kind: model.MeasurementUnknown},
				Completeness: model.ContextCompletenessComplete,
			}),
		)},
	}})

	if summary.ToolOutputs != 3 {
		t.Fatalf("tool outputs = %d, want 3", summary.ToolOutputs)
	}
	if ratio := summary.WithContentBytes; ratio.Count != 1 || ratio.Total != 3 {
		t.Fatalf("with byte size = %+v, want 1 of 3", ratio)
	}
	if ratio := summary.WithToolCallID; ratio.Count != 2 || ratio.Total != 3 {
		t.Fatalf("with call id = %+v, want 2 of 3", ratio)
	}
	if ratio := summary.WithToolName; ratio.Count != 1 || ratio.Total != 3 {
		t.Fatalf("with tool name = %+v, want 1 of 3", ratio)
	}
	if ratio := summary.WithEstimatedTokens; ratio.Count != 2 || ratio.Total != 3 {
		t.Fatalf("with estimated tokens = %+v, want 2 of 3", ratio)
	}
	if summary.EstimatedTokens.ValueOrZero() != 35 {
		t.Fatalf("estimated tokens = %+v, want 35", summary.EstimatedTokens)
	}
}

func TestScanCountsEveryCompletenessState(t *testing.T) {
	component := func(completeness model.ContextCompleteness) model.ContextComponent {
		return toolOutput(model.ContextComponent{
			ContentBytes: model.Int64(10),
			Measurement:  model.NewMeasurement(3, model.MeasurementEstimated),
			Completeness: completeness,
		})
	}
	summary := Scan([]model.Session{{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1,
			component(model.ContextCompletenessComplete),
			component(model.ContextCompletenessTruncated),
			component(model.ContextCompletenessUnavailable),
			toolOutput(model.ContextComponent{ContentBytes: model.Int64(10)}),
		)},
	}})

	want := Completeness{Complete: 1, Truncated: 1, Unavailable: 1, Unknown: 1}
	if summary.RecognizedToolOutputCompleteness != want {
		t.Fatalf("completeness = %+v, want %+v", summary.RecognizedToolOutputCompleteness, want)
	}
	if summary.ToolOutputs != 4 {
		t.Fatalf("tool outputs = %d, want 4", summary.ToolOutputs)
	}
	if summary.UnreadableRecords != 2 {
		t.Fatalf("unreadable records = %d, want 2", summary.UnreadableRecords)
	}
	if summary.UnreadableSessions != 1 {
		t.Fatalf("unreadable sessions = %d, want 1", summary.UnreadableSessions)
	}
}

func TestScanCountsTrailingContextWithoutFreshInput(t *testing.T) {
	summary := Scan([]model.Session{{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1,
			fullToolOutput(100, 25),
		)},
		TrailingContext: []model.ContextComponent{
			fullToolOutput(100, 25),
			{Kind: model.ContextUserPrompt},
		},
	}})

	if summary.ToolOutputs != 2 || summary.TrailingContext != 1 {
		t.Fatalf("tool outputs = %d, trailing = %d, want 2 and 1", summary.ToolOutputs, summary.TrailingContext)
	}
	if ratio := summary.WithFreshInput; ratio.Count != 1 || ratio.Total != 2 {
		t.Fatalf("with fresh input = %+v, want 1 of 2", ratio)
	}
}

func TestScanLinksOnlyAuthoritativeFreshInput(t *testing.T) {
	estimatedUsage := model.Usage{Input: model.NewMeasurement(500, model.MeasurementEstimated)}
	unknownUsage := model.Usage{}
	summary := Scan([]model.Session{{
		ID: "sess-1",
		Turns: []model.Turn{
			measuredTurn(1, fullToolOutput(10, 3)),
			{
				ID:       "turn-2",
				Sequence: 2,
				Usage:    estimatedUsage,
				ContextAttribution: model.ContextAttribution{
					Components: []model.ContextComponent{fullToolOutput(10, 3)},
					Input:      model.SeparateInputAccounting(estimatedUsage),
				},
			},
			{
				ID:       "turn-3",
				Sequence: 3,
				Usage:    unknownUsage,
				ContextAttribution: model.ContextAttribution{
					Components: []model.ContextComponent{fullToolOutput(10, 3)},
					Input:      model.SeparateInputAccounting(unknownUsage),
				},
			},
		},
	}})

	if summary.ToolOutputs != 3 {
		t.Fatalf("tool outputs = %d, want 3", summary.ToolOutputs)
	}
	if ratio := summary.WithFreshInput; ratio.Count != 1 || ratio.Total != 3 {
		t.Fatalf("with fresh input = %+v, want 1 of 3", ratio)
	}
}

// unreadableRecord mirrors what an adapter emits when it cannot read a source
// record: an unknown-kind component with no measurement, tagged with the
// completeness the source stated.
func unreadableRecord(completeness model.ContextCompleteness) model.ContextComponent {
	return model.ContextComponent{
		Kind:         model.ContextUnknown,
		Source:       "codex_rollout",
		Record:       "codex_rollout#line:3",
		Measurement:  model.Measurement{Kind: model.MeasurementUnknown},
		Completeness: completeness,
	}
}

func TestScanKeepsUnreadableRecordsOutOfToolOutputCounts(t *testing.T) {
	summary := Scan([]model.Session{{
		ID:    "sess-1",
		Turns: []model.Turn{measuredTurn(1, fullToolOutput(100, 25))},
		TrailingContext: []model.ContextComponent{
			unreadableRecord(model.ContextCompletenessTruncated),
			unreadableRecord(model.ContextCompletenessUnavailable),
		},
	}})

	if summary.ToolOutputs != 1 {
		t.Fatalf("tool outputs = %d, want 1: an unreadable record is no evidence of a tool output", summary.ToolOutputs)
	}
	want := Completeness{Complete: 1}
	if summary.RecognizedToolOutputCompleteness != want {
		t.Fatalf("completeness = %+v, want %+v counted over recognized tool outputs only",
			summary.RecognizedToolOutputCompleteness, want)
	}
	if ratio := summary.WithContentBytes; ratio.Count != 1 || ratio.Total != 1 {
		t.Fatalf("with byte size = %+v, want 1 of 1", ratio)
	}
	if summary.UnreadableRecords != 2 || summary.UnreadableSessions != 1 {
		t.Fatalf("unreadable = %d records, %d sessions, want 2 and 1",
			summary.UnreadableRecords, summary.UnreadableSessions)
	}
}

func TestScanCountsSessionsWithoutAuthoritativeUsage(t *testing.T) {
	usage := model.Usage{}
	withoutUsage := model.Session{
		ID: "sess-2",
		Turns: []model.Turn{{
			ID:       "turn-1",
			Sequence: 1,
			Usage:    usage,
			ContextAttribution: model.ContextAttribution{
				Components: []model.ContextComponent{fullToolOutput(100, 25)},
				Input:      model.SeparateInputAccounting(usage),
			},
		}},
	}
	summary := Scan([]model.Session{
		{ID: "sess-1", Turns: []model.Turn{measuredTurn(1, fullToolOutput(100, 25))}},
		withoutUsage,
	})

	if summary.Sessions != 2 || summary.Turns != 2 {
		t.Fatalf("scanned = %d sessions, %d turns, want 2 and 2", summary.Sessions, summary.Turns)
	}
	if summary.ToolOutputs != 2 {
		t.Fatalf("tool outputs = %d, want 2: a session without usage still holds tool output", summary.ToolOutputs)
	}
	if summary.RecognizedToolOutputCompleteness.Complete != 2 {
		t.Fatalf("completeness = %+v, want both outputs counted", summary.RecognizedToolOutputCompleteness)
	}
	if ratio := summary.WithFreshInput; ratio.Count != 1 || ratio.Total != 2 {
		t.Fatalf("with fresh input = %+v, want 1 of 2: a turn without authoritative usage links nothing", ratio)
	}
}

func TestByteStatsPercentiles(t *testing.T) {
	cases := []struct {
		name   string
		sizes  []int64
		expect Bytes
	}{
		{
			name:   "no samples",
			sizes:  nil,
			expect: Bytes{},
		},
		{
			name:  "one sample",
			sizes: []int64{42},
			expect: Bytes{
				Total: 42, Samples: 1,
				Min: model.Int64(42), P50: model.Int64(42), P90: model.Int64(42),
				P95: model.Int64(42), Max: model.Int64(42),
			},
		},
		{
			name:  "many samples",
			sizes: []int64{10, 20, 30, 40},
			expect: Bytes{
				Total: 100, Samples: 4,
				Min: model.Int64(10), P50: model.Int64(20), P90: model.Int64(40),
				P95: model.Int64(40), Max: model.Int64(40),
			},
		},
		{
			name:  "unsorted samples keep nearest rank",
			sizes: []int64{9, 1, 5, 3, 7, 2, 4, 6, 8, 10},
			expect: Bytes{
				Total: 55, Samples: 10,
				Min: model.Int64(1), P50: model.Int64(5), P90: model.Int64(9),
				P95: model.Int64(10), Max: model.Int64(10),
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := byteStats(testCase.sizes)
			if got.Total != testCase.expect.Total || got.Samples != testCase.expect.Samples {
				t.Fatalf("total/samples = %d/%d, want %d/%d", got.Total, got.Samples, testCase.expect.Total, testCase.expect.Samples)
			}
			for name, pair := range map[string][2]*int64{
				"min": {got.Min, testCase.expect.Min},
				"p50": {got.P50, testCase.expect.P50},
				"p90": {got.P90, testCase.expect.P90},
				"p95": {got.P95, testCase.expect.P95},
				"max": {got.Max, testCase.expect.Max},
			} {
				if !equalInt64(pair[0], pair[1]) {
					t.Fatalf("%s = %v, want %v", name, pair[0], pair[1])
				}
			}
		})
	}
}

func TestScanAggregatesSessions(t *testing.T) {
	first := model.Session{ID: "sess-1", Turns: []model.Turn{measuredTurn(1, fullToolOutput(100, 25))}}
	second := model.Session{ID: "sess-2", Turns: []model.Turn{
		measuredTurn(1, toolOutput(model.ContextComponent{
			ToolName:    "read_file",
			Measurement: model.NewMeasurement(50, model.MeasurementEstimated),
		})),
	}}

	combined := Scan([]model.Session{first, second})
	if combined.Sessions != 2 || combined.Turns != 2 || combined.ToolOutputs != 2 {
		t.Fatalf("combined = %d sessions, %d turns, %d tool outputs, want 2 each",
			combined.Sessions, combined.Turns, combined.ToolOutputs)
	}
	if ratio := combined.WithContentBytes; ratio.Count != 1 || ratio.Total != 2 {
		t.Fatalf("with byte size = %+v, want 1 of 2", ratio)
	}
	if ratio := combined.WithToolName; ratio.Count != 2 || ratio.Total != 2 {
		t.Fatalf("with tool name = %+v, want 2 of 2", ratio)
	}
	if combined.EstimatedTokens.ValueOrZero() != 75 {
		t.Fatalf("estimated tokens = %+v, want 75", combined.EstimatedTokens)
	}

	firstOnly := Scan([]model.Session{first})
	secondOnly := Scan([]model.Session{second})
	if firstOnly.ToolOutputs != 1 || secondOnly.ToolOutputs != 1 {
		t.Fatalf("per-session scans = %d and %d tool outputs, want 1 each",
			firstOnly.ToolOutputs, secondOnly.ToolOutputs)
	}
}

func equalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

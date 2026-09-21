package analyze

import (
	"context"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestAnalyzeHealthyResult(t *testing.T) {
	result := New().Analyze(context.Background(), model.Session{
		ID:    "test",
		Agent: model.AgentCodex,
	})

	if result.Source != "codex" {
		t.Fatalf("Source = %q, want codex", result.Source)
	}
	if result.Summary.Status != "healthy" {
		t.Fatalf("Status = %q, want healthy", result.Summary.Status)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("Findings len = %d, want 0", len(result.Findings))
	}
}

func TestAnalyzeRunsOversizedToolOutputRule(t *testing.T) {
	result := New().Analyze(context.Background(), model.Session{
		ID:    "test",
		Agent: model.AgentCodex,
		Turns: []model.Turn{{
			Sequence: 1,
			ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{{
				Kind: model.ContextToolResult, ContentBytes: model.Int64(65 * 1024),
				Completeness: model.ContextCompletenessComplete,
			}}},
		}},
	})
	if result.Summary.Status != "findings" || len(result.Findings) != 1 {
		t.Fatalf("result = %+v, want one finding", result)
	}
}

func TestAnalyzeRunsRepeatedToolCallRule(t *testing.T) {
	component := func(callID string) model.ContextComponent {
		return model.ContextComponent{
			Kind:                model.ContextToolResult,
			ToolCallID:          callID,
			ToolName:            "exec_command",
			ToolCallFingerprint: "v1:argument-fingerprint",
			ContentHash:         "sha256:output-digest",
			ContentBytes:        model.Int64(4096),
			Completeness:        model.ContextCompletenessComplete,
			Measurement:         model.NewMeasurement(512, model.MeasurementEstimated),
		}
	}

	result := New().Analyze(context.Background(), model.Session{
		ID:    "test",
		Agent: model.AgentCodex,
		Turns: []model.Turn{
			{Sequence: 1, ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{component("call-1")}}},
			{Sequence: 2, ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{component("call-2")}}},
		},
	})
	if result.Summary.Status != "findings" || len(result.Findings) != 1 {
		t.Fatalf("result = %+v, want the analyzer to emit the repeated call finding", result)
	}
	finding := result.Findings[0]
	if finding.RuleID != "TOOL002" || finding.Name != "repeated-tool-call" {
		t.Fatalf("finding = %s/%s, want TOOL002 repeated-tool-call", finding.RuleID, finding.Name)
	}
}

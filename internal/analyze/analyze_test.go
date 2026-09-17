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

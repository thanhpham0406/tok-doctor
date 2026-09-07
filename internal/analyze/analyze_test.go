package analyze

import (
	"context"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestAnalyzeScaffoldResult(t *testing.T) {
	result := New().Analyze(context.Background(), model.Session{
		ID:    "test",
		Agent: model.AgentCodex,
	})

	if result.Source != "codex" {
		t.Fatalf("Source = %q, want codex", result.Source)
	}
	if result.Summary.Status != "ready" {
		t.Fatalf("Status = %q, want ready", result.Summary.Status)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("Findings len = %d, want 0", len(result.Findings))
	}
}

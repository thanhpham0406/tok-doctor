package terminal

import (
	"bytes"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestRender(t *testing.T) {
	var out bytes.Buffer
	result := analyze.Result{
		Source:  "codex",
		Session: model.Session{ID: "scaffold"},
		Summary: analyze.Summary{Status: "ready", Message: "ok"},
	}

	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "Status: ready") {
		t.Fatalf("output = %q, want status", got)
	}
}

func TestRenderFinding(t *testing.T) {
	var out bytes.Buffer
	result := analyze.Result{
		Source:  "codex",
		Session: model.Session{ID: "session-1"},
		Summary: analyze.Summary{Status: "findings", Message: "one issue"},
		Findings: []model.Finding{{
			RuleID: "TOOL001", Severity: model.SeverityHigh,
			Title:          "exec_command returned an oversized output",
			Description:    "complete output",
			Evidence:       []model.FindingEvidence{{TurnSequence: 3, ToolName: "exec_command", OutputBytes: 128 * 1024, EstimatedTokens: model.NewMeasurement(32000, model.MeasurementEstimated)}},
			Recommendation: "filter it",
		}},
	}
	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := out.String()
	for _, want := range []string{"TOOL001", "exec_command", "128.0 KiB", "Estimated tokens: 32000", "filter it"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

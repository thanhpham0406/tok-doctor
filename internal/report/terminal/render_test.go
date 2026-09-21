package terminal

import (
	"bytes"
	"context"
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

// A repeated call finding reaches the terminal without the internal tool call
// fingerprint or the source arguments that produced it.
func TestRenderRepeatedToolCallFinding(t *testing.T) {
	const secretArguments = `{"cmd":"cat SECRET_ARGUMENT_LITERAL_DO_NOT_RETAIN"}`
	component := func(callID string) model.ContextComponent {
		return model.ContextComponent{
			Kind:                model.ContextToolResult,
			ToolCallID:          callID,
			ToolName:            "exec_command",
			ToolCallFingerprint: "v1:internal-argument-fingerprint",
			ContentHash:         "sha256:internal-output-digest",
			ContentBytes:        model.Int64(2048),
			Completeness:        model.ContextCompletenessComplete,
			Measurement:         model.NewMeasurement(512, model.MeasurementEstimated),
		}
	}

	result := analyze.New().Analyze(context.Background(), model.Session{
		ID:    "session-1",
		Agent: model.AgentCodex,
		Turns: []model.Turn{
			{Sequence: 1, ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{component("call-1")}}},
			{Sequence: 2, ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{component("call-2")}}},
		},
	})

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := out.String()
	for _, want := range []string{"TOOL002", "exec_command repeated the same tool call", "estimated repeated tool output", "Estimated tokens: 512"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
	for _, secret := range []string{"v1:internal-argument-fingerprint", "sha256:internal-output-digest", "internal-output-digest", secretArguments, "SECRET_ARGUMENT_LITERAL_DO_NOT_RETAIN", "call-1", "call-2"} {
		if strings.Contains(got, secret) {
			t.Fatalf("output exposes %q: %s", secret, got)
		}
	}
}

package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
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
			Evidence:       []model.FindingEvidence{{TurnSequence: 3, ToolName: "exec_command", OutputBytes: model.Int64(128 * 1024), EstimatedTokens: model.NewMeasurement(32000, model.MeasurementEstimated)}},
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
// fingerprint, the content hash of the tool output, the arguments the
// fingerprint was derived from, or the path the output was read from.
func TestRenderRepeatedToolCallFindingExposesNoInternalDigests(t *testing.T) {
	const (
		secretArguments = `{"cmd":"cat SECRET_COMMAND_LITERAL_DO_NOT_RETAIN"}`
		secretOutput    = "SECRET_TOOL_OUTPUT_LITERAL_DO_NOT_RETAIN"
		secretPath      = "/secret/path/SECRET_PATH_LITERAL_DO_NOT_RETAIN"
	)
	component := func(callID string) model.ContextComponent {
		return model.ContextComponent{
			Kind:                model.ContextToolResult,
			ToolCallID:          callID,
			ToolName:            "exec_command",
			ToolCallFingerprint: source.StructuredToolCallFingerprintOrEmpty("exec_command", json.RawMessage(secretArguments)),
			ContentHash:         source.ContentHash(secretOutput),
			Path:                secretPath,
			ContentBytes:        model.Int64(2048),
			Completeness:        model.ContextCompletenessComplete,
			Measurement:         model.NewMeasurement(512, model.MeasurementEstimated),
		}
	}
	first, second := component("call-1"), component("call-2")
	if first.ToolCallFingerprint == "" || first.ToolCallFingerprint != second.ToolCallFingerprint {
		t.Fatalf("fingerprint = %q/%q, want the same derived digest", first.ToolCallFingerprint, second.ToolCallFingerprint)
	}

	result := analyze.New().Analyze(context.Background(), model.Session{
		ID:    "session-1",
		Agent: model.AgentCodex,
		Turns: []model.Turn{
			{Sequence: 1, ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{first}}},
			{Sequence: 2, ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{second}}},
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
	for _, secret := range []string{first.ToolCallFingerprint, "v1:", first.ContentHash, "sha256:", secretOutput, secretArguments, secretPath, "call-1", "call-2"} {
		if strings.Contains(got, secret) {
			t.Fatalf("output exposes %q: %s", secret, got)
		}
	}
}

// An output whose size the source never reported must not be rendered as a zero.
func TestRenderFindingWithUnknownOutputBytes(t *testing.T) {
	result := analyze.Result{
		Source:  "codex",
		Session: model.Session{ID: "session-1"},
		Summary: analyze.Summary{Status: "findings", Message: "one issue"},
		Findings: []model.Finding{{
			RuleID: "TOOL002", Severity: model.SeverityMedium,
			Title:          "exec_command repeated the same tool call",
			Description:    "complete output",
			Evidence:       []model.FindingEvidence{{TurnSequence: 3, ToolName: "exec_command"}},
			Recommendation: "reuse the earlier result",
		}},
	}

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Output: -") {
		t.Fatalf("output = %q, want the missing size marked as unavailable", got)
	}
	for _, unwanted := range []string{"Output: 0 B", "0x", "&{"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("output = %q, must not render %q", got, unwanted)
		}
	}
}

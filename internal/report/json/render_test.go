package json

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestRender(t *testing.T) {
	var out bytes.Buffer

	if err := Render(&out, analyze.Result{Source: "codex"}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var decoded analyze.Result
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal rendered JSON: %v", err)
	}
	if decoded.Source != "codex" {
		t.Fatalf("Source = %q, want codex", decoded.Source)
	}
}

// The internal tool call fingerprint is never part of the rendered JSON, and a
// repeated call finding carries no fingerprint, hash, or source arguments.
func TestRenderRepeatedToolCallFindingOmitsInternalDigests(t *testing.T) {
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
	for _, secret := range []string{"v1:", "ToolCallFingerprint", "toolCallFingerprint", "internal-argument-fingerprint"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("rendered JSON exposes %q: %s", secret, out.String())
		}
	}

	var decoded struct {
		Findings []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal rendered JSON: %v", err)
	}
	if len(decoded.Findings) != 1 {
		t.Fatalf("findings = %+v, want one repeated call finding", decoded.Findings)
	}
	if decoded.Findings[0]["rule_id"] != "TOOL002" {
		t.Fatalf("finding = %+v, want TOOL002", decoded.Findings[0])
	}
	encoded, err := json.Marshal(decoded.Findings)
	if err != nil {
		t.Fatalf("marshal finding: %v", err)
	}
	for _, secret := range []string{"v1:", "sha256:", "internal-output-digest", "internal-argument-fingerprint"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("finding exposes %q: %s", secret, encoded)
		}
	}
}

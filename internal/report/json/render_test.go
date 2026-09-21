package json

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

const (
	secretArguments = `{"cmd":"cat SECRET_ARGUMENT_LITERAL_DO_NOT_RETAIN"}`
	secretOutput    = "SECRET_TOOL_OUTPUT_LITERAL_DO_NOT_RETAIN"
	secretPath      = "/secret/SECRET_PATH_LITERAL_DO_NOT_RETAIN"
	secretRecord    = "record-SECRET_RECORD_LITERAL_DO_NOT_RETAIN"
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

func repeatedCallComponent(callID string) model.ContextComponent {
	return model.ContextComponent{
		Kind:                model.ContextToolResult,
		ToolCallID:          callID,
		ToolName:            "exec_command",
		ToolCallFingerprint: source.StructuredToolCallFingerprintOrEmpty("exec_command", json.RawMessage(secretArguments)),
		ContentHash:         source.ContentHash(secretOutput),
		Path:                secretPath,
		Record:              secretRecord,
		ContentBytes:        model.Int64(2048),
		Completeness:        model.ContextCompletenessComplete,
		Measurement:         model.NewMeasurement(512, model.MeasurementEstimated),
	}
}

func repeatedCallResult(t *testing.T) analyze.Result {
	t.Helper()
	first, second := repeatedCallComponent("call-1"), repeatedCallComponent("call-2")
	if first.ToolCallFingerprint == "" || first.ToolCallFingerprint != second.ToolCallFingerprint {
		t.Fatalf("fingerprint = %q/%q, want the same derived digest", first.ToolCallFingerprint, second.ToolCallFingerprint)
	}
	if first.ContentHash == "" {
		t.Fatal("content hash is empty, want a derived digest")
	}
	return analyze.New().Analyze(context.Background(), model.Session{
		ID:    "session-1",
		Agent: model.AgentCodex,
		Turns: []model.Turn{
			{ID: "turn-1", Sequence: 1, ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{first}}},
			{ID: "turn-2", Sequence: 2, ContextAttribution: model.ContextAttribution{Components: []model.ContextComponent{second}}},
		},
	})
}

// The internal tool call fingerprint and the content digest the repeated call
// rule matches on must not appear anywhere in the doctor JSON, including the
// session it echoes back.
func TestRenderKeepsInternalDigestsOutOfTheWholeOutput(t *testing.T) {
	result := repeatedCallResult(t)
	analyzed := result.Session.Turns[0].ContextAttribution.Components[0]
	if analyzed.ToolCallFingerprint == "" || analyzed.ContentHash == "" {
		t.Fatalf("session component = %+v, want the digests stored before rendering", analyzed)
	}

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}
	rendered := out.String()

	for _, secret := range []string{
		"toolCallFingerprint",
		"ToolCallFingerprint",
		analyzed.ToolCallFingerprint,
		"v1:",
		analyzed.ContentHash,
		"sha256:",
		secretOutput,
	} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("rendered JSON exposes %q: %s", secret, rendered)
		}
	}

	var decoded struct {
		Findings []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal rendered JSON: %v", err)
	}
	if len(decoded.Findings) != 1 || decoded.Findings[0]["rule_id"] != "TOOL002" {
		t.Fatalf("findings = %+v, want one TOOL002 finding", decoded.Findings)
	}
}

// The finding itself must carry no raw argument text, command, path, record, or
// tool output.
func TestRenderKeepsRawSourceDataOutOfFindings(t *testing.T) {
	result := repeatedCallResult(t)
	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var decoded struct {
		Findings json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal rendered JSON: %v", err)
	}
	findings := string(decoded.Findings)
	if findings == "" || findings == "null" {
		t.Fatalf("findings = %s, want the repeated call finding", findings)
	}
	for _, secret := range []string{
		secretArguments,
		"SECRET_ARGUMENT_LITERAL_DO_NOT_RETAIN",
		secretOutput,
		secretPath,
		secretRecord,
		"sha256:",
		"v1:",
	} {
		if strings.Contains(findings, secret) {
			t.Fatalf("finding exposes %q: %s", secret, findings)
		}
	}
}

// The session echoed back by the doctor JSON keeps every non-sensitive field a
// consumer relies on, and the redaction never touches the analyzed session.
func TestRenderPreservesSessionFieldsAndTheOriginalSession(t *testing.T) {
	result := repeatedCallResult(t)
	originalFingerprint := result.Session.Turns[0].ContextAttribution.Components[0].ToolCallFingerprint
	originalHash := result.Session.Turns[0].ContextAttribution.Components[0].ContentHash

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var decoded struct {
		Session model.Session `json:"session"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal rendered JSON: %v", err)
	}
	if decoded.Session.ID != "session-1" || decoded.Session.Agent != model.AgentCodex {
		t.Fatalf("session = %+v, want the analyzed session", decoded.Session)
	}
	if len(decoded.Session.Turns) != 2 {
		t.Fatalf("session turns = %+v, want both turns", decoded.Session.Turns)
	}
	component := decoded.Session.Turns[0].ContextAttribution.Components[0]
	if component.Kind != model.ContextToolResult || component.ToolCallID != "call-1" ||
		component.ToolName != "exec_command" || component.ContentBytes == nil ||
		*component.ContentBytes != 2048 || component.Completeness != model.ContextCompletenessComplete {
		t.Fatalf("component = %+v, want the non-sensitive fields preserved", component)
	}
	if component.Path != secretPath || component.Record != secretRecord {
		t.Fatalf("component = %+v, want the source locators preserved", component)
	}

	analyzed := result.Session.Turns[0].ContextAttribution.Components[0]
	if analyzed.ToolCallFingerprint != originalFingerprint || analyzed.ContentHash != originalHash {
		t.Fatalf("analyzed session was mutated: %+v", analyzed)
	}
}

package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func functionCallWithArgumentsLine(t *testing.T, callID, name, arguments string) string {
	t.Helper()
	return jsonLine(t, map[string]any{
		"type": "response_item",
		"payload": map[string]any{
			"type":      "function_call",
			"id":        callID,
			"call_id":   callID,
			"name":      name,
			"arguments": arguments,
		},
	})
}

func resultFingerprints(t *testing.T, parsed ParsedSession) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, component := range findComponents(parsed.Snapshots[0].Context, model.ContextToolResult) {
		out[component.ToolCallID] = component.ToolCallFingerprint
	}
	return out
}

func TestParseSessionFingerprintsLinkedToolCall(t *testing.T) {
	path := writeSessionLines(t, []string{
		functionCallWithArgumentsLine(t, "call-1", "functions.exec_command", `{"cmd":"cat AGENTS.md"}`),
		functionCallOutputLine(t, "call-1", "synthetic output"),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if !strings.HasPrefix(component.ToolCallFingerprint, "v1:") {
		t.Fatalf("fingerprint = %q, want a versioned fingerprint", component.ToolCallFingerprint)
	}
	if strings.Contains(component.ToolCallFingerprint, "AGENTS") {
		t.Fatalf("fingerprint = %q, want an opaque digest", component.ToolCallFingerprint)
	}
}

func TestParseSessionFingerprintsIdenticalArgumentsEqually(t *testing.T) {
	path := writeSessionLines(t, []string{
		functionCallWithArgumentsLine(t, "call-1", "functions.exec_command", `{"cmd":"ls","timeout":5}`),
		functionCallOutputLine(t, "call-1", "first output"),
		functionCallWithArgumentsLine(t, "call-2", "functions.exec_command", `{"timeout":5,"cmd":"ls"}`),
		functionCallOutputLine(t, "call-2", "second output"),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fingerprints := resultFingerprints(t, parsed)
	if fingerprints["call-1"] == "" || fingerprints["call-1"] != fingerprints["call-2"] {
		t.Fatalf("fingerprints = %+v, want the same value for identical arguments", fingerprints)
	}
}

func TestParseSessionFingerprintsDifferentArgumentsDifferently(t *testing.T) {
	path := writeSessionLines(t, []string{
		functionCallWithArgumentsLine(t, "call-1", "functions.exec_command", `{"cmd":"ls"}`),
		functionCallOutputLine(t, "call-1", "first output"),
		functionCallWithArgumentsLine(t, "call-2", "functions.exec_command", `{"cmd":"pwd"}`),
		functionCallOutputLine(t, "call-2", "second output"),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fingerprints := resultFingerprints(t, parsed)
	if fingerprints["call-1"] == "" || fingerprints["call-1"] == fingerprints["call-2"] {
		t.Fatalf("fingerprints = %+v, want different values", fingerprints)
	}
}

func TestParseSessionSurvivesUnfingerprintableArguments(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
	}{
		{name: "malformed arguments", arguments: `{"cmd":`},
		{name: "freeform arguments", arguments: `cat AGENTS.md`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeSessionLines(t, []string{
				functionCallWithArgumentsLine(t, "call-1", "functions.exec_command", tt.arguments),
				functionCallOutputLine(t, "call-1", "synthetic output"),
				tokenCountLine(t, 10, 0, 1, 11),
				tokenCountLine(t, 20, 0, 2, 22),
			})

			parsed, err := ParseSession(path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if parsed.Usage.Total != 22 {
				t.Fatalf("usage total = %d, want the session to keep parsing", parsed.Usage.Total)
			}
			component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
			if component.ToolCallFingerprint != "" {
				t.Fatalf("fingerprint = %q, want none for unusable arguments", component.ToolCallFingerprint)
			}
			if component.ToolName != "functions.exec_command" {
				t.Fatalf("tool name = %q, want the declared name preserved", component.ToolName)
			}
		})
	}
}

func TestParseSessionLeavesFingerprintEmptyWithoutDeclaredArguments(t *testing.T) {
	path := writeSessionLines(t, []string{
		jsonLine(t, map[string]any{
			"type": "response_item",
			"payload": map[string]any{
				"type": "function_call", "id": "call-1", "call_id": "call-1",
				"name": "functions.exec_command",
			},
		}),
		functionCallOutputLine(t, "call-1", "synthetic output"),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if component.ToolCallFingerprint != "" {
		t.Fatalf("fingerprint = %q, want none when the source declared no arguments", component.ToolCallFingerprint)
	}
}

// An output whose call the transcript never declared keeps an empty fingerprint
// instead of one guessed from the result.
func TestParseSessionLeavesOrphanResultWithoutFingerprint(t *testing.T) {
	path := writeSessionLines(t, []string{
		functionCallOutputLine(t, "call-orphan", "orphan output"),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if component.ToolCallFingerprint != "" {
		t.Fatalf("fingerprint = %q, want none for an orphan result", component.ToolCallFingerprint)
	}
}

func TestParseSessionFingerprintsCustomToolCallInput(t *testing.T) {
	path := writeSessionLines(t, []string{
		customToolCallLine(t, "call-1", "exec", `{"cmd":"ls","timeout":5}`),
		customToolCallOutputLine(t, "call-1", customTextOutput("first output")),
		customToolCallLine(t, "call-2", "exec", `{"timeout":5,"cmd":"ls"}`),
		customToolCallOutputLine(t, "call-2", customTextOutput("second output")),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fingerprints := resultFingerprints(t, parsed)
	if fingerprints["call-1"] == "" || fingerprints["call-1"] != fingerprints["call-2"] {
		t.Fatalf("fingerprints = %+v, want the custom shape fingerprinted too", fingerprints)
	}
}

// A custom tool call with freeform, non-JSON input keeps parsing and stays
// unfingerprinted rather than being described as an equal call.
func TestParseSessionKeepsFreeformCustomToolCallInput(t *testing.T) {
	path := writeSessionLines(t, []string{
		customToolCallLine(t, "call-1", "apply_patch", "*** Begin Patch"),
		customToolCallOutputLine(t, "call-1", customTextOutput("patch applied")),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if component.ToolName != "apply_patch" {
		t.Fatalf("tool name = %q, want the custom call linked", component.ToolName)
	}
	if component.ToolCallFingerprint != "" {
		t.Fatalf("fingerprint = %q, want none for freeform input", component.ToolCallFingerprint)
	}
}

func TestContextComponentNeverSerializesTheFingerprint(t *testing.T) {
	path := writeSessionLines(t, []string{
		functionCallWithArgumentsLine(t, "call-1", "functions.exec_command", `{"cmd":"ls"}`),
		functionCallOutputLine(t, "call-1", "synthetic output"),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if component.ToolCallFingerprint == "" {
		t.Fatal("fixture must carry a fingerprint")
	}
	raw, err := json.Marshal(parsed.Snapshots[0].Context)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	for _, secret := range []string{"fingerprint", component.ToolCallFingerprint, "v1:"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("serialized context exposes %q: %s", secret, raw)
		}
	}
}

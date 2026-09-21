package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func assistantToolUseWithInputLine(t *testing.T, messageID, toolUseID, toolName string, input any) string {
	t.Helper()
	return jsonLine(t, map[string]any{
		"uuid": "a-" + messageID,
		"type": "assistant",
		"message": map[string]any{
			"id":    messageID,
			"role":  "assistant",
			"model": "synthetic-model",
			"content": []any{
				map[string]any{"type": "tool_use", "id": toolUseID, "name": toolName, "input": input},
			},
			"usage": map[string]any{
				"input_tokens":                20,
				"output_tokens":               5,
				"cache_read_input_tokens":     0,
				"cache_creation_input_tokens": 0,
			},
		},
	})
}

func toolResultFingerprints(t *testing.T, parsed ParsedSession) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, invocation := range parsed.Invocations {
		for _, component := range invocation.Context {
			if component.Kind == model.ContextToolResult {
				out[component.ToolCallID] = component.ToolCallFingerprint
			}
		}
	}
	return out
}

func TestParseSessionFingerprintsToolUseInput(t *testing.T) {
	path := writeSessionLines(t, []string{
		assistantToolUseWithInputLine(t, "msg_1", "toolu_1", "Bash", map[string]any{"cmd": "ls"}),
		toolResultLine(t, "toolu_1", "synthetic result"),
		assistantTextLine(t, "msg_2", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[1].Context, model.ContextToolResult)
	if !strings.HasPrefix(component.ToolCallFingerprint, "v1:") {
		t.Fatalf("fingerprint = %q, want a versioned fingerprint", component.ToolCallFingerprint)
	}
	if component.ToolCallID != "toolu_1" || component.ToolName != "Bash" {
		t.Fatalf("tool linkage = %q/%q, want the declared call preserved", component.ToolCallID, component.ToolName)
	}
}

func TestParseSessionFingerprintsIdenticalInputsEqually(t *testing.T) {
	path := writeSessionLines(t, []string{
		assistantToolUseWithInputLine(t, "msg_1", "toolu_1", "Bash", json.RawMessage(`{"cmd":"ls","timeout":5}`)),
		toolResultLine(t, "toolu_1", "first result"),
		assistantToolUseWithInputLine(t, "msg_2", "toolu_2", "Bash", json.RawMessage(`{"timeout":5,"cmd":"ls"}`)),
		toolResultLine(t, "toolu_2", "second result"),
		assistantTextLine(t, "msg_3", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fingerprints := toolResultFingerprints(t, parsed)
	if fingerprints["toolu_1"] == "" || fingerprints["toolu_1"] != fingerprints["toolu_2"] {
		t.Fatalf("fingerprints = %+v, want the same value for identical inputs", fingerprints)
	}
}

func TestParseSessionFingerprintsDifferentInputsDifferently(t *testing.T) {
	path := writeSessionLines(t, []string{
		assistantToolUseWithInputLine(t, "msg_1", "toolu_1", "Bash", map[string]any{"cmd": "ls"}),
		toolResultLine(t, "toolu_1", "first result"),
		assistantToolUseWithInputLine(t, "msg_2", "toolu_2", "Bash", map[string]any{"cmd": "pwd"}),
		toolResultLine(t, "toolu_2", "second result"),
		assistantTextLine(t, "msg_3", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fingerprints := toolResultFingerprints(t, parsed)
	if fingerprints["toolu_1"] == "" || fingerprints["toolu_1"] == fingerprints["toolu_2"] {
		t.Fatalf("fingerprints = %+v, want different values", fingerprints)
	}
}

func TestParseSessionLeavesFingerprintEmptyWithoutToolUseInput(t *testing.T) {
	line := jsonLine(t, map[string]any{
		"uuid": "a-msg_1",
		"type": "assistant",
		"message": map[string]any{
			"id":      "msg_1",
			"role":    "assistant",
			"model":   "synthetic-model",
			"content": []any{map[string]any{"type": "tool_use", "id": "toolu_1", "name": "Bash"}},
			"usage": map[string]any{
				"input_tokens":                20,
				"output_tokens":               5,
				"cache_read_input_tokens":     0,
				"cache_creation_input_tokens": 0,
			},
		},
	})
	path := writeSessionLines(t, []string{line, toolResultLine(t, "toolu_1", "synthetic result"), assistantTextLine(t, "msg_2", "done", 30)})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[1].Context, model.ContextToolResult)
	if component.ToolName != "Bash" {
		t.Fatalf("tool name = %q, want the declared name preserved", component.ToolName)
	}
	if component.ToolCallFingerprint != "" {
		t.Fatalf("fingerprint = %q, want none when no input was declared", component.ToolCallFingerprint)
	}
}

// A tool_use with an input that is not an object keeps parsing and stays
// unfingerprinted instead of being described as an equal call.
func TestParseSessionLeavesUnfingerprintableToolUseInputEmpty(t *testing.T) {
	path := writeSessionLines(t, []string{
		assistantToolUseWithInputLine(t, "msg_1", "toolu_1", "Bash", "ls -la"),
		toolResultLine(t, "toolu_1", "synthetic result"),
		assistantTextLine(t, "msg_2", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[1].Context, model.ContextToolResult)
	if component.ToolCallFingerprint != "" {
		t.Fatalf("fingerprint = %q, want none for a non-JSON input", component.ToolCallFingerprint)
	}
}

func TestParseSessionLeavesOrphanToolResultWithoutFingerprint(t *testing.T) {
	path := writeSessionLines(t, []string{
		toolResultLine(t, "toolu_unknown", "orphan result"),
		assistantTextLine(t, "msg_1", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[0].Context, model.ContextToolResult)
	if component.ToolCallFingerprint != "" {
		t.Fatalf("fingerprint = %q, want none for an orphan result", component.ToolCallFingerprint)
	}
}

func TestClaudeContextComponentNeverSerializesTheFingerprint(t *testing.T) {
	path := writeSessionLines(t, []string{
		assistantToolUseWithInputLine(t, "msg_1", "toolu_1", "Bash", map[string]any{"cmd": "ls"}),
		toolResultLine(t, "toolu_1", "synthetic result"),
		assistantTextLine(t, "msg_2", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[1].Context, model.ContextToolResult)
	if component.ToolCallFingerprint == "" {
		t.Fatal("fixture must carry a fingerprint")
	}
	raw, err := json.Marshal(parsed.Invocations[1].Context)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	for _, secret := range []string{"fingerprint", component.ToolCallFingerprint, "v1:"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("serialized context exposes %q: %s", secret, raw)
		}
	}
}

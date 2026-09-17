package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func writeSessionLines(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func jsonLine(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture line: %v", err)
	}
	return string(raw)
}

func assistantToolUseLine(t *testing.T, messageID, toolUseID, toolName string, input int64) string {
	t.Helper()
	return jsonLine(t, map[string]any{
		"uuid": "a-" + messageID,
		"type": "assistant",
		"message": map[string]any{
			"id":    messageID,
			"role":  "assistant",
			"model": "synthetic-model",
			"content": []any{
				map[string]any{"type": "tool_use", "id": toolUseID, "name": toolName, "input": map[string]any{"cmd": "ls"}},
			},
			"usage": map[string]any{
				"input_tokens":                input,
				"output_tokens":               5,
				"cache_read_input_tokens":     0,
				"cache_creation_input_tokens": 0,
			},
		},
	})
}

func toolResultLine(t *testing.T, toolUseID string, content any) string {
	t.Helper()
	return jsonLine(t, map[string]any{
		"uuid": "u-" + toolUseID,
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": toolUseID, "content": content},
			},
		},
	})
}

func assistantTextLine(t *testing.T, messageID, text string, input int64) string {
	t.Helper()
	return jsonLine(t, map[string]any{
		"uuid": "a-" + messageID,
		"type": "assistant",
		"message": map[string]any{
			"id":      messageID,
			"role":    "assistant",
			"model":   "synthetic-model",
			"content": []any{map[string]any{"type": "text", "text": text}},
			"usage": map[string]any{
				"input_tokens":                input,
				"output_tokens":               5,
				"cache_read_input_tokens":     0,
				"cache_creation_input_tokens": 0,
			},
		},
	})
}

func findComponent(t *testing.T, components []model.ContextComponent, kind model.ContextComponentKind) model.ContextComponent {
	t.Helper()
	for _, component := range components {
		if component.Kind == kind {
			return component
		}
	}
	t.Fatalf("no %q component in %+v", kind, components)
	return model.ContextComponent{}
}

func TestParseSessionLinksToolResultToToolUse(t *testing.T) {
	path := writeSessionLines(t, []string{
		assistantToolUseLine(t, "msg_1", "toolu_1", "Bash", 20),
		toolResultLine(t, "toolu_1", "synthetic result"),
		assistantTextLine(t, "msg_2", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[1].Context, model.ContextToolResult)
	if component.ToolCallID != "toolu_1" || component.ToolName != "Bash" {
		t.Fatalf("tool linkage = %q/%q, want toolu_1/Bash", component.ToolCallID, component.ToolName)
	}
	if component.ContentBytes == nil || *component.ContentBytes != int64(len("synthetic result")) {
		t.Fatalf("content bytes = %v, want %d", component.ContentBytes, len("synthetic result"))
	}
	if component.Completeness != model.ContextCompletenessComplete {
		t.Fatalf("completeness = %q, want complete", component.Completeness)
	}
}

func TestParseSessionLeavesUndeclaredToolNameEmpty(t *testing.T) {
	path := writeSessionLines(t, []string{
		toolResultLine(t, "toolu_unknown", "orphan result"),
		assistantTextLine(t, "msg_1", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[0].Context, model.ContextToolResult)
	if component.ToolCallID != "toolu_unknown" {
		t.Fatalf("tool use id = %q, want the source id", component.ToolCallID)
	}
	if component.ToolName != "" {
		t.Fatalf("tool name = %q, want empty when no tool_use declared it", component.ToolName)
	}
}

func TestParseSessionKeepsToolOutputLargerThanFourMiB(t *testing.T) {
	output := strings.Repeat("y", 4*1024*1024+1024)
	path := writeSessionLines(t, []string{
		assistantToolUseLine(t, "msg_1", "toolu_big", "Read", 20),
		toolResultLine(t, "toolu_big", output),
		assistantTextLine(t, "msg_2", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Invocations) != 2 {
		t.Fatalf("invocations = %d, want 2", len(parsed.Invocations))
	}
	component := findComponent(t, parsed.Invocations[1].Context, model.ContextToolResult)
	if component.ContentBytes == nil || *component.ContentBytes != int64(len(output)) {
		t.Fatalf("content bytes = %v, want %d", component.ContentBytes, len(output))
	}
	if component.Measurement.Kind != model.MeasurementEstimated {
		t.Fatalf("measurement = %+v, want estimated tokens", component.Measurement)
	}
}

func TestParseSessionKeepsMultimodalToolResultWithoutTokens(t *testing.T) {
	content := []any{map[string]any{"type": "image", "source": map[string]any{"data": "AAAA"}}}
	path := writeSessionLines(t, []string{
		toolResultLine(t, "toolu_1", content),
		assistantTextLine(t, "msg_1", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[0].Context, model.ContextToolResult)
	if component.ContentBytes == nil || *component.ContentBytes == 0 {
		t.Fatalf("content bytes = %v, want the payload size", component.ContentBytes)
	}
	if component.Measurement.Kind != model.MeasurementUnknown {
		t.Fatalf("measurement = %+v, want unknown for non-text content", component.Measurement)
	}
	if component.ContentHash != "" {
		t.Fatalf("content hash = %q, want none when no text was extracted", component.ContentHash)
	}
}

func TestParseSessionPreservesTrailingContextAfterFinalInvocation(t *testing.T) {
	path := writeSessionLines(t, []string{
		assistantTextLine(t, "msg_1", "first", 20),
		assistantToolUseLine(t, "msg_2", "toolu_tail", "Bash", 40),
		toolResultLine(t, "toolu_tail", "trailing output"),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Invocations) != 2 {
		t.Fatalf("invocations = %d, want the trailing record to add no turn", len(parsed.Invocations))
	}
	component := findComponent(t, parsed.TrailingContext, model.ContextToolResult)
	if component.ToolCallID != "toolu_tail" {
		t.Fatalf("trailing tool call = %q, want toolu_tail", component.ToolCallID)
	}
}

func TestParseSessionReportsMalformedRecordAsUnavailableContext(t *testing.T) {
	path := writeSessionLines(t, []string{
		`{"uuid":"u1","type":"user","message":` + "\n",
		assistantTextLine(t, "msg_1", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Invocations) != 1 {
		t.Fatalf("invocations = %d, want the session to keep parsing", len(parsed.Invocations))
	}
	unreadable := findComponent(t, parsed.Invocations[0].Context, model.ContextUnknown)
	if unreadable.Completeness != model.ContextCompletenessUnavailable {
		t.Fatalf("completeness = %q, want unavailable", unreadable.Completeness)
	}
	if unreadable.Evidence[0].Record != "claude_session#line:1" {
		t.Fatalf("evidence = %+v, want the unreadable line", unreadable.Evidence)
	}
}

func TestParseSessionRetainsOnlyHashAndSizeForToolOutput(t *testing.T) {
	const secret = "SECRET_TOOL_OUTPUT_LITERAL_DO_NOT_RETAIN"
	path := writeSessionLines(t, []string{
		toolResultLine(t, "toolu_1", secret),
		assistantTextLine(t, "msg_1", "done", 30),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Invocations[0].Context, model.ContextToolResult)
	if !strings.HasPrefix(component.ContentHash, "sha256:") || strings.Contains(component.ContentHash, secret) {
		t.Fatalf("content hash = %q, want a non-raw hash", component.ContentHash)
	}
	if rendered := fmt.Sprintf("%+v", parsed); strings.Contains(rendered, secret) {
		t.Fatalf("parsed session retains raw tool output: %s", rendered)
	}
}

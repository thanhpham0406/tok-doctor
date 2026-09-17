package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

const secretToolOutput = "SECRET_TOOL_OUTPUT_LITERAL_DO_NOT_RETAIN"

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

func tokenCountLine(t *testing.T, input, cached, output, total int64) string {
	t.Helper()
	return jsonLine(t, map[string]any{
		"type": "event_msg",
		"payload": map[string]any{
			"type": "token_count",
			"info": map[string]any{
				"total_token_usage": map[string]any{
					"input_tokens":            input,
					"cached_input_tokens":     cached,
					"output_tokens":           output,
					"reasoning_output_tokens": 0,
					"total_tokens":            total,
				},
			},
		},
	})
}

func functionCallLine(t *testing.T, callID, name string) string {
	t.Helper()
	return jsonLine(t, map[string]any{
		"type": "response_item",
		"payload": map[string]any{
			"type":      "function_call",
			"id":        callID,
			"call_id":   callID,
			"name":      name,
			"arguments": `{"cmd":"cat AGENTS.md"}`,
		},
	})
}

func functionCallOutputLine(t *testing.T, callID string, output any) string {
	t.Helper()
	return jsonLine(t, map[string]any{
		"type": "response_item",
		"payload": map[string]any{
			"type":    "function_call_output",
			"id":      callID + "-out",
			"call_id": callID,
			"output":  output,
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

// A tool output above the previous 4 MiB scanner ceiling must not fail the
// session; it is a normal record with a real byte size.
func TestParseSessionKeepsToolOutputLargerThanFourMiB(t *testing.T) {
	output := strings.Repeat("x", 4*1024*1024+1024)
	path := writeSessionLines(t, []string{
		`{"type":"session_meta","payload":{"id":"big"}}`,
		functionCallLine(t, "call-big", "functions.exec_command"),
		functionCallOutputLine(t, "call-big", output),
		tokenCountLine(t, 900, 100, 200, 1200),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Snapshots) != 1 || parsed.Usage.Total != 1200 {
		t.Fatalf("usage = %+v snapshots=%d, want one snapshot with total 1200", parsed.Usage, len(parsed.Snapshots))
	}

	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if component.ContentBytes == nil || *component.ContentBytes != int64(len(output)) {
		t.Fatalf("content bytes = %v, want %d", component.ContentBytes, len(output))
	}
	if component.Measurement.Kind != model.MeasurementEstimated {
		t.Fatalf("measurement = %+v, want estimated tokens", component.Measurement)
	}
	if component.Completeness != model.ContextCompletenessComplete {
		t.Fatalf("completeness = %q, want complete", component.Completeness)
	}
}

func TestParseSessionLinksFunctionCallOutputToToolName(t *testing.T) {
	path := writeSessionLines(t, []string{
		functionCallLine(t, "call-1", "functions.exec_command"),
		functionCallOutputLine(t, "call-1", "synthetic output"),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if component.ToolCallID != "call-1" || component.ToolName != "functions.exec_command" {
		t.Fatalf("tool linkage = %q/%q, want call-1/functions.exec_command", component.ToolCallID, component.ToolName)
	}
	if component.ContentBytes == nil || *component.ContentBytes != int64(len("synthetic output")) {
		t.Fatalf("content bytes = %v", component.ContentBytes)
	}
}

// A tool name the transcript never declared must stay empty rather than being
// guessed from the output.
func TestParseSessionLeavesUnknownToolNameEmpty(t *testing.T) {
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
	if component.ToolCallID != "call-orphan" {
		t.Fatalf("tool call id = %q, want the source call id", component.ToolCallID)
	}
	if component.ToolName != "" {
		t.Fatalf("tool name = %q, want empty when the source never declared it", component.ToolName)
	}
}

// Records after the final token_count belong to no turn. They are preserved as
// trailing context instead of being attributed to the previous turn.
func TestParseSessionPreservesTrailingContextAfterFinalSnapshot(t *testing.T) {
	path := writeSessionLines(t, []string{
		functionCallLine(t, "call-1", "functions.exec_command"),
		functionCallOutputLine(t, "call-1", "first output"),
		tokenCountLine(t, 100, 0, 10, 110),
		functionCallLine(t, "call-2", "functions.exec_command"),
		functionCallOutputLine(t, "call-2", "trailing output"),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(parsed.Snapshots))
	}
	if parsed.Usage.Total != 110 {
		t.Fatalf("usage total = %d, want the final snapshot only", parsed.Usage.Total)
	}
	trailing := findComponent(t, parsed.TrailingContext, model.ContextToolResult)
	if trailing.ToolCallID != "call-2" {
		t.Fatalf("trailing tool call = %q, want call-2", trailing.ToolCallID)
	}
	if len(parsed.Snapshots[0].Context) != 2 {
		t.Fatalf("turn context = %d components, want the trailing record excluded", len(parsed.Snapshots[0].Context))
	}
}

// A malformed record is reported as unreadable context so partial coverage is
// visible, and the rest of the session still parses.
func TestParseSessionReportsMalformedRecordAsUnavailableContext(t *testing.T) {
	path := writeSessionLines(t, []string{
		`this is not valid json`,
		functionCallOutputLine(t, "call-1", "synthetic output"),
		tokenCountLine(t, 900, 100, 200, 1200),
		tokenCountLine(t, 1000, 100, 200, 1300),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Usage.Total != 1300 {
		t.Fatalf("usage total = %d, want the session to keep parsing", parsed.Usage.Total)
	}
	unreadable := findComponent(t, parsed.Snapshots[0].Context, model.ContextUnknown)
	if unreadable.Completeness != model.ContextCompletenessUnavailable {
		t.Fatalf("completeness = %q, want unavailable", unreadable.Completeness)
	}
	if unreadable.Measurement.Available() {
		t.Fatalf("measurement = %+v, want unavailable for a record that was never read", unreadable.Measurement)
	}
}

// A structured output is not text, so it keeps its byte size without a token
// estimate, and it must not break the record.
func TestParseSessionKeepsMultimodalToolOutputWithoutTokens(t *testing.T) {
	output := []any{
		map[string]any{"type": "image", "data": strings.Repeat("A", 512)},
	}
	path := writeSessionLines(t, []string{
		functionCallOutputLine(t, "call-1", output),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if component.ContentBytes == nil || *component.ContentBytes == 0 {
		t.Fatalf("content bytes = %v, want the structured payload size", component.ContentBytes)
	}
	if component.Measurement.Kind != model.MeasurementUnknown {
		t.Fatalf("measurement = %+v, want unknown for non-text output", component.Measurement)
	}
	if component.ContentHash != "" {
		t.Fatalf("content hash = %q, want none when no text was extracted", component.ContentHash)
	}
}

// The canonical model keeps a hash and a size, never the tool output text.
func TestParseSessionRetainsOnlyHashAndSizeForToolOutput(t *testing.T) {
	path := writeSessionLines(t, []string{
		functionCallOutputLine(t, "call-1", secretToolOutput),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})

	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	component := findComponent(t, parsed.Snapshots[0].Context, model.ContextToolResult)
	if !strings.HasPrefix(component.ContentHash, "sha256:") || strings.Contains(component.ContentHash, secretToolOutput) {
		t.Fatalf("content hash = %q, want a non-raw hash", component.ContentHash)
	}
	if rendered := fmt.Sprintf("%+v", parsed); strings.Contains(rendered, secretToolOutput) {
		t.Fatalf("parsed session retains raw tool output: %s", rendered)
	}
}

func TestUnreadableRecordMarksOversizedAndMalformedDifferently(t *testing.T) {
	oversized := unreadableRecord(7, codexFieldOversized, model.ContextCompletenessTruncated)
	if oversized.Completeness != model.ContextCompletenessTruncated || oversized.Record != "codex_rollout#line:7" {
		t.Fatalf("oversized marker = %+v", oversized)
	}
	malformed := unreadableRecord(8, codexFieldRecord, model.ContextCompletenessUnavailable)
	if malformed.Completeness != model.ContextCompletenessUnavailable {
		t.Fatalf("malformed marker = %+v", malformed)
	}
	if len(malformed.Evidence) != 1 || malformed.Evidence[0].Record != "codex_rollout#line:8" {
		t.Fatalf("evidence = %+v, want provenance for the unreadable record", malformed.Evidence)
	}
}

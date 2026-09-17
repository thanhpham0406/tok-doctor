package codex

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/coverage"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	ruletool "github.com/thanhpham0406/tok-doctor/internal/rule/tool"
)

// canonicalSession projects a parsed transcript the same way the adapter does,
// so downstream analysis and coverage see the custom call shape through the
// canonical model only.
func canonicalSession(t *testing.T, path, id string) model.Session {
	t.Helper()
	parsed, err := ParseSession(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return model.Session{
		ID:              id,
		Source:          string(model.AgentCodex),
		Agent:           model.AgentCodex,
		Turns:           reconstructTurns(id, parsed),
		TrailingContext: parsed.TrailingContext,
	}
}

// A custom tool output above the oversized threshold must reach TOOL001 through
// the canonical session, without the rule learning anything Codex-specific.
func TestCustomToolOutputAboveThresholdProducesOversizedFinding(t *testing.T) {
	text := strings.Repeat("x", 70*1024-len(secretToolOutput)) + secretToolOutput
	path := writeSessionLines(t, []string{
		customToolCallLine(t, "call-big", "exec", "echo hi"),
		customToolCallOutputLine(t, "call-big", customTextOutput(text)),
		tokenCountLine(t, 900, 100, 200, 1200),
		tokenCountLine(t, 1000, 100, 200, 1300),
	})
	session := canonicalSession(t, path, "sess-custom")

	result := analyze.New().Analyze(context.Background(), session)
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %d (%+v), want one oversized tool output", len(result.Findings), result.Findings)
	}
	finding := result.Findings[0]
	if finding.RuleID != ruletool.OversizedOutputRuleID {
		t.Fatalf("rule = %q, want %q", finding.RuleID, ruletool.OversizedOutputRuleID)
	}
	if len(finding.Evidence) != 1 {
		t.Fatalf("evidence = %+v, want one entry", finding.Evidence)
	}
	evidence := finding.Evidence[0]
	if evidence.ToolCallID != "call-big" || evidence.ToolName != "exec" {
		t.Fatalf("finding tool = %q/%q, want call-big/exec", evidence.ToolCallID, evidence.ToolName)
	}
	if evidence.OutputBytes <= ruletool.DefaultMaxOutputBytes {
		t.Fatalf("finding bytes = %d, want a size above the threshold", evidence.OutputBytes)
	}
	if rendered := fmt.Sprintf("%+v", result); strings.Contains(rendered, secretToolOutput) {
		t.Fatalf("analysis result retains raw tool output: %s", rendered)
	}
}

// The same transcript must show up in coverage as a linked tool output, again
// without the raw output leaving the parser.
func TestCustomToolOutputCountsInCoverage(t *testing.T) {
	path := writeSessionLines(t, []string{
		customToolCallLine(t, "call-custom", "exec", "echo hi"),
		customToolCallOutputLine(t, "call-custom", customTextOutput(secretToolOutput)),
		tokenCountLine(t, 10, 0, 1, 11),
		tokenCountLine(t, 20, 0, 2, 22),
	})
	session := canonicalSession(t, path, "sess-custom")

	summary := coverage.Scan([]model.Session{session})
	if summary.ToolOutputs != 1 {
		t.Fatalf("tool outputs = %d, want the custom tool output counted", summary.ToolOutputs)
	}
	if summary.WithToolCallID.Count != 1 || summary.WithToolName.Count != 1 {
		t.Fatalf("linkage = %+v / %+v, want call id and name preserved", summary.WithToolCallID, summary.WithToolName)
	}
	if summary.WithContentBytes.Count != 1 || summary.ContentBytes.Samples != 1 {
		t.Fatalf("bytes = %+v samples=%d, want a measured size", summary.WithContentBytes, summary.ContentBytes.Samples)
	}
	if summary.WithEstimatedTokens.Count != 1 {
		t.Fatalf("estimated tokens = %+v, want the text output estimated", summary.WithEstimatedTokens)
	}
	if rendered := fmt.Sprintf("%+v", summary); strings.Contains(rendered, secretToolOutput) {
		t.Fatalf("coverage result retains raw tool output: %s", rendered)
	}
}

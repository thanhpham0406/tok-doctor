package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func repeatedCallComponent(callID string) model.ContextComponent {
	return model.ContextComponent{
		Kind:                model.ContextToolResult,
		ToolCallID:          callID,
		ToolName:            "exec_command",
		ToolCallFingerprint: "v1:argument-fingerprint",
		ContentHash:         "sha256:output-digest",
		ContentBytes:        model.Int64(4096),
		Completeness:        model.ContextCompletenessComplete,
		Measurement:         model.NewMeasurement(1024, model.MeasurementEstimated),
	}
}

func sessionWithCallTurns(turns ...[]model.ContextComponent) model.Session {
	session := model.Session{ID: "sess-1", Agent: model.AgentCodex}
	for i, components := range turns {
		session.Turns = append(session.Turns, model.Turn{
			ID:                 fmt.Sprintf("turn-%d", i+1),
			Sequence:           i + 1,
			ContextAttribution: model.ContextAttribution{Components: components},
		})
	}
	return session
}

func analyzeRepeatedCalls(session model.Session) []model.Finding {
	return NewRepeatedToolCall().Analyze(context.Background(), session)
}

func TestRepeatedToolCallGroupsOneFindingPerRepeatedGroup(t *testing.T) {
	session := sessionWithCallTurns(
		[]model.ContextComponent{repeatedCallComponent("call-1")},
		[]model.ContextComponent{repeatedCallComponent("call-2")},
		[]model.ContextComponent{repeatedCallComponent("call-3")},
	)

	findings := analyzeRepeatedCalls(session)
	if len(findings) != 1 {
		t.Fatalf("findings = %d (%+v), want one grouped finding", len(findings), findings)
	}
	finding := findings[0]
	if finding.RuleID != RepeatedToolCallRuleID || finding.Name != RepeatedToolCallRuleName {
		t.Fatalf("finding = %s/%s, want %s/%s", finding.RuleID, finding.Name, RepeatedToolCallRuleID, RepeatedToolCallRuleName)
	}
	if finding.Severity != model.SeverityMedium || finding.Confidence != model.ConfidenceHigh {
		t.Fatalf("severity/confidence = %s/%s, want medium/high", finding.Severity, finding.Confidence)
	}
	if finding.Title != "exec_command repeated the same tool call" {
		t.Fatalf("title = %q", finding.Title)
	}
	if len(finding.Evidence) != 3 {
		t.Fatalf("evidence = %+v, want the baseline and both repeats", finding.Evidence)
	}
}

func TestRepeatedToolCallCountsOnlyOccurrencesAfterTheBaseline(t *testing.T) {
	baseline := repeatedCallComponent("call-1")
	baseline.Measurement = model.NewMeasurement(100, model.MeasurementEstimated)
	second := repeatedCallComponent("call-2")
	second.Measurement = model.NewMeasurement(200, model.MeasurementEstimated)
	third := repeatedCallComponent("call-3")
	third.Measurement = model.NewMeasurement(300, model.MeasurementEstimated)

	findings := analyzeRepeatedCalls(sessionWithCallTurns(
		[]model.ContextComponent{baseline},
		[]model.ContextComponent{second},
		[]model.ContextComponent{third},
	))
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	estimated := findings[0].EstimatedTokens
	if estimated.Kind != model.MeasurementEstimated || estimated.ValueOrZero() != 500 {
		t.Fatalf("estimated tokens = %+v, want 500 estimated", estimated)
	}
	if !strings.Contains(findings[0].Description, "500") {
		t.Fatalf("description = %q, want the estimated total", findings[0].Description)
	}
	if !strings.Contains(findings[0].Description, "estimated repeated tool output") ||
		!strings.Contains(findings[0].Description, "potential context contribution") ||
		!strings.Contains(findings[0].Description, "not measured waste") {
		t.Fatalf("description = %q, want the cautious wording", findings[0].Description)
	}
}

// A missing measurement must stay missing. The baseline is never part of the
// total either, because only the repeats can be avoided.
func TestRepeatedToolCallKeepsMissingMeasurementsMissing(t *testing.T) {
	baseline := repeatedCallComponent("call-1")
	baseline.Measurement = model.NewMeasurement(4096, model.MeasurementEstimated)
	second := repeatedCallComponent("call-2")
	second.Measurement = model.Measurement{Kind: model.MeasurementUnknown}
	third := repeatedCallComponent("call-3")
	third.Measurement = model.Measurement{}

	findings := analyzeRepeatedCalls(sessionWithCallTurns(
		[]model.ContextComponent{baseline},
		[]model.ContextComponent{second},
		[]model.ContextComponent{third},
	))
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	estimated := findings[0].EstimatedTokens
	if estimated.Available() || estimated.Value != nil {
		t.Fatalf("estimated tokens = %+v, want unavailable rather than zero", estimated)
	}
	if strings.Contains(findings[0].Description, "0 tokens") {
		t.Fatalf("description = %q, want no invented zero", findings[0].Description)
	}
}

// One unmeasured repeat must not erase a measured one.
func TestRepeatedToolCallSumsOnlyAvailableMeasurements(t *testing.T) {
	baseline := repeatedCallComponent("call-1")
	second := repeatedCallComponent("call-2")
	second.Measurement = model.Measurement{Kind: model.MeasurementUnknown}
	third := repeatedCallComponent("call-3")
	third.Measurement = model.NewMeasurement(700, model.MeasurementEstimated)

	findings := analyzeRepeatedCalls(sessionWithCallTurns(
		[]model.ContextComponent{baseline},
		[]model.ContextComponent{second},
		[]model.ContextComponent{third},
	))
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	if estimated := findings[0].EstimatedTokens; estimated.ValueOrZero() != 700 || estimated.Kind != model.MeasurementEstimated {
		t.Fatalf("estimated tokens = %+v, want 700 estimated from the available repeat", estimated)
	}
}

func TestRepeatedToolCallReportsIndependentGroupsInOrder(t *testing.T) {
	first := repeatedCallComponent("call-1")
	second := repeatedCallComponent("call-2")
	otherFirst := repeatedCallComponent("call-3")
	otherFirst.ToolName = "read_file"
	otherFirst.ToolCallFingerprint = "v1:other-fingerprint"
	otherSecond := otherFirst
	otherSecond.ToolCallID = "call-4"

	findings := analyzeRepeatedCalls(sessionWithCallTurns(
		[]model.ContextComponent{otherFirst, first},
		[]model.ContextComponent{second},
		[]model.ContextComponent{otherSecond},
	))
	if len(findings) != 2 {
		t.Fatalf("findings = %d (%+v), want two groups", len(findings), findings)
	}
	if findings[0].Title != "read_file repeated the same tool call" || findings[1].Title != "exec_command repeated the same tool call" {
		t.Fatalf("order = %q then %q, want the first occurrence order", findings[0].Title, findings[1].Title)
	}
}

// Turns are processed in sequence order, so the baseline is the earliest
// occurrence even when the slice arrives shuffled.
func TestRepeatedToolCallOrdersOccurrencesByTurnSequence(t *testing.T) {
	session := sessionWithCallTurns(
		[]model.ContextComponent{repeatedCallComponent("call-1")},
		[]model.ContextComponent{repeatedCallComponent("call-2")},
	)
	session.Turns[0], session.Turns[1] = session.Turns[1], session.Turns[0]

	findings := analyzeRepeatedCalls(session)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	evidence := findings[0].Evidence
	if len(evidence) != 2 || evidence[0].TurnSequence != 1 || evidence[1].TurnSequence != 2 {
		t.Fatalf("evidence = %+v, want the baseline first", evidence)
	}
	if evidence[0].ToolCallID != "call-1" {
		t.Fatalf("baseline = %q, want the earliest call", evidence[0].ToolCallID)
	}
}

func TestRepeatedToolCallEvidenceIdentifiesEveryOccurrence(t *testing.T) {
	findings := analyzeRepeatedCalls(sessionWithCallTurns(
		[]model.ContextComponent{repeatedCallComponent("call-1")},
		[]model.ContextComponent{repeatedCallComponent("call-2")},
	))
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	evidence := findings[0].Evidence
	if len(evidence) != 2 {
		t.Fatalf("evidence = %+v, want both occurrences", evidence)
	}
	for i, want := range []string{"call-1", "call-2"} {
		got := evidence[i]
		if got.TurnID != fmt.Sprintf("turn-%d", i+1) || got.TurnSequence != i+1 || got.ToolCallID != want {
			t.Fatalf("evidence[%d] = %+v, want turn and call identified", i, got)
		}
		if got.ToolName != "exec_command" || got.OutputBytes != 4096 || got.Completeness != model.ContextCompletenessComplete {
			t.Fatalf("evidence[%d] = %+v, want the observed size and completeness", i, got)
		}
		if got.EstimatedTokens.ValueOrZero() != 1024 {
			t.Fatalf("evidence[%d] = %+v, want the occurrence measurement", i, got)
		}
	}
}

func TestRepeatedToolCallIgnoresIneligibleOccurrences(t *testing.T) {
	tests := []struct {
		name   string
		second model.ContextComponent
	}{
		{name: "different tool name", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.ToolName = "read_file" })},
		{name: "different argument fingerprint", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.ToolCallFingerprint = "v1:other" })},
		{name: "different output hash", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.ContentHash = "sha256:other" })},
		{name: "empty call id", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.ToolCallID = "" })},
		{name: "empty tool name", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.ToolName = "" })},
		{name: "empty fingerprint", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.ToolCallFingerprint = "" })},
		{name: "empty output hash", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.ContentHash = "" })},
		{name: "truncated result", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.Completeness = model.ContextCompletenessTruncated })},
		{name: "unavailable result", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.Completeness = model.ContextCompletenessUnavailable })},
		{name: "unknown completeness", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.Completeness = model.ContextCompletenessUnknown })},
		{name: "not a tool result", second: repeatedCallComponentWith(func(c *model.ContextComponent) { c.Kind = model.ContextHistory })},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := sessionWithCallTurns(
				[]model.ContextComponent{repeatedCallComponent("call-1")},
				[]model.ContextComponent{tt.second},
			)
			if findings := analyzeRepeatedCalls(session); len(findings) != 0 {
				t.Fatalf("findings = %+v, want none", findings)
			}
		})
	}
}

// The same call seen twice is one call. Two records that share a call ID must
// not be counted as a repeat of each other.
func TestRepeatedToolCallDoesNotCountOneCallTwice(t *testing.T) {
	session := sessionWithCallTurns(
		[]model.ContextComponent{repeatedCallComponent("call-1")},
		[]model.ContextComponent{repeatedCallComponent("call-1")},
	)
	if findings := analyzeRepeatedCalls(session); len(findings) != 0 {
		t.Fatalf("findings = %+v, want none for a single call", findings)
	}
}

func TestRepeatedToolCallIgnoresASingleCall(t *testing.T) {
	session := sessionWithCallTurns([]model.ContextComponent{repeatedCallComponent("call-1")})
	if findings := analyzeRepeatedCalls(session); len(findings) != 0 {
		t.Fatalf("findings = %+v, want none", findings)
	}
}

func TestRepeatedToolCallStopsOnCancelledContext(t *testing.T) {
	session := sessionWithCallTurns(
		[]model.ContextComponent{repeatedCallComponent("call-1")},
		[]model.ContextComponent{repeatedCallComponent("call-2")},
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if findings := NewRepeatedToolCall().Analyze(ctx, session); len(findings) != 0 {
		t.Fatalf("findings = %+v, want none once the context is cancelled", findings)
	}
}

func TestRepeatedToolCallDoesNotExposeFingerprintOrHash(t *testing.T) {
	component := repeatedCallComponent("call-1")
	findings := analyzeRepeatedCalls(sessionWithCallTurns(
		[]model.ContextComponent{component},
		[]model.ContextComponent{repeatedCallComponent("call-2")},
	))
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	raw, err := json.Marshal(findings[0])
	if err != nil {
		t.Fatalf("marshal finding: %v", err)
	}
	for _, secret := range []string{component.ToolCallFingerprint, component.ContentHash, "v1:", "sha256:", "fingerprint"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("finding exposed %q: %s", secret, raw)
		}
	}
}

func repeatedCallComponentWith(change func(*model.ContextComponent)) model.ContextComponent {
	component := repeatedCallComponent("call-2")
	change(&component)
	return component
}

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

// Every repeated occurrence has a measurement, so the total is the complete
// repeated output. The baseline is excluded because only the repeats can be
// avoided.
func TestRepeatedToolCallSumsEveryRepeatedMeasurement(t *testing.T) {
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
	if strings.Contains(findings[0].Description, "at least") {
		t.Fatalf("description = %q, want no lower-bound wording for a complete total", findings[0].Description)
	}
}

// No repeated occurrence has a measurement, so the total stays unavailable. The
// baseline is never substituted for it either, because only the repeats can be
// avoided.
func TestRepeatedToolCallKeepsEveryMissingMeasurementMissing(t *testing.T) {
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
	if !strings.Contains(findings[0].Description, "could not be sized") ||
		!strings.Contains(findings[0].Description, "unavailable") {
		t.Fatalf("description = %q, want the unavailable wording", findings[0].Description)
	}
	for _, invented := range []string{"0 tokens", "at least 0", "is 0 "} {
		if strings.Contains(findings[0].Description, invented) {
			t.Fatalf("description = %q, want no invented zero", findings[0].Description)
		}
	}
}

// One unmeasured repeat must not erase a measured one, and the sum must be
// presented as the observed portion rather than as the complete total.
func TestRepeatedToolCallReportsAPartialTotalAsALowerBound(t *testing.T) {
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
	description := findings[0].Description
	for _, want := range []string{"observed portion", "at least 700 estimated tokens", "1 of 2 repeated results", "remaining result has no token estimate"} {
		if !strings.Contains(description, want) {
			t.Fatalf("description = %q, want %q", description, want)
		}
	}
	for _, unwanted := range []string{"estimated repeated tool output is", "potential context contribution", "0 tokens"} {
		if strings.Contains(description, unwanted) {
			t.Fatalf("description = %q, must not claim a complete total via %q", description, unwanted)
		}
	}
}

// Several measured repeats and several missing ones keep both counts accurate.
func TestRepeatedToolCallReportsPartialCoverageAcrossManyOccurrences(t *testing.T) {
	measured := func(callID string, tokens int64) model.ContextComponent {
		component := repeatedCallComponent(callID)
		component.Measurement = model.NewMeasurement(tokens, model.MeasurementEstimated)
		return component
	}
	unmeasured := func(callID string) model.ContextComponent {
		component := repeatedCallComponent(callID)
		component.Measurement = model.Measurement{Kind: model.MeasurementUnknown}
		return component
	}

	findings := analyzeRepeatedCalls(sessionWithCallTurns(
		[]model.ContextComponent{measured("call-1", 5000)},
		[]model.ContextComponent{measured("call-2", 300)},
		[]model.ContextComponent{unmeasured("call-3")},
		[]model.ContextComponent{measured("call-4", 450)},
		[]model.ContextComponent{unmeasured("call-5")},
	))
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	estimated := findings[0].EstimatedTokens
	if estimated.Kind != model.MeasurementEstimated || estimated.ValueOrZero() != 750 {
		t.Fatalf("estimated tokens = %+v, want 750 estimated from the two measured repeats", estimated)
	}
	description := findings[0].Description
	for _, want := range []string{"at least 750 estimated tokens", "2 of 4 repeated results", "remaining 2 results have no token estimate"} {
		if !strings.Contains(description, want) {
			t.Fatalf("description = %q, want %q", description, want)
		}
	}
	if strings.Contains(description, "5000") || strings.Contains(description, "5750") {
		t.Fatalf("description = %q, want the baseline excluded from the total", description)
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
		if got.ToolName != "exec_command" || got.Completeness != model.ContextCompletenessComplete {
			t.Fatalf("evidence[%d] = %+v, want the observed completeness", i, got)
		}
		if got.OutputBytes == nil || *got.OutputBytes != 4096 {
			t.Fatalf("evidence[%d] = %+v, want the observed byte size", i, got)
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

func TestRepeatedToolCallReturnsNoFindingWhenCancelledBeforeAnyGroupCompletes(t *testing.T) {
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

// cancellingContext reports cancellation only after a fixed number of
// Err calls, so a test can cancel between turns deterministically.
type cancellingContext struct {
	context.Context
	remaining int
}

func (c *cancellingContext) Err() error {
	if c.remaining <= 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func TestRepeatedToolCallKeepsGroupsCompletedBeforeCancellation(t *testing.T) {
	session := sessionWithCallTurns(
		[]model.ContextComponent{repeatedCallComponent("call-1")},
		[]model.ContextComponent{repeatedCallComponent("call-2")},
		[]model.ContextComponent{repeatedCallComponent("call-3")},
	)
	ctx := &cancellingContext{Context: context.Background(), remaining: 2}

	findings := NewRepeatedToolCall().Analyze(ctx, session)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want the group completed before cancellation", findings)
	}
	evidence := findings[0].Evidence
	if len(evidence) != 2 || evidence[0].ToolCallID != "call-1" || evidence[1].ToolCallID != "call-2" {
		t.Fatalf("evidence = %+v, want only the turns observed before cancellation", evidence)
	}
}

func TestRepeatedToolCallFindingOmitsFingerprintAndContentHash(t *testing.T) {
	first := repeatedCallComponent("call-1")
	second := repeatedCallComponent("call-2")
	findings := analyzeRepeatedCalls(sessionWithCallTurns(
		[]model.ContextComponent{first},
		[]model.ContextComponent{second},
	))
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	raw, err := json.Marshal(findings[0])
	if err != nil {
		t.Fatalf("marshal finding: %v", err)
	}
	for _, secret := range []string{first.ToolCallFingerprint, first.ContentHash, "v1:", "sha256:", "fingerprint", "digest"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("finding exposed %q: %s", secret, raw)
		}
	}
}

func TestRepeatedToolCallEvidenceKeepsMissingOutputBytesMissing(t *testing.T) {
	tests := []struct {
		name        string
		contentByte *int64
		want        *int64
	}{
		{name: "unavailable", contentByte: nil, want: nil},
		{name: "observed zero", contentByte: model.Int64(0), want: model.Int64(0)},
		{name: "observed size", contentByte: model.Int64(4096), want: model.Int64(4096)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := repeatedCallComponent("call-1")
			first.ContentBytes = tt.contentByte
			second := repeatedCallComponent("call-2")
			second.ContentBytes = tt.contentByte

			findings := analyzeRepeatedCalls(sessionWithCallTurns(
				[]model.ContextComponent{first},
				[]model.ContextComponent{second},
			))
			if len(findings) != 1 {
				t.Fatalf("findings = %+v, want a finding even when the byte size is missing", findings)
			}
			for i, evidence := range findings[0].Evidence {
				switch {
				case tt.want == nil && evidence.OutputBytes != nil:
					t.Fatalf("evidence[%d] output bytes = %d, want missing", i, *evidence.OutputBytes)
				case tt.want != nil && (evidence.OutputBytes == nil || *evidence.OutputBytes != *tt.want):
					t.Fatalf("evidence[%d] output bytes = %v, want %d", i, evidence.OutputBytes, *tt.want)
				}
			}
		})
	}
}

func repeatedCallComponentWith(change func(*model.ContextComponent)) model.ContextComponent {
	component := repeatedCallComponent("call-2")
	change(&component)
	return component
}

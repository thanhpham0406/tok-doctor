package tool

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

const (
	RepeatedToolCallRuleID   = "TOOL002"
	RepeatedToolCallRuleName = "repeated-tool-call"

	minRepeatedCallOccurrences = 2
)

type RepeatedToolCall struct{}

func NewRepeatedToolCall() RepeatedToolCall {
	return RepeatedToolCall{}
}

type repeatedCallGroupKey struct {
	ToolName    string
	Fingerprint string
	OutputHash  string
}

type repeatedCallOccurrence struct {
	turn      model.Turn
	component model.ContextComponent
}

type repeatedCallGroup struct {
	key         repeatedCallGroupKey
	occurrences []repeatedCallOccurrence
	callIDs     map[string]struct{}
}

type repeatedCallBuilder struct {
	groups map[repeatedCallGroupKey]*repeatedCallGroup
	order  []*repeatedCallGroup
}

func (r RepeatedToolCall) Analyze(ctx context.Context, session model.Session) []model.Finding {
	builder := &repeatedCallBuilder{groups: map[repeatedCallGroupKey]*repeatedCallGroup{}}
	for _, turn := range turnsInSequenceOrder(session.Turns) {
		if ctx.Err() != nil {
			return builder.findings()
		}
		for _, component := range turn.ContextAttribution.Components {
			builder.observe(turn, component)
		}
	}
	return builder.findings()
}

func turnsInSequenceOrder(turns []model.Turn) []model.Turn {
	ordered := slices.Clone(turns)
	slices.SortStableFunc(ordered, func(a, b model.Turn) int {
		return cmp.Compare(a.Sequence, b.Sequence)
	})
	return ordered
}

func (b *repeatedCallBuilder) observe(turn model.Turn, component model.ContextComponent) {
	if !repeatedCallCandidate(component) {
		return
	}
	key := repeatedCallGroupKey{
		ToolName:    component.ToolName,
		Fingerprint: component.ToolCallFingerprint,
		OutputHash:  component.ContentHash,
	}
	group, ok := b.groups[key]
	if !ok {
		group = &repeatedCallGroup{key: key, callIDs: map[string]struct{}{}}
		b.groups[key] = group
		b.order = append(b.order, group)
	}
	if _, seen := group.callIDs[component.ToolCallID]; seen {
		return
	}
	group.callIDs[component.ToolCallID] = struct{}{}
	group.occurrences = append(group.occurrences, repeatedCallOccurrence{turn: turn, component: component})
}

func (b *repeatedCallBuilder) findings() []model.Finding {
	var findings []model.Finding
	for _, group := range b.order {
		if len(group.occurrences) < minRepeatedCallOccurrences {
			continue
		}
		findings = append(findings, group.finding())
	}
	return findings
}

func repeatedCallCandidate(component model.ContextComponent) bool {
	return component.Kind == model.ContextToolResult &&
		component.ToolCallID != "" &&
		component.ToolName != "" &&
		component.ToolCallFingerprint != "" &&
		component.ContentHash != "" &&
		component.Completeness == model.ContextCompletenessComplete
}

func (g *repeatedCallGroup) finding() model.Finding {
	estimate := estimatedRepeatedOutput(g.occurrences)
	return model.Finding{
		RuleID:          RepeatedToolCallRuleID,
		Name:            RepeatedToolCallRuleName,
		Severity:        model.SeverityMedium,
		Confidence:      model.ConfidenceHigh,
		Title:           fmt.Sprintf("%s repeated the same tool call", g.key.ToolName),
		Description:     repeatedCallDescription(g.key.ToolName, len(g.occurrences), estimate),
		EstimatedTokens: estimate.Measurement,
		Evidence:        g.evidence(),
		Recommendation:  "Reuse the earlier result when it is still valid, or narrow later calls to request only new information.",
	}
}

func repeatedCallDescription(toolName string, occurrences int, estimate repeatedOutputEstimate) string {
	description := fmt.Sprintf(
		"%s appears to have been called %d times with the same normalized arguments, and every complete result had the same content. The first occurrence is the baseline and the %d later ones are repeated calls.",
		toolName, occurrences, occurrences-1,
	)
	return description + " " + repeatedOutputEstimateDescription(estimate)
}

func repeatedOutputEstimateDescription(estimate repeatedOutputEstimate) string {
	if estimate.Available == 0 {
		return "Their repeated tool output could not be sized, so its token contribution is unavailable."
	}
	if estimate.Available == estimate.Total {
		return fmt.Sprintf(
			"Their estimated repeated tool output is %d tokens: a potential context contribution, not measured waste.",
			estimate.Measurement.ValueOrZero(),
		)
	}
	return fmt.Sprintf(
		"The observed portion of their repeated tool output is at least %d estimated tokens across %d of %d repeated results; the remaining %s no token estimate.",
		estimate.Measurement.ValueOrZero(), estimate.Available, estimate.Total,
		unmeasuredOccurrencesClause(estimate.Total-estimate.Available),
	)
}

func unmeasuredOccurrencesClause(count int) string {
	if count == 1 {
		return "result has"
	}
	return fmt.Sprintf("%d results have", count)
}

func (g *repeatedCallGroup) evidence() []model.FindingEvidence {
	evidence := make([]model.FindingEvidence, 0, len(g.occurrences))
	for _, occurrence := range g.occurrences {
		evidence = append(evidence, occurrence.evidence())
	}
	return evidence
}

func (o repeatedCallOccurrence) evidence() model.FindingEvidence {
	return model.FindingEvidence{
		TurnID:          o.turn.ID,
		TurnSequence:    o.turn.Sequence,
		ToolCallID:      o.component.ToolCallID,
		ToolName:        o.component.ToolName,
		OutputBytes:     outputByteSize(o.component.ContentBytes),
		EstimatedTokens: o.component.Measurement,
		Completeness:    o.component.Completeness,
	}
}

func outputByteSize(contentBytes *int64) *int64 {
	if contentBytes == nil {
		return nil
	}
	return model.Int64(*contentBytes)
}

type repeatedOutputEstimate struct {
	Measurement model.Measurement
	Available   int
	Total       int
}

func estimatedRepeatedOutput(occurrences []repeatedCallOccurrence) repeatedOutputEstimate {
	estimate := repeatedOutputEstimate{Total: len(occurrences) - 1}
	var total int64
	for _, occurrence := range occurrences[1:] {
		measurement := occurrence.component.Measurement
		if !measurement.Available() {
			continue
		}
		estimate.Available++
		total += measurement.ValueOrZero()
	}
	if estimate.Available == 0 {
		estimate.Measurement = model.Measurement{Kind: model.MeasurementUnknown}
		return estimate
	}
	estimate.Measurement = model.NewMeasurement(total, model.MeasurementEstimated)
	return estimate
}

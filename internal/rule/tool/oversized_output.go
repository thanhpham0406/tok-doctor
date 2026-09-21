package tool

import (
	"context"
	"fmt"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

const (
	OversizedOutputRuleID   = "TOOL001"
	OversizedOutputRuleName = "oversized-tool-output"

	DefaultMaxOutputBytes = int64(64 * 1024)
	highSeverityBytes     = int64(256 * 1024)
)

type OversizedOutput struct {
	maxBytes int64
}

func NewOversizedOutput(maxBytes int64) OversizedOutput {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxOutputBytes
	}
	return OversizedOutput{maxBytes: maxBytes}
}

func (r OversizedOutput) Analyze(ctx context.Context, session model.Session) []model.Finding {
	var findings []model.Finding
	for _, turn := range session.Turns {
		if err := ctx.Err(); err != nil {
			return findings
		}
		for _, component := range turn.ContextAttribution.Components {
			if component.Kind != model.ContextToolResult ||
				component.Completeness != model.ContextCompletenessComplete ||
				component.ContentBytes == nil ||
				*component.ContentBytes <= r.maxBytes {
				continue
			}
			findings = append(findings, oversizedOutputFinding(turn, component))
		}
	}
	return findings
}

func oversizedOutputFinding(turn model.Turn, component model.ContextComponent) model.Finding {
	bytes := *component.ContentBytes
	severity := model.SeverityMedium
	if bytes >= highSeverityBytes {
		severity = model.SeverityHigh
	}
	toolName := component.ToolName
	if toolName == "" {
		toolName = "Tool"
	}
	description := fmt.Sprintf("The transcript contains a complete tool output of %d bytes.", bytes)
	if component.Measurement.Kind == model.MeasurementEstimated && component.Measurement.Available() {
		description += " Its token contribution is estimated, not provider-reported."
	} else {
		description += " Its token contribution is unavailable."
	}
	return model.Finding{
		RuleID:          OversizedOutputRuleID,
		Name:            OversizedOutputRuleName,
		Severity:        severity,
		Confidence:      model.ConfidenceHigh,
		Title:           fmt.Sprintf("%s returned an oversized output", toolName),
		Description:     description,
		EstimatedTokens: component.Measurement,
		Evidence: []model.FindingEvidence{{
			TurnID:          turn.ID,
			TurnSequence:    turn.Sequence,
			ToolCallID:      component.ToolCallID,
			ToolName:        component.ToolName,
			OutputBytes:     model.Int64(bytes),
			EstimatedTokens: component.Measurement,
			Completeness:    component.Completeness,
		}},
		Recommendation: "Limit the command scope, filter the result, or read the output in smaller relevant sections.",
	}
}

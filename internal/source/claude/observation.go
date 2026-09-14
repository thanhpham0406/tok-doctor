package claude

import (
	"fmt"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func ObservationsFromSession(session model.Session) ([]model.Observation, error) {
	if session.ID == "" {
		return nil, fmt.Errorf("project claude session: id is required")
	}
	if session.Agent != "" && session.Agent != model.AgentClaude {
		return nil, fmt.Errorf("project claude session %s: unexpected agent %q", session.ID, session.Agent)
	}
	if session.Source != "" && session.Source != "claude" {
		return nil, fmt.Errorf("project claude session %s: unexpected source %q", session.ID, session.Source)
	}

	observations := make([]model.Observation, 0, len(session.Turns)+1)
	sessionObservation := projectClaudeSessionObservation(session)
	if err := sessionObservation.Validate(); err != nil {
		return nil, fmt.Errorf("project claude session %s: %w", session.ID, err)
	}
	observations = append(observations, sessionObservation)

	seenTurns := make(map[string]struct{}, len(session.Turns))
	for _, turn := range session.Turns {
		if turn.ID == "" {
			return nil, fmt.Errorf("project claude session %s: turn id is required", session.ID)
		}
		if _, ok := seenTurns[turn.ID]; ok {
			return nil, fmt.Errorf("project claude session %s: duplicate turn id %q", session.ID, turn.ID)
		}
		seenTurns[turn.ID] = struct{}{}

		turnObservation := projectClaudeTurnObservation(session, turn)
		if err := turnObservation.Validate(); err != nil {
			return nil, fmt.Errorf("project claude session %s turn %s: %w", session.ID, turn.ID, err)
		}
		observations = append(observations, turnObservation)
	}
	return observations, nil
}

func projectClaudeSessionObservation(session model.Session) model.Observation {
	usage := projectClaudeSessionUsage(session.Usage, acceptedTurnCount(session.Turns))
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            "claude:session:" + session.ID,
		Channel:       model.ObservationChannelSessionTranscript,
		Scope:         model.ObservationScopeSession,
		Source:        string(model.AgentClaude),
		Identity:      model.ObservationIdentity{SessionID: session.ID},
		StartedAt:     cloneClaudeTime(session.StartedAt),
		Model:         session.Model,
		Usage:         usage,
		Outcome:       model.ObservationOutcomeUnknown,
		Completeness:  claudeObservationCompleteness(usage),
	}
}

func projectClaudeTurnObservation(session model.Session, turn model.Turn) model.Observation {
	usage := projectClaudeTurnUsage(turn.Usage, turn.ID)
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            "claude:turn:" + turn.ID,
		Channel:       model.ObservationChannelSessionTranscript,
		Scope:         model.ObservationScopeTurn,
		Source:        string(model.AgentClaude),
		Identity:      model.ObservationIdentity{SessionID: session.ID, TurnID: turn.ID},
		StartedAt:     cloneClaudeTime(turn.Timestamp),
		Model:         claudeTurnModel(session, turn),
		Usage:         usage,
		Outcome:       model.ObservationOutcomeUnknown,
		Completeness:  claudeObservationCompleteness(usage),
	}
}

func claudeTurnModel(session model.Session, turn model.Turn) string {
	if turn.Model != "" {
		return turn.Model
	}
	return session.Model
}

func projectClaudeTurnUsage(usage model.Usage, record string) model.ObservationUsage {
	fresh := claudeSourceValueField(usage.Input, record, claudeFieldInputTokens)
	cached := claudeSourceValueField(claudeCacheRead(usage), record, claudeFieldCacheRead)
	created := claudeSourceValueField(claudeCacheWrite(usage), record, claudeFieldCacheCreation)
	output := claudeSourceValueField(usage.Output, record, claudeFieldOutputTokens)
	totalInput := deriveClaudeTotalInput(fresh, cached, created, record)
	return model.ObservationUsage{
		FreshInput:         fresh,
		CachedInput:        cached,
		CacheCreationInput: created,
		TotalInput:         totalInput,
		Output:             output,
		Total:              deriveClaudeTotal(totalInput, output, record),
	}
}

func projectClaudeSessionUsage(usage model.Usage, turnCount int) model.ObservationUsage {
	fresh := claudeAggregateField(usage.Input, turnCount)
	cached := claudeAggregateField(claudeCacheRead(usage), turnCount)
	created := claudeAggregateField(claudeCacheWrite(usage), turnCount)
	output := claudeAggregateField(usage.Output, turnCount)
	totalInput := deriveClaudeTotalInput(fresh, cached, created, "")
	return model.ObservationUsage{
		FreshInput:         fresh,
		CachedInput:        cached,
		CacheCreationInput: created,
		TotalInput:         totalInput,
		Output:             output,
		Total:              deriveClaudeTotal(totalInput, output, ""),
	}
}

func claudeCacheRead(usage model.Usage) model.Measurement {
	if usage.Billable == nil {
		return model.Measurement{}
	}
	return usage.Billable.CacheRead
}

func claudeCacheWrite(usage model.Usage) model.Measurement {
	if usage.Billable == nil {
		return model.Measurement{}
	}
	return usage.Billable.CacheWrite
}

func claudeSourceValueField(measurement model.Measurement, record, field string) model.Measurement {
	switch {
	case measurement.Kind == model.MeasurementMeasured && measurement.Available():
		return model.Measurement{
			Value:    model.Int64(measurement.ValueOrZero()),
			Kind:     model.MeasurementMeasured,
			Evidence: []model.Evidence{claudeSourceValueEvidence(record, field)},
		}
	case measurement.Kind != "":
		return model.Measurement{Kind: model.MeasurementUnknown}
	default:
		return model.Measurement{}
	}
}

func claudeAggregateField(measurement model.Measurement, count int) model.Measurement {
	switch {
	case measurement.Available():
		return model.Measurement{
			Value: model.Int64(measurement.ValueOrZero()),
			Kind:  model.MeasurementDerived,
			Evidence: []model.Evidence{{
				Kind:      model.EvidenceAggregate,
				Source:    claudeObservationSource,
				Operation: model.AggregateSum,
				Count:     count,
			}},
		}
	case measurement.Kind != "":
		return model.Measurement{Kind: model.MeasurementUnknown}
	default:
		return model.Measurement{}
	}
}

func deriveClaudeTotalInput(fresh, cached, created model.Measurement, record string) model.Measurement {
	if !fresh.Available() || !cached.Available() || !created.Available() {
		return model.Measurement{}
	}
	sum := fresh.ValueOrZero() + cached.ValueOrZero() + created.ValueOrZero()
	return model.Measurement{
		Value: model.Int64(sum),
		Kind:  model.MeasurementDerived,
		Evidence: []model.Evidence{{
			Kind:      model.EvidenceAggregate,
			Source:    claudeObservationSource,
			Record:    record,
			Operation: model.AggregateSum,
			Count:     3,
		}},
	}
}

func deriveClaudeTotal(totalInput, output model.Measurement, record string) model.Measurement {
	if !totalInput.Available() || !output.Available() {
		return model.Measurement{}
	}
	return model.Measurement{
		Value: model.Int64(totalInput.ValueOrZero() + output.ValueOrZero()),
		Kind:  model.MeasurementDerived,
		Evidence: []model.Evidence{{
			Kind:      model.EvidenceAggregate,
			Source:    claudeObservationSource,
			Record:    record,
			Operation: model.AggregateSum,
			Count:     2,
		}},
	}
}

func claudeObservationCompleteness(usage model.ObservationUsage) model.ObservationCompleteness {
	if !claudeHasRecordedUsage(usage) {
		return model.ObservationCompletenessUnknown
	}
	if usage.FreshInput.Available() && usage.CachedInput.Available() && usage.CacheCreationInput.Available() &&
		usage.Output.Available() && usage.TotalInput.Available() && usage.Total.Available() {
		return model.ObservationCompletenessComplete
	}
	return model.ObservationCompletenessPartial
}

func claudeHasRecordedUsage(usage model.ObservationUsage) bool {
	for _, measurement := range []model.Measurement{
		usage.FreshInput, usage.CachedInput, usage.CacheCreationInput,
		usage.TotalInput, usage.Output, usage.ReasoningOutput, usage.Total,
	} {
		if measurement.Kind != "" {
			return true
		}
	}
	return false
}

func acceptedTurnCount(turns []model.Turn) int {
	count := 0
	for _, turn := range turns {
		if turn.Usage.HasAuthoritativeUsage() {
			count++
		}
	}
	return count
}

func cloneClaudeTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

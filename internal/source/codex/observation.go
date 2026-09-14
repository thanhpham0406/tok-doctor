package codex

import (
	"fmt"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

const (
	codexObservationEvidenceSource = "codex_rollout"
	codexFreshInputField           = "total_token_usage.input_tokens-total_token_usage.cached_input_tokens"
)

func ObservationsFromSession(session model.Session) ([]model.Observation, error) {
	if session.ID == "" {
		return nil, fmt.Errorf("project codex session: id is required")
	}
	if session.Agent != "" && session.Agent != model.AgentCodex {
		return nil, fmt.Errorf("project codex session %s: unexpected agent %q", session.ID, session.Agent)
	}
	if session.Source != "" && session.Source != string(model.AgentCodex) {
		return nil, fmt.Errorf("project codex session %s: unexpected source %q", session.ID, session.Source)
	}

	observations := make([]model.Observation, 0, len(session.Turns)+1)
	sessionObservation := projectSessionObservation(session)
	if err := sessionObservation.Validate(); err != nil {
		return nil, fmt.Errorf("project codex session %s: %w", session.ID, err)
	}
	observations = append(observations, sessionObservation)

	seenTurns := make(map[string]struct{}, len(session.Turns))
	for _, turn := range session.Turns {
		if turn.ID == "" {
			return nil, fmt.Errorf("project codex session %s: turn id is required", session.ID)
		}
		if _, ok := seenTurns[turn.ID]; ok {
			return nil, fmt.Errorf("project codex session %s: duplicate turn id %q", session.ID, turn.ID)
		}
		seenTurns[turn.ID] = struct{}{}

		turnObservation := projectTurnObservation(session, turn)
		if err := turnObservation.Validate(); err != nil {
			return nil, fmt.Errorf("project codex session %s turn %s: %w", session.ID, turn.ID, err)
		}
		observations = append(observations, turnObservation)
	}
	return observations, nil
}

func projectSessionObservation(session model.Session) model.Observation {
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            "codex:session:" + session.ID,
		Channel:       model.ObservationChannelSessionTranscript,
		Scope:         model.ObservationScopeSession,
		Source:        string(model.AgentCodex),
		Identity:      model.ObservationIdentity{SessionID: session.ID},
		StartedAt:     cloneTime(session.StartedAt),
		Model:         session.Model,
		Usage:         projectObservationUsage(session.Usage),
		Outcome:       model.ObservationOutcomeUnknown,
		Completeness:  observationCompleteness(session.Usage),
	}
}

func projectTurnObservation(session model.Session, turn model.Turn) model.Observation {
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            "codex:turn:" + turn.ID,
		Channel:       model.ObservationChannelSessionTranscript,
		Scope:         model.ObservationScopeTurn,
		Source:        string(model.AgentCodex),
		Identity:      model.ObservationIdentity{SessionID: session.ID, TurnID: turn.ID},
		StartedAt:     cloneTime(turn.Timestamp),
		Model:         turnModel(session, turn),
		Usage:         projectObservationUsage(turn.Usage),
		Outcome:       model.ObservationOutcomeUnknown,
		Completeness:  observationCompleteness(turn.Usage),
	}
}

func turnModel(session model.Session, turn model.Turn) string {
	if turn.Model != "" {
		return turn.Model
	}
	return session.Model
}

func projectObservationUsage(usage model.Usage) model.ObservationUsage {
	return model.ObservationUsage{
		FreshInput:      derivedFreshInput(usage.Input, usage.Cached),
		CachedInput:     cloneMeasurement(usage.Cached),
		TotalInput:      cloneMeasurement(usage.Input),
		Output:          cloneMeasurement(usage.Output),
		ReasoningOutput: cloneMeasurement(usage.Reasoning),
		Total:           cloneMeasurement(usage.Total),
	}
}

func derivedFreshInput(totalInput, cachedInput model.Measurement) model.Measurement {
	if !totalInput.Available() || !cachedInput.Available() {
		return model.Measurement{}
	}
	fresh := totalInput.ValueOrZero() - cachedInput.ValueOrZero()
	if fresh < 0 {
		return model.Measurement{Kind: model.MeasurementUnknown}
	}
	return model.Measurement{
		Value: model.Int64(fresh),
		Kind:  model.MeasurementDerived,
		Evidence: []model.Evidence{{
			Kind:   model.EvidenceProvenance,
			Source: codexObservationEvidenceSource,
			Field:  codexFreshInputField,
		}},
	}
}

func observationCompleteness(usage model.Usage) model.ObservationCompleteness {
	if !usage.HasUsage() {
		return model.ObservationCompletenessUnknown
	}
	if usage.Input.Available() && usage.Output.Available() && usage.Total.Available() {
		return model.ObservationCompletenessComplete
	}
	return model.ObservationCompletenessPartial
}

func cloneMeasurement(measurement model.Measurement) model.Measurement {
	cloned := model.Measurement{Kind: measurement.Kind}
	if measurement.Value != nil {
		cloned.Value = model.Int64(*measurement.Value)
	}
	if len(measurement.Evidence) > 0 {
		cloned.Evidence = append([]model.Evidence(nil), measurement.Evidence...)
	}
	return cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

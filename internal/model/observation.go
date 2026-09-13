package model

import (
	"fmt"
	"time"
)

const ObservationSchemaVersion = 1

type ObservationChannel string

const (
	ObservationChannelGateway           ObservationChannel = "gateway"
	ObservationChannelAgentTelemetry    ObservationChannel = "agent_telemetry"
	ObservationChannelSessionTranscript ObservationChannel = "session_transcript"
)

func ValidObservationChannel(channel ObservationChannel) bool {
	switch channel {
	case ObservationChannelGateway, ObservationChannelAgentTelemetry, ObservationChannelSessionTranscript:
		return true
	default:
		return false
	}
}

type ObservationScope string

const (
	ObservationScopeRequest ObservationScope = "request"
	ObservationScopeTurn    ObservationScope = "turn"
	ObservationScopeSession ObservationScope = "session"
)

func ValidObservationScope(scope ObservationScope) bool {
	switch scope {
	case ObservationScopeRequest, ObservationScopeTurn, ObservationScopeSession:
		return true
	default:
		return false
	}
}

type ObservationCompleteness string

const (
	ObservationCompletenessComplete ObservationCompleteness = "complete"
	ObservationCompletenessPartial  ObservationCompleteness = "partial"
	ObservationCompletenessUnknown  ObservationCompleteness = "unknown"
)

func ValidObservationCompleteness(value ObservationCompleteness) bool {
	switch value {
	case ObservationCompletenessComplete, ObservationCompletenessPartial, ObservationCompletenessUnknown:
		return true
	default:
		return false
	}
}

type ObservationOutcome string

const (
	ObservationOutcomeUnknown          ObservationOutcome = "unknown"
	ObservationOutcomeSucceeded        ObservationOutcome = "succeeded"
	ObservationOutcomeProviderError    ObservationOutcome = "provider_error"
	ObservationOutcomeTransportFailure ObservationOutcome = "transport_failure"
	ObservationOutcomeCanceled         ObservationOutcome = "canceled"
	ObservationOutcomeTruncated        ObservationOutcome = "truncated"
)

func ValidObservationOutcome(outcome ObservationOutcome) bool {
	switch outcome {
	case ObservationOutcomeUnknown, ObservationOutcomeSucceeded, ObservationOutcomeProviderError,
		ObservationOutcomeTransportFailure, ObservationOutcomeCanceled, ObservationOutcomeTruncated:
		return true
	default:
		return false
	}
}

type ObservationIdentity struct {
	ExchangeID             string `json:"exchangeId,omitempty"`
	AgentRequestID         string `json:"agentRequestId,omitempty"`
	ProviderRequestID      string `json:"providerRequestId,omitempty"`
	ResponseObjectID       string `json:"responseObjectId,omitempty"`
	ParentResponseObjectID string `json:"parentResponseObjectId,omitempty"`
	SessionID              string `json:"sessionId,omitempty"`
	TurnID                 string `json:"turnId,omitempty"`
	InvocationID           string `json:"invocationId,omitempty"`
	ChainID                string `json:"chainId,omitempty"`
}

type ObservationUsage struct {
	FreshInput         Measurement `json:"freshInput"`
	CachedInput        Measurement `json:"cachedInput"`
	CacheCreationInput Measurement `json:"cacheCreationInput"`
	TotalInput         Measurement `json:"totalInput"`
	Output             Measurement `json:"output"`
	ReasoningOutput    Measurement `json:"reasoningOutput"`
	Total              Measurement `json:"total"`
}

type Observation struct {
	SchemaVersion int                     `json:"schemaVersion"`
	ID            string                  `json:"id"`
	Channel       ObservationChannel      `json:"channel"`
	Scope         ObservationScope        `json:"scope"`
	Source        string                  `json:"source"`
	Identity      ObservationIdentity     `json:"identity"`
	StartedAt     *time.Time              `json:"startedAt,omitempty"`
	FinishedAt    *time.Time              `json:"finishedAt,omitempty"`
	Model         string                  `json:"model,omitempty"`
	Usage         ObservationUsage        `json:"usage"`
	Outcome       ObservationOutcome      `json:"outcome"`
	Completeness  ObservationCompleteness `json:"completeness"`
	Evidence      []Evidence              `json:"evidence,omitempty"`
}

func (o Observation) Validate() error {
	if o.SchemaVersion != ObservationSchemaVersion {
		return fmt.Errorf("validate observation %s: unsupported schema version %d", o.ID, o.SchemaVersion)
	}
	if o.ID == "" {
		return fmt.Errorf("validate observation: id is required")
	}
	if !ValidObservationChannel(o.Channel) {
		return fmt.Errorf("validate observation %s: invalid channel %q", o.ID, o.Channel)
	}
	if !ValidObservationScope(o.Scope) {
		return fmt.Errorf("validate observation %s: invalid scope %q", o.ID, o.Scope)
	}
	if o.Source == "" {
		return fmt.Errorf("validate observation %s: source is required", o.ID)
	}
	if !ValidObservationCompleteness(o.Completeness) {
		return fmt.Errorf("validate observation %s: invalid completeness %q", o.ID, o.Completeness)
	}
	if !ValidObservationOutcome(o.Outcome) {
		return fmt.Errorf("validate observation %s: invalid outcome %q", o.ID, o.Outcome)
	}
	if o.StartedAt != nil && o.FinishedAt != nil && o.FinishedAt.Before(*o.StartedAt) {
		return fmt.Errorf("validate observation %s: finishedAt precedes startedAt", o.ID)
	}
	for _, field := range []struct {
		name        string
		measurement Measurement
	}{
		{"usage.freshInput", o.Usage.FreshInput},
		{"usage.cachedInput", o.Usage.CachedInput},
		{"usage.cacheCreationInput", o.Usage.CacheCreationInput},
		{"usage.totalInput", o.Usage.TotalInput},
		{"usage.output", o.Usage.Output},
		{"usage.reasoningOutput", o.Usage.ReasoningOutput},
		{"usage.total", o.Usage.Total},
	} {
		if err := validateObservationMeasurement(field.name, field.measurement); err != nil {
			return err
		}
	}
	for i, e := range o.Evidence {
		if !ValidEvidenceKind(e.Kind) {
			return fmt.Errorf("validate observation %s evidence[%d]: invalid evidence kind %q", o.ID, i, e.Kind)
		}
	}
	return nil
}

func validateObservationMeasurement(field string, m Measurement) error {
	for i, e := range m.Evidence {
		if !ValidEvidenceKind(e.Kind) {
			return fmt.Errorf("validate observation %s evidence[%d]: invalid evidence kind %q", field, i, e.Kind)
		}
	}
	if m.Kind != "" && !ValidMeasurementKind(m.Kind) {
		return fmt.Errorf("validate observation %s: unrecognized measurement kind %q", field, m.Kind)
	}
	if m.Value == nil {
		if m.Kind != "" && m.Kind != MeasurementUnknown {
			return fmt.Errorf("validate observation %s: measurement kind %q requires a value", field, m.Kind)
		}
		return nil
	}
	if m.Kind == "" {
		return fmt.Errorf("validate observation %s: measurement value requires a kind", field)
	}
	if m.Kind == MeasurementUnknown {
		return fmt.Errorf("validate observation %s: measurement value requires a usable kind", field)
	}
	return nil
}

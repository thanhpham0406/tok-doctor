package gateway

import (
	"fmt"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

const (
	observationEvidenceSource = "gateway"

	observationFieldInputTokens              = "response.providerUsage.inputTokens"
	observationFieldCacheReadInputTokens     = "response.providerUsage.cacheReadInputTokens"
	observationFieldCacheCreationInputTokens = "response.providerUsage.cacheCreationInputTokens"
	observationFieldTotalInputTokens         = "response.providerUsage.totalInputTokens"
	observationFieldOutputTokens             = "response.providerUsage.outputTokens"
	observationFieldReasoningOutputTokens    = "response.providerUsage.reasoningOutputTokens"
	observationFieldTotalTokens              = "response.providerUsage.totalTokens"

	observationFieldDerivedFreshInput = "response.providerUsage.totalInput-cacheReadInputTokens"
)

func ObservationFromExchange(e Exchange) (model.Observation, error) {
	if e.Kind != RequestKindModel {
		return model.Observation{}, fmt.Errorf("project gateway exchange %s: unsupported request kind %q", e.ID, e.Kind)
	}
	if e.ID == "" {
		return model.Observation{}, fmt.Errorf("project gateway exchange: id is required")
	}
	id, err := gatewayObservationID(e.Profile, e.ID)
	if err != nil {
		return model.Observation{}, fmt.Errorf("project gateway exchange %s: %w", e.ID, err)
	}
	source := exchangeObservationSource(e)
	if source == "" {
		return model.Observation{}, fmt.Errorf("project gateway exchange %s: source hint and profile are empty", e.ID)
	}

	providerUsage := e.Response.ProviderUsage
	observation := model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            id,
		Channel:       model.ObservationChannelGateway,
		Scope:         model.ObservationScopeRequest,
		Source:        source,
		Identity:      exchangeObservationIdentity(e),
		StartedAt:     exchangeObservationStartedAt(e),
		Model:         exchangeObservationModel(e),
		Usage:         observationUsage(providerUsage, e.ID),
		Outcome:       observationOutcome(e.Outcome),
		Completeness:  observationCompleteness(e.Outcome, providerUsage),
	}
	if err := validateProjectedObservation(e, observation); err != nil {
		return model.Observation{}, err
	}
	return observation, nil
}

func validateProjectedObservation(e Exchange, observation model.Observation) error {
	if err := observation.Validate(); err != nil {
		return fmt.Errorf("project gateway exchange %s: %w", e.ID, err)
	}
	return nil
}

func gatewayObservationID(profile, exchangeID string) (string, error) {
	if exchangeID == "" {
		return "", fmt.Errorf("exchange id is required to build observation id")
	}
	if profile == "" {
		return "", fmt.Errorf("profile is required to build observation id")
	}
	return "gateway:" + profile + ":" + exchangeID, nil
}

func exchangeObservationSource(e Exchange) string {
	if e.SourceHint != "" {
		return e.SourceHint
	}
	return e.Profile
}

func exchangeObservationModel(e Exchange) string {
	if e.Response.Model != "" {
		return e.Response.Model
	}
	return e.Model
}

func exchangeObservationStartedAt(e Exchange) *time.Time {
	if e.StartedAt.IsZero() {
		return nil
	}
	started := e.StartedAt
	return &started
}

func exchangeObservationIdentity(e Exchange) model.ObservationIdentity {
	identity := model.ObservationIdentity{
		ExchangeID:        e.ID,
		ProviderRequestID: e.Response.ProviderRequestID,
		ResponseObjectID:  e.Response.ResponseObjectID,
	}
	if metadata := e.Request.Metadata.OpenAIResponses; metadata != nil {
		identity.ParentResponseObjectID = metadata.PreviousResponseID
	}
	return identity
}

func observationOutcome(outcome RequestOutcome) model.ObservationOutcome {
	switch outcome {
	case OutcomeUpstreamOK:
		return model.ObservationOutcomeSucceeded
	case OutcomeUpstreamHTTPError:
		return model.ObservationOutcomeProviderError
	case OutcomeTransportFailure:
		return model.ObservationOutcomeTransportFailure
	case OutcomeClientCanceled:
		return model.ObservationOutcomeCanceled
	case OutcomeStreamTruncated:
		return model.ObservationOutcomeTruncated
	default:
		return model.ObservationOutcomeUnknown
	}
}

func observationCompleteness(outcome RequestOutcome, providerUsage *ProviderUsage) model.ObservationCompleteness {
	switch outcome {
	case OutcomeStreamTruncated:
		return model.ObservationCompletenessPartial
	case OutcomeUpstreamHTTPError, OutcomeTransportFailure, OutcomeClientCanceled:
		return model.ObservationCompletenessComplete
	case OutcomeUpstreamOK:
		return successfulRequestCompleteness(providerUsage)
	default:
		return model.ObservationCompletenessUnknown
	}
}

func successfulRequestCompleteness(providerUsage *ProviderUsage) model.ObservationCompleteness {
	if providerUsage == nil {
		return model.ObservationCompletenessPartial
	}
	switch providerUsage.Source {
	case string(ProtocolAnthropicMessages):
		if providerUsage.InputTokens != nil && providerUsage.OutputTokens != nil {
			return model.ObservationCompletenessComplete
		}
	case string(ProtocolOpenAIResponses):
		hasInput := providerUsage.InputTokens != nil || providerUsage.TotalInputTokens != nil
		if hasInput && providerUsage.OutputTokens != nil {
			return model.ObservationCompletenessComplete
		}
	}
	return model.ObservationCompletenessPartial
}

func observationUsage(providerUsage *ProviderUsage, record string) model.ObservationUsage {
	if providerUsage == nil {
		return model.ObservationUsage{}
	}
	kinds := providerFieldKindsFor(providerUsage)
	switch providerUsage.Source {
	case string(ProtocolAnthropicMessages):
		return anthropicObservationUsage(providerUsage, kinds, record)
	case string(ProtocolOpenAIResponses):
		return openAIResponsesObservationUsage(providerUsage, kinds, record)
	default:
		return unknownProviderObservationUsage(providerUsage, kinds, record)
	}
}

func anthropicObservationUsage(providerUsage *ProviderUsage, kinds providerFieldKinds, record string) model.ObservationUsage {
	observed := ProviderUsageToObserved(providerUsage)
	return model.ObservationUsage{
		FreshInput:         directObservationMeasurement(providerUsage.InputTokens, kinds.Fresh, record, observationFieldInputTokens),
		CachedInput:        directObservationMeasurement(providerUsage.CacheReadInputTokens, kinds.Cached, record, observationFieldCacheReadInputTokens),
		CacheCreationInput: directObservationMeasurement(providerUsage.CacheCreationInputTokens, kinds.CacheCreation, record, observationFieldCacheCreationInputTokens),
		TotalInput:         anthropicTotalInput(providerUsage, observed, kinds, record),
		Output:             directObservationMeasurement(providerUsage.OutputTokens, kinds.Output, record, observationFieldOutputTokens),
		ReasoningOutput:    directObservationMeasurement(providerUsage.ReasoningOutputTokens, kinds.Reasoning, record, observationFieldReasoningOutputTokens),
		Total:              derivedObservationTotal(providerUsage, observed, kinds, record),
	}
}

func anthropicTotalInput(providerUsage *ProviderUsage, observed ObservedUsage, kinds providerFieldKinds, record string) model.Measurement {
	if providerUsage.TotalInputTokens != nil {
		return directObservationMeasurement(providerUsage.TotalInputTokens, kinds.TotalInput, record, observationFieldTotalInputTokens)
	}
	if providerUsage.InputTokens == nil {
		return model.Measurement{}
	}
	count := 1
	if providerUsage.CacheReadInputTokens != nil {
		count++
	}
	if providerUsage.CacheCreationInputTokens != nil {
		count++
	}
	return model.Measurement{
		Value:    model.Int64(observed.TotalInput),
		Kind:     kinds.TotalInput,
		Evidence: []model.Evidence{aggregateObservationEvidence(record, count)},
	}
}

func openAIResponsesObservationUsage(providerUsage *ProviderUsage, kinds providerFieldKinds, record string) model.ObservationUsage {
	observed := ProviderUsageToObserved(providerUsage)
	return model.ObservationUsage{
		FreshInput:      openAIFreshInput(providerUsage, observed, record),
		CachedInput:     directObservationMeasurement(providerUsage.CacheReadInputTokens, kinds.Cached, record, observationFieldCacheReadInputTokens),
		TotalInput:      openAITotalInput(providerUsage, kinds, record),
		Output:          directObservationMeasurement(providerUsage.OutputTokens, kinds.Output, record, observationFieldOutputTokens),
		ReasoningOutput: directObservationMeasurement(providerUsage.ReasoningOutputTokens, kinds.Reasoning, record, observationFieldReasoningOutputTokens),
		Total:           derivedObservationTotal(providerUsage, observed, kinds, record),
	}
}

func openAITotalInput(providerUsage *ProviderUsage, kinds providerFieldKinds, record string) model.Measurement {
	if providerUsage.TotalInputTokens != nil {
		return directObservationMeasurement(providerUsage.TotalInputTokens, kinds.TotalInput, record, observationFieldTotalInputTokens)
	}
	if providerUsage.InputTokens != nil {
		return directObservationMeasurement(providerUsage.InputTokens, kinds.TotalInput, record, observationFieldInputTokens)
	}
	return model.Measurement{}
}

func openAIFreshInput(providerUsage *ProviderUsage, observed ObservedUsage, record string) model.Measurement {
	hasTotalInput := providerUsage.InputTokens != nil || providerUsage.TotalInputTokens != nil
	if !hasTotalInput || providerUsage.CacheReadInputTokens == nil {
		return model.Measurement{}
	}
	return model.Measurement{
		Value:    model.Int64(observed.RawInput),
		Kind:     model.MeasurementDerived,
		Evidence: []model.Evidence{provenanceObservationEvidence(record, observationFieldDerivedFreshInput)},
	}
}

func derivedObservationTotal(providerUsage *ProviderUsage, observed ObservedUsage, kinds providerFieldKinds, record string) model.Measurement {
	if providerUsage.TotalTokens != nil {
		return directObservationMeasurement(providerUsage.TotalTokens, kinds.Total, record, observationFieldTotalTokens)
	}
	total, ok := derivedTotal(providerUsage, observed)
	if !ok {
		return model.Measurement{}
	}
	return model.Measurement{
		Value:    model.Int64(total),
		Kind:     kinds.Total,
		Evidence: []model.Evidence{aggregateObservationEvidence(record, 2)},
	}
}

func unknownProviderObservationUsage(providerUsage *ProviderUsage, kinds providerFieldKinds, record string) model.ObservationUsage {
	return model.ObservationUsage{
		TotalInput:      directObservationMeasurement(providerUsage.TotalInputTokens, kinds.TotalInput, record, observationFieldTotalInputTokens),
		Output:          directObservationMeasurement(providerUsage.OutputTokens, kinds.Output, record, observationFieldOutputTokens),
		ReasoningOutput: directObservationMeasurement(providerUsage.ReasoningOutputTokens, kinds.Reasoning, record, observationFieldReasoningOutputTokens),
		Total:           directObservationMeasurement(providerUsage.TotalTokens, kinds.Total, record, observationFieldTotalTokens),
	}
}

func directObservationMeasurement(value *int64, kind model.MeasurementKind, record, field string) model.Measurement {
	if value == nil {
		return model.Measurement{}
	}
	return model.Measurement{
		Value:    model.Int64(*value),
		Kind:     kind,
		Evidence: []model.Evidence{sourceValueObservationEvidence(record, field)},
	}
}

func sourceValueObservationEvidence(record, field string) model.Evidence {
	return model.Evidence{
		Kind:   model.EvidenceSourceValue,
		Source: observationEvidenceSource,
		Record: record,
		Field:  field,
	}
}

func aggregateObservationEvidence(record string, count int) model.Evidence {
	return model.Evidence{
		Kind:      model.EvidenceAggregate,
		Source:    observationEvidenceSource,
		Record:    record,
		Operation: model.AggregateSum,
		Count:     count,
	}
}

func provenanceObservationEvidence(record, field string) model.Evidence {
	return model.Evidence{
		Kind:   model.EvidenceProvenance,
		Source: observationEvidenceSource,
		Record: record,
		Field:  field,
	}
}

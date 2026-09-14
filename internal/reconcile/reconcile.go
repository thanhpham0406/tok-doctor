package reconcile

import (
	"fmt"

	"github.com/thanhpham0406/tok-doctor/internal/match"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type Status string

const (
	StatusEqual       Status = "equal"
	StatusDifferent   Status = "different"
	StatusUnavailable Status = "unavailable"
)

type FieldComparison struct {
	Field  string
	Left   model.Measurement
	Right  model.Measurement
	Delta  *int64
	Status Status
}

type Result struct {
	GatewayObservationID    string
	TranscriptObservationID string
	Confidence              match.Confidence
	Reasons                 []match.Reason
	Fields                  []FieldComparison
	Status                  Status
}

type Pair struct {
	Gateway    model.Observation
	Transcript model.Observation
	Match      match.ObservationMatch
}

func Reconcile(pair Pair) (Result, error) {
	if err := validatePair(pair); err != nil {
		return Result{}, err
	}

	fields := make([]FieldComparison, 0, len(usageFields))
	for _, field := range usageFields {
		fields = append(fields, compareField(field.name, field.pick(pair.Gateway.Usage), field.pick(pair.Transcript.Usage)))
	}

	return Result{
		GatewayObservationID:    pair.Gateway.ID,
		TranscriptObservationID: pair.Transcript.ID,
		Confidence:              pair.Match.Confidence,
		Reasons:                 append([]match.Reason(nil), pair.Match.Reasons...),
		Fields:                  fields,
		Status:                  overallStatus(fields),
	}, nil
}

type fieldSpec struct {
	name string
	pick func(model.ObservationUsage) model.Measurement
}

var usageFields = []fieldSpec{
	{"freshInput", func(u model.ObservationUsage) model.Measurement { return u.FreshInput }},
	{"cachedInput", func(u model.ObservationUsage) model.Measurement { return u.CachedInput }},
	{"cacheCreationInput", func(u model.ObservationUsage) model.Measurement { return u.CacheCreationInput }},
	{"totalInput", func(u model.ObservationUsage) model.Measurement { return u.TotalInput }},
	{"output", func(u model.ObservationUsage) model.Measurement { return u.Output }},
	{"reasoningOutput", func(u model.ObservationUsage) model.Measurement { return u.ReasoningOutput }},
	{"total", func(u model.ObservationUsage) model.Measurement { return u.Total }},
}

func compareField(name string, left, right model.Measurement) FieldComparison {
	comparison := FieldComparison{
		Field:  name,
		Left:   cloneMeasurement(left),
		Right:  cloneMeasurement(right),
		Status: StatusUnavailable,
	}
	if !left.Available() || !right.Available() {
		return comparison
	}
	delta := right.ValueOrZero() - left.ValueOrZero()
	comparison.Delta = model.Int64(delta)
	if delta == 0 {
		comparison.Status = StatusEqual
	} else {
		comparison.Status = StatusDifferent
	}
	return comparison
}

func overallStatus(fields []FieldComparison) Status {
	comparable := 0
	different := false
	for _, field := range fields {
		switch field.Status {
		case StatusEqual:
			comparable++
		case StatusDifferent:
			comparable++
			different = true
		}
	}
	if comparable == 0 {
		return StatusUnavailable
	}
	if different {
		return StatusDifferent
	}
	return StatusEqual
}

func validatePair(pair Pair) error {
	if pair.Match.Status != match.StatusMatched {
		return fmt.Errorf("reconcile match %s: status %q is not matched", matchResultLabel(pair.Match), pair.Match.Status)
	}
	if pair.Match.GatewayObservationID == "" || pair.Match.TranscriptObservationID == "" {
		return fmt.Errorf("reconcile match %s: match result must reference both a gateway and a transcript observation", matchResultLabel(pair.Match))
	}
	if len(pair.Match.CandidateTranscriptIDs) != 0 {
		return fmt.Errorf("reconcile match %s: matched result must not carry candidate transcript ids", matchResultLabel(pair.Match))
	}
	if err := pair.Gateway.Validate(); err != nil {
		return err
	}
	if err := pair.Transcript.Validate(); err != nil {
		return err
	}
	if pair.Gateway.ID == pair.Transcript.ID {
		return fmt.Errorf("reconcile observation %s: same observation on both sides", pair.Gateway.ID)
	}
	if pair.Match.GatewayObservationID != pair.Gateway.ID {
		return fmt.Errorf("reconcile observation %s: match gateway id %q does not match observation", pair.Gateway.ID, pair.Match.GatewayObservationID)
	}
	if pair.Match.TranscriptObservationID != pair.Transcript.ID {
		return fmt.Errorf("reconcile observation %s: match transcript id %q does not match observation", pair.Transcript.ID, pair.Match.TranscriptObservationID)
	}
	if pair.Gateway.Channel != model.ObservationChannelGateway || pair.Gateway.Scope != model.ObservationScopeRequest {
		return fmt.Errorf("reconcile observation %s: gateway side must be a gateway request observation", pair.Gateway.ID)
	}
	if pair.Transcript.Channel != model.ObservationChannelSessionTranscript || pair.Transcript.Scope != model.ObservationScopeTurn {
		return fmt.Errorf("reconcile observation %s: transcript side must be a session_transcript turn observation", pair.Transcript.ID)
	}
	return nil
}

func matchResultLabel(observationMatch match.ObservationMatch) string {
	if observationMatch.ID != "" {
		return observationMatch.ID
	}
	if observationMatch.GatewayObservationID != "" {
		return observationMatch.GatewayObservationID
	}
	if observationMatch.TranscriptObservationID != "" {
		return observationMatch.TranscriptObservationID
	}
	return "unknown"
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

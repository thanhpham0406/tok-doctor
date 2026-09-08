package model

import (
	"encoding/json"
	"fmt"
	"time"
)

type Agent string

const (
	AgentCodex   Agent = "codex"
	AgentClaude  Agent = "claude"
	AgentRouter9 Agent = "router9"
)

type MeasurementKind string

const (
	MeasurementMeasured  MeasurementKind = "measured"
	MeasurementDerived   MeasurementKind = "derived"
	MeasurementCounted   MeasurementKind = "counted"
	MeasurementEstimated MeasurementKind = "estimated"
	MeasurementUnknown   MeasurementKind = "unknown"
)

type Session struct {
	ID          string       `json:"id"`
	Source      string       `json:"source,omitempty"`
	Agent       Agent        `json:"agent,omitempty"`
	StartedAt   *time.Time   `json:"startedAt,omitempty"`
	UpdatedAt   *time.Time   `json:"updatedAt,omitempty"`
	Model       string       `json:"model,omitempty"`
	Usage       Usage        `json:"usage"`
	Invocations []Invocation `json:"invocations,omitempty"`
	Turns       []Turn       `json:"turns,omitempty"`
	Evidence    []Evidence   `json:"evidence,omitempty"`
}

func (s Session) HasAuthoritativeUsage() bool {
	return s.Usage.HasAuthoritativeUsage()
}

type Turn struct {
	ID        string     `json:"id"`
	Sequence  int        `json:"sequence"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
	Model     string     `json:"model,omitempty"`
	Usage     Usage      `json:"usage"`
	Evidence  []Evidence `json:"evidence,omitempty"`
}

type Invocation struct {
	ID           string        `json:"id,omitempty"`
	StartedAt    *time.Time    `json:"startedAt,omitempty"`
	UpdatedAt    *time.Time    `json:"updatedAt,omitempty"`
	Model        string        `json:"model,omitempty"`
	Measurements []Measurement `json:"measurements,omitempty"`
	Evidence     []Evidence    `json:"evidence,omitempty"`
}

type Measurement struct {
	Value    *int64          `json:"value,omitempty"`
	Kind     MeasurementKind `json:"kind,omitempty"`
	Evidence []Evidence      `json:"evidence,omitempty"`
}

func NewMeasurement(value int64, kind MeasurementKind) Measurement {
	return Measurement{Value: Int64(value), Kind: kind}
}

func MeasurementWithEvidence(value int64, kind MeasurementKind, evidence ...Evidence) Measurement {
	m := NewMeasurement(value, kind)
	m.Evidence = append(m.Evidence, evidence...)
	return m
}

func Int64(value int64) *int64 {
	return &value
}

func ValidMeasurementKind(kind MeasurementKind) bool {
	switch kind {
	case MeasurementMeasured, MeasurementDerived, MeasurementCounted, MeasurementEstimated, MeasurementUnknown:
		return true
	default:
		return false
	}
}

func (m Measurement) Available() bool {
	return m.Value != nil && m.Kind != "" && m.Kind != MeasurementUnknown
}

func (m Measurement) ValueOrZero() int64 {
	if m.Value == nil {
		return 0
	}
	return *m.Value
}

func (m Measurement) DisplayKind() MeasurementKind {
	if !m.Available() {
		return MeasurementUnknown
	}
	return m.Kind
}

func (m Measurement) HasAuthoritativeUsage() bool {
	if !m.Available() {
		return false
	}
	return m.Kind == MeasurementMeasured || m.Kind == MeasurementDerived || m.Kind == MeasurementCounted
}

func (m Measurement) HasEvidence() bool {
	return len(m.Evidence) > 0
}

type EvidenceKind string

const (
	EvidenceSourceValue     EvidenceKind = "source_value"
	EvidenceCumulativeDelta EvidenceKind = "cumulative_delta"
	EvidenceAggregate       EvidenceKind = "aggregate"
)

func ValidEvidenceKind(kind EvidenceKind) bool {
	switch kind {
	case EvidenceSourceValue, EvidenceCumulativeDelta, EvidenceAggregate:
		return true
	default:
		return false
	}
}

type AggregateOperation string

const (
	AggregateSum AggregateOperation = "sum"
)

// Evidence describes the origin of a Measurement without restating the value.
// References must remain stable enough for audit within the source data.
type Evidence struct {
	Kind      EvidenceKind       `json:"kind"`
	Source    string             `json:"source,omitempty"`
	Record    string             `json:"record,omitempty"`
	Field     string             `json:"field,omitempty"`
	Previous  string             `json:"previous,omitempty"`
	Current   string             `json:"current,omitempty"`
	Operation AggregateOperation `json:"operation,omitempty"`
	Count     int                `json:"count,omitempty"`
}

// ConsistentWithKind reports whether the evidence kind matches the measurement
// kind in an obviously expected way. Unknown measurement kinds always pass.
func (e Evidence) ConsistentWithKind(kind MeasurementKind) bool {
	if !ValidEvidenceKind(e.Kind) {
		return false
	}
	if kind == "" || kind == MeasurementUnknown {
		return false
	}
	switch e.Kind {
	case EvidenceSourceValue:
		return kind == MeasurementMeasured
	case EvidenceCumulativeDelta, EvidenceAggregate:
		return kind == MeasurementDerived
	}
	return false
}

type Usage struct {
	Input     Measurement `json:"input"`
	Cached    Measurement `json:"cached"`
	Output    Measurement `json:"output"`
	Reasoning Measurement `json:"reasoning"`
	Total     Measurement `json:"total"`
}

func (u Usage) HasAuthoritativeUsage() bool {
	return u.Input.HasAuthoritativeUsage() ||
		u.Cached.HasAuthoritativeUsage() ||
		u.Output.HasAuthoritativeUsage() ||
		u.Reasoning.HasAuthoritativeUsage() ||
		u.Total.HasAuthoritativeUsage()
}

func (u Usage) HasUsage() bool {
	return u.Input.Available() ||
		u.Cached.Available() ||
		u.Output.Available() ||
		u.Reasoning.Available() ||
		u.Total.Available()
}

func (u Usage) MarshalJSON() ([]byte, error) {
	type usageJSON struct {
		Input        *int64            `json:"input"`
		Cached       *int64            `json:"cached"`
		Output       *int64            `json:"output"`
		Reasoning    *int64            `json:"reasoning"`
		Total        *int64            `json:"total"`
		Measurements *measurementsJSON `json:"measurements,omitempty"`
	}
	encoded := usageJSON{
		Input:     u.Input.Value,
		Cached:    u.Cached.Value,
		Output:    u.Output.Value,
		Reasoning: u.Reasoning.Value,
		Total:     u.Total.Value,
	}
	if m := encodeMeasurements(u); m != nil {
		encoded.Measurements = m
	}
	return json.Marshal(encoded)
}

func encodeMeasurements(u Usage) *measurementsJSON {
	hasKindOrEvidence := func(m Measurement) bool {
		return m.Kind != "" || len(m.Evidence) > 0
	}
	if !hasKindOrEvidence(u.Input) && !hasKindOrEvidence(u.Cached) && !hasKindOrEvidence(u.Output) && !hasKindOrEvidence(u.Reasoning) && !hasKindOrEvidence(u.Total) {
		return nil
	}
	return &measurementsJSON{
		Input:     encodeMeasurement(u.Input),
		Cached:    encodeMeasurement(u.Cached),
		Output:    encodeMeasurement(u.Output),
		Reasoning: encodeMeasurement(u.Reasoning),
		Total:     encodeMeasurement(u.Total),
	}
}

type measurementsJSON struct {
	Input     *measurementDetail `json:"input,omitempty"`
	Cached    *measurementDetail `json:"cached,omitempty"`
	Output    *measurementDetail `json:"output,omitempty"`
	Reasoning *measurementDetail `json:"reasoning,omitempty"`
	Total     *measurementDetail `json:"total,omitempty"`
}

type measurementDetail struct {
	Kind     MeasurementKind `json:"kind,omitempty"`
	Evidence []Evidence      `json:"evidence,omitempty"`
}

func encodeMeasurement(m Measurement) *measurementDetail {
	if m.Kind == "" && len(m.Evidence) == 0 {
		return nil
	}
	return &measurementDetail{Kind: m.Kind, Evidence: m.Evidence}
}

func (u *Usage) UnmarshalJSON(data []byte) error {
	type usageJSON struct {
		Input     *int64 `json:"input"`
		Cached    *int64 `json:"cached"`
		Output    *int64 `json:"output"`
		Reasoning *int64 `json:"reasoning"`
		Total     *int64 `json:"total"`
	}
	var raw usageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = Usage{
		Input:     measurementFromJSON(raw.Input),
		Cached:    measurementFromJSON(raw.Cached),
		Output:    measurementFromJSON(raw.Output),
		Reasoning: measurementFromJSON(raw.Reasoning),
		Total:     measurementFromJSON(raw.Total),
	}
	return nil
}

func measurementFromJSON(value *int64) Measurement {
	if value == nil {
		return Measurement{}
	}
	return Measurement{Value: value, Kind: MeasurementUnknown}
}

func MeasuredUsage(input, cached, output int64, reasoning *int64, total int64) Usage {
	return usageWithKind(input, cached, output, reasoning, total, MeasurementMeasured)
}

func DerivedUsage(input, cached, output int64, reasoning *int64, total int64) Usage {
	return usageWithKind(input, cached, output, reasoning, total, MeasurementDerived)
}

func usageWithKind(input, cached, output int64, reasoning *int64, total int64, kind MeasurementKind) Usage {
	usage := Usage{
		Input:  NewMeasurement(input, kind),
		Cached: NewMeasurement(cached, kind),
		Output: NewMeasurement(output, kind),
		Total:  NewMeasurement(total, kind),
	}
	if reasoning != nil {
		usage.Reasoning = NewMeasurement(*reasoning, kind)
	}
	return usage
}

func SumUsage(usages []Usage) Usage {
	var sum Usage
	sum.Input = sumMeasurements(metricValues(usages, func(u Usage) Measurement { return u.Input }))
	sum.Cached = sumMeasurements(metricValues(usages, func(u Usage) Measurement { return u.Cached }))
	sum.Output = sumMeasurements(metricValues(usages, func(u Usage) Measurement { return u.Output }))
	sum.Reasoning = sumMeasurements(metricValues(usages, func(u Usage) Measurement { return u.Reasoning }))
	sum.Total = sumMeasurements(metricValues(usages, func(u Usage) Measurement { return u.Total }))
	return sum
}

func metricValues(usages []Usage, pick func(Usage) Measurement) []Measurement {
	values := make([]Measurement, 0, len(usages))
	for _, usage := range usages {
		values = append(values, pick(usage))
	}
	return values
}

func sumMeasurements(measurements []Measurement) Measurement {
	var total int64
	seen := false
	for _, measurement := range measurements {
		if !measurement.Available() {
			continue
		}
		seen = true
		total += measurement.ValueOrZero()
	}
	if !seen {
		return Measurement{}
	}
	return NewMeasurement(total, MeasurementDerived)
}

func RequireValidMeasurementKind(kind MeasurementKind) error {
	if !ValidMeasurementKind(kind) {
		return fmt.Errorf("invalid measurement kind %q", kind)
	}
	return nil
}

type UsageResult struct {
	Sources []UsageEntry `json:"sources"`
}

type UsageEntry struct {
	Source string `json:"source"`
	Usage  Usage  `json:"usage"`
}

type SessionsResult struct {
	Sessions []Session `json:"sessions"`
}

type Finding struct {
	RuleID string `json:"rule_id"`
	Title  string `json:"title"`
}

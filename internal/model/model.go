package model

import "time"

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

type Confidence = MeasurementKind

const (
	ConfidenceMeasured                 = MeasurementMeasured
	ConfidenceHigh     MeasurementKind = "high"
	ConfidenceMedium   MeasurementKind = "medium"
	ConfidenceLow      MeasurementKind = "low"
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
	Evidence    []Evidence   `json:"evidence,omitempty"`
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
	Name     string          `json:"name"`
	Value    *int64          `json:"value,omitempty"`
	Kind     MeasurementKind `json:"kind,omitempty"`
	Evidence []Evidence      `json:"evidence,omitempty"`
}

type Evidence struct {
	Kind   string `json:"kind"`
	Source string `json:"source,omitempty"`
}

type Usage struct {
	Input       int64           `json:"input"`
	Cached      int64           `json:"cached"`
	Output      int64           `json:"output"`
	Reasoning   *int64          `json:"reasoning,omitempty"`
	Total       int64           `json:"total"`
	Measurement MeasurementKind `json:"measurement,omitempty"`
	Confidence  Confidence      `json:"confidence,omitempty"`
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

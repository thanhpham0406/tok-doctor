package model

import "time"

type Agent string

const (
	AgentCodex Agent = "codex"
)

type Confidence string

const (
	ConfidenceMeasured Confidence = "measured"
	ConfidenceHigh     Confidence = "high"
	ConfidenceMedium   Confidence = "medium"
	ConfidenceLow      Confidence = "low"
)

type Session struct {
	ID        string    `json:"id"`
	Agent     Agent     `json:"agent"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Usage     Usage     `json:"usage"`
}

type Usage struct {
	Input      int64      `json:"input"`
	Cached     int64      `json:"cached"`
	Output     int64      `json:"output"`
	Reasoning  int64      `json:"reasoning"`
	Total      int64      `json:"total"`
	Confidence Confidence `json:"confidence,omitempty"`
}

type UsageResult struct {
	Sources []UsageEntry `json:"sources"`
}

type UsageEntry struct {
	Source string `json:"source"`
	Usage  Usage  `json:"usage"`
}

type Finding struct {
	RuleID string `json:"rule_id"`
	Title  string `json:"title"`
}

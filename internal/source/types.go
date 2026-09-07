package source

import (
	"context"
	"time"
)

type SessionRef struct {
	ID        string
	StartedAt time.Time
	Path      string
}

type Status string

const (
	StatusReady       Status = "ready"
	StatusInstalled   Status = "installed"
	StatusUnavailable Status = "unavailable"
	StatusBroken      Status = "broken"
)

type Origin string

const (
	OriginCLI    Origin = "cli"
	OriginConfig Origin = "config"
	OriginAuto   Origin = "auto"
	OriginNone   Origin = "none"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type Kind string

const (
	KindPath     Kind = "path"
	KindEndpoint Kind = "endpoint"
)

type Evidence struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	OK    bool   `json:"ok"`
}

type PathCandidate struct {
	Kind string
	Path string
}

type Capabilities struct {
	SessionDiscovery bool `json:"session_discovery,omitempty"`
	TokenUsage       bool `json:"token_usage,omitempty"`
	ToolCalls        bool `json:"tool_calls,omitempty"`
	ToolOutput       bool `json:"tool_output,omitempty"`
}

type Detection struct {
	Name         string        `json:"name"`
	DisplayName  string        `json:"display_name"`
	Status       Status        `json:"status"`
	Origin       Origin        `json:"origin"`
	Confidence   Confidence    `json:"confidence,omitempty"`
	Kind         Kind          `json:"kind,omitempty"`
	Location     string        `json:"location,omitempty"`
	Endpoint     string        `json:"endpoint,omitempty"`
	Reason       string        `json:"reason,omitempty"`
	Evidence     []Evidence    `json:"evidence,omitempty"`
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}

type Override struct {
	Path     string `json:"path,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Origin   Origin `json:"origin,omitempty"`
}

type Detector interface {
	Name() string
	DisplayName() string
	Kind() Kind
	Detect(context.Context, Override) Detection
	Test(context.Context, Override) Detection
}

type ListResult struct {
	Sources []Detection `json:"sources"`
}

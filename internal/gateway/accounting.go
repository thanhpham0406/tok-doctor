package gateway

import (
	"sort"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

type CategoryBreakdown struct {
	Instructions   model.Measurement `json:"instructions,omitempty"`
	UserPrompt     model.Measurement `json:"userPrompt,omitempty"`
	History        model.Measurement `json:"history,omitempty"`
	File           model.Measurement `json:"file,omitempty"`
	ToolResult     model.Measurement `json:"toolResult,omitempty"`
	ToolDefinition model.Measurement `json:"toolDefinition,omitempty"`
	Other          model.Measurement `json:"other,omitempty"`
	Unknown        model.Measurement `json:"unknown,omitempty"`
}

type AttributedTotal struct {
	Value model.Measurement `json:"value"`
}

type FileAttribution struct {
	Path        string            `json:"path"`
	Measurement model.Measurement `json:"measurement"`
}

type RequestSummary struct {
	ExchangeID   string                  `json:"exchangeId"`
	Profile      string                  `json:"profile,omitempty"`
	Model        string                  `json:"model,omitempty"`
	StartedAt    string                  `json:"startedAt,omitempty"`
	Endpoint     string                  `json:"endpoint,omitempty"`
	Protocol     Protocol                `json:"protocol,omitempty"`
	PayloadBytes int64                   `json:"payloadBytes"`
	Categories   CategoryBreakdown       `json:"categories"`
	Attributed   AttributedTotal         `json:"attributed"`
	Files        []FileAttribution       `json:"files,omitempty"`
	Provider     *RequestProviderSummary `json:"provider,omitempty"`
}

type RequestProviderSummary struct {
	Input           *int64                    `json:"input,omitempty"`
	CachedInput     *int64                    `json:"cachedInput,omitempty"`
	FreshInput      *int64                    `json:"freshInput,omitempty"`
	Output          *int64                    `json:"output,omitempty"`
	ReasoningOutput *int64                    `json:"reasoningOutput,omitempty"`
	Kind            string                    `json:"kind,omitempty"`
	Gap             *model.Measurement        `json:"gap,omitempty"`
	Coverage        *AttributionCoverageValue `json:"coverage,omitempty"`
}

func SummarizeRequest(e Exchange) RequestSummary {
	summary := RequestSummary{
		ExchangeID:   e.ID,
		Profile:      e.Profile,
		Model:        e.Model,
		Endpoint:     e.Request.Endpoint,
		Protocol:     e.Protocol,
		PayloadBytes: e.Request.Bytes,
	}
	if !e.StartedAt.IsZero() {
		summary.StartedAt = e.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	cat := &summary.Categories
	for _, c := range e.Request.Components {
		measurement := summaryMeasurement(c)
		switch c.Kind {
		case model.ContextInstructions:
			cat.Instructions = addMeasurement(cat.Instructions, measurement)
		case model.ContextUserPrompt:
			cat.UserPrompt = addMeasurement(cat.UserPrompt, measurement)
		case model.ContextHistory:
			cat.History = addMeasurement(cat.History, measurement)
		case model.ContextFile:
			cat.File = addMeasurement(cat.File, measurement)
		case model.ContextToolResult:
			cat.ToolResult = addMeasurement(cat.ToolResult, measurement)
		case model.ContextToolDefinition:
			cat.ToolDefinition = addMeasurement(cat.ToolDefinition, measurement)
		case model.ContextOther:
			cat.Other = addMeasurement(cat.Other, measurement)
		case model.ContextUnknown:
			cat.Unknown = addMeasurement(cat.Unknown, measurement)
		}
	}

	summary.Attributed.Value = sumCategoryMeasurements(summary.Categories)
	summary.Files = aggregateFileAttribution(e.Request.Components)
	if summary.Provider = buildRequestProviderSummary(e, summary.Attributed.Value); summary.Provider == nil {
		summary.Provider = nil
	}
	return summary
}

func buildRequestProviderSummary(e Exchange, attributed model.Measurement) *RequestProviderSummary {
	pu := e.Response.ProviderUsage
	if pu == nil {
		return nil
	}
	out := &RequestProviderSummary{Kind: "measured"}
	if pu.Input != nil {
		v := *pu.Input
		out.Input = &v
	}
	if pu.CachedInput != nil {
		v := *pu.CachedInput
		out.CachedInput = &v
	}
	if pu.Output != nil {
		v := *pu.Output
		out.Output = &v
	}
	if pu.ReasoningOutput != nil {
		v := *pu.ReasoningOutput
		out.ReasoningOutput = &v
	}
	if out.Input != nil && out.CachedInput != nil {
		fresh := *out.Input - *out.CachedInput
		out.FreshInput = &fresh
	}
	if out.Input == nil && out.CachedInput == nil && out.Output == nil && out.ReasoningOutput == nil {
		return nil
	}
	if out.Input != nil && attributed.Available() {
		provider := model.NewMeasurement(*out.Input, model.MeasurementMeasured)
		gap := subtractMetric(provider, attributed)
		out.Gap = &gap
		out.Coverage = computeCoverage(attributed, provider, attributed.Kind)
	}
	return out
}

func summaryMeasurement(c model.ContextComponent) model.Measurement {
	if c.Kind == model.ContextToolDefinition && c.Measurement.Kind == model.MeasurementCounted {
		return model.Measurement{Kind: model.MeasurementUnknown}
	}
	return c.Measurement
}

func addMeasurement(acc, next model.Measurement) model.Measurement {
	if !next.Available() {
		return acc
	}
	if !acc.Available() {
		return next
	}
	return model.Measurement{
		Value: model.Int64(acc.ValueOrZero() + next.ValueOrZero()),
		Kind:  weakenKind(acc.Kind, next.Kind),
	}
}

func sumCategoryMeasurements(c CategoryBreakdown) model.Measurement {
	var result model.Measurement
	add := func(m model.Measurement) {
		if !m.Available() {
			return
		}
		if !result.Available() {
			result = m
			return
		}
		result = model.Measurement{
			Value: model.Int64(result.ValueOrZero() + m.ValueOrZero()),
			Kind:  weakenKind(result.Kind, m.Kind),
		}
	}
	add(c.Instructions)
	add(c.UserPrompt)
	add(c.History)
	add(c.File)
	add(c.ToolResult)
	add(c.ToolDefinition)
	add(c.Other)
	add(c.Unknown)
	return result
}

func weakenKind(a, b model.MeasurementKind) model.MeasurementKind {
	if a == model.MeasurementEstimated || b == model.MeasurementEstimated {
		return model.MeasurementEstimated
	}
	if a == model.MeasurementUnknown || b == model.MeasurementUnknown {
		return model.MeasurementUnknown
	}
	if a == model.MeasurementDerived || b == model.MeasurementDerived {
		return model.MeasurementDerived
	}
	if a == model.MeasurementCounted || b == model.MeasurementCounted {
		return model.MeasurementCounted
	}
	return model.MeasurementMeasured
}

func aggregateFileAttribution(components []model.ContextComponent) []FileAttribution {
	bucket := map[string]*FileAttribution{}
	for _, c := range components {
		if c.Kind != model.ContextFile {
			continue
		}
		if c.Path == "" {
			continue
		}
		path := source.NormalizePath(c.Path)
		if path == "" {
			continue
		}
		if !c.Measurement.Available() {
			continue
		}
		entry, ok := bucket[path]
		if !ok {
			bucket[path] = &FileAttribution{
				Path:        path,
				Measurement: c.Measurement,
			}
			continue
		}
		entry.Measurement = addMeasurement(entry.Measurement, c.Measurement)
	}
	out := make([]FileAttribution, 0, len(bucket))
	for _, entry := range bucket {
		out = append(out, *entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, ri := out[i].Measurement.ValueOrZero(), out[j].Measurement.ValueOrZero()
		if li != ri {
			return li > ri
		}
		return out[i].Path < out[j].Path
	})
	return out
}

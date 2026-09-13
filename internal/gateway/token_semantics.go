package gateway

import "github.com/thanhpham0406/tok-doctor/internal/model"

func ProviderUsageToObserved(p *ProviderUsage) ObservedUsage {
	if p == nil {
		return ObservedUsage{Source: model.MeasurementDerived}
	}
	switch p.Source {
	case string(ProtocolAnthropicMessages):
		out := ObservedUsage{Source: model.MeasurementMeasured}
		if p.CacheReadInputTokens != nil {
			out.Cached = *p.CacheReadInputTokens
		}
		if p.CacheCreationInputTokens != nil {
			out.CacheCreation = *p.CacheCreationInputTokens
		}
		if p.InputTokens != nil {
			out.RawInput = *p.InputTokens
			out.TotalInput = out.RawInput + out.Cached + out.CacheCreation
		} else if p.TotalInputTokens != nil {
			out.TotalInput = *p.TotalInputTokens
		}
		if p.OutputTokens != nil {
			out.Output = *p.OutputTokens
		}
		if p.ReasoningOutputTokens != nil {
			out.Reasoning = *p.ReasoningOutputTokens
		}
		out.Total = out.TotalInput + out.Output
		return out
	case string(ProtocolOpenAIResponses):
		out := ObservedUsage{Source: model.MeasurementMeasured}
		if p.InputTokens != nil {
			out.TotalInput = *p.InputTokens
		}
		if p.TotalInputTokens != nil {
			out.TotalInput = *p.TotalInputTokens
		}
		if p.CacheReadInputTokens != nil {
			out.Cached = *p.CacheReadInputTokens
		}
		fresh := out.TotalInput - out.Cached
		if fresh < 0 {
			fresh = 0
		}
		out.RawInput = fresh
		if p.OutputTokens != nil {
			out.Output = *p.OutputTokens
		}
		if p.ReasoningOutputTokens != nil {
			out.Reasoning = *p.ReasoningOutputTokens
		}
		if p.TotalTokens != nil {
			out.Total = *p.TotalTokens
		} else {
			out.Total = out.TotalInput + out.Output
		}
		return out
	default:
		out := ObservedUsage{Source: model.MeasurementDerived}
		if p.OutputTokens != nil {
			out.Output = *p.OutputTokens
		}
		if p.ReasoningOutputTokens != nil {
			out.Reasoning = *p.ReasoningOutputTokens
		}
		if p.TotalInputTokens != nil {
			out.TotalInput = *p.TotalInputTokens
		}
		if p.TotalTokens != nil {
			out.Total = *p.TotalTokens
		}
		return out
	}
}

func ProviderUsageAdd(dst, src *ProviderUsage) *ProviderUsage {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = &ProviderUsage{Source: src.Source}
	}
	dst.InputTokens = addInt64Ptr(dst.InputTokens, src.InputTokens)
	dst.CacheCreationInputTokens = addInt64Ptr(dst.CacheCreationInputTokens, src.CacheCreationInputTokens)
	dst.CacheReadInputTokens = addInt64Ptr(dst.CacheReadInputTokens, src.CacheReadInputTokens)
	dst.OutputTokens = addInt64Ptr(dst.OutputTokens, src.OutputTokens)
	dst.ReasoningOutputTokens = addInt64Ptr(dst.ReasoningOutputTokens, src.ReasoningOutputTokens)
	dst.TotalInputTokens = addInt64Ptr(dst.TotalInputTokens, src.TotalInputTokens)
	dst.TotalTokens = addInt64Ptr(dst.TotalTokens, src.TotalTokens)
	return dst
}

func addInt64Ptr(a, b *int64) *int64 {
	switch {
	case a == nil && b == nil:
		return nil
	case a == nil:
		v := *b
		return &v
	case b == nil:
		return a
	default:
		v := *a + *b
		return &v
	}
}

func (p *ProviderUsage) HasUsage() bool {
	return p != nil && p.ReportedFields() > 0
}

// providerFieldKinds describes the provenance of each canonical provider
// field: direct provider reports keep measured provenance, derived values
// (sums or gaps) keep derived provenance, and unrecognized schemas stay
// derived so unknown fields are never promoted to measured.
type providerFieldKinds struct {
	Fresh         model.MeasurementKind
	Cached        model.MeasurementKind
	CacheCreation model.MeasurementKind
	TotalInput    model.MeasurementKind
	Output        model.MeasurementKind
	Reasoning     model.MeasurementKind
	Total         model.MeasurementKind
}

func providerFieldKindsFor(pu *ProviderUsage) providerFieldKinds {
	measured := model.MeasurementMeasured
	derived := model.MeasurementDerived
	if pu == nil {
		return providerFieldKinds{
			Fresh: derived, Cached: derived, CacheCreation: derived,
			TotalInput: derived, Output: derived, Reasoning: derived, Total: derived,
		}
	}
	switch pu.Source {
	case string(ProtocolAnthropicMessages):
		out := providerFieldKinds{
			Fresh:         measured,
			Cached:        measured,
			CacheCreation: measured,
			Output:        measured,
			Reasoning:     measured,
			TotalInput:    derived,
			Total:         derived,
		}
		if pu.TotalInputTokens != nil {
			out.TotalInput = measured
		}
		if pu.TotalTokens != nil {
			out.Total = measured
		}
		return out
	case string(ProtocolOpenAIResponses):
		out := providerFieldKinds{
			Cached:        measured,
			CacheCreation: measured,
			TotalInput:    measured,
			Output:        measured,
			Reasoning:     measured,
			Fresh:         derived,
			Total:         derived,
		}
		if pu.TotalTokens != nil {
			out.Total = measured
		}
		return out
	default:
		return providerFieldKinds{
			Fresh:         derived,
			Cached:        derived,
			CacheCreation: derived,
			TotalInput:    derived,
			Output:        derived,
			Reasoning:     derived,
			Total:         derived,
		}
	}
}

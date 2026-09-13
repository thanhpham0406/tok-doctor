package gateway

import "github.com/thanhpham0406/tok-doctor/internal/model"

func ProviderUsageToObserved(p *ProviderUsage) ObservedUsage {
	out := ObservedUsage{Source: model.MeasurementMeasured}
	if p == nil {
		return out
	}
	inputIncludesCache := inputSourceIncludesCache(p.Source)
	if p.CacheReadInputTokens != nil {
		v := *p.CacheReadInputTokens
		out.Cached = v
		if p.InputTokens != nil {
			raw := *p.InputTokens
			out.RawInput = raw
			if inputIncludesCache {
				out.TotalInput = raw
				if p.CacheCreationInputTokens == nil {
					out.RawInput = raw - v
				}
			} else {
				out.TotalInput = out.RawInput + out.Cached
			}
		} else if p.TotalInputTokens != nil {
			out.TotalInput = *p.TotalInputTokens
			out.RawInput = out.TotalInput - out.Cached
		}
	} else if p.TotalInputTokens != nil {
		out.TotalInput = *p.TotalInputTokens
		out.RawInput = out.TotalInput - out.Cached
	} else if p.InputTokens != nil {
		out.RawInput = *p.InputTokens
		out.TotalInput = out.RawInput
	}
	if p.CacheCreationInputTokens != nil {
		v := *p.CacheCreationInputTokens
		out.CacheCreation = v
		if !inputIncludesCache {
			out.TotalInput = out.RawInput + out.Cached + out.CacheCreation
		} else {
			out.TotalInput = out.RawInput + out.CacheCreation
		}
	}
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
	} else {
		out.Total = out.TotalInput + out.Output
	}
	return out
}

func inputSourceIncludesCache(source string) bool {
	switch source {
	case "openai_responses":
		return true
	}
	return false
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

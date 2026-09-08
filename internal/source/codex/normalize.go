package codex

import "github.com/thanhpham0406/tok-doctor/internal/model"

type UsageSnapshot struct {
	Input     int64
	Cached    int64
	Output    int64
	Reasoning *int64
	Total     int64
	HasUsage  bool
}

func (s UsageSnapshot) ToModelUsage() model.Usage {
	return s.toModelUsage(model.MeasurementMeasured)
}

func (s UsageSnapshot) ToModelUsageWithEvidence(recordID string) model.Usage {
	usage := s.ToModelUsage()
	if recordID == "" || !s.hasAuthoritativeUsage() {
		return usage
	}
	attachSourceValue(&usage, recordID)
	return usage
}

func (s UsageSnapshot) toModelUsage(kind model.MeasurementKind) model.Usage {
	if !s.hasAuthoritativeUsage() {
		return model.Usage{}
	}
	if kind == model.MeasurementDerived {
		usage := model.DerivedUsage(s.Input, s.Cached, s.Output, s.Reasoning, s.Total)
		usage.Billable = model.NewBillableUsage(s.Input, s.Cached, s.Output, nil, model.MeasurementDerived)
		return usage
	}
	usage := model.MeasuredUsage(s.Input, s.Cached, s.Output, s.Reasoning, s.Total)
	usage.Billable = model.NewBillableUsage(s.Input, s.Cached, s.Output, nil, model.MeasurementMeasured)
	return usage
}

func attachSourceValue(u *model.Usage, recordID string) {
	attachField := func(m *model.Measurement, field string) {
		if !m.Available() {
			return
		}
		m.Evidence = append(m.Evidence, sourceValueEvidence(recordID, field))
	}
	attachField(&u.Input, "total_token_usage.input_tokens")
	attachField(&u.Cached, "total_token_usage.cached_input_tokens")
	attachField(&u.Output, "total_token_usage.output_tokens")
	attachField(&u.Reasoning, "total_token_usage.reasoning_output_tokens")
	attachField(&u.Total, "total_token_usage.total_tokens")
}

func attachCumulativeDelta(u *model.Usage, prev, curr string) {
	attachField := func(m *model.Measurement, field string) {
		if !m.Available() {
			return
		}
		m.Evidence = append(m.Evidence, cumulativeDeltaEvidence(prev, curr, field))
	}
	attachField(&u.Input, "input_tokens")
	attachField(&u.Cached, "cached_input_tokens")
	attachField(&u.Output, "output_tokens")
	attachField(&u.Reasoning, "reasoning_output_tokens")
	attachField(&u.Total, "total_tokens")
}

func sourceValueEvidence(recordID, field string) model.Evidence {
	return model.Evidence{
		Kind:   model.EvidenceSourceValue,
		Source: "codex_rollout",
		Record: recordID,
		Field:  field,
	}
}

func cumulativeDeltaEvidence(prev, curr, field string) model.Evidence {
	return model.Evidence{
		Kind:     model.EvidenceCumulativeDelta,
		Source:   "codex_rollout",
		Field:    field,
		Previous: prev,
		Current:  curr,
	}
}

func SumSnapshots(snaps []UsageSnapshot) model.Usage {
	var (
		totalInput     int64
		totalCached    int64
		totalOutput    int64
		totalTotal     int64
		totalReasoning *int64
		hasUsage       bool
	)
	for _, s := range snaps {
		if !s.hasAuthoritativeUsage() {
			continue
		}
		hasUsage = true
		totalInput += s.Input
		totalCached += s.Cached
		totalOutput += s.Output
		totalTotal += s.Total
		if s.Reasoning != nil {
			if totalReasoning == nil {
				v := *s.Reasoning
				totalReasoning = &v
			} else {
				*totalReasoning += *s.Reasoning
			}
		}
	}
	if !hasUsage {
		return model.Usage{}
	}
	return model.DerivedUsage(totalInput, totalCached, totalOutput, totalReasoning, totalTotal)
}

func (s UsageSnapshot) hasAuthoritativeUsage() bool {
	return s.HasUsage || s.Input != 0 || s.Cached != 0 || s.Output != 0 || s.Total != 0 || s.Reasoning != nil
}

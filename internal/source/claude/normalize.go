package claude

import "github.com/thanhpham0406/tok-doctor/internal/model"

type UsageSnapshot struct {
	Input      int64
	CacheRead  int64
	CacheWrite int64
	Cached     int64
	Output     int64
	Total      int64
	HasUsage   bool
}

func (s UsageSnapshot) add(other UsageSnapshot) UsageSnapshot {
	if !other.HasUsage {
		return s
	}
	s.HasUsage = true
	s.Input += other.Input
	s.Output += other.Output
	s.CacheRead += other.CacheRead
	s.CacheWrite += other.CacheWrite
	s.Cached += other.Cached
	s.Total += other.Total
	return s
}

func (s UsageSnapshot) HasPositiveUsage() bool {
	if !s.HasUsage {
		return false
	}
	return s.Input > 0 || s.Output > 0 || s.Cached > 0
}

func (s UsageSnapshot) ToModelUsage() model.Usage {
	return s.toModelUsage(model.MeasurementMeasured)
}

func (s UsageSnapshot) ToModelUsageWithEvidence(recordID string) model.Usage {
	usage := s.toModelUsage(model.MeasurementMeasured)
	if recordID == "" {
		return usage
	}
	evidence := sourceValueEvidence(recordID)
	attachEvidence(&usage, evidence)
	return usage
}

func (s UsageSnapshot) toModelUsage(kind model.MeasurementKind) model.Usage {
	if !s.HasUsage {
		return model.Usage{}
	}
	switch kind {
	case model.MeasurementDerived:
		usage := model.DerivedUsage(s.Input, s.Cached, s.Output, nil, s.Total)
		cw := s.CacheWrite
		usage.Billable = model.NewBillableUsage(s.Input+s.Cached, s.CacheRead, s.Output, &cw, model.MeasurementDerived)
		return usage
	default:
		usage := model.MeasuredUsage(s.Input, s.Cached, s.Output, nil, s.Total)
		cw := s.CacheWrite
		usage.Billable = model.NewBillableUsage(s.Input+s.Cached, s.CacheRead, s.Output, &cw, model.MeasurementMeasured)
		return usage
	}
}

func attachEvidence(u *model.Usage, evidence model.Evidence) {
	attachField := func(m *model.Measurement) {
		if !m.Available() {
			return
		}
		m.Evidence = append(m.Evidence, evidence)
	}
	attachField(&u.Input)
	attachField(&u.Cached)
	attachField(&u.Output)
	attachField(&u.Total)
}

func sourceValueEvidence(recordID string) model.Evidence {
	return model.Evidence{
		Kind:   model.EvidenceSourceValue,
		Source: "claude_session",
		Record: recordID,
	}
}

func SumSnapshots(snaps []UsageSnapshot) model.Usage {
	var total UsageSnapshot
	for _, s := range snaps {
		if !s.HasUsage {
			continue
		}
		total = total.add(s)
	}
	return total.toModelUsage(model.MeasurementDerived)
}

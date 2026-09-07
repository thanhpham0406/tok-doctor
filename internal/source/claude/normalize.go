package claude

import "github.com/thanhpham0406/tok-doctor/internal/model"

type UsageSnapshot struct {
	Input    int64
	Cached   int64
	Output   int64
	Total    int64
	HasUsage bool
}

func (s UsageSnapshot) add(other UsageSnapshot) UsageSnapshot {
	if !other.HasUsage {
		return s
	}
	s.HasUsage = true
	s.Input += other.Input
	s.Output += other.Output
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
	if !s.HasUsage {
		return model.Usage{}
	}
	return model.Usage{
		Input:       s.Input,
		Cached:      s.Cached,
		Output:      s.Output,
		Reasoning:   nil,
		Total:       s.Total,
		Measurement: model.MeasurementMeasured,
		Confidence:  model.ConfidenceMeasured,
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
	return total.ToModelUsage()
}

package codex

import "github.com/thanhpham0406/tok-doctor/internal/model"

type UsageSnapshot struct {
	Input     int64
	Cached    int64
	Output    int64
	Reasoning *int64
	Total     int64
}

func (s UsageSnapshot) ToModelUsage() model.Usage {
	if s.Input == 0 && s.Cached == 0 && s.Output == 0 && s.Total == 0 && s.Reasoning == nil {
		return model.Usage{}
	}
	return model.Usage{
		Input:       s.Input,
		Cached:      s.Cached,
		Output:      s.Output,
		Reasoning:   s.Reasoning,
		Total:       s.Total,
		Measurement: model.MeasurementMeasured,
		Confidence:  model.ConfidenceMeasured,
	}
}

func SumSnapshots(snaps []UsageSnapshot) model.Usage {
	var (
		totalInput     int64
		totalCached    int64
		totalOutput    int64
		totalTotal     int64
		totalReasoning *int64
	)
	for _, s := range snaps {
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
	if totalInput == 0 && totalCached == 0 && totalOutput == 0 && totalTotal == 0 && totalReasoning == nil {
		return model.Usage{}
	}
	return model.Usage{
		Input:       totalInput,
		Cached:      totalCached,
		Output:      totalOutput,
		Reasoning:   totalReasoning,
		Total:       totalTotal,
		Measurement: model.MeasurementMeasured,
		Confidence:  model.ConfidenceMeasured,
	}
}

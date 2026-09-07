package claude

import "github.com/thanhpham0406/tok-doctor/internal/model"

type UsageSnapshot struct {
	Input  int64
	Cached int64
	Output int64
	Total  int64
}

func (s UsageSnapshot) ToModelUsage() model.Usage {
	if s.Input == 0 && s.Cached == 0 && s.Output == 0 && s.Total == 0 {
		return model.Usage{}
	}
	return model.Usage{
		Input:      s.Input,
		Cached:     s.Cached,
		Output:     s.Output,
		Reasoning:  nil,
		Total:      s.Total,
		Confidence: model.ConfidenceMeasured,
	}
}

func SumSnapshots(snaps []UsageSnapshot) model.Usage {
	var total UsageSnapshot
	for _, s := range snaps {
		total.Input += s.Input
		total.Cached += s.Cached
		total.Output += s.Output
		total.Total += s.Total
	}
	return total.ToModelUsage()
}

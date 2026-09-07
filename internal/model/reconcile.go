package model

type UsageComparison struct {
	InputDelta     int64
	CachedDelta    int64
	OutputDelta    int64
	ReasoningDelta *int64
	TotalDelta     int64
}

func (c UsageComparison) Matches() bool {
	if c.InputDelta != 0 || c.CachedDelta != 0 || c.OutputDelta != 0 || c.TotalDelta != 0 {
		return false
	}
	if c.ReasoningDelta != nil && *c.ReasoningDelta != 0 {
		return false
	}
	return true
}

func ReconcileUsage(s Session) UsageComparison {
	var sum Usage
	for _, turn := range s.Turns {
		sum.Input += turn.Usage.Input
		sum.Cached += turn.Usage.Cached
		sum.Output += turn.Usage.Output
		sum.Total += turn.Usage.Total
		if turn.Usage.Reasoning != nil {
			if sum.Reasoning == nil {
				reasoning := *turn.Usage.Reasoning
				sum.Reasoning = &reasoning
			} else {
				*sum.Reasoning += *turn.Usage.Reasoning
			}
		}
	}
	comparison := UsageComparison{
		InputDelta:  sum.Input - s.Usage.Input,
		CachedDelta: sum.Cached - s.Usage.Cached,
		OutputDelta: sum.Output - s.Usage.Output,
		TotalDelta:  sum.Total - s.Usage.Total,
	}
	if s.Usage.Reasoning == nil && sum.Reasoning != nil {
		zero := int64(0)
		comparison.ReasoningDelta = &zero
	} else if s.Usage.Reasoning != nil && sum.Reasoning == nil {
		reasoning := -*s.Usage.Reasoning
		comparison.ReasoningDelta = &reasoning
	} else if s.Usage.Reasoning != nil && sum.Reasoning != nil {
		reasoning := *sum.Reasoning - *s.Usage.Reasoning
		comparison.ReasoningDelta = &reasoning
	}
	return comparison
}

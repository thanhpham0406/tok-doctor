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
	sum := SumUsage(turnUsages(s.Turns))
	return UsageComparison{
		InputDelta:     sum.Input.ValueOrZero() - s.Usage.Input.ValueOrZero(),
		CachedDelta:    sum.Cached.ValueOrZero() - s.Usage.Cached.ValueOrZero(),
		OutputDelta:    sum.Output.ValueOrZero() - s.Usage.Output.ValueOrZero(),
		ReasoningDelta: reasoningDelta(s.Usage.Reasoning, sum.Reasoning),
		TotalDelta:     sum.Total.ValueOrZero() - s.Usage.Total.ValueOrZero(),
	}
}

func turnUsages(turns []Turn) []Usage {
	usages := make([]Usage, 0, len(turns))
	for _, turn := range turns {
		usages = append(usages, turn.Usage)
	}
	return usages
}

func reasoningDelta(session, sum Measurement) *int64 {
	if !session.Available() && sum.Available() {
		zero := int64(0)
		return &zero
	}
	if session.Available() && !sum.Available() {
		reasoning := -session.ValueOrZero()
		return &reasoning
	}
	if session.Available() && sum.Available() {
		reasoning := sum.ValueOrZero() - session.ValueOrZero()
		return &reasoning
	}
	return nil
}

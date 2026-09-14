package claude

import "github.com/thanhpham0406/tok-doctor/internal/model"

const (
	claudeObservationSource  = "claude_session"
	claudeFieldInputTokens   = "message.usage.input_tokens"
	claudeFieldCacheRead     = "message.usage.cache_read_input_tokens"
	claudeFieldCacheCreation = "message.usage.cache_creation_input_tokens"
	claudeFieldOutputTokens  = "message.usage.output_tokens"
)

type UsageSnapshot struct {
	Input             int64
	InputPresent      bool
	CacheRead         int64
	CacheReadPresent  bool
	CacheWrite        int64
	CacheWritePresent bool
	Output            int64
	OutputPresent     bool
	HasUsage          bool

	acceptedCount   int
	inputCount      int
	cacheReadCount  int
	cacheWriteCount int
	outputCount     int
}

type usageAccumulator struct {
	accepted           int
	inputReported      int
	cacheReadReported  int
	cacheWriteReported int
	outputReported     int
	inputCount         int
	cacheReadCount     int
	cacheWriteCount    int
	outputCount        int
	input              int64
	cacheRead          int64
	cacheWrite         int64
	output             int64
}

func (a *usageAccumulator) add(snapshot UsageSnapshot) {
	if !snapshot.HasUsage {
		return
	}
	a.accepted++
	if snapshot.InputPresent {
		a.inputReported++
		if snapshot.Input >= 0 {
			a.inputCount++
			a.input += snapshot.Input
		}
	}
	if snapshot.CacheReadPresent {
		a.cacheReadReported++
		if snapshot.CacheRead >= 0 {
			a.cacheReadCount++
			a.cacheRead += snapshot.CacheRead
		}
	}
	if snapshot.CacheWritePresent {
		a.cacheWriteReported++
		if snapshot.CacheWrite >= 0 {
			a.cacheWriteCount++
			a.cacheWrite += snapshot.CacheWrite
		}
	}
	if snapshot.OutputPresent {
		a.outputReported++
		if snapshot.Output >= 0 {
			a.outputCount++
			a.output += snapshot.Output
		}
	}
}

func (a usageAccumulator) snapshot() UsageSnapshot {
	snapshot := UsageSnapshot{
		HasUsage:        a.accepted > 0,
		acceptedCount:   a.accepted,
		inputCount:      a.inputCount,
		cacheReadCount:  a.cacheReadCount,
		cacheWriteCount: a.cacheWriteCount,
		outputCount:     a.outputCount,
	}
	if a.inputReported > 0 {
		snapshot.Input = a.input
		snapshot.InputPresent = true
	}
	if a.cacheReadReported > 0 {
		snapshot.CacheRead = a.cacheRead
		snapshot.CacheReadPresent = true
	}
	if a.cacheWriteReported > 0 {
		snapshot.CacheWrite = a.cacheWrite
		snapshot.CacheWritePresent = true
	}
	if a.outputReported > 0 {
		snapshot.Output = a.output
		snapshot.OutputPresent = true
	}
	return snapshot
}

func (s UsageSnapshot) fieldComplete(present bool, value int64, count int) bool {
	if s.acceptedCount == 0 {
		return present && value >= 0
	}
	return count == s.acceptedCount
}

func (s UsageSnapshot) inputComplete() bool {
	return s.fieldComplete(s.InputPresent, s.Input, s.inputCount)
}

func (s UsageSnapshot) cacheReadComplete() bool {
	return s.fieldComplete(s.CacheReadPresent, s.CacheRead, s.cacheReadCount)
}

func (s UsageSnapshot) cacheWriteComplete() bool {
	return s.fieldComplete(s.CacheWritePresent, s.CacheWrite, s.cacheWriteCount)
}

func (s UsageSnapshot) outputComplete() bool {
	return s.fieldComplete(s.OutputPresent, s.Output, s.outputCount)
}

func (s UsageSnapshot) CachedValue() (int64, bool) {
	if !s.cacheReadComplete() || !s.cacheWriteComplete() {
		return 0, false
	}
	return s.CacheRead + s.CacheWrite, true
}

func (s UsageSnapshot) TotalValue() (int64, bool) {
	cached, ok := s.CachedValue()
	if !ok || !s.inputComplete() || !s.outputComplete() {
		return 0, false
	}
	return s.Input + cached + s.Output, true
}

func (s UsageSnapshot) HasPositiveUsage() bool {
	if !s.HasUsage {
		return false
	}
	return s.Input > 0 || s.Output > 0 || s.CacheRead > 0 || s.CacheWrite > 0
}

func (s UsageSnapshot) HasReportedUsage() bool {
	if !s.HasUsage {
		return false
	}
	if s.acceptedCount > 0 {
		return s.inputCount > 0 || s.cacheReadCount > 0 || s.cacheWriteCount > 0 || s.outputCount > 0
	}
	return (s.InputPresent && s.Input >= 0) ||
		(s.OutputPresent && s.Output >= 0) ||
		(s.CacheReadPresent && s.CacheRead >= 0) ||
		(s.CacheWritePresent && s.CacheWrite >= 0)
}

func (s UsageSnapshot) ToModelUsage() model.Usage {
	return s.toModelUsage(model.MeasurementMeasured)
}

func (s UsageSnapshot) ToModelUsageWithEvidence(recordID string) model.Usage {
	usage := s.toModelUsage(model.MeasurementMeasured)
	if recordID == "" {
		return usage
	}
	attachUsageEvidence(&usage, recordID)
	return usage
}

func (s UsageSnapshot) conflictedUsage() model.Usage {
	unknown := model.Measurement{Kind: model.MeasurementUnknown}
	usage := model.Usage{}
	if s.InputPresent {
		usage.Input = unknown
	}
	if s.OutputPresent {
		usage.Output = unknown
	}
	if s.CacheReadPresent || s.CacheWritePresent || s.OutputPresent {
		usage.Billable = &model.BillableUsage{}
		if s.CacheReadPresent {
			usage.Billable.CacheRead = unknown
		}
		if s.CacheWritePresent {
			usage.Billable.CacheWrite = unknown
		}
		if s.OutputPresent {
			usage.Billable.Output = unknown
		}
	}
	return usage
}

func (s UsageSnapshot) toModelUsage(directKind model.MeasurementKind) model.Usage {
	if !s.HasUsage {
		return model.Usage{}
	}
	if directKind == "" {
		directKind = model.MeasurementMeasured
	}
	return model.Usage{
		Input:    measurementFor(s.inputComplete(), s.InputPresent, s.Input, directKind),
		Cached:   s.cachedMeasurement(),
		Output:   measurementFor(s.outputComplete(), s.OutputPresent, s.Output, directKind),
		Total:    s.totalMeasurement(),
		Billable: s.billableUsage(directKind),
	}
}

func measurementFor(complete, reported bool, value int64, kind model.MeasurementKind) model.Measurement {
	switch {
	case complete:
		return model.NewMeasurement(value, kind)
	case reported:
		return model.Measurement{Kind: model.MeasurementUnknown}
	default:
		return model.Measurement{}
	}
}

func (s UsageSnapshot) cachedMeasurement() model.Measurement {
	value, ok := s.CachedValue()
	if !ok {
		return model.Measurement{}
	}
	return model.NewMeasurement(value, model.MeasurementDerived)
}

func (s UsageSnapshot) totalMeasurement() model.Measurement {
	value, ok := s.TotalValue()
	if !ok {
		return model.Measurement{}
	}
	return model.NewMeasurement(value, model.MeasurementDerived)
}

func (s UsageSnapshot) billableUsage(directKind model.MeasurementKind) *model.BillableUsage {
	billable := &model.BillableUsage{
		Input:      s.billableInputMeasurement(),
		CacheRead:  measurementFor(s.cacheReadComplete(), s.CacheReadPresent, s.CacheRead, directKind),
		CacheWrite: measurementFor(s.cacheWriteComplete(), s.CacheWritePresent, s.CacheWrite, directKind),
		Output:     measurementFor(s.outputComplete(), s.OutputPresent, s.Output, directKind),
	}
	if billable.Input.Kind == "" && billable.CacheRead.Kind == "" && billable.CacheWrite.Kind == "" && billable.Output.Kind == "" {
		return nil
	}
	return billable
}

func (s UsageSnapshot) billableInputMeasurement() model.Measurement {
	if !s.inputComplete() || !s.cacheReadComplete() || !s.cacheWriteComplete() {
		return model.Measurement{}
	}
	return model.NewMeasurement(s.Input+s.CacheRead+s.CacheWrite, model.MeasurementDerived)
}

func attachUsageEvidence(usage *model.Usage, recordID string) {
	attachSourceValue := func(m *model.Measurement, field string) {
		if !m.Available() {
			return
		}
		m.Evidence = append(m.Evidence, claudeSourceValueEvidence(recordID, field))
	}
	attachSourceValue(&usage.Input, claudeFieldInputTokens)
	attachSourceValue(&usage.Output, claudeFieldOutputTokens)
	if usage.Billable != nil {
		attachSourceValue(&usage.Billable.CacheRead, claudeFieldCacheRead)
		attachSourceValue(&usage.Billable.CacheWrite, claudeFieldCacheCreation)
	}

	attachAggregate := func(m *model.Measurement, count int) {
		if !m.Available() {
			return
		}
		m.Evidence = append(m.Evidence, claudeAggregateEvidence(recordID, count))
	}
	attachAggregate(&usage.Cached, 2)
	attachAggregate(&usage.Total, 4)
	if usage.Billable != nil {
		attachAggregate(&usage.Billable.Input, 3)
	}
}

func claudeSourceValueEvidence(recordID, field string) model.Evidence {
	return model.Evidence{
		Kind:   model.EvidenceSourceValue,
		Source: claudeObservationSource,
		Record: recordID,
		Field:  field,
	}
}

func claudeAggregateEvidence(recordID string, count int) model.Evidence {
	return model.Evidence{
		Kind:      model.EvidenceAggregate,
		Source:    claudeObservationSource,
		Record:    recordID,
		Operation: model.AggregateSum,
		Count:     count,
	}
}

func SumSnapshots(snaps []UsageSnapshot) model.Usage {
	var accumulator usageAccumulator
	for _, snap := range snaps {
		accumulator.add(snap)
	}
	usage := accumulator.snapshot().toModelUsage(model.MeasurementDerived)
	attachSumEvidence(&usage, accumulator.accepted)
	return usage
}

func attachSumEvidence(usage *model.Usage, count int) {
	if count == 0 {
		return
	}
	ev := model.Evidence{
		Kind:      model.EvidenceAggregate,
		Source:    claudeObservationSource,
		Operation: model.AggregateSum,
		Count:     count,
	}
	attach := func(m *model.Measurement) {
		if !m.Available() {
			return
		}
		m.Evidence = append(m.Evidence, ev)
	}
	attach(&usage.Input)
	attach(&usage.Cached)
	attach(&usage.Output)
	attach(&usage.Total)
	if usage.Billable != nil {
		attach(&usage.Billable.Input)
		attach(&usage.Billable.CacheRead)
		attach(&usage.Billable.CacheWrite)
		attach(&usage.Billable.Output)
	}
}

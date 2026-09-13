package gateway

import (
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type ProfileAccountCounts struct {
	HTTPRequests          int `json:"httpRequests"`
	ModelRequests         int `json:"modelRequests"`
	ChainedRequests       int `json:"chainedRequests"`
	UncorrelatedReqs      int `json:"uncorrelatedRequests"`
	NonModelRequests      int `json:"nonModelRequests"`
	RecorderFailures      int `json:"recorderFailures"`
	UsageObservedRequests int `json:"usageObservedRequests"`
}

type OutcomeCounts struct {
	OutcomeUpstreamOK        int `json:"upstream_ok"`
	OutcomeUpstreamHTTPError int `json:"upstream_http_error"`
	OutcomeTransportFailure  int `json:"transport_failure"`
	OutcomeClientCanceled    int `json:"client_canceled"`
	OutcomeStreamTruncated   int `json:"stream_truncated"`
	OutcomeUnknown           int `json:"unknown"`
}

type FieldAggregate struct {
	Count int                   `json:"count"`
	Sum   int64                 `json:"sum"`
	Kind  model.MeasurementKind `json:"kind,omitempty"`
}

type ProfileUsageAggregate struct {
	RawInput      FieldAggregate `json:"rawInput"`
	Cached        FieldAggregate `json:"cached"`
	CacheCreation FieldAggregate `json:"cacheCreation"`
	TotalInput    FieldAggregate `json:"totalInput"`
	Output        FieldAggregate `json:"output"`
	Reasoning     FieldAggregate `json:"reasoning"`
	Total         FieldAggregate `json:"total"`
	ModelRequests int            `json:"modelRequests"`
	Observed      int            `json:"observed"`
	Complete      bool           `json:"complete"`
}

func (a *ProfileUsageAggregate) record(usage *ProviderUsage) {
	if usage == nil {
		return
	}
	a.Observed++
	if usage.InputTokens != nil {
		v := *usage.InputTokens
		mergeField(&a.RawInput, v)
	}
	if usage.CacheReadInputTokens != nil {
		v := *usage.CacheReadInputTokens
		mergeField(&a.Cached, v)
	}
	if usage.CacheCreationInputTokens != nil {
		v := *usage.CacheCreationInputTokens
		mergeField(&a.CacheCreation, v)
	}
	if usage.OutputTokens != nil {
		v := *usage.OutputTokens
		mergeField(&a.Output, v)
	}
	if usage.ReasoningOutputTokens != nil {
		v := *usage.ReasoningOutputTokens
		mergeField(&a.Reasoning, v)
	}
	observed := ProviderUsageToObserved(usage)
	mergeField(&a.TotalInput, observed.TotalInput)
	mergeField(&a.Total, observed.Total)
}

func mergeField(field *FieldAggregate, value int64) {
	field.Count++
	field.Sum += value
	if field.Kind == "" {
		field.Kind = model.MeasurementMeasured
	}
}

type ProfileAccount struct {
	Profile        string                `json:"profile"`
	Counts         ProfileAccountCounts  `json:"counts"`
	Outcomes       OutcomeCounts         `json:"outcomes"`
	Observed       ProfileUsageAggregate `json:"observed"`
	Chained        ProfileUsageAggregate `json:"chained"`
	Uncorrelated   ProfileUsageAggregate `json:"uncorrelated"`
	Completeness   string                `json:"completeness"`
	RecorderErrors []*RecorderError      `json:"recorderErrors,omitempty"`
}

func AccountProfile(profile string, exchanges []Exchange, chainResult ChainBuildResult, recorderErrors []*RecorderError) ProfileAccount {
	account := ProfileAccount{
		Profile:      profile,
		Completeness: "complete",
		Observed:     ProfileUsageAggregate{Complete: true},
		Chained:      ProfileUsageAggregate{Complete: true},
		Uncorrelated: ProfileUsageAggregate{Complete: true},
	}
	account.RecorderErrors = recorderErrors
	account.Counts.RecorderFailures = len(recorderErrors)

	chainSet := map[string]bool{}
	for _, chain := range chainResult.Chains {
		for _, ex := range chain.Exchanges {
			chainSet[ex.ID] = true
		}
	}

	for _, ex := range exchanges {
		account.Counts.HTTPRequests++
		bucketOutcome(&account.Outcomes, ex.Outcome)
		switch ex.Kind {
		case RequestKindModel:
			account.Counts.ModelRequests++
			if chainSet[ex.ID] {
				account.Counts.ChainedRequests++
			} else if ex.Kind != RequestKindNonModel {
				account.Counts.UncorrelatedReqs++
			}
		case RequestKindNonModel:
			account.Counts.NonModelRequests++
		}

		if !isChainModelCall(ex) {
			if ex.Kind != RequestKindNonModel {
				continue
			}
			continue
		}
		pu := ex.Response.ProviderUsage
		if pu == nil {
			continue
		}
		if chainSet[ex.ID] {
			account.Chained.record(pu)
			account.Observed.record(pu)
		} else {
			account.Uncorrelated.record(pu)
			account.Observed.record(pu)
		}
		account.Counts.UsageObservedRequests++
	}
	finalizeAggregate(&account.Observed, account.Counts.ModelRequests)
	finalizeAggregate(&account.Chained, account.Counts.ChainedRequests)
	finalizeAggregate(&account.Uncorrelated, account.Counts.UncorrelatedReqs)

	if account.Counts.ModelRequests == 0 {
		account.Completeness = "complete"
	} else if !account.Observed.Complete || !account.Chained.Complete || !account.Uncorrelated.Complete {
		account.Completeness = "partial"
	}
	if account.Counts.RecorderFailures > 0 {
		account.Completeness = "partial"
	}
	if account.Outcomes.OutcomeStreamTruncated > 0 {
		account.Completeness = "partial"
	}
	return account
}

func finalizeAggregate(agg *ProfileUsageAggregate, totalRequests int) {
	if agg.ModelRequests == 0 && totalRequests > 0 {
		agg.ModelRequests = totalRequests
	}
	if totalRequests == 0 {
		agg.Complete = true
		return
	}
	if agg.Observed < agg.ModelRequests {
		agg.Complete = false
		return
	}
	for _, field := range []FieldAggregate{agg.RawInput, agg.Output} {
		if field.Count < agg.ModelRequests {
			agg.Complete = false
			return
		}
	}
	agg.Complete = true
}

func bucketOutcome(c *OutcomeCounts, outcome RequestOutcome) {
	switch outcome {
	case OutcomeUpstreamOK:
		c.OutcomeUpstreamOK++
	case OutcomeUpstreamHTTPError:
		c.OutcomeUpstreamHTTPError++
	case OutcomeTransportFailure:
		c.OutcomeTransportFailure++
	case OutcomeClientCanceled:
		c.OutcomeClientCanceled++
	case OutcomeStreamTruncated:
		c.OutcomeStreamTruncated++
	default:
		c.OutcomeUnknown++
	}
}

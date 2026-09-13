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
	UnknownRequests       int `json:"unknownRequests"`
	UnclassifiedRequests  int `json:"unclassifiedRequests"`
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
	if !usage.HasUsage() {
		return
	}
	a.Observed++
	observed := ProviderUsageToObserved(usage)
	k := providerFieldKindsFor(usage)
	switch usage.Source {
	case string(ProtocolAnthropicMessages):
		if usage.InputTokens != nil {
			mergeField(&a.RawInput, observed.RawInput, k.Fresh)
		}
		if usage.CacheReadInputTokens != nil {
			mergeField(&a.Cached, observed.Cached, k.Cached)
		}
		if usage.CacheCreationInputTokens != nil {
			mergeField(&a.CacheCreation, observed.CacheCreation, k.CacheCreation)
		}
		if usage.InputTokens != nil || usage.TotalInputTokens != nil {
			mergeField(&a.TotalInput, observed.TotalInput, k.TotalInput)
		}
	case string(ProtocolOpenAIResponses):
		if usage.InputTokens != nil || usage.TotalInputTokens != nil {
			mergeField(&a.TotalInput, observed.TotalInput, k.TotalInput)
			if usage.CacheReadInputTokens != nil {
				mergeField(&a.Cached, observed.Cached, k.Cached)
				mergeField(&a.RawInput, observed.RawInput, k.Fresh)
			}
		}
	default:
		if usage.TotalInputTokens != nil {
			mergeField(&a.TotalInput, observed.TotalInput, k.TotalInput)
		}
	}
	if usage.OutputTokens != nil {
		mergeField(&a.Output, observed.Output, k.Output)
	}
	if usage.ReasoningOutputTokens != nil {
		mergeField(&a.Reasoning, observed.Reasoning, k.Reasoning)
	}
	a.recordTotal(usage, observed, k)
}

func (a *ProfileUsageAggregate) recordTotal(usage *ProviderUsage, observed ObservedUsage, k providerFieldKinds) {
	if usage.TotalTokens != nil {
		mergeField(&a.Total, *usage.TotalTokens, k.Total)
		return
	}
	total, ok := derivedTotal(usage, observed)
	if !ok {
		return
	}
	mergeField(&a.Total, total, k.Total)
}

func derivedTotal(usage *ProviderUsage, observed ObservedUsage) (int64, bool) {
	if !isKnownProviderSource(usage.Source) {
		return 0, false
	}
	hasInput := usage.InputTokens != nil || usage.TotalInputTokens != nil
	if !hasInput || usage.OutputTokens == nil {
		return 0, false
	}
	return observed.Total, true
}

func mergeField(field *FieldAggregate, value int64, kind model.MeasurementKind) {
	field.Count++
	field.Sum += value
	field.Kind = weakenKind(field.Kind, kind)
}

type ProfileAccount struct {
	SchemaVersion             int                   `json:"schemaVersion,omitempty"`
	Profile                   string                `json:"profile"`
	Counts                    ProfileAccountCounts  `json:"counts"`
	Outcomes                  OutcomeCounts         `json:"outcomes"`
	Observed                  ProfileUsageAggregate `json:"observed"`
	Chained                   ProfileUsageAggregate `json:"chained"`
	Uncorrelated              ProfileUsageAggregate `json:"uncorrelated"`
	Completeness              string                `json:"completeness"`
	RecorderFailures          []RecorderFailure     `json:"recorderFailures,omitempty"`
	RecorderFailuresTruncated bool                  `json:"recorderFailuresTruncated,omitempty"`
}

func AccountProfile(profile string, exchanges []Exchange, chainResult ChainBuildResult, failures RecorderFailureRead) ProfileAccount {
	account := ProfileAccount{
		SchemaVersion: 1,
		Profile:       profile,
		Completeness:  "complete",
		Observed:      ProfileUsageAggregate{Complete: true},
		Chained:       ProfileUsageAggregate{Complete: true},
		Uncorrelated:  ProfileUsageAggregate{Complete: true},
	}
	account.RecorderFailures = failures.Failures
	account.RecorderFailuresTruncated = failures.Truncated
	account.Counts.RecorderFailures = failures.Total

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
		case RequestKindUnknown:
			account.Counts.UnknownRequests++
		case RequestKindUnclassified:
			account.Counts.UnclassifiedRequests++
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
		if !isKnownProviderSource(pu.Source) {
			// A model request with an unrecognised usage schema must not
			// silently flip the profile to "complete".
			account.Observed.Complete = false
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
	// Completeness is computed against model traffic only; unknown and
	// unclassified requests are reported but do not affect the observed
	// versus complete ratio.
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

func isKnownProviderSource(source string) bool {
	return source == string(ProtocolAnthropicMessages) || source == string(ProtocolOpenAIResponses)
}

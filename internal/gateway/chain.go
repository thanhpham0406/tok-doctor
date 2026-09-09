package gateway

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type ChainRelation string

const (
	ChainRelationStart    ChainRelation = "start"
	ChainRelationContinue ChainRelation = "continue"
	ChainRelationUnknown  ChainRelation = "unknown"
)

type CorrelationMethod string

const (
	CorrelationExplicitID     CorrelationMethod = "explicit_id"
	CorrelationToolLineage    CorrelationMethod = "tool_lineage"
	CorrelationContextLineage CorrelationMethod = "context_lineage"
	CorrelationChainStart     CorrelationMethod = "chain_start"
	CorrelationUnknown        CorrelationMethod = "unknown"
)

type CorrelationEvidence struct {
	Kind     CorrelationMethod `json:"kind"`
	Source   string            `json:"source,omitempty"`
	Previous string            `json:"previous,omitempty"`
	Current  string            `json:"current,omitempty"`
	Field    string            `json:"field,omitempty"`
	Value    string            `json:"value,omitempty"`
}

type CorrelationResult struct {
	Relation ChainRelation         `json:"relation"`
	Method   CorrelationMethod     `json:"method,omitempty"`
	Evidence []CorrelationEvidence `json:"evidence,omitempty"`
}

type ChainCorrelator interface {
	Correlate(previous Exchange, current Exchange) CorrelationResult
}

type Chain struct {
	ID           string             `json:"id"`
	Profile      string             `json:"profile,omitempty"`
	Protocol     Protocol           `json:"protocol,omitempty"`
	Exchanges    []ChainExchange    `json:"exchanges,omitempty"`
	Correlations []ChainCorrelation `json:"correlations,omitempty"`
}

type ChainExchange struct {
	ID        string `json:"id"`
	StartedAt string `json:"startedAt,omitempty"`
}

type ChainCorrelation struct {
	PreviousExchangeID string                `json:"previousExchangeId,omitempty"`
	CurrentExchangeID  string                `json:"currentExchangeId"`
	Relation           ChainRelation         `json:"relation"`
	Method             CorrelationMethod     `json:"method,omitempty"`
	Evidence           []CorrelationEvidence `json:"evidence,omitempty"`
}

type ChainBuildResult struct {
	Chains       []Chain    `json:"chains,omitempty"`
	Uncorrelated []Exchange `json:"uncorrelated,omitempty"`
}

type ChainCallSummary struct {
	Index               int                       `json:"index"`
	ExchangeID          string                    `json:"exchangeId"`
	StartedAt           string                    `json:"startedAt,omitempty"`
	Model               string                    `json:"model,omitempty"`
	Context             model.Measurement         `json:"context"`
	ContextDelta        model.Measurement         `json:"contextDelta"`
	Categories          CategoryBreakdown         `json:"categories"`
	Files               []FileAttribution         `json:"files,omitempty"`
	ProviderUsage       *CallProviderUsage        `json:"providerUsage,omitempty"`
	AttributionGap      *model.Measurement        `json:"attributionGap,omitempty"`
	AttributionCoverage *AttributionCoverageValue `json:"attributionCoverage,omitempty"`
}

type ChainSummary struct {
	ID                  string                    `json:"id"`
	Profile             string                    `json:"profile,omitempty"`
	Protocol            Protocol                  `json:"protocol,omitempty"`
	Model               string                    `json:"model,omitempty"`
	StartedAt           string                    `json:"startedAt,omitempty"`
	ModelCalls          int                       `json:"modelCalls"`
	Calls               []ChainCallSummary        `json:"calls,omitempty"`
	TotalContextSent    model.Measurement         `json:"totalContext"`
	InitialContext      model.Measurement         `json:"initialContext"`
	PeakContext         model.Measurement         `json:"peakContext"`
	FinalContext        model.Measurement         `json:"finalContext"`
	ContextGrowth       model.Measurement         `json:"contextGrowth"`
	LargestGrowth       model.Measurement         `json:"largestGrowth"`
	LargestGrowthCall   int                       `json:"largestGrowthCall,omitempty"`
	ToolResultsGrowth   model.Measurement         `json:"toolResultsGrowth"`
	HistoryGrowth       model.Measurement         `json:"historyGrowth"`
	ToolsGrowth         model.Measurement         `json:"toolsGrowth"`
	ProviderInput       *model.Measurement        `json:"providerInput,omitempty"`
	ProviderCachedInput *model.Measurement        `json:"providerCachedInput,omitempty"`
	ProviderFreshInput  *model.Measurement        `json:"providerFreshInput,omitempty"`
	ProviderOutput      *model.Measurement        `json:"providerOutput,omitempty"`
	ProviderReasoning   *model.Measurement        `json:"providerReasoning,omitempty"`
	UsageObservedCalls  *int                      `json:"usageObservedCalls,omitempty"`
	UsageTotalCalls     *int                      `json:"usageTotalCalls,omitempty"`
	AttributionGap      *model.Measurement        `json:"attributionGap,omitempty"`
	AttributionCoverage *AttributionCoverageValue `json:"attributionCoverage,omitempty"`
}

type AttributionCoverageValue struct {
	Percent *float64 `json:"percent,omitempty"`
	Kind    string   `json:"kind,omitempty"`
}

type CallProviderUsage struct {
	Input           *int64 `json:"input,omitempty"`
	CachedInput     *int64 `json:"cachedInput,omitempty"`
	FreshInput      *int64 `json:"freshInput,omitempty"`
	Output          *int64 `json:"output,omitempty"`
	ReasoningOutput *int64 `json:"reasoningOutput,omitempty"`
	Kind            string `json:"kind,omitempty"`
}

var (
	ErrChainNotFound  = errors.New("chain not found")
	ErrChainAmbiguous = errors.New("ambiguous chain id")
)

type ChainBuilder struct {
	correlators map[Protocol]ChainCorrelator
}

func AnalyzeChains(exchanges []Exchange) []ChainSummary {
	ordered := append([]Exchange(nil), exchanges...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].StartedAt.Equal(ordered[j].StartedAt) {
			return ordered[i].StartedAt.Before(ordered[j].StartedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	byID := map[string]Exchange{}
	for _, exchange := range ordered {
		byID[exchange.ID] = exchange
	}
	built := NewChainBuilder().Build(ordered)
	summaries := make([]ChainSummary, 0, len(built.Chains))
	for _, chain := range built.Chains {
		summaries = append(summaries, summarizeChain(chain, byID))
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].StartedAt > summaries[j].StartedAt
	})
	return summaries
}

func FindChain(summaries []ChainSummary, id string) (ChainSummary, error) {
	if id == "" {
		return ChainSummary{}, fmt.Errorf("chain id is required: %w", ErrChainNotFound)
	}
	for _, summary := range summaries {
		if summary.ID == id {
			return summary, nil
		}
	}
	var matches []ChainSummary
	for _, summary := range summaries {
		if strings.HasPrefix(summary.ID, id) {
			matches = append(matches, summary)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return ChainSummary{}, fmt.Errorf("chain %q: %w", id, ErrChainNotFound)
	default:
		return ChainSummary{}, fmt.Errorf("chain %q: %w", id, ErrChainAmbiguous)
	}
}

func summarizeChain(chain Chain, exchanges map[string]Exchange) ChainSummary {
	summary := ChainSummary{
		ID:       chain.ID,
		Profile:  chain.Profile,
		Protocol: chain.Protocol,
	}
	for i, entry := range chain.Exchanges {
		exchange, ok := exchanges[entry.ID]
		if !ok {
			continue
		}
		request := SummarizeRequest(exchange)
		call := ChainCallSummary{
			Index:      len(summary.Calls) + 1,
			ExchangeID: request.ExchangeID,
			StartedAt:  request.StartedAt,
			Model:      request.Model,
			Context:    request.Attributed.Value,
			Categories: request.Categories,
			Files:      request.Files,
		}
		if i > 0 && len(summary.Calls) > 0 {
			call.ContextDelta = subtractMetric(call.Context, summary.Calls[len(summary.Calls)-1].Context)
		} else {
			call.ContextDelta = model.Measurement{Kind: model.MeasurementUnknown}
		}
		call.ProviderUsage, call.AttributionGap, call.AttributionCoverage = summarizeCallProviderUsage(exchange, request)
		summary.Calls = append(summary.Calls, call)
	}
	summary.ModelCalls = len(summary.Calls)
	if len(summary.Calls) == 0 {
		return summary
	}
	summary.StartedAt = summary.Calls[0].StartedAt
	summary.Model = summary.Calls[0].Model
	summary.InitialContext = summary.Calls[0].Context
	summary.FinalContext = summary.Calls[len(summary.Calls)-1].Context
	summary.TotalContextSent = sumAllCallContext(summary.Calls)
	summary.PeakContext = peakContext(summary.Calls)
	summary.ContextGrowth = subtractMetric(summary.FinalContext, summary.InitialContext)
	summary.LargestGrowth, summary.LargestGrowthCall = largestGrowth(summary.Calls)
	summary.ToolResultsGrowth = subtractMetric(summary.Calls[len(summary.Calls)-1].Categories.ToolResult, summary.Calls[0].Categories.ToolResult)
	summary.HistoryGrowth = subtractMetric(summary.Calls[len(summary.Calls)-1].Categories.History, summary.Calls[0].Categories.History)
	summary.ToolsGrowth = subtractMetric(summary.Calls[len(summary.Calls)-1].Categories.ToolDefinition, summary.Calls[0].Categories.ToolDefinition)
	summary.ProviderInput, summary.ProviderCachedInput, summary.ProviderFreshInput,
		summary.ProviderOutput, summary.ProviderReasoning, summary.UsageObservedCalls,
		summary.UsageTotalCalls = summarizeChainProviderUsage(summary.Calls)
	if summary.ProviderInput != nil && summary.TotalContextSent.Available() {
		gap := subtractMetric(*summary.ProviderInput, summary.TotalContextSent)
		summary.AttributionGap = &gap
		summary.AttributionCoverage = computeCoverage(summary.TotalContextSent, *summary.ProviderInput, summary.TotalContextSent.Kind)
	}
	return summary
}

func summarizeCallProviderUsage(exchange Exchange, request RequestSummary) (*CallProviderUsage, *model.Measurement, *AttributionCoverageValue) {
	pu := exchange.Response.ProviderUsage
	if pu == nil {
		return nil, nil, nil
	}
	out := &CallProviderUsage{Kind: "measured"}
	if pu.Input != nil {
		v := *pu.Input
		out.Input = &v
	}
	if pu.CachedInput != nil {
		v := *pu.CachedInput
		out.CachedInput = &v
	}
	if pu.Output != nil {
		v := *pu.Output
		out.Output = &v
	}
	if pu.ReasoningOutput != nil {
		v := *pu.ReasoningOutput
		out.ReasoningOutput = &v
	}
	if out.Input != nil && out.CachedInput != nil {
		fresh := *out.Input - *out.CachedInput
		out.FreshInput = &fresh
	}
	if out.Input == nil && out.CachedInput == nil && out.Output == nil && out.ReasoningOutput == nil {
		return nil, nil, nil
	}
	var providerInput model.Measurement
	if out.Input != nil {
		providerInput = model.NewMeasurement(*out.Input, model.MeasurementMeasured)
	}
	attributed := request.Attributed.Value
	var gap *model.Measurement
	if providerInput.Available() && attributed.Available() {
		g := subtractMetric(providerInput, attributed)
		gap = &g
	}
	var coverage *AttributionCoverageValue
	if providerInput.Available() && attributed.Available() {
		coverage = computeCoverage(attributed, providerInput, attributed.Kind)
	}
	return out, gap, coverage
}

func summarizeChainProviderUsage(calls []ChainCallSummary) (*model.Measurement, *model.Measurement, *model.Measurement, *model.Measurement, *model.Measurement, *int, *int) {
	if len(calls) == 0 {
		return nil, nil, nil, nil, nil, nil, nil
	}
	total := len(calls)
	observed := 0
	var inputSum, cachedSum, freshSum, outputSum, reasoningSum model.Measurement
	inputAll := true
	cachedAll := true
	freshAll := true
	outputAll := true
	reasoningAll := true
	for _, call := range calls {
		pu := call.ProviderUsage
		if pu == nil {
			inputAll = false
			cachedAll = false
			freshAll = false
			outputAll = false
			reasoningAll = false
			continue
		}
		observed++
		if pu.Input != nil {
			inputSum = addMeasurement(inputSum, model.NewMeasurement(*pu.Input, model.MeasurementMeasured))
		} else {
			inputAll = false
		}
		if pu.CachedInput != nil {
			cachedSum = addMeasurement(cachedSum, model.NewMeasurement(*pu.CachedInput, model.MeasurementMeasured))
		} else {
			cachedAll = false
		}
		if pu.FreshInput != nil {
			freshSum = addMeasurement(freshSum, model.NewMeasurement(*pu.FreshInput, model.MeasurementDerived))
		} else {
			freshAll = false
		}
		if pu.Output != nil {
			outputSum = addMeasurement(outputSum, model.NewMeasurement(*pu.Output, model.MeasurementMeasured))
		} else {
			outputAll = false
		}
		if pu.ReasoningOutput != nil {
			reasoningSum = addMeasurement(reasoningSum, model.NewMeasurement(*pu.ReasoningOutput, model.MeasurementMeasured))
		} else {
			reasoningAll = false
		}
	}
	ptr := func(m model.Measurement, ok bool) *model.Measurement {
		if !ok || !m.Available() {
			return nil
		}
		out := m
		return &out
	}
	observedPtr := func(v int, ok bool) *int {
		if !ok {
			return nil
		}
		out := v
		return &out
	}
	input := ptr(inputSum, inputAll)
	cached := ptr(cachedSum, cachedAll)
	fresh := ptr(freshSum, freshAll)
	output := ptr(outputSum, outputAll)
	reasoning := ptr(reasoningSum, reasoningAll)
	if observed == 0 {
		return nil, nil, nil, nil, nil, observedPtr(0, true), observedPtr(total, true)
	}
	return input, cached, fresh, output, reasoning, observedPtr(observed, true), observedPtr(total, true)
}

func computeCoverage(attributed, provider model.Measurement, attributionKind model.MeasurementKind) *AttributionCoverageValue {
	if !attributed.Available() || !provider.Available() {
		return nil
	}
	if provider.ValueOrZero() == 0 {
		return nil
	}
	percent := float64(attributed.ValueOrZero()) / float64(provider.ValueOrZero()) * 100
	kind := "measured"
	if attributionKind != model.MeasurementMeasured {
		kind = "estimated"
	}
	out := &AttributionCoverageValue{Percent: &percent, Kind: kind}
	return out
}

func sumAllCallContext(calls []ChainCallSummary) model.Measurement {
	var out model.Measurement
	for _, call := range calls {
		if !call.Context.Available() {
			return model.Measurement{Kind: model.MeasurementUnknown}
		}
		out = addMeasurement(out, call.Context)
	}
	return out
}

func peakContext(calls []ChainCallSummary) model.Measurement {
	var out model.Measurement
	for _, call := range calls {
		if !call.Context.Available() {
			return model.Measurement{Kind: model.MeasurementUnknown}
		}
		if !out.Available() || call.Context.ValueOrZero() > out.ValueOrZero() {
			out = call.Context
			continue
		}
		out.Kind = weakenKind(out.Kind, call.Context.Kind)
	}
	return out
}

func largestGrowth(calls []ChainCallSummary) (model.Measurement, int) {
	var out model.Measurement
	found := false
	for _, call := range calls {
		if call.Index == 1 {
			continue
		}
		if !call.ContextDelta.Available() {
			return model.Measurement{Kind: model.MeasurementUnknown}, 0
		}
		if !found || call.ContextDelta.ValueOrZero() > out.ValueOrZero() {
			out = call.ContextDelta
			found = true
		}
	}
	if !found {
		return model.Measurement{Kind: model.MeasurementUnknown}, 0
	}
	if out.ValueOrZero() <= 0 {
		return model.NewMeasurement(0, out.Kind), 0
	}
	for _, call := range calls {
		if call.ContextDelta.Available() && call.ContextDelta.ValueOrZero() == out.ValueOrZero() {
			return out, call.Index
		}
	}
	return out, 0
}

func subtractMetric(left, right model.Measurement) model.Measurement {
	if !left.Available() || !right.Available() {
		return model.Measurement{Kind: model.MeasurementUnknown}
	}
	return model.NewMeasurement(left.ValueOrZero()-right.ValueOrZero(), weakenKind(left.Kind, right.Kind))
}

func NewChainBuilder() ChainBuilder {
	return ChainBuilder{
		correlators: map[Protocol]ChainCorrelator{
			ProtocolAnthropicMessages: AnthropicMessagesCorrelator{},
			ProtocolOpenAIResponses:   OpenAIResponsesCorrelator{},
		},
	}
}

func (b ChainBuilder) Build(exchanges []Exchange) ChainBuildResult {
	ordered := append([]Exchange(nil), exchanges...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].StartedAt.Equal(ordered[j].StartedAt) {
			return ordered[i].StartedAt.Before(ordered[j].StartedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	var result ChainBuildResult
	for _, exchange := range ordered {
		correlator := b.correlators[exchange.Protocol]
		if correlator == nil || !isChainModelCall(exchange) {
			continue
		}
		type candidate struct {
			index  int
			result CorrelationResult
		}
		var candidates []candidate
		for i := range result.Chains {
			last := result.Chains[i].Exchanges[len(result.Chains[i].Exchanges)-1]
			previous, ok := findExchangeByID(ordered, last.ID)
			if !ok {
				continue
			}
			correlation := correlator.Correlate(previous, exchange)
			if correlation.Relation == ChainRelationContinue {
				candidates = append(candidates, candidate{index: i, result: correlation})
			}
		}
		if len(candidates) == 1 {
			chain := &result.Chains[candidates[0].index]
			previousID := chain.Exchanges[len(chain.Exchanges)-1].ID
			chain.Exchanges = append(chain.Exchanges, chainExchange(exchange))
			chain.Correlations = append(chain.Correlations, ChainCorrelation{
				PreviousExchangeID: previousID,
				CurrentExchangeID:  exchange.ID,
				Relation:           candidates[0].result.Relation,
				Method:             candidates[0].result.Method,
				Evidence:           candidates[0].result.Evidence,
			})
			continue
		}
		start := correlator.Correlate(Exchange{}, exchange)
		if len(candidates) == 0 && start.Relation == ChainRelationStart {
			result.Chains = append(result.Chains, Chain{
				ID:       chainID(exchange.ID),
				Profile:  exchange.Profile,
				Protocol: exchange.Protocol,
				Exchanges: []ChainExchange{
					chainExchange(exchange),
				},
				Correlations: []ChainCorrelation{{
					CurrentExchangeID: exchange.ID,
					Relation:          start.Relation,
					Method:            start.Method,
					Evidence:          start.Evidence,
				}},
			})
			continue
		}
		result.Uncorrelated = append(result.Uncorrelated, exchange)
	}
	return result
}

func findExchangeByID(exchanges []Exchange, id string) (Exchange, bool) {
	for _, exchange := range exchanges {
		if exchange.ID == id {
			return exchange, true
		}
	}
	return Exchange{}, false
}

func isChainModelCall(exchange Exchange) bool {
	if exchange.Request.Method != "" && exchange.Request.Method != "POST" {
		return false
	}
	return chainEligibleEndpoints[exchange.Protocol][exchange.Request.Endpoint]
}

var chainEligibleEndpoints = map[Protocol]map[string]bool{
	ProtocolAnthropicMessages: {
		"/v1/messages":              true,
		"/v1/messages/count_tokens": false,
	},
	ProtocolOpenAIResponses: {
		"/responses":    true,
		"/v1/responses": true,
		"/models":       false,
		"/v1/models":    false,
	},
}

func chainExchange(exchange Exchange) ChainExchange {
	out := ChainExchange{ID: exchange.ID}
	if !exchange.StartedAt.IsZero() {
		out.StartedAt = exchange.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return out
}

func chainID(exchangeID string) string {
	suffix := strings.TrimPrefix(exchangeID, "gw-")
	if len(suffix) > 12 {
		suffix = suffix[len(suffix)-12:]
	}
	if suffix == "" {
		suffix = "unknown"
	}
	return "gwc-" + suffix
}

type AnthropicMessagesCorrelator struct{}

func (AnthropicMessagesCorrelator) Correlate(previous Exchange, current Exchange) CorrelationResult {
	if previous.ID == "" {
		if anthropicHasStartShape(current) {
			return CorrelationResult{
				Relation: ChainRelationStart,
				Method:   CorrelationChainStart,
				Evidence: []CorrelationEvidence{{
					Kind:    CorrelationChainStart,
					Source:  string(ProtocolAnthropicMessages),
					Current: current.ID,
					Field:   "messages.role=user",
				}},
			}
		}
		return unknownCorrelation()
	}
	if previous.Protocol != current.Protocol || previous.Profile != current.Profile {
		return unknownCorrelation()
	}
	if !modelsCompatible(previous.Model, current.Model) {
		return unknownCorrelation()
	}
	if result := anthropicToolLineage(previous, current); result.Relation == ChainRelationContinue {
		return result
	}
	return anthropicContextLineage(previous, current)
}

func anthropicToolLineage(previous Exchange, current Exchange) CorrelationResult {
	uses := anthropicToolUses(previous)
	results := anthropicToolResults(current)
	var evidence []CorrelationEvidence
	for id, use := range uses {
		result, ok := results[id]
		if !ok {
			continue
		}
		evidence = append(evidence, CorrelationEvidence{
			Kind:     CorrelationToolLineage,
			Source:   string(ProtocolAnthropicMessages),
			Previous: previous.ID,
			Current:  current.ID,
			Field:    use + "->" + result,
			Value:    id,
		})
	}
	if len(evidence) == 0 {
		return unknownCorrelation()
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		return evidence[i].Value < evidence[j].Value
	})
	return CorrelationResult{
		Relation: ChainRelationContinue,
		Method:   CorrelationToolLineage,
		Evidence: evidence,
	}
}

func anthropicContextLineage(previous Exchange, current Exchange) CorrelationResult {
	previousHashes := lineageHashes(previous.Request.Components)
	currentHashes := lineageHashes(current.Request.Components)
	if len(previousHashes) == 0 || len(currentHashes) <= len(previousHashes) {
		return unknownCorrelation()
	}
	for hash := range previousHashes {
		if _, ok := currentHashes[hash]; !ok {
			return unknownCorrelation()
		}
	}
	return CorrelationResult{
		Relation: ChainRelationContinue,
		Method:   CorrelationContextLineage,
		Evidence: []CorrelationEvidence{{
			Kind:     CorrelationContextLineage,
			Source:   string(ProtocolAnthropicMessages),
			Previous: previous.ID,
			Current:  current.ID,
			Field:    "component_hashes",
		}},
	}
}

func lineageHashes(components []model.ContextComponent) map[string]struct{} {
	out := map[string]struct{}{}
	for _, component := range components {
		if component.ContentHash == "" {
			continue
		}
		switch component.Kind {
		case model.ContextHistory, model.ContextToolResult, model.ContextFile:
			out[component.ContentHash] = struct{}{}
		}
	}
	return out
}

func anthropicToolUses(exchange Exchange) map[string]string {
	out := map[string]string{}
	meta := exchange.Request.Metadata.AnthropicMessages
	if meta == nil {
		return out
	}
	for _, message := range meta.Messages {
		if message.Role != "assistant" {
			continue
		}
		for _, block := range message.Blocks {
			if block.ToolUseID == "" {
				continue
			}
			out[block.ToolUseID] = anthropicBlockField(message.Index, block.Index, "tool_use.id")
		}
	}
	return out
}

func anthropicToolResults(exchange Exchange) map[string]string {
	out := map[string]string{}
	meta := exchange.Request.Metadata.AnthropicMessages
	if meta == nil {
		return out
	}
	for _, message := range meta.Messages {
		if message.Role != "user" {
			continue
		}
		for _, block := range message.Blocks {
			if block.ToolResultToolUseID == "" {
				continue
			}
			out[block.ToolResultToolUseID] = anthropicBlockField(message.Index, block.Index, "tool_result.tool_use_id")
		}
	}
	return out
}

func anthropicHasStartShape(exchange Exchange) bool {
	meta := exchange.Request.Metadata.AnthropicMessages
	if meta == nil {
		return false
	}
	for _, message := range meta.Messages {
		if message.Role != "user" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Type != "tool_result" {
				return true
			}
		}
	}
	return false
}

func anthropicBlockField(messageIndex, blockIndex int, field string) string {
	return "messages[" + strconv.Itoa(messageIndex) + "].content[" + strconv.Itoa(blockIndex) + "]." + field
}

func modelsCompatible(previous, current string) bool {
	return previous == "" || current == "" || previous == current
}

func unknownCorrelation() CorrelationResult {
	return CorrelationResult{
		Relation: ChainRelationUnknown,
		Method:   CorrelationUnknown,
	}
}

type OpenAIResponsesCorrelator struct{}

func (OpenAIResponsesCorrelator) Correlate(previous Exchange, current Exchange) CorrelationResult {
	if previous.ID == "" {
		if openAIHasStartShape(current) {
			return CorrelationResult{
				Relation: ChainRelationStart,
				Method:   CorrelationChainStart,
				Evidence: []CorrelationEvidence{{
					Kind:    CorrelationChainStart,
					Source:  string(ProtocolOpenAIResponses),
					Current: current.ID,
					Field:   "input[0].role=user",
				}},
			}
		}
		return unknownCorrelation()
	}
	if previous.Protocol != current.Protocol || previous.Profile != current.Profile {
		return unknownCorrelation()
	}
	if !modelsCompatible(previous.Model, current.Model) {
		return unknownCorrelation()
	}
	if result := openAIExplicitIDLineage(previous, current); result.Relation == ChainRelationContinue {
		return result
	}
	if result := openAIToolLineage(previous, current); result.Relation == ChainRelationContinue {
		return result
	}
	return openAIContextLineage(previous, current)
}

func openAIExplicitIDLineage(previous Exchange, current Exchange) CorrelationResult {
	cur := current.Request.Metadata.OpenAIResponses
	if cur == nil || cur.PreviousResponseID == "" {
		return unknownCorrelation()
	}
	prev := previous.Response.OpenAIResponses
	if prev == nil || prev.ResponseID == "" {
		return unknownCorrelation()
	}
	if cur.PreviousResponseID != prev.ResponseID {
		return unknownCorrelation()
	}
	return CorrelationResult{
		Relation: ChainRelationContinue,
		Method:   CorrelationExplicitID,
		Evidence: []CorrelationEvidence{{
			Kind:     CorrelationExplicitID,
			Source:   string(ProtocolOpenAIResponses),
			Previous: previous.ID,
			Current:  current.ID,
			Field:    "previous_response_id",
			Value:    cur.PreviousResponseID,
		}},
	}
}

func openAIToolLineage(previous Exchange, current Exchange) CorrelationResult {
	prev := previous.Response.OpenAIResponses
	cur := current.Request.Metadata.OpenAIResponses
	if prev == nil || cur == nil {
		return unknownCorrelation()
	}
	var evidence []CorrelationEvidence
	for _, callID := range prev.OutputItemCallIDs {
		for _, outputID := range cur.FunctionCallOutputIDs {
			if callID == outputID {
				evidence = append(evidence, CorrelationEvidence{
					Kind:     CorrelationToolLineage,
					Source:   string(ProtocolOpenAIResponses),
					Previous: previous.ID,
					Current:  current.ID,
					Field:    "function_call.call_id->function_call_output.call_id",
					Value:    callID,
				})
				break
			}
		}
	}
	if len(evidence) == 0 {
		return unknownCorrelation()
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		return evidence[i].Value < evidence[j].Value
	})
	return CorrelationResult{
		Relation: ChainRelationContinue,
		Method:   CorrelationToolLineage,
		Evidence: evidence,
	}
}

func openAIContextLineage(previous Exchange, current Exchange) CorrelationResult {
	previousHashes := lineageHashes(previous.Request.Components)
	currentHashes := lineageHashes(current.Request.Components)
	if len(previousHashes) == 0 || len(currentHashes) <= len(previousHashes) {
		return unknownCorrelation()
	}
	for hash := range previousHashes {
		if _, ok := currentHashes[hash]; !ok {
			return unknownCorrelation()
		}
	}
	return CorrelationResult{
		Relation: ChainRelationContinue,
		Method:   CorrelationContextLineage,
		Evidence: []CorrelationEvidence{{
			Kind:     CorrelationContextLineage,
			Source:   string(ProtocolOpenAIResponses),
			Previous: previous.ID,
			Current:  current.ID,
			Field:    "component_hashes",
		}},
	}
}

func openAIHasStartShape(exchange Exchange) bool {
	cur := exchange.Request.Metadata.OpenAIResponses
	if cur == nil {
		return false
	}
	for _, item := range cur.Items {
		if item.Type == "message" && (item.ItemID != "" || item.Index == 0) {
			return true
		}
	}
	return false
}

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
	Index        int               `json:"index"`
	ExchangeID   string            `json:"exchangeId"`
	StartedAt    string            `json:"startedAt,omitempty"`
	Model        string            `json:"model,omitempty"`
	Context      model.Measurement `json:"context"`
	ContextDelta model.Measurement `json:"contextDelta"`
	Categories   CategoryBreakdown `json:"categories"`
	Files        []FileAttribution `json:"files,omitempty"`
}

type ChainSummary struct {
	ID                string             `json:"id"`
	Profile           string             `json:"profile,omitempty"`
	Protocol          Protocol           `json:"protocol,omitempty"`
	Model             string             `json:"model,omitempty"`
	StartedAt         string             `json:"startedAt,omitempty"`
	ModelCalls        int                `json:"modelCalls"`
	Calls             []ChainCallSummary `json:"calls,omitempty"`
	TotalContextSent  model.Measurement  `json:"totalContext"`
	InitialContext    model.Measurement  `json:"initialContext"`
	PeakContext       model.Measurement  `json:"peakContext"`
	FinalContext      model.Measurement  `json:"finalContext"`
	ContextGrowth     model.Measurement  `json:"contextGrowth"`
	LargestGrowth     model.Measurement  `json:"largestGrowth"`
	LargestGrowthCall int                `json:"largestGrowthCall,omitempty"`
	ToolResultsGrowth model.Measurement  `json:"toolResultsGrowth"`
	HistoryGrowth     model.Measurement  `json:"historyGrowth"`
	ToolsGrowth       model.Measurement  `json:"toolsGrowth"`
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
	return summary
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
	if exchange.Protocol == ProtocolAnthropicMessages {
		return exchange.Request.Endpoint == "/v1/messages"
	}
	return false
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

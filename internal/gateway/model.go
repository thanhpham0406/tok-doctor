package gateway

import (
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type Protocol string

const (
	ProtocolAnthropicMessages Protocol = "anthropic_messages"
	ProtocolOpenAIResponses   Protocol = "openai_responses"
)

func IsSupportedProtocol(name string) bool {
	switch Protocol(name) {
	case ProtocolAnthropicMessages, ProtocolOpenAIResponses:
		return true
	}
	return false
}

type RequestKind string

const (
	RequestKindUnknown      RequestKind = ""
	RequestKindModel        RequestKind = "model"
	RequestKindNonModel     RequestKind = "non_model"
	RequestKindUnclassified RequestKind = "unclassified"
)

type RequestOutcome string

const (
	OutcomeUnknown           RequestOutcome = ""
	OutcomeUpstreamOK        RequestOutcome = "upstream_ok"
	OutcomeUpstreamHTTPError RequestOutcome = "upstream_http_error"
	OutcomeTransportFailure  RequestOutcome = "transport_failure"
	OutcomeClientCanceled    RequestOutcome = "client_canceled"
	OutcomeStreamTruncated   RequestOutcome = "stream_truncated"
)

type Exchange struct {
	ID         string    `json:"id"`
	Profile    string    `json:"profile"`
	SourceHint string    `json:"source,omitempty"`
	Protocol   Protocol  `json:"protocol"`
	StartedAt  time.Time `json:"startedAt"`
	Upstream   string    `json:"upstream"`
	Model      string    `json:"model,omitempty"`

	Kind    RequestKind    `json:"kind,omitempty"`
	Outcome RequestOutcome `json:"outcome,omitempty"`

	Request  ExchangeRequest  `json:"request"`
	Response ExchangeResponse `json:"response"`
}

type ExchangeRequest struct {
	Method     string                   `json:"method"`
	Endpoint   string                   `json:"endpoint"`
	Bytes      int64                    `json:"bytes"`
	BodyHash   string                   `json:"bodyHash,omitempty"`
	Components []model.ContextComponent `json:"components,omitempty"`
	Metadata   ExchangeRequestMetadata  `json:"metadata,omitempty"`
}

type ExchangeRequestMetadata struct {
	AnthropicMessages *AnthropicMessagesMetadata `json:"anthropicMessages,omitempty"`
	OpenAIResponses   *OpenAIResponsesMetadata   `json:"openaiResponses,omitempty"`
}

type AnthropicMessagesMetadata struct {
	Messages []AnthropicMessageMetadata `json:"messages,omitempty"`
}

type AnthropicMessageMetadata struct {
	Index  int                      `json:"index"`
	Role   string                   `json:"role,omitempty"`
	Blocks []AnthropicBlockMetadata `json:"blocks,omitempty"`
}

type AnthropicBlockMetadata struct {
	Index               int    `json:"index"`
	Type                string `json:"type,omitempty"`
	ToolUseID           string `json:"toolUseId,omitempty"`
	ToolResultToolUseID string `json:"toolResultToolUseId,omitempty"`
}

type OpenAIResponsesMetadata struct {
	PreviousResponseID    string                        `json:"previousResponseId,omitempty"`
	FunctionCallIDs       []string                      `json:"functionCallIds,omitempty"`
	FunctionCallOutputIDs []string                      `json:"functionCallOutputIds,omitempty"`
	Items                 []OpenAIResponsesItemMetadata `json:"items,omitempty"`
}

type OpenAIResponsesItemMetadata struct {
	Index        int    `json:"index"`
	Type         string `json:"type,omitempty"`
	ItemID       string `json:"itemId,omitempty"`
	CallID       string `json:"callId,omitempty"`
	OutputCallID string `json:"outputCallId,omitempty"`
}

type ExchangeResponse struct {
	Status          int                          `json:"status"`
	ResponseID      string                       `json:"responseId,omitempty"`
	Model           string                       `json:"model,omitempty"`
	LatencyMs       int64                        `json:"latencyMs"`
	Usage           *ObservedUsage               `json:"usage,omitempty"`
	ProviderUsage   *ProviderUsage               `json:"providerUsage,omitempty"`
	Stream          bool                         `json:"stream"`
	OpenAIResponses *OpenAIResponsesResponseMeta `json:"openaiResponses,omitempty"`
}

type OpenAIResponsesResponseMeta struct {
	ResponseID        string   `json:"responseId,omitempty"`
	OutputItemCallIDs []string `json:"outputItemCallIDs,omitempty"`
}

type ObservedUsage struct {
	RawInput      int64                 `json:"rawInput"`
	Cached        int64                 `json:"cached"`
	CacheCreation int64                 `json:"cacheCreation"`
	TotalInput    int64                 `json:"totalInput"`
	Output        int64                 `json:"output"`
	Reasoning     int64                 `json:"reasoning"`
	Total         int64                 `json:"total"`
	Source        model.MeasurementKind `json:"source,omitempty"`
	Truncated     bool                  `json:"truncated,omitempty"`
}

type ProviderUsage struct {
	InputTokens              *int64 `json:"inputTokens,omitempty"`
	CacheCreationInputTokens *int64 `json:"cacheCreationInputTokens,omitempty"`
	CacheReadInputTokens     *int64 `json:"cacheReadInputTokens,omitempty"`
	OutputTokens             *int64 `json:"outputTokens,omitempty"`
	ReasoningOutputTokens    *int64 `json:"reasoningOutputTokens,omitempty"`
	TotalInputTokens         *int64 `json:"totalInputTokens,omitempty"`
	TotalTokens              *int64 `json:"totalTokens,omitempty"`
	Source                   string `json:"source,omitempty"`
}

func (p *ProviderUsage) ReportedFields() int {
	if p == nil {
		return 0
	}
	n := 0
	if p.InputTokens != nil {
		n++
	}
	if p.CacheCreationInputTokens != nil {
		n++
	}
	if p.CacheReadInputTokens != nil {
		n++
	}
	if p.OutputTokens != nil {
		n++
	}
	if p.ReasoningOutputTokens != nil {
		n++
	}
	if p.TotalInputTokens != nil {
		n++
	}
	if p.TotalTokens != nil {
		n++
	}
	return n
}

type AttributionCoverage struct {
	Percent *float64 `json:"percent,omitempty"`
	Kind    string   `json:"kind,omitempty"`
}

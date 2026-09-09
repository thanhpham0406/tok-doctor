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

type Exchange struct {
	ID         string    `json:"id"`
	Profile    string    `json:"profile"`
	SourceHint string    `json:"source,omitempty"`
	Protocol   Protocol  `json:"protocol"`
	StartedAt  time.Time `json:"startedAt"`
	Upstream   string    `json:"upstream"`
	Model      string    `json:"model,omitempty"`

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
	Finish          string                       `json:"finish,omitempty"`
	OpenAIResponses *OpenAIResponsesResponseMeta `json:"openaiResponses,omitempty"`
}

type OpenAIResponsesResponseMeta struct {
	ResponseID        string   `json:"responseId,omitempty"`
	OutputItemCallIDs []string `json:"outputItemCallIds,omitempty"`
}

type ObservedUsage struct {
	Input     int64                 `json:"input"`
	Cached    int64                 `json:"cached"`
	Output    int64                 `json:"output"`
	Reasoning int64                 `json:"reasoning"`
	Total     int64                 `json:"total"`
	Source    model.MeasurementKind `json:"source,omitempty"`
}

type ProviderUsage struct {
	Input           *int64 `json:"input,omitempty"`
	CachedInput     *int64 `json:"cachedInput,omitempty"`
	Output          *int64 `json:"output,omitempty"`
	ReasoningOutput *int64 `json:"reasoningOutput,omitempty"`
	Source          string `json:"source,omitempty"`
}

type AttributionCoverage struct {
	Percent *float64 `json:"percent,omitempty"`
	Kind    string   `json:"kind,omitempty"`
}

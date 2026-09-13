package gateway

import (
	"bytes"
	"encoding/json"
)

type anthropicMessagesUsage struct {
	InputTokens              *int64 `json:"input_tokens"`
	OutputTokens             *int64 `json:"output_tokens"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
}

type anthropicEnvelopeSSELine struct {
	Type    string                  `json:"type"`
	Message *anthropicMessageStart  `json:"message"`
	Usage   *anthropicMessagesUsage `json:"usage"`
}

type anthropicMessageStart struct {
	ID    string                  `json:"id"`
	Usage *anthropicMessagesUsage `json:"usage"`
}

type anthropicStreamState struct {
	snapshot  anthropicMessagesUsage
	started   bool
	stopped   bool
	messageID string
}

func newAnthropicStreamState() *anthropicStreamState {
	return &anthropicStreamState{}
}

func (AnthropicMessagesObserver) ParseResponse(body []byte) *ResponseMetadata {
	if len(body) == 0 {
		return nil
	}
	var env struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.ID == "" {
		return nil
	}
	return &ResponseMetadata{ResponseObjectID: env.ID}
}

func (AnthropicMessagesObserver) ParseResponseUsage(body []byte) *ProviderUsage {
	if len(body) == 0 {
		return nil
	}
	var env struct {
		Usage *anthropicMessagesUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Usage == nil {
		return nil
	}
	return anthropicBuildProviderUsage(env.Usage)
}

func (AnthropicMessagesObserver) NewStreamState() any {
	return newAnthropicStreamState()
}

func (AnthropicMessagesObserver) MaxStreamEventBytes() int {
	return maxSSEResponseEventBytes
}

func (AnthropicMessagesObserver) ParseStreamFrame(state any, payload []byte) StreamFrameObservation {
	st, _ := state.(*anthropicStreamState)
	if st == nil {
		st = newAnthropicStreamState()
	}
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return StreamFrameObservation{ResponseObjectID: st.messageID, Terminal: st.stopped}
	}
	var env anthropicEnvelopeSSELine
	if err := json.Unmarshal(payload, &env); err != nil {
		// A malformed frame must not clear the message ID captured earlier.
		return StreamFrameObservation{ResponseObjectID: st.messageID, Terminal: st.stopped}
	}
	switch env.Type {
	case "message_start":
		st.started = true
		if env.Message != nil {
			if env.Message.ID != "" {
				st.messageID = env.Message.ID
			}
			if env.Message.Usage != nil {
				st.snapshot = mergeAnthropicUsage(st.snapshot, env.Message.Usage)
			}
		}
		if env.Usage != nil {
			st.snapshot = mergeAnthropicUsage(st.snapshot, env.Usage)
		}
	case "message_delta":
		if env.Usage != nil {
			st.snapshot = mergeAnthropicUsage(st.snapshot, env.Usage)
		}
	case "message_stop":
		st.stopped = true
	default:
		return StreamFrameObservation{ResponseObjectID: st.messageID, Terminal: st.stopped}
	}
	var usage *ProviderUsage
	if st.snapshot.InputTokens != nil || st.snapshot.OutputTokens != nil ||
		st.snapshot.CacheCreationInputTokens != nil || st.snapshot.CacheReadInputTokens != nil {
		usage = anthropicBuildProviderUsage(&st.snapshot)
	}
	return StreamFrameObservation{
		Usage:            usage,
		ResponseObjectID: st.messageID,
		Terminal:         st.stopped,
	}
}

func mergeAnthropicUsage(existing anthropicMessagesUsage, next *anthropicMessagesUsage) anthropicMessagesUsage {
	out := existing
	if next.InputTokens != nil {
		v := *next.InputTokens
		out.InputTokens = &v
	}
	if next.OutputTokens != nil {
		v := *next.OutputTokens
		out.OutputTokens = &v
	}
	if next.CacheCreationInputTokens != nil {
		v := *next.CacheCreationInputTokens
		out.CacheCreationInputTokens = &v
	}
	if next.CacheReadInputTokens != nil {
		v := *next.CacheReadInputTokens
		out.CacheReadInputTokens = &v
	}
	return out
}

func anthropicBuildProviderUsage(u *anthropicMessagesUsage) *ProviderUsage {
	if u == nil {
		return nil
	}
	out := &ProviderUsage{Source: "anthropic_messages"}
	if u.InputTokens != nil {
		v := *u.InputTokens
		out.InputTokens = &v
	}
	if u.OutputTokens != nil {
		v := *u.OutputTokens
		out.OutputTokens = &v
	}
	if u.CacheCreationInputTokens != nil {
		v := *u.CacheCreationInputTokens
		out.CacheCreationInputTokens = &v
	}
	if u.CacheReadInputTokens != nil {
		v := *u.CacheReadInputTokens
		out.CacheReadInputTokens = &v
	}
	if !out.HasUsage() {
		return nil
	}
	return out
}

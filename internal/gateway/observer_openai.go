package gateway

import (
	"bytes"
	"encoding/json"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

type OpenAIResponsesObserver struct{}

func (OpenAIResponsesObserver) Protocol() Protocol { return ProtocolOpenAIResponses }

func (OpenAIResponsesObserver) ParseMetadata(body []byte) ExchangeRequestMetadata {
	var req struct {
		PreviousResponseID string            `json:"previous_response_id"`
		Input              []json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ExchangeRequestMetadata{}
	}
	meta := OpenAIResponsesMetadata{}
	if req.PreviousResponseID != "" {
		meta.PreviousResponseID = req.PreviousResponseID
	}
	for index, raw := range req.Input {
		item, ok := openAIResponsesItem(index, raw)
		if !ok {
			continue
		}
		meta.Items = append(meta.Items, item)
		if item.CallID != "" && item.Type == "function_call" {
			meta.FunctionCallIDs = append(meta.FunctionCallIDs, item.CallID)
		}
		if item.OutputCallID != "" && item.Type == "function_call_output" {
			meta.FunctionCallOutputIDs = append(meta.FunctionCallOutputIDs, item.OutputCallID)
		}
	}
	if meta.PreviousResponseID == "" && len(meta.Items) == 0 {
		return ExchangeRequestMetadata{}
	}
	return ExchangeRequestMetadata{OpenAIResponses: &meta}
}

func openAIResponsesItem(index int, raw json.RawMessage) (OpenAIResponsesItemMetadata, bool) {
	if len(raw) == 0 {
		return OpenAIResponsesItemMetadata{}, false
	}
	var typed struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		CallID  string `json:"call_id"`
		CallID2 string `json:"callId"`
	}
	if err := json.Unmarshal(raw, &typed); err != nil {
		return OpenAIResponsesItemMetadata{}, false
	}
	item := OpenAIResponsesItemMetadata{Index: index, Type: typed.Type, ItemID: typed.ID}
	switch typed.Type {
	case "function_call":
		if typed.CallID != "" {
			item.CallID = typed.CallID
		} else {
			item.CallID = typed.CallID2
		}
	case "function_call_output":
		if typed.CallID != "" {
			item.OutputCallID = typed.CallID
		} else {
			item.OutputCallID = typed.CallID2
		}
	}
	return item, true
}

func (OpenAIResponsesObserver) Parse(body []byte) []model.ContextComponent {
	var req struct {
		Model        string            `json:"model"`
		Instructions json.RawMessage   `json:"instructions"`
		Input        []json.RawMessage `json:"input"`
		Tools        []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil
	}
	out := make([]model.ContextComponent, 0, 4)
	if text := openAIText(req.Instructions); text != "" {
		out = append(out, ComponentFor(model.ContextInstructions, len(out), text, ""))
	}
	toolNames := openAIFunctionCallNames(req.Input)
	for _, item := range req.Input {
		details, ok := openAIInputItem(item, toolNames)
		if !ok {
			continue
		}
		component := ComponentFor(details.Kind, len(out), details.Text, "")
		component.ToolCallID = details.ToolCallID
		component.ToolName = details.ToolName
		component.ContentBytes = details.ContentBytes
		component.Completeness = details.Completeness
		out = append(out, component)
	}
	if len(req.Tools) > 0 {
		raw, _ := json.Marshal(req.Tools)
		out = append(out, ComponentTools(len(out), raw))
	}
	return dedupeComponents(out)
}

// openAIFunctionCallNames maps a call ID to the function name declared by the
// function_call item that carries it, so the matching output can report the
// tool name without inferring it from the output shape.
func openAIFunctionCallNames(input []json.RawMessage) map[string]string {
	names := map[string]string{}
	for _, raw := range input {
		var call struct {
			Type   string `json:"type"`
			Name   string `json:"name"`
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(raw, &call); err != nil {
			continue
		}
		if call.Type == "function_call" && call.CallID != "" && call.Name != "" {
			names[call.CallID] = call.Name
		}
	}
	return names
}

func openAIText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	out := ""
	for _, b := range blocks {
		out += b.Content
	}
	return out
}

// openAIItemDetails is the canonical reading of one input item: what kind of
// context it is plus, for a function call output, the call it answers.
type openAIItemDetails struct {
	Kind         model.ContextComponentKind
	Text         string
	ToolCallID   string
	ToolName     string
	ContentBytes *int64
	Completeness model.ContextCompleteness
}

func openAIInputItem(raw json.RawMessage, toolNames map[string]string) (openAIItemDetails, bool) {
	if len(raw) == 0 {
		return openAIItemDetails{}, false
	}
	var typed struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		CallID  string          `json:"call_id"`
		CallID2 string          `json:"callId"`
		Content json.RawMessage `json:"content"`
		Output  json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(raw, &typed); err != nil {
		return openAIItemDetails{}, false
	}
	switch typed.Type {
	case "function_call_output":
		callID := firstNonEmpty(typed.CallID, typed.CallID2)
		return openAIItemDetails{
			Kind:         model.ContextToolResult,
			Text:         source.ToolOutputText(typed.Output),
			ToolCallID:   callID,
			ToolName:     toolNames[callID],
			ContentBytes: source.ToolOutputBytes(typed.Output),
			Completeness: model.ContextCompletenessComplete,
		}, true
	case "message":
		switch typed.Role {
		case "user":
			return openAIItemDetails{Kind: model.ContextUserPrompt, Text: openAIText(typed.Content)}, true
		case "system", "developer":
			return openAIItemDetails{Kind: model.ContextInstructions, Text: openAIText(typed.Content)}, true
		case "assistant":
			return openAIItemDetails{Kind: model.ContextHistory, Text: openAIText(typed.Content)}, true
		}
	}
	switch typed.Role {
	case "user":
		return openAIItemDetails{Kind: model.ContextUserPrompt, Text: openAIText(typed.Content)}, true
	case "assistant":
		return openAIItemDetails{Kind: model.ContextHistory, Text: openAIText(typed.Content)}, true
	case "system", "developer":
		return openAIItemDetails{Kind: model.ContextInstructions, Text: openAIText(typed.Content)}, true
	}
	return openAIItemDetails{}, false
}

func (OpenAIResponsesObserver) ParseResponse(body []byte) *ResponseMetadata {
	if len(body) == 0 {
		return nil
	}
	var resp struct {
		ID    string            `json:"id"`
		Items []json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	out := &ResponseMetadata{}
	if resp.ID != "" {
		out.ResponseObjectID = resp.ID
	}
	meta := &OpenAIResponsesResponseMeta{}
	if resp.ID != "" {
		meta.ResponseID = resp.ID
	}
	for _, raw := range resp.Items {
		var item struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if item.Type == "function_call" && item.CallID != "" {
			meta.OutputItemCallIDs = append(meta.OutputItemCallIDs, item.CallID)
		}
	}
	if meta.ResponseID != "" || len(meta.OutputItemCallIDs) > 0 {
		out.OpenAIResponses = meta
	}
	if out.ResponseObjectID == "" && out.OpenAIResponses == nil {
		return nil
	}
	return out
}

func (OpenAIResponsesObserver) ParseResponseUsage(body []byte) *ProviderUsage {
	if len(body) == 0 {
		return nil
	}
	var resp struct {
		Usage *openAIResponsesUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	return buildOpenAIProviderUsage(resp.Usage)
}

type openAIResponsesUsage struct {
	InputTokens         *int64                        `json:"input_tokens"`
	OutputTokens        *int64                        `json:"output_tokens"`
	TotalTokens         *int64                        `json:"total_tokens"`
	InputTokensDetails  *openAIResponsesInputDetails  `json:"input_tokens_details"`
	OutputTokensDetails *openAIResponsesOutputDetails `json:"output_tokens_details"`
}

type openAIResponsesInputDetails struct {
	CachedTokens *int64 `json:"cached_tokens"`
}

type openAIResponsesOutputDetails struct {
	ReasoningTokens *int64 `json:"reasoning_tokens"`
}

func buildOpenAIProviderUsage(raw *openAIResponsesUsage) *ProviderUsage {
	if raw == nil {
		return nil
	}
	out := &ProviderUsage{Source: "openai_responses"}
	if raw.InputTokens != nil {
		v := *raw.InputTokens
		out.InputTokens = &v
	}
	if raw.InputTokensDetails != nil && raw.InputTokensDetails.CachedTokens != nil {
		v := *raw.InputTokensDetails.CachedTokens
		out.CacheReadInputTokens = &v
	}
	if raw.OutputTokens != nil {
		v := *raw.OutputTokens
		out.OutputTokens = &v
	}
	if raw.OutputTokensDetails != nil && raw.OutputTokensDetails.ReasoningTokens != nil {
		v := *raw.OutputTokensDetails.ReasoningTokens
		out.ReasoningOutputTokens = &v
	}
	if raw.TotalTokens != nil {
		v := *raw.TotalTokens
		out.TotalTokens = &v
	}
	if !out.HasUsage() {
		return nil
	}
	return out
}

func (OpenAIResponsesObserver) NewStreamState() any {
	return newOpenAIStreamState()
}

func (OpenAIResponsesObserver) MaxStreamEventBytes() int {
	return maxSSEResponseEventBytes
}

type openAIStreamState struct {
	snapshot   openAIResponsesUsage
	terminal   bool
	responseID string
}

func newOpenAIStreamState() *openAIStreamState { return &openAIStreamState{} }

func (OpenAIResponsesObserver) ParseStreamFrame(state any, payload []byte) StreamFrameObservation {
	st, _ := state.(*openAIStreamState)
	if st == nil {
		st = newOpenAIStreamState()
	}
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return StreamFrameObservation{ResponseObjectID: st.responseID, Terminal: st.terminal}
	}
	var env struct {
		Type     string `json:"type"`
		Response *struct {
			ID string `json:"id"`
		} `json:"response"`
		Usage *openAIResponsesUsage `json:"usage"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return StreamFrameObservation{ResponseObjectID: st.responseID, Terminal: st.terminal}
	}
	switch env.Type {
	case "response.completed", "response.done", "response.usage":
		st.terminal = true
	}
	if env.Response != nil && env.Response.ID != "" {
		st.responseID = env.Response.ID
	}
	if env.Usage != nil {
		st.snapshot = mergeOpenAIUsage(st.snapshot, env.Usage)
	}
	var usage *ProviderUsage
	if buildOpenAIProviderUsage(&st.snapshot) != nil {
		usage = buildOpenAIProviderUsage(&st.snapshot)
	}
	return StreamFrameObservation{
		Usage:            usage,
		ResponseObjectID: st.responseID,
		Terminal:         st.terminal,
	}
}

func mergeOpenAIUsage(existing openAIResponsesUsage, next *openAIResponsesUsage) openAIResponsesUsage {
	out := existing
	if next.InputTokens != nil {
		v := *next.InputTokens
		out.InputTokens = &v
	}
	if next.OutputTokens != nil {
		v := *next.OutputTokens
		out.OutputTokens = &v
	}
	if next.TotalTokens != nil {
		v := *next.TotalTokens
		out.TotalTokens = &v
	}
	if next.InputTokensDetails != nil {
		det := openAIResponsesInputDetails{}
		if next.InputTokensDetails.CachedTokens != nil {
			v := *next.InputTokensDetails.CachedTokens
			det.CachedTokens = &v
		}
		out.InputTokensDetails = &det
	}
	if next.OutputTokensDetails != nil {
		det := openAIResponsesOutputDetails{}
		if next.OutputTokensDetails.ReasoningTokens != nil {
			v := *next.OutputTokensDetails.ReasoningTokens
			det.ReasoningTokens = &v
		}
		out.OutputTokensDetails = &det
	}
	return out
}

const maxSSEResponseEventBytes = 1 * 1024 * 1024

func splitSSELines(event []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range event {
		if b == '\n' {
			out = append(out, event[start:i])
			start = i + 1
		}
	}
	if start < len(event) {
		out = append(out, event[start:])
	}
	return out
}

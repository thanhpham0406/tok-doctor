package gateway

import (
	"bytes"
	"encoding/json"

	"github.com/thanhpham0406/tok-doctor/internal/model"
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
	for _, item := range req.Input {
		kind, text := openAIInputItem(item)
		if kind == "" {
			continue
		}
		if kind == model.ContextFile {
			out = append(out, ComponentFor(kind, len(out), text, openAIFilePath(item)))
			continue
		}
		out = append(out, ComponentFor(kind, len(out), text, ""))
	}
	if len(req.Tools) > 0 {
		raw, _ := json.Marshal(req.Tools)
		out = append(out, ComponentTools(len(out), raw))
	}
	return dedupeComponents(out)
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

func openAIInputItem(raw json.RawMessage) (model.ContextComponentKind, string) {
	if len(raw) == 0 {
		return "", ""
	}
	var typed struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &typed); err != nil {
		return "", ""
	}
	switch typed.Type {
	case "function_call_output":
		var s struct {
			Output string `json:"output"`
		}
		_ = json.Unmarshal(raw, &s)
		return model.ContextToolResult, s.Output
	case "message":
		switch typed.Role {
		case "user":
			return model.ContextUserPrompt, openAIText(typed.Content)
		case "assistant", "system", "developer":
			if typed.Role == "system" || typed.Role == "developer" {
				return model.ContextInstructions, openAIText(typed.Content)
			}
			return model.ContextHistory, openAIText(typed.Content)
		}
	}
	switch typed.Role {
	case "user":
		return model.ContextUserPrompt, openAIText(typed.Content)
	case "assistant":
		return model.ContextHistory, openAIText(typed.Content)
	case "system", "developer":
		return model.ContextInstructions, openAIText(typed.Content)
	}
	return "", ""
}

func openAIFilePath(raw json.RawMessage) string {
	var s struct {
		File string `json:"file"`
		Path string `json:"path"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &s)
	if s.File != "" {
		return s.File
	}
	if s.Path != "" {
		return s.Path
	}
	return s.Name
}

func (OpenAIResponsesObserver) ParseResponse(body []byte) *OpenAIResponsesResponseMeta {
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
	if meta.ResponseID == "" && len(meta.OutputItemCallIDs) == 0 {
		return nil
	}
	return meta
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
	snapshot openAIResponsesUsage
	terminal bool
}

func newOpenAIStreamState() *openAIStreamState { return &openAIStreamState{} }

func (OpenAIResponsesObserver) ParseStreamFrame(state any, payload []byte) (*ProviderUsage, bool) {
	st, _ := state.(*openAIStreamState)
	if st == nil {
		st = newOpenAIStreamState()
	}
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return nil, st.terminal
	}
	var env struct {
		Type  string                `json:"type"`
		Usage *openAIResponsesUsage `json:"usage"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, st.terminal
	}
	switch env.Type {
	case "response.completed", "response.done", "response.usage":
		st.terminal = true
	}
	if env.Usage != nil {
		st.snapshot = mergeOpenAIUsage(st.snapshot, env.Usage)
	}
	if pu := buildOpenAIProviderUsage(&st.snapshot); pu != nil {
		return pu, st.terminal
	}
	return nil, st.terminal
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

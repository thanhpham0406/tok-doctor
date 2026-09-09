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
	return buildProviderUsage(resp.Usage)
}

type openAIResponsesUsage struct {
	InputTokens         int64                         `json:"input_tokens"`
	OutputTokens        int64                         `json:"output_tokens"`
	TotalTokens         int64                         `json:"total_tokens"`
	InputTokensDetails  *openAIResponsesInputDetails  `json:"input_tokens_details"`
	OutputTokensDetails *openAIResponsesOutputDetails `json:"output_tokens_details"`
}

type openAIResponsesInputDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type openAIResponsesOutputDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

func buildProviderUsage(raw *openAIResponsesUsage) *ProviderUsage {
	if raw == nil {
		return nil
	}
	out := &ProviderUsage{Source: "openai_responses"}
	if raw.InputTokens > 0 {
		v := raw.InputTokens
		out.Input = &v
	}
	if raw.InputTokensDetails != nil {
		v := raw.InputTokensDetails.CachedTokens
		out.CachedInput = &v
	}
	if raw.OutputTokens > 0 {
		v := raw.OutputTokens
		out.Output = &v
	}
	if raw.OutputTokensDetails != nil {
		v := raw.OutputTokensDetails.ReasoningTokens
		out.ReasoningOutput = &v
	}
	if out.Input == nil && out.CachedInput == nil && out.Output == nil && out.ReasoningOutput == nil {
		return nil
	}
	return out
}

func (OpenAIResponsesObserver) ParseStreamEvent(event []byte) *ProviderUsage {
	if len(event) == 0 {
		return nil
	}
	for _, line := range splitSSELines(event) {
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[5:])
		if len(payload) == 0 {
			continue
		}
		if bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var env struct {
			Type  string                `json:"type"`
			Usage *openAIResponsesUsage `json:"usage"`
		}
		if err := json.Unmarshal(payload, &env); err != nil {
			continue
		}
		if env.Type != "response.completed" && env.Type != "response.done" && env.Type != "response.usage" {
			if env.Usage == nil {
				continue
			}
		}
		if usage := buildProviderUsage(env.Usage); usage != nil {
			return usage
		}
	}
	return nil
}

func (OpenAIResponsesObserver) MaxStreamEventBytes() int {
	return maxSSEResponseEventBytes
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

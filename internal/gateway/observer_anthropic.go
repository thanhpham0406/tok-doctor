package gateway

import (
	"encoding/json"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type AnthropicMessagesObserver struct{}

func (AnthropicMessagesObserver) Protocol() Protocol { return ProtocolAnthropicMessages }

func (AnthropicMessagesObserver) Parse(body []byte) []model.ContextComponent {
	var req struct {
		Model    string          `json:"model"`
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil
	}
	out := make([]model.ContextComponent, 0, 4)
	if text := anthropicText(req.System); text != "" {
		out = append(out, ComponentFor(model.ContextInstructions, len(out), text, ""))
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			for _, part := range anthropicContentParts(m.Content) {
				out = append(out, ComponentFor(model.ContextUserPrompt, len(out), part, ""))
			}
		case "assistant":
			out = append(out, ComponentFor(model.ContextHistory, len(out), anthropicText(m.Content), ""))
		}
	}
	if len(req.Tools) > 0 {
		raw, _ := json.Marshal(req.Tools)
		out = append(out, ComponentTools(len(out), raw))
	}
	return dedupeComponents(out)
}

func anthropicText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	out := ""
	for _, b := range blocks {
		out += b.Text
	}
	return out
}

func anthropicContentParts(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []string{s}
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "text" || b.Type == "" {
			out = append(out, b.Text)
		}
	}
	return out
}

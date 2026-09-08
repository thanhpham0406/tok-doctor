package gateway

import (
	"encoding/json"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type OpenAIResponsesObserver struct{}

func (OpenAIResponsesObserver) Protocol() Protocol { return ProtocolOpenAIResponses }

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

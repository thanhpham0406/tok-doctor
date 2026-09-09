package gateway

import (
	"encoding/json"
	"fmt"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type AnthropicMessagesObserver struct{}

func (AnthropicMessagesObserver) Protocol() Protocol { return ProtocolAnthropicMessages }

func (AnthropicMessagesObserver) ParseMetadata(body []byte) ExchangeRequestMetadata {
	var req struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Messages) == 0 {
		return ExchangeRequestMetadata{}
	}
	messages := make([]AnthropicMessageMetadata, 0, len(req.Messages))
	for messageIndex, message := range req.Messages {
		meta := AnthropicMessageMetadata{Index: messageIndex, Role: message.Role}
		meta.Blocks = anthropicBlockMetadata(message.Content)
		messages = append(messages, meta)
	}
	return ExchangeRequestMetadata{
		AnthropicMessages: &AnthropicMessagesMetadata{Messages: messages},
	}
}

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
		out = append(out, anthropicComponentFor(model.ContextInstructions, 0, text, "", "system", "system", ""))
	}
	latestUser := -1
	for i, m := range req.Messages {
		if m.Role == "user" {
			latestUser = i
		}
	}
	for i, m := range req.Messages {
		switch m.Role {
		case "user":
			textKind := model.ContextHistory
			if i == latestUser {
				textKind = model.ContextOther
			}
			for _, part := range anthropicContentParts(i, m.Role, m.Content, textKind) {
				out = append(out, componentFromAnthropicPart(part))
			}
		case "assistant":
			for _, part := range anthropicContentParts(i, m.Role, m.Content, model.ContextHistory) {
				out = append(out, componentFromAnthropicPart(part))
			}
		}
	}
	if len(req.Tools) > 0 {
		raw, _ := json.Marshal(req.Tools)
		c := ComponentTools(anthropicToolsPosition(len(req.Messages)), raw)
		c.Source = string(ProtocolAnthropicMessages)
		c.Record = "tools"
		c.Evidence = []model.Evidence{anthropicEvidence("tools", "", "tools")}
		out = append(out, c)
	}
	return dedupeComponents(out)
}

func anthropicBlockMetadata(raw json.RawMessage) []AnthropicBlockMetadata {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []AnthropicBlockMetadata{{Index: 0, Type: "text"}}
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	out := make([]AnthropicBlockMetadata, 0, len(blocks))
	for blockIndex, block := range blocks {
		var b struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			ToolUseID string `json:"tool_use_id"`
		}
		if err := json.Unmarshal(block, &b); err != nil {
			continue
		}
		blockType := b.Type
		if blockType == "" {
			blockType = "text"
		}
		meta := AnthropicBlockMetadata{Index: blockIndex, Type: blockType}
		if blockType == "tool_use" {
			meta.ToolUseID = b.ID
		}
		if blockType == "tool_result" {
			meta.ToolResultToolUseID = b.ToolUseID
		}
		out = append(out, meta)
	}
	return out
}

type anthropicContentPart struct {
	Kind     model.ContextComponentKind
	Text     string
	Path     string
	Position int
	Role     string
	Record   string
	Type     string
}

func componentFromAnthropicPart(part anthropicContentPart) model.ContextComponent {
	if part.Text == "" {
		c := model.ContextComponent{
			Kind:        part.Kind,
			Position:    part.Position,
			Source:      string(ProtocolAnthropicMessages),
			Record:      part.Record,
			Path:        part.Path,
			Observation: ObservationScope,
			Measurement: model.Measurement{Kind: model.MeasurementUnknown},
		}
		c.Evidence = []model.Evidence{anthropicEvidence(part.Record, part.Role, part.Type)}
		return c
	}
	return anthropicComponentFor(part.Kind, part.Position, part.Text, part.Path, part.Record, part.Role, part.Type)
}

func anthropicComponentFor(kind model.ContextComponentKind, position int, text string, path string, record string, role string, blockType string) model.ContextComponent {
	c := ComponentFor(kind, position, text, path)
	c.Source = string(ProtocolAnthropicMessages)
	c.Record = record
	c.Evidence = []model.Evidence{anthropicEvidence(record, role, blockType)}
	return c
}

func anthropicEvidence(record string, role string, blockType string) model.Evidence {
	field := ""
	if role != "" && blockType != "" {
		field = fmt.Sprintf("role=%s,type=%s", role, blockType)
	} else if role != "" {
		field = "role=" + role
	} else if blockType != "" {
		field = "type=" + blockType
	}
	return model.Evidence{Kind: model.EvidenceProvenance, Source: string(ProtocolAnthropicMessages), Record: record, Field: field}
}

func anthropicText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var block struct {
		Text    string          `json:"text"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &block); err == nil {
		if block.Text != "" {
			return block.Text
		}
		if len(block.Content) > 0 {
			return anthropicText(block.Content)
		}
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err == nil {
		out := ""
		for _, part := range parts {
			out += anthropicText(part)
		}
		return out
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

func anthropicContentParts(messageIndex int, role string, raw json.RawMessage, textKind model.ContextComponentKind) []anthropicContentPart {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []anthropicContentPart{{
			Kind:     textKind,
			Text:     s,
			Position: anthropicMessagePosition(messageIndex, 0),
			Role:     role,
			Record:   fmt.Sprintf("messages[%d].content", messageIndex),
			Type:     "text",
		}}
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	out := make([]anthropicContentPart, 0, len(blocks))
	for blockIndex, block := range blocks {
		var b struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Content  json.RawMessage `json:"content"`
			Path     string          `json:"path"`
			File     string          `json:"file"`
			Name     string          `json:"name"`
			Filename string          `json:"filename"`
		}
		if err := json.Unmarshal(block, &b); err != nil {
			continue
		}
		blockType := b.Type
		if blockType == "" {
			blockType = "text"
		}
		position := anthropicMessagePosition(messageIndex, blockIndex)
		record := fmt.Sprintf("messages[%d].content[%d]", messageIndex, blockIndex)
		switch b.Type {
		case "text", "":
			path := anthropicBlockPath(b.Path, b.File, b.Filename, b.Name)
			kind := textKind
			if path != "" {
				kind = model.ContextFile
			}
			out = append(out, anthropicContentPart{Kind: kind, Text: b.Text, Path: path, Position: position, Role: role, Record: record, Type: blockType})
		case "tool_result":
			out = append(out, anthropicContentPart{Kind: model.ContextToolResult, Text: anthropicText(b.Content), Position: position, Role: role, Record: record, Type: blockType})
		case "tool_use":
			text := ""
			if raw, err := json.Marshal(json.RawMessage(block)); err == nil {
				text = string(raw)
			}
			out = append(out, anthropicContentPart{Kind: model.ContextHistory, Text: text, Position: position, Role: role, Record: record, Type: blockType})
		default:
			path := anthropicBlockPath(b.Path, b.File, b.Filename, b.Name)
			text := b.Text
			if text == "" {
				text = anthropicText(b.Content)
			}
			kind := model.ContextOther
			if path != "" && text != "" {
				kind = model.ContextFile
			}
			out = append(out, anthropicContentPart{Kind: kind, Text: text, Path: path, Position: position, Role: role, Record: record, Type: blockType})
		}
	}
	return out
}

func anthropicMessagePosition(messageIndex int, blockIndex int) int {
	return 1000 + messageIndex*1000 + blockIndex
}

func anthropicToolsPosition(messageCount int) int {
	return 1000 + messageCount*1000
}

func anthropicBlockPath(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

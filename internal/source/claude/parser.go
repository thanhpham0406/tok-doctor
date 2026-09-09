package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

type assistantUsage struct {
	UUID       string `json:"uuid"`
	ParentUUID string `json:"parentUuid"`
	Timestamp  string `json:"timestamp"`
	Type       string `json:"type"`
	Message    struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *usageFields    `json:"usage"`
	} `json:"message"`
}

type usageFields struct {
	InputTokens              *int64 `json:"input_tokens"`
	OutputTokens             *int64 `json:"output_tokens"`
	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
}

func (u *usageFields) snapshot() UsageSnapshot {
	if u == nil {
		return UsageSnapshot{}
	}
	var snap UsageSnapshot
	populated := false
	if u.InputTokens != nil {
		snap.Input += *u.InputTokens
		populated = true
	}
	if u.OutputTokens != nil {
		snap.Output += *u.OutputTokens
		populated = true
	}
	if u.CacheReadInputTokens != nil {
		snap.CacheRead += *u.CacheReadInputTokens
		snap.Cached += *u.CacheReadInputTokens
		populated = true
	}
	if u.CacheCreationInputTokens != nil {
		snap.CacheWrite += *u.CacheCreationInputTokens
		snap.Cached += *u.CacheCreationInputTokens
		populated = true
	}
	if !populated {
		return snap
	}
	snap.HasUsage = true
	snap.Total = snap.Input + snap.Output + snap.Cached
	return snap
}

type invocation struct {
	ID        string
	Model     string
	Snapshot  UsageSnapshot
	Timestamp string
	Conflict  bool
	Context   []model.ContextComponent
}

type ParsedSession struct {
	Model       string
	StartedAt   string
	UpdatedAt   string
	Usage       UsageSnapshot
	modelNames  map[string]struct{}
	Invocations []invocation
	duplicates  int
	conflicts   int
}

func ParseSessionUsage(path string) (UsageSnapshot, error) {
	session, err := ParseSession(path)
	if err != nil {
		return UsageSnapshot{}, err
	}
	return session.Usage, nil
}

func ParseSession(path string) (ParsedSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return ParsedSession{}, fmt.Errorf("open claude session %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	session, err := parseSession(f)
	if err != nil {
		return ParsedSession{}, fmt.Errorf("read claude session %s: %w", path, err)
	}
	return session, nil
}

func parseSession(r io.Reader) (ParsedSession, error) {
	scanner := bufio.NewScanner(r)
	// Raise the default 64 KiB scanner ceiling
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var session ParsedSession
	// Claude Code records carry provider-issued message.id on every
	// assistant usage block; treat it as the deduplication key because
	// the local transcript can re-emit the same model response.
	seenByMessageID := map[string]invocation{}
	var pending []model.ContextComponent
	historyCount := 0

	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var entry assistantUsage
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		applyTimestamp(&session, entry.Timestamp)
		recordID := claudeRecordID(entry, len(session.Invocations)+len(pending)+historyCount+1)
		if entry.Type != "assistant" || entry.Message.Role != "assistant" {
			pending = appendClaudeMessageContext(pending, entry, recordID, historyCount)
			if entry.Message.Role != "" {
				historyCount++
			}
			continue
		}
		if isRealModel(entry.Message.Model) {
			if session.modelNames == nil {
				session.modelNames = map[string]struct{}{}
			}
			session.modelNames[entry.Message.Model] = struct{}{}
			session.Model = selectedModel(session.modelNames)
		}
		if isSynthetic(entry.Message.Model) {
			continue
		}
		invocationContext := dedupeContext(pending)
		usage := entry.Message.Usage.snapshot()
		if !usage.HasUsage {
			pending = appendClaudeAssistantHistory(pending, entry, recordID, historyCount)
			historyCount++
			continue
		}

		ident := entry.Message.ID
		if ident == "" {
			session.Invocations = append(session.Invocations, invocation{
				Model:     entry.Message.Model,
				Snapshot:  usage,
				Timestamp: entry.Timestamp,
				Context:   invocationContext,
			})
			session.Usage = session.Usage.add(usage)
			pending = appendClaudeAssistantHistory(nil, entry, recordID, historyCount)
			historyCount++
			continue
		}

		prev, seen := seenByMessageID[ident]
		switch {
		case !seen:
			inv := invocation{
				ID:        ident,
				Model:     entry.Message.Model,
				Snapshot:  usage,
				Timestamp: entry.Timestamp,
				Context:   invocationContext,
			}
			seenByMessageID[ident] = inv
			session.Invocations = append(session.Invocations, inv)
			session.Usage = session.Usage.add(usage)
			pending = appendClaudeAssistantHistory(nil, entry, recordID, historyCount)
			historyCount++
		case !snapshotsEqual(prev.Snapshot, usage):
			for i := range session.Invocations {
				if session.Invocations[i].ID == ident {
					session.Invocations[i].Conflict = true
					break
				}
			}
			session.conflicts++
		default:
			session.duplicates++
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedSession{}, err
	}
	return session, nil
}

func appendClaudeMessageContext(components []model.ContextComponent, entry assistantUsage, recordID string, historyCount int) []model.ContextComponent {
	switch entry.Message.Role {
	case "user":
		return append(components, claudeUserComponents(entry, recordID)...)
	case "assistant":
		return appendClaudeAssistantHistory(components, entry, recordID, historyCount)
	default:
		return components
	}
}

func claudeUserComponents(entry assistantUsage, recordID string) []model.ContextComponent {
	toolResults := claudeToolResultComponents(entry, recordID)
	if len(toolResults) > 0 {
		return toolResults
	}
	text := claudeTextContent(entry.Message.Content)
	return []model.ContextComponent{{
		Kind:        model.ContextUserPrompt,
		Source:      "claude_session",
		Record:      recordID,
		ContentHash: source.ContentHash(text),
		Observation: model.ContextObservedByAgent,
		Measurement: source.EstimatedTextMeasurement(text),
		Evidence:    []model.Evidence{claudeProvenance(recordID, "message.content")},
	}}
}

func claudeToolResultComponents(entry assistantUsage, recordID string) []model.ContextComponent {
	var blocks []struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
		return nil
	}
	var out []model.ContextComponent
	for i, block := range blocks {
		if block.Type != "tool_result" {
			continue
		}
		text := claudeTextContent(block.Content)
		component := model.ContextComponent{
			Kind:        model.ContextToolResult,
			Source:      "claude_session",
			Record:      recordID + "#tool_result:" + fmt.Sprint(i+1),
			ContentHash: source.ContentHash(text),
			Observation: model.ContextObservedByAgent,
			Measurement: source.EstimatedTextMeasurement(text),
			Evidence:    []model.Evidence{claudeProvenance(recordID, "message.content.tool_result")},
		}
		out = append(out, component)
	}
	return out
}

func appendClaudeAssistantHistory(components []model.ContextComponent, entry assistantUsage, recordID string, historyCount int) []model.ContextComponent {
	if historyCount == 0 {
		return components
	}
	text := claudeTextContent(entry.Message.Content)
	return append(components, model.ContextComponent{
		Kind:        model.ContextHistory,
		Source:      "claude_session",
		Record:      recordID,
		ContentHash: source.ContentHash(text),
		Observation: model.ContextObservedByAgent,
		Measurement: source.EstimatedTextMeasurement(text),
		Evidence:    []model.Evidence{claudeProvenance(recordID, "message.content")},
	})
}

func claudeTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Text    string          `json:"text"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var b strings.Builder
	for _, block := range blocks {
		if block.Text != "" {
			b.WriteString(block.Text)
		} else if len(block.Content) > 0 {
			b.WriteString(claudeTextContent(block.Content))
		}
	}
	return b.String()
}

func claudeRecordID(entry assistantUsage, fallback int) string {
	if entry.UUID != "" {
		return entry.UUID
	}
	if entry.Message.ID != "" {
		return entry.Message.ID
	}
	return "claude_session#record:" + fmt.Sprint(fallback)
}

func claudeProvenance(recordID, field string) model.Evidence {
	return model.Evidence{Kind: model.EvidenceProvenance, Source: "claude_session", Record: recordID, Field: field}
}

func dedupeContext(components []model.ContextComponent) []model.ContextComponent {
	seen := map[string]struct{}{}
	out := make([]model.ContextComponent, 0, len(components))
	for _, component := range components {
		key := string(component.Kind) + "|" + component.Source + "|" + component.Record + "|" + component.Path
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, component)
	}
	return out
}

func snapshotsEqual(a, b UsageSnapshot) bool {
	return a.Input == b.Input && a.Cached == b.Cached && a.Output == b.Output && a.Total == b.Total
}

func isRealModel(model string) bool {
	return model != "" && !isSynthetic(model)
}

func isSynthetic(model string) bool {
	return model == "<synthetic>"
}

func selectedModel(models map[string]struct{}) string {
	if len(models) != 1 {
		return ""
	}
	for model := range models {
		return model
	}
	return ""
}

func applyTimestamp(session *ParsedSession, timestamp string) {
	if timestamp == "" {
		return
	}
	if session.StartedAt == "" {
		session.StartedAt = timestamp
	}
	session.UpdatedAt = timestamp
}

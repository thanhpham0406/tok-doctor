package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

const (
	claudeFieldRecord    = "record"
	claudeFieldOversized = "record.oversized"
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
	if u.InputTokens != nil {
		snap.Input = *u.InputTokens
		snap.InputPresent = true
		snap.HasUsage = true
		if *u.InputTokens >= 0 {
			snap.inputCount = 1
		}
	}
	if u.OutputTokens != nil {
		snap.Output = *u.OutputTokens
		snap.OutputPresent = true
		snap.HasUsage = true
		if *u.OutputTokens >= 0 {
			snap.outputCount = 1
		}
	}
	if u.CacheReadInputTokens != nil {
		snap.CacheRead = *u.CacheReadInputTokens
		snap.CacheReadPresent = true
		snap.HasUsage = true
		if *u.CacheReadInputTokens >= 0 {
			snap.cacheReadCount = 1
		}
	}
	if u.CacheCreationInputTokens != nil {
		snap.CacheWrite = *u.CacheCreationInputTokens
		snap.CacheWritePresent = true
		snap.HasUsage = true
		if *u.CacheCreationInputTokens >= 0 {
			snap.cacheWriteCount = 1
		}
	}
	if snap.HasUsage {
		snap.acceptedCount = 1
	}
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
	// TrailingContext holds records that follow the final usage-bearing
	// assistant message. They belong to no turn, so they are preserved rather
	// than attributed to the previous one.
	TrailingContext []model.ContextComponent
}

// toolUseIdentity is what a tool_use block declares about itself and what its
// result repeats: the name the source stated and a fingerprint of the input it
// stated, both empty when the source stated neither.
type toolUseIdentity struct {
	name        string
	fingerprint string
}

// contextBuilder keeps the tool_use declarations seen so far so a tool_result
// can carry the name and the argument fingerprint its matching tool_use stated.
// Neither is ever inferred.
type contextBuilder struct {
	toolUses map[string]toolUseIdentity
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
	reader := source.NewRecordReader(r)
	builder := &contextBuilder{toolUses: map[string]toolUseIdentity{}}

	var session ParsedSession
	// Claude Code records carry provider-issued message.id on every
	// assistant usage block; treat it as the deduplication key because
	// the local transcript can re-emit the same model response.
	seenByMessageID := map[string]invocation{}
	var pending []model.ContextComponent
	historyCount := 0

	for {
		raw, line, oversized, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ParsedSession{}, fmt.Errorf("line %d: %w", line+1, err)
		}
		if oversized {
			pending = append(pending, unreadableRecord(line, claudeFieldOversized, model.ContextCompletenessTruncated))
			continue
		}
		if len(raw) == 0 {
			continue
		}

		var entry assistantUsage
		if err := json.Unmarshal(raw, &entry); err != nil {
			pending = append(pending, unreadableRecord(line, claudeFieldRecord, model.ContextCompletenessUnavailable))
			continue
		}
		applyTimestamp(&session, entry.Timestamp)
		recordID := claudeRecordID(entry, len(session.Invocations)+len(pending)+historyCount+1)
		if entry.Message.Role == "assistant" {
			builder.registerToolUses(entry.Message.Content)
		}
		if entry.Type != "assistant" || entry.Message.Role != "assistant" {
			pending = builder.messageContext(pending, entry, recordID, historyCount)
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
	session.TrailingContext = dedupeContext(pending)
	session.Usage = acceptedUsage(session.Invocations)
	return session, nil
}

func acceptedUsage(invocations []invocation) UsageSnapshot {
	var accumulator usageAccumulator
	for _, inv := range invocations {
		if inv.Conflict {
			continue
		}
		accumulator.add(inv.Snapshot)
	}
	return accumulator.snapshot()
}

func unreadableRecord(line int, field string, completeness model.ContextCompleteness) model.ContextComponent {
	recordID := "claude_session#line:" + fmt.Sprint(line)
	return source.UnreadableRecordComponent("claude_session", recordID, field, completeness)
}

// registerToolUses records what every tool_use block declares so the matching
// tool_result can report it without guessing from the output shape.
func (c *contextBuilder) registerToolUses(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var blocks []struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return
	}
	for _, block := range blocks {
		if block.Type == "tool_use" && block.ID != "" && block.Name != "" {
			c.toolUses[block.ID] = toolUseIdentity{
				name:        block.Name,
				fingerprint: source.StructuredToolCallFingerprintOrEmpty(block.Name, block.Input),
			}
		}
	}
}

func (c *contextBuilder) messageContext(components []model.ContextComponent, entry assistantUsage, recordID string, historyCount int) []model.ContextComponent {
	switch entry.Message.Role {
	case "user":
		return append(components, c.userComponents(entry, recordID)...)
	case "assistant":
		return appendClaudeAssistantHistory(components, entry, recordID, historyCount)
	default:
		return components
	}
}

func (c *contextBuilder) userComponents(entry assistantUsage, recordID string) []model.ContextComponent {
	toolResults := c.toolResultComponents(entry, recordID)
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

func (c *contextBuilder) toolResultComponents(entry assistantUsage, recordID string) []model.ContextComponent {
	var blocks []struct {
		Type      string          `json:"type"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
		return nil
	}
	var out []model.ContextComponent
	for i, block := range blocks {
		if block.Type != "tool_result" {
			continue
		}
		text := source.ToolOutputText(block.Content)
		identity := c.toolUses[block.ToolUseID]
		component := model.ContextComponent{
			Kind:                model.ContextToolResult,
			Source:              "claude_session",
			Record:              recordID + "#tool_result:" + fmt.Sprint(i+1),
			ContentHash:         source.ContentHash(text),
			Observation:         model.ContextObservedByAgent,
			Measurement:         source.EstimatedTextMeasurement(text),
			Completeness:        model.ContextCompletenessComplete,
			ToolCallID:          block.ToolUseID,
			ToolName:            identity.name,
			ToolCallFingerprint: identity.fingerprint,
			ContentBytes:        source.ToolOutputBytes(block.Content),
			Evidence:            []model.Evidence{claudeProvenance(recordID, "message.content.tool_result")},
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
	return a.InputPresent == b.InputPresent && a.Input == b.Input &&
		a.CacheReadPresent == b.CacheReadPresent && a.CacheRead == b.CacheRead &&
		a.CacheWritePresent == b.CacheWritePresent && a.CacheWrite == b.CacheWrite &&
		a.OutputPresent == b.OutputPresent && a.Output == b.Output
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

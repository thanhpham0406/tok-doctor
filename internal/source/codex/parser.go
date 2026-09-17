package codex

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
	codexSourceName     = "codex_rollout"
	codexFieldRecord    = "record"
	codexFieldOversized = "record.oversized"
)

type usageInfo struct {
	InputTokens           int64  `json:"input_tokens"`
	CachedInputTokens     int64  `json:"cached_input_tokens"`
	OutputTokens          int64  `json:"output_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64  `json:"total_tokens"`
}

type tokenCountPayload struct {
	Type string `json:"type"`
	Info struct {
		TotalTokenUsage usageInfo `json:"total_token_usage"`
		LastTokenUsage  usageInfo `json:"last_token_usage"`
	} `json:"info"`
}

type sessionMetaPayload struct {
	ID string `json:"id"`
}

type turnContextPayload struct {
	Model string `json:"model"`
}

type eventMessagePayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type snapshot struct {
	Snap      UsageSnapshot
	Timestamp string
	Context   []model.ContextComponent
}

type ParsedSession struct {
	ID        string
	Model     string
	StartedAt string
	UpdatedAt string
	Usage     UsageSnapshot
	Snapshots []snapshot
	// TrailingContext holds records that follow the final token_count event.
	// They belong to no snapshot, so they are preserved without inventing a
	// turn or attributing usage the source never reported.
	TrailingContext []model.ContextComponent
}

type codexRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type responseItemPayload struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Name      string          `json:"name"`
	CallID    string          `json:"call_id"`
	Content   json.RawMessage `json:"content"`
	Arguments string          `json:"arguments"`
	Output    json.RawMessage `json:"output"`
}

// contextBuilder keeps the cross-record state needed to link a tool call, in
// either the function_call or the custom_tool_call shape, to the output record
// that carries its result.
type contextBuilder struct {
	callNames map[string]string
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
		return ParsedSession{}, fmt.Errorf("open codex session %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	session, err := parseSession(f)
	if err != nil {
		return ParsedSession{}, fmt.Errorf("read codex session %s: %w", path, err)
	}
	return session, nil
}

func parseSession(r io.Reader) (ParsedSession, error) {
	reader := source.NewRecordReader(r)
	builder := &contextBuilder{callNames: map[string]string{}}

	var session ParsedSession
	var pending []model.ContextComponent

	for {
		raw, line, oversized, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ParsedSession{}, fmt.Errorf("line %d: %w", line+1, err)
		}
		if oversized {
			pending = append(pending, unreadableRecord(line, codexFieldOversized, model.ContextCompletenessTruncated))
			continue
		}
		if len(raw) == 0 {
			continue
		}

		var record codexRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			pending = append(pending, unreadableRecord(line, codexFieldRecord, model.ContextCompletenessUnavailable))
			continue
		}
		applyTimestamp(&session, record.Timestamp)
		// A record without a payload carries nothing to read, which is not the
		// same as a payload this parser failed to read.
		if len(record.Payload) == 0 {
			continue
		}

		switch record.Type {
		case "session_meta":
			var meta sessionMetaPayload
			if err := json.Unmarshal(record.Payload, &meta); err != nil {
				pending = append(pending, unreadableRecord(line, codexFieldRecord, model.ContextCompletenessUnavailable))
				continue
			}
			if meta.ID != "" {
				session.ID = meta.ID
			}
		case "turn_context":
			var turn turnContextPayload
			if err := json.Unmarshal(record.Payload, &turn); err != nil {
				pending = append(pending, unreadableRecord(line, codexFieldRecord, model.ContextCompletenessUnavailable))
				continue
			}
			if turn.Model != "" {
				session.Model = turn.Model
			}
		case "response_item":
			components, err := builder.responseItem(record.Payload, line)
			if err != nil {
				pending = append(pending, unreadableRecord(line, codexFieldRecord, model.ContextCompletenessUnavailable))
				continue
			}
			pending = append(pending, components...)
		case "event_msg":
			next, snap, ok := builder.eventMessage(record.Payload, record.Timestamp, line)
			if !ok {
				pending = append(pending, unreadableRecord(line, codexFieldRecord, model.ContextCompletenessUnavailable))
				continue
			}
			pending = append(pending, next...)
			if snap == nil {
				continue
			}
			session.Snapshots = append(session.Snapshots, snapshot{
				Snap:      snap.Snap,
				Timestamp: record.Timestamp,
				Context:   dedupeContext(pending),
			})
			pending = nil
			if snap.Snap.Total >= session.Usage.Total {
				session.Usage = snap.Snap
			}
		}
	}

	session.TrailingContext = dedupeContext(pending)
	return session, nil
}

// eventMessage returns the context contributed by one event_msg record and,
// when the record is a token_count, the snapshot it reports.
func (b *contextBuilder) eventMessage(payload json.RawMessage, timestamp string, line int) ([]model.ContextComponent, *snapshot, bool) {
	var event eventMessagePayload
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, nil, false
	}
	if event.Type == "user_message" {
		recordID := codexRecordID(line)
		return []model.ContextComponent{{
			Kind:        model.ContextUserPrompt,
			Source:      codexSourceName,
			Record:      recordID,
			ContentHash: source.ContentHash(event.Message),
			Observation: model.ContextObservedByAgent,
			Measurement: source.EstimatedTextMeasurement(event.Message),
			Evidence:    []model.Evidence{codexProvenance(recordID, "payload.message")},
		}}, nil, true
	}
	if event.Type != "token_count" {
		return nil, nil, true
	}
	var counts tokenCountPayload
	if err := json.Unmarshal(payload, &counts); err != nil {
		return nil, nil, false
	}
	snap := snapshotFromEvent(counts)
	return nil, &snapshot{Snap: snap, Timestamp: timestamp}, true
}

func (b *contextBuilder) responseItem(payload json.RawMessage, line int) ([]model.ContextComponent, error) {
	var item responseItemPayload
	if err := json.Unmarshal(payload, &item); err != nil {
		return nil, fmt.Errorf("decode response_item payload: %w", err)
	}
	recordID := codexRecordID(line)
	if item.ID != "" {
		recordID = item.ID
	} else if item.CallID != "" {
		recordID = item.CallID
	}

	switch item.Type {
	case "message":
		kind := codexMessageKind(item.Role)
		if kind == "" {
			return nil, nil
		}
		text := codexTextContent(item.Content)
		return []model.ContextComponent{{
			Kind:        kind,
			Source:      codexSourceName,
			Record:      recordID,
			ContentHash: source.ContentHash(text),
			Observation: model.ContextObservedByAgent,
			Measurement: source.EstimatedTextMeasurement(text),
			Evidence:    []model.Evidence{codexProvenance(recordID, "payload.content")},
		}}, nil
	case "function_call", "custom_tool_call":
		// Both call shapes declare the tool name on the call and link to their
		// result through call_id, so they normalize into the same components.
		b.registerCall(item)
		return codexFileComponents(item, recordID), nil
	case "function_call_output", "custom_tool_call_output":
		return []model.ContextComponent{b.toolResultComponent(item, recordID)}, nil
	}
	return nil, nil
}

// registerCall remembers the tool name a call ID refers to so the matching
// output record can carry it. The name is never guessed.
func (b *contextBuilder) registerCall(item responseItemPayload) {
	if item.Name == "" {
		return
	}
	for _, key := range []string{item.CallID, item.ID} {
		if key != "" {
			b.callNames[key] = item.Name
		}
	}
}

func (b *contextBuilder) toolResultComponent(item responseItemPayload, recordID string) model.ContextComponent {
	text := source.ToolOutputText(item.Output)
	return model.ContextComponent{
		Kind:         model.ContextToolResult,
		Source:       codexSourceName,
		Record:       recordID,
		ContentHash:  source.ContentHash(text),
		Observation:  model.ContextObservedByAgent,
		Measurement:  source.EstimatedTextMeasurement(text),
		Completeness: model.ContextCompletenessComplete,
		ToolCallID:   item.CallID,
		ToolName:     b.callNames[item.CallID],
		ContentBytes: source.ToolOutputBytes(item.Output),
		Evidence:     []model.Evidence{codexProvenance(recordID, "payload.output")},
	}
}

func unreadableRecord(line int, field string, completeness model.ContextCompleteness) model.ContextComponent {
	return source.UnreadableRecordComponent(codexSourceName, codexRecordID(line), field, completeness)
}

func codexMessageKind(role string) model.ContextComponentKind {
	switch role {
	case "developer", "system":
		return model.ContextInstructions
	case "user":
		return model.ContextUserPrompt
	case "assistant":
		return model.ContextHistory
	default:
		return ""
	}
}

func codexTextContent(raw json.RawMessage) string {
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
	var b strings.Builder
	for _, block := range blocks {
		b.WriteString(block.Text)
	}
	return b.String()
}

func codexFileComponents(item responseItemPayload, recordID string) []model.ContextComponent {
	if item.Name != "functions.exec_command" && item.Name != "exec_command" {
		return nil
	}
	var args struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal([]byte(item.Arguments), &args); err != nil || args.Cmd == "" {
		return nil
	}
	paths := fileReadPaths(args.Cmd)
	out := make([]model.ContextComponent, 0, len(paths))
	for i, path := range paths {
		rec := recordID + "#file:" + fmt.Sprint(i+1)
		out = append(out, model.ContextComponent{
			Kind:        model.ContextFile,
			Source:      codexSourceName,
			Record:      rec,
			Path:        source.NormalizePath(path),
			Observation: model.ContextObservedByAgent,
			Measurement: model.Measurement{Kind: model.MeasurementUnknown},
			Evidence:    []model.Evidence{codexProvenance(recordID, "payload.arguments.cmd")},
		})
	}
	return out
}

func fileReadPaths(cmd string) []string {
	fields := strings.Fields(cmd)
	if len(fields) < 2 {
		return nil
	}
	switch fields[0] {
	case "cat", "sed", "nl", "awk", "head", "tail":
		return commandPathArgs(fields[1:])
	default:
		return nil
	}
}

func commandPathArgs(fields []string) []string {
	var out []string
	for _, field := range fields {
		if strings.HasPrefix(field, "-") || strings.Contains(field, "=") || strings.ContainsAny(field, "'\"{}") {
			continue
		}
		if strings.Contains(field, "/") || strings.Contains(field, ".") {
			out = append(out, field)
		}
	}
	return out
}

func codexRecordID(line int) string {
	return codexSourceName + "#line:" + fmt.Sprint(line)
}

func codexProvenance(recordID, field string) model.Evidence {
	return model.Evidence{Kind: model.EvidenceProvenance, Source: codexSourceName, Record: recordID, Field: field}
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

func applyTimestamp(session *ParsedSession, timestamp string) {
	if timestamp == "" {
		return
	}
	if session.StartedAt == "" {
		session.StartedAt = timestamp
	}
	session.UpdatedAt = timestamp
}

func snapshotFromEvent(e tokenCountPayload) UsageSnapshot {
	t := e.Info.TotalTokenUsage
	return UsageSnapshot{
		Input:     t.InputTokens,
		Cached:    t.CachedInputTokens,
		Output:    t.OutputTokens,
		Reasoning: t.ReasoningOutputTokens,
		Total:     t.TotalTokens,
		HasUsage:  true,
	}
}

func deltaSnapshot(prev, curr snapshot) (UsageSnapshot, bool) {
	if curr.Snap.Total < prev.Snap.Total ||
		curr.Snap.Input < prev.Snap.Input ||
		curr.Snap.Output < prev.Snap.Output ||
		curr.Snap.Cached < prev.Snap.Cached {
		return UsageSnapshot{}, false
	}
	out := UsageSnapshot{
		Input:    curr.Snap.Input - prev.Snap.Input,
		Cached:   curr.Snap.Cached - prev.Snap.Cached,
		Output:   curr.Snap.Output - prev.Snap.Output,
		Total:    curr.Snap.Total - prev.Snap.Total,
		HasUsage: true,
	}
	if curr.Snap.Reasoning != nil {
		if prev.Snap.Reasoning == nil {
			reasoning := *curr.Snap.Reasoning
			out.Reasoning = &reasoning
		} else if *curr.Snap.Reasoning >= *prev.Snap.Reasoning {
			reasoning := *curr.Snap.Reasoning - *prev.Snap.Reasoning
			out.Reasoning = &reasoning
		} else {
			return UsageSnapshot{}, false
		}
	}
	return out, true
}

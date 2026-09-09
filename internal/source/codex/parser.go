package codex

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

type tokenCountEvent struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		Type string `json:"type"`
		Info struct {
			TotalTokenUsage struct {
				InputTokens           int64  `json:"input_tokens"`
				CachedInputTokens     int64  `json:"cached_input_tokens"`
				OutputTokens          int64  `json:"output_tokens"`
				ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
				TotalTokens           int64  `json:"total_tokens"`
			} `json:"total_token_usage"`
			LastTokenUsage struct {
				InputTokens           int64  `json:"input_tokens"`
				CachedInputTokens     int64  `json:"cached_input_tokens"`
				OutputTokens          int64  `json:"output_tokens"`
				ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
				TotalTokens           int64  `json:"total_tokens"`
			} `json:"last_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

type sessionMetaEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		ID string `json:"id"`
	} `json:"payload"`
}

type turnContextEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Model string `json:"model"`
	} `json:"payload"`
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
}

type codexRecord struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type responseItemEvent struct {
	Type    string `json:"type"`
	Payload struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Role      string          `json:"role"`
		Name      string          `json:"name"`
		CallID    string          `json:"call_id"`
		Content   json.RawMessage `json:"content"`
		Arguments string          `json:"arguments"`
		Output    string          `json:"output"`
	} `json:"payload"`
}

type codexEventMessage struct {
	Type    string `json:"type"`
	Payload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"payload"`
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
	scanner := bufio.NewScanner(r)
	// Raise the default 64 KiB scanner ceiling
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var session ParsedSession
	var pending []model.ContextComponent
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		pending = appendCodexContext(pending, raw, line)
		var meta sessionMetaEvent
		if err := json.Unmarshal(raw, &meta); err == nil && meta.Type == "session_meta" {
			if meta.Payload.ID != "" {
				session.ID = meta.Payload.ID
			}
			applyTimestamp(&session, meta.Timestamp)
			continue
		}
		var turn turnContextEvent
		if err := json.Unmarshal(raw, &turn); err == nil && turn.Type == "turn_context" {
			if turn.Payload.Model != "" {
				session.Model = turn.Payload.Model
			}
			applyTimestamp(&session, turn.Timestamp)
			continue
		}
		var event tokenCountEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		applyTimestamp(&session, event.Timestamp)
		if event.Payload.Type != "token_count" {
			continue
		}
		snap := snapshotFromEvent(event)
		session.Snapshots = append(session.Snapshots, snapshot{Snap: snap, Timestamp: event.Timestamp, Context: dedupeContext(pending)})
		pending = nil
		if snap.Total >= session.Usage.Total {
			session.Usage = snap
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedSession{}, err
	}
	return session, nil
}

func appendCodexContext(components []model.ContextComponent, raw []byte, line int) []model.ContextComponent {
	var record codexRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return components
	}
	recordID := codexRecordID(line)
	if record.Type == "response_item" {
		var event responseItemEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return components
		}
		if event.Payload.ID != "" {
			recordID = event.Payload.ID
		} else if event.Payload.CallID != "" {
			recordID = event.Payload.CallID
		}
		switch event.Payload.Type {
		case "message":
			kind := codexMessageKind(event.Payload.Role)
			if kind == "" {
				return components
			}
			text := codexTextContent(event.Payload.Content)
			return append(components, model.ContextComponent{
				Kind:        kind,
				Source:      "codex_rollout",
				Record:      recordID,
				ContentHash: source.ContentHash(text),
				Observation: model.ContextObservedByAgent,
				Measurement: source.EstimatedTextMeasurement(text),
				Evidence:    []model.Evidence{codexProvenance(recordID, "payload.content")},
			})
		case "function_call_output":
			return append(components, model.ContextComponent{
				Kind:        model.ContextToolResult,
				Source:      "codex_rollout",
				Record:      recordID,
				ContentHash: source.ContentHash(event.Payload.Output),
				Observation: model.ContextObservedByAgent,
				Measurement: source.EstimatedTextMeasurement(event.Payload.Output),
				Evidence:    []model.Evidence{codexProvenance(recordID, "payload.output")},
			})
		case "function_call":
			return append(components, codexFileComponents(event, recordID)...)
		}
		return components
	}
	if record.Type == "event_msg" {
		var event codexEventMessage
		if err := json.Unmarshal(raw, &event); err != nil {
			return components
		}
		if event.Payload.Type == "user_message" {
			return append(components, model.ContextComponent{
				Kind:        model.ContextUserPrompt,
				Source:      "codex_rollout",
				Record:      recordID,
				ContentHash: source.ContentHash(event.Payload.Message),
				Observation: model.ContextObservedByAgent,
				Measurement: source.EstimatedTextMeasurement(event.Payload.Message),
				Evidence:    []model.Evidence{codexProvenance(recordID, "payload.message")},
			})
		}
	}
	return components
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

func codexFileComponents(event responseItemEvent, recordID string) []model.ContextComponent {
	if event.Payload.Name != "functions.exec_command" && event.Payload.Name != "exec_command" {
		return nil
	}
	var args struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal([]byte(event.Payload.Arguments), &args); err != nil || args.Cmd == "" {
		return nil
	}
	paths := fileReadPaths(args.Cmd)
	out := make([]model.ContextComponent, 0, len(paths))
	for i, path := range paths {
		rec := recordID + "#file:" + fmt.Sprint(i+1)
		out = append(out, model.ContextComponent{
			Kind:        model.ContextFile,
			Source:      "codex_rollout",
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
	return "codex_rollout#line:" + fmt.Sprint(line)
}

func codexProvenance(recordID, field string) model.Evidence {
	return model.Evidence{Kind: model.EvidenceProvenance, Source: "codex_rollout", Record: recordID, Field: field}
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

func snapshotFromEvent(e tokenCountEvent) UsageSnapshot {
	t := e.Payload.Info.TotalTokenUsage
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

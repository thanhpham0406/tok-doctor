package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
}

type ParsedSession struct {
	ID        string
	Model     string
	StartedAt string
	UpdatedAt string
	Usage     UsageSnapshot
	Snapshots []snapshot
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
	defer f.Close()

	session, err := parseSession(f)
	if err != nil {
		return ParsedSession{}, fmt.Errorf("read codex session %s: %w", path, err)
	}
	return session, nil
}

func parseSessionUsage(r io.Reader) (UsageSnapshot, error) {
	session, err := parseSession(r)
	if err != nil {
		return UsageSnapshot{}, err
	}
	return session.Usage, nil
}

func parseSession(r io.Reader) (ParsedSession, error) {
	scanner := bufio.NewScanner(r)
	// Raise the default 64 KiB scanner ceiling
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var session ParsedSession
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
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
		session.Snapshots = append(session.Snapshots, snapshot{Snap: snap, Timestamp: event.Timestamp})
		if snap.Total >= session.Usage.Total {
			session.Usage = snap
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedSession{}, err
	}
	return session, nil
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
		Input:  curr.Snap.Input - prev.Snap.Input,
		Cached: curr.Snap.Cached - prev.Snap.Cached,
		Output: curr.Snap.Output - prev.Snap.Output,
		Total:  curr.Snap.Total - prev.Snap.Total,
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

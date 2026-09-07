package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type tokenCountEvent struct {
	Payload struct {
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

func ParseSessionUsage(path string) (UsageSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return UsageSnapshot{}, fmt.Errorf("open codex session %s: %w", path, err)
	}
	defer f.Close()

	final, err := parseSessionUsage(f)
	if err != nil {
		return UsageSnapshot{}, fmt.Errorf("read codex session %s: %w", path, err)
	}
	return final, nil
}

func parseSessionUsage(r io.Reader) (UsageSnapshot, error) {
	scanner := bufio.NewScanner(r)
	// Raise the default 64 KiB scanner ceiling
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var final UsageSnapshot
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var event tokenCountEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		if event.Payload.Type != "token_count" {
			continue
		}
		snap := snapshotFromEvent(event)
		if snap.Total >= final.Total {
			final = snap
		}
	}
	if err := scanner.Err(); err != nil {
		return UsageSnapshot{}, err
	}
	return final, nil
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

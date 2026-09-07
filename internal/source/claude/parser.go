package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type assistantUsage struct {
	Type    string `json:"type"`
	Message struct {
		Role  string `json:"role"`
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func ParseSessionUsage(path string) (UsageSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return UsageSnapshot{}, fmt.Errorf("open claude session %s: %w", path, err)
	}
	defer f.Close()
	return parseSessionUsage(f)
}

func parseSessionUsage(r io.Reader) (UsageSnapshot, error) {
	scanner := bufio.NewScanner(r)
	// Raise the default 64 KiB scanner ceiling
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var total UsageSnapshot
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var entry assistantUsage
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		if entry.Type != "assistant" || entry.Message.Role != "assistant" {
			continue
		}
		u := entry.Message.Usage
		total.Input += u.InputTokens
		total.Output += u.OutputTokens
		total.Cached += u.CacheReadInputTokens + u.CacheCreationInputTokens
		total.Total += u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	}
	if err := scanner.Err(); err != nil {
		return UsageSnapshot{}, err
	}
	return total, nil
}

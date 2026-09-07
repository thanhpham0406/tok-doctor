package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type assistantUsage struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Message   struct {
		Role  string       `json:"role"`
		Model string       `json:"model"`
		Usage *usageFields `json:"usage"`
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
		snap.Cached += *u.CacheReadInputTokens
		populated = true
	}
	if u.CacheCreationInputTokens != nil {
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

type ParsedSession struct {
	Model      string
	StartedAt  string
	UpdatedAt  string
	Usage      UsageSnapshot
	modelNames map[string]struct{}
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
	defer f.Close()
	session, err := parseSession(f)
	if err != nil {
		return ParsedSession{}, fmt.Errorf("read claude session %s: %w", path, err)
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
		var entry assistantUsage
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		applyTimestamp(&session, entry.Timestamp)
		if entry.Type != "assistant" || entry.Message.Role != "assistant" {
			continue
		}
		if isRealModel(entry.Message.Model) {
			if session.modelNames == nil {
				session.modelNames = map[string]struct{}{}
			}
			session.modelNames[entry.Message.Model] = struct{}{}
			session.Model = selectedModel(session.modelNames)
		}
		add := entry.Message.Usage.snapshot()
		if add.HasUsage {
			session.Usage = session.Usage.add(add)
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedSession{}, err
	}
	return session, nil
}

func isRealModel(model string) bool {
	return model != "" && model != "<synthetic>"
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

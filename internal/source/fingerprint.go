package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ToolCallFingerprintVersion is part of every fingerprint, so a future change
// to the canonical form cannot silently collide with values an older version
// produced.
const ToolCallFingerprintVersion = 1

const toolCallFingerprintPrefix = "v1:"

type toolCallFingerprintPayload struct {
	Version   int             `json:"version"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolCallFingerprint returns an opaque, versioned digest of a tool name and
// its raw JSON arguments. The name is trimmed but never case-folded. The
// arguments are decoded with json.Number, re-encoded so object keys order
// deterministically, and hashed with array order preserved. Arguments that are
// not a single well-formed JSON value are rejected, and the arguments never
// appear in the returned error.
func ToolCallFingerprint(name, rawArguments string) (string, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return "", errors.New("tool call fingerprint: tool name is required")
	}
	canonicalArguments, err := canonicalJSONValue(rawArguments)
	if err != nil {
		return "", fmt.Errorf("tool call fingerprint: %w", err)
	}
	encoded, err := json.Marshal(toolCallFingerprintPayload{
		Version:   ToolCallFingerprintVersion,
		Name:      trimmedName,
		Arguments: canonicalArguments,
	})
	if err != nil {
		return "", errors.New("tool call fingerprint: payload could not be encoded")
	}
	sum := sha256.Sum256(encoded)
	return toolCallFingerprintPrefix + hex.EncodeToString(sum[:]), nil
}

// ToolCallFingerprintOrEmpty fingerprints a tool call from the arguments a
// source declared. It returns an empty string when the source declared no
// usable arguments or when they could not be canonicalized, because a call that
// cannot be fingerprinted must not fail the session that contains it.
func ToolCallFingerprintOrEmpty(name string, rawArguments json.RawMessage) string {
	arguments := toolCallArgumentText(rawArguments)
	if arguments == "" {
		return ""
	}
	fingerprint, err := ToolCallFingerprint(name, arguments)
	if err != nil {
		return ""
	}
	return fingerprint
}

// toolCallArgumentText unwraps the argument text of one tool call. A payload
// that is a JSON string is unwrapped so a source that nests its arguments in a
// string and a source that inlines them as JSON both yield the same text.
func toolCallArgumentText(rawArguments json.RawMessage) string {
	trimmed := bytes.TrimSpace(rawArguments)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text
	}
	return string(trimmed)
}

func canonicalJSONValue(raw string) (json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("arguments are not valid JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("arguments carry data after the first JSON value")
	}
	if value == nil {
		return nil, errors.New("arguments are not a JSON value")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("arguments could not be canonicalized")
	}
	return encoded, nil
}

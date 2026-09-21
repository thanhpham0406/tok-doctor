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

// Structured and freeform fingerprints describe different kinds of tool call
// arguments, so they are built from different payloads and carry different
// prefixes. A structured call and a freeform call whose visible text happens to
// match can therefore never produce the same fingerprint.
const (
	structuredToolCallFingerprintPrefix = "v1:"
	freeformToolCallFingerprintPrefix   = "v1-text:"
)

// toolCallFingerprintFormatFreeform marks a payload as freeform text. The marker
// is inside the hashed payload, so the domains stay apart even if a prefix is
// ever changed.
const toolCallFingerprintFormatFreeform = "text"

type structuredToolCallFingerprintPayload struct {
	Version   int             `json:"version"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type freeformToolCallFingerprintPayload struct {
	Version int    `json:"version"`
	Format  string `json:"format"`
	Name    string `json:"name"`
	Input   string `json:"input"`
}

// StructuredToolCallFingerprint returns an opaque, versioned digest of a tool
// name and its raw JSON arguments. The name is trimmed but never case-folded.
// The arguments are decoded with json.Number, re-encoded so object keys order
// deterministically, and hashed with array order preserved. Arguments that are
// not a single well-formed JSON value are rejected, and the arguments never
// appear in the returned error.
func StructuredToolCallFingerprint(name, rawArguments string) (string, error) {
	trimmedName, err := fingerprintToolName(name)
	if err != nil {
		return "", err
	}
	canonicalArguments, err := canonicalJSONValue(rawArguments)
	if err != nil {
		return "", fmt.Errorf("tool call fingerprint: %w", err)
	}
	encoded, err := json.Marshal(structuredToolCallFingerprintPayload{
		Version:   ToolCallFingerprintVersion,
		Name:      trimmedName,
		Arguments: canonicalArguments,
	})
	if err != nil {
		return "", errors.New("tool call fingerprint: payload could not be encoded")
	}
	return structuredToolCallFingerprintPrefix + toolCallFingerprintDigest(encoded), nil
}

// FreeformToolCallFingerprint returns an opaque, versioned digest of a tool
// name and the exact freeform text a call declared. The name is trimmed but
// never case-folded. The input is hashed as text: it is never parsed as JSON
// and never trimmed or normalized, so two inputs differ unless every byte
// matches. Empty input is rejected, and the input never appears in the returned
// error.
func FreeformToolCallFingerprint(name, input string) (string, error) {
	trimmedName, err := fingerprintToolName(name)
	if err != nil {
		return "", err
	}
	if input == "" {
		return "", errors.New("tool call fingerprint: freeform input is required")
	}
	encoded, err := json.Marshal(freeformToolCallFingerprintPayload{
		Version: ToolCallFingerprintVersion,
		Format:  toolCallFingerprintFormatFreeform,
		Name:    trimmedName,
		Input:   input,
	})
	if err != nil {
		return "", errors.New("tool call fingerprint: payload could not be encoded")
	}
	return freeformToolCallFingerprintPrefix + toolCallFingerprintDigest(encoded), nil
}

// StructuredToolCallFingerprintOrEmpty fingerprints a call whose arguments the
// source declared as JSON. It returns an empty string when the source declared
// no arguments or when they could not be canonicalized, because a call that
// cannot be fingerprinted must not fail the session that contains it.
func StructuredToolCallFingerprintOrEmpty(name string, rawArguments json.RawMessage) string {
	arguments := toolCallArgumentText(rawArguments)
	if arguments == "" {
		return ""
	}
	fingerprint, err := StructuredToolCallFingerprint(name, arguments)
	if err != nil {
		return ""
	}
	return fingerprint
}

// FreeformToolCallFingerprintOrEmpty fingerprints a call whose input the source
// declared as freeform text. It returns an empty string when there is no input
// to fingerprint, because a call that cannot be fingerprinted must not fail the
// session that contains it.
func FreeformToolCallFingerprintOrEmpty(name, input string) string {
	fingerprint, err := FreeformToolCallFingerprint(name, input)
	if err != nil {
		return ""
	}
	return fingerprint
}

func fingerprintToolName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("tool call fingerprint: tool name is required")
	}
	return trimmed, nil
}

func toolCallFingerprintDigest(encoded []byte) string {
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// toolCallArgumentText unwraps the argument text of one tool call. A payload
// that is a JSON string is unwrapped so a source that nests its arguments in a
// string and a source that inlines them as JSON both yield the same text. The
// result is still parsed as JSON by the caller, so only the structured domain
// ever flows through here.
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

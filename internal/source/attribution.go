package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func EstimatedTextMeasurement(text string) model.Measurement {
	if text == "" {
		return model.Measurement{Kind: model.MeasurementUnknown}
	}
	runes := int64(utf8.RuneCountInString(text))
	tokens := (runes + 3) / 4
	if tokens == 0 {
		tokens = 1
	}
	return model.NewMeasurement(tokens, model.MeasurementEstimated)
}

// ToolOutputText extracts the textual part of a tool output payload. Outputs
// that carry no text, such as images, yield an empty string so the caller
// keeps the token estimate unknown instead of inventing one.
func ToolOutputText(raw json.RawMessage) string {
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
			b.WriteString(ToolOutputText(block.Content))
		}
	}
	return b.String()
}

// ToolOutputBytes reports the size a tool output payload has in the source:
// the decoded length for string payloads and the raw JSON length otherwise.
// It stays nil when the source omits the output, because a missing output is
// not the same as a zero-byte one.
func ToolOutputBytes(raw json.RawMessage) *int64 {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		return model.Int64(int64(len(s)))
	}
	return model.Int64(int64(len(trimmed)))
}

// UnreadableRecordComponent describes a source record TokDoctor could not read.
// Emitting it keeps partial coverage visible to analysis instead of silently
// dropping the record; it carries no measurement because nothing was read.
func UnreadableRecordComponent(sourceName, recordID, field string, completeness model.ContextCompleteness) model.ContextComponent {
	return model.ContextComponent{
		Kind:         model.ContextUnknown,
		Source:       sourceName,
		Record:       recordID,
		Observation:  model.ContextObservedByAgent,
		Measurement:  model.Measurement{Kind: model.MeasurementUnknown},
		Completeness: completeness,
		Evidence: []model.Evidence{{
			Kind:   model.EvidenceProvenance,
			Source: sourceName,
			Record: recordID,
			Field:  field,
		}},
	}
}

func ContentHash(text string) string {
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

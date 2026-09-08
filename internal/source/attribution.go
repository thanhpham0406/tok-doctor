package source

import (
	"crypto/sha256"
	"encoding/hex"
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

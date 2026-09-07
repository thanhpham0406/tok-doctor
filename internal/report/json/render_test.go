package json

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
)

func TestRender(t *testing.T) {
	var out bytes.Buffer

	if err := Render(&out, analyze.Result{Source: "codex"}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var decoded analyze.Result
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal rendered JSON: %v", err)
	}
	if decoded.Source != "codex" {
		t.Fatalf("Source = %q, want codex", decoded.Source)
	}
}

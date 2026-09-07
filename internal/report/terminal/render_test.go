package terminal

import (
	"bytes"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestRender(t *testing.T) {
	var out bytes.Buffer
	result := analyze.Result{
		Source:  "codex",
		Session: model.Session{ID: "scaffold"},
		Summary: analyze.Summary{Status: "ready", Message: "ok"},
	}

	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "Status: ready") {
		t.Fatalf("output = %q, want status", got)
	}
}

package sessions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestRenderUsesThousandsSeparatorsAndUnavailableValues(t *testing.T) {
	var out bytes.Buffer
	result := model.SessionsResult{Sessions: []model.Session{
		{
			ID:     "session-with-long-id",
			Source: "codex",
			Usage: model.Usage{
				Input:      111249946,
				Total:      111249946,
				Confidence: model.ConfidenceMeasured,
			},
		},
		{
			ID:     "empty",
			Source: "claude",
		},
	}}

	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := out.String()
	for _, want := range []string{"111,249,946", "session-with", "claude", "-"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestRenderNoSessions(t *testing.T) {
	var out bytes.Buffer
	if err := Render(&out, model.SessionsResult{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "No sessions available") {
		t.Fatalf("output = %q, want no sessions message", got)
	}
}

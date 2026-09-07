package cli

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "tok dev") {
		t.Fatalf("version output = %q, want tok dev", got)
	}
}

func TestDoctorCommand(t *testing.T) {
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"doctor"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute doctor: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{"TokDoctor", "Source: codex", "Findings: 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output = %q, want %q", got, want)
		}
	}
}

func TestDoctorJSONCommand(t *testing.T) {
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"doctor", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute doctor json: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, `"source": "codex"`) {
		t.Fatalf("doctor json output = %q, want source", got)
	}
}

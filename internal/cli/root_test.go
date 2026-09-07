package cli

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
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

func TestSourcesCommand(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sources"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute sources: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{"Sources", "Codex", "9Router"} {
		if !strings.Contains(got, want) {
			t.Fatalf("sources output = %q, want %q", got, want)
		}
	}
}

func TestSourceShowJSONCommand(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"source", "show", "codex", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute source show json: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, `"name": "codex"`) {
		t.Fatalf("source show json output = %q, want codex name", got)
	}
}

func TestSourceSetAndResetCommand(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("TOKDOCTOR_CONFIG", configPath)
	sourcePath := t.TempDir()

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"source", "set", "codex", "--path", sourcePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute source set: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), sourcePath) {
		t.Fatalf("config = %q, want configured path", data)
	}

	stdout.Reset()
	cmd = newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"source", "reset", "codex"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute source reset: %v", err)
	}

	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read reset config: %v", err)
	}
	if strings.Contains(string(data), sourcePath) {
		t.Fatalf("config = %q, want path reset", data)
	}
}

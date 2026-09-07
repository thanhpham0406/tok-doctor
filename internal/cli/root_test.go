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

func TestUsageCommandMissingSourceFlag(t *testing.T) {
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"usage"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --source is missing")
	}
	if !strings.Contains(err.Error(), "--source is required") {
		t.Fatalf("error = %q, want --source required", err.Error())
	}
}

func TestUsageCommandUnsupportedRouter9(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"usage", "--source", "router9"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for router9")
	}
	if !strings.Contains(err.Error(), "usage not supported for router9") {
		t.Fatalf("error = %q, want router9 unsupported message", err.Error())
	}
}

func TestUsageCommandUnsupportedUnknownSource(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"usage", "--source", "nosuch"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
	if !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("error = %q, want unknown source", err.Error())
	}
}

func TestUsageCommandNoDataAvailable(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"usage", "--source", "codex"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute usage: %v", err)
	}
	if !strings.Contains(stdout.String(), "no usage data available") {
		t.Fatalf("output = %q, want no usage data available", stdout.String())
	}
}

func TestUsageAllCommand(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"usage", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute usage all: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "codex: no usage data available") ||
		!strings.Contains(got, "claude: no usage data available") {
		t.Fatalf("output = %q, want codex and claude no data", got)
	}
}

func TestUsageAllJSONCommand(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"usage", "--all", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute usage all json: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"sources"`) ||
		!strings.Contains(got, `"source": "codex"`) ||
		!strings.Contains(got, `"source": "claude"`) {
		t.Fatalf("output = %q, want usage sources json", got)
	}
}

func TestUsageAllRejectsSource(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"usage", "--all", "--source", "codex"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --all with --source")
	}
	if !strings.Contains(err.Error(), "--all cannot be used with --source") {
		t.Fatalf("error = %q, want flag conflict", err.Error())
	}
}

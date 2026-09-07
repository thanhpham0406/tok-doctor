package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
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

func TestSessionsCommand(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute sessions: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{"Sessions", "Source", "codex", "claude"} {
		if !strings.Contains(got, want) {
			t.Fatalf("sessions output = %q, want %q", got, want)
		}
	}
	for _, unwanted := range []string{"empty-artifa", "synthetic-o", "<synthetic>"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("sessions output = %q, did not want %q", got, unwanted)
		}
	}
}

func TestSessionsCommandUnsupportedRouter9(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions", "--source", "router9"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for router9")
	}
	if !strings.Contains(err.Error(), "sessions not supported for router9") {
		t.Fatalf("error = %q, want router9 unsupported message", err.Error())
	}
}

func TestSessionsJSONCommandKeepsNumbersNumeric(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions", "--source", "codex", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute sessions json: %v", err)
	}

	var result model.SessionsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if len(result.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(result.Sessions))
	}
	for _, session := range result.Sessions {
		if session.ID == "sess-2" && session.Usage.Total != 4450 {
			t.Fatalf("total = %d, want numeric 4450", session.Usage.Total)
		}
	}
	if strings.Contains(stdout.String(), `"total": "`) {
		t.Fatalf("json output has string total: %s", stdout.String())
	}
}

func TestClaudeSessionsJSONMatchesFilteredTerminalSet(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var terminalOut bytes.Buffer
	cmd := newRootCommand(context.Background(), &terminalOut, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions", "--source", "claude"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute sessions terminal: %v", err)
	}

	var jsonOut bytes.Buffer
	cmd = newRootCommand(context.Background(), &jsonOut, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions", "--source", "claude", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute sessions json: %v", err)
	}

	var result model.SessionsResult
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("decode json: %v\n%s", err, jsonOut.String())
	}
	if len(result.Sessions) != 4 {
		t.Fatalf("sessions = %d, want 4 (basic-session, cached-only, output-only, input-only)", len(result.Sessions))
	}
	gotIDs := map[string]bool{}
	for _, session := range result.Sessions {
		gotIDs[session.ID] = true
	}
	for _, want := range []string{"basic-session", "cached-only-session", "output-only-session", "input-only-session"} {
		if !gotIDs[want] {
			t.Fatalf("missing session %q, got %v", want, gotIDs)
		}
	}
	for _, out := range []string{terminalOut.String(), jsonOut.String()} {
		for _, unwanted := range []string{"empty-artifact", "synthetic-only", "explicit-zero", "empty-usage", "<synthetic>"} {
			if strings.Contains(out, unwanted) {
				t.Fatalf("output = %q, did not want %q", out, unwanted)
			}
		}
	}
}

func TestClaudeSessionsTerminalAndJsonAgreeOnIds(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var terminalOut bytes.Buffer
	cmd := newRootCommand(context.Background(), &terminalOut, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions", "--source", "claude"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("terminal sessions: %v", err)
	}

	var jsonOut bytes.Buffer
	cmd = newRootCommand(context.Background(), &jsonOut, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions", "--source", "claude", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("json sessions: %v", err)
	}

	var result model.SessionsResult
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("decode json: %v\n%s", err, jsonOut.String())
	}
	if len(result.Sessions) == 0 {
		t.Fatalf("json returned no sessions; terminal: %s", terminalOut.String())
	}
	for _, session := range result.Sessions {
		if !strings.Contains(terminalOut.String(), session.ID[:12]) {
			t.Fatalf("session %q present in json but missing from terminal output", session.ID)
		}
	}
}

func withCLIFixtureHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	copyFixture := func(src, dst string) {
		t.Helper()
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %v", src, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir fixture dir: %v", err)
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", dst, err)
		}
	}
	copyFixture(filepath.Join("..", "..", "fixtures", "codex", "basic-session.jsonl"),
		filepath.Join(home, ".codex", "sessions", "basic-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "codex", "multi-snapshot-session.jsonl"),
		filepath.Join(home, ".codex", "sessions", "multi-snapshot-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "claude", "basic-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "basic-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "claude", "empty-artifact-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "empty-artifact-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "claude", "synthetic-only-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "synthetic-only-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "claude", "explicit-zero-usage-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "explicit-zero-usage-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "claude", "empty-usage-object-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "empty-usage-object-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "claude", "cached-only-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "cached-only-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "claude", "output-only-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "output-only-session.jsonl"))
	copyFixture(filepath.Join("..", "..", "fixtures", "claude", "input-only-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "input-only-session.jsonl"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

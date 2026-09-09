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

func TestGatewayStatusShowsProxyURLWhenStopped(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("TOKDOCTOR_CONFIG", configPath)
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(dir, "gateway"))
	if err := os.WriteFile(configPath, []byte(`
[gateway.profiles.codex]
enabled = "true"
listen = "127.0.0.1:8787"
protocol = "openai_responses"
source = "codex"
upstream = "https://api.openai.com"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"gateway", "status", "--profile", "codex"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute gateway status: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "http://127.0.0.1:8787") || !strings.Contains(got, "stopped") {
		t.Fatalf("status output = %q", got)
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

func TestCostJSONCommand(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir codex: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "codex", "basic-session.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "sessions", "basic.jsonl"), data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"cost", "sess-1", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost json: %v", err)
	}
	var decoded struct {
		SessionID string `json:"session_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode cost json: %v\n%s", err, stdout.String())
	}
	if decoded.SessionID != "sess-1" || decoded.Status != "unavailable" {
		t.Fatalf("cost result = %+v, want sess-1 unavailable with empty production catalog", decoded)
	}
}

func TestCostAllJSONCommand(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"cost", "--all", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost all json: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"overlap"`) || !strings.Contains(got, `"groups"`) {
		t.Fatalf("output = %q, want cost collection json", got)
	}
}

func TestCostProviderJSONCommand(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"cost", "--provider", "openai", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost provider json: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"provider": "openai"`) {
		t.Fatalf("output = %q, want provider json", got)
	}
}

func TestPricingJSONCommands(t *testing.T) {
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	for _, args := range [][]string{
		{"pricing", "status", "--format", "json"},
		{"pricing", "list", "--format", "json"},
		{"pricing", "show", "gpt-5.5", "--format", "json"},
	} {
		var stdout bytes.Buffer
		cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
		if !json.Valid(stdout.Bytes()) {
			t.Fatalf("%v output is not json: %q", args, stdout.String())
		}
	}
}

func TestPricingMissingJSONCommandDeduplicatesSessions(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"pricing", "missing", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute pricing missing json: %v", err)
	}
	var decoded struct {
		Missing []struct {
			Model    string `json:"model"`
			Sessions int    `json:"sessions"`
			Turns    int    `json:"turns"`
		} `json:"missing"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode pricing missing json: %v\n%s", err, stdout.String())
	}
	found := false
	for _, miss := range decoded.Missing {
		if miss.Model == "gpt-5" {
			found = true
			if miss.Sessions == 0 || miss.Turns == 0 {
				t.Fatalf("gpt-5 missing counts = %+v, want non-zero", miss)
			}
		}
	}
	if !found {
		t.Fatalf("missing = %+v, want gpt-5 unresolved", decoded.Missing)
	}
}

func TestPricingAddPersistsOverrideAndShowResolves(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir codex: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "codex", "basic-session.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "sessions", "basic.jsonl"), data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"pricing", "add", "custom/model-x", "--input", "1.25", "--cached-input", "0.10", "--output", "9", "--currency", "usd"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute pricing add: %v", err)
	}
	stdout.Reset()
	cmd = newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"pricing", "show", "model-x"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute pricing show: %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"Provider       custom", "SKU            model-x", "Catalog        override"} {
		if !strings.Contains(got, want) {
			t.Fatalf("show output = %q, want %q", got, want)
		}
	}

	stdout.Reset()
	cmd = newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"pricing", "add", "openai/gpt-5", "--input", "1", "--output", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute pricing add gpt-5: %v", err)
	}
	stdout.Reset()
	cmd = newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"cost", "sess-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost with override: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "API-equivalent cost") || strings.Contains(got, "unavailable") {
		t.Fatalf("cost output = %q, want resolved cost from override", got)
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
	if strings.Contains(got, "sess-empty") {
		t.Fatalf("sessions output = %q, did not want Codex no-usage session", got)
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
		if session.ID == "sess-2" && session.Usage.Total.ValueOrZero() != 4450 {
			t.Fatalf("total = %d, want numeric 4450", session.Usage.Total.ValueOrZero())
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

func TestCodexSessionsTerminalAndJsonAgreeOnIds(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var terminalOut bytes.Buffer
	cmd := newRootCommand(context.Background(), &terminalOut, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions", "--source", "codex"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("terminal sessions: %v", err)
	}

	var jsonOut bytes.Buffer
	cmd = newRootCommand(context.Background(), &jsonOut, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"sessions", "--source", "codex", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("json sessions: %v", err)
	}

	var result model.SessionsResult
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("decode json: %v\n%s", err, jsonOut.String())
	}
	if len(result.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2 Codex sessions with usage", len(result.Sessions))
	}
	for _, session := range result.Sessions {
		if !strings.Contains(terminalOut.String(), session.ID[:min(12, len(session.ID))]) {
			t.Fatalf("session %q present in json but missing from terminal output", session.ID)
		}
	}
	for _, out := range []string{terminalOut.String(), jsonOut.String()} {
		if strings.Contains(out, "sess-empty") || strings.Contains(out, "no-usage") {
			t.Fatalf("output = %q, did not want Codex no-usage session", out)
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
	copyFixture(filepath.Join("..", "..", "fixtures", "codex", "no-usage-session.jsonl"),
		filepath.Join(home, ".codex", "sessions", "no-usage-session.jsonl"))
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

func TestInspectCommandAppearsInHelp(t *testing.T) {
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "inspect") {
		t.Fatalf("help = %q, want inspect subcommand listed", got)
	}
}

func TestInspectCommandRequiresExactlyOneArg(t *testing.T) {
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no id provided")
	}
}

func TestInspectCommandFullID(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"Session sess-1", "Source          codex", "Model calls"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestInspectCommandShortPrefix(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "sess-2") {
		t.Fatalf("output = %q, want sess-2", got)
	}
}

func TestInspectCommandUnknownSession(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "zzznotreal"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want 'not found'", err.Error())
	}
}

func TestInspectCommandAmbiguousPrefix(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for ambiguous session id")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %q, want 'ambiguous'", err.Error())
	}
}

func TestInspectCommandJSONOutput(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-2", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect json: %v", err)
	}

	var decoded struct {
		Session struct {
			ID     string       `json:"id"`
			Source string       `json:"source"`
			Usage  model.Usage  `json:"usage"`
			Turns  []model.Turn `json:"turns"`
		} `json:"session"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if decoded.Session.ID != "sess-2" {
		t.Fatalf("session.id = %q, want sess-2", decoded.Session.ID)
	}
	if decoded.Session.Source != "codex" {
		t.Fatalf("session.source = %q, want codex", decoded.Session.Source)
	}
	if decoded.Session.Usage.Total.ValueOrZero() != 4450 {
		t.Fatalf("usage.total = %d, want 4450", decoded.Session.Usage.Total.ValueOrZero())
	}
	if len(decoded.Session.Turns) != 3 {
		t.Fatalf("turns = %d, want 3", len(decoded.Session.Turns))
	}
	if strings.Contains(stdout.String(), `"total": "`) {
		t.Fatalf("json output has string total: %s", stdout.String())
	}
}

func TestInspectCommandEvidenceFlagShowsProvenance(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-2", "--turn", "2", "--evidence"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"Evidence", "cumulative delta", "codex_rollout", "snap:1", "snap:2", "input_tokens"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestInspectCommandContextFlagShowsAttribution(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-1", "--turn", "1", "--context"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect context: %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"Context", "Instructions", "User prompt", "Tool results", "Top files", "AGENTS.md", "Reconciliation"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestInspectCommandJSONContextFlagIncludesTurn(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-1", "--turn", "1", "--context", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect context json: %v", err)
	}
	var decoded struct {
		Turn *model.Turn `json:"turn"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if decoded.Turn == nil || len(decoded.Turn.ContextAttribution.Components) == 0 {
		t.Fatalf("decoded = %+v, want turn context components", decoded)
	}
	if strings.Contains(stdout.String(), "Inspect the fixture.") || strings.Contains(stdout.String(), "synthetic output") {
		t.Fatalf("json leaked raw context content: %s", stdout.String())
	}
}

func TestInspectCommandWithoutEvidenceFlagHidesProvenance(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-2", "--turn", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	if strings.Contains(stdout.String(), "Evidence") {
		t.Fatalf("evidence section should be hidden without --evidence, got %q", stdout.String())
	}
}

func TestInspectCommandJSONIncludesEvidenceMetadata(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-2", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect json: %v", err)
	}
	for _, want := range []string{`"kind": "cumulative_delta"`, `"source": "codex_rollout"`, `"field": "input_tokens"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("json output missing %q: %s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), `"total": "`) {
		t.Fatalf("json output must keep numeric totals: %s", stdout.String())
	}
}

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
)

func runCoverage(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs(append([]string{"coverage", "tool-output"}, args...))
	err := cmd.Execute()
	return stdout.String(), err
}

func coverageHome(t *testing.T) {
	t.Helper()
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
}

// coverageHomeWithCodexFixtures installs only the named Codex fixtures, so a
// test can pin the exact session population a source scan sees.
func coverageHomeWithCodexFixtures(t *testing.T, names ...string) {
	t.Helper()
	home := t.TempDir()
	sessions := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatalf("mkdir codex sessions: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatalf("mkdir claude projects: %v", err)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "codex", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(sessions, name), data, 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
}

func TestCoverageToolOutputRequiresSelector(t *testing.T) {
	coverageHome(t)

	_, err := runCoverage(t)
	if err == nil || !strings.Contains(err.Error(), "session-id, --source, or --all is required") {
		t.Fatalf("error = %v, want missing selector", err)
	}
}

func TestCoverageToolOutputRejectsConflictingSelectors(t *testing.T) {
	coverageHome(t)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"sess-1", "--source", "codex"}, "session-id cannot be combined with --source or --all"},
		{[]string{"sess-1", "--all"}, "session-id cannot be combined with --source or --all"},
		{[]string{"--all", "--source", "codex"}, "--all cannot be combined with --source"},
	}
	for _, testCase := range cases {
		_, err := runCoverage(t, testCase.args...)
		if err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Fatalf("%v error = %v, want %q", testCase.args, err, testCase.want)
		}
	}
}

func TestCoverageToolOutputSessionErrors(t *testing.T) {
	coverageHome(t)

	if _, err := runCoverage(t, "zzz-missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found", err)
	}
	if _, err := runCoverage(t, "sess"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %v, want ambiguous", err)
	}
}

func TestCoverageToolOutputSourceErrors(t *testing.T) {
	coverageHome(t)

	_, err := runCoverage(t, "--source", "router9")
	if err == nil || !strings.Contains(err.Error(), "tool output coverage not supported for router9") {
		t.Fatalf("error = %v, want router9 unsupported", err)
	}
	_, err = runCoverage(t, "--source", "nosuch")
	if err == nil || !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("error = %v, want unknown source", err)
	}
}

func TestCoverageToolOutputSessionTerminal(t *testing.T) {
	coverageHome(t)

	got, err := runCoverage(t, "sess-1")
	if err != nil {
		t.Fatalf("execute coverage: %v", err)
	}
	if !strings.Contains(got, "Tool Output Coverage — codex (session sess-1)") {
		t.Fatalf("output = %q, want session title", got)
	}
	lines := coverageLines(got)
	for _, want := range []string{
		"Tool outputs: 1",
		"With byte size: 1 (100.0%)",
		"With call ID: 1 (100.0%)",
		"With tool name: 1 (100.0%)",
		"Recognized tool outputs:",
		"Complete: 1",
		"Truncated: 0",
		"counted over 1 recognized tool output only",
		"Unreadable records: 0 (0 sessions)",
		"Linked to fresh input: 1 (100.0%)",
		"total 16 B",
		"Output tokens (estimated): 4",
	} {
		if !lines[want] {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestCoverageToolOutputSessionWithoutToolOutput(t *testing.T) {
	coverageHome(t)

	got, err := runCoverage(t, "sess-2")
	if err != nil {
		t.Fatalf("execute coverage: %v", err)
	}
	lines := coverageLines(got)
	for _, want := range []string{
		"Tool outputs: 0",
		"With byte size: 0 (n/a)",
		"Linked to fresh input: 0 (n/a)",
		"no byte size reported",
		"Output tokens (estimated): n/a",
	} {
		if !lines[want] {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestCoverageToolOutputSessionJSON(t *testing.T) {
	coverageHome(t)

	got, err := runCoverage(t, "sess-1", "--format", "json")
	if err != nil {
		t.Fatalf("execute coverage json: %v", err)
	}
	var decoded struct {
		Scope     string `json:"scope"`
		Source    string `json:"source"`
		SessionID string `json:"sessionId"`
		Overall   struct {
			ToolOutputs  int `json:"toolOutputs"`
			ContentBytes struct {
				Total   int64 `json:"total"`
				Samples int   `json:"samples"`
			} `json:"contentBytes"`
			WithContentBytes struct {
				Count int      `json:"count"`
				Total int      `json:"total"`
				Value *float64 `json:"value"`
			} `json:"withContentBytes"`
			EstimatedTokens struct {
				Value int64  `json:"value"`
				Kind  string `json:"kind"`
			} `json:"estimatedTokens"`
		} `json:"overall"`
		Gateway struct {
			Supported bool   `json:"supported"`
			Reason    string `json:"reason"`
		} `json:"gateway"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, got)
	}
	if decoded.Scope != "session" || decoded.Source != "codex" || decoded.SessionID != "sess-1" {
		t.Fatalf("scope/source/session = %q/%q/%q, want session/codex/sess-1", decoded.Scope, decoded.Source, decoded.SessionID)
	}
	if decoded.Overall.ToolOutputs != 1 || decoded.Overall.ContentBytes.Total != 16 {
		t.Fatalf("counts = %+v, want 1 tool output over 16 bytes", decoded.Overall)
	}
	if decoded.Overall.WithContentBytes.Value == nil || *decoded.Overall.WithContentBytes.Value != 1 {
		t.Fatalf("with byte size = %+v, want full coverage", decoded.Overall.WithContentBytes)
	}
	if decoded.Overall.EstimatedTokens.Kind != "estimated" || decoded.Overall.EstimatedTokens.Value != 4 {
		t.Fatalf("estimated tokens = %+v, want 4 estimated", decoded.Overall.EstimatedTokens)
	}
	if decoded.Gateway.Supported || decoded.Gateway.Reason == "" {
		t.Fatalf("gateway = %+v, want unsupported with a reason", decoded.Gateway)
	}
	for _, unwanted := range []string{"%", "KiB", "sha256:"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("json contains formatted or private value %q: %s", unwanted, got)
		}
	}
}

func TestCoverageToolOutputSourceAggregatesSessions(t *testing.T) {
	coverageHome(t)

	got, err := runCoverage(t, "--source", "codex")
	if err != nil {
		t.Fatalf("execute coverage: %v", err)
	}
	if !strings.Contains(got, "Tool Output Coverage — codex") {
		t.Fatalf("output = %q, want source title", got)
	}
	lines := coverageLines(got)
	// Every session the adapter reads is scanned, including the no-usage one,
	// so the count covers the source rather than only its usage artifacts.
	if !lines["Sessions scanned: 3"] || !lines["Tool outputs: 1"] {
		t.Fatalf("output = %q, want three sessions over one tool output", got)
	}
	if strings.Contains(got, "session sess-") {
		t.Fatalf("source aggregate must not name a session: %q", got)
	}
}

// A session holding tool output but no provider usage stays in the scan, its
// tool output is counted, and that output is not linked to fresh input it never
// reported.
func TestCoverageToolOutputSourceCountsSessionWithoutUsage(t *testing.T) {
	coverageHomeWithCodexFixtures(t, "basic-session.jsonl", "tool-output-without-usage-session.jsonl")

	got, err := runCoverage(t, "--source", "codex")
	if err != nil {
		t.Fatalf("execute coverage: %v", err)
	}
	lines := coverageLines(got)
	for _, want := range []string{
		"Sessions scanned: 2",
		"Turns scanned: 1",
		"Tool outputs: 2",
		"With call ID: 2 (100.0%)",
		"With tool name: 2 (100.0%)",
		"In trailing context: 1",
		"Linked to fresh input: 1 (50.0%)",
		"Recognized tool outputs:",
		"Complete: 2",
		"counted over 2 recognized tool outputs only",
		"Unreadable records: 0 (0 sessions)",
	} {
		if !lines[want] {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// At the aggregate scope the pooled and per-source summaries both keep the
// session.
func TestCoverageToolOutputAllCountsSessionWithoutUsage(t *testing.T) {
	coverageHomeWithCodexFixtures(t, "basic-session.jsonl", "tool-output-without-usage-session.jsonl")

	got, err := runCoverage(t, "--all")
	if err != nil {
		t.Fatalf("execute coverage all: %v", err)
	}
	if !strings.Contains(got, "Tool Output Coverage — all sources") {
		t.Fatalf("output = %q, want aggregate title", got)
	}
	lines := coverageLines(got)
	for _, want := range []string{
		"Sessions scanned: 2",
		"Tool outputs: 2",
		"Linked to fresh input: 1 (50.0%)",
		"Recognized tool outputs:",
	} {
		if !lines[want] {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"coverage", "tool-output", "--all", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute coverage all json: %v", err)
	}
	var decoded struct {
		Overall struct {
			Sessions    int `json:"sessions"`
			Turns       int `json:"turns"`
			ToolOutputs int `json:"toolOutputs"`
			WithFresh   struct {
				Count int `json:"count"`
				Total int `json:"total"`
			} `json:"withFreshInput"`
			Completeness struct {
				Complete int `json:"complete"`
			} `json:"recognizedToolOutputCompleteness"`
			UnreadableRecords int `json:"unreadableRecords"`
		} `json:"overall"`
		Sources []struct {
			Source  string `json:"source"`
			Summary struct {
				Sessions    int `json:"sessions"`
				ToolOutputs int `json:"toolOutputs"`
			} `json:"summary"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if decoded.Overall.Sessions != 2 || decoded.Overall.ToolOutputs != 2 {
		t.Fatalf("overall = %+v, want the session without usage counted", decoded.Overall)
	}
	if decoded.Overall.Completeness.Complete != 2 {
		t.Fatalf("recognized complete = %d, want 2", decoded.Overall.Completeness.Complete)
	}
	if decoded.Overall.WithFresh.Count != 1 || decoded.Overall.WithFresh.Total != 2 {
		t.Fatalf("with fresh input = %+v, want 1 of 2", decoded.Overall.WithFresh)
	}
	var codex *struct {
		Source  string `json:"source"`
		Summary struct {
			Sessions    int `json:"sessions"`
			ToolOutputs int `json:"toolOutputs"`
		} `json:"summary"`
	}
	for i := range decoded.Sources {
		if decoded.Sources[i].Source == "codex" {
			codex = &decoded.Sources[i]
		}
	}
	if codex == nil {
		t.Fatalf("json = %s, want a codex summary", stdout.String())
	}
	if codex.Summary.Sessions != 2 || codex.Summary.ToolOutputs != 2 {
		t.Fatalf("codex summary = %+v, want two sessions over two tool outputs", codex.Summary)
	}
}

func TestCoverageToolOutputAllSourcesReportsUnsupported(t *testing.T) {
	coverageHome(t)

	got, err := runCoverage(t, "--all")
	if err != nil {
		t.Fatalf("execute coverage all: %v", err)
	}
	for _, want := range []string{
		"Tool Output Coverage — all sources",
		"\ncodex\n",
		"\nclaude\n",
		"\nOverall\n",
		"Unsupported sources:",
		"router9: tool output coverage not supported for router9: source exposes no sessions",
		"Gateway coverage: not supported in this iteration",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"coverage", "tool-output", "--all", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute coverage all json: %v", err)
	}
	var decoded struct {
		Scope   string `json:"scope"`
		Overall struct {
			Sessions    int `json:"sessions"`
			ToolOutputs int `json:"toolOutputs"`
		} `json:"overall"`
		Sources []struct {
			Source  string `json:"source"`
			Summary struct {
				Sessions int `json:"sessions"`
			} `json:"summary"`
		} `json:"sources"`
		UnsupportedSources []struct {
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"unsupportedSources"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if decoded.Scope != "all" || len(decoded.Sources) != 2 {
		t.Fatalf("decoded = %+v, want all sources over two own summaries", decoded)
	}
	if decoded.Overall.Sessions < 2 || decoded.Overall.ToolOutputs < 1 {
		t.Fatalf("overall = %+v, want pooled counts", decoded.Overall)
	}
	if len(decoded.UnsupportedSources) != 1 || decoded.UnsupportedSources[0].Source != "router9" {
		t.Fatalf("unsupported = %+v, want router9 listed with a reason", decoded.UnsupportedSources)
	}
}

func TestCoverageToolOutputNeverLeaksSessionContent(t *testing.T) {
	coverageHome(t)

	terminal, err := runCoverage(t, "sess-1")
	if err != nil {
		t.Fatalf("execute coverage: %v", err)
	}
	jsonOut, err := runCoverage(t, "sess-1", "--format", "json")
	if err != nil {
		t.Fatalf("execute coverage json: %v", err)
	}
	for name, out := range map[string]string{"terminal": terminal, "json": jsonOut} {
		for _, unwanted := range []string{"synthetic output", "Inspect the fixture.", "AGENTS.md", "sha256:", "codex_rollout"} {
			if strings.Contains(out, unwanted) {
				t.Fatalf("%s output leaks %q: %s", name, unwanted, out)
			}
		}
	}
}

func TestCoverageToolOutputAppearsInHelp(t *testing.T) {
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "coverage") {
		t.Fatalf("help = %q, want coverage subcommand listed", got)
	}
}

func coverageLines(out string) map[string]bool {
	lines := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		lines[strings.Join(strings.Fields(line), " ")] = true
	}
	return lines
}

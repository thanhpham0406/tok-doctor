package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/app"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func runInspectCLI(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func fixtureSessions(t *testing.T) []model.Session {
	t.Helper()
	result, err := app.New().SessionsAll(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(result.Sessions) == 0 {
		t.Fatal("fixture home lists no sessions")
	}
	return result.Sessions
}

func fixtureSessionIndex(t *testing.T, id string) int {
	t.Helper()
	for index, session := range fixtureSessions(t) {
		if session.ID == id {
			return index + 1
		}
	}
	t.Fatalf("session %q not listed by fixture home", id)
	return 0
}

func pickerListing(t *testing.T) string {
	t.Helper()
	out, err := runInspectCLI(t, "q\n", "inspect")
	if err != nil && err != errInspectCancelled {
		t.Fatalf("execute inspect: %v", err)
	}
	return out
}

func sessionHeader(id string) string {
	return "Session " + id[:min(12, len(id))]
}

func pickerPrelude(t *testing.T, out string, sessions int) string {
	t.Helper()
	marker := fmt.Sprintf("Choose [1-%d]:", sessions)
	index := strings.Index(out, marker)
	if index < 0 {
		t.Fatalf("output = %q, want picker prompt %q", out, marker)
	}
	return out[:index]
}

func pickerRows(t *testing.T, prelude string) []string {
	t.Helper()
	rows := []string{}
	for _, line := range strings.Split(prelude, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "Select a session") {
			continue
		}
		rows = append(rows, trimmed)
	}
	return rows
}

func TestInspectCommandPickerListsSessionsInSelectableOrder(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	sessions := fixtureSessions(t)
	prelude := pickerPrelude(t, pickerListing(t), len(sessions))
	if !strings.Contains(prelude, "Select a session (or q to quit):") {
		t.Fatalf("prelude = %q, want picker header", prelude)
	}
	rows := pickerRows(t, prelude)
	if len(rows) != len(sessions) {
		t.Fatalf("picker rows = %d, want %d: %q", len(rows), len(sessions), prelude)
	}
	for index, session := range sessions {
		row := rows[index]
		if !strings.HasPrefix(row, fmt.Sprintf("%d.", index+1)) {
			t.Fatalf("row %d = %q, want number %d", index, row, index+1)
		}
		for _, want := range []string{pickerSource(session), pickerUpdated(session), pickerModel(session)} {
			if !strings.Contains(row, want) {
				t.Fatalf("row %d = %q, want %q", index, row, want)
			}
		}
	}
	if !strings.Contains(prelude, "tokens") {
		t.Fatalf("prelude = %q, want token totals", prelude)
	}
}

func TestInspectCommandPickerOrderIsDeterministic(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	sessions := fixtureSessions(t)
	firstPrelude := pickerPrelude(t, pickerListing(t), len(sessions))
	secondPrelude := pickerPrelude(t, pickerListing(t), len(sessions))
	if firstPrelude != secondPrelude {
		t.Fatalf("picker order changed between runs:\n%s\n%s", firstPrelude, secondPrelude)
	}
	if rows := pickerRows(t, firstPrelude); len(rows) != len(sessions) {
		t.Fatalf("picker rows = %d, want %d stable rows", len(rows), len(sessions))
	}
}

func TestInspectCommandPickerRendersChosenSessionAndReconciliation(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	sessions := fixtureSessions(t)
	out, err := runInspectCLI(t, "1\n", "inspect")
	if err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	chosen := sessions[0]
	if !strings.Contains(out, sessionHeader(chosen.ID)) {
		t.Fatalf("output = %q, want chosen session %q", out, chosen.ID)
	}
	if !strings.Contains(out, "Reconciliation") {
		t.Fatalf("output = %q, want reconciliation section", out)
	}
	if strings.Contains(out, sessionHeader(sessions[1].ID)) {
		t.Fatalf("output = %q, rendered an unselected session", out)
	}
}

func TestInspectCommandPickerMatchesDirectInspect(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	index := fixtureSessionIndex(t, "sess-1")
	for _, flags := range [][]string{
		{},
		{"--turn", "1", "--context"},
		{"--all-turns"},
	} {
		picked, err := runInspectCLI(t, fmt.Sprint(index)+"\n", append([]string{"inspect"}, flags...)...)
		if err != nil {
			t.Fatalf("execute picker inspect %v: %v", flags, err)
		}
		direct, err := runInspectCLI(t, "", append([]string{"inspect", "sess-1"}, flags...)...)
		if err != nil {
			t.Fatalf("execute direct inspect %v: %v", flags, err)
		}
		if !strings.Contains(picked, direct) {
			t.Fatalf("picker output for %v does not render the direct inspect output:\n%s\n---\n%s", flags, picked, direct)
		}
	}
}

func TestInspectCommandPickerAppliesFlags(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	index := fixtureSessionIndex(t, "sess-2")
	out, err := runInspectCLI(t, fmt.Sprint(index)+"\n", "inspect", "--turn", "2", "--evidence")
	if err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	for _, want := range []string{"Evidence", "cumulative delta", "codex_rollout"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, want %q", out, want)
		}
	}
}

func TestInspectCommandPickerRetriesInvalidInput(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	sessions := fixtureSessions(t)
	index := fixtureSessionIndex(t, "sess-1")
	prompt := fmt.Sprintf("Choose [1-%d]:", len(sessions))
	out, err := runInspectCLI(t, "abc\n0\n"+fmt.Sprint(len(sessions)+1)+"\n"+fmt.Sprint(index)+"\n", "inspect")
	if err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	invalid := fmt.Sprintf("Enter a number between 1 and %d, or q to quit.", len(sessions))
	if got := strings.Count(out, invalid); got != 3 {
		t.Fatalf("invalid-input messages = %d, want 3: %q", got, out)
	}
	if got := strings.Count(out, prompt); got != 4 {
		t.Fatalf("picker prompts = %d, want 4: %q", got, out)
	}
	if !strings.Contains(out, "Session sess-1") {
		t.Fatalf("output = %q, want session rendered after retries", out)
	}
}

func TestInspectCommandPickerQuitCancels(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	for _, input := range []string{"q\n", "Q\n", "quit\n", "QUIT\n"} {
		out, err := runInspectCLI(t, input, "inspect")
		if err == nil {
			t.Fatalf("input %q: expected cancellation error", input)
		}
		if err != errInspectCancelled {
			t.Fatalf("input %q: error = %v, want cancellation", input, err)
		}
		if strings.Contains(out, "Session ") || strings.Contains(out, "Reconciliation") {
			t.Fatalf("input %q: cancelled picker rendered a session: %q", input, out)
		}
	}
}

func TestInspectCommandPickerStopsOnClosedInput(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	out, err := runInspectCLI(t, "", "inspect")
	if err == nil {
		t.Fatal("expected error for closed stdin")
	}
	if !strings.Contains(err.Error(), "input closed") {
		t.Fatalf("error = %q, want closed-input message", err.Error())
	}
	if strings.Contains(out, "Session ") {
		t.Fatalf("output = %q, cancelled picker rendered a session", out)
	}
}

func TestInspectCommandPickerAcceptsSelectionWithoutTrailingNewline(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	out, err := runInspectCLI(t, "1", "inspect")
	if err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	if !strings.Contains(out, sessionHeader(fixtureSessions(t)[0].ID)) {
		t.Fatalf("output = %q, want first session rendered", out)
	}
}

func TestInspectCommandPickerRequiresSessions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	out, err := runInspectCLI(t, "1\n", "inspect")
	if err == nil {
		t.Fatal("expected error when no sessions are available")
	}
	if !strings.Contains(err.Error(), "no sessions available") {
		t.Fatalf("error = %q, want empty listing message", err.Error())
	}
	if strings.Contains(out, "Choose [") {
		t.Fatalf("output = %q, want no picker prompt", out)
	}
}

func TestInspectCommandJSONWithoutSessionIDRequiresID(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	out, err := runInspectCLI(t, "1\n", "inspect", "--format", "json")
	if err == nil {
		t.Fatal("expected error for json without session id")
	}
	if !strings.Contains(err.Error(), "session-id is required") {
		t.Fatalf("error = %q, want required session-id message", err.Error())
	}
	if out != "" {
		t.Fatalf("output = %q, want no picker output", out)
	}
}

func TestInspectCommandJSONWithSessionIDUnchanged(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	out, err := runInspectCLI(t, "", "inspect", "sess-1", "--format", "json")
	if err != nil {
		t.Fatalf("execute inspect json: %v", err)
	}
	var decoded struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out)
	}
	if decoded.Session.ID != "sess-1" {
		t.Fatalf("session.id = %q, want sess-1", decoded.Session.ID)
	}
}

func TestInspectCommandHelpShowsOptionalSessionID(t *testing.T) {
	out, err := runInspectCLI(t, "", "inspect", "--help")
	if err != nil {
		t.Fatalf("execute inspect help: %v", err)
	}
	for _, want := range []string{"inspect [session-id]", "tok inspect", "--all-turns", "--context-all", "--evidence"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help = %q, want %q", out, want)
		}
	}
}

func TestInspectCommandPickerDoesNotLeakPrivateContent(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	home := os.Getenv("HOME")
	sessions := fixtureSessions(t)
	prelude := pickerPrelude(t, pickerListing(t), len(sessions))
	if len(pickerRows(t, prelude)) == 0 {
		t.Fatal("picker rendered no listing")
	}
	for _, marker := range []string{
		home,
		".codex",
		".claude",
		"Follow local instructions.",
		"Inspect the fixture.",
		"Bearer ",
		"sk-",
		"api_key",
		"Authorization",
		"SECRET_",
	} {
		if strings.Contains(prelude, marker) {
			t.Fatalf("picker listing leaks %q: %q", marker, prelude)
		}
	}
}

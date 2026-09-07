package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/config"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestSetSourceValidatesKind(t *testing.T) {
	app := NewWithStore(config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml")))

	err := app.SetSource("codex", source.Override{Endpoint: "http://127.0.0.1:20128"})
	if err == nil {
		t.Fatal("SetSource accepted endpoint for path source")
	}
}

func TestSourceResolutionPrecedence(t *testing.T) {
	dirConfig := t.TempDir()
	dirCLI := t.TempDir()
	if err := touch(filepath.Join(dirConfig, "config.jsonl")); err != nil {
		t.Fatalf("touch config: %v", err)
	}
	if err := touch(filepath.Join(dirCLI, "cli.jsonl")); err != nil {
		t.Fatalf("touch cli: %v", err)
	}

	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml"))
	app := NewWithStore(store)
	if err := app.SetSource("codex", source.Override{Path: dirConfig}); err != nil {
		t.Fatalf("SetSource: %v", err)
	}

	result, err := app.ShowSource(context.Background(), "codex", source.Override{Path: dirCLI})
	if err != nil {
		t.Fatalf("ShowSource: %v", err)
	}
	if result.Origin != source.OriginCLI {
		t.Fatalf("Origin = %q, want cli", result.Origin)
	}
	if result.Location != dirCLI {
		t.Fatalf("Location = %q, want CLI path", result.Location)
	}
}

func TestResetSourceFallsBackToAuto(t *testing.T) {
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml"))
	app := NewWithStore(store)
	if err := app.SetSource("codex", source.Override{Path: t.TempDir()}); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	if err := app.ResetSource("codex"); err != nil {
		t.Fatalf("ResetSource: %v", err)
	}

	result, err := app.ShowSource(context.Background(), "codex", source.Override{})
	if err != nil {
		t.Fatalf("ShowSource: %v", err)
	}
	if result.Origin != source.OriginAuto {
		t.Fatalf("Origin = %q, want auto", result.Origin)
	}
}

func touch(path string) error {
	return os.WriteFile(path, []byte("{}\n"), 0o600)
}

func TestUsageUnknownSource(t *testing.T) {
	app := NewWithStore(config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml")))
	_, err := app.Usage(context.Background(), "nosuch")
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestUsageUnsupportedRouter9(t *testing.T) {
	app := NewWithStore(config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml")))
	_, err := app.Usage(context.Background(), "router9")
	if err == nil {
		t.Fatal("expected error for router9")
	}
	if err.Error() != "usage not supported for router9" {
		t.Fatalf("error = %q, want router9 unsupported", err.Error())
	}
}

func TestUsageAllIncludesUsageCapableSources(t *testing.T) {
	app := NewWithStore(config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml")))

	result, err := app.UsageAll(context.Background())
	if err != nil {
		t.Fatalf("UsageAll: %v", err)
	}

	var names []string
	for _, entry := range result.Sources {
		names = append(names, entry.Source)
	}
	if !slices.Contains(names, "codex") || !slices.Contains(names, "claude") {
		t.Fatalf("sources = %v, want codex and claude", names)
	}
}

func TestSessionsUnsupportedRouter9(t *testing.T) {
	app := NewWithStore(config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml")))
	_, err := app.Sessions(context.Background(), "router9")
	if err == nil {
		t.Fatal("expected error for router9")
	}
	if err.Error() != "sessions not supported for router9" {
		t.Fatalf("error = %q, want router9 unsupported", err.Error())
	}
}

func TestSessionsAllIncludesSessionCapableSources(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir codex: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}
	copyTestFixture(t, filepath.Join("..", "..", "fixtures", "codex", "basic-session.jsonl"),
		filepath.Join(home, ".codex", "sessions", "codex.jsonl"))
	copyTestFixture(t, filepath.Join("..", "..", "fixtures", "claude", "basic-session.jsonl"),
		filepath.Join(home, ".claude", "projects", "claude.jsonl"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	app := NewWithStore(config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml")))

	result, err := app.SessionsAll(context.Background())
	if err != nil {
		t.Fatalf("SessionsAll: %v", err)
	}

	seen := map[string]bool{}
	for _, session := range result.Sessions {
		seen[session.Source] = true
	}
	if !seen["codex"] || !seen["claude"] {
		t.Fatalf("session sources = %v, want codex and claude", seen)
	}
	if seen["router9"] {
		t.Fatalf("session sources = %v, did not expect router9", seen)
	}
}

func copyTestFixture(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", dst, err)
	}
}

func TestInspectResolvesByFullID(t *testing.T) {
	sessions := []model.Session{
		{ID: "abc123full", Source: "codex"},
		{ID: "xyz789full", Source: "claude"},
	}
	got, err := pickSessionByID(sessions, "abc123full")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.ID != "abc123full" {
		t.Fatalf("session ID = %q, want abc123full", got.ID)
	}
}

func TestInspectResolvesByUniqueShortPrefix(t *testing.T) {
	sessions := []model.Session{
		{ID: "abc123full-aaaa", Source: "codex"},
		{ID: "xyz789full-bbbb", Source: "claude"},
	}
	got, err := pickSessionByID(sessions, "abc123full")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.ID != "abc123full-aaaa" {
		t.Fatalf("session ID = %q, want abc123full-aaaa", got.ID)
	}
}

func TestInspectUnknownReturnsNotFound(t *testing.T) {
	sessions := []model.Session{
		{ID: "abc"},
		{ID: "def"},
	}
	_, err := pickSessionByID(sessions, "zzzz")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInspectSessionNotFound) {
		t.Fatalf("error = %v, want ErrInspectSessionNotFound", err)
	}
}

func TestInspectAmbiguousPrefixReturnsError(t *testing.T) {
	sessions := []model.Session{
		{ID: "abc123"},
		{ID: "abc456"},
	}
	_, err := pickSessionByID(sessions, "abc")
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	if !errors.Is(err, ErrInspectAmbiguousSession) {
		t.Fatalf("error = %v, want ErrInspectAmbiguousSession", err)
	}
}

func TestInspectEmptyIDReturnsNotFound(t *testing.T) {
	sessions := []model.Session{{ID: "abc"}}
	_, err := pickSessionByID(sessions, "")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
	if !errors.Is(err, ErrInspectSessionNotFound) {
		t.Fatalf("error = %v, want ErrInspectSessionNotFound", err)
	}
}

func TestInspectFullIDWinsOverPrefix(t *testing.T) {
	sessions := []model.Session{
		{ID: "abc123"},
		{ID: "abc456"},
	}
	got, err := pickSessionByID(sessions, "abc123")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.ID != "abc123" {
		t.Fatalf("session ID = %q, want abc123 (exact match)", got.ID)
	}
}

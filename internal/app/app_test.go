package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/config"
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

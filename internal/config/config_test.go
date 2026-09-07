package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLoadResetSource(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "config.toml"))

	if err := store.SetSource("codex", Source{Path: "/tmp/codex"}); err != nil {
		t.Fatalf("SetSource codex: %v", err)
	}
	if err := store.SetSource("cursor", Source{Path: "/tmp/cursor"}); err != nil {
		t.Fatalf("SetSource cursor: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sources["codex"].Path != "/tmp/codex" {
		t.Fatalf("codex path = %q", cfg.Sources["codex"].Path)
	}

	if err := store.ResetSource("codex"); err != nil {
		t.Fatalf("ResetSource: %v", err)
	}

	cfg, err = store.Load()
	if err != nil {
		t.Fatalf("Load after reset: %v", err)
	}
	if _, ok := cfg.Sources["codex"]; ok {
		t.Fatal("codex config was not reset")
	}
	if cfg.Sources["cursor"].Path != "/tmp/cursor" {
		t.Fatal("reset removed unrelated source config")
	}
}

func TestLoadQuotedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := `[sources.codex]
path = "/tmp/codex sessions"

[sources.9router]
endpoint = "http://127.0.0.1:30128"
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := NewStoreAt(path).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sources["codex"].Path != "/tmp/codex sessions" {
		t.Fatalf("codex path = %q", cfg.Sources["codex"].Path)
	}
}

func TestSaveWritesTOMLShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store := NewStoreAt(path)

	if err := store.Save(Config{Sources: map[string]Source{
		"codex": {Path: `/tmp/codex"sessions`},
	}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `[sources.codex]`) {
		t.Fatalf("config = %q, want source section", data)
	}
	if !strings.Contains(string(data), `path = "/tmp/codex\"sessions"`) {
		t.Fatalf("config = %q, want escaped path", data)
	}
}

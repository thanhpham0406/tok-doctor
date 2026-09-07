package catalog

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/config"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestAllSourcesRegistered(t *testing.T) {
	result := Builtins().All(context.Background(), config.Config{Sources: map[string]config.Source{}})

	if len(result.Sources) != 3 {
		t.Fatalf("sources len = %d, want 3", len(result.Sources))
	}
	names := Builtins().Names()
	for _, want := range []string{"codex", "claude", "router9"} {
		if !slices.Contains(names, want) {
			t.Fatalf("source names = %v, want %s", names, want)
		}
	}
}

func TestConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfg := config.Config{Sources: map[string]config.Source{
		"codex": {Path: dir},
	}}
	result, err := Builtins().Show(context.Background(), cfg, "codex", source.Override{})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if result.Origin != source.OriginConfig {
		t.Fatalf("Origin = %q, want config", result.Origin)
	}
}

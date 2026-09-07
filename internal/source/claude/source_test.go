package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestConfiguredPathWithSessionDataIsReady(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}

	result := New().Detect(context.Background(), source.Override{
		Path:   dir,
		Origin: source.OriginConfig,
	})

	if result.Status != source.StatusReady {
		t.Fatalf("Status = %q, want ready", result.Status)
	}
}

func TestConfiguredPathWithoutSessionDataIsInstalled(t *testing.T) {
	result := New().Detect(context.Background(), source.Override{
		Path:   t.TempDir(),
		Origin: source.OriginConfig,
	})

	if result.Status != source.StatusInstalled {
		t.Fatalf("Status = %q, want installed", result.Status)
	}
}

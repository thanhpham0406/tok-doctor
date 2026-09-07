package cursor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestConfiguredPathIsInstalled(t *testing.T) {
	result := New().Detect(context.Background(), source.Override{
		Path:   t.TempDir(),
		Origin: source.OriginConfig,
	})

	if result.Status != source.StatusInstalled {
		t.Fatalf("Status = %q, want installed", result.Status)
	}
}

func TestMissingConfiguredPathIsBroken(t *testing.T) {
	result := New().Detect(context.Background(), source.Override{
		Path:   t.TempDir() + "/missing",
		Origin: source.OriginConfig,
	})

	if result.Status != source.StatusBroken {
		t.Fatalf("Status = %q, want broken", result.Status)
	}
}

func TestConfiguredPathReadyWhenStorageContainsVscdb(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "state.vscdb"), []byte("sqlite"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := New().Detect(context.Background(), source.Override{
		Path:   dir,
		Origin: source.OriginCLI,
	})

	if result.Status != source.StatusReady {
		t.Fatalf("Status = %q, want ready", result.Status)
	}
}

func TestConfiguredPathInstalledWhenNoSupportedData(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("no supported data"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := New().Detect(context.Background(), source.Override{
		Path:   dir,
		Origin: source.OriginCLI,
	})

	if result.Status != source.StatusInstalled {
		t.Fatalf("Status = %q, want installed", result.Status)
	}
}

func TestHasCursorSupportedDataDetectsNestedVscdb(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "subdir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "state.vscdb"), []byte("sqlite"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if !hasCursorSupportedData(root) {
		t.Fatal("expected supported data detection to find nested .vscdb")
	}
}

func TestHasCursorSupportedDataIgnoresUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if hasCursorSupportedData(root) {
		t.Fatal("expected supported data detection to ignore .json files")
	}
}
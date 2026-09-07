package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoPathSourceInstalledFromAppEvidence(t *testing.T) {
	appDir := t.TempDir()

	result := DetectAutoPathSource("example", "Example", []PathCandidate{
		{Kind: "filesystem", Path: appDir},
	}, nil, nil, nil, func(path string) bool {
		return false
	})

	if result.Status != StatusInstalled {
		t.Fatalf("Status = %q, want installed", result.Status)
	}
	if result.Origin != OriginAuto {
		t.Fatalf("Origin = %q, want auto", result.Origin)
	}
}

func TestAutoPathSourceReadyFromSupportedData(t *testing.T) {
	sessionDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sessionDir, "session.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}

	result := DetectAutoPathSource("example", "Example", nil, []PathCandidate{
		{Kind: "filesystem", Path: sessionDir},
	}, nil, nil, func(path string) bool {
		return HasFileWithSuffix(path, ".jsonl", 1)
	})

	if result.Status != StatusReady {
		t.Fatalf("Status = %q, want ready", result.Status)
	}
	if result.Location != sessionDir {
		t.Fatalf("Location = %q, want session dir", result.Location)
	}
}

func TestAutoPathSourceUnavailableWithoutEvidence(t *testing.T) {
	result := DetectAutoPathSource("example", "Example", []PathCandidate{
		{Kind: "filesystem", Path: filepath.Join(t.TempDir(), "missing")},
	}, nil, nil, nil, func(path string) bool {
		return false
	})

	if result.Status != StatusUnavailable {
		t.Fatalf("Status = %q, want unavailable", result.Status)
	}
	if result.Origin != OriginNone {
		t.Fatalf("Origin = %q, want none", result.Origin)
	}
}

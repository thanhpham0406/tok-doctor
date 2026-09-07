package codex

import (
	"context"
	"os"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestDetectMissingSource(t *testing.T) {
	result := source.DetectAutoPathSource("codex", "Codex", nil, []source.PathCandidate{
		{Kind: "filesystem", Path: t.TempDir() + "/missing"},
	}, nil, nil, hasCodexSessionData)
	if result.Status != source.StatusUnavailable {
		t.Fatalf("Status = %q, want unavailable", result.Status)
	}
}

func TestDetectMissingConfiguredSource(t *testing.T) {
	result := New().Detect(context.Background(), source.Override{
		Path:   t.TempDir() + "/configured-missing",
		Origin: source.OriginConfig,
	})
	if result.Status != source.StatusBroken {
		t.Fatalf("Status = %q, want broken", result.Status)
	}
}

func TestDetectReadySource(t *testing.T) {
	dir := t.TempDir()
	touch := dir + "/session.jsonl"
	if err := os.WriteFile(touch, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := New().Detect(context.Background(), source.Override{
		Path:   dir,
		Origin: source.OriginCLI,
	})

	if result.Status != source.StatusReady {
		t.Fatalf("Status = %q, want ready", result.Status)
	}
	if result.Origin != source.OriginCLI {
		t.Fatalf("Origin = %q, want cli", result.Origin)
	}
}

func TestEmptySession(t *testing.T) {
	session := New().EmptySession(context.Background())

	if session.Agent != "codex" {
		t.Fatalf("Agent = %q, want codex", session.Agent)
	}
	if session.ID == "" {
		t.Fatal("ID is empty")
	}
}

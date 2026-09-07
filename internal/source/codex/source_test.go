package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
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

func withFixtureHome(t *testing.T, names ...string) {
	t.Helper()
	home := t.TempDir()
	sessions := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	for _, name := range names {
		src := filepath.Join("..", "..", "..", "fixtures", "codex", name)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		dst := filepath.Join(sessions, name)
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			t.Fatalf("copy fixture %s: %v", name, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestSourceUsageAggregatesAcrossFixtures(t *testing.T) {
	withFixtureHome(t, "basic-session.jsonl", "multi-snapshot-session.jsonl")

	src := New()
	refs, err := src.Sessions(context.Background())
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("expected sessions to be discovered")
	}

	usage, err := src.Usage(context.Background(), refs)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}

	wantInput := int64(3600)
	wantCached := int64(1100)
	wantOutput := int64(1550)
	wantReasoning := int64(150)
	wantTotal := int64(6400)
	if usage.Input != wantInput || usage.Cached != wantCached || usage.Output != wantOutput ||
		usage.Total != wantTotal {
		t.Fatalf("usage = %+v, want input=%d cached=%d output=%d total=%d",
			usage, wantInput, wantCached, wantOutput, wantTotal)
	}
	if usage.Reasoning == nil || *usage.Reasoning != wantReasoning {
		t.Fatalf("reasoning = %v, want pointer to %d", usage.Reasoning, wantReasoning)
	}
	if usage.Confidence != model.ConfidenceMeasured {
		t.Fatalf("confidence = %q, want measured", usage.Confidence)
	}
}

func TestSourceUsageNoDataIsZero(t *testing.T) {
	withFixtureHome(t, "no-usage-session.jsonl")

	src := New()
	refs, err := src.Sessions(context.Background())
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}

	usage, err := src.Usage(context.Background(), refs)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.Confidence != "" {
		t.Fatalf("confidence = %q, want empty", usage.Confidence)
	}
	if usage.Total != 0 {
		t.Fatalf("total = %d, want 0", usage.Total)
	}
}

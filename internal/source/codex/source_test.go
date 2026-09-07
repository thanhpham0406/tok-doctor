package codex

import (
	"context"
	"testing"
)

func TestDetectMissingSource(t *testing.T) {
	source := &Source{sessionDir: t.TempDir() + "/missing"}

	if source.Detect(context.Background()) {
		t.Fatal("Detect returned true for a missing source directory")
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

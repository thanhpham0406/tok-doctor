package codex

import (
	"context"
	"os"
	"path/filepath"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type Source struct {
	sessionDir string
}

func New() *Source {
	return &Source{sessionDir: defaultSessionDir()}
}

func (s *Source) Name() string {
	return string(model.AgentCodex)
}

func (s *Source) Detect(ctx context.Context) bool {
	_ = ctx

	info, err := os.Stat(s.sessionDir)
	return err == nil && info.IsDir()
}

func (s *Source) EmptySession(ctx context.Context) model.Session {
	_ = ctx

	return model.Session{
		ID:    "scaffold",
		Agent: model.AgentCodex,
	}
}

func defaultSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".codex", "sessions")
}

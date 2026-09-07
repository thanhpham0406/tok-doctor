package codex

import (
	"os"
	"path/filepath"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func installPaths() []source.PathCandidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []source.PathCandidate{{Kind: "filesystem", Path: filepath.Join(home, ".codex")}}
}

func sessionPaths() []source.PathCandidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []source.PathCandidate{{Kind: "filesystem", Path: filepath.Join(home, ".codex", "sessions")}}
}

func executableNames() []string {
	return []string{"codex"}
}

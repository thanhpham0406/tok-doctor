package antigravity

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
	return []source.PathCandidate{
		{Kind: "filesystem", Path: filepath.Join(home, ".config", "Antigravity")},
		{Kind: "filesystem", Path: filepath.Join(home, ".antigravity")},
	}
}

func executableNames() []string {
	return []string{"antigravity"}
}

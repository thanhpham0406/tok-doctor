package kiro

import (
	"os"
	"path/filepath"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func installPaths() []source.PathCandidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return []source.PathCandidate{{Kind: "application", Path: "/Applications/Kiro.app"}}
	}
	return []source.PathCandidate{
		{Kind: "application", Path: "/Applications/Kiro.app"},
		{Kind: "filesystem", Path: filepath.Join(home, "Library", "Application Support", "Kiro")},
		{Kind: "filesystem", Path: filepath.Join(home, ".kiro")},
	}
}

func executableNames() []string {
	return []string{"kiro"}
}

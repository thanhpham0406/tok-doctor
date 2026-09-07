package antigravity

import (
	"os"
	"path/filepath"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func installPaths() []source.PathCandidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return []source.PathCandidate{{Kind: "application", Path: "/Applications/Antigravity.app"}}
	}
	return []source.PathCandidate{
		{Kind: "application", Path: "/Applications/Antigravity.app"},
		{Kind: "application", Path: "/Applications/Google Antigravity.app"},
		{Kind: "filesystem", Path: filepath.Join(home, "Library", "Application Support", "Antigravity")},
		{Kind: "filesystem", Path: filepath.Join(home, ".antigravity")},
	}
}

func executableNames() []string {
	return []string{"antigravity"}
}

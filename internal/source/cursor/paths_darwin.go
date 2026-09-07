package cursor

import (
	"os"
	"path/filepath"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func installPaths() []source.PathCandidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return []source.PathCandidate{{Kind: "application", Path: "/Applications/Cursor.app"}}
	}
	return []source.PathCandidate{
		{Kind: "application", Path: "/Applications/Cursor.app"},
		{Kind: "filesystem", Path: filepath.Join(home, "Library", "Application Support", "Cursor")},
	}
}

func readyPaths() []source.PathCandidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []source.PathCandidate{
		{Kind: "filesystem", Path: filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage")},
	}
}

func executableNames() []string {
	return []string{"cursor"}
}

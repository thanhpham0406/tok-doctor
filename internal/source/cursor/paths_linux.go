package cursor

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
		{Kind: "filesystem", Path: filepath.Join(home, ".config", "Cursor")},
		{Kind: "filesystem", Path: filepath.Join(home, ".cursor")},
	}
}

func readyPaths() []source.PathCandidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []source.PathCandidate{
		{Kind: "filesystem", Path: filepath.Join(home, ".config", "Cursor", "User", "globalStorage")},
		{Kind: "filesystem", Path: filepath.Join(home, ".cursor", "User", "globalStorage")},
	}
}

func executableNames() []string {
	return []string{"cursor"}
}
package router9

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
		{Kind: "filesystem", Path: filepath.Join(home, ".9router")},
		{Kind: "filesystem", Path: filepath.Join(home, ".config", "9router")},
	}
}

func executableNames() []string {
	return []string{"9router", "router9"}
}

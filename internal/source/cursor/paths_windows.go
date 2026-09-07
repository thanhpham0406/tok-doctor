package cursor

import (
	"os"
	"path/filepath"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func installPaths() []source.PathCandidate {
	var paths []source.PathCandidate
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths, source.PathCandidate{Kind: "filesystem", Path: filepath.Join(appData, "Cursor")})
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		paths = append(paths, source.PathCandidate{Kind: "application", Path: filepath.Join(localAppData, "Programs", "Cursor")})
	}
	return paths
}

func readyPaths() []source.PathCandidate {
	var paths []source.PathCandidate
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths, source.PathCandidate{
			Kind: "filesystem",
			Path: filepath.Join(appData, "Cursor", "User", "globalStorage"),
		})
	}
	return paths
}

func executableNames() []string {
	return []string{"cursor.exe", "cursor"}
}

package antigravity

import (
	"os"
	"path/filepath"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func installPaths() []source.PathCandidate {
	var paths []source.PathCandidate
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths, source.PathCandidate{Kind: "filesystem", Path: filepath.Join(appData, "Antigravity")})
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		paths = append(paths, source.PathCandidate{Kind: "application", Path: filepath.Join(localAppData, "Programs", "Antigravity")})
		paths = append(paths, source.PathCandidate{Kind: "application", Path: filepath.Join(localAppData, "Programs", "Google Antigravity")})
	}
	return paths
}

func executableNames() []string {
	return []string{"antigravity.exe", "antigravity"}
}

package router9

import (
	"os"
	"path/filepath"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func installPaths() []source.PathCandidate {
	var paths []source.PathCandidate
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths, source.PathCandidate{Kind: "filesystem", Path: filepath.Join(appData, "9router")})
	}
	return paths
}

func executableNames() []string {
	return []string{"9router.exe", "router9.exe", "9router", "router9"}
}

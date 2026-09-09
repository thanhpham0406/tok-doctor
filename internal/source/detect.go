package source

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func DetectPath(name, displayName, path string, origin Origin, capabilities *Capabilities) Detection {
	result := Detection{
		Name:         name,
		DisplayName:  displayName,
		Origin:       origin,
		Kind:         KindPath,
		Location:     path,
		Capabilities: capabilities,
	}
	if path == "" {
		result.Status = StatusUnavailable
		result.Origin = OriginNone
		result.Reason = "no reliable source location is known"
		return result
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		result.Status = StatusUnavailable
		if origin == OriginCLI || origin == OriginConfig {
			result.Status = StatusBroken
		}
		result.Confidence = ConfidenceHigh
		result.Reason = "source path was not found"
		result.Evidence = []Evidence{{Kind: "filesystem", Value: path, OK: false}}
		return result
	}
	if err != nil {
		result.Status = StatusBroken
		result.Confidence = ConfidenceHigh
		result.Reason = "source path could not be read"
		result.Evidence = []Evidence{{Kind: "filesystem", Value: path, OK: false}}
		return result
	}
	if !info.IsDir() {
		result.Status = StatusBroken
		result.Confidence = ConfidenceHigh
		result.Reason = "source path is not a directory"
		result.Evidence = []Evidence{{Kind: "filesystem", Value: path, OK: false}}
		return result
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		result.Status = StatusBroken
		result.Confidence = ConfidenceHigh
		result.Reason = "source path is not readable"
		result.Evidence = []Evidence{{Kind: "filesystem", Value: path, OK: false}}
		return result
	}

	result.Evidence = []Evidence{{Kind: "filesystem", Value: path, OK: true}}
	if len(entries) == 0 {
		result.Status = StatusInstalled
		result.Confidence = ConfidenceMedium
		result.Reason = "source directory exists but no data was found"
		return result
	}

	result.Status = StatusReady
	result.Confidence = ConfidenceHigh
	return result
}

func DetectConfiguredPath(name, displayName, path string, origin Origin, capabilities *Capabilities, ready func(string) bool) Detection {
	result := DetectPath(name, displayName, path, origin, capabilities)
	if result.Status != StatusReady {
		return result
	}
	if ready(path) {
		return result
	}
	result.Status = StatusInstalled
	result.Confidence = ConfidenceMedium
	result.Reason = "source path exists but supported data was not found"
	return result
}

func DetectConfiguredInstallPath(name, displayName, path string, origin Origin, reason string) Detection {
	result := DetectPath(name, displayName, path, origin, nil)
	if result.Status == StatusReady {
		result.Status = StatusInstalled
		result.Confidence = ConfidenceMedium
		result.Reason = reason
	}
	return result
}

func DetectAutoPathSource(name, displayName string, installPaths, readyPaths []PathCandidate, binaries []string, capabilities *Capabilities, ready func(string) bool) Detection {
	result := Detection{
		Name:         name,
		DisplayName:  displayName,
		Origin:       OriginAuto,
		Kind:         KindPath,
		Capabilities: capabilities,
	}

	var installed []Evidence
	var broken []Evidence
	for _, candidate := range installPaths {
		evidence, state := checkPath(candidate)
		if state == pathMissing {
			continue
		}
		result.Evidence = append(result.Evidence, evidence)
		if state == pathUsable {
			installed = append(installed, evidence)
		} else {
			broken = append(broken, evidence)
		}
	}
	for _, binary := range binaries {
		if path, ok := executablePath(binary); ok {
			evidence := Evidence{Kind: "executable", Value: path, OK: true}
			result.Evidence = append(result.Evidence, evidence)
			installed = append(installed, evidence)
		}
	}

	for _, candidate := range readyPaths {
		evidence, state := checkPath(candidate)
		if state == pathMissing {
			continue
		}
		result.Evidence = append(result.Evidence, evidence)
		result.Location = candidate.Path
		if state != pathUsable {
			result.Status = StatusBroken
			result.Confidence = ConfidenceHigh
			result.Reason = "source data path was found but is not usable"
			return result
		}
		if ready(candidate.Path) {
			result.Status = StatusReady
			result.Confidence = ConfidenceHigh
			return result
		}
		installed = append(installed, evidence)
	}

	if len(installed) > 0 {
		result.Status = StatusInstalled
		result.Confidence = ConfidenceMedium
		result.Reason = "source appears installed but supported data was not found"
		return result
	}
	if len(broken) > 0 {
		result.Status = StatusBroken
		result.Confidence = ConfidenceMedium
		result.Reason = "source evidence was found but could not be read"
		return result
	}

	result.Status = StatusUnavailable
	result.Origin = OriginNone
	result.Reason = "no reliable source evidence was found"
	return result
}

func HasFileWithSuffix(root, suffix string, maxDepth int) bool {
	root = filepath.Clean(root)
	rootDepth := strings.Count(root, string(os.PathSeparator))
	found := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if path != root && entry.IsDir() && strings.Count(filepath.Clean(path), string(os.PathSeparator))-rootDepth > maxDepth {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), suffix) {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func ValidateLocalEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("endpoint must use http or https")
	}
	host := parsed.Hostname()
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("endpoint must use a loopback host")
	}
	return nil
}

func ProbeEndpoint(ctx context.Context, name, displayName, endpoint string, origin Origin) Detection {
	result := Detection{
		Name:        name,
		DisplayName: displayName,
		Origin:      origin,
		Kind:        KindEndpoint,
		Endpoint:    endpoint,
		Evidence:    []Evidence{{Kind: "http", Value: endpoint, OK: false}},
	}
	if endpoint == "" {
		result.Status = StatusUnavailable
		result.Origin = OriginNone
		result.Reason = "no reliable source endpoint is known"
		return result
	}
	if err := ValidateLocalEndpoint(endpoint); err != nil {
		result.Status = StatusBroken
		result.Confidence = ConfidenceHigh
		result.Reason = err.Error()
		return result
	}

	reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		result.Status = StatusBroken
		result.Confidence = ConfidenceHigh
		result.Reason = "endpoint URL is invalid"
		return result
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		result.Status = StatusUnavailable
		result.Confidence = ConfidenceMedium
		result.Reason = "source endpoint did not respond"
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	result.Evidence[0].OK = true
	server := strings.ToLower(resp.Header.Get("Server"))
	sourceHeader := strings.ToLower(resp.Header.Get("X-Source"))
	for _, signature := range []string{strings.ToLower(name), strings.ToLower(displayName)} {
		if strings.Contains(server, signature) || strings.Contains(sourceHeader, signature) {
			result.Status = StatusReady
			result.Confidence = ConfidenceHigh
			return result
		}
	}

	result.Status = StatusInstalled
	result.Confidence = ConfidenceMedium
	result.Reason = "endpoint responded but did not identify itself as this source"
	return result
}

type pathState int

const (
	pathMissing pathState = iota
	pathUsable
	pathBroken
)

func checkPath(candidate PathCandidate) (Evidence, pathState) {
	evidence := Evidence{Kind: candidate.Kind, Value: candidate.Path}
	info, err := os.Stat(candidate.Path)
	if os.IsNotExist(err) {
		return evidence, pathMissing
	}
	if err != nil || !info.IsDir() {
		return evidence, pathBroken
	}
	if _, err := os.ReadDir(candidate.Path); err != nil {
		return evidence, pathBroken
	}
	evidence.OK = true
	return evidence, pathUsable
}

func executablePath(binary string) (string, bool) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", false
	}
	return path, true
}

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

type Manager struct {
	Store   *stateStore
	LogDir  string
	Timeout time.Duration
}

type StartResult struct {
	Profile        string
	Listen         string
	Proxy          string
	PID            int
	LogFile        string
	AlreadyRunning bool
}

func NewManager() (*Manager, error) {
	stateDir, err := DefaultRuntimeDir()
	if err != nil {
		return nil, err
	}
	logDir, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return &Manager{
		Store:   newStateStore(stateDir),
		LogDir:  logDir,
		Timeout: 5 * time.Second,
	}, nil
}

func (m *Manager) Start(ctx context.Context, profiles []Profile) ([]StartResult, error) {
	if len(profiles) == 0 {
		return nil, errors.New("no enabled gateway profiles to start")
	}
	sorted := append([]Profile(nil), profiles...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var out []StartResult
	for _, p := range sorted {
		result, err := m.startOne(ctx, p)
		if err != nil {
			return out, err
		}
		out = append(out, result)
	}
	return out, nil
}

func (m *Manager) startOne(ctx context.Context, p Profile) (StartResult, error) {
	if st, ok, err := m.Store.Read(p.Name); err == nil && ok {
		if controlChecker(st) {
			return StartResult{
				Profile:        p.Name,
				Listen:         st.Listen,
				Proxy:          ProxyURL(st.Listen),
				PID:            st.PID,
				LogFile:        m.logFile(p.Name),
				AlreadyRunning: true,
			}, nil
		}
		m.Store.Remove(p.Name)
	} else if err != nil {
		return StartResult{}, fmt.Errorf("read runtime state for %s: %w", p.Name, err)
	}

	exe, err := os.Executable()
	if err != nil {
		return StartResult{}, fmt.Errorf("find current executable: %w", err)
	}
	if err := os.MkdirAll(m.LogDir, 0o700); err != nil {
		return StartResult{}, fmt.Errorf("create gateway log dir: %w", err)
	}
	logFile := m.logFile(p.Name)
	log, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return StartResult{}, fmt.Errorf("open gateway log %s: %w", logFile, err)
	}
	defer func() { _ = log.Close() }()

	cmd := exec.Command(exe, "gateway", "_serve", "--profile", p.Name)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Env = os.Environ()
	detachCommand(cmd)
	if err := cmd.Start(); err != nil {
		return StartResult{}, fmt.Errorf("start gateway daemon for %s: %w", p.Name, err)
	}
	defer func() { _ = cmd.Process.Release() }()

	result, err := m.waitReady(ctx, p, cmd.Process.Pid, logFile)
	if err != nil {
		return StartResult{}, err
	}
	return result, nil
}

func (m *Manager) waitReady(ctx context.Context, p Profile, pid int, logFile string) (StartResult, error) {
	timeout := m.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return StartResult{}, fmt.Errorf("gateway daemon for %s did not become ready before timeout; see log %s", p.Name, logFile)
		case <-ticker.C:
			st, ok, err := m.Store.Read(p.Name)
			if err != nil {
				return StartResult{}, fmt.Errorf("read runtime state for %s: %w", p.Name, err)
			}
			if !ok {
				continue
			}
			if st.PID != pid {
				if controlChecker(st) {
					return StartResult{Profile: p.Name, Listen: st.Listen, Proxy: ProxyURL(st.Listen), PID: st.PID, LogFile: logFile, AlreadyRunning: true}, nil
				}
				m.Store.Remove(p.Name)
				continue
			}
			if controlChecker(st) {
				return StartResult{Profile: p.Name, Listen: st.Listen, Proxy: ProxyURL(st.Listen), PID: st.PID, LogFile: logFile}, nil
			}
		}
	}
}

func (m *Manager) Stop(ctx context.Context, profiles []Profile) ([]StatusEntry, error) {
	var out []StatusEntry
	for _, p := range profiles {
		entry := StatusEntry{Profile: p.Name, Listen: p.Listen, Proxy: ProxyURL(p.Listen), State: "stopped"}
		st, ok, err := m.Store.Read(p.Name)
		if err != nil {
			return out, fmt.Errorf("read runtime state for %s: %w", p.Name, err)
		}
		if !ok {
			out = append(out, entry)
			continue
		}
		if !controlChecker(st) {
			m.Store.Remove(p.Name)
			out = append(out, entry)
			continue
		}
		if err := requestShutdown(ctx, st); err != nil {
			return out, fmt.Errorf("stop gateway profile %s: %w", p.Name, err)
		}
		if err := m.waitStopped(ctx, st); err != nil {
			return out, fmt.Errorf("wait for gateway profile %s to stop: %w", p.Name, err)
		}
		m.Store.Remove(p.Name)
		out = append(out, entry)
	}
	return out, nil
}

func (m *Manager) waitStopped(ctx context.Context, st RuntimeState) error {
	timeout := m.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !controlChecker(st) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("shutdown timed out")
		case <-ticker.C:
		}
	}
}

func (m *Manager) logFile(profile string) string {
	return filepath.Join(m.LogDir, profile+".log")
}

func requestShutdown(ctx context.Context, st RuntimeState) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+st.ControlAddr+"/shutdown", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-TokDoctor-Control-Token", st.ControlToken)
	resp, err := controlClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control endpoint returned %s", resp.Status)
	}
	return nil
}

func FreeLoopbackAddress() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("find free loopback address: %w", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String(), nil
}

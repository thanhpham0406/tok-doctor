package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type RuntimeState struct {
	Profile      string    `json:"profile"`
	PID          int       `json:"pid"`
	Listen       string    `json:"listen"`
	ControlAddr  string    `json:"controlAddr"`
	ControlToken string    `json:"controlToken"`
	StartedAt    time.Time `json:"startedAt"`
	InstanceID   string    `json:"instanceId"`
}

type stateStore struct {
	dir string
	mu  sync.Mutex
}

func newStateStore(dir string) *stateStore {
	return &stateStore{dir: dir}
}

func DefaultRuntimeDir() (string, error) {
	if dir := os.Getenv("TOKDOCTOR_GATEWAY_RUNTIME_DIR"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("TOKDOCTOR_GATEWAY_DIR"); dir != "" {
		return filepath.Join(dir, "runtime"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config dir: %w", err)
	}
	return filepath.Join(base, "tokdoctor", "gateway", "runtime"), nil
}

func (s *stateStore) pathFor(profile string) string {
	return filepath.Join(s.dir, profile+".json")
}

func (s *stateStore) Write(st RuntimeState) error {
	if st.Profile == "" {
		return errors.New("runtime state requires profile")
	}
	if st.Listen == "" {
		return errors.New("runtime state requires listen address")
	}
	if st.ControlAddr == "" {
		return errors.New("runtime state requires control address")
	}
	if st.ControlToken == "" {
		return errors.New("runtime state requires control token")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create runtime dir %s: %w", s.dir, err)
	}
	target := s.pathFor(st.Profile)
	tmp, err := os.CreateTemp(s.dir, st.Profile+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp runtime state: %w", err)
	}
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(st); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("encode runtime state: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("chmod runtime state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("close temp runtime state: %w", err)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("rename runtime state to %s: %w", target, err)
	}
	return nil
}

func (s *stateStore) Read(profile string) (RuntimeState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.pathFor(profile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuntimeState{}, false, nil
		}
		return RuntimeState{}, false, err
	}
	var st RuntimeState
	if err := json.Unmarshal(data, &st); err != nil {
		return RuntimeState{}, false, fmt.Errorf("parse runtime state for %s: %w", profile, err)
	}
	return st, true, nil
}

func (s *stateStore) Remove(profile string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(s.pathFor(profile))
}

var (
	processCheck   = func(pid int) bool { return pidAlive(pid) }
	controlChecker = verifyRuntimeState
	controlClient  = &http.Client{Timeout: 500 * time.Millisecond}
)

type controlHealth struct {
	Profile    string `json:"profile"`
	PID        int    `json:"pid"`
	Listen     string `json:"listen"`
	InstanceID string `json:"instanceId"`
}

func verifyRuntimeState(st RuntimeState) bool {
	if st.ControlAddr == "" || st.ControlToken == "" || !processCheck(st.PID) {
		return false
	}
	if !IsLoopbackAddress(st.ControlAddr) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+st.ControlAddr+"/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-TokDoctor-Control-Token", st.ControlToken)
	resp, err := controlClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var health controlHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return false
	}
	return health.Profile == st.Profile &&
		health.PID == st.PID &&
		health.Listen == st.Listen &&
		health.InstanceID == st.InstanceID
}

func writeControlJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func ProxyURL(addr string) string {
	if addr == "" {
		return ""
	}
	return "http://" + addr
}

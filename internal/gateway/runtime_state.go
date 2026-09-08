package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type RuntimeState struct {
	Profile    string    `json:"profile"`
	PID        int       `json:"pid"`
	Listen     string    `json:"listen"`
	StartedAt  time.Time `json:"startedAt"`
	InstanceID string    `json:"instanceId"`
}

type stateStore struct {
	dir string
	mu  sync.Mutex
}

func newStateStore(dir string) *stateStore {
	return &stateStore{dir: dir}
}

func DefaultRuntimeDir() (string, error) {
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
	listenerCheck  = func(addr string) bool { return dialListener(addr) }
	listenerDialer = &net.Dialer{Timeout: 250 * time.Millisecond}
)

func dialListener(addr string) bool {
	if addr == "" {
		return false
	}
	conn, err := listenerDialer.Dial("tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

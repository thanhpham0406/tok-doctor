package gateway

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Recorder interface {
	Record(Exchange)
	Summary(profile string) (Summary, error)
}

type Summary struct {
	Profile     string
	Requests    int64
	LastRequest time.Time
	CaptureFile string
}

type FileRecorder struct {
	dir   string
	now   func() time.Time
	mu    sync.Mutex
	files map[string]*os.File
}

func NewFileRecorder(dir string) (*FileRecorder, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create gateway dir %s: %w", dir, err)
	}
	return &FileRecorder{
		dir:   dir,
		now:   time.Now,
		files: map[string]*os.File{},
	}, nil
}

func (r *FileRecorder) Record(e Exchange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := r.fileFor(e.Profile)
	if err != nil {
		return
	}
	enc := json.NewEncoder(f)
	_ = enc.Encode(e)
}

func (r *FileRecorder) Summary(profile string) (Summary, error) {
	path := r.pathFor(profile)
	count := countJSONLLines(path)
	last := time.Time{}
	if info, err := os.Stat(path); err == nil {
		last = info.ModTime()
	}
	return Summary{
		Profile:     profile,
		Requests:    count,
		LastRequest: last,
		CaptureFile: path,
	}, nil
}

func countJSONLLines(path string) int64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var count int64
	for scanner.Scan() {
		count++
	}
	return count
}

func (r *FileRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for _, f := range r.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.files = map[string]*os.File{}
	return firstErr
}

func (r *FileRecorder) fileFor(profile string) (*os.File, error) {
	if f, ok := r.files[profile]; ok {
		return f, nil
	}
	path := r.pathFor(profile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	r.files[profile] = f
	return f, nil
}

func (r *FileRecorder) pathFor(profile string) string {
	return filepath.Join(r.dir, profile+".jsonl")
}

func (r *FileRecorder) Purge(profile string) error {
	if err := ValidateCaptureProfile(profile); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.pathFor(profile)
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("purge capture %s: %w", profile, err)
}

func ValidateCaptureProfile(profile string) error {
	if profile == "" {
		return errors.New("capture profile is required")
	}
	if strings.ContainsAny(profile, "/\\") || profile == "." || profile == ".." || strings.Contains(profile, "\x00") {
		return fmt.Errorf("invalid capture profile %q", profile)
	}
	if filepath.Base(profile) != profile {
		return fmt.Errorf("invalid capture profile %q", profile)
	}
	return nil
}

func DefaultDir() (string, error) {
	if dir := os.Getenv("TOKDOCTOR_GATEWAY_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config dir: %w", err)
	}
	return filepath.Join(base, "tokdoctor", "gateway"), nil
}

func Replay(r *FileRecorder, profile string) ([]Exchange, error) {
	path := r.pathFor(profile)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	dec := json.NewDecoder(file)
	var out []Exchange
	for {
		var e Exchange
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return out, err
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}

func (r *FileRecorder) ListExchanges(profile string) ([]Exchange, error) {
	if r == nil {
		return nil, nil
	}
	exchanges, err := Replay(r, profile)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(exchanges, func(i, j int) bool {
		return exchanges[i].StartedAt.After(exchanges[j].StartedAt)
	})
	return exchanges, nil
}

var (
	ErrExchangeNotFound  = errors.New("exchange not found")
	ErrExchangeAmbiguous = errors.New("ambiguous exchange id")
)

func (r *FileRecorder) FindExchange(profile, id string) (Exchange, error) {
	if id == "" {
		return Exchange{}, fmt.Errorf("exchange id: %w", ErrExchangeNotFound)
	}
	exchanges, err := r.ListExchanges(profile)
	if err != nil {
		return Exchange{}, err
	}
	for _, e := range exchanges {
		if e.ID == id {
			return e, nil
		}
	}
	var matches []Exchange
	for _, e := range exchanges {
		if strings.HasPrefix(e.ID, id) {
			matches = append(matches, e)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Exchange{}, fmt.Errorf("exchange %q in profile %q: %w", id, profile, ErrExchangeNotFound)
	default:
		return Exchange{}, fmt.Errorf("exchange %q in profile %q: %w", id, profile, ErrExchangeAmbiguous)
	}
}

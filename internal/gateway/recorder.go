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
	Record(Exchange) error
	Summary(profile string) (Summary, error)
}

type RecorderError struct {
	Profile string
	Op      string
	Err     error
}

func (e *RecorderError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("recorder %s %s: %v", e.Op, e.Profile, e.Err)
}

func (e *RecorderError) Unwrap() error { return e.Err }

const RecorderFailureSchemaVersion = 1

type RecorderFailure struct {
	SchemaVersion int       `json:"schemaVersion,omitempty"`
	Profile       string    `json:"profile"`
	Operation     string    `json:"operation"`
	ExchangeID    string    `json:"exchangeId,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
}

const recorderFailureJournalReadLimit = 1024

type Summary struct {
	Profile     string
	Requests    int64
	LastRequest time.Time
	CaptureFile string
}

type RecorderSink interface {
	RecordRecorderFailure(profile string, err *RecorderError)
}

type noopSink struct{}

func (noopSink) RecordRecorderFailure(string, *RecorderError) {}

type FileRecorder struct {
	dir            string
	now            func() time.Time
	mu             sync.Mutex
	files          map[string]*os.File
	failureHandles map[string]*os.File
	journalErrs    map[string]error
}

func NewFileRecorder(dir string) (*FileRecorder, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create gateway dir %s: %w", dir, err)
	}
	return &FileRecorder{
		dir:            dir,
		now:            time.Now,
		files:          map[string]*os.File{},
		failureHandles: map[string]*os.File{},
		journalErrs:    map[string]error{},
	}, nil
}

func (r *FileRecorder) Record(e Exchange) error {
	failure := r.recordExchange(e)
	if failure != nil {
		return failure
	}
	return nil
}

func (r *FileRecorder) recordExchange(e Exchange) *RecorderError {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := r.fileFor(e.Profile)
	if err != nil {
		r.appendFailureLocked(e.Profile, "open", e.ID, err)
		return &RecorderError{Profile: e.Profile, Op: "open", Err: err}
	}
	if err := json.NewEncoder(f).Encode(e); err != nil {
		r.appendFailureLocked(e.Profile, "encode", e.ID, err)
		return &RecorderError{Profile: e.Profile, Op: "encode", Err: err}
	}
	return nil
}

// appendFailureLocked persists capture-failure metadata exactly once per
// failed capture. Journal write problems never recurse back into this method;
// they are stored per profile so readers can report an unhealthy journal
// instead of silently treating accounting as complete.
func (r *FileRecorder) appendFailureLocked(profile, op, exchangeID string, cause error) {
	f, err := r.failureJournalHandleLocked(profile)
	if err == nil {
		entry := RecorderFailure{
			SchemaVersion: RecorderFailureSchemaVersion,
			Profile:       profile,
			Operation:     op,
			ExchangeID:    exchangeID,
			OccurredAt:    r.now(),
		}
		err = json.NewEncoder(f).Encode(entry)
	}
	if err != nil {
		if r.journalErrs[profile] == nil {
			// The first journal problem wins; the capture error is kept as
			// context so the caller can tell the accounting apart from the
			// journal breakage later on.
			r.journalErrs[profile] = fmt.Errorf("failure=%v journal=%w", cause, err)
		}
	}
}

type RecorderFailureRead struct {
	Failures  []RecorderFailure
	Total     int
	Truncated bool
}

func (r *FileRecorder) RecorderFailures(profile string) (RecorderFailureRead, error) {
	if r == nil {
		return RecorderFailureRead{}, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err, ok := r.journalErrs[profile]; ok {
		return RecorderFailureRead{}, fmt.Errorf("failure journal for %s: %w", profile, err)
	}
	return r.readFailureJournalLocked(profile)
}

func (r *FileRecorder) readFailureJournalLocked(profile string) (RecorderFailureRead, error) {
	path := r.failureJournalPath(profile)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RecorderFailureRead{}, nil
		}
		return RecorderFailureRead{}, fmt.Errorf("open failure journal for %s: %w", profile, err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	window := make([]RecorderFailure, 0, recorderFailureJournalReadLimit)
	total := 0
	for scanner.Scan() {
		var entry RecorderFailure
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		if entry.Profile != "" && entry.Profile != profile {
			continue
		}
		if total < recorderFailureJournalReadLimit {
			window = append(window, entry)
		} else {
			window[total%recorderFailureJournalReadLimit] = entry
		}
		total++
	}
	if err := scanner.Err(); err != nil {
		return RecorderFailureRead{}, fmt.Errorf("read failure journal for %s: %w", profile, err)
	}
	read := RecorderFailureRead{Failures: window, Total: total, Truncated: total > recorderFailureJournalReadLimit}
	if !read.Truncated {
		return read, nil
	}
	start := (total - recorderFailureJournalReadLimit) % recorderFailureJournalReadLimit
	out := make([]RecorderFailure, 0, recorderFailureJournalReadLimit)
	out = append(out, window[start:]...)
	out = append(out, window[:start]...)
	read.Failures = out
	return read, nil
}

func (r *FileRecorder) failureJournalPath(profile string) string {
	return filepath.Join(r.dir, profile+".failures.jsonl")
}

func (r *FileRecorder) failureJournalHandleLocked(profile string) (*os.File, error) {
	if f, ok := r.failureHandles[profile]; ok {
		return f, nil
	}
	path := r.failureJournalPath(profile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	r.failureHandles[profile] = f
	return f, nil
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
	for _, f := range r.failureHandles {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.files = map[string]*os.File{}
	r.failureHandles = map[string]*os.File{}
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
	var closeErr error
	if f, ok := r.files[profile]; ok {
		if err := f.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		delete(r.files, profile)
	}
	if f, ok := r.failureHandles[profile]; ok {
		if err := f.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		delete(r.failureHandles, profile)
	}
	if closeErr != nil {
		return fmt.Errorf("purge %s: close handles: %w", profile, closeErr)
	}
	if err := r.removeFile(r.pathFor(profile)); err != nil {
		return fmt.Errorf("purge capture %s: %w", profile, err)
	}
	if err := r.removeFile(r.failureJournalPath(profile)); err != nil {
		return fmt.Errorf("purge failure journal %s: %w", profile, err)
	}
	delete(r.journalErrs, profile)
	return nil
}

func (r *FileRecorder) removeFile(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
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
	var out []Exchange
	dec := json.NewDecoder(file)
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
		return Exchange{}, fmt.Errorf("exchange %q in profile %s: %w", id, profile, ErrExchangeNotFound)
	default:
		return Exchange{}, fmt.Errorf("exchange %q in profile %s: %w", id, profile, ErrExchangeAmbiguous)
	}
}

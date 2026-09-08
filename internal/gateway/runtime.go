package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

type Runtime struct {
	recorder   *FileRecorder
	states     *stateStore
	instanceID string
	servers    map[string]*http.Server
	listens    map[string]net.Listener
	started    map[string]time.Time
	mu         sync.Mutex
}

func NewRuntime(recorder *FileRecorder) *Runtime {
	dir, err := DefaultRuntimeDir()
	if err != nil {
		dir = ""
	}
	return newRuntimeWithStore(recorder, newStateStore(dir))
}

func newRuntimeWithStore(recorder *FileRecorder, states *stateStore) *Runtime {
	return &Runtime{
		recorder:   recorder,
		states:     states,
		instanceID: newInstanceID(),
		servers:    map[string]*http.Server{},
		listens:    map[string]net.Listener{},
		started:    map[string]time.Time{},
	}
}

func (r *Runtime) Start(ctx context.Context, profiles []Profile) error {
	if len(profiles) == 0 {
		return errors.New("no enabled gateway profiles to start")
	}
	sorted := append([]Profile(nil), profiles...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	started := make([]Profile, 0, len(sorted))
	for _, p := range sorted {
		if err := r.startOne(ctx, p); err != nil {
			r.shutdown(started)
			return fmt.Errorf("start profile %s: %w", p.Name, err)
		}
		started = append(started, p)
	}
	return nil
}

func (r *Runtime) startOne(ctx context.Context, p Profile) error {
	observer := ObserverFor(p.Protocol)
	if observer == nil {
		return fmt.Errorf("unsupported protocol %q", p.Protocol)
	}
	proxy, err := NewProxy(p, observer, r.recorder)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", p.Listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", p.Listen, err)
	}
	server := &http.Server{
		Handler:           proxy.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "gateway profile %s stopped: %v\n", p.Name, err)
		}
	}()
	startedAt := time.Now()
	r.mu.Lock()
	r.servers[p.Name] = server
	r.listens[p.Name] = listener
	r.started[p.Name] = startedAt
	r.mu.Unlock()
	if r.states != nil {
		st := RuntimeState{
			Profile:    p.Name,
			PID:        os.Getpid(),
			Listen:     p.Listen,
			StartedAt:  startedAt,
			InstanceID: r.instanceID,
		}
		if writeErr := r.states.Write(st); writeErr != nil {
			_ = server.Shutdown(context.Background())
			r.mu.Lock()
			delete(r.servers, p.Name)
			delete(r.listens, p.Name)
			delete(r.started, p.Name)
			r.mu.Unlock()
			return fmt.Errorf("persist runtime state for %s: %w", p.Name, writeErr)
		}
	}
	return nil
}

func (r *Runtime) Wait(ctx context.Context) error {
	r.mu.Lock()
	count := len(r.servers)
	r.mu.Unlock()
	if count == 0 {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	profiles := make([]Profile, 0, len(r.servers))
	for name := range r.servers {
		profiles = append(profiles, Profile{Name: name})
	}
	r.mu.Unlock()
	sort.SliceStable(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	r.shutdown(profiles)
	return nil
}

func (r *Runtime) shutdown(profiles []Profile) {
	for _, p := range profiles {
		r.mu.Lock()
		server := r.servers[p.Name]
		r.mu.Unlock()
		if server == nil {
			continue
		}
		_ = server.Shutdown(context.Background())
		if r.states != nil {
			r.states.Remove(p.Name)
		}
		r.mu.Lock()
		delete(r.servers, p.Name)
		delete(r.listens, p.Name)
		delete(r.started, p.Name)
		r.mu.Unlock()
	}
}

func (r *Runtime) Status(profiles []Profile) []StatusEntry {
	out := make([]StatusEntry, 0, len(profiles))
	for _, p := range profiles {
		summary, _ := r.recorder.Summary(p.Name)
		entry := StatusEntry{
			Profile:     p.Name,
			Listen:      p.Listen,
			Protocol:    p.Protocol,
			Upstream:    p.Upstream,
			Requests:    summary.Requests,
			LastSeen:    summary.LastRequest,
			CaptureFile: summary.CaptureFile,
		}
		entry.State = r.stateFor(p)
		out = append(out, entry)
	}
	return out
}

func (r *Runtime) Address(profile string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.listens[profile]; ok {
		return l.Addr().String()
	}
	return ""
}

func (r *Runtime) stateFor(p Profile) string {
	r.mu.Lock()
	_, inMemory := r.servers[p.Name]
	r.mu.Unlock()
	if inMemory {
		return "running"
	}
	if r.states == nil {
		return "stopped"
	}
	st, ok, err := r.states.Read(p.Name)
	if err != nil || !ok {
		return "stopped"
	}
	alive := processCheck(st.PID)
	bound := listenerCheck(st.Listen)
	if alive && bound {
		return "running"
	}
	if !alive || !bound {
		r.states.Remove(p.Name)
	}
	return "stopped"
}

type StatusEntry struct {
	Profile     string
	Listen      string
	Protocol    string
	Upstream    string
	Requests    int64
	LastSeen    time.Time
	CaptureFile string
	State       string
}

func RenderTable(w io.Writer, rows []StatusEntry) {
	fmt.Fprintln(w, "Profile          Listen           Protocol             Upstream")
	fmt.Fprintln(w, "---------------  ---------------  -------------------- -------------------")
	for _, row := range rows {
		fmt.Fprintf(w, "%-15s  %-15s  %-20s %s\n",
			truncate(row.Profile, 15),
			truncate(row.Listen, 15),
			truncate(string(row.Protocol), 20),
			truncate(row.Upstream, 60),
		)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func newInstanceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("tok-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

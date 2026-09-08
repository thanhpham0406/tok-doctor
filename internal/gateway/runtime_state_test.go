package gateway

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// --- stateStore tests --------------------------------------------------------

func TestStateStore_WriteIsAtomicAndRestrictive(t *testing.T) {
	dir := t.TempDir()
	store := newStateStore(dir)
	st := RuntimeState{
		Profile:    "alpha",
		PID:        os.Getpid(),
		Listen:     "127.0.0.1:9999",
		StartedAt:  time.Now(),
		InstanceID: "abc123",
	}
	if err := store.Write(st); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "alpha.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file too permissive: %v", info.Mode().Perm())
	}
	got, ok, err := store.Read("alpha")
	if err != nil || !ok {
		t.Fatalf("Read: ok=%v err=%v", ok, err)
	}
	if got.Profile != "alpha" || got.PID != st.PID || got.Listen != st.Listen || got.InstanceID != "abc123" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	store.Remove("alpha")
	if _, ok, _ := store.Read("alpha"); ok {
		t.Fatalf("expected removal")
	}
}

func TestStateStore_WriteValidatesFields(t *testing.T) {
	dir := t.TempDir()
	store := newStateStore(dir)
	if err := store.Write(RuntimeState{Profile: "", Listen: "127.0.0.1:1"}); err == nil {
		t.Fatalf("expected profile required")
	}
	if err := store.Write(RuntimeState{Profile: "x", Listen: ""}); err == nil {
		t.Fatalf("expected listen required")
	}
}

// --- Runtime state lifecycle --------------------------------------------------

type stubProcess struct {
	alive  bool
	pid    int
	called int
}

func (s *stubProcess) check(pid int) bool {
	s.called++
	s.pid = pid
	return s.alive
}

type stubListener struct {
	bound  bool
	addr   string
	called int
}

func (s *stubListener) check(addr string) bool {
	s.called++
	s.addr = addr
	return s.bound
}

func withStubCheckers(t *testing.T, proc *stubProcess, lst *stubListener) {
	t.Helper()
	origProc, origLst := processCheck, listenerCheck
	processCheck = func(pid int) bool { return proc.check(pid) }
	listenerCheck = func(addr string) bool { return lst.check(addr) }
	t.Cleanup(func() {
		processCheck = origProc
		listenerCheck = origLst
	})
}

func newTestRuntime(t *testing.T, store *stateStore, recorder *FileRecorder) *Runtime {
	t.Helper()
	return newRuntimeWithStore(recorder, store)
}

func profileForListen(name, listen, upstream string) Profile {
	return Profile{
		Name:     name,
		Enabled:  true,
		Listen:   listen,
		Protocol: "anthropic_messages",
		Source:   "claude",
		Upstream: upstream,
	}
}

func TestRuntime_StateWrittenAfterBind(t *testing.T) {
	listen, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	dir := t.TempDir()
	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	rt := newTestRuntime(t, newStateStore(dir), rec)

	if err := rt.Start(context.Background(), []Profile{profileForListen("alpha", listen, upstream.URL)}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	if _, ok, err := newStateStore(dir).Read("alpha"); !ok || err != nil {
		t.Fatalf("expected runtime state after Start, ok=%v err=%v", ok, err)
	}
}

func TestRuntime_GracefulShutdownRemovesState(t *testing.T) {
	listen, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	storeDir := t.TempDir()
	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	store := newStateStore(storeDir)
	rt := newRuntimeWithStore(rec, store)

	if err := rt.Start(context.Background(), []Profile{profileForListen("alpha", listen, upstream.URL)}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if _, ok, _ := store.Read("alpha"); ok {
		t.Fatalf("expected runtime state removed on shutdown")
	}
}

// --- Status validation across processes --------------------------------------

func TestStatus_RunningWhenAnotherProcessHasLiveState(t *testing.T) {
	listen, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	sharedDir := t.TempDir()
	producerRec, _ := NewFileRecorder(t.TempDir())
	defer producerRec.Close()
	producer := newRuntimeWithStore(producerRec, newStateStore(sharedDir))
	if err := producer.Start(context.Background(), []Profile{profileForListen("alpha", listen, upstream.URL)}); err != nil {
		t.Fatalf("producer Start: %v", err)
	}
	t.Cleanup(func() { _ = producer.Shutdown(context.Background()) })

	proc := &stubProcess{alive: true}
	lst := &stubListener{bound: true}
	withStubCheckers(t, proc, lst)

	consumerRec, _ := NewFileRecorder(t.TempDir())
	defer consumerRec.Close()
	consumer := newRuntimeWithStore(consumerRec, newStateStore(sharedDir))
	rows := consumer.Status([]Profile{profileForListen("alpha", listen, upstream.URL)})
	if len(rows) != 1 || rows[0].State != "running" {
		t.Fatalf("status = %+v", rows)
	}
	if proc.called == 0 || lst.called == 0 {
		t.Fatalf("expected checkers to be consulted: proc=%d lst=%d", proc.called, lst.called)
	}
}

func TestStatus_StoppedWhenNoStateExists(t *testing.T) {
	listen, _ := freeLoopback(t)
	proc := &stubProcess{alive: true}
	lst := &stubListener{bound: true}
	withStubCheckers(t, proc, lst)

	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	rt := newRuntimeWithStore(rec, newStateStore(t.TempDir()))
	rows := rt.Status([]Profile{profileForListen("alpha", listen, "http://127.0.0.1:1")})
	if len(rows) != 1 || rows[0].State != "stopped" {
		t.Fatalf("status = %+v", rows)
	}
	if proc.called != 0 || lst.called != 0 {
		t.Fatalf("checkers must not be called without state file")
	}
}

func TestStatus_StoppedWhenPIDDead(t *testing.T) {
	listen, _ := freeLoopback(t)
	sharedDir := t.TempDir()
	store := newStateStore(sharedDir)
	if err := store.Write(RuntimeState{
		Profile:    "alpha",
		PID:        os.Getpid(),
		Listen:     listen,
		StartedAt:  time.Now(),
		InstanceID: "x",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	proc := &stubProcess{alive: false}
	lst := &stubListener{bound: true}
	withStubCheckers(t, proc, lst)

	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	rt := newRuntimeWithStore(rec, store)
	rows := rt.Status([]Profile{profileForListen("alpha", listen, "http://127.0.0.1:1")})
	if rows[0].State != "stopped" {
		t.Fatalf("status = %+v", rows)
	}
	if _, ok, _ := store.Read("alpha"); ok {
		t.Fatalf("expected stale state to be removed")
	}
}

func TestStatus_StoppedWhenListenerMissing(t *testing.T) {
	listen, _ := freeLoopback(t)
	sharedDir := t.TempDir()
	store := newStateStore(sharedDir)
	if err := store.Write(RuntimeState{
		Profile:    "alpha",
		PID:        os.Getpid(),
		Listen:     listen,
		StartedAt:  time.Now(),
		InstanceID: "x",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	proc := &stubProcess{alive: true}
	lst := &stubListener{bound: false}
	withStubCheckers(t, proc, lst)

	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	rt := newRuntimeWithStore(rec, store)
	rows := rt.Status([]Profile{profileForListen("alpha", listen, "http://127.0.0.1:1")})
	if rows[0].State != "stopped" {
		t.Fatalf("status = %+v", rows)
	}
	if _, ok, _ := store.Read("alpha"); ok {
		t.Fatalf("expected stale state to be removed")
	}
}

func TestStatus_UnrelatedListenerCannotMakeRunning(t *testing.T) {
	listen, _ := freeLoopback(t)
	sharedDir := t.TempDir()
	// No state file written. PID and listener checkers report true for the
	// unrelated address — status must NOT flip to running.
	proc := &stubProcess{alive: true}
	lst := &stubListener{bound: true}
	withStubCheckers(t, proc, lst)

	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	rt := newRuntimeWithStore(rec, newStateStore(sharedDir))
	rows := rt.Status([]Profile{profileForListen("alpha", listen, "http://127.0.0.1:1")})
	if rows[0].State != "stopped" {
		t.Fatalf("status must require a runtime state file, got %+v", rows)
	}
	if proc.called != 0 || lst.called != 0 {
		t.Fatalf("checkers must not run without a state file")
	}
}

func TestStatus_RestartReplacesStaleState(t *testing.T) {
	listen, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	sharedDir := t.TempDir()
	store := newStateStore(sharedDir)
	if err := store.Write(RuntimeState{
		Profile:    "alpha",
		PID:        999999,
		Listen:     listen,
		StartedAt:  time.Now().Add(-time.Hour),
		InstanceID: "stale",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	rt := newRuntimeWithStore(rec, store)
	if err := rt.Start(context.Background(), []Profile{profileForListen("alpha", listen, upstream.URL)}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	got, ok, _ := store.Read("alpha")
	if !ok {
		t.Fatalf("expected state to be rewritten")
	}
	if got.InstanceID == "stale" {
		t.Fatalf("stale instance id not replaced: %+v", got)
	}
	if got.PID != os.Getpid() {
		t.Fatalf("PID not updated: %+v", got)
	}
}

func TestStatus_MultipleProfilesIndependent(t *testing.T) {
	listenA, _ := freeLoopback(t)
	listenB, _ := freeLoopback(t)
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamB.Close()

	sharedDir := t.TempDir()
	producerRec, _ := NewFileRecorder(t.TempDir())
	defer producerRec.Close()
	producer := newRuntimeWithStore(producerRec, newStateStore(sharedDir))
	if err := producer.Start(context.Background(), []Profile{
		profileForListen("alpha", listenA, upstreamA.URL),
		profileForListen("beta", listenB, upstreamB.URL),
	}); err != nil {
		t.Fatalf("producer Start: %v", err)
	}
	t.Cleanup(func() { _ = producer.Shutdown(context.Background()) })

	r1, _, _ := newStateStore(sharedDir).Read("alpha")
	r2, _, _ := newStateStore(sharedDir).Read("beta")
	if r1.InstanceID != r2.InstanceID {
		t.Fatalf("instance ids should be equal: %+v vs %+v", r1, r2)
	}
	newStateStore(sharedDir).Remove("alpha")

	proc := &stubProcess{alive: true}
	lst := &stubListener{bound: true}
	withStubCheckers(t, proc, lst)

	consumerRec, _ := NewFileRecorder(t.TempDir())
	defer consumerRec.Close()
	consumer := newRuntimeWithStore(consumerRec, newStateStore(sharedDir))
	rows := consumer.Status([]Profile{
		profileForListen("alpha", listenA, upstreamA.URL),
		profileForListen("beta", listenB, upstreamB.URL),
	})
	byName := map[string]string{}
	for _, row := range rows {
		byName[row.Profile] = row.State
	}
	if byName["alpha"] != "stopped" {
		t.Fatalf("alpha state = %q", byName["alpha"])
	}
	if byName["beta"] != "running" {
		t.Fatalf("beta state = %q", byName["beta"])
	}
	if _, ok, _ := newStateStore(sharedDir).Read("beta"); !ok {
		t.Fatalf("beta state must remain")
	}
}

func TestStatus_ExistingRequestCountAndLastRequestUnchanged(t *testing.T) {
	listen, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	sharedDir := t.TempDir()
	recDir := t.TempDir()
	rec, _ := NewFileRecorder(recDir)
	defer rec.Close()

	producer := newRuntimeWithStore(rec, newStateStore(sharedDir))
	if err := producer.Start(context.Background(), []Profile{profileForListen("alpha", listen, upstream.URL)}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = producer.Shutdown(context.Background()) })

	for i := 0; i < 2; i++ {
		resp, err := http.Post("http://"+listen+"/v1/messages", "application/json", strings.NewReader(`{"model":"x"}`))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		resp.Body.Close()
	}

	proc := &stubProcess{alive: true}
	lst := &stubListener{bound: true}
	withStubCheckers(t, proc, lst)

	consumerRec, _ := NewFileRecorder(recDir)
	defer consumerRec.Close()
	consumer := newRuntimeWithStore(consumerRec, newStateStore(sharedDir))
	rows := consumer.Status([]Profile{profileForListen("alpha", listen, upstream.URL)})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Requests != 2 {
		t.Fatalf("requests = %d, want 2", rows[0].Requests)
	}
	if rows[0].LastSeen.IsZero() {
		t.Fatalf("expected non-zero last-seen")
	}
	if rows[0].State != "running" {
		t.Fatalf("state = %q", rows[0].State)
	}
}

func TestStatus_StateContainsNoSecrets(t *testing.T) {
	listen, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	sharedDir := t.TempDir()
	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	rt := newRuntimeWithStore(rec, newStateStore(sharedDir))
	if err := rt.Start(context.Background(), []Profile{profileForListen("alpha", listen, upstream.URL)}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	data, err := os.ReadFile(filepath.Join(sharedDir, "alpha.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "Bearer") ||
		strings.Contains(body, "sk-") ||
		strings.Contains(body, "anthropic") ||
		strings.Contains(strings.ToLower(body), "secret") {
		t.Fatalf("runtime state leaked sensitive content: %s", body)
	}
}

func TestRuntime_StartFailsCleanlyIfStatePersistFails(t *testing.T) {
	listen, _ := freeLoopback(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rec, _ := NewFileRecorder(t.TempDir())
	defer rec.Close()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	store := newStateStore(blocker)
	rt := newRuntimeWithStore(rec, store)
	if err := rt.Start(context.Background(), []Profile{profileForListen("alpha", listen, upstream.URL)}); err == nil {
		t.Fatalf("expected start failure when state persistence fails")
	}

	// The on-disk in-memory maps must have been rolled back so the address is
	// reusable from a different caller perspective.
	if l, err := net.Listen("tcp", listen); err == nil {
		_ = l.Close()
	} else {
		// SO_REUSEADDR / TIME_WAIT can momentarily block rebinding on the same
		// address. Poll briefly to confirm the listener is actually closed.
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if l, err := net.Listen("tcp", listen); err == nil {
				_ = l.Close()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("listener not released: %v", err)
	}
}

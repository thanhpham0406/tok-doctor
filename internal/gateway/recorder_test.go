package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileRecorder_OneRecordFailureWritesOneJournalEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "p.jsonl"), 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	rec, _ := NewFileRecorder(dir)
	defer func() { _ = rec.Close() }()
	if err := rec.Record(Exchange{ID: "gw-x", Profile: "p", StartedAt: time.Now()}); err == nil {
		t.Fatalf("expected Record to fail when capture file is a directory")
	}
	failures, err := rec.RecorderFailures("p")
	if err != nil {
		t.Fatalf("read failures: %v", err)
	}
	if len(failures.Failures) != 1 {
		t.Fatalf("journal entries = %d, want 1 (no duplicates)", len(failures.Failures))
	}
	if failures.Failures[0].Operation != "open" {
		t.Fatalf("operation = %q, want open", failures.Failures[0].Operation)
	}
	if failures.Failures[0].ExchangeID != "gw-x" {
		t.Fatalf("exchange id = %q, want gw-x", failures.Failures[0].ExchangeID)
	}
	if failures.Failures[0].SchemaVersion != RecorderFailureSchemaVersion {
		t.Fatalf("schema version = %d, want %d", failures.Failures[0].SchemaVersion, RecorderFailureSchemaVersion)
	}
}

func TestFileRecorder_FailureJournalPersistedAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "p.jsonl"), 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	rec, _ := NewFileRecorder(dir)
	if err := rec.Record(Exchange{ID: "gw-a", Profile: "p", StartedAt: time.Now()}); err == nil {
		t.Fatalf("Record a should fail")
	}
	if err := rec.Record(Exchange{ID: "gw-b", Profile: "p", StartedAt: time.Now()}); err == nil {
		t.Fatalf("Record b should fail")
	}
	_ = rec.Close()

	rec2, _ := NewFileRecorder(dir)
	defer func() { _ = rec2.Close() }()
	failures, err := rec2.RecorderFailures("p")
	if err != nil {
		t.Fatalf("read failures: %v", err)
	}
	if len(failures.Failures) != 2 {
		t.Fatalf("new instance saw %d failures, want 2", len(failures.Failures))
	}
	if failures.Failures[0].ExchangeID != "gw-a" || failures.Failures[1].ExchangeID != "gw-b" {
		t.Fatalf("order = %q/%q, want gw-a/gw-b", failures.Failures[0].ExchangeID, failures.Failures[1].ExchangeID)
	}
}

func TestFileRecorder_PurgeRemovesFailureJournal(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "p.jsonl"), 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	rec, _ := NewFileRecorder(dir)
	if err := rec.Record(Exchange{ID: "gw-x", Profile: "p", StartedAt: time.Now()}); err == nil {
		t.Fatalf("Record should fail")
	}
	journalPath := filepath.Join(dir, "p.failures.jsonl")
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("journal not created: %v", err)
	}
	if err := rec.Purge("p"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("journal still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "p.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("capture still present: %v", err)
	}
}

func TestFileRecorder_FailureJournalPrivacy(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "p.jsonl"), 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	rec, _ := NewFileRecorder(dir)
	ex := Exchange{ID: "gw-x", Profile: "p", StartedAt: time.Now()}
	ex.Request.Components = nil
	if err := rec.Record(ex); err == nil {
		t.Fatalf("Record should fail")
	}
	_ = rec.Close()
	data, err := os.ReadFile(filepath.Join(dir, "p.failures.jsonl"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	for _, marker := range []string{"SECRET_PROMPT", "Bearer", "Authorization", "sk-", "error text"} {
		if strings.Contains(string(data), marker) {
			t.Fatalf("journal leaked %q: %s", marker, data)
		}
	}
}

func TestFileRecorder_PurgeClosesHandlesAndRemovesFiles(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()
	if err := rec.Record(Exchange{ID: "gw-ok", Profile: "p", StartedAt: time.Now()}); err != nil {
		t.Fatalf("Record ok: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "conflict.jsonl"), 0o755); err != nil {
		t.Fatalf("seed conflict: %v", err)
	}
	if err := rec.Record(Exchange{ID: "gw-bad", Profile: "conflict", StartedAt: time.Now()}); err == nil {
		t.Fatalf("Record on conflicting profile should fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "conflict.failures.jsonl")); err != nil {
		t.Fatalf("journal not created: %v", err)
	}
	if err := rec.Purge("p"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "p.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("capture still present: %v", err)
	}
	if _, err := rec.RecorderFailures("conflict"); err != nil {
		t.Fatalf("unrelated profile must stay intact: %v", err)
	}
}

func TestFileRecorder_FailedPurgePreservesJournalHealth(t *testing.T) {
	dir := t.TempDir()
	capturePath := filepath.Join(dir, "p.jsonl")
	journalPath := filepath.Join(dir, "p.failures.jsonl")
	if err := os.Mkdir(capturePath, 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	if err := os.Mkdir(journalPath, 0o755); err != nil {
		t.Fatalf("seed journal conflict: %v", err)
	}
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()

	if err := rec.Record(Exchange{ID: "gw-a", Profile: "p", StartedAt: time.Now()}); err == nil {
		t.Fatalf("Record should fail with both paths conflicting")
	}
	if _, err := rec.RecorderFailures("p"); err == nil {
		t.Fatalf("journal must be unhealthy after a failed journal write")
	}
	if err := os.Remove(journalPath); err != nil {
		t.Fatalf("clear journal conflict: %v", err)
	}
	if err := os.WriteFile(journalPath, []byte(`{"profile":"p","operation":"open"}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed journal evidence: %v", err)
	}
	if err := os.WriteFile(filepath.Join(capturePath, "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed non-empty capture dir: %v", err)
	}

	if err := rec.Purge("p"); err == nil {
		t.Fatalf("Purge should fail when capture cannot be removed")
	}
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("failure journal must survive a failed purge: %v", err)
	}
	if _, err := rec.RecorderFailures("p"); err == nil {
		t.Fatalf("journal health must stay unhealthy after failed purge")
	}

	if err := os.RemoveAll(capturePath); err != nil {
		t.Fatalf("clear capture conflict: %v", err)
	}
	if err := rec.Purge("p"); err != nil {
		t.Fatalf("Purge after clearing conflicts: %v", err)
	}
	if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
		t.Fatalf("capture should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("failure journal should be removed, stat err = %v", err)
	}
	read, err := rec.RecorderFailures("p")
	if err != nil {
		t.Fatalf("journal health should be cleared after successful purge: %v", err)
	}
	if read.Total != 0 || len(read.Failures) != 0 {
		t.Fatalf("cleared journal should read empty, got %+v", read)
	}
	if err := rec.Purge("p"); err != nil {
		t.Fatalf("idempotent purge: %v", err)
	}
}

func rawFailureLine(t *testing.T, op string) string {
	t.Helper()
	raw, err := json.Marshal(RecorderFailure{Profile: "p", Operation: op, OccurredAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func TestFileRecorder_FailureJournalReadReturnsMostRecent(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "p.failures.jsonl")
	var lines []string
	for i := 0; i < recorderFailureJournalReadLimit+50; i++ {
		lines = append(lines, rawFailureLine(t, fmt.Sprintf("op-%d", i)))
	}
	// Malformed lines must not erase or shift the valid records around them.
	lines = append(lines, "}{", rawFailureLine(t, "op-final"))
	if err := os.WriteFile(journal, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()
	failures, err := rec.RecorderFailures("p")
	if err != nil {
		t.Fatalf("read failures: %v", err)
	}
	if len(failures.Failures) != recorderFailureJournalReadLimit {
		t.Fatalf("records = %d, want %d", len(failures.Failures), recorderFailureJournalReadLimit)
	}
	if failures.Total != recorderFailureJournalReadLimit+51 {
		t.Fatalf("total = %d, want %d", failures.Total, recorderFailureJournalReadLimit+51)
	}
	if !failures.Truncated {
		t.Fatalf("Truncated = false, want true")
	}
	if got := failures.Failures[len(failures.Failures)-1].Operation; got != "op-final" {
		t.Fatalf("newest record = %q, want op-final", got)
	}
	offset := recorderFailureJournalReadLimit + 51 - recorderFailureJournalReadLimit
	oldest := fmt.Sprintf("op-%d", offset)
	if got := failures.Failures[0].Operation; got != oldest {
		t.Fatalf("oldest kept record = %q, want %q", got, oldest)
	}
}

func TestFileRecorder_UnhealthyJournalStaysUnhealthyUntilPurge(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "p.jsonl"), 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "p.failures.jsonl"), 0o755); err != nil {
		t.Fatalf("seed journal conflict: %v", err)
	}
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()

	if err := rec.Record(Exchange{ID: "gw-a", Profile: "p", StartedAt: time.Now()}); err == nil {
		t.Fatalf("Record should fail when capture path is a directory")
	}
	if _, err := rec.RecorderFailures("p"); err == nil {
		t.Fatalf("unwritable journal must be reported as unhealthy")
	}

	if err := os.Remove(filepath.Join(dir, "p.failures.jsonl")); err != nil {
		t.Fatalf("clear journal conflict: %v", err)
	}
	if err := rec.Record(Exchange{ID: "gw-b", Profile: "p", StartedAt: time.Now()}); err == nil {
		t.Fatalf("Record should still fail when capture path is a directory")
	}
	if _, err := rec.RecorderFailures("p"); err == nil {
		t.Fatalf("journal must stay unhealthy after a later successful journal write")
	}

	if err := rec.Purge("p"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if _, err := rec.RecorderFailures("p"); err != nil {
		t.Fatalf("Purge must clear unhealthy journal state: %v", err)
	}
}

func TestFileRecorder_FailureTotalNotCappedByReadWindow(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "p.failures.jsonl")
	total := recorderFailureJournalReadLimit + 200
	lines := make([]string, 0, total)
	for i := 0; i < total; i++ {
		lines = append(lines, rawFailureLine(t, fmt.Sprintf("op-%d", i)))
	}
	if err := os.WriteFile(journal, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()

	read, err := rec.RecorderFailures("p")
	if err != nil {
		t.Fatalf("read failures: %v", err)
	}
	if read.Total != total {
		t.Fatalf("Total = %d, want %d", read.Total, total)
	}
	if len(read.Failures) != recorderFailureJournalReadLimit {
		t.Fatalf("Failures = %d, want %d", len(read.Failures), recorderFailureJournalReadLimit)
	}
	if !read.Truncated {
		t.Fatalf("Truncated = false, want true")
	}
	if got := read.Failures[len(read.Failures)-1].Operation; got != fmt.Sprintf("op-%d", total-1) {
		t.Fatalf("newest record = %q, want op-%d", got, total-1)
	}
	if got := read.Failures[0].Operation; got != fmt.Sprintf("op-%d", total-recorderFailureJournalReadLimit) {
		t.Fatalf("oldest kept record = %q, want op-%d", got, total-recorderFailureJournalReadLimit)
	}

	account := AccountProfile("p", nil, ChainBuildResult{}, read)
	if account.Counts.RecorderFailures != total {
		t.Fatalf("Counts.RecorderFailures = %d, want %d", account.Counts.RecorderFailures, total)
	}
	if !account.RecorderFailuresTruncated {
		t.Fatalf("RecorderFailuresTruncated = false, want true")
	}
	if len(account.RecorderFailures) != recorderFailureJournalReadLimit {
		t.Fatalf("RecorderFailures = %d, want %d", len(account.RecorderFailures), recorderFailureJournalReadLimit)
	}
	if account.Completeness != "partial" {
		t.Fatalf("Completeness = %q, want partial", account.Completeness)
	}
	raw, err := json.Marshal(account)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"recorderFailuresTruncated":true`) {
		t.Fatalf("json missing truncated state: %s", raw)
	}
	if !strings.Contains(string(raw), fmt.Sprintf(`"recorderFailures":%d`, total)) {
		t.Fatalf("json missing recorder failure total: %s", raw)
	}
}

func TestFileRecorder_UnreadableJournalIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "p.failures.jsonl"), 0o755); err != nil {
		t.Fatalf("seed journal conflict: %v", err)
	}
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()
	if err := os.Mkdir(filepath.Join(dir, "p.jsonl"), 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	if err := rec.Record(Exchange{ID: "gw-x", Profile: "p", StartedAt: time.Now()}); err == nil {
		t.Fatalf("Record should fail")
	}
	if _, err := rec.RecorderFailures("p"); err == nil {
		t.Fatalf("expected read error when journal is unreadable, got none")
	}
}

func TestFileRecorder_ConcurrentRecordAndReadNoDeadlock(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "p.jsonl"), 0o755); err != nil {
		t.Fatalf("seed capture conflict: %v", err)
	}
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = rec.Record(Exchange{ID: fmt.Sprintf("gw-%d", i), Profile: "p", StartedAt: time.Now()})
			if _, err := rec.RecorderFailures("p"); err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock: failure journal read blocked while recorder mutex held")
	}
}

func TestFileRecorder_ParallelRecordsNoSharing(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer func() { _ = rec.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if err := rec.Record(Exchange{ID: fmt.Sprintf("gw-%d-%d", i, j), Profile: "p", StartedAt: time.Now()}); err != nil {
					t.Errorf("record: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	failures, err := rec.RecorderFailures("p")
	if err != nil {
		t.Fatalf("read failures: %v", err)
	}
	if len(failures.Failures) != 0 {
		t.Fatalf("clean records must not produce failures: %v", failures.Failures[:1])
	}
}

package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("VIBEKNOW_CONFIG_HOME", dir)
	return dir
}

func TestAppendThenLoad(t *testing.T) {
	isolate(t)

	if err := Append(Record{TaskID: 1, SessionID: "s1", Status: StatusSubmitted, Source: "a.pdf"}); err != nil {
		t.Fatal(err)
	}
	if err := Append(Record{TaskID: 2, SessionID: "s2", Status: StatusSubmitted}); err != nil {
		t.Fatal(err)
	}

	all, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 records, got %d: %v", len(all), all)
	}
	// Newest first.
	if all[0].TaskID != 2 {
		t.Fatalf("expected the newest record first, got task %d", all[0].TaskID)
	}
}

// TestLoadCollapsesToNewestPerTask is the core of the append-only design:
// Update writes a new line rather than rewriting, so a task's history is a
// run of lines and only the last one describes it.
func TestLoadCollapsesToNewestPerTask(t *testing.T) {
	isolate(t)

	mustAppend(t, Record{TaskID: 7, SessionID: "s7", Status: StatusSubmitted, Source: "deck.pptx"})
	// UpdatedAt has second-or-better resolution; force ordering so the test
	// does not depend on clock granularity.
	time.Sleep(2 * time.Millisecond)
	if err := Update(7, "", func(r *Record) { r.Status = StatusSucceeded; r.ShareURL = "https://x/y" }); err != nil {
		t.Fatal(err)
	}

	all, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("the two lines describe one task and must collapse to one record, got %d: %v", len(all), all)
	}
	got := all[0]
	if got.Status != StatusSucceeded {
		t.Fatalf("status = %q, want the newer value", got.Status)
	}
	// Fields the update did not touch must survive the merge, or every
	// status change would erase what the run was.
	if got.Source != "deck.pptx" {
		t.Fatalf("source lost across update: %+v", got)
	}
	if got.SessionID != "s7" {
		t.Fatalf("session_id lost across update: %+v", got)
	}
	if got.ShareURL != "https://x/y" {
		t.Fatalf("share_url not applied: %+v", got)
	}
}

func TestUpdateCreatesMissingRecord(t *testing.T) {
	isolate(t)

	// `vk video wait` can observe a terminal state for a run this machine
	// never created; the ledger should end up describing it either way.
	if err := Update(99, "s99", func(r *Record) { r.Status = StatusFailed }); err != nil {
		t.Fatal(err)
	}
	r, found, err := Get(99)
	if err != nil || !found {
		t.Fatalf("get: found=%v err=%v", found, err)
	}
	if r.SessionID != "s99" || r.Status != StatusFailed {
		t.Fatalf("unexpected record: %+v", r)
	}
}

func TestLoadOnMissingFile(t *testing.T) {
	isolate(t)
	all, err := Load()
	if err != nil {
		t.Fatalf("a missing ledger is an empty ledger, not an error: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("want none, got %v", all)
	}
}

// TestLoadSkipsCorruptLines: a ledger truncated by a crash, or written by a
// newer version, must still yield the records it does contain. Failing the
// read would make every video subcommand unusable over one bad byte.
func TestLoadSkipsCorruptLines(t *testing.T) {
	dir := isolate(t)
	mustAppend(t, Record{TaskID: 1, SessionID: "s1", Status: StatusSucceeded})

	p := filepath.Join(dir, "jobs.jsonl")
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	all, err := Load()
	if err != nil {
		t.Fatalf("corrupt line should be skipped, not fatal: %v", err)
	}
	if len(all) != 1 || all[0].TaskID != 1 {
		t.Fatalf("want the one good record, got %v", all)
	}
}

func TestGetAndLatest(t *testing.T) {
	isolate(t)
	mustAppend(t, Record{TaskID: 1, SessionID: "s1", Status: StatusSucceeded})
	time.Sleep(2 * time.Millisecond)
	mustAppend(t, Record{TaskID: 2, SessionID: "s2", Status: StatusRunning})

	if r, found, _ := Get(1); !found || r.SessionID != "s1" {
		t.Fatalf("Get(1) = %+v found=%v", r, found)
	}
	if _, found, _ := Get(404); found {
		t.Fatal("Get on an unknown task must report not-found, not a zero record")
	}
	if r, found, _ := GetBySession("s2"); !found || r.TaskID != 2 {
		t.Fatalf("GetBySession = %+v found=%v", r, found)
	}
	if r, found, _ := Latest(); !found || r.TaskID != 2 {
		t.Fatalf("Latest = %+v found=%v", r, found)
	}
}

func TestPrune(t *testing.T) {
	isolate(t)
	old := time.Now().UTC().Add(-90 * 24 * time.Hour)
	mustAppend(t, Record{TaskID: 1, SessionID: "s1", Status: StatusSucceeded, CreatedAt: old})
	mustAppend(t, Record{TaskID: 2, SessionID: "s2", Status: StatusRunning})
	mustAppend(t, Record{TaskID: 3, SessionID: "s3", Status: StatusFailed})

	// --terminal keeps anything still in flight; that is the whole point of
	// the option, since losing the pointer to a live run is unrecoverable.
	removed, kept, err := Prune(PruneOptions{TerminalOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want the two terminal records", removed)
	}
	if len(kept) != 1 || kept[0].TaskID != 2 {
		t.Fatalf("the running record must survive: %v", kept)
	}

	// The rewrite has to be durable, not just returned.
	all, _ := Load()
	if len(all) != 1 || all[0].TaskID != 2 {
		t.Fatalf("prune did not persist: %v", all)
	}
}

func TestPruneAll(t *testing.T) {
	isolate(t)
	mustAppend(t, Record{TaskID: 1, SessionID: "s1", Status: StatusRunning})
	removed, kept, err := Prune(PruneOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || len(kept) != 0 {
		t.Fatalf("removed=%d kept=%v", removed, kept)
	}
}

func TestPruneOlderThanKeepsRecent(t *testing.T) {
	isolate(t)
	mustAppend(t, Record{TaskID: 1, SessionID: "s1", Status: StatusSucceeded})
	removed, kept, err := Prune(PruneOptions{OlderThan: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || len(kept) != 1 {
		t.Fatalf("a record written just now is not an hour old: removed=%d kept=%v", removed, kept)
	}
}

// TestConcurrentAppend covers the reason the ledger is append-only rather
// than read-modify-write: several `vk create` processes can run at once and
// none of them holds a lock.
func TestConcurrentAppend(t *testing.T) {
	isolate(t)

	const n = 32
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = Append(Record{TaskID: int64(i), SessionID: "s", Status: StatusSubmitted})
		}(i)
	}
	wg.Wait()

	all, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n {
		t.Fatalf("want %d distinct records, got %d — interleaved writes corrupted the file", n, len(all))
	}
}

func TestAppendRejectsEmptyRecord(t *testing.T) {
	isolate(t)
	if err := Append(Record{}); err == nil {
		t.Fatal("a record with neither id is not addressable and must be rejected")
	}
}

func TestPathHonorsConfigHome(t *testing.T) {
	dir := isolate(t)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, dir) {
		t.Fatalf("ledger escaped VIBEKNOW_CONFIG_HOME: %s", p)
	}
}

func TestTerminal(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{StatusSucceeded, true},
		{StatusFailed, true},
		{StatusRunning, false},
		{StatusSubmitted, false},
		{StatusUnknown, false},
	} {
		if got := (Record{Status: tc.status}).Terminal(); got != tc.want {
			t.Errorf("Terminal(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func mustAppend(t *testing.T, r Record) {
	t.Helper()
	if err := Append(r); err != nil {
		t.Fatal(err)
	}
}

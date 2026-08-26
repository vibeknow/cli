// Package jobs is the local ledger of video generation runs.
//
// Every figlens run is addressed by a (task_id, session_id) pair, and until
// now the CLI printed that pair once and forgot it. Anything the caller did
// afterwards — wait, status, export, download — required carrying both
// values back by hand. An agent that lost its context, or a user who closed
// the terminal, had no way to reach a run that was still in flight, even
// though it kept costing credits and finished successfully.
//
// The ledger is an append-only JSONL file next to the config. Append-only
// keeps concurrent `vk create` processes from clobbering each other without
// a lock: each writes one line under O_APPEND, and readers collapse the
// file so the newest line for a task_id wins. Compaction rewrites the file
// only from `vk jobs prune`, and from Append when the file grows past
// compactThreshold.
//
// The ledger is a convenience index, never a source of truth. The backend
// owns run state; a missing or stale ledger entry must degrade to "pass
// --session-id yourself", never to a wrong answer.
package jobs

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vibeknow/cli/internal/config"
)

// Status values a ledger record can carry. They mirror the states the CLI
// can actually observe locally — the backend has more.
const (
	StatusSubmitted = "submitted" // init returned; run dispatched, outcome unseen
	StatusRunning   = "running"   // at least one progress event observed
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusUnknown   = "unknown" // stream dropped before a terminal event
	StatusPaused    = "paused"  // backend reported the run paused; resumable
)

// Record is one line of the ledger.
type Record struct {
	TaskID    int64     `json:"task_id"`
	SessionID string    `json:"session_id"`
	WorkID    int64     `json:"work_id,omitempty"`
	Status    string    `json:"status"`
	Source    string    `json:"source,omitempty"` // the --from value
	Mode      string    `json:"mode,omitempty"`   // video_kind, "" for the default graph
	Engine    string    `json:"engine,omitempty"`
	Title     string    `json:"title,omitempty"`
	ShareURL  string    `json:"share_url,omitempty"`
	VideoPath string    `json:"video_path,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Terminal reports whether the run has reached a state that will not change.
func (r Record) Terminal() bool {
	return r.Status == StatusSucceeded || r.Status == StatusFailed
}

// compactThreshold is the line count past which Append rewrites the file.
// Chosen so that a heavy user never notices the rewrite, and a runaway loop
// cannot grow the ledger without bound.
const compactThreshold = 2000

// Path returns the ledger's absolute path. It follows the same config-home
// resolution as profiles.yaml, so VIBEKNOW_CONFIG_HOME relocates it and
// tests get an isolated ledger for free.
func Path() (string, error) {
	d, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "jobs.jsonl"), nil
}

// Append writes one record. Callers treat failure as non-fatal: losing a
// ledger line must never fail the run it was describing.
func Append(r Record) error {
	if r.TaskID == 0 && r.SessionID == "" {
		return errors.New("jobs: record needs a task_id or a session_id")
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now

	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}

	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	// A single write under O_APPEND is what makes concurrent `vk create`
	// runs safe without a lock; splitting it would interleave lines.
	if _, err := f.Write(line); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if n, err := lineCount(p); err == nil && n > compactThreshold {
		_ = compact(p)
	}
	return nil
}

// Update merges a status change onto the existing record for taskID,
// preserving fields the caller does not set (source, mode, created_at).
// A task with no prior record still gets one — the ledger should end up
// describing the run either way.
func Update(taskID int64, sessionID string, mutate func(*Record)) error {
	prev, found, err := Get(taskID)
	if err != nil {
		return err
	}
	if !found {
		prev = Record{TaskID: taskID, SessionID: sessionID}
	}
	if sessionID != "" {
		prev.SessionID = sessionID
	}
	mutate(&prev)
	return Append(prev)
}

// Load returns the collapsed ledger: one record per task_id, the newest
// line winning, sorted newest-updated first.
//
// Unparseable lines are skipped rather than failing the read. A ledger
// truncated by a crash or written by a future version should still yield
// the records it does contain.
func Load() ([]Record, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	byKey := map[string]Record{}
	sc := bufio.NewScanner(f)
	// Records are small, but a pathological line should not abort the read.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		k := key(r)
		if prev, ok := byKey[k]; ok && prev.UpdatedAt.After(r.UpdatedAt) {
			continue
		}
		byKey[k] = r
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	out := make([]Record, 0, len(byKey))
	for _, r := range byKey {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].TaskID > out[j].TaskID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// Get returns the record for taskID.
func Get(taskID int64) (Record, bool, error) {
	all, err := Load()
	if err != nil {
		return Record{}, false, err
	}
	for _, r := range all {
		if r.TaskID == taskID {
			return r, true, nil
		}
	}
	return Record{}, false, nil
}

// GetBySession returns the record for a session_id, for callers that have
// the session half of the pair but not the task half.
func GetBySession(sessionID string) (Record, bool, error) {
	all, err := Load()
	if err != nil {
		return Record{}, false, err
	}
	for _, r := range all {
		if r.SessionID == sessionID {
			return r, true, nil
		}
	}
	return Record{}, false, nil
}

// Latest returns the most recently updated record.
func Latest() (Record, bool, error) {
	all, err := Load()
	if err != nil || len(all) == 0 {
		return Record{}, false, err
	}
	return all[0], true, nil
}

// PruneOptions selects which records `vk jobs prune` removes.
type PruneOptions struct {
	// OlderThan drops records not updated within this window. Zero means
	// no age filter.
	OlderThan time.Duration
	// TerminalOnly keeps records that never reached a terminal state, so a
	// routine prune cannot lose the pointer to a run that is still going.
	TerminalOnly bool
	// All drops everything, ignoring the other fields.
	All bool
}

// Prune rewrites the ledger without the matching records and returns how
// many it removed along with what remains.
func Prune(opts PruneOptions) (removed int, kept []Record, err error) {
	all, err := Load()
	if err != nil {
		return 0, nil, err
	}
	cutoff := time.Time{}
	if opts.OlderThan > 0 {
		cutoff = time.Now().UTC().Add(-opts.OlderThan)
	}
	for _, r := range all {
		drop := opts.All
		if !drop {
			ageOK := cutoff.IsZero() || r.UpdatedAt.Before(cutoff)
			stateOK := !opts.TerminalOnly || r.Terminal()
			// With no age filter and no All, a bare prune would drop
			// everything terminal; that is the documented behaviour of
			// `vk jobs prune --terminal`.
			drop = ageOK && stateOK && (!cutoff.IsZero() || opts.TerminalOnly)
		}
		if drop {
			removed++
			continue
		}
		kept = append(kept, r)
	}
	if removed == 0 {
		return 0, kept, nil
	}
	p, err := Path()
	if err != nil {
		return 0, nil, err
	}
	if err := writeAll(p, kept); err != nil {
		return 0, nil, err
	}
	return removed, kept, nil
}

func key(r Record) string {
	if r.TaskID != 0 {
		return fmt.Sprintf("t:%d", r.TaskID)
	}
	return "s:" + r.SessionID
}

func lineCount(p string) (int, error) {
	f, err := os.Open(p)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}

func compact(p string) error {
	all, err := Load()
	if err != nil {
		return err
	}
	return writeAll(p, all)
}

// writeAll rewrites the ledger atomically: a crash mid-compaction leaves
// the previous file intact rather than a half-written one.
func writeAll(p string, records []Record) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), ".jobs-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	w := bufio.NewWriter(tmp)
	// Oldest first, so the file reads chronologically and an append still
	// lands newest-last.
	for i := len(records) - 1; i >= 0; i-- {
		line, err := json.Marshal(records[i])
		if err != nil {
			tmp.Close()
			return err
		}
		if _, err := w.Write(append(line, '\n')); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, p)
}

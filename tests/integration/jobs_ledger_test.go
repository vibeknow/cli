package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ledgerStub is a full happy-path figlens: init, prompt optimize, a stream
// that succeeds, and a work row. It records the session_id the last stream
// request carried so a test can prove the ledger fed it back.
func ledgerStub(t *testing.T, lastStreamSession *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 4242, "session_id": "s_ledger", "work_id": 77, "v": 3},
			})

		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"answer_done\":{\"text\":\"p\"}}}\n\ndata: [DONE]\n\n")

		case "/v1/agent3forVideo/stream":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if lastStreamSession != nil {
				if s, ok := body["session_id"].(string); ok {
					lastStreamSession.Store(s)
				}
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"process\",\"log\":{\"step_id\":\"plan\",\"status\":\"start\",\"message\":\"go\"}}}\n\n")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"session_id\":\"s_ledger\"}}\n\ndata: [DONE]\n\n")

		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 77, "session_id": "s_ledger",
					"title": "Ledger Test", "share_token": "tok_ledger",
					"status": 1, "exporting": 0,
				},
			})

		default:
			w.WriteHeader(404)
		}
	}))
}

// TestJobsLedger_CreateRecordsTheRun_AndWaitResolvesIt is the end-to-end
// case the ledger exists for: after `vk create`, a caller that kept nothing
// from its output can still reach the run.
//
// Before this, --session-id was mandatory on every video subcommand, so
// losing that one string — an agent's context trimmed, a terminal closed —
// made a live, billed run unreachable.
func TestJobsLedger_CreateRecordsTheRun_AndWaitResolvesIt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var lastStreamSession atomic.Value
	srv := ledgerStub(t, &lastStreamSession)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_ledger123", "--kb-id", "kb_test", "--output", "json")
	if code != 0 {
		t.Fatalf("create exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// 1. The run is in the ledger, with its terminal state.
	stdout, stderr, code = runVideoCmd(t, bin, configHome, "jobs", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("jobs list exit %d: %s", code, stderr)
	}
	var listed map[string]any
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatalf("bad json %q: %v", stdout, err)
	}
	items, _ := listed["jobs"].([]any)
	if len(items) != 1 {
		t.Fatalf("want exactly the one run, got: %v", listed)
	}
	rec, _ := items[0].(map[string]any)
	if rec["task_id"] != float64(4242) || rec["session_id"] != "s_ledger" {
		t.Fatalf("the ledger must carry both halves of the pair: %v", rec)
	}
	if rec["status"] != "succeeded" {
		t.Fatalf("status = %v, want succeeded (the run finished during create)", rec["status"])
	}
	if rec["share_url"] == nil {
		t.Fatalf("a succeeded run should record its share_url: %v", rec)
	}
	if rec["source"] != "doc_ledger123" {
		t.Fatalf("source = %v, want the --from value so the row is identifiable", rec["source"])
	}

	// 2. `vk video wait` with neither task_id nor --session-id resolves the
	//    pair from the ledger and reaches the same run.
	lastStreamSession.Store("")
	stdout, stderr, code = runVideoCmd(t, bin, configHome, "video", "wait", "--output", "json")
	if code != 0 {
		t.Fatalf("bare `video wait` should resolve from the ledger, exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if got := lastStreamSession.Load().(string); got != "s_ledger" {
		t.Fatalf("the stream was opened with session_id=%q, want s_ledger from the ledger", got)
	}
	// The resolution is announced, so a user never wonders which run was hit.
	if !strings.Contains(stderr, "using recorded run") {
		t.Fatalf("resolving from the ledger must be visible on stderr, got: %s", stderr)
	}
}

// TestJobsLedger_ExplicitSessionWins pins the precedence rule: a stale or
// wrong ledger entry can never override what the caller passed.
func TestJobsLedger_ExplicitSessionWins(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var lastStreamSession atomic.Value
	srv := ledgerStub(t, &lastStreamSession)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	if _, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_ledger123", "--kb-id", "kb_test"); code != 0 {
		t.Fatalf("create exit %d: %s", code, stderr)
	}

	lastStreamSession.Store("")
	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "wait", "999", "--session-id", "s_explicit")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := lastStreamSession.Load().(string); got != "s_explicit" {
		t.Fatalf("explicit --session-id must win over the ledger, stream used %q", got)
	}
	if strings.Contains(stderr, "using recorded run") {
		t.Fatalf("the ledger should not have been consulted at all, got: %s", stderr)
	}
}

// TestJobsLedger_EmptyLedgerTeachesTheFix covers the degraded case. With no
// recorded run there is nothing to fall back on, and the error has to say
// what to do instead — otherwise dropping the --session-id requirement just
// trades one unhelpful failure for another.
func TestJobsLedger_EmptyLedgerTeachesTheFix(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildVideoProfile(t, "http://127.0.0.1:1")

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "video", "status")
	if code != 2 {
		t.Fatalf("want validation exit 2, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "--session-id") || !strings.Contains(stderr, "vk jobs list") {
		t.Fatalf("the error must name both the flag and the way to look it up, got: %s", stderr)
	}
}

// TestJobsPrune_RefusesToRunUnfiltered: a bare prune would drop the pointer
// to runs still in flight, which is not recoverable from the CLI.
func TestJobsPrune_RefusesToRunUnfiltered(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildVideoProfile(t, "http://127.0.0.1:1")

	_, stderr, code := runVideoCmd(t, bin, configHome, "jobs", "prune")
	if code != 2 {
		t.Fatalf("unfiltered prune should be refused with exit 2, got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--terminal") {
		t.Fatalf("the refusal should offer the safe filter, got: %s", stderr)
	}
}

// TestJobsPrune_TerminalKeepsActive walks the real sequence: record a
// finished run, prune the terminal ones, confirm it is gone.
func TestJobsPrune_TerminalKeepsActive(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := ledgerStub(t, nil)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	if _, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_ledger123", "--kb-id", "kb_test"); code != 0 {
		t.Fatalf("create exit %d: %s", code, stderr)
	}

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "jobs", "prune", "--terminal", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("bad json %q: %v", stdout, err)
	}
	if res["removed"] != float64(1) || res["remaining"] != float64(0) {
		t.Fatalf("want removed=1 remaining=0, got %v", res)
	}

	stdout, _, _ = runVideoCmd(t, bin, configHome, "jobs", "list", "--output", "json")
	var listed map[string]any
	_ = json.Unmarshal([]byte(stdout), &listed)
	if listed["total"] != float64(0) {
		t.Fatalf("prune did not persist: %v", listed)
	}
}

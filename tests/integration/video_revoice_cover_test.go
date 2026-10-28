package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// revoiceStub plays the backend for one source work (task 42, session
// s_src) and the new work a re-voice creates (session s_new). The job's
// final status is chosen per test.
type revoiceStub struct {
	server *httptest.Server

	mu        sync.Mutex
	finalAs   string           // status the job settles in
	job       map[string]any   // nil until a re-voice exists
	creates   []map[string]any // POST /works/revoice bodies
	retries   int
	supported bool
}

func newRevoiceStub(t *testing.T, finalAs string) *revoiceStub {
	t.Helper()
	s := &revoiceStub{finalAs: finalAs, supported: true}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

func (s *revoiceStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	ok := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data}) }
	switch r.URL.Path {
	case "/v1/works/detailBySession":
		if r.URL.Query().Get("session_id") == "s_new" {
			ok(map[string]any{"id": 99, "task_id": 42, "session_id": "s_new", "title": "季度报告", "status": 1,
				"share_token": "tok_new", "engine": "suite"})
			return
		}
		ok(map[string]any{"id": 77, "task_id": 42, "session_id": "s_src", "title": "季度报告", "status": 1,
			"duration": 60000, "engine": "suite"})
	case "/v1/works/revoiceCapability":
		reason := ""
		if !s.supported {
			reason = "作品正在导出，请导出完成后再换声音"
		}
		ok(map[string]any{"supported": s.supported, "reason": reason, "locked_language": "zh-CN",
			"has_avatar": true, "source_revision": "rev1", "current_voice_id": "v_old"})
	case "/v1/works/revoice":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.creates = append(s.creates, body)
		s.job = map[string]any{"id": "job1", "task_id": 42, "work_id": 99, "session_id": "s_new",
			"source_work_id": 77, "source_revision": body["source_revision"], "voice_id": body["voice_id"],
			"voice_name": "若溪", "language": "zh-CN", "status": s.finalAs, "stage": "tts"}
		ok(s.job)
	case "/v1/works/revoiceRetry":
		s.retries++
		s.job["status"] = "completed"
		ok(s.job)
	case "/v1/works/revoiceJobs":
		if s.job == nil || s.finalAs == "vanish" {
			ok([]any{})
			return
		}
		ok([]any{s.job})
	default:
		w.WriteHeader(404)
	}
}

func (s *revoiceStub) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.creates)
}

func (s *revoiceStub) firstCreate() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates[0]
}

func (s *revoiceStub) retryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retries
}

func (s *revoiceStub) setSupported(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.supported = v
}

func revoiceArgs(extra ...string) []string {
	return append([]string{"video", "revoice", "42", "--session-id", "s_src", "--voice", "v_new", "--output", "json"}, extra...)
}

// The gate comes before anything is started, and what it asks about pins
// the version of the work and the voice.
func TestVideoRevoice_BlocksBeforeStartingAnything(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newRevoiceStub(t, "completed")
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, revoiceArgs()...)
	if code != 8 {
		t.Fatalf("exit %d, want 8\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if stub.createCount() != 0 {
		t.Fatal("a blocked re-voice was started anyway")
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(stdout), &got)
	action := got["pending_actions"].([]any)[0].(map[string]any)
	if action["type"] != "revoice_confirmation" {
		t.Errorf("type = %v", action["type"])
	}
	payload := action["payload"].(map[string]any)
	if payload["source_revision"] != "rev1" || payload["voice_id"] != "v_new" || payload["has_avatar"] != true {
		t.Errorf("payload = %v", payload)
	}
	if ctx, _ := action["context"].(map[string]any); ctx["duration_sec"] != float64(60) {
		t.Errorf("context = %v, want the narration length", action["context"])
	}
	if msg, _ := action["message"].(string); !strings.Contains(msg, "lip-sync") {
		t.Errorf("message does not mention the presenter's lip-sync: %q", msg)
	}
}

// A confirmed re-voice reports the new work, names the original, and is
// recorded locally; the request carries a key derived from the request.
func TestVideoRevoice_ConfirmedRunsAndReportsBothWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newRevoiceStub(t, "completed")
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, revoiceArgs("--yes")...)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	body := stub.firstCreate()
	if body["work_id"] != float64(77) || body["voice_id"] != "v_new" {
		t.Errorf("create body = %v", body)
	}
	key, _ := body["idempotency_key"].(string)
	if !strings.HasPrefix(key, "vk-revoice-") {
		t.Errorf("idempotency_key = %q", key)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if got["session_id"] != "s_new" {
		t.Errorf("session_id = %v, want the new work's", got["session_id"])
	}
	rv, _ := got["revoice"].(map[string]any)
	if rv["source_session_id"] != "s_src" {
		t.Errorf("revoice = %v, want the original named", got["revoice"])
	}

	ledger, _, _ := runVideoCmd(t, bin, configHome, "jobs", "list", "--output", "json")
	if !strings.Contains(ledger, "s_new") {
		t.Errorf("new run not recorded in the ledger: %s", ledger)
	}

	// Re-running the same request attaches to the finished run: no new
	// start, and no consent needed because nothing is spent.
	stdout, stderr, code = runVideoCmd(t, bin, configHome, revoiceArgs()...)
	if code != 0 || stub.createCount() != 1 {
		t.Fatalf("re-run: exit %d, creates %d (want 0, 1)\nstdout: %s\nstderr: %s", code, stub.createCount(), stdout, stderr)
	}
}

// A failed run is retried by re-running — but a retry bills, so it asks.
func TestVideoRevoice_FailedRunRetriesOnlyWithConsent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newRevoiceStub(t, "failed")
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	_, stderr, code := runVideoCmd(t, bin, configHome, revoiceArgs("--yes")...)
	if code != 5 {
		t.Fatalf("failed run: exit %d, want 5\nstderr: %s", code, stderr)
	}
	stdout, _, code := runVideoCmd(t, bin, configHome, revoiceArgs()...)
	if code != 8 || stub.retryCount() != 0 {
		t.Fatalf("retry without consent: exit %d, retries %d (want 8, 0)\nstdout: %s", code, stub.retryCount(), stdout)
	}
	token := actionIDRe.FindString(stdout)
	_, stderr, code = runVideoCmd(t, bin, configHome, revoiceArgs("--confirm", token)...)
	if code != 0 || stub.retryCount() != 1 || stub.createCount() != 1 {
		t.Fatalf("retry: exit %d, retries %d, creates %d\nstderr: %s", code, stub.retryCount(), stub.createCount(), stderr)
	}
}

func TestVideoRevoice_RefusalsCostNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newRevoiceStub(t, "completed")
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	_, _, code := runVideoCmd(t, bin, configHome, "video", "revoice", "42", "--session-id", "s_src", "--voice", "v_old", "--yes")
	if code != 2 {
		t.Errorf("same voice: exit %d, want 2", code)
	}
	stub.setSupported(false)
	_, stderr, code := runVideoCmd(t, bin, configHome, revoiceArgs("--yes")...)
	if code != 5 || !strings.Contains(stderr, "导出") {
		t.Errorf("unsupported: exit %d, want 5 with the backend's reason\nstderr: %s", code, stderr)
	}
	if stub.createCount() != 0 {
		t.Error("a refused re-voice was started")
	}
}

func TestVideoRevoice_NeedsBudgetExits5(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newRevoiceStub(t, "needs_budget")
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)
	_, stderr, code := runVideoCmd(t, bin, configHome, revoiceArgs("--yes")...)
	if code != 5 {
		t.Fatalf("exit %d, want 5\nstderr: %s", code, stderr)
	}
}

// coverStub answers the work read and the cover stream; end decides how the
// stream finishes: "ok", "error", or "drop" (closes with no result).
type coverStub struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  int
	body   map[string]any
}

func (s *coverStub) seen() (int, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.body
}

func newCoverStub(t *testing.T, end string, exported bool) *coverStub {
	return newCoverStubWithStatus(t, end, exported, 1)
}

func newCoverStubWithStatus(t *testing.T, end string, exported bool, status int) *coverStub {
	t.Helper()
	s := &coverStub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			work := map[string]any{"id": 77, "task_id": 42, "session_id": "s_src", "title": "季度报告", "status": status}
			if exported {
				work["video_path"] = "https://cdn.test/old.mp4"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": work})
		case "/v1/works/generateCoverStream":
			s.mu.Lock()
			s.calls++
			_ = json.NewDecoder(r.Body).Decode(&s.body)
			s.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"keepalive\"}}\n\n")
			switch end {
			case "ok":
				fmt.Fprint(w, `event: data`+"\n"+`data: {"code":200,"data":{"type":"cover_gen_completed","data":{"id":77,"cover_url":"https://cdn.test/new.jpg","cover_urls":["https://cdn.test/old.jpg","https://cdn.test/new.jpg"],"video_path":""}}}`+"\n\n")
			case "error":
				fmt.Fprint(w, `event: data`+"\n"+`data: {"code":200,"data":{"msg_type":"ERROR","msg":"生图失败"}}`+"\n\n")
			}
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func TestVideoCover_BlocksThenGenerates(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newCoverStub(t, "ok", true)
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	args := []string{"video", "cover", "42", "--session-id", "s_src", "--output", "json"}
	stdout, stderr, code := runVideoCmd(t, bin, configHome, args...)
	if calls, _ := stub.seen(); code != 8 || calls != 0 {
		t.Fatalf("exit %d, calls %d (want 8, 0)\nstdout: %s\nstderr: %s", code, calls, stdout, stderr)
	}
	var blocked map[string]any
	_ = json.Unmarshal([]byte(stdout), &blocked)
	action := blocked["pending_actions"].([]any)[0].(map[string]any)
	payload := action["payload"].(map[string]any)
	if action["type"] != "cover_confirmation" || payload["title"] != "季度报告" {
		t.Errorf("action = %v", action)
	}
	if msg, _ := action["message"].(string); !strings.Contains(msg, "withdrawn") {
		t.Errorf("message does not warn that the exported MP4 goes: %q", msg)
	}

	token := actionIDRe.FindString(stdout)
	stdout, stderr, code = runVideoCmd(t, bin, configHome, append(args, "--confirm", token)...)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if _, body := stub.seen(); body["title"] != "季度报告" || body["id"] != float64(77) {
		t.Errorf("request body = %v", body)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(stdout), &got)
	if got["cover_url"] != "https://cdn.test/new.jpg" || got["export_cleared"] != true {
		t.Errorf("result = %v", got)
	}
}

// A stream that ends with no result is not "nothing happened": the backend
// finishes and bills the generation regardless. It must not invite a retry.
func TestVideoCover_DroppedStreamIsUnknownNotRetryable(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newCoverStub(t, "drop", false)
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)
	stdout, stderr, code := runVideoCmd(t, bin, configHome, "video", "cover", "42", "--session-id", "s_src", "--yes", "--output", "json")
	if code != 6 {
		t.Fatalf("exit %d, want 6\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "resend_safe") {
		t.Errorf("no resend_safe verdict\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestVideoCover_GenerationFailureIsNotUnknown(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newCoverStub(t, "error", false)
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)
	_, stderr, code := runVideoCmd(t, bin, configHome, "video", "cover", "42", "--session-id", "s_src", "--yes")
	if code == 0 || code == 6 || !strings.Contains(stderr, "生图失败") {
		t.Fatalf("exit %d, want a failure carrying the backend's reason\nstderr: %s", code, stderr)
	}
}

// A failed work is terminal: "wait and check again" (exit 6) would send an
// agent into a loop that can never end.
func TestVideoCover_WorkStateExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)
	for _, tc := range []struct{ status, want int }{{3, 5}, {0, 6}} {
		stub := newCoverStubWithStatus(t, "ok", false, tc.status)
		configHome := buildVideoProfile(t, stub.server.URL)
		_, stderr, code := runVideoCmd(t, bin, configHome, "video", "cover", "42", "--session-id", "s_src", "--yes")
		if calls, _ := stub.seen(); code != tc.want || calls != 0 {
			t.Errorf("work status %d: exit %d, calls %d (want %d, 0)\nstderr: %s", tc.status, code, calls, tc.want, stderr)
		}
	}
}

// A job that stops being listed must end the wait, not leave it polling a
// status it can no longer see change.
func TestVideoRevoice_VanishedJobDoesNotHang(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newRevoiceStub(t, "vanish")
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)
	_, stderr, code := runVideoCmd(t, bin, configHome, revoiceArgs("--yes")...)
	if code != 6 || !strings.Contains(stderr, "s_new") {
		t.Fatalf("exit %d, want 6 naming the new session\nstderr: %s", code, stderr)
	}
}

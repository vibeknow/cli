package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// channelStub is agentStub plus the endpoints the preview, export and
// backend-fallback paths need: a cover image, a signed-URL indirection, an
// export task that succeeds, and a non-empty work list.
func channelStub(t *testing.T, coverBody string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	// The work row only gains a video_path once an export has run, which is
	// what makes "deliver the MP4" reachable at all. Modelling that ordering
	// matters: a stub that always carried it would hide a CLI that delivered
	// the file before one existed.
	var exported atomic.Bool
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"task_id": 4242, "session_id": "s_chan", "work_id": 77, "v": 3},
			})
		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"answer_done\":{\"text\":\"p\"}}}\n\ndata: [DONE]\n\n")
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"process\",\"log\":{\"step_id\":\"script_writing\",\"status\":\"start\",\"message\":\"go\"}}}\n\n")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"process\",\"log\":{\"step_id\":\"script_writing\",\"status\":\"success\",\"message\":\"done\"}}}\n\n")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"session_id\":\"s_chan\"}}\n\ndata: [DONE]\n\n")
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			row := map[string]any{"id": 77, "session_id": "s_chan", "title": "Chan",
				"share_token": "tok_chan", "status": 1, "exporting": 0,
				"cover_url": srv.URL + "/assets/cover.jpg"}
			if exported.Load() {
				row["video_path"] = "obj/final.mp4"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": row})
		case "/assets/cover.jpg":
			_, _ = w.Write([]byte(coverBody))
		case "/v1/works/page":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{
					"list":  []any{map[string]any{"id": 77, "session_id": "s_chan", "title": "Chan", "status": 1}},
					"total": 1,
				},
			})
		case "/v1/agent2forVideo/exportRemoteV2":
			exported.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"task_id": 555}})
		case "/v1/agent2forVideo/exportResultV2":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"status": "completed", "progress": 100, "video_path": "obj/final.mp4"},
			})
		case "/v1/agent2forVideo/signedUrl":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"url": srv.URL + "/assets/final.mp4?sig=abc"},
			})
		case "/assets/final.mp4":
			_, _ = w.Write([]byte("mp4-bytes"))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// eventLines returns the structured events found on a stderr stream.
func eventLines(t *testing.T, stderr string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(stderr, "\n") {
		i := strings.Index(line, "vk_event=")
		if i < 0 {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line[i+len("vk_event="):]), &ev); err != nil {
			t.Fatalf("a vk_event line must be parseable JSON: %v\n%s", err, line)
		}
		out = append(out, ev)
	}
	return out
}

// TestJSONModeSplitsAnswerFromProgress is the contract this whole channel
// exists for: a caller gets the run as it happens *and* a clean result from
// one invocation, without having to pick a format that gives up one of them.
//
// Before, `--output json` was silent until the end and `--output ndjson`
// put the stream on stdout, where a consumer had to work out for itself
// which of N lines was the answer.
func TestJSONModeSplitsAnswerFromProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_chan12345", "--kb-id", "kb_x", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// stdout is exactly one JSON document. Nothing else may share it.
	var answer map[string]any
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&answer); err != nil {
		t.Fatalf("stdout must be one JSON doc: %v\n%q", err, stdout)
	}
	if dec.More() {
		t.Fatalf("stdout carried more than the answer:\n%q", stdout)
	}
	if answer["session_id"] != "s_chan" {
		t.Fatalf("stdout should be the snapshot, got %v", answer)
	}

	// stderr carried the run.
	evs := eventLines(t, stderr)
	if len(evs) == 0 {
		t.Fatalf("no vk_event lines on stderr; json callers got no progress at all:\n%s", stderr)
	}
	var sawNode, sawTerminal bool
	for _, ev := range evs {
		switch ev["type"] {
		case "node.started", "node.succeeded":
			sawNode = true
		case "task.succeeded":
			sawTerminal = true
		}
		if _, ok := ev["schema_version"]; !ok {
			t.Fatalf("stderr events must carry schema_version like the stdout stream: %v", ev)
		}
	}
	if !sawNode || !sawTerminal {
		t.Fatalf("progress incomplete: node=%v terminal=%v\n%s", sawNode, sawTerminal, stderr)
	}
}

// The human rendering and the machine rendering are two spellings of one
// fact. Emitting both would make stderr unreadable and the events noisy.
func TestTextModeKeepsProseAndEmitsNoEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_chan12345", "--kb-id", "kb_x")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if evs := eventLines(t, stderr); len(evs) != 0 {
		t.Fatalf("text mode must stay human-readable, found %d events:\n%s", len(evs), stderr)
	}
}

func TestEventsEnvForcesTheChannelOn(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	_, stderr, code := runCmdEnv(t, bin, configHome, []string{"VIBEKNOW_EVENTS=1"},
		"create", "--from", "doc_chan12345", "--kb-id", "kb_x")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if evs := eventLines(t, stderr); len(evs) == 0 {
		t.Fatalf("VIBEKNOW_EVENTS=1 must turn the channel on in text mode too:\n%s", stderr)
	}
}

// TestPreviewDirDeliversSomethingToLookAt: share_url is a hosted page, which
// an agent in a terminal cannot show anyone. With --preview-dir there is a
// real file, at an absolute path, that is finished being written.
func TestPreviewDirDeliversSomethingToLookAt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)
	dir := t.TempDir()

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_chan12345", "--kb-id", "kb_x",
		"--output", "json", "--preview-dir", dir)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}

	var ready map[string]any
	for _, ev := range eventLines(t, stderr) {
		if ev["type"] == "resource_ready" {
			ready = ev
		}
	}
	if ready == nil {
		t.Fatalf("no resource_ready event:\n%s", stderr)
	}
	if ready["asset_kind"] != "cover_image" {
		t.Fatalf("asset_kind = %v, want cover_image", ready["asset_kind"])
	}
	p, _ := ready["local_path"].(string)
	if !filepath.IsAbs(p) {
		t.Fatalf("local_path %q must be absolute", p)
	}
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("the announced file must exist and be complete: %v", err)
	}
	if string(body) != "cover-v1" {
		t.Fatalf("file holds %q", body)
	}
	// A signed URL relayed by an agent is a published credential.
	if strings.Contains(stderr, "/assets/cover.jpg") {
		t.Fatalf("the source URL leaked into the event stream:\n%s", stderr)
	}
}

// Re-running a command into the same directory must not re-announce
// unchanged bytes, or an agent polling shows the user the same still again.
func TestPreviewDirDoesNotReDeliverUnchangedContent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)
	dir := t.TempDir()

	args := []string{"create", "--from", "doc_chan12345", "--kb-id", "kb_x", "--output", "json", "--preview-dir", dir}
	if _, stderr, code := runVideoCmd(t, bin, configHome, args...); code != 0 {
		t.Fatalf("first run exit %d: %s", code, stderr)
	}
	_, stderr, code := runVideoCmd(t, bin, configHome, args...)
	if code != 0 {
		t.Fatalf("second run exit %d: %s", code, stderr)
	}
	for _, ev := range eventLines(t, stderr) {
		if ev["type"] == "resource_ready" {
			t.Fatalf("re-announced identical content:\n%s", stderr)
		}
	}
}

// TestExportWithoutTTYHandsBackTheDecision is the behaviour change: a paid
// render used to proceed unattended with a note on stderr nobody read.
func TestExportWithoutTTYHandsBackTheDecision(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_chan", "--output", "json")
	if code != 8 {
		t.Fatalf("a blocked spend must exit 8, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("the decision is the command's result and belongs on stdout: %v\n%q", err, stdout)
	}
	if out["status"] != "blocked" {
		t.Fatalf("status = %v, want blocked", out["status"])
	}
	actions, _ := out["pending_actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("pending_actions = %v", out["pending_actions"])
	}
	a, _ := actions[0].(map[string]any)
	if a["blocking"] != true {
		t.Fatal("the action must say it blocks; otherwise a caller may treat it as advisory")
	}
	id, _ := a["action_id"].(string)
	if !strings.HasPrefix(id, "act_") {
		t.Fatalf("action_id = %q", id)
	}
	// The cost must be in front of whoever is being asked.
	pay, _ := a["payload"].(map[string]any)
	if pay["credits"] == nil {
		t.Fatalf("payload must state what this costs: %v", pay)
	}
	// And the exact command that proceeds, not a description of it.
	resume, _ := a["resume_command"].(string)
	if !strings.Contains(resume, "--confirm "+id) {
		t.Fatalf("resume_command = %q, must carry the token", resume)
	}
	opts, _ := a["options"].([]any)
	if len(opts) != 2 {
		t.Fatalf("options = %v, want confirm and cancel", opts)
	}
}

// The round trip: relay the token back and the spend goes through.
func TestExportResumesWithTheIssuedToken(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, _, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_chan", "--output", "json")
	if code != 8 {
		t.Fatalf("expected a block first, got %d", code)
	}
	var blocked map[string]any
	_ = json.Unmarshal([]byte(stdout), &blocked)
	action := blocked["pending_actions"].([]any)[0].(map[string]any)
	token := action["action_id"].(string)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_chan", "--confirm", token, "--output", "json")
	if code != 0 {
		t.Fatalf("a valid token must proceed, exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("bad json: %v\n%q", err, stdout)
	}
	exp, _ := out["export"].(map[string]any)
	if exp["status"] != "succeeded" {
		t.Fatalf("export status = %v", exp["status"])
	}
}

// The failure this exists to prevent: a model producing consent by
// reasoning rather than by relaying. Nothing guessable may work.
func TestExportRejectsAnInventedToken(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	for _, bogus := range []string{"yes", "act_confirm", "act_0000000000000000000000", "confirm"} {
		t.Run(bogus, func(t *testing.T) {
			_, stderr, code := runVideoCmd(t, bin, configHome,
				"video", "export", "42", "--session-id", "s_chan", "--confirm", bogus, "--output", "json")
			if code != 2 {
				t.Fatalf("a guessed token must be rejected with exit 2, got %d: %s", code, stderr)
			}
		})
	}
}

// A token is consent to one run, not to spending in general.
func TestExportTokenDoesNotTransferBetweenRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, _, _ := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_chan", "--output", "json")
	var blocked map[string]any
	_ = json.Unmarshal([]byte(stdout), &blocked)
	token := blocked["pending_actions"].([]any)[0].(map[string]any)["action_id"].(string)

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "99", "--session-id", "s_other", "--confirm", token, "--output", "json")
	if code != 2 {
		t.Fatalf("a token minted for s_chan must not authorise s_other, got exit %d: %s", code, stderr)
	}
}

// --yes remains the way a caller that already has authority says so. Every
// existing agent script uses it, and the block must not break them.
func TestExportYesStillProceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	if _, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_chan", "--yes", "--output", "json"); code != 0 {
		t.Fatalf("--yes must still work, exit %d: %s", code, stderr)
	}
	if _, stderr, code := runCmdEnv(t, bin, configHome, []string{"VIBEKNOW_ASSUME_YES=1"},
		"video", "export", "42", "--session-id", "s_chan", "--output", "json"); code != 0 {
		t.Fatalf("VIBEKNOW_ASSUME_YES must still work, exit %d: %s", code, stderr)
	}
}

// TestLostLedgerFallsBackToTheAccount: the ledger is per-machine. An agent
// that moved hosts still has the run — through the account, not the file.
func TestLostLedgerFallsBackToTheAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL) // no jobs.jsonl at all

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "video", "status", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "no local record") {
		t.Fatalf("the fallback must say it happened, or a caller cannot tell which run it got:\n%s", stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("bad json: %v\n%q", err, stdout)
	}
	if out["session_id"] != "s_chan" {
		t.Fatalf("session_id = %v, want the account's most recent run", out["session_id"])
	}
}

// A task_id cannot be resolved remotely — the backend list carries no
// task_id — so the error must say what will actually work instead of
// implying a lookup that cannot happen.
func TestTaskIDWithoutLedgerSaysWhatToDoInstead(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	_, stderr, code := runVideoCmd(t, bin, configHome, "video", "status", "12345")
	if code != 2 {
		t.Fatalf("exit %d, want 2: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--session-id") {
		t.Fatalf("the hint must name the flag that resolves this:\n%s", stderr)
	}
}

// silentStub accepts a run and then says nothing — the shape that used to
// leave `vk create` exiting 0 with an empty stdout. workStatus selects what
// the backend admits to when the CLI comes back to ask.
func silentStub(t *testing.T, workFound bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"task_id": 4242, "session_id": "s_quiet", "work_id": 77, "v": 3},
			})
		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"answer_done\":{\"text\":\"p\"}}}\n\ndata: [DONE]\n\n")
		case "/v1/agent3forVideo/stream":
			// Opens, closes, never says a word.
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: [DONE]\n\n")
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			if !workFound {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 40400, "message": "no such work"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"id": 77, "session_id": "s_quiet", "status": 0},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// errDetail pulls the machine-readable detail out of a JSON error envelope.
func errDetail(t *testing.T, stderr string) map[string]any {
	t.Helper()
	i := strings.Index(stderr, "{")
	if i < 0 {
		t.Fatalf("no JSON error envelope on stderr:\n%s", stderr)
	}
	var env struct {
		Error struct {
			Detail map[string]any `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr[i:]), &env); err != nil {
		t.Fatalf("bad error envelope: %v\n%s", err, stderr[i:])
	}
	return env.Error.Detail
}

// TestUnknownStateSaysWhetherAResendIsSafe: exit 6 said "state unknown",
// which is true and not actionable. The caller's real question is whether
// running `vk create` again recovers a lost run or pays for a second one,
// and only the backend can answer it.
func TestUnknownStateSaysWhetherAResendIsSafe(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)

	t.Run("backend still has the run", func(t *testing.T) {
		srv := silentStub(t, true)
		configHome := buildVideoProfile(t, srv.URL)

		_, stderr, code := runVideoCmd(t, bin, configHome,
			"create", "--from", "doc_quiet12345", "--kb-id", "kb_x", "--output", "json")
		if code != 6 {
			t.Fatalf("exit %d, want 6: %s", code, stderr)
		}
		d := errDetail(t, stderr)
		if d["delivery"] != "submitted" {
			t.Fatalf("delivery = %v, want submitted", d["delivery"])
		}
		if d["resend_safe"] != false {
			t.Fatalf("resend_safe = %v — a second create here is a second billed render", d["resend_safe"])
		}
		if d["backend_status"] != "running" {
			t.Fatalf("backend_status = %v", d["backend_status"])
		}
		if d["next_actions"] == nil {
			t.Fatalf("no next_actions; the caller is told not to resend but not what to do: %v", d)
		}
	})

	t.Run("backend never heard of it", func(t *testing.T) {
		srv := silentStub(t, false)
		configHome := buildVideoProfile(t, srv.URL)

		_, stderr, code := runVideoCmd(t, bin, configHome,
			"create", "--from", "doc_quiet12345", "--kb-id", "kb_x", "--output", "json")
		if code != 6 {
			t.Fatalf("exit %d, want 6: %s", code, stderr)
		}
		d := errDetail(t, stderr)
		if d["delivery"] != "not_submitted" {
			t.Fatalf("delivery = %v, want not_submitted", d["delivery"])
		}
		if d["resend_safe"] != true {
			t.Fatalf("resend_safe = %v — nothing was billed, so starting over is free", d["resend_safe"])
		}
	})
}

// A render takes minutes. A json caller watching one must not be left with
// a silent stderr for the whole wait — that is the same silence the
// structured channel exists to remove.
func TestExportProgressReachesTheStructuredChannel(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_chan", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var sawTerminal bool
	for _, ev := range eventLines(t, stderr) {
		if ev["type"] == "export.succeeded" {
			sawTerminal = true
			if ev["export_task_id"] == nil {
				t.Fatalf("export events must name the task they describe: %v", ev)
			}
		}
	}
	if !sawTerminal {
		t.Fatalf("no export progress on the structured channel:\n%s", stderr)
	}
}

// --preview-dir on export delivers the MP4 without a separate `download`
// step, and does not re-announce the cover a previous command already gave
// the caller.
func TestExportPreviewDirDeliversTheMP4Only(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := channelStub(t, "cover-v1")
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)
	dir := t.TempDir()

	// First the cover, via a create into the same directory.
	if _, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_chan12345", "--kb-id", "kb_x", "--output", "json", "--preview-dir", dir); code != 0 {
		t.Fatalf("create exit %d: %s", code, stderr)
	}

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_chan", "--yes", "--output", "json", "--preview-dir", dir)
	if code != 0 {
		t.Fatalf("export exit %d: %s", code, stderr)
	}

	var kinds []string
	for _, ev := range eventLines(t, stderr) {
		if ev["type"] == "resource_ready" {
			kinds = append(kinds, ev["asset_kind"].(string))
		}
	}
	if len(kinds) != 1 || kinds[0] != "video_playback" {
		t.Fatalf("resource_ready kinds = %v, want exactly [video_playback] — the cover was already delivered", kinds)
	}
	body, err := os.ReadFile(filepath.Join(dir, "s_chan-video_playback.mp4"))
	if err != nil || string(body) != "mp4-bytes" {
		t.Fatalf("the MP4 should be on disk without a separate download step: %q %v", body, err)
	}
}

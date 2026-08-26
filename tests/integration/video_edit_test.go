package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

// editStub answers the two reads `video edit` makes before it commits to
// anything, and records the edit body if one ever arrives.
//
// Recording the body is half the point of these tests. The request array is
// positional — entry i is scene i+1, there is no index field — so the
// difference between a correct edit and one that rewrites the wrong shot is
// invisible in the exit code and visible only on the wire.
type editStub struct {
	server *httptest.Server
	// body holds the decoded /v1/scene/edit request, or nil if none was sent.
	body atomic.Value
	// calls counts edit submissions, so "was anything spent" is answerable
	// even for a body that failed to decode.
	calls atomic.Int64
}

type editStubOptions struct {
	scenes []map[string]any
	// videoPath is the work's rendered MP4, if it has one.
	videoPath string
	// busyCode, when set, makes /v1/scene/edit answer with a business
	// envelope on HTTP 200 instead of an event stream — the shape the
	// backend actually uses for the edit lock and the credit precheck.
	busyCode int
}

func newEditStub(t *testing.T, opts editStubOptions) *editStub {
	t.Helper()
	s := &editStub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/works/detailBySession"):
			w.Header().Set("Content-Type", "application/json")
			work := map[string]any{"id": 77, "session_id": "s_run", "title": "季度报告解读", "status": 1}
			if opts.videoPath != "" {
				work["video_path"] = opts.videoPath
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": work})

		case strings.HasPrefix(r.URL.Path, "/v1/works/scenes"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"work_id": 77, "scenes": opts.scenes},
			})

		case r.URL.Path == "/v1/scene/edit":
			s.calls.Add(1)
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) == nil {
				s.body.Store(body)
			}
			if opts.busyCode != 0 {
				// Deliberately HTTP 200 with a JSON envelope: the gateway
				// maps unrecognised business codes that way, and the CLI has
				// to notice it is not an event stream.
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": opts.busyCode, "message": "当前作品编辑中",
				})
				return
			}
			writeEditStream(w)

		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

// writeEditStream replays the event shapes SceneEditStream produces: a start
// event, a couple of process logs, then the terminal edit_completed. All
// three are wrapped in the {code:200,data:…} envelope the backend uses even
// for failures, which is why the CLI cannot read success off the envelope.
func writeEditStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	flusher, _ := w.(http.Flusher)
	send := func(data map[string]any) {
		payload, _ := json.Marshal(map[string]any{"code": 200, "data": data})
		fmt.Fprintf(w, "event: data\ndata: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}
	send(map[string]any{"type": "edit_start", "log": map[string]any{"message": "开始处理", "status": "start"}})
	fmt.Fprint(w, ": heartbeat\n\n")
	send(map[string]any{"type": "process", "log": map[string]any{"message": "配音已重新生成", "status": "success"}})
	send(map[string]any{
		"type": "edit_completed",
		"data": map[string]any{"html_path": "https://cdn.test/preview/77.html"},
	})
}

func editScenes() []map[string]any {
	return []map[string]any{
		{"scene_index": 1, "name": "开场", "script_text": "本季度营收同比增长百分之十二。", "duration_sec": 6.5, "layout_type": "cover", "status": 1},
		{"scene_index": 2, "name": "转场", "script_text": "先看结构。", "duration_sec": 1.0, "layout_type": "transition", "status": 1},
		{"scene_index": 3, "name": "结论", "script_text": "增长主要来自海外市场。", "duration_sec": 4.5, "layout_type": "text", "status": 1},
	}
}

var actionIDRe = regexp.MustCompile(`act_[0-9a-f]+`)

// TestVideoEdit_BlocksBeforeSpendingAnything is the property the whole gate
// exists for: with no terminal to ask, the CLI hands the decision back and
// does not touch the paid endpoint.
//
// The assertion that matters most is the call count. An exit 8 that had
// already submitted the edit would be worse than no gate at all — it would
// bill the user and then report that it was waiting for permission.
func TestVideoEdit_BlocksBeforeSpendingAnything(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newEditStub(t, editStubOptions{scenes: editScenes()})
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", "增长几乎全部来自海外市场。", "--output", "json")
	if code != 8 {
		t.Fatalf("exit %d, want 8 (blocked)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if n := stub.calls.Load(); n != 0 {
		t.Fatalf("blocked run still submitted %d edit(s); refusing has to be free", n)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("blocked payload is not one JSON document: %q: %v", stdout, err)
	}
	if got["status"] != "blocked" {
		t.Errorf("status = %v, want blocked", got["status"])
	}
	actions, _ := got["pending_actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("pending_actions = %v, want exactly one", got["pending_actions"])
	}
	action, _ := actions[0].(map[string]any)

	// The user is agreeing to a diff, so both halves have to be in front of
	// them. A payload carrying only the replacement asks for consent to a
	// change nobody can see.
	payload, _ := action["payload"].(map[string]any)
	if payload["from"] != "增长主要来自海外市场。" {
		t.Errorf("payload.from = %v, want the current narration", payload["from"])
	}
	if payload["to"] != "增长几乎全部来自海外市场。" {
		t.Errorf("payload.to = %v, want the proposed narration", payload["to"])
	}
	if payload["scene_index"] != float64(3) {
		t.Errorf("payload.scene_index = %v, want 3", payload["scene_index"])
	}

	resume, _ := action["resume_command"].(string)
	if !strings.Contains(resume, "--confirm act_") || !strings.Contains(resume, "--scene 3") {
		t.Errorf("resume_command is not runnable as-is: %q", resume)
	}
	// The narration is arbitrary human text. A resume command that only
	// survives well-behaved strings fails exactly where it matters.
	if !strings.Contains(resume, "--script '增长几乎全部来自海外市场。'") {
		t.Errorf("resume_command does not quote the script: %q", resume)
	}
}

// TestVideoEdit_ConfirmedEditReachesTheWireCorrectly checks the positional
// array, which is the one part of this request a caller cannot get wrong
// safely: there is no scene_index field, so shot 3 is "the third entry".
func TestVideoEdit_ConfirmedEditReachesTheWireCorrectly(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newEditStub(t, editStubOptions{scenes: editScenes(), videoPath: "out/77.mp4"})
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	const newScript = "增长几乎全部来自海外市场。"
	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", newScript, "--output", "json")
	if code != 8 {
		t.Fatalf("setup: expected a block first, got exit %d (%s)", code, stderr)
	}

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", newScript, "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("confirmed edit exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	body, _ := stub.body.Load().(map[string]any)
	if body == nil {
		t.Fatal("no edit request reached the backend")
	}
	scenes, _ := body["scenes"].([]any)
	if len(scenes) != 3 {
		t.Fatalf("scenes length = %d, want 3 — the backend rejects any other length outright", len(scenes))
	}
	for i, raw := range scenes {
		entry, _ := raw.(map[string]any)
		wantEdit := float64(0)
		if i == 2 {
			wantEdit = 1
		}
		if entry["edit"] != wantEdit {
			t.Errorf("scenes[%d].edit = %v, want %v", i, entry["edit"], wantEdit)
		}
		if i == 2 {
			if entry["scriptText"] != newScript {
				t.Errorf("scenes[2].scriptText = %v, want the new narration", entry["scriptText"])
			}
		} else if _, present := entry["scriptText"]; present {
			t.Errorf("scenes[%d] carries scriptText; untouched shots must send none", i)
		}
	}
	// Not requested, so it must not be sent: the backend treats it as the
	// switch between the cheap and the full regeneration.
	if _, present := body["script_only"]; present {
		t.Errorf("script_only was sent without being asked for: %v", body["script_only"])
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if got["preview_url"] != "https://cdn.test/preview/77.html" {
		t.Errorf("preview_url = %v, want the URL from edit_completed", got["preview_url"])
	}
	// The rendered MP4 is left in place by the backend, still downloadable
	// and now wrong. A caller that is not told this will hand the user a
	// video of the old script.
	if got["export_stale"] != true {
		t.Errorf("export_stale = %v, want true — the work had a rendered MP4", got["export_stale"])
	}
	actions, _ := got["next_actions"].([]any)
	if len(actions) == 0 {
		t.Error("no next_actions naming the re-export")
	}
}

// TestVideoEdit_ConsentIsBoundToTheText stops an agent iterating on wording
// from carrying one approval across several different rewrites.
func TestVideoEdit_ConsentIsBoundToTheText(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newEditStub(t, editStubOptions{scenes: editScenes()})
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	stdout, _, _ := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", "第一版措辞。", "--output", "json")
	token := actionIDRe.FindString(stdout)
	if token == "" {
		t.Fatalf("no action_id in the blocked payload: %s", stdout)
	}

	// Same shot, same session, different words.
	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", "第二版措辞。", "--confirm", token)
	if code != 2 {
		t.Fatalf("a token minted for other text was accepted: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if n := stub.calls.Load(); n != 0 {
		t.Fatalf("%d edit(s) submitted on a mismatched token", n)
	}

	// The same rewrite it was minted for still works, so the binding is to
	// the text and not just to token freshness.
	_, stderr, code = runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", "第一版措辞。", "--confirm", token)
	if code != 0 {
		t.Fatalf("the token's own rewrite was refused: exit %d\n%s", code, stderr)
	}
}

// TestVideoEdit_ScriptOnlyIsPartOfWhatWasAgreedTo covers the flag that
// decides what gets billed. Approving the cheap regeneration must not
// authorise the expensive one.
func TestVideoEdit_ScriptOnlyIsPartOfWhatWasAgreedTo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newEditStub(t, editStubOptions{scenes: editScenes()})
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	const newScript = "增长几乎全部来自海外市场。"
	stdout, _, _ := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", newScript, "--script-only", "--output", "json")
	token := actionIDRe.FindString(stdout)
	if token == "" {
		t.Fatalf("no action_id: %s", stdout)
	}
	if !strings.Contains(stdout, `"script_only":true`) {
		t.Errorf("script_only is not in the payload being agreed to: %s", stdout)
	}

	_, _, code := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", newScript, "--confirm", token)
	if code != 2 {
		t.Fatalf("a --script-only approval authorised a full regeneration: exit %d", code)
	}

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", newScript, "--script-only", "--confirm", token)
	if code != 0 {
		t.Fatalf("the approved cheap edit was refused: exit %d\n%s", code, stderr)
	}
	body, _ := stub.body.Load().(map[string]any)
	if body["script_only"] != true {
		t.Errorf("script_only did not reach the wire: %v", body)
	}
}

// TestVideoEdit_RefusesLocallyWhereItCan keeps the mistakes that cost
// nothing to catch from costing a round trip — or worse, a charge.
func TestVideoEdit_RefusesLocallyWhereItCan(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newEditStub(t, editStubOptions{scenes: editScenes()})
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			// Out of range. Naming the range turns a look-it-up round trip
			// into a one-token correction, so the range itself is part of
			// what is being tested — not just the refusal.
			name: "shot that does not exist",
			args: []string{"--scene", "9", "--script", "任何内容。"},
			want: []string{"no shot 9", "3 shots", "1–3"},
		},
		{
			// The backend would accept this, notice nothing changed, bill
			// nothing and report success — leaving a caller that mangled
			// its own diff believing the edit landed.
			name: "narration identical to the current one",
			args: []string{"--scene", "3", "--script", "增长主要来自海外市场。"},
			want: []string{"already says exactly this"},
		},
		{
			name: "empty narration",
			args: []string{"--scene", "3", "--script", "   "},
			want: []string{"cannot be empty"},
		},
		{
			name: "no script at all",
			args: []string{"--scene", "3"},
			want: []string{"--script is required"},
		},
		{
			name: "no shot named",
			args: []string{"--script", "任何内容。"},
			want: []string{"--scene is required"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"video", "edit", "42", "--session-id", "s_run", "--yes"}, tc.args...)
			stdout, stderr, code := runVideoCmd(t, bin, configHome, args...)
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			for _, want := range tc.want {
				if !strings.Contains(stdout+stderr, want) {
					t.Errorf("message does not say %q\nstdout: %s\nstderr: %s", want, stdout, stderr)
				}
			}
		})
	}
	// --yes was passed on every one of them, so nothing but the local checks
	// stood between these mistakes and a charge.
	if n := stub.calls.Load(); n != 0 {
		t.Fatalf("%d edit(s) submitted for input that should never have left the machine", n)
	}
}

// TestVideoEdit_BusyLockIsRetryable covers the refusal that does not arrive
// as an event stream at all.
//
// The edit lock answers with a business envelope on HTTP 200, before the SSE
// headers go out. Read as a stream it looks like a connection that carried
// no events — a generic exit 1, which tells an agent nothing. Read as an
// envelope it is work_edit_busy: transient by construction, exit 4, worth
// retrying in a moment.
func TestVideoEdit_BusyLockIsRetryable(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newEditStub(t, editStubOptions{scenes: editScenes(), busyCode: 100008})
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", "换个说法。", "--yes")
	if code != 4 {
		t.Fatalf("exit %d, want 4 (retryable)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if n := stub.calls.Load(); n != 1 {
		t.Errorf("edit submissions = %d, want 1", n)
	}
}

// TestVideoEdit_InsufficientCreditsIsNotRetryable rides the same non-stream
// path but must land somewhere else: retrying cannot conjure credits, so
// exit 5 (tell the user) rather than 4 (try again).
func TestVideoEdit_InsufficientCreditsIsNotRetryable(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newEditStub(t, editStubOptions{scenes: editScenes(), busyCode: 100001})
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "3", "--script", "换个说法。", "--yes")
	if code != 5 {
		t.Fatalf("exit %d, want 5\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	_ = stub
}

// TestVideoEdit_ProgressStaysOffStdout keeps the two-channel contract: in
// JSON mode stdout is exactly one document, and everything the stream said
// along the way is on the event channel instead.
func TestVideoEdit_ProgressStaysOffStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stub := newEditStub(t, editStubOptions{scenes: editScenes()})
	bin := build(t)
	configHome := buildVideoProfile(t, stub.server.URL)

	stdout, stderr, code := runCmdEnv(t, bin, configHome, []string{"VIBEKNOW_EVENTS=1"},
		"video", "edit", "42", "--session-id", "s_run",
		"--scene", "2", "--script", "先看整体结构。", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
		t.Fatalf("stdout is not exactly one JSON document: %q: %v", stdout, err)
	}
	if got["scene_index"] != float64(2) {
		t.Errorf("scene_index = %v, want 2", got["scene_index"])
	}
	for _, want := range []string{"edit.started", "edit.progress", "edit.succeeded"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("event channel is missing %q\nstderr: %s", want, stderr)
		}
	}
	if strings.Contains(stdout, "edit.progress") {
		t.Errorf("progress leaked onto stdout: %s", stdout)
	}

	// Shot 2 this time: the target has to move with --scene, not stay
	// wherever the last test put it.
	body, _ := stub.body.Load().(map[string]any)
	scenes, _ := body["scenes"].([]any)
	if len(scenes) != 3 {
		t.Fatalf("scenes = %v", body["scenes"])
	}
	second, _ := scenes[1].(map[string]any)
	if second["edit"] != float64(1) || second["scriptText"] != "先看整体结构。" {
		t.Errorf("the edit did not land on shot 2: %v", scenes)
	}
}

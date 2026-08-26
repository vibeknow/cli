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

// v3ParamServer captures the init and stream request bodies and drives the
// stream to a clean finish. Returns the server and a getter for the bodies.
func v3ParamServer(t *testing.T) (*httptest.Server, func() map[string]map[string]any) {
	t.Helper()
	var mu sync.Mutex
	bodies := map[string]map[string]any{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			bodies["init"] = body
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 7, "session_id": "s_v3p", "work_id": 8, "v": 3},
			})

		case "/v1/agent3forVideo/stream":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			bodies["stream"] = body
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, e := range []string{
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"script_writing","status":"start","message":"go"}}}`,
				`data: {"code":200,"data":{"type":"aim_result","session_id":"s_v3p"}}`,
				`data: [DONE]`,
			} {
				fmt.Fprintln(w, e)
				fmt.Fprintln(w)
				if flusher != nil {
					flusher.Flush()
				}
			}

		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 8, "session_id": "s_v3p", "title": "V3 Params",
					"html_path": "works/v3p/index.html", "share_token": "tok_v3p",
					"exporting": 0, "status": 1,
				},
			})

		default:
			w.WriteHeader(404)
		}
	}))

	return srv, func() map[string]map[string]any {
		mu.Lock()
		defer mu.Unlock()
		out := map[string]map[string]any{}
		for k, v := range bodies {
			out[k] = v
		}
		return out
	}
}

// TestCreate_V3Params_ReachTheWire pins the parameters added for the v3
// creation modes all the way through the real binary.
//
// The avatar fields are the ones worth a test: they are the only camelCase
// keys in an otherwise snake_case request body, which is exactly the kind of
// detail a later "let's make this consistent" edit quietly breaks. The
// backend does not reject unknown keys — it renders a video with no
// presenter — so a rename would cost a paid run and produce no error at all.
func TestCreate_V3Params_ReachTheWire(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv, bodies := v3ParamServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_v3params01", "--kb-id", "kb_test",
		"--prompt", "make it", // skips the optimize round-trip
		"--theme", "design_minimal",
		"--language", "en-US",
		"--avatar", "sys_12",
		"--avatar-position", "bottom-right",
		"--avatar-size", "300",
	)
	if code != 0 {
		t.Fatalf("create exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	stream := bodies()["stream"]
	if stream == nil {
		t.Fatal("stream body never captured")
	}

	want := map[string]any{
		"theme":          "design_minimal",
		"language":       "en-US",
		"avatar":         "sys_12",
		"avatarPosition": "bottom-right",
		"avatarHeightPx": float64(300),
	}
	for k, v := range want {
		if stream[k] != v {
			t.Errorf("stream body %q = %#v, want %#v\nfull body: %+v", k, stream[k], v, stream)
		}
	}

	// Guard the exact spelling of the avatar keys: snake_case versions are
	// silently ignored by the backend.
	for _, wrong := range []string{"avatar_position", "avatar_height_px", "avatar_size"} {
		if _, present := stream[wrong]; present {
			t.Errorf("stream body carries %q; the backend reads camelCase only", wrong)
		}
	}
}

// TestCreate_PageCount_OnInitAndStream covers the preflight-parity fix: the
// backend runs its image2 page checks at init, so an init that omits
// page_count validates a different request than the one that actually runs.
func TestCreate_PageCount_OnInitAndStream(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv, bodies := v3ParamServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--mode", "image", "--from", "doc_pagecount01", "--kb-id", "kb_test",
		"--prompt", "make it", "--pages", "6",
	)
	if code != 0 {
		t.Fatalf("create exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got := bodies()
	if got["init"]["page_count"] != float64(6) {
		t.Errorf("init page_count = %#v, want 6 — init preflight must see the real page count: %+v",
			got["init"]["page_count"], got["init"])
	}
	if got["stream"]["page_count"] != float64(6) {
		t.Errorf("stream page_count = %#v, want 6: %+v", got["stream"]["page_count"], got["stream"])
	}
}

// TestCreate_V3Params_LocalRejections pins the combinations the CLI refuses
// before spending anything.
//
// Two classes are mixed here on purpose. Bad values (a malformed avatar ref,
// an out-of-range size) would earn a 400 — annoying but honest. The mode
// combinations would not: the agent engine stores an avatar without
// compositing it, and the hand-drawn graph has no avatar node, so the
// backend accepts the request, bills the run, and returns a video with no
// presenter in it. Those are the ones that have to die locally.
func TestCreate_V3Params_LocalRejections(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(500)
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	base := []string{"create", "--from", "doc_rejections1", "--kb-id", "kb_test", "--prompt", "p"}

	cases := []struct {
		name string
		args []string
		// wantSubstr is a fragment of the message that has to tell the user
		// what to do next; an exit code alone does not fix anything.
		wantSubstr string
	}{
		{"bad avatar ref", []string{"--avatar", "bogus"}, "sys_"},
		{"avatar position not a corner", []string{"--avatar", "sys_1", "--avatar-position", "middle"}, "avatar-position"},
		{"avatar size below range", []string{"--avatar", "sys_1", "--avatar-size", "50"}, "120"},
		{"avatar size above range", []string{"--avatar", "sys_1", "--avatar-size", "900"}, "480"},
		{"avatar options without avatar", []string{"--avatar-position", "top-left"}, "--avatar"},
		{"avatar with handdraw", []string{"--avatar", "sys_1", "--mode", "handdraw"}, "avatar"},
		{"avatar with agent engine", []string{"--avatar", "sys_1", "--engine", "agent"}, "avatar"},
		{"theme with agent engine", []string{"--theme", "t1", "--engine", "agent"}, "theme"},
		{"language with agent engine", []string{"--language", "en-US", "--engine", "agent"}, "language"},
		{"unknown language", []string{"--language", "kl-KL"}, "language"},
		{"pages above range", []string{"--mode", "image", "--pages", "21"}, "20"},
		{"pages without image mode", []string{"--pages", "4"}, "pages"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, base...), tc.args...)
			stdout, stderr, code := runVideoCmd(t, bin, configHome, args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2 (user-fixable)\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			all := strings.ToLower(stdout + stderr)
			if !strings.Contains(all, strings.ToLower(tc.wantSubstr)) {
				t.Errorf("message does not mention %q:\nstdout: %s\nstderr: %s", tc.wantSubstr, stdout, stderr)
			}
		})
	}

	if reached {
		t.Errorf("a rejected flag combination still hit the backend; validation must run before init")
	}
}

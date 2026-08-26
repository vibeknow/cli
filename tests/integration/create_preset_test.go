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

// presetStub records the /v1/tasks/init body, which is where a preset key
// has to end up for the feature to mean anything. Asserting on stderr alone
// would only prove the CLI said it applied the preset.
func presetStub(t *testing.T, init *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			init.Store(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"task_id": 71, "session_id": "s_preset", "work_id": 72, "v": 3},
			})
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"session_id\":\"s_preset\"}}\n\ndata: [DONE]\n\n")
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"id": 72, "session_id": "s_preset", "title": "Preset", "share_token": "tok_p", "status": 1,
			}})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writePreset(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// presetArgs is the cheapest create invocation that reaches /v1/tasks/init:
// a doc_id plus its kb skips the upload entirely.
func presetArgs(extra ...string) []string {
	return append([]string{"create", "--from", "doc_abcd1234", "--kb-id", "kb_1"}, extra...)
}

func TestPreset_KeysReachTheWire(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)
	var init atomic.Value
	home := buildVideoProfile(t, presetStub(t, &init).URL)
	p := writePreset(t, "name: brand\ncreate:\n  mode: image\n  pages: 8\n  script_lock: true\n")

	_, stderr, code := runVideoCmd(t, bin, home, presetArgs("--preset", p)...)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	body, _ := init.Load().(map[string]any)
	if body == nil {
		t.Fatal("init never ran")
	}
	if body["video_kind"] != "image2" {
		t.Fatalf("video_kind = %v, want image2 — `mode: image` did not reach the request", body["video_kind"])
	}
	if body["page_count"] != float64(8) {
		t.Fatalf("page_count = %v, want 8", body["page_count"])
	}
	if body["script_lock"] != true {
		t.Fatalf("script_lock = %v, want true (underscore key must map to --script-lock)", body["script_lock"])
	}
	if !strings.Contains(stderr, `preset "brand" applied`) {
		t.Fatalf("stderr never says which preset ran, so a wrong-looking video has no trail:\n%s", stderr)
	}
}

func TestPreset_CommandLineWinsEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)
	var init atomic.Value
	home := buildVideoProfile(t, presetStub(t, &init).URL)
	p := writePreset(t, "name: brand\ncreate:\n  mode: image\n  aspect: horizontal\n")

	_, stderr, code := runVideoCmd(t, bin, home, presetArgs("--preset", p, "--mode", "replica")...)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	body, _ := init.Load().(map[string]any)
	if body["video_kind"] != "replica" {
		t.Fatalf("video_kind = %v; the preset overrode an explicit --mode", body["video_kind"])
	}
	// The applied list is what the user reads to understand the run, so it
	// has to reflect the override rather than claim mode came from the file.
	line := ""
	for _, l := range strings.Split(stderr, "\n") {
		if strings.Contains(l, "applied") {
			line = l
		}
	}
	if strings.Contains(line, "mode") {
		t.Fatalf("applied list claims mode came from the preset: %q", line)
	}
	if !strings.Contains(line, "aspect") {
		t.Fatalf("applied list omits the key that did come from the preset: %q", line)
	}
}

// The safety property. A preset is a file that can be mailed around; if it
// could carry `yes: true`, opening someone's preset would approve a charge.
func TestPreset_CannotApproveSpendAndCostsNothingToRefuse(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)
	var init atomic.Value
	home := buildVideoProfile(t, presetStub(t, &init).URL)
	p := writePreset(t, "create:\n  mode: image\n  export: true\n  yes: true\n")

	_, stderr, code := runVideoCmd(t, bin, home, presetArgs("--preset", p)...)
	if code != 2 {
		t.Fatalf("exit %d, want 2 (a bad preset is a bad invocation)\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "command line") {
		t.Fatalf("refusal does not say where consent must be given instead:\n%s", stderr)
	}
	if init.Load() != nil {
		t.Fatal("the run was submitted before the preset was rejected; refusing must be free")
	}
}

func TestPreset_UnknownKeyIsRefusedWithTheValidList(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)
	var init atomic.Value
	home := buildVideoProfile(t, presetStub(t, &init).URL)
	p := writePreset(t, "create:\n  asepct: horizontal\n")

	_, stderr, code := runVideoCmd(t, bin, home, presetArgs("--preset", p)...)
	if code != 2 {
		t.Fatalf("exit %d, want 2\n%s", code, stderr)
	}
	for _, want := range []string{"asepct", "aspect", "mode"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if init.Load() != nil {
		t.Fatal("a preset with an unknown key still started a run")
	}
}

func TestPreset_ResolvedByNameFromTheConfigDir(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)
	var init atomic.Value
	home := buildVideoProfile(t, presetStub(t, &init).URL)
	dir := filepath.Join(home, "presets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shorts.yaml"), []byte("create:\n  mode: handdraw\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runVideoCmd(t, bin, home, presetArgs("--preset", "shorts")...)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if body, _ := init.Load().(map[string]any); body["video_kind"] != "hand-draw" {
		t.Fatalf("video_kind = %v, want hand-draw", body["video_kind"])
	}

	// A typo has to name what does exist: there is no `vk preset list`, so
	// this error is the only listing the feature offers.
	_, stderr, code = runVideoCmd(t, bin, home, presetArgs("--preset", "shrots")...)
	if code != 2 {
		t.Fatalf("exit %d, want 2\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "shorts") {
		t.Fatalf("unknown-preset error does not list what is available:\n%s", stderr)
	}
}

func TestPreset_JSONModeAnnouncesOnTheEventChannelOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)
	var init atomic.Value
	home := buildVideoProfile(t, presetStub(t, &init).URL)
	p := writePreset(t, "name: brand\ncreate:\n  mode: image\n  pages: 6\n")

	stdout, stderr, code := runVideoCmd(t, bin, home, presetArgs("--preset", p, "--output", "json")...)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	// stdout stays exactly one document — the preset notice must not have
	// leaked into the answer.
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, stdout)
	}
	var ev map[string]any
	for _, l := range strings.Split(stderr, "\n") {
		if !strings.HasPrefix(l, "vk_event=") {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(l, "vk_event=")), &m); err == nil && m["type"] == "preset.applied" {
			ev = m
		}
	}
	if ev == nil {
		t.Fatalf("no preset.applied event on the structured channel:\n%s", stderr)
	}
	if ev["preset"] != "brand" {
		t.Fatalf("event names preset %v, want brand", ev["preset"])
	}
	keys, _ := ev["keys"].([]any)
	if len(keys) != 2 {
		t.Fatalf("keys = %v, want the two keys the preset set", ev["keys"])
	}
	// The prose form would be a second rendering of the same fact.
	if strings.Contains(stderr, `preset "brand" applied:`) {
		t.Fatalf("stderr carries both the event and the human line:\n%s", stderr)
	}
}

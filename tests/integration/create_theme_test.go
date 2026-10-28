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

// themeServer serves one design-suite theme, one active custom theme, and
// enough of the create flow to finish a run. It records which paths were
// hit, so a test can assert that a refused theme cost nothing.
func themeServer(t *testing.T) (*httptest.Server, func() []string, func() map[string]any) {
	srv, paths, stream, _ := themeServerWithInit(t)
	return srv, paths, stream
}

// themeServerWithInit is themeServer that also hands back the init body.
func themeServerWithInit(t *testing.T) (*httptest.Server, func() []string, func() map[string]any, func() map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	var stream, initBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/themes":
			var data []map[string]any
			switch r.URL.Query().Get("type") {
			case "design-suite":
				data = []map[string]any{{"id": "mint", "name": "纯白薄荷"}}
			case "image2-suite":
				data = []map[string]any{{"id": "ink", "name": "Ink", "scenes": []map[string]any{
					{"slug": "office", "name": "Office"},
				}}}
			case "stage-craft-suite":
				data = []map[string]any{{"id": "riso", "name": "Riso", "themes": []map[string]any{
					{"id": "riso-aqua", "name": "Aqua", "colors": map[string]any{"bg": "#fff", "accent": "#0aa"}},
				}}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
		case "/v1/assets":
			var data []map[string]any
			if r.URL.Query().Get("type") == "theme" {
				data = []map[string]any{{
					"id": 1234567, "name": "红薄荷", "status": 1, "enabled": 0,
					"config": map[string]any{"baseThemeId": "mint"},
				}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
		case "/v1/assets/image2Theme/ipCatalog":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"items": []map[string]any{
					{"ref": "sys_5", "name": "Panda", "image_url": "https://cdn.test/p.png", "public": true, "enabled": 0},
				},
			}})
		case "/v1/tasks/init":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			initBody = body
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 7, "session_id": "s_th", "work_id": 8, "v": 3},
			})
		case "/v1/agent3forVideo/stream":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			stream = body
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			for _, e := range []string{
				`data: {"code":200,"data":{"type":"aim_result","session_id":"s_th"}}`,
				`data: [DONE]`,
			} {
				fmt.Fprintln(w, e)
				fmt.Fprintln(w)
			}
		case "/v1/works/detailBySession":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 8, "session_id": "s_th", "title": "T", "share_token": "tok",
					"status": 1, "engine": "suite",
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	return srv,
		func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), paths...) },
		func() map[string]any { mu.Lock(); defer mu.Unlock(); return stream },
		func() map[string]any { mu.Lock(); defer mu.Unlock(); return initBody }
}

// The reported failure, end to end: a custom theme passed as the bare asset id
// the web shows. It used to reach the backend, which rendered in a
// substitute theme and billed the run. It must now be refused before
// anything is created, with the spelling that works in the message.
func TestCreate_Theme_BareNumberRefusedBeforeAnyWork(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, paths, _ := themeServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_theme000001", "--kb-id", "kb_test", "--prompt", "p",
		"--theme", "1234567", "--output", "json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "ua_1234567") {
		t.Errorf("refusal does not spell out the working value\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	for _, p := range paths() {
		if p == "/v1/tasks/init" || p == "/v1/agent3forVideo/stream" {
			t.Errorf("a refused theme still reached %s", p)
		}
	}
}

// The working spelling goes through unchanged and the result says what was
// asked for, labelled as a request.
func TestCreate_Theme_CustomReachesWireAndIsReported(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, _, stream := themeServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_theme000001", "--kb-id", "kb_test", "--prompt", "p",
		"--theme", "ua_1234567", "--output", "json")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if got := stream()["theme"]; got != "ua_1234567" {
		t.Errorf("stream theme = %v, want the ua_ token", got)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	ref, _ := out["requested_theme"].(map[string]any)
	if ref["id"] != "ua_1234567" || ref["source"] != "custom" {
		t.Errorf("requested_theme = %v", out["requested_theme"])
	}
	if out["mode"] != "freeform" {
		t.Errorf("mode = %v, want freeform", out["mode"])
	}
}

// A theme from another mode's catalog names that mode, so the caller can
// fix the call in one step instead of guessing.
func TestCreate_Theme_OtherModeNamed(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, _, _ := themeServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_theme000001", "--kb-id", "kb_test", "--prompt", "p",
		"--theme", "ink")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "--mode image") {
		t.Errorf("refusal does not name the owning mode\nstderr: %s", stderr)
	}
}

// `theme list` shows the custom theme an agent would otherwise have no way
// to discover, with the token create accepts.
func TestThemeList_IncludesCustomThemes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, _, _ := themeServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "theme", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	var out struct {
		Themes []map[string]any `json:"themes"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if len(out.Themes) != 2 {
		t.Fatalf("themes = %v, want custom + official", out.Themes)
	}
	first := out.Themes[0]
	if first["id"] != "ua_1234567" || first["source"] != "custom" || first["base"] != "mint" {
		t.Errorf("custom entry = %v", first)
	}
	if out.Themes[1]["source"] != "official" {
		t.Errorf("official entry = %v", out.Themes[1])
	}
}

// runCreate runs one create against a fresh themeServer and returns what the
// backend saw.
func runCreate(t *testing.T, args ...string) (stdout, stderr string, code int, paths []string, stream, initBody map[string]any) {
	t.Helper()
	srv, p, st, in := themeServerWithInit(t)
	defer srv.Close()
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)
	base := []string{"create", "--from", "doc_theme000001", "--kb-id", "kb_test", "--prompt", "p"}
	stdout, stderr, code = runVideoCmd(t, bin, configHome, append(base, args...)...)
	return stdout, stderr, code, p(), st(), in()
}

func reached(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

// Canvas mode reaches the backend as stage_craft on both requests — init is
// what marks the work as a canvas work — and a palette under a direction is
// accepted as a theme of its own.
func TestCreate_Canvas_WireAndPalette(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	stdout, stderr, code, _, stream, initBody := runCreate(t, "--mode", "canvas", "--theme", "riso-aqua", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if initBody["video_kind"] != "stage_craft" || stream["video_kind"] != "stage_craft" {
		t.Errorf("video_kind: init=%v stream=%v, want stage_craft on both", initBody["video_kind"], stream["video_kind"])
	}
	if stream["theme"] != "riso-aqua" {
		t.Errorf("stream theme = %v", stream["theme"])
	}
}

func TestCreate_Canvas_AgentEngineRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	_, stderr, code, paths, _, _ := runCreate(t, "--mode", "canvas", "--engine", "agent")
	if code != 2 || reached(paths, "/v1/tasks/init") {
		t.Fatalf("exit %d (want 2), init reached: %v\nstderr: %s", code, reached(paths, "/v1/tasks/init"), stderr)
	}
}

// --density reaches the wire on freeform and is refused everywhere the
// backend would ignore it.
func TestCreate_Density(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	_, stderr, code, _, stream, _ := runCreate(t, "--density", "dense")
	if code != 0 || stream["scene_density"] != "dense" {
		t.Fatalf("freeform: exit %d, scene_density %v\nstderr: %s", code, stream["scene_density"], stderr)
	}
	for _, args := range [][]string{
		{"--density", "dense", "--mode", "image"},
		{"--density", "lots"},
	} {
		_, stderr, code, paths, _, _ := runCreate(t, args...)
		if code != 2 || reached(paths, "/v1/tasks/init") {
			t.Errorf("%v: exit %d (want 2 before init)\nstderr: %s", args, code, stderr)
		}
	}
}

func TestCreate_ThemeScene(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	_, stderr, code, _, stream, _ := runCreate(t, "--mode", "image", "--theme", "ink", "--theme-scene", "office")
	if code != 0 || stream["theme_scene"] != "office" {
		t.Fatalf("exit %d, theme_scene %v\nstderr: %s", code, stream["theme_scene"], stderr)
	}
	_, stderr, code, paths, _, _ := runCreate(t, "--mode", "image", "--theme", "ink", "--theme-scene", "beach")
	if code != 2 || reached(paths, "/v1/tasks/init") || !strings.Contains(stderr, "office") {
		t.Errorf("unknown scene: exit %d, want 2 naming the real scenes\nstderr: %s", code, stderr)
	}
}

// The IP character is validated before upload and travels on init, where
// the backend snapshots it onto the work, as well as on the stream.
func TestCreate_IPAsset(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	_, stderr, code, _, stream, initBody := runCreate(t, "--mode", "image", "--ip-asset", "sys_5")
	if code != 0 || initBody["ip_asset"] != "sys_5" || stream["ip_asset"] != "sys_5" {
		t.Fatalf("exit %d, init ip_asset %v, stream ip_asset %v\nstderr: %s", code, initBody["ip_asset"], stream["ip_asset"], stderr)
	}
	_, stderr, code, paths, _, _ := runCreate(t, "--mode", "image", "--ip-asset", "sys_404")
	if code != 2 || reached(paths, "/v1/tasks/init") {
		t.Errorf("unknown IP: exit %d, want 2 before init\nstderr: %s", code, stderr)
	}
	_, _, code, paths, _, _ = runCreate(t, "--mode", "image", "--ip-asset", "none")
	if code != 0 || reached(paths, "/v1/assets/image2Theme/ipCatalog") {
		t.Errorf("none: exit %d, catalog fetched %v; none needs no lookup", code, reached(paths, "/v1/assets/image2Theme/ipCatalog"))
	}
	_, _, code, _, _, _ = runCreate(t, "--ip-asset", "sys_5")
	if code != 2 {
		t.Errorf("--ip-asset outside image mode: exit %d, want 2", code)
	}
}

func TestIPList(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, _, _ := themeServer(t)
	defer srv.Close()
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)
	stdout, stderr, code := runVideoCmd(t, bin, configHome, "ip", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstderr: %s", code, stderr)
	}
	var out struct {
		IPs []map[string]any `json:"ips"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if len(out.IPs) != 1 || out.IPs[0]["ref"] != "sys_5" || out.IPs[0]["source"] != "public" {
		t.Errorf("ips = %v", out.IPs)
	}
}

// Canvas palettes are --theme values, so theme list shows them.
func TestThemeList_CanvasVariants(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, _, _ := themeServer(t)
	defer srv.Close()
	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)
	stdout, stderr, code := runVideoCmd(t, bin, configHome, "theme", "list", "--mode", "canvas", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"riso-aqua"`) || !strings.Contains(stdout, `"variants"`) {
		t.Errorf("canvas listing lacks its palettes: %s", stdout)
	}
}

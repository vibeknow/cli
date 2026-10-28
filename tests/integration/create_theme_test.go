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
	t.Helper()
	var mu sync.Mutex
	var paths []string
	var stream map[string]any

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
				data = []map[string]any{{"id": "ink", "name": "Ink"}}
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
		case "/v1/tasks/init":
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
		func() map[string]any { mu.Lock(); defer mu.Unlock(); return stream }
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

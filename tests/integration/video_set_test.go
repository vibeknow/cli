package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// settingsServer records every write `video set` makes, and serves a work that
// already has a fully-populated subtitle style so a partial change has
// something to preserve.
func settingsServer(t *testing.T) (*httptest.Server, func(string) map[string]any) {
	t.Helper()
	var mu sync.Mutex
	seen := map[string]map[string]any{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/v1/works/detailBySession") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"id": 77, "session_id": "s_run", "title": "旧标题", "status": 1,
				"bgm": "on", "subtitle": "on",
				"subtitle_style": `{"fontFamily":"Source Han Sans","fontSize":36,"fontWeight":700,` +
					`"color":"#FFFFFF","strokeColor":"#000000","strokeWidth":2,"animation":"fade"}`,
			}})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		seen[r.URL.Path] = body
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"status": "ok"}})
	}))

	return srv, func(path string) map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return seen[path]
	}
}

// TestVideoSet_SubtitleStyleMergesOntoWhatIsThere is the one that matters.
//
// The style endpoint stores exactly what it receives — no merge — so sending
// only the field the user asked about would clear the font, weight, colour,
// outline and animation along with it. "Make the subtitles bigger" would
// quietly reset everything else about them, and nothing would report it.
func TestVideoSet_SubtitleStyleMergesOntoWhatIsThere(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--subtitle-size", "48", "--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	sent := seen("/v1/works/subtitleStyle")
	if sent == nil {
		t.Fatal("no subtitleStyle write was made")
	}
	style, _ := sent["subtitleStyle"].(map[string]any)
	if style == nil {
		t.Fatalf("no subtitleStyle payload: %+v", sent)
	}
	if style["fontSize"] != float64(48) {
		t.Errorf("fontSize = %v, want 48", style["fontSize"])
	}
	// Everything the user did not mention has to survive verbatim.
	for key, want := range map[string]any{
		"fontFamily":  "Source Han Sans",
		"fontWeight":  float64(700),
		"color":       "#FFFFFF",
		"strokeColor": "#000000",
		"strokeWidth": float64(2),
		"animation":   "fade",
	} {
		if style[key] != want {
			t.Errorf("%s = %v, want %v — changing one field wiped the rest: %+v", key, style[key], want, style)
		}
	}
}

// TestVideoSet_ReportsThatTheDownloadIsGone covers the consequence a user
// cannot otherwise connect to their own action. Any of these changes discards
// the rendered MP4 server-side; a caller that does not say so leaves the user
// to discover a missing download with no idea why.
func TestVideoSet_ReportsThatTheDownloadIsGone(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--bgm", "off", "--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if sent := seen("/v1/works/bgmSwitch"); sent == nil || sent["bgm"] != "off" {
		t.Errorf("bgmSwitch not sent correctly: %+v", sent)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if got["export_invalidated"] != true {
		t.Errorf("export_invalidated = %v, want true: %+v", got["export_invalidated"], got)
	}
	actions, _ := got["next_actions"].([]any)
	if len(actions) == 0 {
		t.Fatal("no next_actions telling the caller how to get a file back")
	}
	first, _ := actions[0].(map[string]any)
	if cmdStr, _ := first["command"].(string); !strings.Contains(cmdStr, "video export") {
		t.Errorf("next action does not point at export: %+v", first)
	}
}

// TestVideoSet_RenameDoesNotInvalidateTheExport keeps the warning honest.
// Renaming touches no rendered output, and claiming otherwise would push
// users into a paid re-export they never needed.
func TestVideoSet_RenameDoesNotInvalidateTheExport(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--title", "季度复盘", "--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if sent := seen("/v1/tasks/updateTitle"); sent == nil || sent["title"] != "季度复盘" {
		t.Errorf("updateTitle not sent correctly: %+v", sent)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(stdout), &got)
	if got["export_invalidated"] != false {
		t.Errorf("export_invalidated = %v, want false — renaming renders nothing", got["export_invalidated"])
	}
	if _, hasActions := got["next_actions"]; hasActions {
		t.Errorf("a rename should not push the caller towards a paid re-export: %+v", got)
	}
}

// TestVideoSet_NothingToChangeIsExit2 stops a no-op from reading as success.
// An agent that meant to change something and named no flag should be told to
// fix the call, not handed a cheerful empty result.
func TestVideoSet_NothingToChangeIsExit2(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--output", "json")
	if code != 2 {
		t.Fatalf("no flags: exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

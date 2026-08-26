package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// scenesServer answers the two reads `video script` makes: the work lookup
// that turns a session into a work_id, and the shot list itself.
func scenesServer(t *testing.T, scenes []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/works/detailBySession"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"id": 77, "session_id": "s_run", "title": "季度报告解读", "status": 1},
			})
		case strings.HasPrefix(r.URL.Path, "/v1/works/scenes"):
			if got := r.URL.Query().Get("work_id"); got != "77" {
				t.Errorf("work_id = %q, want 77 (resolved from the session)", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"work_id": 77, "scenes": scenes},
			})
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestVideoScript_ReadsTheNarration covers the question a caller could not
// previously ask at all: what does this video say?
//
// Everything readable about a finished work described the container — title,
// duration, cover, share link. The narration existed only inside the
// generation stream, whose progress events carry a character *count*, so once
// a run ended the words were unreachable short of watching the video.
func TestVideoScript_ReadsTheNarration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv := scenesServer(t, []map[string]any{
		{"scene_index": 1, "name": "开场", "script_text": "本季度营收同比增长百分之十二。",
			"duration_sec": 6.5, "layout_type": "cover", "status": 1, "tts_url": "https://cdn.test/1.mp3"},
		{"scene_index": 2, "name": "转场", "script_text": "", "duration_sec": 1.0, "layout_type": "transition", "status": 1},
		{"scene_index": 3, "name": "结论", "script_text": "增长主要来自海外市场。",
			"duration_sec": 4.5, "layout_type": "text", "status": 1, "srt_url": "https://cdn.test/3.srt"},
	})
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "script", "42", "--session-id", "s_run", "--output", "json")
	if code != 0 {
		t.Fatalf("script exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}

	// The stitched block is what "read me the script" actually wants; every
	// caller would otherwise rebuild it from the array.
	script, _ := got["script"].(string)
	for _, want := range []string{"本季度营收同比增长百分之十二。", "增长主要来自海外市场。"} {
		if !strings.Contains(script, want) {
			t.Errorf("script is missing %q: %q", want, script)
		}
	}
	// A silent shot contributes nothing to the prose but still counts as a
	// shot — dropping it from the breakdown would misreport the structure.
	if got["scene_count"] != float64(3) {
		t.Errorf("scene_count = %v, want 3", got["scene_count"])
	}
	if got["duration_sec"] != 12.0 {
		t.Errorf("duration_sec = %v, want 12", got["duration_sec"])
	}

	scenes, _ := got["scenes"].([]any)
	if len(scenes) != 3 {
		t.Fatalf("scenes = %d, want 3", len(scenes))
	}
	// Absent media must be absent, not empty: an empty URL reads as a broken
	// link, where a missing key reads as "not ready".
	transition, _ := scenes[1].(map[string]any)
	for _, key := range []string{"tts_url", "srt_url", "bg_image_url"} {
		if _, present := transition[key]; present {
			t.Errorf("shot 2 has no %s but the key was emitted anyway: %+v", key, transition)
		}
	}
	first, _ := scenes[0].(map[string]any)
	if first["tts_url"] != "https://cdn.test/1.mp3" {
		t.Errorf("tts_url dropped: %+v", first)
	}
}

// TestVideoScript_StillGeneratingSaysSo keeps an empty answer from reading as
// "this video is silent". A run that has not produced shots yet is a *not
// yet*, which is exit 6 — reconnect and re-check — not a successful read of
// nothing.
func TestVideoScript_StillGeneratingSaysSo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv := scenesServer(t, []map[string]any{})
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "script", "42", "--session-id", "s_run")
	if code != 6 {
		t.Fatalf("empty shot list: exit %d, want 6 (not terminal)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "no shots recorded") {
		t.Errorf("no explanation of why there is nothing to read\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

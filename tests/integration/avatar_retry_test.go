package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestAvatarRetry_ResetsFailedScenes covers the unblock path for a work that
// cannot export.
//
// Failed avatar scenes are terminal: the export gate refuses to render a
// video with blank presenter windows, so the work is stuck forever until
// those scenes are re-run. Retrying costs nothing and touches nothing else —
// script, images and TTS are all kept — which is why it is safe to offer as
// a plain command rather than a paid re-run.
func TestAvatarRetry_ResetsFailedScenes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/avatar/scenes/retry" {
			w.WriteHeader(404)
			return
		}
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"retry_count": 2},
		})
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "avatar-retry", "--session-id", "s_avatar", "--output", "json")
	if code != 0 {
		t.Fatalf("avatar-retry exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if got["retry_count"] != float64(2) {
		t.Errorf("retry_count = %v, want 2: %+v", got["retry_count"], got)
	}
	// The whole point of retrying is to be able to export; the next step
	// travels with the result so an agent does not have to infer it.
	if _, ok := got["next_actions"]; !ok {
		t.Errorf("no next_actions pointing at export: %+v", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if body["session_id"] != "s_avatar" {
		t.Errorf("request session_id = %v, want s_avatar", body["session_id"])
	}
	// Omitting scene_index means "every failed scene". Sending an explicit
	// zero instead would retry only scene 0 and silently leave the rest
	// failed — the work would still refuse to export, with no indication why.
	if _, present := body["scene_index"]; present {
		t.Errorf("scene_index sent without --scene: %+v", body)
	}
}

// TestAvatarRetry_SingleScene pins the targeted form.
func TestAvatarRetry_SingleScene(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"retry_count": 1},
		})
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "avatar-retry", "--session-id", "s_avatar", "--scene", "3", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstderr: %s", code, stderr)
	}

	mu.Lock()
	defer mu.Unlock()
	if body["scene_index"] != float64(3) {
		t.Errorf("scene_index = %v, want 3: %+v", body["scene_index"], body)
	}
}

// TestAvatarRetry_NothingToRetry covers the answer that is easiest to
// misread. retry_count 0 does not mean "the retry failed" — it means no
// scene was in a failed state. The usual cause is that the scenes are still
// rendering, and the right response is to wait, not to retry in a loop.
func TestAvatarRetry_NothingToRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"retry_count": 0},
		})
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "avatar-retry", "--session-id", "s_avatar")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (nothing to retry is not a failure)\nstderr: %s", code, stderr)
	}
	// The text form has to say so, not print a bare "0".
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("no explanation printed for retry_count 0")
	}

	stdout, _, _ = runVideoCmd(t, bin, configHome,
		"video", "avatar-retry", "--session-id", "s_avatar", "--output", "json")
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if got["retry_count"] != float64(0) {
		t.Errorf("retry_count = %v, want 0", got["retry_count"])
	}
}

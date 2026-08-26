package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestThemeList_SuitePerMode pins the mode→suite mapping through the real
// binary. Passing a theme from the wrong suite is a hard 400 at the stream
// entry, so `theme list` speaks modes rather than suite names — which only
// helps if each mode actually queries the catalog it draws from.
func TestThemeList_SuitePerMode(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	var asked []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/themes" {
			w.WriteHeader(404)
			return
		}
		mu.Lock()
		asked = append(asked, r.URL.Query().Get("type"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": []map[string]any{
				{"id": "th_1", "name": "Minimal", "desc": "clean", "tags": []string{"light"}},
			},
		})
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	cases := []struct{ mode, suite string }{
		{"", "design-suite"},
		{"default", "design-suite"},
		{"replica", "design-suite"}, // PPT walkthrough shares the standard catalog
		{"image", "image2-suite"},
		{"handdraw", "hand-draw-suite"},
	}

	for _, tc := range cases {
		args := []string{"theme", "list", "--output", "json"}
		if tc.mode != "" {
			args = append(args, "--mode", tc.mode)
		}
		stdout, stderr, code := runVideoCmd(t, bin, configHome, args...)
		if code != 0 {
			t.Fatalf("theme list --mode %q exit %d\nstderr: %s", tc.mode, code, stderr)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("decode %q: %v", stdout, err)
		}
		themes, _ := got["themes"].([]any)
		if len(themes) != 1 {
			t.Errorf("--mode %q: themes = %v, want 1 entry", tc.mode, got["themes"])
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(asked) != len(cases) {
		t.Fatalf("suite queries = %v, want %d", asked, len(cases))
	}
	for i, tc := range cases {
		if asked[i] != tc.suite {
			t.Errorf("--mode %q queried suite %q, want %q", tc.mode, asked[i], tc.suite)
		}
	}
}

// TestThemeList_UnknownMode_Exits2 keeps an unknown mode from being silently
// mapped onto the default catalog, which would hand the user theme ids that
// 400 when they finally use them.
func TestThemeList_UnknownMode_Exits2(t *testing.T) {
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

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "theme", "list", "--mode", "nope")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if reached {
		t.Errorf("unknown mode still queried the backend")
	}
}

// TestAvatarList_MergesCatalogAndOwn covers the shape an agent reads to pick
// a presenter: public presets carry the voice that was recorded with them,
// and the user's own avatars carry a training state that decides whether
// they can be used at all.
func TestAvatarList_MergesCatalogAndOwn(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/avatar/catalog":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": []map[string]any{
					{"id": "sys_3", "name": "Anna", "style": "3d", "gender": "female",
						"voiceId": "v_anna", "memberOnly": true},
				},
			})
		case r.URL.Path == "/v1/assets" && r.URL.Query().Get("type") == "avatar":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": []map[string]any{
					{"id": 41, "name": "Mine Active", "status": 1},
					{"id": 42, "name": "Mine Training", "status": 3},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "avatar", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("avatar list exit %d\nstderr: %s", code, stderr)
	}

	var got struct {
		Public []map[string]any `json:"public"`
		Mine   []map[string]any `json:"mine"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}

	if len(got.Public) != 1 || got.Public[0]["id"] != "sys_3" {
		t.Fatalf("public = %+v, want one sys_3 entry", got.Public)
	}
	// The paired voice is the reason this listing is useful: without it an
	// agent picks a female avatar and a male voice and nobody notices until
	// the render is paid for.
	if got.Public[0]["voice_id"] != "v_anna" {
		t.Errorf("public voice_id = %v, want v_anna", got.Public[0]["voice_id"])
	}
	if got.Public[0]["member_only"] != true {
		t.Errorf("member_only lost: %+v", got.Public[0])
	}

	if len(got.Mine) != 2 {
		t.Fatalf("mine = %+v, want 2 entries", got.Mine)
	}
	// The ua_ prefix is what `create --avatar` accepts; a bare id is rejected.
	if got.Mine[0]["id"] != "ua_41" {
		t.Errorf("own avatar id = %v, want ua_41 (the create --avatar form)", got.Mine[0]["id"])
	}
	if got.Mine[0]["usable"] != true || got.Mine[1]["usable"] != false {
		t.Errorf("usable flags wrong: %+v", got.Mine)
	}
	if got.Mine[1]["status"] != "training" {
		t.Errorf("status label = %v, want training", got.Mine[1]["status"])
	}
}

// TestAvatarList_OwnListFailureDegrades keeps a broken own-avatar lookup
// from sinking the public catalog. Most users have no trained avatar at all,
// so the presets are the whole answer for them.
func TestAvatarList_OwnListFailureDegrades(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/avatar/catalog":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": []map[string]any{{"id": "sys_3", "name": "Anna"}},
			})
		default:
			w.WriteHeader(500)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "message": "assets down"})
		}
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "avatar", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (public catalog still usable)\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "sys_3") {
		t.Errorf("public catalog missing from output: %s", stdout)
	}
	// The failure is reported, just not fatally — a silently empty "mine"
	// would read as "you have no avatars", which is a different fact.
	if stderr == "" {
		t.Errorf("own-avatar failure was swallowed entirely; expected a note on stderr")
	}
}

// TestVoiceList_PipelineGrouping covers the migration to /pipeline-voices:
// presets arrive grouped by language, cloned voices arrive in a separate
// list because they are language-independent.
func TestVoiceList_PipelineGrouping(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/pipeline-voices" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"languages": []map[string]any{
					{"language": "zh-CN", "voices": []map[string]any{
						{"id": 1, "speech_voice_id": "zh_a", "name": "小云"},
					}},
					{"language": "en-US", "voices": []map[string]any{
						{"id": 2, "speech_voice_id": "en_a", "name": "Aria"},
					}},
				},
				"cloned": []map[string]any{{"id": 90, "speech_voice_id": "cl_1", "name": "My Clone"}},
			},
		})
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"vibeknow": srv.URL})

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "voice", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("voice list exit %d\nstderr: %s", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	// templates stays in the payload as the flat, language-agnostic view
	// existing scripts already read.
	tmpl, _ := got["templates"].([]any)
	if len(tmpl) != 3 {
		t.Errorf("templates = %v, want 3 (2 preset + 1 cloned)", got["templates"])
	}
	if _, ok := got["languages"]; !ok {
		t.Errorf("languages group missing: %+v", got)
	}
	cloned, _ := got["cloned"].([]any)
	if len(cloned) != 1 {
		t.Errorf("cloned = %v, want 1", got["cloned"])
	}

	// --language narrows the presets without hiding cloned voices, which
	// work in any language.
	stdout, stderr, code = runVideoCmd(t, bin, configHome,
		"voice", "list", "--language", "en-US", "--output", "json")
	if code != 0 {
		t.Fatalf("voice list --language exit %d\nstderr: %s", code, stderr)
	}
	if strings.Contains(stdout, "zh_a") {
		t.Errorf("--language en-US still returned the zh-CN preset: %s", stdout)
	}
	if !strings.Contains(stdout, "en_a") {
		t.Errorf("--language en-US dropped the matching preset: %s", stdout)
	}
}

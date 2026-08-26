package figlens_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

func TestListAvatarCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/avatar/catalog" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":[{"id":"sys_7","name":"小雅","imageUrl":"https://cdn/a.png","style":"3d","gender":"female","voiceId":"sv_9","tags":["知性"],"position":"top-left","heightPx":240}]}`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	items, err := c.ListAvatarCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListAvatarCatalog: %v", err)
	}
	if len(items) != 1 || items[0].ID != "sys_7" || items[0].VoiceID != "sv_9" || items[0].Gender != "female" {
		t.Fatalf("items = %+v", items)
	}
}

func TestListMyAvatars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/assets" || r.URL.Query().Get("type") != "avatar" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":[{"id":12,"type":"avatar","name":"我的形象","status":1},{"id":13,"type":"avatar","name":"训练中","status":3}]}`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	assets, err := c.ListMyAvatars(context.Background())
	if err != nil {
		t.Fatalf("ListMyAvatars: %v", err)
	}
	if len(assets) != 2 || assets[0].ID != 12 || assets[0].Status != figlens.UserAssetStatusActive {
		t.Fatalf("assets = %+v", assets)
	}
	if figlens.AvatarStatusLabel(assets[1].Status) != "training" {
		t.Fatalf("status label = %q", figlens.AvatarStatusLabel(assets[1].Status))
	}
}

func TestRetryAvatarScenes(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/avatar/scenes/retry" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":{"retry_count":2}}`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	n, err := c.RetryAvatarScenes(context.Background(), "s_x", nil)
	if err != nil {
		t.Fatalf("RetryAvatarScenes: %v", err)
	}
	if n != 2 {
		t.Fatalf("retry_count = %d, want 2", n)
	}
	if gotBody["session_id"] != "s_x" {
		t.Fatalf("body = %v", gotBody)
	}
	// nil sceneIndex must stay off the wire (backend treats absent as
	// "retry every failed scene"; an explicit 0 would mean scene #0 only).
	if _, present := gotBody["scene_index"]; present {
		t.Fatalf("nil scene_index leaked onto the wire: %v", gotBody)
	}

	scene := 3
	if _, err := c.RetryAvatarScenes(context.Background(), "s_x", &scene); err != nil {
		t.Fatalf("RetryAvatarScenes(scene): %v", err)
	}
	if gotBody["scene_index"] != float64(3) {
		t.Fatalf("scene_index = %v, want 3", gotBody["scene_index"])
	}
}

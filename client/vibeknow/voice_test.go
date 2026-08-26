package vibeknow_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/client/vibeknow"
)

type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }

func TestListPipelineVoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/pipeline-voices" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"languages": []map[string]any{
					{
						"language": "zh-CN",
						"voices": []map[string]any{
							{"id": 1, "name": "Alice", "category": "female", "language": "zh-CN", "tags": []string{"清新"}, "speech_voice_id": "sv_1"},
						},
					},
					{
						"language": "en-US",
						"voices": []map[string]any{
							{"id": 2, "name": "Bob", "category": "male", "language": "en-US", "tags": []string{}, "speech_voice_id": "sv_2"},
						},
					},
				},
				// Cloned voices carry no language/category by design.
				"cloned": []map[string]any{
					{"id": 9, "name": "我的声音", "category": "", "tags": []string{}, "speech_voice_id": "sv_mine"},
				},
			},
		})
	}))
	defer srv.Close()

	c := vibeknow.New(srv.URL, staticToken("tok"))
	catalog, err := c.ListPipelineVoices(context.Background())
	if err != nil {
		t.Fatalf("ListPipelineVoices: %v", err)
	}
	if len(catalog.Languages) != 2 || catalog.Languages[0].Language != "zh-CN" {
		t.Fatalf("languages = %+v", catalog.Languages)
	}
	if len(catalog.Cloned) != 1 || catalog.Cloned[0].SpeechVoiceID != "sv_mine" {
		t.Fatalf("cloned = %+v", catalog.Cloned)
	}

	// Flatten must keep catalog order and put cloned voices last, so a
	// numeric --voice ref resolves across both populations.
	flat := catalog.Flatten()
	if len(flat) != 3 || flat[0].ID != 1 || flat[1].ID != 2 || flat[2].ID != 9 {
		t.Fatalf("Flatten() = %+v", flat)
	}
}

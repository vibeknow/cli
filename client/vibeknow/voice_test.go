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

func TestListVoiceTemplates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/voice-templates" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") != "1" {
			t.Fatalf("missing page param")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"list": []map[string]any{
					{"id": 1, "name": "Alice", "category": "female", "tags": []string{"清新"}, "speech_voice_id": "sv_1"},
					{"id": 2, "name": "Bob", "category": "male", "tags": []string{"浑厚"}, "speech_voice_id": "sv_2"},
				},
			},
		})
	}))
	defer srv.Close()

	c := vibeknow.New(srv.URL, staticToken("tok"))
	voices, err := c.ListVoiceTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListVoiceTemplates: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices))
	}
	if voices[0].ID != 1 || voices[0].Name != "Alice" {
		t.Fatalf("voice[0] = %+v", voices[0])
	}
}

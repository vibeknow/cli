package vibeknow_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shiliu-ai/vibeknow-cli/client/vibeknow"
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
			"code": 200,
			"data": map[string]any{
				"items": []map[string]any{
					{"id": "v_1", "name": "Alice", "language": "en", "gender": "female"},
					{"id": "v_2", "name": "Bob", "language": "zh", "gender": "male"},
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
	if voices[0].ID != "v_1" || voices[0].Name != "Alice" {
		t.Fatalf("voice[0] = %+v", voices[0])
	}
}

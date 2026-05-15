package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestKBPrune_DryRunDoesNotDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	deleteCalls := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/knowledgebases", func(w http.ResponseWriter, r *http.Request) {
		// list endpoint
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total": 2, "offset": 0, "limit": 100,
			"items": []map[string]any{
				{"id": "kb_a", "name": "vibeknow-cli-1", "description": "", "created_at": "2026-05-14T00:00:00Z"},
				{"id": "kb_b", "name": "manual-kb", "description": "user", "created_at": "2026-05-13T00:00:00Z"},
			},
		})
	})
	mux.HandleFunc("/v1/knowledgebases/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			mu.Lock()
			deleteCalls++
			mu.Unlock()
			w.WriteHeader(204)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"vectoria": srv.URL})

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"kb", "prune", "--pattern", "vibeknow-cli-*",
	)
	if code != 0 {
		t.Fatalf("dry-run unexpected exit %d\nstdout:%s\nstderr:%s", code, stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "vibeknow-cli-1") {
		t.Fatalf("dry-run output should list matched kb:\nstdout:%s\nstderr:%s", stdout, stderr)
	}
	if !strings.Contains(combined, "dry run") {
		t.Fatalf("dry-run output should mention 'dry run' hint:\nstdout:%s\nstderr:%s", stdout, stderr)
	}
	mu.Lock()
	dc := deleteCalls
	mu.Unlock()
	if dc != 0 {
		t.Fatalf("dry-run made %d DELETE calls; expected 0", dc)
	}
}

func TestKBPrune_ApplyDeletesMatchedOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	var deletedIDs []string

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/knowledgebases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total": 3, "offset": 0, "limit": 100,
			"items": []map[string]any{
				{"id": "kb_a", "name": "vibeknow-cli-1", "description": "", "created_at": "2026-05-14T00:00:00Z"},
				{"id": "kb_b", "name": "manual-kb", "description": "user", "created_at": "2026-05-13T00:00:00Z"},
				{"id": "kb_c", "name": "vibeknow-cli-2", "description": "", "created_at": "2026-05-13T00:00:00Z"},
			},
		})
	})
	mux.HandleFunc("/v1/knowledgebases/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			id := strings.TrimPrefix(r.URL.Path, "/v1/knowledgebases/")
			mu.Lock()
			deletedIDs = append(deletedIDs, id)
			mu.Unlock()
			w.WriteHeader(204)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"vectoria": srv.URL})

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"kb", "prune", "--pattern", "vibeknow-cli-*", "--yes",
	)
	if code != 0 {
		t.Fatalf("apply unexpected exit %d\nstderr:%s", code, stderr)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(deletedIDs) != 2 {
		t.Fatalf("expected 2 deletes (vibeknow-cli-*), got %d: %v", len(deletedIDs), deletedIDs)
	}
	// Order-independent check that both expected ids are present.
	seen := map[string]bool{}
	for _, id := range deletedIDs {
		seen[id] = true
	}
	if !seen["kb_a"] || !seen["kb_c"] {
		t.Fatalf("expected kb_a + kb_c deleted, got: %v", deletedIDs)
	}
	if seen["kb_b"] {
		t.Fatalf("manual-kb (kb_b) should NOT have been deleted: %v", deletedIDs)
	}
}

func TestKBPrune_NoFilterExits2(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)
	configHome := buildProfile(t, map[string]string{"vectoria": "http://example.invalid"})
	_, stderr, code := runVideoCmd(t, bin, configHome,
		"kb", "prune", "--yes",
	)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d\nstderr:%s", code, stderr)
	}
	if !strings.Contains(stderr, "--pattern") || !strings.Contains(stderr, "--older-than") {
		t.Fatalf("error message should mention --pattern AND --older-than, got: %s", stderr)
	}
}

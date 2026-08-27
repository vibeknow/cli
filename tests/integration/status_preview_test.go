package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVideoStatus_PreviewDir_DeliversWhatExists covers the case an agent in a
// chat client is actually in: it did not start the run, so it never held the
// artifacts, and the only way to put a file in front of the user is to fetch
// it from a snapshot.
func TestVideoStatus_PreviewDir_DeliversWhatExists(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	const coverBytes = "not-really-a-jpeg-but-bytes-all-the-same"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 43, "session_id": "s_cover",
					"title":       "Quarterly Review",
					"html_path":   "works/x/index.html",
					"share_token": "tok_x",
					"cover_url":   "works/x/cover.jpg",
					"status":      1,
					"exporting":   0,
				},
			})

		case "/v1/agent2forVideo/signedUrl":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"url": "http://" + r.Host + "/asset"},
			})

		case "/asset":
			_, _ = w.Write([]byte(coverBytes))

		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)
	outDir := filepath.Join(t.TempDir(), "artifacts")

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "status", "42", "--session-id", "s_cover",
		"--preview-dir", outDir, "--output", "json",
	)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read %s: %v", outDir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no artifact was written to %s\nstderr: %s", outDir, stderr)
	}

	// The event is what tells the caller a file exists and where; a file on
	// disk nobody was told about is the same as no file.
	if !strings.Contains(stderr, "resource_ready") {
		t.Errorf("stderr should announce the artifact, got: %s", stderr)
	}
	// Deliberately no remote URL in the event: those are signed, and
	// forwarding one hands over a credential.
	if strings.Contains(stderr, "/asset") {
		t.Errorf("the event must not carry the signed URL, got: %s", stderr)
	}
}

// A snapshot taken mid-render has neither artifact. It must still succeed and
// still cost nothing — polling is the normal case, not an error.
func TestVideoStatus_PreviewDir_MidRenderDeliversNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var signedCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"id": 43, "session_id": "s_mid", "status": 0},
			})
		case "/v1/agent2forVideo/signedUrl":
			signedCalls++
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)
	outDir := filepath.Join(t.TempDir(), "artifacts")

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "status", "42", "--session-id", "s_mid",
		"--preview-dir", outDir, "--output", "json",
	)
	if code != 0 {
		t.Fatalf("exit %d, want 0 for a run still going\nstderr: %s", code, stderr)
	}
	if signedCalls != 0 {
		t.Errorf("asked to sign %d URLs for a work with no artifacts, want 0", signedCalls)
	}
}

func TestVideoStatus_PreviewDir_RefusesAnUnwritablePath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	// A regular file where a directory has to go. Refused up front rather
	// than after the request: producing no files is indistinguishable from a
	// run that had none, so the caller would never learn the path was wrong.
	notADir := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "status", "42", "--session-id", "s_x", "--preview-dir", notADir,
	)
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr: %s", code, stderr)
	}
}

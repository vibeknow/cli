package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// TestCreate_InsufficientCreditsOnInit_Exits5 covers the bug fixed in 0.5.1:
// when the backend rejects InitTask with envelope code 100001 (insufficient
// credits), the CLI must exit 5 (business failure) to match the stream-side
// path's behavior, not exit 1 from cobra's default error handler.
//
// It also covers the orphan-kb cleanup fixed in 0.5.2: when InitTask fails
// after the CLI uploaded a doc and created a kb, the CLI must DELETE the
// just-created kb before exiting so it doesn't leak.
func TestCreate_InsufficientCreditsOnInit_Exits5(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	var kbCreated, kbDeleted bool
	var deletedID, createdID string

	const fixedKBID = "kb_insufcredits_test"
	mux := http.NewServeMux()
	// Exact match: vectoria CreateKB (POST /v1/knowledgebases).
	mux.HandleFunc("/v1/knowledgebases", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		kbCreated = true
		createdID = fixedKBID
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": createdID})
	})
	// Prefix match: doc upload (POST), doc status (GET), kb delete (DELETE).
	mux.HandleFunc("/v1/knowledgebases/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			_ = r.ParseMultipartForm(32 << 20)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "doc_x", "status": "completed"})
		case "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "doc_x", "status": "completed"})
		case "DELETE":
			// Assert the full path to fail loudly if upload or poll ever
			// accidentally routes here.
			if r.URL.Path != "/v1/knowledgebases/"+fixedKBID {
				t.Errorf("unexpected DELETE path %q", r.URL.Path)
			}
			mu.Lock()
			kbDeleted = true
			deletedID = strings.TrimPrefix(r.URL.Path, "/v1/knowledgebases/")
			mu.Unlock()
			w.WriteHeader(204)
		}
	})
	mux.HandleFunc("/v1/tasks/init", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    100001,
			"message": "insufficient credits",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Write a tiny local file so the CLI takes the upload path (which creates a kb).
	tmpFile := t.TempDir() + "/test.txt"
	if err := os.WriteFile(tmpFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	bin := build(t)
	configHome := buildProfile(t, map[string]string{
		"figlens":  srv.URL,
		"vectoria": srv.URL,
	})

	cmd := exec.Command(bin, "create", "--from", tmpFile)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VIBEKNOW_CONFIG_HOME="+configHome,
	)

	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}

	if code != 5 {
		t.Fatalf("exit code = %d, want 5 (business failure)\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "insufficient credits") {
		t.Fatalf("stderr missing insufficient-credits message:\n%s", stderr.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if !kbCreated {
		t.Fatalf("expected kb to be created by CLI but no POST /v1/knowledgebases received")
	}
	if !kbDeleted {
		t.Fatalf("expected orphan kb cleanup: DELETE /v1/knowledgebases/<id> was never called.\nstderr:%s", stderr.String())
	}
	if deletedID != createdID {
		t.Fatalf("deleted kb = %q, want %q", deletedID, createdID)
	}
}

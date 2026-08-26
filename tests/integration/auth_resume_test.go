package integration

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAuthLogin_ResumesAfterSubprocessKilled reproduces the exact sequence a
// WorkBuddy connector performs, including the part that used to lose the
// login:
//
//  1. spawn `auth login --headless`
//  2. scrape stdout for the verification URL
//  3. kill the subprocess
//  4. poll `auth status --output json` until authenticated
//
// Step 3 is not hypothetical — the connector spec caps the auth subprocess
// at five minutes while the device code lives fifteen, so any user who takes
// their time gets their auth process killed out from under them. Before the
// parked-code mechanism the device code existed only in that process's
// memory: the user would authorize successfully in the browser and the CLI
// would never find out, leaving the connector stuck at "not connected" with
// no path forward but authorizing all over again.
//
// The test authorizes *after* the kill, so it only passes if `status`
// itself completes the exchange.
func TestAuthLogin_ResumesAfterSubprocessKilled(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	authorized := false

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"device_code":      "dc_killresume",
				"user_code":        "WBUD-KILL",
				"verification_uri": "https://example.test/activate",
				"expires_in":       900,
				"interval":         1,
			},
		})
	})
	mux.HandleFunc("/v1/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ok := authorized
		mu.Unlock()
		if !ok {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    40010,
				"message": "authorization_pending",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"access_token":       "tok_killresume",
				"refresh_token":      "refresh_killresume",
				"token_type":         "bearer",
				"expires_in":         7200,
				"refresh_expires_in": 2592000,
			},
		})
	})
	mux.HandleFunc("/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "ok",
			"data":    map[string]any{"uid": 99, "nickname": "kill-resume-user"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"account": srv.URL})
	// buildProfile writes credential_ref "test"; the login stores the token
	// under that keychain entry, and `auth status` reads it back.
	env := []string{
		"VIBEKNOW_CONFIG_HOME=" + configHome,
		"VIBEKNOW_TOKEN=", // never let a developer's env satisfy this test
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"), // keychain access
	}

	// --- step 1+2: spawn auth, read the URL off stdout ---
	login := exec.Command(bin, "auth", "login", "--headless")
	login.Env = env
	stdout, err := login.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	login.Stderr = nil
	if err := login.Start(); err != nil {
		t.Fatalf("start login: %v", err)
	}

	sawURL := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.Contains(sc.Text(), "verification_uri") {
				sawURL <- sc.Text()
				return
			}
		}
		close(sawURL)
	}()

	select {
	case line, ok := <-sawURL:
		if !ok {
			t.Fatal("login exited without printing verification_uri")
		}
		if !strings.Contains(line, "https://example.test/activate") {
			t.Fatalf("unexpected verification line: %q", line)
		}
	case <-time.After(10 * time.Second):
		// The connector spec allows the auth command ten seconds to emit a URL.
		_ = login.Process.Kill()
		t.Fatal("no verification URL within 10s")
	}

	// --- step 3: kill it, exactly as the host does ---
	if err := login.Process.Kill(); err != nil {
		t.Fatalf("kill login: %v", err)
	}
	_ = login.Wait()

	// The code must have been parked before the URL was printed, or it died
	// with the process.
	parked := filepath.Join(configHome, "pending-device-auth.json")
	if _, err := os.Stat(parked); err != nil {
		t.Fatalf("no parked device code after auth was killed: %v", err)
	}

	// --- the user authorizes in the browser, after the CLI process is gone ---
	mu.Lock()
	authorized = true
	mu.Unlock()

	// --- step 4: the host polls status; it must complete the login ---
	out, stderr, code := runCmdEnv(t, bin, configHome, []string{"VIBEKNOW_TOKEN="},
		"auth", "status", "--output", "json")
	if code != 0 {
		t.Fatalf("auth status exit = %d, want 0\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	if got["authenticated"] != true {
		t.Fatalf("authenticated = %v, want true — `auth status` did not complete the parked login: %+v",
			got["authenticated"], got)
	}

	// Cleanup: don't leave a token in the developer's keychain.
	t.Cleanup(func() {
		_, _, _ = runCmdEnv(t, bin, configHome, []string{"VIBEKNOW_TOKEN="}, "auth", "logout")
	})
}

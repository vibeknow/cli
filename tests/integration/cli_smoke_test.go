package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Shared binary across all integration tests in this package. Built once
// per `go test` run via sync.Once, reused by every build(t) caller.
// Before this caching, each test function paid a fresh ~30s go-build
// cost; with 15+ build(t) callers across the package, that was ~450s
// of redundant compile per CI run.
var (
	sharedBin     string
	sharedBinErr  error
	sharedBinDir  string
	sharedBinOnce sync.Once
)

func build(t *testing.T) string {
	t.Helper()
	sharedBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "vibeknow-integ-bin-")
		if err != nil {
			sharedBinErr = err
			return
		}
		sharedBinDir = dir
		name := "vibeknow"
		if runtime.GOOS == "windows" {
			// Windows requires the .exe suffix for exec.Command to locate
			// and run the binary. Without it, `go build -o` still writes
			// the file but exec.Command(bin, ...) fails with "executable
			// file not found in %PATH%".
			name += ".exe"
		}
		bin := filepath.Join(dir, name)
		root, _ := filepath.Abs("../..")
		cmd := exec.Command("go", "build", "-o", bin, ".")
		cmd.Dir = root
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			sharedBinErr = err
			return
		}
		sharedBin = bin
	})
	if sharedBinErr != nil {
		t.Fatalf("shared build: %v", sharedBinErr)
	}
	return sharedBin
}

// TestMain runs after all tests in the package finish and cleans up the
// shared binary dir. Without this the tmp dir leaks across `go test` runs.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedBinDir != "" {
		_ = os.RemoveAll(sharedBinDir)
	}
	os.Exit(code)
}

func run(t *testing.T, bin, home string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "VIBEKNOW_CONFIG_HOME="+home)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), code
}

func TestCLISmoke(t *testing.T) {
	bin := build(t)
	home := t.TempDir()

	// version works
	out, _, code := run(t, bin, home, "version")
	if code != 0 || out == "" {
		t.Fatalf("version: code=%d out=%q", code, out)
	}

	// profile add
	_, _, code = run(t, bin, home,
		"profile", "add", "dev",
		"--endpoint-vibeknow", "https://staging.example.com",
		"--credential-ref", "vibeknow.dev",
		"--trust", "dev",
		"--is-production=false",
	)
	if code != 0 {
		t.Fatalf("profile add failed: code=%d", code)
	}

	// profile list contains dev
	out, _, code = run(t, bin, home, "profile", "list")
	if code != 0 || !strings.Contains(out, "dev") {
		t.Fatalf("profile list: code=%d out=%q", code, out)
	}

	// profile show
	out, _, code = run(t, bin, home, "profile", "show", "dev")
	if code != 0 || !strings.Contains(out, "staging.example.com") {
		t.Fatalf("profile show: code=%d out=%q", code, out)
	}

	// config set/get
	_, _, code = run(t, bin, home, "config", "set", "k1", "v1")
	if code != 0 {
		t.Fatal("config set")
	}
	out, _, code = run(t, bin, home, "config", "get", "k1")
	if code != 0 || strings.TrimSpace(out) != "v1" {
		t.Fatalf("config get: code=%d out=%q", code, out)
	}

	// doctor (may exit non-zero on headless CI if keychain unreachable; just verify it runs and prints header)
	out, _, _ = run(t, bin, home, "doctor")
	if !strings.Contains(out, "doctor") {
		t.Fatalf("doctor output missing header: %q", out)
	}

	// profile remove
	_, _, code = run(t, bin, home, "profile", "remove", "dev")
	if code != 0 {
		t.Fatal("profile remove")
	}
}

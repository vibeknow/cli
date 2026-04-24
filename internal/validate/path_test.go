package validate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func withCwd(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestSafeOutputPath_RelativeOK(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)

	got, err := SafeOutputPath("foo.mp4")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if !strings.HasPrefix(got, want) {
		t.Errorf("resolved path %q not under %q", got, want)
	}
}

func TestSafeOutputPath_RejectsAbsolute(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)

	// On Windows, absolute paths begin with a drive letter (e.g. `C:\...`);
	// a leading slash alone is not considered absolute by filepath.IsAbs.
	abs := "/etc/passwd"
	if runtime.GOOS == "windows" {
		abs = `C:\Windows\System32\drivers\etc\hosts`
	}
	if _, err := SafeOutputPath(abs); err == nil {
		t.Error("expected error for absolute path")
	}
}

func TestSafeOutputPath_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "inside")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	withCwd(t, nested)

	if _, err := SafeOutputPath("../escape.txt"); err == nil {
		t.Error("expected error for traversal path")
	}
}

func TestSafeOutputPath_RejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	withCwd(t, dir)

	link := filepath.Join(dir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}

	if _, err := SafeOutputPath("escape/foo.mp4"); err == nil {
		t.Error("expected error for symlink that escapes cwd")
	}
}

func TestSafeOutputPath_RejectsControlChars(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)

	if _, err := SafeOutputPath("foo\x00bar"); err == nil {
		t.Error("expected error for null byte")
	}
	if _, err := SafeOutputPath("foo\u202Ebar"); err == nil {
		t.Error("expected error for Bidi override")
	}
}

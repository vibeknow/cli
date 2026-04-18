//go:build !windows

package keychain

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestKeychainCRUD exercises the platform-specific Get/Set/Delete roundtrip.
// On darwin the system Keychain is stubbed (see keychain_darwin_test.go) so
// this test is hermetic on any dev machine and CI runner.
func TestKeychainCRUD(t *testing.T) {
	dir := t.TempDir()

	origStorageDir := storageDir
	t.Cleanup(func() { storageDir = origStorageDir })
	storageDir = func(service string) string {
		return filepath.Join(dir, service)
	}

	stubSystemKeychain(t)

	k, err := OpenFor("vibeknow-test")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := k.Set("e1", []byte("secret")); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := k.Get("e1")
	if err != nil || string(got) != "secret" {
		t.Fatalf("get: %q err=%v", got, err)
	}
	if err := k.Delete("e1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := k.Get("e1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestSafeFileName(t *testing.T) {
	cases := map[string]string{
		"vibeknow.dev":     "vibeknow.dev.enc",
		"weird/../thing":   "weird_.._thing.enc",
		"plain":            "plain.enc",
		"spaces and :bad!": "spaces_and__bad_.enc",
	}
	for in, want := range cases {
		if got := safeFileName(in); got != want {
			t.Errorf("safeFileName(%q)=%q want %q", in, got, want)
		}
	}
}

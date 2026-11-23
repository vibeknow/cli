//go:build !windows

package cliauth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/keychain"
)

// TestReadOnlyStoreOnDisk runs the read-only case against the real file store
// rather than a fake: the credential directory is made read-only, as a sandbox
// that may read the user's home but not write it would leave it. The probe has
// to fail on the real write path, and the refresh token must not be presented.
func TestReadOnlyStoreOnDisk(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	home := t.TempDir()
	t.Setenv("VIBEKNOW_CONFIG_HOME", home)

	kc, err := keychain.OpenFor("vibeknow-test")
	if err != nil {
		t.Fatal(err)
	}
	stored := expiredOAuthToken().Marshal()
	if err := kc.Set("entry", stored); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	dir := filepath.Join(home, "keychain", "vibeknow-test")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	var presented []string
	srv := rotatingStub(t, &presented)
	defer srv.Close()

	p := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL, t.TempDir(),
	)
	if _, err := p.Token(context.Background()); !isStoreReadOnly(err) {
		t.Fatalf("Token err = %v, want the read-only store error", err)
	}
	if len(presented) != 0 {
		t.Errorf("refresh token presented %v with nowhere to keep its replacement", presented)
	}
	got, err := kc.Get("entry")
	if err != nil || string(got) != string(stored) {
		t.Errorf("stored credential changed or unreadable (err=%v)", err)
	}

	// Once the store is writable again the same provider refreshes normally.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if tok, err := p.Token(context.Background()); err != nil || tok != "at_rotated" {
		t.Fatalf("after restoring write access: Token = %q, %v; want at_rotated", tok, err)
	}
	if fresh, _ := (credential.KeychainSource{Keychain: kc, Entry: "entry"}).GetStored(); fresh.RefreshToken != "rt_rotated" {
		t.Errorf("rotated pair not persisted: refresh=%q", fresh.RefreshToken)
	}
}

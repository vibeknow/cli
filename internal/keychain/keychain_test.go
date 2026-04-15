package keychain

import (
	"testing"
)

func TestFileBackendCRUD(t *testing.T) {
	dir := t.TempDir()
	k, err := OpenFor("vibeknow-test", WithFileBackend(dir, "test-passphrase"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := k.Set("e1", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	got, err := k.Get("e1")
	if err != nil || string(got) != "secret" {
		t.Fatalf("get: %q err=%v", got, err)
	}
	if err := k.Delete("e1"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get("e1"); err == nil {
		t.Error("want not-found error after delete")
	}
}

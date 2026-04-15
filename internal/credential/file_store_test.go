package credential

import (
	"path/filepath"
	"testing"
)

func TestFileStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.enc")
	s := NewFileStore(path, "correct-horse-battery-staple")

	if err := s.Set([]byte("tok_abc")); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get()
	if err != nil || string(got) != "tok_abc" {
		t.Fatalf("get: %q err=%v", got, err)
	}

	s2 := NewFileStore(path, "wrong-passphrase")
	if _, err := s2.Get(); err == nil {
		t.Error("wrong passphrase should fail")
	}

	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(); err == nil {
		t.Error("expected not-found after delete")
	}
}

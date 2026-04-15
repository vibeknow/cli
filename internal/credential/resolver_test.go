package credential

import (
	"os"
	"path/filepath"
	"testing"
)

type fakeKC struct{ data map[string][]byte }

func (f *fakeKC) Get(k string) ([]byte, error) {
	v, ok := f.data[k]
	if !ok {
		return nil, ErrNotFound
	}
	return v, nil
}
func (f *fakeKC) Set(k string, v []byte) error { f.data[k] = v; return nil }
func (f *fakeKC) Delete(k string) error        { delete(f.data, k); return nil }

func TestResolverEnvWins(t *testing.T) {
	t.Setenv("VIBEKNOW_TOKEN", "from-env")
	r := Resolver{
		Env: EnvSource{Var: "VIBEKNOW_TOKEN"},
		Keychain: KeychainSource{
			Keychain: &fakeKC{data: map[string][]byte{"k1": []byte("from-keychain")}},
			Entry:    "k1",
		},
	}
	tok, src, err := r.Resolve()
	if err != nil || tok != "from-env" || src != "env" {
		t.Fatalf("tok=%q src=%q err=%v", tok, src, err)
	}
}

func TestResolverKeychainFallback(t *testing.T) {
	os.Unsetenv("VIBEKNOW_TOKEN")
	r := Resolver{
		Env: EnvSource{Var: "VIBEKNOW_TOKEN"},
		Keychain: KeychainSource{
			Keychain: &fakeKC{data: map[string][]byte{"k1": []byte("from-keychain")}},
			Entry:    "k1",
		},
	}
	tok, src, err := r.Resolve()
	if err != nil || tok != "from-keychain" || src != "keychain" {
		t.Fatalf("tok=%q src=%q err=%v", tok, src, err)
	}
}

func TestResolverFileFallback(t *testing.T) {
	os.Unsetenv("VIBEKNOW_TOKEN")
	dir := t.TempDir()
	path := filepath.Join(dir, "c.enc")
	fs := NewFileStore(path, "pw")
	if err := fs.Set([]byte("from-file")); err != nil {
		t.Fatal(err)
	}
	r := Resolver{
		Env:      EnvSource{Var: "VIBEKNOW_TOKEN"},
		Keychain: KeychainSource{Keychain: &fakeKC{data: map[string][]byte{}}, Entry: "missing"},
		File:     FileSource{Store: fs},
	}
	tok, src, err := r.Resolve()
	if err != nil || tok != "from-file" || src != "file" {
		t.Fatalf("tok=%q src=%q err=%v", tok, src, err)
	}
}

func TestResolverNone(t *testing.T) {
	os.Unsetenv("VIBEKNOW_TOKEN")
	r := Resolver{Env: EnvSource{Var: "VIBEKNOW_TOKEN"}}
	_, _, err := r.Resolve()
	if err == nil {
		t.Error("expected error when no source has credential")
	}
}

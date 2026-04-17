package cliauth

import (
	"context"
	"testing"

	"github.com/vibeknow/cli/internal/credential"
)

type fakeKeychain struct{ data map[string][]byte }

func (f *fakeKeychain) Get(key string) ([]byte, error) {
	v, ok := f.data[key]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return v, nil
}

func (f *fakeKeychain) Set(key string, data []byte) error {
	f.data[key] = data
	return nil
}

func (f *fakeKeychain) Delete(key string) error {
	delete(f.data, key)
	return nil
}

func TestOAuthTokenProvider_Valid(t *testing.T) {
	// Create a valid oauth token (expires in 1 hour, refresh in 24 hours).
	st := credential.NewOAuthToken("access-abc", "refresh-xyz", 3600, 86400)
	kc := &fakeKeychain{data: map[string][]byte{
		"test-entry": st.Marshal(),
	}}
	src := credential.KeychainSource{Keychain: kc, Entry: "test-entry"}
	provider := NewOAuthTokenProvider(src, "https://account.example.com", t.TempDir())

	ctx := context.Background()

	tok, err := provider.Token(ctx)
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok != "access-abc" {
		t.Errorf("Token() = %q, want %q", tok, "access-abc")
	}

	tt := provider.TokenType()
	if tt != "oauth" {
		t.Errorf("TokenType() = %q, want %q", tt, "oauth")
	}
}

func TestOAuthTokenProvider_PAT(t *testing.T) {
	st := credential.NewPATToken("pat-my-token")
	kc := &fakeKeychain{data: map[string][]byte{
		"test-entry": st.Marshal(),
	}}
	src := credential.KeychainSource{Keychain: kc, Entry: "test-entry"}
	provider := NewOAuthTokenProvider(src, "https://account.example.com", t.TempDir())

	ctx := context.Background()

	tok, err := provider.Token(ctx)
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok != "pat-my-token" {
		t.Errorf("Token() = %q, want %q", tok, "pat-my-token")
	}

	tt := provider.TokenType()
	if tt != "pat" {
		t.Errorf("TokenType() = %q, want %q", tt, "pat")
	}

	// ForceRefresh should fail for PAT.
	_, err = provider.ForceRefresh(ctx)
	if err == nil {
		t.Error("ForceRefresh() should return error for PAT")
	}
}

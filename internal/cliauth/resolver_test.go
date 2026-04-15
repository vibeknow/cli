package cliauth

import (
	"testing"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
)

func TestResolverForWithoutCredentialRef(t *testing.T) {
	r := ResolverFor(config.Profile{Name: "x"})
	// Env source should still be set.
	if r.Env.Var != "VIBEKNOW_TOKEN" {
		t.Errorf("env source not set: %+v", r.Env)
	}
}

func TestResolverForWithCredentialRef(t *testing.T) {
	r := ResolverFor(config.Profile{Name: "x", CredentialRef: "vibeknow.x"})
	if r.Keychain.Entry != "vibeknow.x" {
		t.Errorf("keychain entry not set: %+v", r.Keychain)
	}
}

// Package cliauth builds a credential.Resolver from a profile using the
// default keychain backend. Consolidates the pattern used by cmd/auth
// and cmd/api.
package cliauth

import (
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/keychain"
)

// ResolverFor builds a Resolver that checks VIBEKNOW_TOKEN first, then the
// keychain entry named by profile.CredentialRef (if set). Keychain errors are
// logged-and-skipped rather than propagated — the env fallback still works.
func ResolverFor(p config.Profile) credential.Resolver {
	r := credential.Resolver{
		Env: credential.EnvSource{Var: "VIBEKNOW_TOKEN"},
	}
	if p.CredentialRef != "" {
		if kc, err := keychain.OpenFor("vibeknow"); err == nil {
			r.Keychain = credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
		}
	}
	return r
}

// CurrentProfile loads profiles.yaml and returns the active profile, or an
// error if none is set. Consolidates the profile-lookup loop.
func CurrentProfile() (config.Profile, error) {
	f, err := config.LoadProfiles()
	if err != nil {
		return config.Profile{}, err
	}
	if f.Current == "" {
		return config.Profile{}, &NoActiveProfileError{}
	}
	for _, p := range f.Profiles {
		if p.Name == f.Current {
			return p, nil
		}
	}
	return config.Profile{}, &ProfileNotFoundError{Name: f.Current}
}

type NoActiveProfileError struct{}

func (*NoActiveProfileError) Error() string {
	return "no active profile; set one with `vibeknow profile use <name>`"
}

type ProfileNotFoundError struct{ Name string }

func (e *ProfileNotFoundError) Error() string {
	return "profile " + e.Name + " not found in profiles list"
}

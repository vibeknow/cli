// Package cmdutil wires cobra commands to shared lookups (profile, token
// provider, endpoint URL, I/O streams) via a small Factory so commands stay
// thin and testable.
package cmdutil

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/i18n"
)

type IOStreams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func DefaultIOStreams() IOStreams {
	return IOStreams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
}

// StaticToken is a bearer token that never refreshes — env PATs or already
// resolved OAuth access tokens.
type StaticToken string

func (s StaticToken) Token(_ context.Context) (string, error) { return string(s), nil }

// Factory is a DI container; every Fn field is overridable in tests.
type Factory struct {
	ProfileFn  func() (config.Profile, error)
	ResolverFn func(p config.Profile) credential.Resolver
	TokenFn    func(p config.Profile) httpclient.TokenProvider
	EndpointFn func(p config.Profile, service string) (string, error)

	IO IOStreams

	once    sync.Once
	profile config.Profile
	profErr error
}

func NewDefault() *Factory {
	return &Factory{
		ProfileFn:  cliauth.CurrentProfile,
		ResolverFn: cliauth.ResolverFor,
		TokenFn:    cliauth.TokenProviderFor,
		EndpointFn: func(p config.Profile, service string) (string, error) {
			return endpoints.Resolve(p, service)
		},
		IO: DefaultIOStreams(),
	}
}

// RequireProfile returns the active profile, cached across calls. Absence of
// an active profile surfaces as a typed auth error (exit 3).
func (f *Factory) RequireProfile() (config.Profile, error) {
	f.once.Do(func() {
		f.profile, f.profErr = f.ProfileFn()
	})
	return f.profile, f.profErr
}

// TokenProvider returns a refresh-capable token provider for the active
// profile (env var or keychain-backed OAuth). Missing credentials produce a
// typed auth error.
func (f *Factory) TokenProvider() (httpclient.TokenProvider, error) {
	p, err := f.RequireProfile()
	if err != nil {
		return nil, err
	}
	if tp := f.TokenFn(p); tp != nil {
		return tp, nil
	}
	return nil, clerr.Auth(i18n.T("auth.not_logged_in")).WithHint(i18n.T("auth.not_logged_in.hint"))
}

// Endpoint resolves a service URL for the active profile.
func (f *Factory) Endpoint(service string) (string, error) {
	p, err := f.RequireProfile()
	if err != nil {
		return "", err
	}
	return f.EndpointFn(p, service)
}

// Service returns (profile, url, tokenProvider) for the named service — the
// triple every typed client constructor needs. Any failure surfaces the
// appropriate typed error (auth / api).
func (f *Factory) Service(service string) (config.Profile, string, httpclient.TokenProvider, error) {
	p, err := f.RequireProfile()
	if err != nil {
		return config.Profile{}, "", nil, err
	}
	url, err := f.EndpointFn(p, service)
	if err != nil {
		return config.Profile{}, "", nil, err
	}
	tp, err := f.TokenProvider()
	if err != nil {
		return config.Profile{}, "", nil, err
	}
	return p, url, tp, nil
}

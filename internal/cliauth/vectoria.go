package cliauth

import (
	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/i18n"
)

// NewVectoriaClient builds a vectoria client for the active profile, using the
// same JWT-based auth chain as other CLI services.
func NewVectoriaClient() (*vectoria.Client, error) {
	p, err := CurrentProfile()
	if err != nil {
		return nil, err
	}
	url, err := endpoints.Resolve(p, "vectoria")
	if err != nil {
		return nil, err
	}
	tp := TokenProviderFor(p)
	if tp == nil {
		return nil, clerr.Auth(i18n.T("auth.not_logged_in")).WithHint(i18n.T("auth.not_logged_in.hint"))
	}
	return vectoria.New(url, tp), nil
}

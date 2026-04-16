package video

import (
	"context"
	"fmt"

	"github.com/shiliu-ai/vibeknow-cli/client/figlens"
	"github.com/shiliu-ai/vibeknow-cli/internal/cliauth"
	"github.com/shiliu-ai/vibeknow-cli/internal/endpoints"
)

func newFiglensClient() (*figlens.Client, error) {
	p, err := cliauth.CurrentProfile()
	if err != nil {
		return nil, err
	}
	tok, _, err := cliauth.ResolverFor(p).Resolve()
	if err != nil {
		return nil, fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
	}
	url, err := endpoints.Resolve(p, "figlens")
	if err != nil {
		return nil, err
	}
	return figlens.New(url, staticToken(tok)), nil
}

type staticToken string

func (s staticToken) Token(_ context.Context) (string, error) { return string(s), nil }

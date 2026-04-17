package video

import (
	"context"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/endpoints"
)

func newFiglensClient() (*figlens.Client, error) {
	p, err := cliauth.CurrentProfile()
	if err != nil {
		return nil, err
	}
	tok, _, err := cliauth.ResolverFor(p).Resolve()
	if err != nil {
		return nil, clerr.Auth("未登录").WithHint("运行 `vk auth login` 或 `vk init` 完成登录")
	}
	url, err := endpoints.Resolve(p, "figlens")
	if err != nil {
		return nil, err
	}
	return figlens.New(url, staticToken(tok)), nil
}

type staticToken string

func (s staticToken) Token(_ context.Context) (string, error) { return string(s), nil }

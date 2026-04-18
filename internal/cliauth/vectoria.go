package cliauth

import (
	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/endpoints"
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
		return nil, clerr.Auth("未登录").WithHint("运行 `vk auth login` 或 `vk init` 完成登录")
	}
	return vectoria.New(url, tp), nil
}

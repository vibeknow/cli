package endpoints

import (
	"fmt"

	"github.com/vibeknow/cli/internal/config"
)

// Resolve returns the effective URL for the given service in the profile.
// Profile override wins over cloud default.
func Resolve(p config.Profile, service string) (string, error) {
	if _, ok := CloudDefaults[service]; !ok {
		return "", fmt.Errorf("unknown service %q (expected one of: account, vectoria, figlens, vibeknow, share)", service)
	}
	if u, ok := p.Endpoints[service]; ok && u != "" {
		return u, nil
	}
	return CloudDefaults[service], nil
}

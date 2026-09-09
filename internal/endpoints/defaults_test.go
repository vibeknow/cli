package endpoints

import (
	"net/url"
	"testing"

	"github.com/vibeknow/cli/internal/config"
)

// Every value CloudDefaults ships must be listed in config's frozen-default
// set: profiles written by binaries ≤v0.6.3 carry a verbatim copy of the
// defaults of their day, and config.LoadProfiles can only heal copies it
// recognizes. Whoever changes CloudDefaults must append the old values to
// config.frozenDefaultEndpoints — this test is what makes forgetting loud.
func TestCloudDefaultsAreRegisteredAsFrozen(t *testing.T) {
	for svc, raw := range CloudDefaults {
		if !config.IsFrozenDefaultEndpoint(raw) {
			t.Errorf("CloudDefaults[%q]=%q missing from config.frozenDefaultEndpoints", svc, raw)
		}
	}
}

func TestCloudDefaultsAreValidAbsoluteURLs(t *testing.T) {
	// Ensure the original four core API services are always present.
	required := []string{"account", "vectoria", "figlens", "vibeknow"}
	for _, svc := range required {
		if _, ok := CloudDefaults[svc]; !ok {
			t.Errorf("CloudDefaults missing %q", svc)
		}
	}

	// Validate every entry in CloudDefaults is a valid absolute HTTPS URL.
	for svc, raw := range CloudDefaults {
		u, err := url.Parse(raw)
		if err != nil {
			t.Errorf("%s: url.Parse(%q) failed: %v", svc, raw, err)
			continue
		}
		if u.Scheme != "https" {
			t.Errorf("%s: scheme=%q, want https", svc, u.Scheme)
		}
		if u.Host == "" {
			t.Errorf("%s: empty host in %q", svc, raw)
		}
	}
}

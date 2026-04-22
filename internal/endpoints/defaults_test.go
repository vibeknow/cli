package endpoints

import (
	"net/url"
	"testing"
)

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

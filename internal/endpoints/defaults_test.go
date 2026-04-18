package endpoints

import (
	"net/url"
	"testing"
)

func TestCloudDefaultsAreValidAbsoluteURLs(t *testing.T) {
	required := []string{"account", "vectoria", "figlens", "vibeknow"}
	for _, svc := range required {
		raw, ok := CloudDefaults[svc]
		if !ok {
			t.Errorf("CloudDefaults missing %q", svc)
			continue
		}
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

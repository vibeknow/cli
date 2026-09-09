package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const frozenBetaYAML = `schema_version: "2"
current: default
profiles:
    - name: default
      endpoints:
        account: https://beta.lab.shiliu.chat/account
        figlens: https://beta.lab.shiliu.chat/figlens
        vectoria: https://beta.lab.shiliu.chat/vectoria
        vibeknow: https://beta.lab.shiliu.chat/vibeknow
      credential_ref: vibeknow.default
      trust: user
      is_production: true
`

func writeProfilesYAML(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "profiles.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadScrubsFrozenBetaEndpoints(t *testing.T) {
	dir := withTempHome(t)
	path := writeProfilesYAML(t, dir, frozenBetaYAML)

	f, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(f.Profiles[0].Endpoints); got != 0 {
		t.Errorf("frozen beta endpoints survived the load: %v", f.Profiles[0].Endpoints)
	}

	// The heal must reach the disk too, or every other consumer of the file
	// (and every future binary) starts from the poisoned copy again.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "beta.lab.shiliu.chat") {
		t.Errorf("profiles.yaml still carries beta endpoints:\n%s", data)
	}
}

func TestLoadScrubsFrozenProdEndpointsButKeepsUserOverride(t *testing.T) {
	dir := withTempHome(t)
	writeProfilesYAML(t, dir, `schema_version: "2"
current: default
profiles:
    - name: default
      endpoints:
        account: https://vibeknow.com/account
        vibeknow: https://corp-proxy.example.com/vibeknow
      credential_ref: vibeknow.default
      trust: user
      is_production: true
`)

	f, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	eps := f.Profiles[0].Endpoints
	if _, ok := eps["account"]; ok {
		t.Error("frozen production default should be scrubbed")
	}
	if eps["vibeknow"] != "https://corp-proxy.example.com/vibeknow" {
		t.Errorf("user override must survive, got %v", eps)
	}
}

func TestLoadKeepsDevNonProdProfileUntouched(t *testing.T) {
	dir := withTempHome(t)
	writeProfilesYAML(t, dir, `schema_version: "2"
current: beta
profiles:
    - name: beta
      endpoints:
        account: https://beta.lab.shiliu.chat/account
        vibeknow: https://beta.lab.shiliu.chat/vibeknow
      credential_ref: vibeknow.beta
      trust: dev
      is_production: false
`)

	f, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	eps := f.Profiles[0].Endpoints
	if eps["vibeknow"] != "https://beta.lab.shiliu.chat/vibeknow" {
		t.Errorf("deliberate dev/non-prod endpoints must be kept, got %v", eps)
	}
}

func TestLoadScrubsFrozenLegacyAPIEndpoint(t *testing.T) {
	dir := withTempHome(t)
	writeProfilesYAML(t, dir, `schema_version: "2"
current: default
profiles:
    - name: default
      api_endpoint: https://api.vibeknow.com
      credential_ref: vibeknow.default
      trust: user
      is_production: true
`)

	f, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	p := f.Profiles[0]
	if p.APIEndpoint != "" || len(p.Endpoints) != 0 {
		t.Errorf("frozen api_endpoint should be scrubbed, got api_endpoint=%q endpoints=%v", p.APIEndpoint, p.Endpoints)
	}
}

package config

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigDir(t *testing.T) {
	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(dir, "vibeknow") {
			t.Errorf("windows dir %q missing vibeknow", dir)
		}
	} else {
		home, _ := os.UserHomeDir()
		if !strings.HasPrefix(dir, home) || !strings.Contains(dir, "vibeknow") {
			t.Errorf("unix dir %q not under %s/.../vibeknow", dir, home)
		}
	}
}

func TestProfileValidate(t *testing.T) {
	valid := Profile{
		Name:          "prod",
		Endpoints:     map[string]string{"vibeknow": "https://api.vibeknow.com"},
		CredentialRef: "vibeknow.prod",
		Trust:         "user",
		IsProduction:  true,
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid profile rejected: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*Profile)
		want string
	}{
		{"empty name", func(p *Profile) { p.Name = "" }, "name"},
		{"bad url", func(p *Profile) { p.Endpoints = map[string]string{"vibeknow": "not-a-url"} }, "endpoints"},
		{"bad trust", func(p *Profile) { p.Trust = "admin" }, "trust"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := valid
			c.mut(&p)
			err := p.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestProfileYAMLOmittedIsProduction(t *testing.T) {
	data := []byte(`
name: legacy
api_endpoint: https://api.example.com
credential_ref: k
trust: dev
endpoints:
  figlens: http://localhost:9000
`)
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !p.IsProduction {
		t.Errorf("IsProduction defaulted to %v; want true", p.IsProduction)
	}
	// And because IsProduction defaults true, localhost endpoint must be rejected.
	if err := p.Validate(); err == nil {
		t.Error("Validate should reject localhost endpoint when IsProduction defaulted to true")
	}
}

func TestProfileYAMLExplicitIsProductionFalse(t *testing.T) {
	data := []byte(`
name: dev
api_endpoint: https://api.example.com
credential_ref: k
trust: dev
is_production: false
endpoints:
  figlens: http://localhost:9000
`)
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.IsProduction {
		t.Errorf("IsProduction = true despite explicit false")
	}
	if err := p.Validate(); err != nil {
		t.Errorf("expected validate ok, got: %v", err)
	}
}

func TestProfileEndpointsRoundtrip(t *testing.T) {
	data := []byte(`
name: prod
endpoints:
  account: https://account.example.com
  vectoria: https://vectoria.example.com
  figlens: https://figlens.example.com
  vibeknow: https://api.example.com
credential_ref: k
trust: user
is_production: true
`)
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Endpoints["figlens"] != "https://figlens.example.com" {
		t.Errorf("figlens missing: %+v", p.Endpoints)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("valid profile rejected: %v", err)
	}
}

func TestProfileLegacyAPIEndpointMapping(t *testing.T) {
	data := []byte(`
name: legacy
api_endpoint: https://api.example.com
credential_ref: k
trust: user
is_production: true
`)
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if got := p.Endpoints["vibeknow"]; got != "https://api.example.com" {
		t.Errorf("legacy api_endpoint not migrated; endpoints=%+v", p.Endpoints)
	}
	if p.APIEndpoint != "https://api.example.com" {
		t.Errorf("APIEndpoint should be preserved for deprecation warning: %q", p.APIEndpoint)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("migrated profile should validate: %v", err)
	}
}

func TestProfileRejectsUnknownEndpointKey(t *testing.T) {
	data := []byte(`
name: bad
endpoints:
  banana: https://oops
credential_ref: k
trust: user
`)
	var p Profile
	_ = yaml.Unmarshal(data, &p)
	if err := p.Validate(); err != nil {
		t.Errorf("unknown keys dropped; remaining empty map should validate: %v", err)
	}
	if _, ok := p.Endpoints["banana"]; ok {
		t.Errorf("banana endpoint should have been dropped: %+v", p.Endpoints)
	}
}

func TestProfileNonProdEndpointRequiresDevTrust(t *testing.T) {
	data := []byte(`
name: sneaky
endpoints:
  figlens: http://localhost:20067
credential_ref: k
trust: user
is_production: true
`)
	var p Profile
	_ = yaml.Unmarshal(data, &p)
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "trust") {
		t.Errorf("localhost endpoint without trust=dev should fail with trust error, got: %v", err)
	}
}

func TestProfileNonProdEndpointAllowedWithDevTrust(t *testing.T) {
	data := []byte(`
name: dev
endpoints:
  figlens: http://localhost:20067
credential_ref: k
trust: dev
is_production: false
`)
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("dev profile with localhost should validate: %v", err)
	}
}

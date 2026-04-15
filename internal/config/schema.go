package config

import (
	"fmt"
	"net/url"
	"regexp"
)

// Profile is the canonical profile shape. See spec §4.3 and §11.3.
type Profile struct {
	Name             string            `yaml:"name"`
	APIEndpoint      string            `yaml:"api_endpoint"`
	CredentialRef    string            `yaml:"credential_ref"`
	DefaultProject   string            `yaml:"default_project,omitempty"`
	Trust            string            `yaml:"trust,omitempty"`    // user | dev
	IsProduction     bool              `yaml:"is_production"`      // default true
	ServiceOverrides map[string]string `yaml:"service_overrides,omitempty"`
}

// ProfilesFile is the top-level YAML shape.
type ProfilesFile struct {
	SchemaVersion string    `yaml:"schema_version"`
	Current       string    `yaml:"current"`
	Profiles      []Profile `yaml:"profiles"`
}

var nameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// Validate enforces the rules listed in spec §11.3.
func (p Profile) Validate() error {
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("profile.name %q invalid (must match %s)", p.Name, nameRe)
	}
	u, err := url.Parse(p.APIEndpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("profile.api_endpoint %q must be an absolute URL", p.APIEndpoint)
	}
	if p.CredentialRef == "" {
		return fmt.Errorf("profile.credential_ref is required")
	}
	trust := p.Trust
	if trust == "" {
		trust = "user"
	}
	if trust != "user" && trust != "dev" {
		return fmt.Errorf("profile.trust must be 'user' or 'dev', got %q", trust)
	}
	if len(p.ServiceOverrides) > 0 {
		if trust != "dev" {
			return fmt.Errorf("profile.service_overrides requires trust=dev")
		}
		if p.IsProduction {
			return fmt.Errorf("profile.service_overrides requires is_production=false")
		}
	}
	return nil
}

// ValidateFile checks top-level invariants and each profile.
func (f ProfilesFile) Validate() error {
	seen := map[string]bool{}
	for _, p := range f.Profiles {
		if seen[p.Name] {
			return fmt.Errorf("duplicate profile name %q", p.Name)
		}
		seen[p.Name] = true
		if err := p.Validate(); err != nil {
			return fmt.Errorf("profile %q: %w", p.Name, err)
		}
	}
	if f.Current != "" && !seen[f.Current] {
		return fmt.Errorf("current %q references unknown profile", f.Current)
	}
	return nil
}

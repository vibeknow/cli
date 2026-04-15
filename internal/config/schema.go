package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var allowedEndpointKeys = map[string]bool{
	"account":  true,
	"vectoria": true,
	"figlens":  true,
	"vibeknow": true,
}

type Profile struct {
	Name           string            `yaml:"name"`
	Endpoints      map[string]string `yaml:"endpoints,omitempty"`
	APIEndpoint    string            `yaml:"api_endpoint,omitempty"`
	CredentialRef  string            `yaml:"credential_ref"`
	DefaultProject string            `yaml:"default_project,omitempty"`
	Trust          string            `yaml:"trust,omitempty"`
	IsProduction   bool              `yaml:"is_production"`
}

type ProfilesFile struct {
	SchemaVersion string    `yaml:"schema_version"`
	Current       string    `yaml:"current"`
	Profiles      []Profile `yaml:"profiles"`
}

var nameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

func (p *Profile) UnmarshalYAML(node *yaml.Node) error {
	type shadow struct {
		Name           string            `yaml:"name"`
		Endpoints      map[string]string `yaml:"endpoints,omitempty"`
		APIEndpoint    string            `yaml:"api_endpoint,omitempty"`
		CredentialRef  string            `yaml:"credential_ref"`
		DefaultProject string            `yaml:"default_project,omitempty"`
		Trust          string            `yaml:"trust,omitempty"`
		IsProduction   *bool             `yaml:"is_production,omitempty"`
	}
	var s shadow
	if err := node.Decode(&s); err != nil {
		return err
	}
	p.Name = s.Name
	p.CredentialRef = s.CredentialRef
	p.DefaultProject = s.DefaultProject
	p.Trust = s.Trust
	if s.IsProduction == nil {
		p.IsProduction = true
	} else {
		p.IsProduction = *s.IsProduction
	}
	p.Endpoints = map[string]string{}
	for k, v := range s.Endpoints {
		if allowedEndpointKeys[k] {
			p.Endpoints[k] = v
		}
	}
	p.APIEndpoint = s.APIEndpoint
	if s.APIEndpoint != "" {
		if _, ok := p.Endpoints["vibeknow"]; !ok {
			p.Endpoints["vibeknow"] = s.APIEndpoint
		}
	}
	return nil
}

// Validate enforces the rules listed in spec §11.3 v2.
func (p Profile) Validate() error {
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("profile.name %q invalid (must match %s)", p.Name, nameRe)
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
	for svc, rawURL := range p.Endpoints {
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("profile.endpoints[%q] %q must be absolute URL", svc, rawURL)
		}
		if isNonProdURL(u) {
			if trust != "dev" || p.IsProduction {
				return fmt.Errorf("profile.endpoints[%q]=%q is non-production; requires trust=dev and is_production=false", svc, rawURL)
			}
		}
	}
	return nil
}

func isNonProdURL(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") {
		return true
	}
	return false
}

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

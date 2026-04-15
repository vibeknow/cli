package config

import (
	"os"
	"runtime"
	"strings"
	"testing"
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
		Name:         "prod",
		APIEndpoint:  "https://api.example.com",
		CredentialRef: "vibeknow.prod",
		Trust:        "user",
		IsProduction: true,
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
		{"bad url", func(p *Profile) { p.APIEndpoint = "not-a-url" }, "api_endpoint"},
		{"bad trust", func(p *Profile) { p.Trust = "admin" }, "trust"},
		{"overrides when trusted-user", func(p *Profile) {
			p.Trust = "user"
			p.ServiceOverrides = map[string]string{"figlens": "http://localhost"}
		}, "service_overrides"},
		{"overrides when production", func(p *Profile) {
			p.Trust = "dev"
			p.IsProduction = true
			p.ServiceOverrides = map[string]string{"figlens": "http://localhost"}
		}, "is_production"},
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

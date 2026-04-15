package endpoints

import (
	"testing"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
)

func TestResolveUsesProfileOverride(t *testing.T) {
	p := config.Profile{
		Trust: "dev", IsProduction: false,
		Endpoints: map[string]string{"figlens": "http://localhost:9000"},
	}
	url, err := Resolve(p, "figlens")
	if err != nil || url != "http://localhost:9000" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestResolveFallsBackToCloud(t *testing.T) {
	p := config.Profile{Trust: "user", IsProduction: true, Endpoints: map[string]string{}}
	url, err := Resolve(p, "account")
	if err != nil {
		t.Fatal(err)
	}
	if url != CloudDefaults["account"] {
		t.Errorf("expected cloud default, got %q", url)
	}
}

func TestResolveUnknownService(t *testing.T) {
	p := config.Profile{}
	_, err := Resolve(p, "banana")
	if err == nil {
		t.Error("unknown service should error")
	}
}

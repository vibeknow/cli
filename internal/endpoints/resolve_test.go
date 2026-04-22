package endpoints

import (
	"testing"

	"github.com/vibeknow/cli/internal/config"
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

func TestResolveShare_Default(t *testing.T) {
	p := config.Profile{Name: "default"}
	url, err := Resolve(p, "share")
	if err != nil {
		t.Fatalf("resolve share: %v", err)
	}
	if url != "https://vibeknow.com/share" {
		t.Fatalf("url = %q, want https://vibeknow.com/share", url)
	}
}

func TestResolveShare_ProfileOverride(t *testing.T) {
	p := config.Profile{
		Name:      "self",
		Endpoints: map[string]string{"share": "https://self.example.com/share"},
	}
	url, err := Resolve(p, "share")
	if err != nil {
		t.Fatalf("resolve share: %v", err)
	}
	if url != "https://self.example.com/share" {
		t.Fatalf("url = %q", url)
	}
}

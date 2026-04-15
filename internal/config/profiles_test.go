package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("VIBEKNOW_CONFIG_HOME", dir)
	return dir
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	withTempHome(t)
	f, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if f.SchemaVersion != "2" || len(f.Profiles) != 0 {
		t.Errorf("unexpected initial state: %+v", f)
	}
}

func TestSaveThenLoadRoundtrip(t *testing.T) {
	dir := withTempHome(t)
	f := ProfilesFile{
		SchemaVersion: "1",
		Current:       "prod",
		Profiles: []Profile{{
			Name: "prod", APIEndpoint: "https://x",
			CredentialRef: "k", Trust: "user", IsProduction: true,
		}},
	}
	if err := SaveProfiles(f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "profiles.yaml")); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	got, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "prod" || len(got.Profiles) != 1 || got.Profiles[0].Name != "prod" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

func TestAddUseRemove(t *testing.T) {
	withTempHome(t)
	p := Profile{Name: "dev", APIEndpoint: "https://d", CredentialRef: "k", Trust: "dev", IsProduction: false}
	if err := AddProfile(p); err != nil {
		t.Fatal(err)
	}
	if err := AddProfile(p); err == nil {
		t.Error("expected duplicate error")
	}
	if err := UseProfile("dev"); err != nil {
		t.Fatal(err)
	}
	if err := UseProfile("missing"); err == nil {
		t.Error("expected not-found error")
	}
	if err := RemoveProfile("dev"); err != nil {
		t.Fatal(err)
	}
	f, _ := LoadProfiles()
	if len(f.Profiles) != 0 || f.Current != "" {
		t.Errorf("remove did not clear state: %+v", f)
	}
}

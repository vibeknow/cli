package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
)

func TestLoginNoWaitJSONShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/device/code" {
			http.Error(w, "unexpected path", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "ok",
			"data": {
				"device_code": "dc_test_abc",
				"user_code": "WXYZ-1234",
				"verification_uri": "https://example.test/activate",
				"expires_in": 900,
				"interval": 5
			}
		}`))
	}))
	defer srv.Close()

	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())

	if err := config.AddProfile(config.Profile{
		Name:          "default",
		CredentialRef: "vibeknow.default",
		Endpoints:     map[string]string{"account": srv.URL},
		Trust:         "dev",
		IsProduction:  false,
	}); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	if err := config.UseProfile("default"); err != nil {
		t.Fatalf("use profile: %v", err)
	}

	root := &cobra.Command{Use: "vibeknow"}
	root.AddCommand(loginCmd)

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"login", "--no-wait"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}

	for _, field := range []string{"device_code", "user_code", "verification_uri", "expires_in", "hint"} {
		if _, ok := got[field]; !ok {
			t.Errorf("missing field %q in envelope: %+v", field, got)
		}
	}
	if got["device_code"] != "dc_test_abc" {
		t.Errorf("device_code = %v, want dc_test_abc", got["device_code"])
	}
	if v, ok := got["expires_in"].(float64); !ok || v != 900 {
		t.Errorf("expires_in = %v (%T), want 900 (float64 after JSON decode)", got["expires_in"], got["expires_in"])
	}
}

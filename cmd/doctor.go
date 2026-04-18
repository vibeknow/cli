package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/keychain"
)

type check struct {
	name string
	fn   func() error
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "diagnose local setup and endpoint reachability",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(i18n.T("doctor.header"))
		checks := []check{
			{"config directory writable", checkConfigDir},
			{"profiles.yaml parseable", checkProfiles},
			{"keychain backend reachable", checkKeychain},
			{"locale detection", checkLocale},
		}
		failed := 0
		for _, c := range checks {
			if err := c.fn(); err != nil {
				fmt.Println(i18n.T("doctor.fail", c.name, err.Error()))
				failed++
			} else {
				fmt.Println(i18n.T("doctor.ok", c.name))
			}
		}

		failed += checkEndpoints()

		if failed > 0 {
			return fmt.Errorf("%d check(s) failed", failed)
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(doctorCmd) }

func checkConfigDir() error {
	d, err := config.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(d, ".write-test")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_ = f.Close()
	return os.Remove(tmp)
}

func checkProfiles() error {
	_, err := config.LoadProfiles()
	return err
}

func checkKeychain() error {
	kc, err := keychain.OpenFor("vibeknow")
	if err != nil {
		return err
	}
	const probe = "__doctor_probe__"
	if err := kc.Set(probe, []byte("ok")); err != nil {
		return err
	}
	defer func() { _ = kc.Delete(probe) }()
	got, err := kc.Get(probe)
	if err != nil {
		return err
	}
	if string(got) != "ok" {
		return fmt.Errorf("round-trip mismatch: got %q", got)
	}
	return nil
}

func checkLocale() error {
	if got := i18n.T("doctor.header"); got == "" {
		return fmt.Errorf("i18n returned empty string")
	}
	return nil
}

// checkEndpoints resolves each service endpoint and probes /healthz
// concurrently. Every backend runs go-atlas ≥ v0.3.6, which registers health
// routes under the service base group and responds with {"status":"healthy"}
// on 200 or {"status":"unhealthy"} on 503.
func checkEndpoints() int {
	f, err := config.LoadProfiles()
	if err != nil || f.Current == "" {
		fmt.Println("[skip] endpoints: no active profile")
		return 0
	}
	var prof *config.Profile
	for i := range f.Profiles {
		if f.Profiles[i].Name == f.Current {
			prof = &f.Profiles[i]
		}
	}
	if prof == nil {
		return 0
	}
	services := []string{"account", "vectoria", "figlens", "vibeknow"}
	type result struct {
		svc, url string
		ok       bool
		detail   string
	}
	results := make([]result, len(services))
	var wg sync.WaitGroup
	for i, svc := range services {
		wg.Add(1)
		go func(i int, svc string) {
			defer wg.Done()
			url, err := endpoints.Resolve(*prof, svc)
			if err != nil {
				results[i] = result{svc: svc, detail: err.Error()}
				return
			}
			results[i] = result{svc: svc, url: url}
			results[i].ok, results[i].detail = probeHealth(url)
		}(i, svc)
	}
	wg.Wait()

	failed := 0
	for _, r := range results {
		name := fmt.Sprintf("%s endpoint %s", r.svc, r.url)
		if r.ok {
			fmt.Println(i18n.T("doctor.ok", name+"  "+r.detail))
		} else {
			fmt.Println(i18n.T("doctor.fail", name, r.detail))
			failed++
		}
	}
	return failed
}

// probeHealth GETs /healthz and returns (ok, detail). A response is healthy
// iff it returns HTTP 200 and a body with `"status": "healthy"`.
func probeHealth(baseURL string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/healthz", nil)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var shape struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(body, &shape)

	if resp.StatusCode == 200 && shape.Status == "healthy" {
		return true, "api=" + resp.Header.Get("X-Vibeknow-Api-Version")
	}
	return false, fmt.Sprintf("http=%d status=%q", resp.StatusCode, shape.Status)
}

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// checkEndpoints resolves each service endpoint and probes its health route
// concurrently. Each backend is expected to expose /healthz (with /health as
// a fallback) returning HTTP 200 + {"status":"healthy"} when fully up, or
// HTTP 503 + {"status":"unhealthy"} when degraded.
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
		status   probeStatus
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
				results[i] = result{svc: svc, status: probeFail, detail: err.Error()}
				return
			}
			results[i] = result{svc: svc, url: url}
			results[i].status, results[i].detail = probeHealth(url)
		}(i, svc)
	}
	wg.Wait()

	failed := 0
	for _, r := range results {
		name := fmt.Sprintf("%s endpoint %s", r.svc, r.url)
		switch r.status {
		case probeOK:
			msg := name
			if r.detail != "" {
				msg = name + "  " + r.detail
			}
			fmt.Println(i18n.T("doctor.ok", msg))
		case probeDegraded:
			fmt.Println(i18n.T("doctor.warn", name, r.detail))
		case probeFail:
			fmt.Println(i18n.T("doctor.fail", name, r.detail))
			failed++
		}
	}
	return failed
}

type probeStatus int

const (
	probeOK probeStatus = iota
	probeDegraded
	probeFail
)

// probeHealth checks a service's health endpoint and classifies the result.
// It probes /healthz first; if that 404s, it falls back to /health so backends
// that haven't standardised on the /healthz path still report correctly.
//
//   - probeOK:       HTTP 200 (the service is reachable and self-reports up;
//                    body status string varies across services — "healthy",
//                    "ok", "up" — so we treat any 2xx as success rather than
//                    coupling to a specific keyword)
//   - probeDegraded: HTTP 503 + body status="unhealthy" but pillars.databases
//                    is healthy (non-critical subsystem like email is down,
//                    primary request path is still usable)
//   - probeFail:     transport error, unexpected HTTP, DB pillar unhealthy,
//                    or 503 with no parseable pillars info
func probeHealth(baseURL string) (probeStatus, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 3 * time.Second}

	code, header, body, err := tryProbe(ctx, client, baseURL+"/healthz")
	if err == nil && code == http.StatusNotFound {
		code, header, body, err = tryProbe(ctx, client, baseURL+"/health")
	}
	if err != nil {
		return probeFail, err.Error()
	}

	if code == http.StatusOK {
		detail := ""
		if v := header.Get("X-Vibeknow-Api-Version"); v != "" {
			detail = "api=" + v
		}
		return probeOK, detail
	}

	var shape struct {
		Status  string `json:"status"`
		Pillars map[string]struct {
			Status string `json:"status"`
		} `json:"pillars"`
	}
	_ = json.Unmarshal(body, &shape)

	if code == http.StatusServiceUnavailable && shape.Pillars["databases"].Status == "healthy" {
		var down []string
		for name, p := range shape.Pillars {
			if p.Status != "healthy" {
				down = append(down, name)
			}
		}
		sort.Strings(down)
		return probeDegraded, fmt.Sprintf("non-critical pillars down: %s", strings.Join(down, ","))
	}

	return probeFail, fmt.Sprintf("http=%d status=%q", code, shape.Status)
}

func tryProbe(ctx context.Context, client *http.Client, url string) (int, http.Header, []byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, body, nil
}

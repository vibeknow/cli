package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// healthProbePaths is the ordered list of paths doctor tries on each service
// before concluding the service has no reachable health endpoint. Order
// matters: atlas-based services expose /healthz at the service base group
// (post go-atlas v0.3.5); older services kept /v1/health; some Python
// services ship /health at root.
var healthProbePaths = []string{"/healthz", "/v1/health", "/health"}

// probeResult tracks the outcome of probing one service.
type probeResult struct {
	svc    string
	url    string
	status probeStatus
	detail string
}

type probeStatus int

const (
	probeOK probeStatus = iota
	// probeNoHealthEndpoint: every tried path 404'd but the host is reachable
	// (at least one response came back). Not a CLI failure — the service is
	// simply not exposing a health endpoint the CLI recognizes. Reported as
	// WARN rather than FAIL.
	probeNoHealthEndpoint
	probeFail
)

// checkEndpoints resolves each service endpoint and probes health concurrently.
// Returns the number of hard failures (transport errors, 5xx, malformed
// envelopes). "Health endpoint not found" is surfaced as a warning and does
// not count toward the failure total.
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
	results := make([]probeResult, len(services))
	var wg sync.WaitGroup
	for i, svc := range services {
		wg.Add(1)
		go func(i int, svc string) {
			defer wg.Done()
			results[i] = probeService(*prof, svc)
		}(i, svc)
	}
	wg.Wait()

	failed := 0
	for _, r := range results {
		name := fmt.Sprintf("%s endpoint %s", r.svc, r.url)
		switch r.status {
		case probeOK:
			fmt.Println(i18n.T("doctor.ok", name+"  "+r.detail))
		case probeNoHealthEndpoint:
			fmt.Println(i18n.T("doctor.warn", name, r.detail))
		case probeFail:
			fmt.Println(i18n.T("doctor.fail", name, r.detail))
			failed++
		}
	}
	return failed
}

// probeService tries the configured health paths in order. The first path
// returning 200 with a recognizable "ok" body wins. If every path returns 404,
// the service is classified as probeNoHealthEndpoint (warn, not fail). Any
// other failure (transport error, 5xx, malformed body on 200) is a hard fail
// reported by the first path that produced it.
func probeService(prof config.Profile, svc string) probeResult {
	url, err := endpoints.Resolve(prof, svc)
	if err != nil {
		return probeResult{svc: svc, status: probeFail, detail: err.Error()}
	}
	res := probeResult{svc: svc, url: url}

	var firstHardFail string
	all404 := true

	for _, path := range healthProbePaths {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "GET", url+path, nil)
		client := &http.Client{Timeout: 3 * time.Second}
		resp, rerr := client.Do(req)
		cancel()
		if rerr != nil {
			// Transport error (DNS, TLS, timeout) — no point trying more paths;
			// they'll all fail the same way.
			res.status = probeFail
			res.detail = rerr.Error()
			return res
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		gotVer := resp.Header.Get("X-Vibeknow-Api-Version")

		if resp.StatusCode == 404 {
			continue
		}
		all404 = false

		if resp.StatusCode == 200 && parseHealthOK(body) {
			res.status = probeOK
			res.detail = fmt.Sprintf("path=%s api=%s", path, gotVer)
			return res
		}

		// Got a response but not a good one. Remember the first one so we can
		// report it if no later path works.
		if firstHardFail == "" {
			firstHardFail = fmt.Sprintf("path=%s http=%d body=%s", path, resp.StatusCode, truncateForLog(body, 80))
		}
	}

	if all404 {
		res.status = probeNoHealthEndpoint
		res.detail = "health endpoint not exposed (tried " + strings.Join(healthProbePaths, ", ") + ")"
		return res
	}
	res.status = probeFail
	res.detail = firstHardFail
	return res
}

// parseHealthOK accepts both shapes:
//   - flat:     {"status":"ok", ...}
//   - envelope: {"code":0,"data":{"status":"ok",...}, ...}
func parseHealthOK(body []byte) bool {
	var flat struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &flat); err == nil && flat.Status == "ok" {
		return true
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Code == 0 && envelope.Data.Status == "ok" {
		return true
	}
	// Atlas framework livez/healthz bodies use "healthy" rather than "ok".
	var atlasShape struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &atlasShape); err == nil && atlasShape.Status == "healthy" {
		return true
	}
	return false
}

func truncateForLog(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

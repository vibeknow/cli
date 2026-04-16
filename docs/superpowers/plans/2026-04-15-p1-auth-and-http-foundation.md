# P1: Auth & HTTP Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the CLI-side HTTP foundation and minimum auth surface so that any subsequent command (P2 shortcuts, P2 clients) can talk to the 4 backend services using a `VIBEKNOW_TOKEN`-injected JWT. Profile schema migrates from P0's single `api_endpoint` to a 4-entry `endpoints` map.

**Architecture:** Multi-endpoint direct-connect (spec §4.1 v2.3). CLI holds per-service HTTP clients built on a shared `internal/httpclient` stack with middleware for token injection, trace-id, version-skew check, verbose+redact logging, and 5xx retry. Auth in P1 is **`VIBEKNOW_TOKEN` env only** — no interactive login; Device Flow + PAT are deferred to a separate P1.5 project (spec §4.2).

**Tech Stack:** Go 1.25+, existing P0 packages (`internal/config`, `internal/credential`, `internal/output`, `internal/i18n`, `internal/redact`, `internal/charcheck`), net/http, testing/httptest, stretchr/testify (add).

**Scope boundary (what P1 does NOT include):**
- `auth login` interactive command (P1.5 — Device Flow + PAT).
- `client/{vectoria,figlens,vibeknow,speech}` service-specific methods (P2 — built alongside shortcuts).
- `cmd/video` / `cmd/doc` / `cmd/rag` / `cmd/voice` / `cmd/project` (P2).
- Refresh token auto-renewal (needs stored refresh_token; P1's env-only path doesn't have one).
- Rate limit header surfacing (future).

---

## Backend prerequisites (parallel track, not in this plan)

Three lightweight changes in the 4 external services (all use go-atlas shared framework, so one PR covers all):

1. **Response middleware adds `X-Vibeknow-Api-Version: v1` header** on every response. CLI compares; major-version mismatch → exit 3 with version_mismatch error.
2. **`/v1/health` returns** `{status: "ok", version: "<git-sha>", api_version: "v1"}` (JSON body, 200).
3. **Error shape adds `retryable: bool` field** (default false). Retryable-for-CLI: transient network/5xx/lock conflicts.

CLI development does NOT block on these — integration tests use `httptest` fakes that emit the target shape. Backend changes can land in parallel and integration is verified when staging picks them up.

Contract doc for these is Task 1's deliverable.

---

## File Structure (what this plan creates/modifies)

```
vibeknow-cli/
├── docs/
│   └── contracts/
│       └── p1-backend.md              # T1: backend contract doc
├── internal/
│   ├── config/
│   │   ├── schema.go                  # T2: add Endpoints map, keep APIEndpoint compat
│   │   └── schema_test.go             # T2: migration tests
│   ├── endpoints/                     # T3: NEW
│   │   ├── defaults.go                # cloud defaults
│   │   ├── resolve.go                 # profile→URL, trust boundary checks
│   │   └── resolve_test.go
│   ├── httpclient/                    # T4–T9: NEW
│   │   ├── client.go                  # T4: BaseClient, Request/Do, error mapping
│   │   ├── errors.go                  # T4: backend {code,message,...} → cli error.Object
│   │   ├── transport.go               # T5: RoundTripper wrapper + middleware chain
│   │   ├── mw_auth.go                 # T6: token injection
│   │   ├── mw_traceid.go              # T6: X-Trace-Id header
│   │   ├── mw_verbose.go              # T7: --verbose logging with redact
│   │   ├── mw_version.go              # T8: X-Vibeknow-Api-Version check
│   │   ├── mw_retry.go                # T9: 5xx exponential backoff
│   │   └── *_test.go
│   └── errs/                          # T4: NEW (shared error object)
│       ├── object.go
│       └── object_test.go
├── client/
│   └── account/                       # T10: NEW
│       ├── client.go
│       ├── whoami.go
│       └── whoami_test.go
├── cmd/
│   ├── auth/                          # T11: NEW
│   │   ├── auth.go                    # parent
│   │   ├── whoami.go
│   │   ├── status.go
│   │   └── logout.go
│   ├── profile/
│   │   ├── add.go                     # T2b: --endpoint-* flags
│   │   └── show.go                    # T2b: print endpoints map
│   ├── api/                           # T12: NEW (raw call)
│   │   ├── api.go
│   │   └── call.go
│   ├── doctor.go                      # T13: extend with endpoint reachability
│   └── root.go                        # T11: register auth + api groups
└── tests/
    └── integration/
        ├── cli_smoke_test.go          # preserved from P0
        └── auth_flow_test.go          # T14: httptest fake account
```

---

## Conventions (apply to every task)

- TDD: failing test first for every new logic.
- Commit after each task with Conventional Commits.
- All user-visible strings route through `internal/i18n`.
- HTTP client tests use `httptest.NewServer` — no real network.
- Schema migration (P0 → P1) must be tested both directions.
- Exit codes (spec §5.4): 3=auth, 5=task_failed_fatal, 6=stream_interrupted, 1=generic.

---

## Task 1: Backend contract document

**Files:**
- Create: `docs/contracts/p1-backend.md`

Pure documentation: freezes the shape the CLI expects so backend can implement in parallel.

- [ ] **Step 1: Write `docs/contracts/p1-backend.md`**

````markdown
# P1 backend contract (CLI expectations)

**Status:** DRAFT — awaiting backend sign-off from go-vibeknow / go-figlens / go-account / go-vectoria owners.
**CLI assumed version when implementing:** these shapes; integration tests mock them.

## 1. `X-Vibeknow-Api-Version` header

Every response from every external service includes:

```
X-Vibeknow-Api-Version: v1
```

- CLI compares to its compile-time constant `httpclient.ClientAPIVersion = "v1"`.
- Major mismatch → CLI returns exit 3 (version_mismatch).
- Missing header → CLI logs warning in `--verbose` only; does NOT fail (graceful degradation for services that haven't adopted yet).

## 2. `/v1/health` endpoint

Every external service exposes:

```
GET /v1/health
```

Response body (200 OK):
```json
{
  "status": "ok",
  "version": "<git-sha-or-semver>",
  "api_version": "v1"
}
```

Used by `vibeknow doctor`. Unauthenticated.

## 3. Error response shape

All non-2xx responses (except 204) have body:

```json
{
  "code": 40401,
  "message": "document not found",
  "data": null,
  "trace_id": "tx_abc123",
  "retryable": false
}
```

- `code`: aether error code (int). HTTP status is derived from the upper digits.
- `message`: human-readable.
- `data`: optional extra context (any JSON).
- `trace_id`: for correlation with backend logs.
- `retryable`: **new field**, default false. true for transient failures (upstream timeout, lock conflict, rate-limited).

CLI maps this into spec §11.2 `ErrorObject`:
- `code: "auth_required"` if HTTP 401
- `code: "auth_expired"` if backend code indicates token expiry
- `code: "not_found"` if HTTP 404
- `code: "permission_denied"` if HTTP 403
- `code: "rate_limited"` if HTTP 429
- `code: "internal_error"` if HTTP 5xx
- `code: "unknown"` otherwise
- `retryable` propagates from backend

## 4. Authentication

- All requests include `Authorization: Bearer <jwt>` header.
- JWT is signed by go-account. Every external service verifies via shared go-atlas JWT secret.
- Missing / invalid / expired: HTTP 401 with `code: 40101` (or similar) and `message` indicating the reason.

## 5. go-account endpoints used by P1

- `GET /v1/user/profile` — CLI's `auth whoami` calls this. Returns `{uid, nickname, email, phone, created_at}`.

(Login endpoints `/v1/auth/email` etc. exist but P1 does NOT use them — P1.5 scope.)

## 6. What P1 does NOT require yet

- Rate limit response headers (`X-RateLimit-*`) — future.
- Task event streams — P2 prereq.
- Device flow endpoints — P1.5 scope.
````

- [ ] **Step 2: Commit**

```bash
git add docs/contracts/
git commit -m "docs(p1): backend contract doc for CLI HTTP foundation"
```

---

## Task 2: Profile schema v2 migration

**Files:**
- Modify: `internal/config/schema.go`
- Modify: `internal/config/schema_test.go`
- Modify: `internal/config/profiles.go` (bump schema_version)

Migrates P0's `APIEndpoint string` to `Endpoints map[string]string`, preserves backward compat.

- [ ] **Step 1: Write failing migration tests** (append to `internal/config/schema_test.go`)

```go
func TestProfileEndpointsRoundtrip(t *testing.T) {
	data := []byte(`
name: prod
endpoints:
  account: https://account.example.com
  vectoria: https://vectoria.example.com
  figlens: https://figlens.example.com
  vibeknow: https://api.example.com
credential_ref: k
trust: user
is_production: true
`)
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Endpoints["figlens"] != "https://figlens.example.com" {
		t.Errorf("figlens missing: %+v", p.Endpoints)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("valid profile rejected: %v", err)
	}
}

func TestProfileLegacyAPIEndpointMapping(t *testing.T) {
	// P0-era profile with only api_endpoint should migrate to endpoints.vibeknow.
	data := []byte(`
name: legacy
api_endpoint: https://api.example.com
credential_ref: k
trust: user
is_production: true
`)
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if got := p.Endpoints["vibeknow"]; got != "https://api.example.com" {
		t.Errorf("legacy api_endpoint not migrated; endpoints=%+v", p.Endpoints)
	}
	if p.APIEndpoint != "https://api.example.com" {
		t.Errorf("APIEndpoint should be preserved for deprecation warning: %q", p.APIEndpoint)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("migrated profile should validate: %v", err)
	}
}

func TestProfileRejectsUnknownEndpointKey(t *testing.T) {
	data := []byte(`
name: bad
endpoints:
  banana: https://oops
credential_ref: k
trust: user
`)
	var p Profile
	_ = yaml.Unmarshal(data, &p)
	// Unknown keys should be dropped (not in the allowed set).
	// Validate should still pass if remaining set is valid (including empty).
	if err := p.Validate(); err != nil {
		t.Errorf("unknown keys dropped; remaining empty map should validate: %v", err)
	}
	if _, ok := p.Endpoints["banana"]; ok {
		t.Errorf("banana endpoint should have been dropped: %+v", p.Endpoints)
	}
}

func TestProfileNonProdEndpointRequiresDevTrust(t *testing.T) {
	data := []byte(`
name: sneaky
endpoints:
  figlens: http://localhost:20067
credential_ref: k
trust: user
is_production: true
`)
	var p Profile
	_ = yaml.Unmarshal(data, &p)
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "trust") {
		t.Errorf("localhost endpoint without trust=dev should fail with trust error, got: %v", err)
	}
}

func TestProfileNonProdEndpointAllowedWithDevTrust(t *testing.T) {
	data := []byte(`
name: dev
endpoints:
  figlens: http://localhost:20067
credential_ref: k
trust: dev
is_production: false
`)
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("dev profile with localhost should validate: %v", err)
	}
}
```

- [ ] **Step 2: Update `internal/config/schema.go`**

Replace the `Profile` struct and its `UnmarshalYAML`/`Validate` with:

```go
package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// allowedEndpointKeys is the set of services the CLI knows about. Others are dropped on load.
var allowedEndpointKeys = map[string]bool{
	"account":  true,
	"vectoria": true,
	"figlens":  true,
	"vibeknow": true,
}

// Profile is the canonical profile shape. See spec §4.3 and §11.3 (v2 schema).
type Profile struct {
	Name             string            `yaml:"name"`
	Endpoints        map[string]string `yaml:"endpoints,omitempty"`
	APIEndpoint      string            `yaml:"api_endpoint,omitempty"` // P0 compat; deprecated
	CredentialRef    string            `yaml:"credential_ref"`
	DefaultProject   string            `yaml:"default_project,omitempty"`
	Trust            string            `yaml:"trust,omitempty"`
	IsProduction     bool              `yaml:"is_production"`
}

// ProfilesFile is the top-level YAML shape (schema_version "2" since P1).
type ProfilesFile struct {
	SchemaVersion string    `yaml:"schema_version"`
	Current       string    `yaml:"current"`
	Profiles      []Profile `yaml:"profiles"`
}

var nameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// UnmarshalYAML normalizes endpoints map (dropping unknown keys), migrates
// legacy api_endpoint, and defaults IsProduction=true.
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
	// Drop unknown endpoint keys.
	p.Endpoints = map[string]string{}
	for k, v := range s.Endpoints {
		if allowedEndpointKeys[k] {
			p.Endpoints[k] = v
		}
	}
	// Legacy api_endpoint → endpoints.vibeknow if not already present.
	p.APIEndpoint = s.APIEndpoint
	if s.APIEndpoint != "" {
		if _, ok := p.Endpoints["vibeknow"]; !ok {
			p.Endpoints["vibeknow"] = s.APIEndpoint
		}
	}
	return nil
}

// Validate enforces schema §11.3 rules.
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

// isNonProdURL returns true for localhost, 127.0.0.1, or hosts not ending in
// a known production suffix. Conservative: unknown hosts are treated as non-prod.
func isNonProdURL(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") {
		return true
	}
	// Permissive default: any *.vibeknow.com is considered prod-capable here.
	// Cloud defaults use this suffix; custom domains must be whitelisted via a
	// later enhancement. For now, *.vibeknow.com and *.vibeknow.ai pass.
	if strings.HasSuffix(host, ".vibeknow.com") || strings.HasSuffix(host, ".vibeknow.ai") {
		return false
	}
	return true
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
```

- [ ] **Step 3: Bump schema version in `internal/config/profiles.go`**

Find the line inside `LoadProfiles`:
```go
if f.SchemaVersion == "" {
    f.SchemaVersion = "1"
}
```
Change the default to `"2"`. Find the same line in `SaveProfiles` and update to `"2"`. Legacy `"1"` files still load (the migration happens at `UnmarshalYAML` per-profile level).

- [ ] **Step 4: Run tests**

```bash
go test -v ./internal/config/
make test && make lint
```

All new tests pass; pre-existing profile tests still pass (verify `TestSaveThenLoadRoundtrip` and `TestAddUseRemove` survive).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): profile schema v2 with endpoints map + P0 api_endpoint compat"
```

---

## Task 2b: Update `cmd/profile` to use endpoints

**Files:**
- Modify: `cmd/profile/add.go`
- Modify: `cmd/profile/show.go`

- [ ] **Step 1: Rewrite `cmd/profile/add.go`**

```go
package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/i18n"
)

var addFlags struct {
	endpointAccount  string
	endpointVectoria string
	endpointFiglens  string
	endpointVibeknow string
	apiEndpoint      string // P0 compat alias for --endpoint-vibeknow
	credentialRef    string
	defaultProject   string
	trust            string
	isProduction     bool
}

var addCmd = &cobra.Command{
	Use:   "add NAME",
	Short: "add a new profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoints := map[string]string{}
		if addFlags.endpointAccount != "" {
			endpoints["account"] = addFlags.endpointAccount
		}
		if addFlags.endpointVectoria != "" {
			endpoints["vectoria"] = addFlags.endpointVectoria
		}
		if addFlags.endpointFiglens != "" {
			endpoints["figlens"] = addFlags.endpointFiglens
		}
		if addFlags.endpointVibeknow != "" {
			endpoints["vibeknow"] = addFlags.endpointVibeknow
		}
		if addFlags.apiEndpoint != "" && endpoints["vibeknow"] == "" {
			endpoints["vibeknow"] = addFlags.apiEndpoint
		}
		p := config.Profile{
			Name:           args[0],
			Endpoints:      endpoints,
			CredentialRef:  addFlags.credentialRef,
			DefaultProject: addFlags.defaultProject,
			Trust:          addFlags.trust,
			IsProduction:   addFlags.isProduction,
		}
		if err := config.AddProfile(p); err != nil {
			return err
		}
		fmt.Println(i18n.T("msg.profile.added", p.Name))
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addFlags.endpointAccount, "endpoint-account", "", "go-account URL override (optional; default uses cloud)")
	addCmd.Flags().StringVar(&addFlags.endpointVectoria, "endpoint-vectoria", "", "go-vectoria URL override")
	addCmd.Flags().StringVar(&addFlags.endpointFiglens, "endpoint-figlens", "", "go-figlens URL override")
	addCmd.Flags().StringVar(&addFlags.endpointVibeknow, "endpoint-vibeknow", "", "go-vibeknow URL override")
	addCmd.Flags().StringVar(&addFlags.apiEndpoint, "api-endpoint", "", "DEPRECATED: alias for --endpoint-vibeknow")
	addCmd.Flags().StringVar(&addFlags.credentialRef, "credential-ref", "", "keychain entry name or file:// path (required)")
	addCmd.Flags().StringVar(&addFlags.defaultProject, "default-project", "", "optional default project name")
	addCmd.Flags().StringVar(&addFlags.trust, "trust", "user", "user|dev")
	addCmd.Flags().BoolVar(&addFlags.isProduction, "is-production", true, "treat as production (must be false to allow non-prod endpoint overrides)")
	_ = addCmd.MarkFlagRequired("credential-ref")
}
```

Note: `--api-endpoint` is no longer required; at least one endpoint flag or cloud defaults will be used. Validation in `config.Profile.Validate` handles missing endpoints via empty map (all services fall through to cloud defaults, resolved in Task 3).

- [ ] **Step 2: Update `cmd/profile/show.go`**

Change the Printf to include endpoints map:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    f, err := config.LoadProfiles()
    if err != nil {
        return err
    }
    name := f.Current
    if len(args) == 1 {
        name = args[0]
    }
    for _, p := range f.Profiles {
        if p.Name == name {
            fmt.Printf("name: %s\ntrust: %s\nis_production: %v\ncredential_ref: %s\ndefault_project: %s\n",
                p.Name, p.Trust, p.IsProduction, p.CredentialRef, p.DefaultProject)
            fmt.Println("endpoints:")
            if len(p.Endpoints) == 0 {
                fmt.Println("  (all using cloud defaults)")
            } else {
                for _, k := range []string{"account", "vectoria", "figlens", "vibeknow"} {
                    if v, ok := p.Endpoints[k]; ok {
                        fmt.Printf("  %s: %s\n", k, v)
                    }
                }
            }
            if p.APIEndpoint != "" {
                fmt.Fprintln(os.Stderr, "warning: api_endpoint is deprecated; use endpoints.vibeknow")
            }
            return nil
        }
    }
    return fmt.Errorf("profile %q not found", name)
},
```

Add `"os"` to imports.

- [ ] **Step 3: Smoke test**

```bash
make build
export VIBEKNOW_CONFIG_HOME=$(mktemp -d)
./vibeknow profile add prod --credential-ref vibeknow.prod --endpoint-vibeknow https://api.vibeknow.com --endpoint-account https://account.vibeknow.com
./vibeknow profile show prod
./vibeknow profile add dev --credential-ref vibeknow.dev --trust dev --is-production=false --endpoint-figlens http://localhost:20067
./vibeknow profile show dev
./vibeknow profile list
rm -rf "$VIBEKNOW_CONFIG_HOME"
unset VIBEKNOW_CONFIG_HOME
```

- [ ] **Step 4: Commit**

```bash
git add cmd/profile/
git commit -m "feat(profile): per-service endpoint flags (--endpoint-account, etc.)"
```

---

## Task 3: `internal/endpoints` — cloud defaults + resolution

**Files:**
- Create: `internal/endpoints/defaults.go`, `internal/endpoints/resolve.go`, `internal/endpoints/resolve_test.go`

- [ ] **Step 1: Write `internal/endpoints/resolve_test.go`**

```go
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
```

- [ ] **Step 2: Write `internal/endpoints/defaults.go`**

```go
// Package endpoints resolves per-service URLs from a profile, with built-in
// cloud defaults. See spec §4.3.
package endpoints

// CloudDefaults lists the built-in production URLs for each service.
// Values are placeholders until ops confirms real domains (spec §10).
var CloudDefaults = map[string]string{
	"account":  "https://account.vibeknow.com",
	"vectoria": "https://vectoria.vibeknow.com",
	"figlens":  "https://figlens.vibeknow.com",
	"vibeknow": "https://api.vibeknow.com",
}
```

- [ ] **Step 3: Write `internal/endpoints/resolve.go`**

```go
package endpoints

import (
	"fmt"

	"github.com/vibeknow/cli/internal/config"
)

// Resolve returns the effective URL for the given service in the profile.
// Profile override wins over cloud default.
func Resolve(p config.Profile, service string) (string, error) {
	if _, ok := CloudDefaults[service]; !ok {
		return "", fmt.Errorf("unknown service %q (expected one of: account, vectoria, figlens, vibeknow)", service)
	}
	if u, ok := p.Endpoints[service]; ok && u != "" {
		return u, nil
	}
	return CloudDefaults[service], nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test -v ./internal/endpoints/
make test && make lint
```

- [ ] **Step 5: Commit**

```bash
git add internal/endpoints/
git commit -m "feat(endpoints): cloud defaults + profile override resolution"
```

---

## Task 4: `internal/errs` + `internal/httpclient` core

**Files:**
- Create: `internal/errs/object.go`, `internal/errs/object_test.go`
- Create: `internal/httpclient/client.go`, `internal/httpclient/errors.go`, `internal/httpclient/client_test.go`

- [ ] **Step 1: Write `internal/errs/object_test.go`**

```go
package errs

import "testing"

func TestObjectImplementsError(t *testing.T) {
	var _ error = (*Object)(nil)
	o := &Object{Code: "not_found", Message: "x"}
	if o.Error() == "" {
		t.Error("Error() empty")
	}
}

func TestIsRetryable(t *testing.T) {
	r := &Object{Code: "rate_limited", Retryable: true}
	if !r.IsRetryable() {
		t.Error("should be retryable")
	}
}
```

- [ ] **Step 2: Write `internal/errs/object.go`**

```go
// Package errs defines the canonical Error Object (spec §11.2).
package errs

import "fmt"

// Object is the canonical CLI-side error. All client / middleware errors
// that reach user surfaces are wrapped in this shape.
type Object struct {
	SchemaVersion string                 `json:"schema_version"`
	Code          string                 `json:"code"`
	Message       string                 `json:"message"`
	Details       map[string]any         `json:"details,omitempty"`
	Retryable     bool                   `json:"retryable"`
	TraceID       string                 `json:"trace_id,omitempty"`
}

func (o *Object) Error() string {
	if o.TraceID != "" {
		return fmt.Sprintf("[%s] %s (trace=%s)", o.Code, o.Message, o.TraceID)
	}
	return fmt.Sprintf("[%s] %s", o.Code, o.Message)
}

// IsRetryable reports whether the caller should retry.
func (o *Object) IsRetryable() bool { return o.Retryable }

// New constructs an Object with schema_version prefilled.
func New(code, message string) *Object {
	return &Object{SchemaVersion: "1", Code: code, Message: message}
}
```

- [ ] **Step 3: Write `internal/httpclient/client_test.go`**

```go
package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	var out map[string]string
	if err := c.Do(context.Background(), "GET", "/ping", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out["hello"] != "world" {
		t.Errorf("got %+v", out)
	}
}

func TestDoBackendErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":      40401,
			"message":   "document not found",
			"trace_id":  "tx_abc",
			"retryable": false,
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Do(context.Background(), "GET", "/docs/x", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if errObj, ok := err.(*errObject); !ok {
		t.Fatalf("want *errObject, got %T", err)
	} else if errObj.Code != "not_found" || errObj.Message != "document not found" || errObj.TraceID != "tx_abc" {
		t.Errorf("unexpected mapping: %+v", errObj)
	}
}

func TestDo5xxMappingRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":      50200,
			"message":   "upstream down",
			"retryable": true,
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Do(context.Background(), "GET", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	eo := err.(*errObject)
	if eo.Code != "internal_error" || !eo.Retryable {
		t.Errorf("5xx with retryable should map correctly: %+v", eo)
	}
}
```

- [ ] **Step 4: Write `internal/httpclient/client.go`**

```go
// Package httpclient provides a shared HTTP client used by all service
// clients in client/*. See spec §4 and §11.2.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClientAPIVersion is the API major version the CLI is compiled for.
// Changes when backend contract breaks compatibility.
const ClientAPIVersion = "v1"

// Client performs HTTP calls against a fixed baseURL with optional middleware.
type Client struct {
	baseURL string
	http    *http.Client
}

// New constructs a Client with default timeouts.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithTransport lets callers override the underlying RoundTripper — used to
// install middleware (token injection, retry, etc.).
func (c *Client) WithTransport(rt http.RoundTripper) *Client {
	nc := *c
	nc.http = &http.Client{Transport: rt, Timeout: c.http.Timeout}
	return &nc
}

// Do performs METHOD on path with optional JSON body, decoding into out.
// out may be nil to skip decoding.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("httpclient: marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("httpclient: new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return &errObject{Code: "network_error", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseBackendError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &errObject{Code: "unknown", Message: "decode response: " + err.Error()}
	}
	return nil
}
```

- [ ] **Step 5: Write `internal/httpclient/errors.go`**

```go
package httpclient

import (
	"encoding/json"
	"net/http"
)

// errObject is the internal error shape; satisfies error. Converted to
// errs.Object by the command layer when displaying to users.
type errObject struct {
	Code      string
	Message   string
	TraceID   string
	Retryable bool
	HTTPCode  int
}

func (e *errObject) Error() string {
	if e.TraceID != "" {
		return "[" + e.Code + "] " + e.Message + " (trace=" + e.TraceID + ")"
	}
	return "[" + e.Code + "] " + e.Message
}

// IsRetryable satisfies a Retryer interface used by mw_retry.
func (e *errObject) IsRetryable() bool { return e.Retryable }

type backendBody struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	TraceID   string          `json:"trace_id,omitempty"`
	Retryable bool            `json:"retryable,omitempty"`
}

// parseBackendError reads the response body and maps it to errObject.
func parseBackendError(resp *http.Response) error {
	var body backendBody
	_ = json.NewDecoder(resp.Body).Decode(&body) // tolerate non-JSON bodies
	return &errObject{
		Code:      mapHTTPCode(resp.StatusCode),
		Message:   body.Message,
		TraceID:   body.TraceID,
		Retryable: body.Retryable || is5xx(resp.StatusCode),
		HTTPCode:  resp.StatusCode,
	}
}

func mapHTTPCode(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "auth_required"
	case status == http.StatusForbidden:
		return "permission_denied"
	case status == http.StatusNotFound:
		return "not_found"
	case status == http.StatusConflict:
		return "conflict"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status >= 500 && status < 600:
		return "internal_error"
	case status >= 400 && status < 500:
		return "invalid_args"
	default:
		return "unknown"
	}
}

func is5xx(status int) bool { return status >= 500 && status < 600 }
```

- [ ] **Step 6: Run tests**

```bash
go test -v ./internal/errs/ ./internal/httpclient/
make test && make lint
```

- [ ] **Step 7: Commit**

```bash
git add internal/errs/ internal/httpclient/
git commit -m "feat(httpclient): core client with backend error mapping + canonical error object"
```

---

## Task 5: Middleware chain scaffold (`transport.go`)

**Files:**
- Create: `internal/httpclient/transport.go`, `internal/httpclient/transport_test.go`

Provides a `RoundTripper`-based middleware chain. Each subsequent task adds one middleware layer using this.

- [ ] **Step 1: Write `internal/httpclient/transport_test.go`**

```go
package httpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type header struct{ key, value string }

func (h header) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		r.Header.Set(h.key, h.value)
		return next.RoundTrip(r)
	})
}

func TestChainAppliesInOrder(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport,
		header{"X-A", "1"},
		header{"X-B", "2"},
	)
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(nil, "GET", "/", nil, nil)
	if got.Get("X-A") != "1" || got.Get("X-B") != "2" {
		t.Errorf("headers not applied: %+v", got)
	}
}
```

Note: `c.Do(nil, ...)` uses a nil context — adjust if `Do` panics on nil; the test could use `context.Background()` instead. Implementer should use `context.Background()` in the test to be safe.

- [ ] **Step 2: Write `internal/httpclient/transport.go`**

```go
package httpclient

import "net/http"

// Middleware wraps a RoundTripper with additional behavior (token inject,
// retry, logging, etc.).
type Middleware interface {
	Wrap(next http.RoundTripper) http.RoundTripper
}

// Chain applies middlewares in order: Chain(base, A, B, C) yields
// A(B(C(base))) semantically — A sees the request first, C sends it last.
func Chain(base http.RoundTripper, mws ...Middleware) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	rt := base
	for i := len(mws) - 1; i >= 0; i-- {
		rt = mws[i].Wrap(rt)
	}
	return rt
}

// roundTripperFunc adapts a function to RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
```

- [ ] **Step 3: Fix test context.Background**

In `transport_test.go`, change `_ = c.Do(nil, "GET", "/", nil, nil)` to `_ = c.Do(context.Background(), "GET", "/", nil, nil)` and import `"context"`.

- [ ] **Step 4: Run tests**

```bash
go test -v ./internal/httpclient/
make test && make lint
```

- [ ] **Step 5: Commit**

```bash
git add internal/httpclient/
git commit -m "feat(httpclient): middleware chain scaffold via RoundTripper"
```

---

## Task 6: Auth + trace-id middleware

**Files:**
- Create: `internal/httpclient/mw_auth.go`, `internal/httpclient/mw_traceid.go`, `internal/httpclient/mw_auth_test.go`

- [ ] **Step 1: Write `internal/httpclient/mw_auth_test.go`**

```go
package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type staticTokenProvider string

func (s staticTokenProvider) Token(ctx context.Context) (string, error) { return string(s), nil }

func TestAuthMiddlewareInjectsBearer(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, AuthMiddleware{Provider: staticTokenProvider("tok_xyz")})
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if got != "Bearer tok_xyz" {
		t.Errorf("Authorization=%q", got)
	}
}

func TestTraceIDMiddlewareInjectsHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Trace-Id")
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, TraceIDMiddleware{})
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if got == "" {
		t.Error("X-Trace-Id missing")
	}
}
```

- [ ] **Step 2: Write `internal/httpclient/mw_auth.go`**

```go
package httpclient

import (
	"context"
	"net/http"
)

// TokenProvider returns a bearer token for the current request.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// AuthMiddleware injects Authorization: Bearer <token>. Empty token skips.
type AuthMiddleware struct{ Provider TokenProvider }

func (m AuthMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if m.Provider != nil {
			tok, err := m.Provider.Token(r.Context())
			if err == nil && tok != "" {
				r.Header.Set("Authorization", "Bearer "+tok)
			}
		}
		return next.RoundTrip(r)
	})
}
```

- [ ] **Step 3: Write `internal/httpclient/mw_traceid.go`**

```go
package httpclient

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
)

// TraceIDMiddleware sets X-Trace-Id. Honors VIBEKNOW_TRACE=1 to surface the
// value in verbose output; otherwise generates silently.
type TraceIDMiddleware struct{}

func (TraceIDMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Trace-Id") == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			tid := "cli-" + hex.EncodeToString(buf)
			r.Header.Set("X-Trace-Id", tid)
			if os.Getenv("VIBEKNOW_TRACE") == "1" {
				r.Header.Set("X-Trace-Id-Display", tid)
			}
		}
		return next.RoundTrip(r)
	})
}
```

- [ ] **Step 4: Run tests**

```bash
go test -v ./internal/httpclient/
```

- [ ] **Step 5: Commit**

```bash
git add internal/httpclient/
git commit -m "feat(httpclient): auth + trace-id middleware"
```

---

## Task 7: Verbose logging middleware (with redact)

**Files:**
- Create: `internal/httpclient/mw_verbose.go`, `internal/httpclient/mw_verbose_test.go`

- [ ] **Step 1: Write `internal/httpclient/mw_verbose_test.go`**

```go
package httpclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerboseLogsSummaryWithRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	chain := Chain(http.DefaultTransport,
		AuthMiddleware{Provider: staticTokenProvider("secret-token-shh")},
		VerboseMiddleware{Out: &buf},
	)
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "POST", "/x", map[string]string{"k": "v"}, nil)

	log := buf.String()
	if !strings.Contains(log, "POST") || !strings.Contains(log, "/x") {
		t.Errorf("log missing method/path: %q", log)
	}
	if strings.Contains(log, "secret-token-shh") {
		t.Errorf("log leaked token: %q", log)
	}
}

func TestVerboseDisabledProducesNoOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	var buf bytes.Buffer
	chain := Chain(http.DefaultTransport, VerboseMiddleware{Out: &buf, Enabled: false})
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/x", nil, nil)
	if buf.Len() != 0 {
		t.Errorf("expected empty log, got %q", buf.String())
	}
}
```

- [ ] **Step 2: Write `internal/httpclient/mw_verbose.go`**

```go
package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vibeknow/cli/internal/redact"
)

// VerboseMiddleware logs request/response summaries to Out.
// Out defaults to os.Stderr. Enabled defaults to true if Out is non-nil.
type VerboseMiddleware struct {
	Out     io.Writer
	Enabled bool
}

func (m VerboseMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	enabled := m.Enabled
	if m.Out != nil && !enabled {
		enabled = true
	}
	if !enabled {
		return next
	}
	out := m.Out
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		start := time.Now()
		resp, err := next.RoundTrip(r)
		dur := time.Since(start)
		line := fmt.Sprintf("%s %s -> ", r.Method, redact.String(r.URL.String()))
		if err != nil {
			line += fmt.Sprintf("err=%s (%s)", redact.String(err.Error()), dur)
		} else {
			line += fmt.Sprintf("%d (%s)", resp.StatusCode, dur)
		}
		fmt.Fprintln(out, line)
		return resp, err
	})
}
```

- [ ] **Step 3: Run tests**

- [ ] **Step 4: Commit**

```bash
git add internal/httpclient/
git commit -m "feat(httpclient): verbose logging middleware with redact"
```

---

## Task 8: Version skew middleware

**Files:**
- Create: `internal/httpclient/mw_version.go`, `internal/httpclient/mw_version_test.go`

- [ ] **Step 1: Write test**

```go
package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVersionMatchOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Vibeknow-Api-Version", "v1")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, VersionMiddleware{Expected: "v1"})
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestVersionMismatchErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Vibeknow-Api-Version", "v2")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, VersionMiddleware{Expected: "v1"})
	c := New(srv.URL).WithTransport(chain)
	err := c.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected version mismatch")
	}
	eo, ok := err.(*errObject)
	if !ok || eo.Code != "version_mismatch" {
		t.Errorf("wrong error: %+v", err)
	}
}

func TestVersionMissingHeaderIsWarningOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200) // no version header
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, VersionMiddleware{Expected: "v1"})
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Errorf("missing header should not fail: %v", err)
	}
}
```

- [ ] **Step 2: Write `internal/httpclient/mw_version.go`**

```go
package httpclient

import "net/http"

// VersionMiddleware verifies the X-Vibeknow-Api-Version response header matches
// Expected (CLI's compile-time version). Missing header is tolerated (warning-only)
// for services that haven't adopted the header yet.
type VersionMiddleware struct {
	Expected string
}

func (m VersionMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := next.RoundTrip(r)
		if err != nil || resp == nil {
			return resp, err
		}
		got := resp.Header.Get("X-Vibeknow-Api-Version")
		if got == "" {
			return resp, nil // graceful: not adopted yet
		}
		if got != m.Expected {
			resp.Body.Close()
			return nil, &errObject{
				Code:      "version_mismatch",
				Message:   "server API version " + got + " incompatible with CLI version " + m.Expected + "; please update the CLI",
				Retryable: false,
			}
		}
		return resp, nil
	})
}
```

- [ ] **Step 3: Run tests**

- [ ] **Step 4: Commit**

```bash
git add internal/httpclient/
git commit -m "feat(httpclient): version skew middleware"
```

---

## Task 9: Retry middleware (5xx + network)

**Files:**
- Create: `internal/httpclient/mw_retry.go`, `internal/httpclient/mw_retry_test.go`

- [ ] **Step 1: Write test**

```go
package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryOn502ThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, RetryMiddleware{
		MaxAttempts: 4,
		BaseDelay:   10 * time.Millisecond,
	})
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatalf("should have succeeded after retries: %v", err)
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("expected 3 hits, got %d", atomic.LoadInt32(&hits))
	}
}

func TestRetryGivesUp(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, RetryMiddleware{
		MaxAttempts: 2,
		BaseDelay:   1 * time.Millisecond,
	})
	c := New(srv.URL).WithTransport(chain)
	err := c.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("should have failed")
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("expected 2 hits, got %d", atomic.LoadInt32(&hits))
	}
}

func TestRetrySkipsNon5xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, RetryMiddleware{MaxAttempts: 5, BaseDelay: 1 * time.Millisecond})
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("404 should not retry, got %d hits", atomic.LoadInt32(&hits))
	}
}
```

- [ ] **Step 2: Write `internal/httpclient/mw_retry.go`**

```go
package httpclient

import (
	"net/http"
	"time"
)

// RetryMiddleware retries 5xx responses (and network errors) with exponential backoff.
// MaxAttempts counts the initial attempt; 3 means "try + 2 retries".
type RetryMiddleware struct {
	MaxAttempts int
	BaseDelay   time.Duration // first backoff; doubles each retry
}

func (m RetryMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		attempts := m.MaxAttempts
		if attempts < 1 {
			attempts = 1
		}
		delay := m.BaseDelay
		if delay <= 0 {
			delay = 100 * time.Millisecond
		}
		var resp *http.Response
		var err error
		for i := 0; i < attempts; i++ {
			resp, err = next.RoundTrip(r)
			if err == nil && !is5xx(resp.StatusCode) {
				return resp, nil
			}
			if resp != nil {
				resp.Body.Close()
			}
			if i == attempts-1 {
				break
			}
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
		return resp, err
	})
}
```

- [ ] **Step 3: Run tests**

- [ ] **Step 4: Commit**

```bash
git add internal/httpclient/
git commit -m "feat(httpclient): retry middleware with exponential backoff for 5xx"
```

---

## Task 10: `client/account` with Whoami

**Files:**
- Create: `client/account/client.go`, `client/account/whoami.go`, `client/account/whoami_test.go`

- [ ] **Step 1: Write test**

```go
package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWhoami(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/profile" {
			http.Error(w, "wrong path", 404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok_xyz" {
			http.Error(w, "no auth", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uid":      "u_123",
			"nickname": "alice",
			"email":    "alice@example.com",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, tokenProviderStub{"tok_xyz"})
	u, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.UID != "u_123" || u.Nickname != "alice" {
		t.Errorf("unexpected user: %+v", u)
	}
}

type tokenProviderStub struct{ tok string }

func (t tokenProviderStub) Token(ctx context.Context) (string, error) { return t.tok, nil }
```

- [ ] **Step 2: Write `client/account/client.go`**

```go
// Package account is the CLI client for go-account. P1 implements only Whoami.
package account

import (
	"net/http"

	"github.com/vibeknow/cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

// New constructs an account client with auth + trace-id + version + retry middleware.
func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	chain := httpclient.Chain(http.DefaultTransport,
		httpclient.AuthMiddleware{Provider: tokenProvider},
		httpclient.TraceIDMiddleware{},
		httpclient.VersionMiddleware{Expected: httpclient.ClientAPIVersion},
	)
	return &Client{http: httpclient.New(baseURL).WithTransport(chain)}
}
```

Note: no retry middleware in the default stack for account — login/whoami should not retry (idempotency is ok but retry-on-5xx isn't needed for these specific calls and adding it complicates test setup). Callers needing retry should wrap themselves.

- [ ] **Step 3: Write `client/account/whoami.go`**

```go
package account

import "context"

// User is the subset of /v1/user/profile we care about.
type User struct {
	UID      string `json:"uid"`
	Nickname string `json:"nickname"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
}

// Whoami calls GET /v1/user/profile.
func (c *Client) Whoami(ctx context.Context) (*User, error) {
	var u User
	if err := c.http.Do(ctx, "GET", "/v1/user/profile", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
```

- [ ] **Step 4: Run tests**

- [ ] **Step 5: Commit**

```bash
git add client/account/
git commit -m "feat(client/account): Whoami over HTTP client stack"
```

---

## Task 11: `cmd/auth` — whoami + status + logout

**Files:**
- Create: `cmd/auth/auth.go`, `cmd/auth/whoami.go`, `cmd/auth/status.go`, `cmd/auth/logout.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Write `cmd/auth/auth.go`**

```go
// Package auth provides the `vibeknow auth` command tree.
// P1 includes whoami / status / logout only; login is P1.5.
package auth

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "auth",
	Short: "manage authentication state",
}

func init() {
	Cmd.AddCommand(whoamiCmd)
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(logoutCmd)
}
```

- [ ] **Step 2: Write `cmd/auth/whoami.go`**

```go
package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/keychain"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "print the current authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, tok, err := resolveProfileAndToken()
		if err != nil {
			return err
		}
		url, err := endpoints.Resolve(p, "account")
		if err != nil {
			return err
		}
		c := account.New(url, staticToken(tok))
		u, err := c.Whoami(context.Background())
		if err != nil {
			return err
		}
		fmt.Printf("uid: %s\nnickname: %s\nemail: %s\nphone: %s\n", u.UID, u.Nickname, u.Email, u.Phone)
		return nil
	},
}

// staticToken implements httpclient.TokenProvider with a fixed value.
type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }

func resolveProfileAndToken() (config.Profile, string, error) {
	f, err := config.LoadProfiles()
	if err != nil {
		return config.Profile{}, "", err
	}
	if f.Current == "" {
		return config.Profile{}, "", fmt.Errorf("no active profile; set one with `vibeknow profile use <name>`")
	}
	var prof *config.Profile
	for i := range f.Profiles {
		if f.Profiles[i].Name == f.Current {
			prof = &f.Profiles[i]
			break
		}
	}
	if prof == nil {
		return config.Profile{}, "", fmt.Errorf("current profile %q not found in profiles list", f.Current)
	}
	// Build a Resolver from the profile's credential_ref.
	r := credential.Resolver{
		Env: credential.EnvSource{Var: "VIBEKNOW_TOKEN"},
	}
	if prof.CredentialRef != "" {
		kc, err := keychain.OpenFor("vibeknow")
		if err == nil {
			r.Keychain = credential.KeychainSource{Keychain: kc, Entry: prof.CredentialRef}
		}
	}
	tok, _, err := r.Resolve()
	if err != nil {
		return *prof, "", fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
	}
	return *prof, tok, nil
}
```

- [ ] **Step 3: Write `cmd/auth/status.go`**

```go
package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/keychain"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "show credential source and active profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		fmt.Printf("active profile: %s\n", orNone(f.Current))
		r := credential.Resolver{Env: credential.EnvSource{Var: "VIBEKNOW_TOKEN"}}
		if f.Current != "" {
			for _, p := range f.Profiles {
				if p.Name == f.Current && p.CredentialRef != "" {
					if kc, err := keychain.OpenFor("vibeknow"); err == nil {
						r.Keychain = credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
					}
					break
				}
			}
		}
		_, src, err := r.Resolve()
		if err != nil {
			fmt.Println("credential: none (set VIBEKNOW_TOKEN or configure credential_ref)")
			return nil
		}
		fmt.Printf("credential source: %s\n", src)
		return nil
	},
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
```

- [ ] **Step 4: Write `cmd/auth/logout.go`**

```go
package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/keychain"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "clear stored credential for the current profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		if f.Current == "" {
			return fmt.Errorf("no active profile")
		}
		var prof *config.Profile
		for i := range f.Profiles {
			if f.Profiles[i].Name == f.Current {
				prof = &f.Profiles[i]
				break
			}
		}
		if prof == nil || prof.CredentialRef == "" {
			return fmt.Errorf("current profile has no credential_ref configured")
		}
		kc, err := keychain.OpenFor("vibeknow")
		if err != nil {
			return err
		}
		if err := kc.Delete(prof.CredentialRef); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: credential was not present in keychain (may have been env-only)")
		} else {
			fmt.Printf("cleared keychain entry %q\n", prof.CredentialRef)
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "note: VIBEKNOW_TOKEN env var (if set) remains — unset it manually to complete logout")
		return nil
	},
}
```

- [ ] **Step 5: Register in `cmd/root.go`**

Add import `authcmd "github.com/vibeknow/cli/cmd/auth"` and inside `init()`:
```go
rootCmd.AddCommand(authcmd.Cmd)
```

- [ ] **Step 6: Smoke test**

```bash
make build
export VIBEKNOW_CONFIG_HOME=$(mktemp -d)
./vibeknow profile add dev --credential-ref vibeknow.dev --trust dev --is-production=false
./vibeknow profile use dev
./vibeknow auth status            # credential: none
./vibeknow auth whoami            # errors with "no credential"
VIBEKNOW_TOKEN=fake ./vibeknow auth status   # credential source: env
./vibeknow auth logout
rm -rf "$VIBEKNOW_CONFIG_HOME"
unset VIBEKNOW_CONFIG_HOME
```

- [ ] **Step 7: Commit**

```bash
git add cmd/ 
git commit -m "feat(auth): whoami/status/logout commands (no interactive login in P1)"
```

---

## Task 12: `cmd/api/call` — raw tunneling

**Files:**
- Create: `cmd/api/api.go`, `cmd/api/call.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Write `cmd/api/api.go`**

```go
// Package api provides the `vibeknow api` subtree for raw backend calls.
package api

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "api",
	Short: "raw backend API access (escape hatch)",
}

func init() {
	Cmd.AddCommand(callCmd)
}
```

- [ ] **Step 2: Write `cmd/api/call.go`**

```go
package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/keychain"
)

var callFlags struct {
	service string
	method  string
	path    string
	body    string
}

var callCmd = &cobra.Command{
	Use:   "call",
	Short: "call a raw backend endpoint",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		if f.Current == "" {
			return fmt.Errorf("no active profile")
		}
		var prof *config.Profile
		for i := range f.Profiles {
			if f.Profiles[i].Name == f.Current {
				prof = &f.Profiles[i]
				break
			}
		}
		if prof == nil {
			return fmt.Errorf("current profile %q not found", f.Current)
		}
		url, err := endpoints.Resolve(*prof, callFlags.service)
		if err != nil {
			return err
		}

		// Token
		r := credential.Resolver{Env: credential.EnvSource{Var: "VIBEKNOW_TOKEN"}}
		if prof.CredentialRef != "" {
			if kc, err := keychain.OpenFor("vibeknow"); err == nil {
				r.Keychain = credential.KeychainSource{Keychain: kc, Entry: prof.CredentialRef}
			}
		}
		tok, _, _ := r.Resolve() // empty tok is fine for unauth endpoints

		// Read body
		var bodyReader *bytes.Reader
		if callFlags.body != "" {
			data, err := readBody(callFlags.body)
			if err != nil {
				return err
			}
			bodyReader = bytes.NewReader(data)
		}

		// Build & send
		fullURL := strings.TrimRight(url, "/") + callFlags.path
		req, err := http.NewRequestWithContext(context.Background(), callFlags.method, fullURL, nil)
		if err != nil {
			return err
		}
		if bodyReader != nil {
			req.Body = io.NopCloser(bodyReader)
			req.Header.Set("Content-Type", "application/json")
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		chain := httpclient.Chain(http.DefaultTransport,
			httpclient.TraceIDMiddleware{},
			httpclient.VersionMiddleware{Expected: httpclient.ClientAPIVersion},
		)
		resp, err := chain.RoundTrip(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(os.Stdout, resp.Body)
		if resp.StatusCode >= 400 {
			return fmt.Errorf("\nHTTP %d", resp.StatusCode)
		}
		return nil
	},
}

func init() {
	callCmd.Flags().StringVar(&callFlags.service, "service", "", "target service: account|vectoria|figlens|vibeknow (required)")
	callCmd.Flags().StringVar(&callFlags.method, "method", "GET", "HTTP method")
	callCmd.Flags().StringVar(&callFlags.path, "path", "", "URL path including leading slash (required)")
	callCmd.Flags().StringVar(&callFlags.body, "body", "", "JSON body (literal) or @file to read from file")
	_ = callCmd.MarkFlagRequired("service")
	_ = callCmd.MarkFlagRequired("path")
}

func readBody(spec string) ([]byte, error) {
	if strings.HasPrefix(spec, "@") {
		path := strings.TrimPrefix(spec, "@")
		return os.ReadFile(path)
	}
	return []byte(spec), nil
}
```

- [ ] **Step 3: Register in `cmd/root.go`**

Add import `apicmd "github.com/vibeknow/cli/cmd/api"` and inside `init()`:
```go
rootCmd.AddCommand(apicmd.Cmd)
```

- [ ] **Step 4: Smoke test**

```bash
make build
export VIBEKNOW_CONFIG_HOME=$(mktemp -d)
./vibeknow profile add dev --credential-ref vibeknow.dev --trust dev --is-production=false --endpoint-account http://127.0.0.1:1
./vibeknow profile use dev
./vibeknow api call --service account --path /v1/health 2>&1 || true
./vibeknow api call --service banana --path /x 2>&1 || true   # unknown service errors
rm -rf "$VIBEKNOW_CONFIG_HOME"
unset VIBEKNOW_CONFIG_HOME
```

Expected: first call fails with network error (localhost:1 nothing listening — fine, proves routing); second fails with "unknown service".

- [ ] **Step 5: Commit**

```bash
git add cmd/
git commit -m "feat(api): raw call with --service routing via endpoints map"
```

---

## Task 13: `cmd/doctor` — endpoint reachability

**Files:**
- Modify: `cmd/doctor.go`

Extends P0's local-only doctor with concurrent reachability checks against each resolved endpoint's `/v1/health`.

- [ ] **Step 1: Modify `cmd/doctor.go` to add endpoint checks**

```go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/httpclient"
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

		// Endpoint reachability (concurrent).
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
	tmp, err := os.MkdirTemp("", "vibeknow-doctor-probe-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	_, err = keychain.OpenFor("vibeknow-doctor-probe",
		keychain.WithFileBackend(tmp, "probe-passphrase"))
	return err
}

func checkLocale() error {
	if got := i18n.T("doctor.header"); got == "" {
		return fmt.Errorf("i18n returned empty string")
	}
	return nil
}

// checkEndpoints resolves each service endpoint and probes /v1/health
// concurrently. Returns the number of failures.
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
				results[i] = result{svc: svc, ok: false, detail: err.Error()}
				return
			}
			results[i] = result{svc: svc, url: url}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", url+"/v1/health", nil)
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				results[i].detail = err.Error()
				return
			}
			defer resp.Body.Close()
			gotVer := resp.Header.Get("X-Vibeknow-Api-Version")
			var body struct {
				Status     string `json:"status"`
				Version    string `json:"version"`
				APIVersion string `json:"api_version"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if resp.StatusCode != 200 || body.Status != "ok" {
				results[i].detail = fmt.Sprintf("http=%d status=%q", resp.StatusCode, body.Status)
				return
			}
			results[i].ok = true
			results[i].detail = fmt.Sprintf("api=%s build=%s", gotVer, body.Version)
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
```

- [ ] **Step 2: Smoke test**

```bash
make build
export VIBEKNOW_CONFIG_HOME=$(mktemp -d)
./vibeknow profile add dev --credential-ref vibeknow.dev
./vibeknow profile use dev
./vibeknow doctor 2>&1 || true   # endpoints will fail in dev without real servers; that's expected
rm -rf "$VIBEKNOW_CONFIG_HOME"
unset VIBEKNOW_CONFIG_HOME
```

- [ ] **Step 3: Commit**

```bash
git add cmd/
git commit -m "feat(doctor): concurrent endpoint reachability + version probe"
```

---

## Task 14: Integration test — auth flow with fake account

**Files:**
- Create: `tests/integration/auth_flow_test.go`

- [ ] **Step 1: Write the test**

```go
package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAuthWhoamiAgainstFakeAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/profile" {
			http.Error(w, "not found", 404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer e2e-token" {
			http.Error(w, "forbidden", 401)
			return
		}
		w.Header().Set("X-Vibeknow-Api-Version", "v1")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uid":      "u_e2e",
			"nickname": "e2eUser",
			"email":    "e2e@example.com",
		})
	}))
	defer srv.Close()

	bin := build(t)
	home := t.TempDir()

	// Create dev profile with account endpoint pointing at fake server.
	_, _, code := run(t, bin, home,
		"profile", "add", "dev",
		"--credential-ref", "vibeknow.dev",
		"--trust", "dev",
		"--is-production=false",
		"--endpoint-account", srv.URL,
	)
	if code != 0 {
		t.Fatal("profile add")
	}
	_, _, _ = run(t, bin, home, "profile", "use", "dev")

	// Call whoami with VIBEKNOW_TOKEN set.
	cmd := exec_command(bin, "auth", "whoami")
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_CONFIG_HOME="+home,
		"VIBEKNOW_TOKEN=e2e-token",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("whoami: err=%v out=%s", err, string(out))
	}
	s := string(out)
	if !strings.Contains(s, "u_e2e") || !strings.Contains(s, "e2eUser") {
		t.Errorf("whoami output missing user info: %q", s)
	}
}
```

Add `exec_command` to `cli_smoke_test.go` if not present:

```go
import "os/exec"

func exec_command(bin string, args ...string) *exec.Cmd {
	return exec.Command(bin, args...)
}
```

(Alternative: just use `exec.Command` directly in the test.)

- [ ] **Step 2: Run locally**

```bash
go test -count=1 ./tests/integration/...
```

- [ ] **Step 3: Commit**

```bash
git add tests/integration/
git commit -m "test(p1): auth whoami integration against httptest fake account"
```

---

## Task 15: Release — CHANGELOG + tag v0.2.0-p1

- [ ] **Step 1: Full test sweep**

```bash
make lint && make test
go test -count=1 ./tests/integration/...
```

All must pass.

- [ ] **Step 2: Update `CHANGELOG.md`**

Add a new section above the existing `[0.1.0-p0]` entry:

```markdown
## [0.2.0-p1] — 2026-04-15
### Added
- Multi-endpoint direct-connect: profile schema v2 with `endpoints` map for account/vectoria/figlens/vibeknow. Cloud defaults built in.
- `internal/httpclient` stack: core client + middleware chain (auth / trace-id / version skew / verbose+redact / retry).
- `internal/errs` canonical Error Object (spec §11.2).
- `client/account` with `Whoami`.
- `cmd/auth whoami / status / logout` (no interactive login in P1; use `VIBEKNOW_TOKEN` env or P1.5's Device Flow).
- `cmd/api call --service <name> --method ... --path ...` raw tunneling.
- `cmd/doctor` extended with concurrent endpoint reachability + API version probe.
- Backend contract document at `docs/contracts/p1-backend.md`.

### Changed
- `profile add` accepts `--endpoint-{account,vectoria,figlens,vibeknow}`; `--api-endpoint` retained as deprecated alias for `--endpoint-vibeknow`.
- `profile show` prints endpoints map instead of single `api_endpoint`.
- Profile schema_version bumped from "1" to "2"; v1 profiles auto-migrate on load.

### Deferred
- Interactive `auth login` (Device Flow + PAT) → P1.5 standalone project.
- Service clients for vectoria / figlens / vibeknow / speech → P2 (alongside shortcuts).
```

Keep the earlier `[0.1.0-p0]` entry unchanged.

- [ ] **Step 3: Commit CHANGELOG**

```bash
git add CHANGELOG.md
git commit -m "chore(release): cut v0.2.0-p1"
```

- [ ] **Step 4: Tag**

```bash
git tag -a v0.2.0-p1 -m "P1: auth & HTTP foundation"
git tag
git log --oneline -5
```

- [ ] **Step 5: Verify tagged binary**

```bash
make build
./vibeknow version   # prints v0.2.0-p1
./vibeknow auth status
./vibeknow --help | head -20
```

---

## Self-review notes (after writing)

- **Spec coverage**: §4.1 multi-endpoint architecture (T2/T3/T11/T12), §4.2 auth (T10/T11; login deferred as documented), §4.3 profile schema (T2/T2b), §5.4 exit codes (T4 errs + T8 version mismatch), §8.4 verbose (T7), §8.5 redact (T7 reused), §8.8 version skew (T8), §11.2 error object (T4), §11.3 profile YAML (T2). §5 task streaming, §6 hero command, §7 Skills — all correctly scoped to P2.
- **Type consistency**: `httpclient.TokenProvider` / `httpclient.Middleware` used consistently. `errs.Object` is the user-surface type; `httpclient.errObject` is the internal type that satisfies `error` but isn't exposed.
- **Placeholders**: None. `<org>` already resolved in P0; cloud default URLs are placeholders but flagged as such in §10 and in `endpoints/defaults.go` comment.

---

**Plan complete and saved.** Ready for execution handoff.

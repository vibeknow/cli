# P0: Bootstrap & Base Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a buildable, `npm install`-able vibeknow CLI whose base commands (`profile`, `config`, `doctor`, `completion`, `version`) work end-to-end with no backend dependency — establishing the framework (output / i18n / charcheck / credential / config) that P1+ will layer onto.

**Architecture:** Cobra-based Go CLI borrowing lark-cli's internal infrastructure patterns (not verbatim code; patterns). Packaged as a Go binary wrapped by an npm package for cross-platform distribution. All framework packages are TDD'd with table-driven tests before command wiring.

**Tech Stack:** Go 1.23+, `github.com/spf13/cobra`, `github.com/spf13/viper`, `github.com/99designs/keyring` (keychain), `gopkg.in/yaml.v3`, `github.com/gofrs/flock` (lockfile). Reference spec: `docs/superpowers/specs/2026-04-15-vibeknow-cli-design.md` v2.2.

**Scope boundary (what P0 does NOT include):**
- `auth login/logout/whoami` (needs go-account client — P1).
- `api call` / any backend-calling command (P1).
- `create` / `video *` / `doc *` / `rag *` / `voice *` / `project *` shortcuts (P2).
- Skills, final release pipeline, cross-platform CI matrix polish (P3).
- `update` command implementation (stub only in P0; real logic in P3).

---

## File Structure (to be created across tasks)

```
vibeknow-cli/
├── main.go                            # cobra Execute()
├── go.mod / go.sum
├── package.json                       # npm wrapper
├── Makefile / build.sh
├── LICENSE (MIT)
├── README.md / README.zh.md           # P0 stub
├── AGENTS.md                          # repo-level agent guidance
├── CHANGELOG.md                       # seed
├── .gitignore
├── .github/workflows/ci.yml           # lint + test + build matrix
├── cmd/
│   ├── root.go                        # root cobra + global flags
│   ├── version.go                     # `vibeknow version`
│   ├── completion.go                  # shell completion
│   ├── doctor.go                      # local-only checks
│   ├── update.go                      # stub returning "not implemented in P0"
│   ├── config/
│   │   ├── config.go                  # `config` parent
│   │   ├── get.go / set.go / list.go
│   ├── profile/
│   │   ├── profile.go                 # `profile` parent
│   │   ├── add.go / list.go / use.go / remove.go / show.go
├── internal/
│   ├── charcheck/                     # strip C0/C1/ANSI
│   ├── redact/                        # mask tokens in logs
│   ├── i18n/                          # LANG-aware string table
│   ├── output/                        # text/json/ndjson writers
│   ├── lockfile/                      # thin wrapper over gofrs/flock
│   ├── keychain/                      # OS keychain abstraction
│   ├── credential/                    # env > keychain > file resolver
│   ├── config/                        # profiles.yaml r/w
│   ├── errs/                          # §11.2 Error object helpers
│   └── cmdutil/                       # shared cobra helpers
├── scripts/
│   └── npm-postinstall.js             # platform binary selector
└── tests/
    └── integration/
        └── cli_smoke_test.go          # spawns built binary
```

Each `internal/*` package is one focused responsibility; boundaries are defined below. Files that change together live together (e.g., profile CRUD commands share a directory).

---

## Conventions (apply to every task)

- **Tests first.** Every `internal/*` package starts with a failing test.
- **Commit after each task** with a Conventional Commits subject line.
- **No network calls in P0 tests.** `doctor` tests use `httptest` only for the *locale* of config validation; no real endpoints.
- **Exit codes** follow §5.4 spec even in P0 scope (unused codes reserved but not emitted).
- **All user-visible strings** go through `internal/i18n` from day one — no hardcoded English in command output.

---

## Task 1: Repository bootstrap

**Files:**
- Create: `go.mod`, `main.go`, `.gitignore`, `LICENSE`, `README.md`, `README.zh.md`, `AGENTS.md`, `CHANGELOG.md`, `cmd/root.go`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
cd ~/laoshen/vibeknow-cli
go mod init github.com/vibeknow/cli
```
(Org name confirmed: `vibeknow`.)

- [ ] **Step 2: Write `.gitignore`**

```
vibeknow
/dist/
/build/
*.test
*.out
.DS_Store
/node_modules/
/.vibeknow-test-home/
```

- [ ] **Step 3: Write `LICENSE` (MIT)**

Copy MIT license text verbatim with `Copyright (c) 2026 <org>`.

- [ ] **Step 4: Write minimal `main.go`**

```go
package main

import (
	"os"

	"github.com/vibeknow/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Write `cmd/root.go` with empty root command**

```go
package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "vibeknow",
	Short: "vibeknow CLI — turn docs into videos",
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}
```

- [ ] **Step 6: Add cobra dependency and verify build**

Run:
```bash
go get github.com/spf13/cobra@latest
go build -o vibeknow .
./vibeknow --help
```
Expected: help text printed, exit 0.

- [ ] **Step 7: Write README stub** (`README.md`)

```markdown
# vibeknow-cli

The official vibeknow CLI — turn docs and links into videos.

> **Status:** Pre-alpha (P0 bootstrap). Not for production use.

See `docs/superpowers/specs/2026-04-15-vibeknow-cli-design.md` for the full design.
```

Plus a one-paragraph `README.zh.md` mirror.

- [ ] **Step 8: Write `AGENTS.md` seed**

```markdown
# Agent guidance — vibeknow-cli repo

This repo is a Cobra-based Go CLI with an npm distribution wrapper.

## Where to look
- `cmd/` — Cobra command definitions, one file per command.
- `internal/` — Reusable framework packages (output, i18n, credential, config).
- `docs/superpowers/specs/` — Design documents; `docs/superpowers/plans/` — Implementation plans.
- `skills/` — AI agent Skills (empty in P0).

## Conventions
- All user-visible strings go through `internal/i18n`.
- Tests live alongside source (`*_test.go`) plus `tests/integration/`.
- `--output` values: `text` (default in TTY), `json`, `ndjson`.
- Exit codes: see spec §5.4.

## Before making changes
- Run `make test` and `make build`.
- Never commit credentials, cache files (`~/.cache/vibeknow/*`), or `dist/`.
```

- [ ] **Step 9: Write `CHANGELOG.md` seed**

```markdown
# Changelog

## [Unreleased]
### Added
- P0 bootstrap: repo scaffold, base framework packages, `profile` / `config` / `doctor` / `completion` / `version` commands.
```

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat(p0): repository bootstrap with empty root command"
```

---

## Task 2: Build toolchain (Makefile, build.sh, CI skeleton)

**Files:**
- Create: `Makefile`, `build.sh`, `.github/workflows/ci.yml`

- [ ] **Step 1: Write `Makefile`**

```makefile
BINARY := vibeknow
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/vibeknow/cli/cmd.version=$(VERSION)

.PHONY: build test lint install clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test -race -count=1 ./...

lint:
	go vet ./...

install: build
	install -m 0755 $(BINARY) $(GOPATH)/bin/$(BINARY) || install -m 0755 $(BINARY) $(HOME)/go/bin/$(BINARY)

clean:
	rm -f $(BINARY) $(BINARY)-* $(BINARY).exe
	rm -rf dist/
```

- [ ] **Step 2: Write `build.sh` for cross-platform binaries**

```bash
#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-X github.com/vibeknow/cli/cmd.version=${VERSION}"
DIST="${DIST:-./dist}"
mkdir -p "$DIST"

platforms=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

for platform in "${platforms[@]}"; do
  IFS='/' read -r OS ARCH <<<"$platform"
  out="$DIST/vibeknow-${OS}-${ARCH}"
  [[ "$OS" == "windows" ]] && out="${out}.exe"
  echo "Building $out"
  GOOS="$OS" GOARCH="$ARCH" CGO_ENABLED=0 \
    go build -ldflags "$LDFLAGS" -o "$out" .
done
```
```bash
chmod +x build.sh
```

- [ ] **Step 3: Write `.github/workflows/ci.yml`**

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go vet ./...
      - run: go test -race -count=1 ./...
      - run: go build -o vibeknow .
```

- [ ] **Step 4: Verify locally**

```bash
make lint && make test && make build
./vibeknow version 2>&1 || true   # version cmd not wired yet; OK
```

- [ ] **Step 5: Commit**

```bash
git add Makefile build.sh .github/
git commit -m "build(p0): add Makefile, cross-platform build.sh, and CI workflow"
```

---

## Task 3: `internal/charcheck` — strip control chars (TDD)

**Files:**
- Create: `internal/charcheck/charcheck.go`, `internal/charcheck/charcheck_test.go`

**Responsibility:** sanitize arbitrary bytes before they hit stdout/stderr (§8.5). Strip ANSI CSI (ESC `[` …), C0 (0x00–0x1F except `\t` `\n`), and C1 (0x80–0x9F) characters. Preserve valid UTF-8.

- [ ] **Step 1: Write failing test `charcheck_test.go`**

```go
package charcheck

import "testing"

func TestStrip(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain ascii", "hello", "hello"},
		{"preserve tab/newline", "a\tb\nc", "a\tb\nc"},
		{"strip bare ESC", "a\x1bb", "ab"},
		{"strip CSI color", "\x1b[31mred\x1b[0m", "red"},
		{"strip carriage return overwrite", "progress\r100%", "progress100%"},
		{"strip C1", "a\x9bb", "ab"},
		{"preserve utf8", "你好\n世界", "你好\n世界"},
		{"strip OSC", "\x1b]0;title\x07rest", "rest"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Strip(c.in)
			if got != c.want {
				t.Fatalf("Strip(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test (should fail: package not defined)**

```bash
go test ./internal/charcheck/
```
Expected: compile error — no `Strip` function.

- [ ] **Step 3: Implement `charcheck.go`**

```go
// Package charcheck strips control characters and escape sequences from
// untrusted text before it is written to a terminal. See spec §8.5.
package charcheck

import (
	"strings"
	"unicode/utf8"
)

// Strip removes C0 (except \t \n), C1, and ANSI escape sequences (CSI + OSC)
// from s. Valid UTF-8 above the control ranges is preserved.
func Strip(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == 0x1b && i+1 < len(s) && s[i+1] == '[':
			// CSI: ESC [ ... final byte in 0x40..0x7E
			j := i + 2
			for j < len(s) {
				b := s[j]
				j++
				if b >= 0x40 && b <= 0x7e {
					break
				}
			}
			i = j
			continue
		case r == 0x1b && i+1 < len(s) && s[i+1] == ']':
			// OSC: ESC ] ... BEL or ST
			j := i + 2
			for j < len(s) && s[j] != 0x07 {
				j++
			}
			if j < len(s) {
				j++ // consume BEL
			}
			i = j
			continue
		case r == '\t' || r == '\n':
			b.WriteByte(byte(r))
		case r < 0x20 || (r >= 0x7f && r <= 0x9f):
			// drop
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests — should pass**

```bash
go test -v ./internal/charcheck/
```
Expected: all test cases pass.

- [ ] **Step 5: Commit**

```bash
git add internal/charcheck/
git commit -m "feat(charcheck): strip C0/C1 and ANSI escape sequences"
```

---

## Task 4: `internal/redact` — mask sensitive strings in logs (TDD)

**Files:**
- Create: `internal/redact/redact.go`, `internal/redact/redact_test.go`

**Responsibility:** given a string (log line or HTTP header dump), replace anything that looks like a bearer token, cookie value, or basic-auth credential with `***`.

- [ ] **Step 1: Write failing test**

```go
package redact

import "testing"

func TestRedact(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Authorization: Bearer abcdef1234567890", "Authorization: Bearer ***"},
		{"authorization: bearer  xyz", "authorization: bearer ***"},
		{"Cookie: session=abc123def; user=bob", "Cookie: session=***; user=bob"},
		{"X-Api-Key: sk_live_abcDEF123", "X-Api-Key: ***"},
		{"unrelated text", "unrelated text"},
		{"Basic dXNlcjpwYXNz", "Basic ***"},
	}
	for _, c := range cases {
		got := String(c.in)
		if got != c.want {
			t.Errorf("String(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test — should fail**

```bash
go test ./internal/redact/
```

- [ ] **Step 3: Implement `redact.go`**

```go
// Package redact masks sensitive values in strings before logging.
// See spec §8.5.
package redact

import "regexp"

var (
	bearerRe = regexp.MustCompile(`(?i)(authorization:\s*bearer)\s+\S+`)
	basicRe  = regexp.MustCompile(`(?i)(basic)\s+[A-Za-z0-9+/=]+`)
	apiKeyRe = regexp.MustCompile(`(?i)(x-[a-z-]*api[a-z-]*key|x-auth-token):\s*\S+`)
	sessionRe = regexp.MustCompile(`(?i)(session|sid|token)=[^;,\s]+`)
)

// String returns s with common credential patterns replaced by "***".
func String(s string) string {
	s = bearerRe.ReplaceAllString(s, "$1 ***")
	s = basicRe.ReplaceAllString(s, "$1 ***")
	s = apiKeyRe.ReplaceAllString(s, "$1: ***")
	s = sessionRe.ReplaceAllString(s, "$1=***")
	return s
}
```

- [ ] **Step 4: Run tests**

```bash
go test -v ./internal/redact/
```
Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add internal/redact/
git commit -m "feat(redact): mask bearer/basic/api-key/session values"
```

---

## Task 5: `internal/i18n` — LANG-aware string table (TDD)

**Files:**
- Create: `internal/i18n/i18n.go`, `internal/i18n/strings.go`, `internal/i18n/i18n_test.go`

**Responsibility:** §8.9. Provide `T("msg.key", args...)` returning the localized template formatted with `args`. Language selection: `VIBEKNOW_LANG` overrides `LANG`; unknown locale falls back to `en`. Unknown key returns the key itself with a stderr warning (in debug only).

- [ ] **Step 1: Write failing test**

```go
package i18n

import (
	"os"
	"testing"
)

func TestSelectLocale(t *testing.T) {
	cases := []struct {
		vibe, lang, want string
	}{
		{"", "", "en"},
		{"", "zh_CN.UTF-8", "zh"},
		{"", "en_US.UTF-8", "en"},
		{"zh", "en_US.UTF-8", "zh"},
		{"fr_FR", "zh_CN", "en"}, // unknown falls back
	}
	for _, c := range cases {
		os.Setenv("VIBEKNOW_LANG", c.vibe)
		os.Setenv("LANG", c.lang)
		if got := selectLocale(); got != c.want {
			t.Errorf("VIBEKNOW_LANG=%q LANG=%q -> %q, want %q", c.vibe, c.lang, got, c.want)
		}
	}
	os.Unsetenv("VIBEKNOW_LANG")
	os.Unsetenv("LANG")
}

func TestT(t *testing.T) {
	Register("en", map[string]string{"hello": "Hello, %s!"})
	Register("zh", map[string]string{"hello": "你好，%s！"})

	SetLocale("en")
	if got := T("hello", "world"); got != "Hello, world!" {
		t.Errorf("en: %q", got)
	}
	SetLocale("zh")
	if got := T("hello", "world"); got != "你好，world！" {
		t.Errorf("zh: %q", got)
	}
	if got := T("missing.key"); got != "missing.key" {
		t.Errorf("missing key: %q", got)
	}
}
```

- [ ] **Step 2: Run — fails**

- [ ] **Step 3: Implement `i18n.go`**

```go
// Package i18n provides a minimal key-based string table with locale
// selection via VIBEKNOW_LANG / LANG env vars. See spec §8.9.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	mu      sync.RWMutex
	tables  = map[string]map[string]string{}
	current = "en"
)

// Register merges entries for a locale (call once per locale at init).
func Register(locale string, entries map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	if tables[locale] == nil {
		tables[locale] = map[string]string{}
	}
	for k, v := range entries {
		tables[locale][k] = v
	}
}

// SetLocale forces the active locale (used by root cmd after flag parsing).
func SetLocale(l string) {
	mu.Lock()
	current = l
	mu.Unlock()
}

// Init reads env vars and picks a locale.
func Init() { SetLocale(selectLocale()) }

func selectLocale() string {
	candidates := []string{os.Getenv("VIBEKNOW_LANG"), os.Getenv("LANG")}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		lc := strings.ToLower(strings.SplitN(c, ".", 2)[0])
		lc = strings.ReplaceAll(lc, "-", "_")
		if strings.HasPrefix(lc, "zh") {
			return "zh"
		}
		if strings.HasPrefix(lc, "en") {
			return "en"
		}
	}
	return "en"
}

// T returns the localized string for key formatted with args, falling back
// to English, then to the key itself if unknown.
func T(key string, args ...any) string {
	mu.RLock()
	defer mu.RUnlock()
	if tpl, ok := tables[current][key]; ok {
		return fmt.Sprintf(tpl, args...)
	}
	if tpl, ok := tables["en"][key]; ok {
		return fmt.Sprintf(tpl, args...)
	}
	return key
}
```

- [ ] **Step 4: Implement `strings.go` with seed entries**

```go
package i18n

func init() {
	Register("en", map[string]string{
		"err.profile.not_found":    "profile %q not found",
		"err.profile.duplicate":    "profile %q already exists",
		"err.config.invalid":       "config invalid: %s",
		"msg.profile.switched":     "active profile is now %q",
		"msg.profile.added":        "added profile %q",
		"msg.profile.removed":      "removed profile %q",
		"doctor.header":            "vibeknow doctor — local checks only (P0)",
		"doctor.ok":                "[ok] %s",
		"doctor.fail":              "[fail] %s: %s",
	})
	Register("zh", map[string]string{
		"err.profile.not_found":    "profile %q 不存在",
		"err.profile.duplicate":    "profile %q 已存在",
		"err.config.invalid":       "配置无效：%s",
		"msg.profile.switched":     "当前 profile 已切换为 %q",
		"msg.profile.added":        "已添加 profile %q",
		"msg.profile.removed":      "已删除 profile %q",
		"doctor.header":            "vibeknow doctor — 仅本地检查（P0）",
		"doctor.ok":                "[通过] %s",
		"doctor.fail":              "[失败] %s: %s",
	})
}
```

- [ ] **Step 5: Run tests — pass**

```bash
go test -v ./internal/i18n/
```

- [ ] **Step 6: Commit**

```bash
git add internal/i18n/
git commit -m "feat(i18n): key-based string table with en/zh and env-driven selection"
```

---

## Task 6: `internal/output` — format-aware writers (TDD)

**Files:**
- Create: `internal/output/output.go`, `internal/output/text.go`, `internal/output/json.go`, `internal/output/ndjson.go`, `internal/output/output_test.go`

**Responsibility:** §8.10. One `Writer` interface with three implementations. Every write passes through `charcheck.Strip` for text mode (JSON/NDJSON handle escaping intrinsically).

- [ ] **Step 1: Write failing test**

```go
package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestSelectFormat(t *testing.T) {
	cases := []struct {
		flag, expected string
		isTTY          bool
		streaming      bool
	}{
		{"", "text", true, false},
		{"", "json", false, false},
		{"", "ndjson", false, true},
		{"json", "json", true, true},
		{"ndjson", "ndjson", false, false},
		{"text", "text", false, false},
	}
	for _, c := range cases {
		got := Select(c.flag, c.isTTY, c.streaming)
		if got != c.expected {
			t.Errorf("Select(%q, tty=%v, stream=%v) = %q, want %q",
				c.flag, c.isTTY, c.streaming, got, c.expected)
		}
	}
}

func TestTextStripsControl(t *testing.T) {
	var buf bytes.Buffer
	w := NewText(&buf)
	w.Print("clean ", "\x1b[31mred\x1b[0m", " tail")
	if got := buf.String(); got != "clean red tail" {
		t.Errorf("got %q", got)
	}
}

func TestJSONObject(t *testing.T) {
	var buf bytes.Buffer
	w := NewJSON(&buf)
	w.Object(map[string]any{"k": "v", "n": 1})
	got := buf.String()
	if !strings.Contains(got, `"k":"v"`) || !strings.Contains(got, `"n":1`) || !strings.Contains(got, `"schema_version":"1"`) {
		t.Errorf("got %q", got)
	}
}

func TestNDJSONEvent(t *testing.T) {
	var buf bytes.Buffer
	w := NewNDJSON(&buf)
	w.Event(map[string]any{"event": "x", "task_id": "t_1"})
	w.Event(map[string]any{"event": "y", "task_id": "t_1"})
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), buf.String())
	}
	for _, l := range lines {
		if !strings.Contains(l, `"schema_version":"1"`) || !strings.Contains(l, `"ts":`) {
			t.Errorf("line missing canonical fields: %q", l)
		}
	}
}

func TestUnsupportedFormat(t *testing.T) {
	if _, err := New("yaml", nil, false, false); err == nil {
		t.Error("expected error for unsupported format")
	}
}
```

- [ ] **Step 2: Run — fails**

- [ ] **Step 3: Implement `output.go`**

```go
// Package output provides format-aware writers for CLI command results.
// Supported formats per spec §8.10: text, json, ndjson.
package output

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type Writer interface {
	Format() string
}

// Select resolves a user-supplied --output flag value with TTY and streaming
// context into a concrete format. See §8.10.
func Select(flag string, isTTY, streaming bool) string {
	if flag != "" {
		return flag
	}
	if isTTY {
		return "text"
	}
	if streaming {
		return "ndjson"
	}
	return "json"
}

// New creates a writer for the given format. Returns an error for
// unsupported formats so the CLI can exit 2 with a clear message.
func New(format string, w io.Writer, isTTY, streaming bool) (Writer, error) {
	switch strings.ToLower(format) {
	case "text":
		return NewText(w), nil
	case "json":
		return NewJSON(w), nil
	case "ndjson":
		return NewNDJSON(w), nil
	case "yaml", "table":
		return nil, fmt.Errorf("output format %q is not implemented yet; supported: text, json, ndjson", format)
	default:
		return nil, fmt.Errorf("unknown output format %q; supported: text, json, ndjson", format)
	}
}

var errSchemaVersion = errors.New("internal: schema version must be set")

// schemaVersion is stamped on every structured output per spec §11.
const schemaVersion = "1"
```

- [ ] **Step 4: Implement `text.go`**

```go
package output

import (
	"fmt"
	"io"

	"github.com/vibeknow/cli/internal/charcheck"
)

type textW struct{ w io.Writer }

func NewText(w io.Writer) *textW { return &textW{w: w} }

func (t *textW) Format() string { return "text" }

// Print writes args concatenated, stripping control chars.
func (t *textW) Print(args ...any) {
	s := fmt.Sprint(args...)
	_, _ = io.WriteString(t.w, charcheck.Strip(s))
}

func (t *textW) Printf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	_, _ = io.WriteString(t.w, charcheck.Strip(s))
}

func (t *textW) Println(args ...any) {
	t.Print(args...)
	_, _ = io.WriteString(t.w, "\n")
}
```

- [ ] **Step 5: Implement `json.go`**

```go
package output

import (
	"encoding/json"
	"io"
)

type jsonW struct{ enc *json.Encoder; w io.Writer }

func NewJSON(w io.Writer) *jsonW {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &jsonW{enc: enc, w: w}
}

func (j *jsonW) Format() string { return "json" }

func (j *jsonW) Object(payload map[string]any) error {
	out := map[string]any{"schema_version": schemaVersion}
	for k, v := range payload {
		out[k] = v
	}
	return j.enc.Encode(out)
}
```

- [ ] **Step 6: Implement `ndjson.go`**

```go
package output

import (
	"encoding/json"
	"io"
	"time"
)

type ndjsonW struct{ enc *json.Encoder }

func NewNDJSON(w io.Writer) *ndjsonW {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &ndjsonW{enc: enc}
}

func (n *ndjsonW) Format() string { return "ndjson" }

func (n *ndjsonW) Event(evt map[string]any) error {
	out := map[string]any{
		"schema_version": schemaVersion,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
	}
	for k, v := range evt {
		out[k] = v
	}
	return n.enc.Encode(out)
}
```

- [ ] **Step 7: Run tests — should pass**

```bash
go test -v ./internal/output/
```

- [ ] **Step 8: Commit**

```bash
git add internal/output/
git commit -m "feat(output): text/json/ndjson writers with charcheck and schema_version"
```

---

## Task 7: `internal/lockfile` — cross-process file lock

**Files:**
- Create: `internal/lockfile/lockfile.go`, `internal/lockfile/lockfile_test.go`

**Responsibility:** §8.7. Thin wrapper over `github.com/gofrs/flock` providing `WithLock(path, fn)` blocking helper. Used only by profile writes and update checks.

- [ ] **Step 1: Add dependency**

```bash
go get github.com/gofrs/flock
```

- [ ] **Step 2: Write failing test**

```go
package lockfile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWithLockSerializes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")
	var counter int
	var mu sync.Mutex // for scoreboard of observed counter values
	var observed []int
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := WithLock(path, func() error {
				mu.Lock()
				counter++
				observed = append(observed, counter)
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Errorf("WithLock: %v", err)
			}
		}()
	}
	wg.Wait()
	if counter != 5 {
		t.Fatalf("counter=%d want 5", counter)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("lock file should exist after use: %v", err)
	}
}
```

- [ ] **Step 3: Run — fails**

- [ ] **Step 4: Implement `lockfile.go`**

```go
// Package lockfile provides a cross-process file lock helper.
// See spec §8.7.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// WithLock acquires an exclusive OS-level advisory lock on path, runs fn,
// then releases. Blocks until the lock is obtained.
func WithLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("lockfile: mkdir: %w", err)
	}
	l := flock.New(path)
	if err := l.Lock(); err != nil {
		return fmt.Errorf("lockfile: acquire %s: %w", path, err)
	}
	defer l.Unlock()
	return fn()
}
```

- [ ] **Step 5: Run tests — pass**

- [ ] **Step 6: Commit**

```bash
git add internal/lockfile/ go.mod go.sum
git commit -m "feat(lockfile): cross-process lock wrapper over gofrs/flock"
```

---

## Task 8: `internal/config` — paths + Profile schema + validation (TDD)

**Files:**
- Create: `internal/config/path.go`, `internal/config/schema.go`, `internal/config/schema_test.go`

**Responsibility:** §4.3 + §11.3. Define `Profile` struct matching canonical schema; validate fields per rules; compute OS-aware config dir.

- [ ] **Step 1: Write failing test**

```go
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
```

- [ ] **Step 2: Run — fails**

- [ ] **Step 3: Implement `path.go`**

```go
// Package config manages profiles.yaml and related configuration.
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir returns the platform-appropriate vibeknow config directory,
// honoring VIBEKNOW_CONFIG_HOME override. Directory is NOT created.
func ConfigDir() (string, error) {
	if d := os.Getenv("VIBEKNOW_CONFIG_HOME"); d != "" {
		return d, nil
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("AppData"), "vibeknow"), nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "vibeknow"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "vibeknow"), nil
}

// ProfilesPath returns the absolute path to profiles.yaml.
func ProfilesPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "profiles.yaml"), nil
}
```

- [ ] **Step 4: Implement `schema.go`**

```go
package config

import (
	"fmt"
	"net/url"
	"regexp"
)

// Profile is the canonical profile shape. See spec §4.3 and §11.3.
type Profile struct {
	Name             string            `yaml:"name"`
	APIEndpoint      string            `yaml:"api_endpoint"`
	CredentialRef    string            `yaml:"credential_ref"`
	DefaultProject   string            `yaml:"default_project,omitempty"`
	Trust            string            `yaml:"trust,omitempty"`            // user | dev
	IsProduction     bool              `yaml:"is_production"`              // default true
	ServiceOverrides map[string]string `yaml:"service_overrides,omitempty"`
}

// ProfilesFile is the top-level YAML shape.
type ProfilesFile struct {
	SchemaVersion string    `yaml:"schema_version"`
	Current       string    `yaml:"current"`
	Profiles      []Profile `yaml:"profiles"`
}

var nameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// Validate enforces the rules listed in spec §11.3.
func (p Profile) Validate() error {
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("profile.name %q invalid (must match %s)", p.Name, nameRe)
	}
	u, err := url.Parse(p.APIEndpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("profile.api_endpoint %q must be an absolute URL", p.APIEndpoint)
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
	if len(p.ServiceOverrides) > 0 {
		if trust != "dev" {
			return fmt.Errorf("profile.service_overrides requires trust=dev")
		}
		if p.IsProduction {
			return fmt.Errorf("profile.service_overrides requires is_production=false")
		}
	}
	return nil
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

- [ ] **Step 5: Run tests — pass**

```bash
go test -v ./internal/config/
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/
git commit -m "feat(config): Profile schema with validation rules from spec §11.3"
```

---

## Task 9: `internal/config/profiles.yaml` load/save with lockfile (TDD)

**Files:**
- Create: `internal/config/profiles.go`, `internal/config/profiles_test.go`

**Responsibility:** §8.7 + §11.3. Load, save, and CRUD profiles. All writes use `lockfile.WithLock` over a sidecar `.lock` file. When the file is absent, a Load returns an empty `ProfilesFile` with schema_version `"1"`.

- [ ] **Step 1: Add YAML dep**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 2: Write failing test**

```go
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
	if f.SchemaVersion != "1" || len(f.Profiles) != 0 {
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
```

- [ ] **Step 3: Run — fails**

- [ ] **Step 4: Implement `profiles.go`**

```go
package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/vibeknow/cli/internal/lockfile"
	"gopkg.in/yaml.v3"
)

func lockPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return d + "/profiles.lock", nil
}

// LoadProfiles reads profiles.yaml, returning an empty file if absent.
func LoadProfiles() (ProfilesFile, error) {
	var f ProfilesFile
	path, err := ProfilesPath()
	if err != nil {
		return f, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ProfilesFile{SchemaVersion: "1"}, nil
	}
	if err != nil {
		return f, fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.SchemaVersion == "" {
		f.SchemaVersion = "1"
	}
	if err := f.Validate(); err != nil {
		return f, fmt.Errorf("validate %s: %w", path, err)
	}
	return f, nil
}

// SaveProfiles writes profiles.yaml atomically under a file lock.
func SaveProfiles(f ProfilesFile) error {
	if f.SchemaVersion == "" {
		f.SchemaVersion = "1"
	}
	if err := f.Validate(); err != nil {
		return err
	}
	path, err := ProfilesPath()
	if err != nil {
		return err
	}
	lp, err := lockPath()
	if err != nil {
		return err
	}
	return lockfile.WithLock(lp, func() error {
		data, err := yaml.Marshal(f)
		if err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	})
}

// AddProfile inserts p; fails if p.Name already exists.
func AddProfile(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	f, err := LoadProfiles()
	if err != nil {
		return err
	}
	for _, existing := range f.Profiles {
		if existing.Name == p.Name {
			return fmt.Errorf("profile %q already exists", p.Name)
		}
	}
	f.Profiles = append(f.Profiles, p)
	if f.Current == "" {
		f.Current = p.Name
	}
	return SaveProfiles(f)
}

// UseProfile sets current.
func UseProfile(name string) error {
	f, err := LoadProfiles()
	if err != nil {
		return err
	}
	for _, p := range f.Profiles {
		if p.Name == name {
			f.Current = name
			return SaveProfiles(f)
		}
	}
	return fmt.Errorf("profile %q not found", name)
}

// RemoveProfile deletes by name; clears current if it matched.
func RemoveProfile(name string) error {
	f, err := LoadProfiles()
	if err != nil {
		return err
	}
	out := f.Profiles[:0]
	found := false
	for _, p := range f.Profiles {
		if p.Name == name {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return fmt.Errorf("profile %q not found", name)
	}
	f.Profiles = out
	if f.Current == name {
		f.Current = ""
	}
	return SaveProfiles(f)
}
```

- [ ] **Step 5: Run tests — pass**

- [ ] **Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(config): profiles.yaml load/save with lockfile and CRUD helpers"
```

---

## Task 10: `internal/keychain` — OS keychain abstraction (TDD with fallback-only test)

**Files:**
- Create: `internal/keychain/keychain.go`, `internal/keychain/keychain_test.go`

**Responsibility:** abstract CRUD over an OS keychain entry. Use `github.com/99designs/keyring` which already handles macOS Keychain, Windows Credential Manager, Linux Secret Service. Tests use the in-memory backend.

- [ ] **Step 1: Add dep**

```bash
go get github.com/99designs/keyring
```

- [ ] **Step 2: Write failing test**

```go
package keychain

import "testing"

func TestInMemoryCRUD(t *testing.T) {
	k, err := OpenFor("vibeknow-test", WithInMemory())
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Set("e1", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	got, err := k.Get("e1")
	if err != nil || string(got) != "secret" {
		t.Fatalf("get: %q err=%v", got, err)
	}
	if err := k.Delete("e1"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get("e1"); err == nil {
		t.Error("want not-found error after delete")
	}
}
```

- [ ] **Step 3: Run — fails**

- [ ] **Step 4: Implement `keychain.go`**

```go
// Package keychain wraps an OS-native secret store (delegating to
// 99designs/keyring). See spec §4.2.
package keychain

import (
	"fmt"

	"github.com/99designs/keyring"
)

type Keychain struct{ ring keyring.Keyring }

type Option func(*keyring.Config)

// WithInMemory forces the FileBackend or in-memory backend for tests.
func WithInMemory() Option {
	return func(c *keyring.Config) {
		c.AllowedBackends = []keyring.BackendType{keyring.FileBackend}
		c.FileDir = "" // in-memory-ish when combined with no password
		c.FilePasswordFunc = keyring.FixedStringPrompt("test")
	}
}

// OpenFor opens (or creates) a keychain scoped to service.
func OpenFor(service string, opts ...Option) (*Keychain, error) {
	cfg := keyring.Config{
		ServiceName: service,
		AllowedBackends: []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.WinCredBackend,
			keyring.SecretServiceBackend,
			keyring.FileBackend,
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	r, err := keyring.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("keychain open: %w", err)
	}
	return &Keychain{ring: r}, nil
}

func (k *Keychain) Set(key string, data []byte) error {
	return k.ring.Set(keyring.Item{Key: key, Data: data})
}

func (k *Keychain) Get(key string) ([]byte, error) {
	item, err := k.ring.Get(key)
	if err != nil {
		return nil, err
	}
	return item.Data, nil
}

func (k *Keychain) Delete(key string) error { return k.ring.Remove(key) }
```

- [ ] **Step 5: Run tests — pass**

If the `FileBackend` requires a directory, adjust the test by providing `t.TempDir()` via a test-only helper. If the keychain test cannot run on CI (no Secret Service on headless Linux), ensure only `FileBackend` path is exercised in tests; skip others with `t.Skip` guarded by `runtime.GOOS`.

- [ ] **Step 6: Commit**

```bash
git add internal/keychain/ go.mod go.sum
git commit -m "feat(keychain): OS keychain abstraction via 99designs/keyring"
```

---

## Task 11: `internal/credential/file_store` — AES-GCM encrypted file (TDD)

**Files:**
- Create: `internal/credential/file_store.go`, `internal/credential/file_store_test.go`

**Responsibility:** §4.2 + §8.5. Store a token encrypted at rest for Linux headless fallback. Key derivation: scrypt(passphrase, machine-id-salt). Passphrase is supplied by caller (env var or prompt by future `auth login` — P0 tests supply explicitly).

- [ ] **Step 1: Write failing test**

```go
package credential

import (
	"path/filepath"
	"testing"
)

func TestFileStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.enc")
	s := NewFileStore(path, "correct-horse-battery-staple")

	if err := s.Set([]byte("tok_abc")); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get()
	if err != nil || string(got) != "tok_abc" {
		t.Fatalf("get: %q err=%v", got, err)
	}

	s2 := NewFileStore(path, "wrong-passphrase")
	if _, err := s2.Get(); err == nil {
		t.Error("wrong passphrase should fail")
	}

	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(); err == nil {
		t.Error("expected not-found after delete")
	}
}
```

- [ ] **Step 2: Run — fails**

- [ ] **Step 3: Implement `file_store.go`**

```go
package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/scrypt"
)

type FileStore struct {
	path       string
	passphrase string
}

func NewFileStore(path, passphrase string) *FileStore {
	return &FileStore{path: path, passphrase: passphrase}
}

// ErrNotFound signals absent credential.
var ErrNotFound = errors.New("credential: not found")

func (f *FileStore) Set(plaintext []byte) error {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	key, err := scrypt.Key([]byte(f.passphrase), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	buf := make([]byte, 0, len(salt)+len(nonce)+len(ct))
	buf = append(buf, salt...)
	buf = append(buf, nonce...)
	buf = append(buf, ct...)
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(f.path, buf, 0o600)
}

func (f *FileStore) Get() ([]byte, error) {
	buf, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(buf) < 16+12 {
		return nil, fmt.Errorf("credential: file too short")
	}
	salt := buf[:16]
	key, err := scrypt.Key([]byte(f.passphrase), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(buf) < 16+ns {
		return nil, fmt.Errorf("credential: file too short for nonce")
	}
	nonce := buf[16 : 16+ns]
	ct := buf[16+ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

func (f *FileStore) Delete() error {
	err := os.Remove(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}
```

- [ ] **Step 4: Add scrypt dep**

```bash
go get golang.org/x/crypto/scrypt
```

- [ ] **Step 5: Run tests — pass**

- [ ] **Step 6: Commit**

```bash
git add internal/credential/ go.mod go.sum
git commit -m "feat(credential): AES-GCM file store with scrypt key derivation"
```

---

## Task 12: `internal/credential/resolver` — env + keychain + file priority (TDD)

**Files:**
- Create: `internal/credential/resolver.go`, `internal/credential/resolver_test.go`, `internal/credential/env.go`

**Responsibility:** §8.5. Given a profile, return the effective token by consulting `VIBEKNOW_TOKEN` env first, then keychain, then file. Used by P1+'s HTTP client middleware.

- [ ] **Step 1: Write failing test**

```go
package credential

import (
	"os"
	"path/filepath"
	"testing"
)

type fakeKC struct{ data map[string][]byte }

func (f *fakeKC) Get(k string) ([]byte, error) {
	v, ok := f.data[k]
	if !ok {
		return nil, ErrNotFound
	}
	return v, nil
}
func (f *fakeKC) Set(k string, v []byte) error { f.data[k] = v; return nil }
func (f *fakeKC) Delete(k string) error        { delete(f.data, k); return nil }

func TestResolverEnvWins(t *testing.T) {
	t.Setenv("VIBEKNOW_TOKEN", "from-env")
	r := Resolver{
		Env: EnvSource{Var: "VIBEKNOW_TOKEN"},
		Keychain: KeychainSource{
			Keychain: &fakeKC{data: map[string][]byte{"k1": []byte("from-keychain")}},
			Entry:    "k1",
		},
	}
	tok, src, err := r.Resolve()
	if err != nil || tok != "from-env" || src != "env" {
		t.Fatalf("tok=%q src=%q err=%v", tok, src, err)
	}
}

func TestResolverKeychainFallback(t *testing.T) {
	os.Unsetenv("VIBEKNOW_TOKEN")
	r := Resolver{
		Env: EnvSource{Var: "VIBEKNOW_TOKEN"},
		Keychain: KeychainSource{
			Keychain: &fakeKC{data: map[string][]byte{"k1": []byte("from-keychain")}},
			Entry:    "k1",
		},
	}
	tok, src, err := r.Resolve()
	if err != nil || tok != "from-keychain" || src != "keychain" {
		t.Fatalf("tok=%q src=%q err=%v", tok, src, err)
	}
}

func TestResolverFileFallback(t *testing.T) {
	os.Unsetenv("VIBEKNOW_TOKEN")
	dir := t.TempDir()
	path := filepath.Join(dir, "c.enc")
	fs := NewFileStore(path, "pw")
	if err := fs.Set([]byte("from-file")); err != nil {
		t.Fatal(err)
	}
	r := Resolver{
		Env:      EnvSource{Var: "VIBEKNOW_TOKEN"},
		Keychain: KeychainSource{Keychain: &fakeKC{data: map[string][]byte{}}, Entry: "missing"},
		File:     FileSource{Store: fs},
	}
	tok, src, err := r.Resolve()
	if err != nil || tok != "from-file" || src != "file" {
		t.Fatalf("tok=%q src=%q err=%v", tok, src, err)
	}
}

func TestResolverNone(t *testing.T) {
	os.Unsetenv("VIBEKNOW_TOKEN")
	r := Resolver{Env: EnvSource{Var: "VIBEKNOW_TOKEN"}}
	_, _, err := r.Resolve()
	if err == nil {
		t.Error("expected error when no source has credential")
	}
}
```

- [ ] **Step 2: Run — fails**

- [ ] **Step 3: Implement `env.go`**

```go
package credential

import "os"

type EnvSource struct{ Var string }

func (e EnvSource) Get() (string, error) {
	v := os.Getenv(e.Var)
	if v == "" {
		return "", ErrNotFound
	}
	return v, nil
}
```

- [ ] **Step 4: Implement `resolver.go`**

```go
package credential

import "fmt"

// KeychainAccess is the subset of internal/keychain we need, broken out so
// tests can substitute a fake.
type KeychainAccess interface {
	Get(key string) ([]byte, error)
	Set(key string, data []byte) error
	Delete(key string) error
}

type KeychainSource struct {
	Keychain KeychainAccess
	Entry    string
}

func (k KeychainSource) Get() (string, error) {
	if k.Keychain == nil || k.Entry == "" {
		return "", ErrNotFound
	}
	data, err := k.Keychain.Get(k.Entry)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type FileSource struct{ Store *FileStore }

func (f FileSource) Get() (string, error) {
	if f.Store == nil {
		return "", ErrNotFound
	}
	data, err := f.Store.Get()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Resolver implements the priority order from spec §8.5: env > keychain > file.
type Resolver struct {
	Env      EnvSource
	Keychain KeychainSource
	File     FileSource
}

// Resolve returns (token, sourceName, error).
func (r Resolver) Resolve() (string, string, error) {
	if tok, err := r.Env.Get(); err == nil {
		return tok, "env", nil
	}
	if tok, err := r.Keychain.Get(); err == nil {
		return tok, "keychain", nil
	}
	if tok, err := r.File.Get(); err == nil {
		return tok, "file", nil
	}
	return "", "", fmt.Errorf("no credential available (checked env, keychain, file)")
}
```

- [ ] **Step 5: Run tests — pass**

- [ ] **Step 6: Commit**

```bash
git add internal/credential/
git commit -m "feat(credential): resolver honoring env>keychain>file priority"
```

---

## Task 13: `cmd/root` — global flags, version, completion, update-stub

**Files:**
- Modify: `cmd/root.go`
- Create: `cmd/version.go`, `cmd/completion.go`, `cmd/update.go`

**Responsibility:** §4.3 global flags (`--profile`, `--output`, `--verbose`, `--help`, `--version`) wiring, initializing i18n once. `version` prints `{{version}}` embedded via ldflags. `completion` delegates to cobra's builtin. `update` in P0 prints "not implemented in P0" and exits 0 (so CI scripts don't break).

- [ ] **Step 1: Rewrite `cmd/root.go`**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/i18n"
)

var (
	version      = "dev" // set via -ldflags
	flagProfile  string
	flagOutput   string
	flagVerbose  bool
)

var rootCmd = &cobra.Command{
	Use:           "vibeknow",
	Short:         "vibeknow CLI — turn docs into videos",
	SilenceUsage:  true,
	SilenceErrors: false,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		i18n.Init()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagProfile, "profile", "", "override active profile for this command")
	rootCmd.PersistentFlags().StringVar(&flagOutput, "output", "", "output format: text|json|ndjson (auto-selects based on TTY)")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "emit request/response summaries (credentials redacted)")
	rootCmd.Version = version // enables --version
}

func Execute() error {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	return err
}
```

- [ ] **Step 2: Write `cmd/version.go`**

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "print CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}

func init() { rootCmd.AddCommand(versionCmd) }
```

- [ ] **Step 3: Write `cmd/completion.go`**

```go
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:       "completion [bash|zsh|fish|powershell]",
	Short:     "generate shell completion script",
	Args:      cobra.ExactValidArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			_ = rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			_ = rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			_ = rootCmd.GenPowerShellCompletion(os.Stdout)
		}
	},
}

func init() { rootCmd.AddCommand(completionCmd) }
```

- [ ] **Step 4: Write `cmd/update.go` stub**

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "update the CLI (not implemented in P0)",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("update: not implemented in P0; use `npm update -g @vibeknow/cli`")
	},
}

func init() { rootCmd.AddCommand(updateCmd) }
```

- [ ] **Step 5: Verify build & flags**

```bash
make build
./vibeknow --help | grep -E "(profile|output|verbose)"
./vibeknow version
./vibeknow completion bash | head -5
./vibeknow update
```
Expected: flags shown, version prints `dev` (or ldflags value), completion emits bash script prelude, update prints stub message.

- [ ] **Step 6: Commit**

```bash
git add cmd/
git commit -m "feat(cmd): root with global flags, version, completion, update-stub"
```

---

## Task 14: `cmd/profile` — add / list / use / remove / show

**Files:**
- Create: `cmd/profile/profile.go`, `cmd/profile/add.go`, `cmd/profile/list.go`, `cmd/profile/use.go`, `cmd/profile/remove.go`, `cmd/profile/show.go`

**Responsibility:** CRUD wrappers over `internal/config`. All user-facing messages go through `i18n.T`. Output obeys `--output` (text=friendly, json=object, ndjson=treated as json for these non-streaming commands).

- [ ] **Step 1: Write `cmd/profile/profile.go` parent**

```go
package profile

import "github.com/spf13/cobra"

// Cmd is the parent "profile" command, added to root by cmd.init().
var Cmd = &cobra.Command{
	Use:   "profile",
	Short: "manage vibeknow profiles",
}

func init() {
	Cmd.AddCommand(addCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(useCmd)
	Cmd.AddCommand(removeCmd)
	Cmd.AddCommand(showCmd)
}
```

- [ ] **Step 2: Register in root**

Add to `cmd/root.go` init():
```go
import profilecmd "github.com/vibeknow/cli/cmd/profile"
// inside init():
rootCmd.AddCommand(profilecmd.Cmd)
```

- [ ] **Step 3: Write `add.go`**

```go
package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/i18n"
)

var addFlags struct {
	apiEndpoint    string
	credentialRef  string
	defaultProject string
	trust          string
	isProduction   bool
}

var addCmd = &cobra.Command{
	Use:   "add NAME",
	Short: "add a new profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p := config.Profile{
			Name:           args[0],
			APIEndpoint:    addFlags.apiEndpoint,
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
	addCmd.Flags().StringVar(&addFlags.apiEndpoint, "api-endpoint", "", "gateway URL (required)")
	addCmd.Flags().StringVar(&addFlags.credentialRef, "credential-ref", "", "keychain entry name or file:// path (required)")
	addCmd.Flags().StringVar(&addFlags.defaultProject, "default-project", "", "optional default project name")
	addCmd.Flags().StringVar(&addFlags.trust, "trust", "user", "user|dev")
	addCmd.Flags().BoolVar(&addFlags.isProduction, "is-production", true, "treat as production (required false to allow service_overrides)")
	_ = addCmd.MarkFlagRequired("api-endpoint")
	_ = addCmd.MarkFlagRequired("credential-ref")
}
```

- [ ] **Step 4: Write `list.go`**

```go
package profile

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		if len(f.Profiles) == 0 {
			fmt.Fprintln(os.Stdout, "(no profiles)")
			return nil
		}
		for _, p := range f.Profiles {
			marker := "  "
			if p.Name == f.Current {
				marker = "* "
			}
			fmt.Printf("%s%s\t%s\n", marker, p.Name, p.APIEndpoint)
		}
		return nil
	},
}
```

- [ ] **Step 5: Write `use.go`**

```go
package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/i18n"
)

var useCmd = &cobra.Command{
	Use:   "use NAME",
	Short: "switch active profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.UseProfile(args[0]); err != nil {
			return err
		}
		fmt.Println(i18n.T("msg.profile.switched", args[0]))
		return nil
	},
}
```

- [ ] **Step 6: Write `remove.go`**

```go
package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/i18n"
)

var removeCmd = &cobra.Command{
	Use:   "remove NAME",
	Short: "delete a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.RemoveProfile(args[0]); err != nil {
			return err
		}
		fmt.Println(i18n.T("msg.profile.removed", args[0]))
		return nil
	},
}
```

- [ ] **Step 7: Write `show.go`**

```go
package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
)

var showCmd = &cobra.Command{
	Use:   "show [NAME]",
	Short: "show profile details (default: current)",
	Args:  cobra.MaximumNArgs(1),
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
				fmt.Printf("name: %s\napi_endpoint: %s\ncredential_ref: %s\ntrust: %s\nis_production: %v\ndefault_project: %s\n",
					p.Name, p.APIEndpoint, p.CredentialRef, p.Trust, p.IsProduction, p.DefaultProject)
				return nil
			}
		}
		return fmt.Errorf("profile %q not found", name)
	},
}
```

- [ ] **Step 8: Manual smoke test**

```bash
make build
./vibeknow profile list
./vibeknow profile add dev --api-endpoint https://staging.example.com --credential-ref vibeknow.dev --trust dev --is-production=false
./vibeknow profile list
./vibeknow profile use dev
./vibeknow profile show
./vibeknow profile remove dev
./vibeknow profile list
```

Clean up test state:
```bash
rm -rf ~/.config/vibeknow/
```

- [ ] **Step 9: Commit**

```bash
git add cmd/
git commit -m "feat(profile): add/list/use/remove/show commands"
```

---

## Task 15: `cmd/config` — get / set / list

**Files:**
- Create: `cmd/config/config.go`, `cmd/config/get.go`, `cmd/config/set.go`, `cmd/config/list.go`
- Modify: `internal/config/kv.go` (new, for arbitrary KV storage separate from profiles)

**Responsibility:** `vibeknow config` targets a simple flat string→string map stored in `~/.config/vibeknow/config.yaml`, separate from profiles. P0 keys: `default_profile` (alias for `profile use`), `analytics_enabled` (reserved for future). Implementation is minimal — just enough so users can set arbitrary keys without us preempting the namespace.

- [ ] **Step 1: Implement `internal/config/kv.go`**

```go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vibeknow/cli/internal/lockfile"
	"gopkg.in/yaml.v3"
)

type KV map[string]string

func kvPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

func kvLock() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.lock"), nil
}

func LoadKV() (KV, error) {
	path, err := kvPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return KV{}, nil
	}
	if err != nil {
		return nil, err
	}
	kv := KV{}
	if err := yaml.Unmarshal(data, &kv); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return kv, nil
}

func SaveKV(kv KV) error {
	path, err := kvPath()
	if err != nil {
		return err
	}
	lp, err := kvLock()
	if err != nil {
		return err
	}
	return lockfile.WithLock(lp, func() error {
		data, err := yaml.Marshal(kv)
		if err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	})
}
```

- [ ] **Step 2: Write `cmd/config/config.go`**

```go
package config

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "config",
	Short: "manage vibeknow global config",
}

func init() {
	Cmd.AddCommand(getCmd)
	Cmd.AddCommand(setCmd)
	Cmd.AddCommand(listCmd)
}
```

- [ ] **Step 3: Write `get.go` / `set.go` / `list.go`**

`cmd/config/get.go`:
```go
package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
)

var getCmd = &cobra.Command{
	Use:   "get KEY",
	Short: "read a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kv, err := config.LoadKV()
		if err != nil {
			return err
		}
		v, ok := kv[args[0]]
		if !ok {
			return fmt.Errorf("key %q not set", args[0])
		}
		fmt.Println(v)
		return nil
	},
}
```

`cmd/config/set.go`:
```go
package config

import (
	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
)

var setCmd = &cobra.Command{
	Use:   "set KEY VALUE",
	Short: "write a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		kv, err := config.LoadKV()
		if err != nil {
			return err
		}
		kv[args[0]] = args[1]
		return config.SaveKV(kv)
	},
}
```

`cmd/config/list.go`:
```go
package config

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list all config keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		kv, err := config.LoadKV()
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(kv))
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s=%s\n", k, kv[k])
		}
		return nil
	},
}
```

- [ ] **Step 4: Register in root**

Add to `cmd/root.go`:
```go
import configcmd "github.com/vibeknow/cli/cmd/config"
// inside init():
rootCmd.AddCommand(configcmd.Cmd)
```

- [ ] **Step 5: Smoke test**

```bash
make build
./vibeknow config set analytics_enabled false
./vibeknow config get analytics_enabled
./vibeknow config list
rm -rf ~/.config/vibeknow/
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/kv.go cmd/config/ cmd/root.go
git commit -m "feat(config): get/set/list commands backed by config.yaml"
```

---

## Task 16: `cmd/doctor` — local-only checks

**Files:**
- Create: `cmd/doctor.go`

**Responsibility:** §8.4. In P0 scope: verify config dir writable, profiles.yaml parses (or is absent), keychain backend reachable, locale detection sane. Each check prints `[ok]` / `[fail]`. Non-zero exit if any check fails.

- [ ] **Step 1: Write `cmd/doctor.go`**

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/keychain"
)

type check struct {
	name string
	fn   func() error
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "diagnose local setup (P0 runs local checks only)",
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
	_, err := keychain.OpenFor("vibeknow-doctor-probe")
	return err
}

func checkLocale() error {
	// Any non-empty output from i18n.T for a known key is success.
	if got := i18n.T("doctor.header"); got == "" {
		return fmt.Errorf("i18n returned empty string")
	}
	return nil
}
```

- [ ] **Step 2: Smoke test**

```bash
make build
./vibeknow doctor
echo "exit=$?"
```
Expected: four `[ok]` lines (or `[fail]` on headless systems without keychain — acceptable in P0).

- [ ] **Step 3: Commit**

```bash
git add cmd/doctor.go
git commit -m "feat(doctor): local checks for config dir, profiles, keychain, locale"
```

---

## Task 17: npm packaging

**Files:**
- Create: `package.json`, `scripts/npm-postinstall.js`, `scripts/npm-launcher.js`

**Responsibility:** §8.2. User runs `npm install -g @vibeknow/cli` → postinstall picks the matching prebuilt binary from `dist/` (or downloads from GitHub Releases — P0 uses local `dist/` only; release automation is P3).

- [ ] **Step 1: Write `package.json`**

```json
{
  "name": "@vibeknow/cli",
  "version": "0.1.0-p0",
  "description": "vibeknow CLI — turn docs into videos",
  "license": "MIT",
  "bin": {
    "vibeknow": "scripts/npm-launcher.js"
  },
  "scripts": {
    "postinstall": "node scripts/npm-postinstall.js"
  },
  "files": [
    "scripts/",
    "dist/"
  ],
  "engines": {
    "node": ">=16"
  }
}
```

- [ ] **Step 2: Write `scripts/npm-postinstall.js`**

```js
#!/usr/bin/env node
const fs = require('fs');
const os = require('os');
const path = require('path');

const platformMap = {
  'darwin-x64':  'vibeknow-darwin-amd64',
  'darwin-arm64':'vibeknow-darwin-arm64',
  'linux-x64':   'vibeknow-linux-amd64',
  'linux-arm64': 'vibeknow-linux-arm64',
  'win32-x64':   'vibeknow-windows-amd64.exe',
};

const key = `${process.platform}-${process.arch}`;
const fname = platformMap[key];
if (!fname) {
  console.error(`[vibeknow] unsupported platform: ${key}`);
  process.exit(1);
}
const src = path.join(__dirname, '..', 'dist', fname);
const dst = path.join(__dirname, '..', 'dist', 'vibeknow' + (process.platform === 'win32' ? '.exe' : ''));
if (!fs.existsSync(src)) {
  console.error(`[vibeknow] missing binary: ${src}`);
  process.exit(1);
}
fs.copyFileSync(src, dst);
fs.chmodSync(dst, 0o755);
```

- [ ] **Step 3: Write `scripts/npm-launcher.js`**

```js
#!/usr/bin/env node
const { spawnSync } = require('child_process');
const path = require('path');
const bin = path.join(__dirname, '..', 'dist', 'vibeknow' + (process.platform === 'win32' ? '.exe' : ''));
const r = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
process.exit(r.status ?? 1);
```

- [ ] **Step 4: Local end-to-end smoke**

```bash
./build.sh
npm pack
# In another temp dir:
mkdir -p /tmp/vibe-install && cd /tmp/vibe-install
npm install -g $OLDPWD/vibeknow-cli-0.1.0-p0.tgz   # adjust path
vibeknow version
vibeknow profile list
npm uninstall -g @vibeknow/cli
```
Expected: `vibeknow version` prints a git-derived version.

- [ ] **Step 5: Commit**

```bash
git add package.json scripts/
git commit -m "feat(npm): postinstall selects prebuilt binary per platform"
```

---

## Task 18: End-to-end smoke test in CI

**Files:**
- Create: `tests/integration/cli_smoke_test.go`
- Modify: `.github/workflows/ci.yml` (add smoke job)

**Responsibility:** guard against regressions as P1+ layers on. The test builds the binary, runs a scripted sequence (profile add/list/use/show/remove, config set/get, doctor), and asserts exit codes and key output fragments.

- [ ] **Step 1: Write `tests/integration/cli_smoke_test.go`**

```go
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func build(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "vibeknow")
	root, _ := filepath.Abs("../..")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("build: %v", err)
	}
	return bin
}

func run(t *testing.T, bin, home string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "VIBEKNOW_CONFIG_HOME="+home)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), code
}

func TestCLISmoke(t *testing.T) {
	bin := build(t)
	home := t.TempDir()

	// version works
	out, _, code := run(t, bin, home, "version")
	if code != 0 || out == "" {
		t.Fatalf("version: code=%d out=%q", code, out)
	}

	// profile add
	_, _, code = run(t, bin, home,
		"profile", "add", "dev",
		"--api-endpoint", "https://staging.example.com",
		"--credential-ref", "vibeknow.dev",
		"--trust", "dev",
		"--is-production=false",
	)
	if code != 0 {
		t.Fatalf("profile add failed: code=%d", code)
	}

	// profile list contains dev
	out, _, code = run(t, bin, home, "profile", "list")
	if code != 0 || !strings.Contains(out, "dev") {
		t.Fatalf("profile list: code=%d out=%q", code, out)
	}

	// profile show
	out, _, code = run(t, bin, home, "profile", "show", "dev")
	if code != 0 || !strings.Contains(out, "staging.example.com") {
		t.Fatalf("profile show: code=%d out=%q", code, out)
	}

	// config set/get
	_, _, code = run(t, bin, home, "config", "set", "k1", "v1")
	if code != 0 {
		t.Fatal("config set")
	}
	out, _, code = run(t, bin, home, "config", "get", "k1")
	if code != 0 || strings.TrimSpace(out) != "v1" {
		t.Fatalf("config get: code=%d out=%q", code, out)
	}

	// doctor (may exit non-zero on headless CI if keychain unreachable; just verify it runs and prints header)
	out, _, _ = run(t, bin, home, "doctor")
	if !strings.Contains(out, "doctor") {
		t.Fatalf("doctor output missing header: %q", out)
	}

	// profile remove
	_, _, code = run(t, bin, home, "profile", "remove", "dev")
	if code != 0 {
		t.Fatal("profile remove")
	}
}
```

- [ ] **Step 2: Run locally**

```bash
go test -count=1 ./tests/integration/
```
Expected: pass.

- [ ] **Step 3: Update CI workflow**

In `.github/workflows/ci.yml`, the existing `go test ./...` already picks up `tests/integration/`. Add a separate step to make it obvious:

```yaml
      - name: Integration smoke
        run: go test -count=1 ./tests/integration/...
```

- [ ] **Step 4: Commit**

```bash
git add tests/integration/ .github/workflows/ci.yml
git commit -m "test(p0): end-to-end CLI smoke test covering profile/config/doctor"
```

---

## Task 19: Verify + tag P0 release candidate

- [ ] **Step 1: Run full test suite**

```bash
make lint && make test && make build
go test -count=1 ./tests/integration/...
```
Expected: all pass.

- [ ] **Step 2: Manual sanity sweep**

```bash
rm -rf /tmp/vibe-home && VIBEKNOW_CONFIG_HOME=/tmp/vibe-home ./vibeknow doctor
VIBEKNOW_CONFIG_HOME=/tmp/vibe-home ./vibeknow profile add prod \
  --api-endpoint https://api.example.com \
  --credential-ref vibeknow.prod
VIBEKNOW_CONFIG_HOME=/tmp/vibe-home ./vibeknow profile list
VIBEKNOW_CONFIG_HOME=/tmp/vibe-home ./vibeknow profile show
VIBEKNOW_CONFIG_HOME=/tmp/vibe-home ./vibeknow --version
```

- [ ] **Step 3: Tag**

```bash
git tag -a v0.1.0-p0 -m "P0: bootstrap and base framework"
```

- [ ] **Step 4: Update CHANGELOG**

Move the `[Unreleased]` bullets to a new `[0.1.0-p0] — 2026-04-15` section.

```bash
git add CHANGELOG.md
git commit -m "chore(release): cut v0.1.0-p0"
```

---

## Self-review notes

- **Spec coverage**: §3 structure (partial, skills deferred to P3), §4.2/§4.3 profile, §8.5 security primitives (charcheck/redact/credential), §8.7 lockfile, §8.9 i18n, §8.10 output, §11.3 profile schema — all touched. §4.1 gates are documented only; validation belongs to P1+. §5 task model, §6 hero, §7 Skills, §11.1/§11.2 streaming and error schemas, §8.2/§8.3/§8.4 release/telemetry, are explicit non-P0 scope.
- **Types consistency**: `Profile.IsProduction` / `ServiceOverrides` used consistently from Task 8 onward. `KeychainAccess` interface defined once in Task 12 and satisfied by the keychain package (both have `Get/Set/Delete` with matching signatures).
- **Placeholders**: none; every code step is complete. Open question markers (`<org>` module name) are flagged in Task 1 Step 1 with explicit fallback.

---

**Plan complete and saved.** Ready for execution.

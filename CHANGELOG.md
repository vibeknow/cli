# Changelog

## 0.5.0 — 2026-05-14

### New

- `vk create --engine pipeline|agent` selects which figlens engine to
  invoke. Default `pipeline` keeps 0.4.2 behavior bit-identical;
  `--engine agent` routes to the v=2 agent engine
  (`/agent2forVideo/stream`, mirrors the web frontend's engine toggle).
- The v=2 agent engine emits free-form progress events without a node
  graph. The CLI now surfaces these as `node.progress` events in
  NDJSON output and `[agent] <message>` lines in text output, instead
  of silently filtering them out.
- `vk video status` / `vk create` JSON snapshot now includes an
  `engine` field (`"pipeline"` or `"agent"`) so agents can confirm
  which engine actually ran. The DB enum `"suite"` is remapped to
  `"pipeline"` for output to match the `--engine` flag's vocabulary.

### Changed

- `--engine agent --mode replica` is rejected at the CLI boundary with
  exit 2 and a clear message: replica is a v=3-only pipeline feature
  with no agent-engine analog (verified against go-figlens source).

## 0.4.2 — 2026-05-14

### New

- `vk create --mode replica` runs the figlens PPT/PDF page-by-page
  replica pipeline. `vk create --mode script` runs the verbatim-script
  ("讲稿锁定") pipeline that uses the uploaded document as the
  narration. Both modes are now visible to humans and AI agents
  through the same single-flag surface; default invocation (no
  `--mode`) is unchanged.
- `vk create --aspect horizontal|vertical` selects 16:9 or 9:16
  output. Accepts `16:9` / `9:16` as aliases.
- `vk create --bgm` enables background music (off by default).
- SSE progress events for the replica pipeline's new nodes
  (`doc_replica_plan`, `doc_replica_shoot`) now surface in `text` and
  `ndjson` output — previously filtered out by the CLI's stage map.

### Changed

- Script-mode preflight failures (`POST /v1/tasks/init` returns code
  `100004`) now exit **2** (validation, user fixes input) with the
  backend's localized message, instead of exit 5 with a generic
  "business error" label.
- `figlens.InitTask` now takes `InitTaskParams{KnowledgeID, DocID,
  VideoKind}`. Default-zero params produce the same wire body as
  before (`{"v": 3}`), so callers that don't use script mode are
  unaffected.

## 0.4.1 — 2026-04-24

### Fixed

- `vk auth status` now shows the refresh-token lifetime (the real session
  window) instead of the 2h access-token lifetime, so the countdown
  reflects when the user actually has to sign in again. Added a "days"
  duration unit so week- or month-long sessions render as "6 days"
  rather than "168h".
- `httpclient.parseBackendError` now prefers the envelope business code
  on HTTP 4xx/5xx responses. Previously account-service codes 110004 /
  110008 / 110013 on HTTP 401 were collapsed to generic `auth_required`;
  they now surface as `account_disabled`, `session_replaced`, and
  `account_pending_deletion`.
- On a permanently-dead session (replaced on another device, account
  disabled, account pending deletion), `OAuthTokenProvider` now purges
  the stored credential and returns a single `session_expired` error
  with the underlying cause preserved in `details.cause_code` /
  `details.cause_message` / `trace_id`. Previously the stale token
  lingered in the keychain and each subsequent command produced a
  confusing `business_error` message.

## 0.4.0 — 2026-04-22

### Breaking

- `vk video download` no longer auto-triggers export. If the MP4 is not
  yet rendered, it now exits 2 with a hint pointing to `vk video export`.
  Migrate: replace `vk video download <id> --session-id <sess>` with
  `vk video export <id> --session-id <sess> && vk video download <id> --session-id <sess>`.
- Text-mode `duration=` changes from raw milliseconds to human-readable
  (`duration=42s`). JSON `duration_ms` remains ms. Scripts parsing
  `duration=` as an int must switch to `--output json`.

### New

- `vk video export` renders the MP4 as an explicit step. Supports
  `--async`, `--yes`, `--timeout`, `--poll-interval`.
- `vk video export-status <export_task_id>` polls a specific export.
- `vk create --export` chains preview + export in one call. Exits
  **7** on partial success (preview ready, export failed).
- `vk video status` now returns a complete snapshot: preview state,
  export state, and `next_actions` hints.
- `--output ndjson` on `create` / `wait` / `export` streams one event
  per line for agent consumers.
- `VIBEKNOW_ASSUME_YES=1` / `--yes` skip confirmation prompts on paid
  operations.

### Fixed

- `vk create` no longer prints empty `video_path=` / `video_url=` lines
  at preview time. The pipeline's `share_url` (HTML preview page) is
  now the primary output.

## [0.3.3] — 2026-04-18
### Added
- All user-visible strings now route through the i18n table (`VIBEKNOW_LANG=en|zh`). Previously ~30 strings across `init`, `auth login/status/whoami`, `create`, `credits`, `video list`, and upgrade notices were hardcoded in Chinese and ignored the locale flag.
- `vibeknow update` actually checks the npm registry and reports `up to date` or the pending upgrade. Previously it was a P0 stub that only pointed at `npm update -g`.
- `VIBEKNOW_DEBUG=1` and `--verbose` now emit the HTTP request/response summaries the docs promised. The flag is sugar for the env var.
- `--output json` wired into `voice list`, `video status`, `video url`, and `doc get` — agents can parse the response directly instead of scraping text.
- `cmd/video/{status,download,url,wait}` and `doc get` route flag/argument validation through `clerr.Validation` so `--output json` gives a clean `{"ok": false, "error": {"type": "validation", ...}}` envelope with exit code 2.

### Changed
- User-facing flag help and package docstrings no longer reference internal Go-module names (`go-account`, `go-figlens`, `go-atlas`, …). External users see `Account service URL override`, `VibeKnow API service`, etc.
- `vibeknow doctor` assumes every backend exposes a `/healthz` returning `{"status":"healthy"}` on 200 or `{"status":"unhealthy"}` on 503 (matches `go-atlas` v0.3.6+). The transitional multi-path / multi-shape tolerance was dropped now that backends are aligned.

### Removed
- Deprecated `--api-endpoint` flag on `profile add` (old alias for `--endpoint-vibeknow`). Silent YAML migration via `config.Profile.APIEndpoint` remains, so existing `profiles.yaml` files still load.
- 5 redundant `staticToken` type declarations collapsed into one `httpclient.StaticToken`.
- Unused `clerr.{TypePermission, TypeNotFound, TypeRateLimit}`, `output.Select`, `output.Writer.Format()`, and `cmdutil.Factory.Endpoint()` (kept `TokenProvider`, which `Service` calls internally).

## [0.3.2] — 2026-04-18
### Fixed
- `vibeknow doctor` no longer reports spurious `[fail]` lines against services whose health endpoints live at `/healthz` or `/health` or return envelope-wrapped JSON. Probes `/healthz` → `/v1/health` → `/health` in order, accepts flat / envelope / atlas-style response shapes, and reports services with no exposed health endpoint as `[warn]` (not counted toward the failure exit code).

## [0.3.1] — 2026-04-18
### Fixed
- `CloudDefaults` pointed at unreachable placeholder hostnames (`*.vibeknow.com`). Fresh `npm install -g vibeknow-cli && vibeknow init` now reaches the device-code step against the beta cluster instead of failing DNS resolution.

### Added
- `vibeknow auth status --output json` emits a machine-parseable envelope (`authenticated / profile / source / auth_method / token_status / expires_at / user`) for AI Agents and CI.
- `vibeknow init` mirrors the `VIBEKNOW_TOKEN` env-var warning already emitted by `auth login`, so users who carried over old credentials get an early signal before the wizard stores a keychain token that would be shadowed by the env var.
- Regression test pinning the `vibeknow auth login --no-wait` JSON envelope shape.
- Sanity test guarding `CloudDefaults` against typos (every URL parses as an absolute `https://` URL).

### Changed
- README (English and Chinese) Quickstart rewritten to match the shipped `vibeknow init` flow (humans) and the two-phase device flow (`--no-wait` / `--device-code`) for AI Agents. The "Coming in v1" Device-Flow note is removed — it has shipped.
- Hero Command example stops hardcoding a specific voice ID; users are directed to `vibeknow voice list` first.

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
- `doctor` header message updated to reflect P1 scope (environment + endpoint diagnostics).

### Deferred
- Interactive `auth login` (Device Flow + PAT) → P1.5 standalone project.
- Service clients for vectoria / figlens / vibeknow / speech → P2 (alongside shortcuts).

## [0.1.0-p0] — 2026-04-15
### Added
- Repository scaffold with cobra-based `vibeknow` CLI.
- Cross-platform build toolchain (`Makefile`, `build.sh`, GitHub Actions CI).
- Framework packages: `internal/charcheck`, `internal/redact`, `internal/i18n`, `internal/output`, `internal/lockfile`, `internal/keychain`, `internal/credential` (file store with AES-GCM + scrypt; env/keychain/file resolver), `internal/config` (profiles schema with validation, profiles.yaml r/w, generic KV).
- Base commands: `vibeknow version`, `vibeknow --version`, `vibeknow completion {bash|zsh|fish|powershell}`, `vibeknow update` (P0 stub), `vibeknow doctor` (local checks), `vibeknow profile add/list/use/remove/show`, `vibeknow config get/set/list`.
- Global flags: `--profile`, `--output`, `--verbose`.
- npm distribution via `@vibeknow/cli` with per-platform postinstall binary selection.
- End-to-end integration smoke test.

### Known limitations (P0)
- No `auth login/logout/whoami` (needs go-account client — P1).
- No backend-calling commands (P1 / P2).
- No `create` / `video` / `doc` / `rag` shortcuts (P2).
- No Skills published (P3).
- `update` command is a stub.

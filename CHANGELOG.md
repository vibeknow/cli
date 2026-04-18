# Changelog

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

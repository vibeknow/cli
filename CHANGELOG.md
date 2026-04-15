# Changelog

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

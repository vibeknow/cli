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

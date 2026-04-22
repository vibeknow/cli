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

## Two-stage video workflow

The video pipeline has two stages — agents should understand both:

1. **Preview** (produced by `vk create`): a shareable HTML page. The
   `share_url` in the output is the primary artifact; it plays the
   finished video.
2. **Export** (optional, via `vk video export`): renders a downloadable
   MP4. Takes several minutes and extra credits.

Commands:
- `vk create --from <src>` — pipeline → preview → `share_url`
- `vk video status <task_id> --session-id <sess> --output json` —
  full snapshot: `{preview, export, next_actions}`. Agents should
  follow `next_actions[].command` to plan the next step.
- `vk video export <task_id> --session-id <sess> --yes --output json` —
  render MP4. `--yes` skips the confirmation prompt; `VIBEKNOW_ASSUME_YES=1`
  works at the environment level.
- `vk video download <task_id> --session-id <sess>` — pure download
  (requires export to have completed).

Exit codes: `0` success, `2` user error, `5` business failure, `6`
interrupted, `7` partial success (preview ready, export failed).

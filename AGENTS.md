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

## Video kinds

`vk create --mode <kind>` picks which figlens pipeline runs:

- **default (no flag)** — generative video from the document.
- **`--mode replica`** — PPT/PDF page-by-page reproduction. Suited
  for slide decks where the visual structure is the message.
- **`--mode script`** — uses the uploaded document text *verbatim*
  as the narration. The doc must pass a quality preflight (length,
  characters, content). Preflight failures exit **2** with a clear
  message; agents should treat them as user-input problems, not
  retries.

Modes combine freely with `--aspect horizontal|vertical` (or `16:9` /
`9:16`) and `--bgm`. All three flags are independent of `--from`,
`--prompt`, `--voice`, and `--export`.

Exit-code summary for new modes:
- `2` — `--mode <bad>`, `--aspect <bad>`, or script preflight rejected the document.
- `5` — pipeline business failure (e.g., insufficient credits) — same as today.
- `7` — preview ok, MP4 export failed — same as today.

## Engine selection

`vk create --engine <name>` picks which go-figlens engine generates the
video. Two engines exist and are both actively maintained on the
backend:

- **`--engine pipeline`** (default) — v=3, graph-based pipeline. Has
  rich SSE node events (`[parse] prepare started`, `[outline]
  text_speech done`, ...). All `--mode` values supported.
- **`--engine agent`** — v=2, agent-driven flow. SSE events are
  free-form progress messages without a node graph; CLI shows them as
  `[agent] <message>` in text mode or `node.progress` in NDJSON.
  Supports `--mode script` (script_lock) but **not** `--mode replica`
  — the CLI rejects that combination with exit 2.

The `engine` field in JSON snapshot output reflects which engine ran
(`"pipeline"` / `"agent"`), letting agents verify routing.

## KB management

`vk kb list / delete / prune` manage vectoria knowledgebases. The
headline command is `vk kb prune`, which exists to clean up the kb
backlog that `vk create` accumulates (one new kb per invocation).

Safety contract for `vk kb prune`:

- **Refuses to run without `--pattern` or `--older-than`** (exit 2).
  No "delete all" shortcut.
- **Dry-run by default**: prints matched kbs without issuing any
  DELETE. `--yes` (or `VIBEKNOW_ASSUME_YES=1`) actually deletes.
- **Idempotent**: a 404 on DELETE counts as success (kb already gone).
- **Partial failure**: exit 7 if some succeed and some fail; exit 5
  if all fail; exit 0 if all succeed or matched empty.

Recipes:

- Clean CLI's own orphans: `vk kb prune --pattern 'vibeknow-cli-*' --yes`
- Clean old kbs (last 30 days): `vk kb prune --older-than 30d --yes`
- Both: `vk kb prune --pattern '*' --older-than 90d --yes`

Output via `--output json` for piping; default text mode is
human-friendly.

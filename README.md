# vibeknow-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.25-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/vibeknow-cli.svg)](https://www.npmjs.com/package/vibeknow-cli)

[中文版](./README.zh.md) | [English](./README.md)

The official [VibeKnow](https://vibeknow.com) CLI tool — built for humans and AI Agents. Turn documents, URLs, and files into professional videos from the command line. One command, zero editing skills.

[Install](#installation--quick-start) · [AI Agent Skills](#agent-skills) · [Auth](#authentication) · [Commands](#command-reference) · [Advanced](#advanced-usage) · [Contributing](#contributing)

## Why vibeknow-cli?

- **One Command, Full Video** — `vibeknow create --from report.pdf` handles everything: document parsing, script generation, TTS, scene design, rendering, and packaging
- **Agent-Native Design** — 3 structured [Skills](./skills/) out of the box, compatible with Claude Code, Cursor, and other AI tools — Agents can create videos with zero extra setup
- **Real-Time Stage Progress** — SSE streaming with 4-stage progress tracking (outline → tts → render → publish), both for human progress bars and machine-readable NDJSON
- **Multi-Service Architecture** — Connects to multiple backend services with per-service endpoint configuration and independent auth
- **Open Source, Zero Barriers** — MIT license, ready to use, just `npm install`
- **Secure & Controllable** — OS-native keychain credential storage, ANSI escape sanitization, credential redaction in verbose logs, non-production endpoint trust boundaries

## Features

| Category | Capabilities |
|----------|-------------|
| **Create** | One-command video generation from files, URLs, or doc IDs; custom prompts; voice selection; async mode |
| **Document** | Upload files/URLs, poll parsing status, fetch document details |
| **Video** | Check task status, stream live progress (SSE), download exported videos |
| **Voice** | List available voice templates with category, tags, and preview URLs |
| **Auth** | Browser-based Device Flow login (`vibeknow init` / `vibeknow auth login`); two-phase agent flow (`--no-wait` / `--device-code`); PAT via stdin; `VIBEKNOW_TOKEN` env var for CI |
| **Profile** | Multi-environment profiles (prod/staging/dev), per-service endpoint overrides, trust boundaries |
| **Config** | Global key-value config store, persistent across sessions |
| **Doctor** | Environment diagnostics: config dir, keychain, locale, endpoint reachability checks |
| **Raw API** | Escape hatch: `vibeknow api call` for direct backend access |

## Installation & Quick Start

### Requirements

- Node.js (`npm`/`npx`) for distribution
- Go `v1.25`+ (only required for building from source)

### Quick Start (Human Users)

> **Note for AI assistants:** If you are an AI Agent helping the user with installation, jump directly to [Quick Start (AI Agent)](#quick-start-ai-agent).

```bash
# 1. Install
npm install -g vibeknow-cli

# 2. Log in — creates the default profile and walks you through browser auth
vibeknow init

# 3. Make a video
vibeknow create --from https://example.com/article
```

That's the whole flow. `vibeknow init` handles profile creation, opens the browser for Device Flow authentication, and stores the token in your OS keychain.

> **Where the binary comes from**: `vibeknow-cli` ships each platform's Go
> binary as its own npm package (`@vibeknow/cli-darwin-arm64` and friends),
> listed in `optionalDependencies`. npm matches their `os`/`cpu` against your
> machine and installs only the one that fits — so the binary arrives over
> **whichever registry you already use**, public mirror or corporate proxy, with
> no second host needing to be reachable.
>
> If those were skipped (`--no-optional` and similar), `postinstall` falls back
> to downloading from GitHub Releases. On restricted networks that step tends to
> fail; reinstall with `--include=optional` instead.

**From source** (only needed if you want to build the Go binary yourself):

```bash
git clone https://github.com/vibeknow/cli.git
cd vibeknow-cli
make install
```

### Quick Start (AI Agent)

`vibeknow init` requires a TTY, so agents use the two-phase Device Flow instead. One human in the loop clicks a verification URL once; after that the agent runs unattended.

```bash
# 1. Install
npm install -g vibeknow-cli

# 2. Start a device-code flow WITHOUT blocking — prints JSON with the
#    verification_uri and device_code. The agent extracts both.
vibeknow auth login --no-wait
# Sample output:
# {
#   "device_code":      "dc_2913bcc...",
#   "user_code":        "UWWA-R8KS",
#   "verification_uri": "https://vibeknow.com/account/device",
#   "expires_in":       900,
#   "hint":             "请访问 https://... 并输入验证码 UWWA-R8KS"
# }

# 3. The agent shows the verification_uri to the human operator, who opens
#    it in a browser and approves. (One interaction per token, not per call.)

# 4. Resume polling with the device_code from step 2 — blocks until approved.
vibeknow auth login --device-code dc_2913bcc...

# 5. Token is now in the OS keychain; every subsequent command is authenticated.
vibeknow auth whoami
vibeknow auth status --output json   # machine-parseable login state
vibeknow create --from report.pdf
```

For CI / container environments with a pre-issued JWT, skip the Device Flow entirely — see [Environment Variables](#environment-variables) below.

**Hosts that spawn the CLI** (connector platforms, IDE integrations) want one
blocking command instead of the two-phase flow above:

```bash
vibeknow auth login --headless
# Prints {"user_code", "verification_uri", "expires_in", "hint"} to stdout
# immediately, then polls until authorized. No TTY, no Enter key, and it
# never opens a browser itself — the host does that after reading the URL.
```

The pending authorization is also written to the config dir, so a host that
kills the process before the user finishes authorizing does not lose the
login: the next `vibeknow auth status` completes the token exchange. While
one is outstanding, `auth status --output json` reports
`"pending_authorization": true` alongside `"authenticated": false`, which
distinguishes "waiting on the user" from "never logged in". `auth logout`
discards it.

## Agent Skills

The [`./skills/`](./skills/) directory ships three skills in the open
[Agent Skills](https://agentskills.io) format, compatible with 55+ AI agent
runtimes (Claude Code, Cursor, OpenCode, GitHub Copilot, Gemini CLI, and more).

| Skill | Description |
|-------|-------------|
| `vibeknow-core` | Profile setup, auth management, environment diagnostics, credential configuration |
| `vibeknow-create` | End-to-end video generation: `create` command, `video status/wait/download`, voice selection, async workflows |
| `vibeknow-doc` | Document upload (file + URL), parsing status polling, document retrieval |

### Install

```bash
npx skills add vibeknow/cli             # install all three to current project
npx skills add vibeknow/cli -g          # install globally (across projects)
npx skills add vibeknow/cli --skill vibeknow-create   # install one
```

Auto-detects locally installed agents and symlinks the skills into each agent's
skill directory. See [skills.sh](https://skills.sh) for the full option set.

Each skill follows the `SKILL.md` + `references/` structure with trigger/skip
conditions, command recipes, and exit-code-driven error handling.

## Authentication

vibeknow-cli supports three authentication paths, covering humans, AI Agents, and CI.

| Method | When to use | Storage |
|--------|-------------|---------|
| `vibeknow init` / `vibeknow auth login` | Interactive human setup (Device Flow via browser) | OS keychain |
| `vibeknow auth login --no-wait` then `--device-code <code>` | AI Agents — initiate without blocking, resume after human approval | OS keychain |
| `VIBEKNOW_TOKEN=<jwt>` env var | CI pipelines, containers, short-lived scripts (bypasses keychain) | None (per-invocation) |
| `vibeknow auth login --with-token` (reads PAT from stdin) | Scripted install with a pre-issued token | OS keychain |

```bash
# Who am I?
vibeknow auth whoami

# Token source, profile, expiry — add --output json for agents
vibeknow auth status
vibeknow auth status --output json

# Clear stored credentials
vibeknow auth logout
```

## Command Reference

### Hero Command

```bash
# Generate video from a URL
vibeknow create --from https://example.com/article

# List voices first, then pass either column (# or SPEECH_VOICE_ID)
vibeknow voice list
vibeknow create --from report.pdf --voice 1

# Reuse an already-uploaded document (doc_id needs its --kb-id)
vibeknow create --from <doc_id> --kb-id <kb_id>

# Text you already have, rather than a file — for a passage pasted into a chat
vibeknow create --text "Three common mistakes in knowledge management…"
vibeknow create --from - --script-lock <<'EOF'          # narrate it verbatim
Hello. Today I want to talk about one thing only…
EOF

# Custom prompt
vibeknow create --from data.csv --prompt "Create a 2-minute explainer video"

# Reuse a saved style bundle (~/.config/vibeknow/presets/brand.yaml)
vibeknow create --from deck.pdf --preset brand
vibeknow create --from deck.pdf --preset brand --aspect vertical   # your flag wins

# Async mode — submit, confirm the run started, then detach
vibeknow create --from doc.pdf --async

# Follow it in steps short enough for a caller that cannot block for minutes.
# Exit 6 with reason "wait_budget_expired" means keep going; exit 0 means done.
vibeknow video wait --for 90s --output json
vibeknow video wait          # reattaches to the most recent run
```

A **preset** is a YAML file holding the style flags you reuse — mode, aspect,
theme, voice, language, bgm, avatar placement. It supplies defaults only:
anything you also pass on the command line wins. It cannot carry `--export`,
`--yes` or `--confirm`, so opening someone else's preset can never approve a
charge. See [AGENTS.md](AGENTS.md) for the full contract.

`--async` returns once the backend confirms the run is live (seconds),
not once the video is done (minutes). The render continues server-side
after the CLI disconnects. An immediate rejection — bad input, no
credits — is reported by `--async` itself with a non-zero exit, so a
printed `task_id` means the run really started.

### Finding a run again

Every run is a `(task_id, session_id)` pair, and `create` records it
locally so you never have to keep it yourself:

```bash
vk jobs list                  # every run this machine started, newest first
vk jobs list --active         # only the ones still going
vk video wait                 # reattach to the most recent
vk video wait 42              # …or to a specific task, session looked up for you
vk jobs prune --terminal      # forget finished runs
```

Passing `--session-id` explicitly still works and always wins over the
ledger.

### Document Management

```bash
# Upload a file (creates KB, uploads, polls until ready)
vibeknow doc upload report.pdf

# Upload a URL
vibeknow doc upload --url https://example.com/page

# Check document status
vibeknow doc get --kb-id <kb_id> --doc-id <doc_id>
```

### Video Tasks

```bash
# Check task status
vibeknow video status <task_id>

# Stream progress (blocks until complete)
vibeknow video wait <task_id>

# Download rendered video (--dest is the file path; --output is the format)
vibeknow video download <task_id> --dest ./final.mp4
```

### Create a shareable video

```
$ vk create --from ./slides.pdf
…
share_url=https://vibeknow.com/share/tok_abc
hint: Render MP4 (several minutes, extra credits) — vk video export 42 --session-id sess_xxx
```

The pipeline finishes at the **preview** stage: the `share_url` plays
the finished video in a browser, ready to share.

### (Optional) Render an MP4 for download

```
$ vk video export 42 --session-id sess_xxx --yes
exporting: 72% — rendering frames
export complete

$ vk video download 42 --session-id sess_xxx
output=sess_xxx.mp4
```

A failed render exits **7**, not 0 — `vk video export` takes its exit
code from the state it waited for.

Or one-shot: `vk create --from ... --export --yes`.

### Fix one line without regenerating the video

```
$ vk video script 42                     # free: read what it says, shot by shot
[3] 结论  (4.5s)
增长主要来自海外市场。

$ vk video edit 42 --scene 3 --script "增长几乎全部来自海外市场。"
```

A wrong sentence used to mean a fresh `create` at full price. `video edit`
replaces one shot's narration and regenerates that shot; `--script-only`
regenerates the voice-over alone, for less.

It bills, so it goes through the same confirmation gate as `video export` —
and shows both the current and the proposed wording, because that diff is
what you are agreeing to. There is no undo, and the previously rendered MP4
is left in place: `video download` returns the old narration until you
export again.

### Make the subtitles readable

```
$ vk subtitle presets
#  NAME   LOOK
1  白字·黑底  text #ffffff · plate rgba(8,8,12,0.68) · no outline
2  白字·黑边  text #ffffff · no plate · outline 3px rgba(0,0,0,0.92) · Noto Sans SC 600

$ vk video set 42 --subtitle-preset 2 --subtitle-size 52
```

Subtitle readability is a combination, not a set of independent settings — an
outline behind a solid plate is invisible, a plate under an outline is muddy.
The presets are the looks the design team ships, each carrying every field it
needs. Individual flags still apply on top, so the call above means "that
look, but bigger".

`vk subtitle fonts` lists the families that are allowed, which previously had
no way to be discovered at all.

### Choose a video mode

```bash
vk create --from deck.pdf  --mode replica   # PPT/PDF page-by-page
vk create --from post.md   --mode image --pages 8   # one AI illustration per page
vk create --from notes.md  --mode handdraw  # hand-drawn animation (mid-run silence is normal)
vk create --from <src>     --aspect vertical --bgm
```

Style and language ride along with any pipeline mode:

```bash
vk theme list --mode image                   # browse a mode's style catalog
vk create --from post.md --mode image --theme <theme_id>
vk create --from post.md --language en-US    # script + narration language
```

### Add a talking-head avatar

```bash
vk avatar list                               # sys_<id> presets + your trained ua_<id>
vk create --from deck.pdf --avatar sys_7 --voice <its VOICE_ID>
vk create --from deck.pdf --avatar ua_12 --avatar-position bottom-right --avatar-size 300
```

Not available with `--mode handdraw` or `--engine agent`. If `video
export` is refused because avatar scenes failed, run
`vk video avatar-retry` (free) and export again once they finish.

`--script-lock` narrates the document verbatim instead of writing a
script from it, and stacks on top of any mode (it replaced the old
`--mode script`, which still works but warns):

```bash
vk create --from talk.docx --script-lock                 # verbatim narration
vk create --from talk.docx --mode image --script-lock    # …illustrated per page
```

### Pick a generation engine (optional)

```bash
vk create --from <src> --engine agent       # v=2 agent engine (frontend toggle parity)
vk create --from <src> --engine pipeline    # v=3 pipeline (default)
```

### Clean up accumulated knowledgebases

```bash
vk kb list --output json --size 5             # peek at what's there
vk kb prune --pattern 'vibeknow-cli-*'        # dry-run (default)
vk kb prune --pattern 'vibeknow-cli-*' --yes  # actually delete
```

### Voice Templates

```bash
# List all available voices
vibeknow voice list
```

### Profile Management

```bash
# Add a dev profile with local endpoint overrides
vibeknow profile add dev \
  --endpoint-figlens http://localhost:<port> \
  --credential-ref vibeknow.dev \
  --trust dev --is-production=false

# Switch profiles
vibeknow profile use prod

# Show profile details
vibeknow profile show

# List all profiles
vibeknow profile list
```

### Raw API Access

```bash
# Call any backend endpoint directly
vibeknow api call --service <service> --method GET --path /v1/<resource>

# POST with JSON body
vibeknow api call --service <service> --method POST --path /v1/<resource> --body '{"key":"value"}'

# POST with body from file
vibeknow api call --service <service> --method POST --path /v1/<resource> --body @request.json
```

## Advanced Usage

### Output Formats

```bash
# Default: human-friendly text (auto-detected in TTY)
vibeknow voice list

# JSON output
vibeknow voice list --output json

# Pipe-friendly (non-TTY auto-selects json)
vibeknow voice list --output json | jq '.templates[0].name'
```

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `VIBEKNOW_TOKEN` | JWT token for all services (highest priority credential source) |
| `VIBEKNOW_CONFIG_HOME` | Override config directory (default: `~/.config/vibeknow`) |
| `VIBEKNOW_TRACE` | Set to `1` to display trace IDs for debugging |
| `VIBEKNOW_DEBUG` | Set to `1` for verbose logging (use with care) |

### Diagnostics

```bash
# Full environment check (config, credentials, endpoint reachability)
vibeknow doctor
```

## Architecture

vibeknow-cli uses a **multi-endpoint** architecture — the CLI connects to multiple backend services, each responsible for a specific domain (auth, documents, video pipeline, etc.). Services are configured via profiles, allowing per-environment endpoint overrides.

The `create` command orchestrates the full pipeline: document upload → video generation (SSE) → export & download.

## Contributing

Issues and pull requests are welcome at [github.com/vibeknow/cli](https://github.com/vibeknow/cli).

```bash
# Development setup
git clone https://github.com/vibeknow/cli.git
cd vibeknow-cli
make build    # build binary
make test     # run all tests (with race detector)
make lint     # go vet
```

## License

[MIT](./LICENSE)

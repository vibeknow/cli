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
- **Real-Time Stage Progress** — SSE streaming with 6-stage progress tracking (parse → outline → tts → render → publish → suggest), both for human progress bars and machine-readable NDJSON
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
#   "verification_uri": "https://beta.lab.shiliu.chat/account/device",
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

## Agent Skills

| Skill | Description |
|-------|-------------|
| `vibeknow-core` | Profile setup, auth management, environment diagnostics, credential configuration |
| `vibeknow-create` | End-to-end video generation: `create` command, `video status/wait/download`, voice selection, async workflows |
| `vibeknow-doc` | Document upload (file + URL), parsing status polling, document retrieval |

Skills are located in [`./skills/`](./skills/) and follow the `SKILL.md` + `references/` structure. Each skill includes trigger/skip conditions, command recipes, and error handling guides.

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

# List available voice IDs first, then pass one via --voice
vibeknow voice list
vibeknow create --from report.pdf --voice <voice-id-from-above>

# Custom prompt
vibeknow create --from data.csv --prompt "Create a 2-minute explainer video"

# Async mode — get task ID immediately, check later
vibeknow create --from doc.pdf --async
vibeknow video wait <task_id> --session-id <session_id>
```

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
vibeknow video wait <task_id> --session-id <session_id>

# Download rendered video
vibeknow video download <task_id> --session-id <session_id>
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

Or one-shot: `vk create --from ... --export --yes`.

### Choose a video mode

```bash
vk create --from deck.pdf  --mode replica   # PPT/PDF page-by-page
vk create --from talk.docx --mode script    # narrate the doc verbatim
vk create --from <src>     --aspect vertical --bgm
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
vibeknow voice list | jq '.list[0].name'
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

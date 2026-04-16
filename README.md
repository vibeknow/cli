# vibeknow-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.25-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/@vibeknow/cli.svg)](https://www.npmjs.com/package/@vibeknow/cli)

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
| **Auth** | Token-based auth (env var), whoami, credential status, logout; Device Flow planned for v1 |
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

#### Install

**Option 1 — From npm (recommended):**

```bash
npm install -g @vibeknow/cli
```

**Option 2 — From source:**

```bash
git clone https://github.com/vibeknow/cli.git
cd vibeknow-cli
make install
```

#### Configure

```bash
# 1. Add a profile (get endpoints from your VibeKnow dashboard)
vibeknow profile add prod \
  --endpoint-account <account-endpoint> \
  --endpoint-vectoria <vectoria-endpoint> \
  --endpoint-figlens <figlens-endpoint> \
  --endpoint-vibeknow <vibeknow-endpoint> \
  --credential-ref vibeknow.prod

# 2. Set your token (from web dashboard)
export VIBEKNOW_TOKEN="your-jwt-token-here"

# 3. Set document service API key
export VECTORIA_API_KEY="your-api-key"

# 4. Verify
vibeknow auth whoami
vibeknow doctor
```

#### Create Your First Video

```bash
vibeknow create --from https://example.com/article --voice t260312180132IV37e611
```

### Quick Start (AI Agent)

> Run each step and verify before proceeding.

**Step 1 — Install**

```bash
npm install -g @vibeknow/cli
```

**Step 2 — Configure profile** (get endpoints from your VibeKnow dashboard)

```bash
vibeknow profile add prod \
  --endpoint-account <account-endpoint> \
  --endpoint-vectoria <vectoria-endpoint> \
  --endpoint-figlens <figlens-endpoint> \
  --endpoint-vibeknow <vibeknow-endpoint> \
  --credential-ref vibeknow.prod
```

**Step 3 — Set credentials** (obtain from web dashboard or CI secrets)

```bash
export VIBEKNOW_TOKEN="<jwt>"
export VECTORIA_API_KEY="<key>"
```

**Step 4 — Verify**

```bash
vibeknow auth whoami
vibeknow voice list
```

## Agent Skills

| Skill | Description |
|-------|-------------|
| `vibeknow-core` | Profile setup, auth management, environment diagnostics, credential configuration |
| `vibeknow-create` | End-to-end video generation: `create` command, `video status/wait/download`, voice selection, async workflows |
| `vibeknow-doc` | Document upload (file + URL), parsing status polling, document retrieval |

Skills are located in [`./skills/`](./skills/) and follow the `SKILL.md` + `references/` structure. Each skill includes trigger/skip conditions, command recipes, and error handling guides.

## Authentication

vibeknow-cli currently supports token-based authentication via environment variables:

| Method | Usage |
|--------|-------|
| `VIBEKNOW_TOKEN` env var | JWT token for VibeKnow services |
| `VECTORIA_API_KEY` env var | API key for the document service |
| Keychain storage | Token persisted in OS keychain via `credential_ref` in profile |

```bash
# Check who you're logged in as
vibeknow auth whoami

# Show credential source (env / keychain / none)
vibeknow auth status

# Clear stored credentials
vibeknow auth logout
```

> **Coming in v1:** Interactive `auth login` via OAuth Device Flow + Personal Access Tokens (PAT).

## Command Reference

### Hero Command

```bash
# Generate video from a URL
vibeknow create --from https://example.com/article

# Generate from a local file with specific voice
vibeknow create --from report.pdf --voice t260312180132IV37e611

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
| `VIBEKNOW_TOKEN` | JWT token (highest priority credential source) |
| `VECTORIA_API_KEY` | Document service API key |
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

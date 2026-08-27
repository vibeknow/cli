---
name: vibeknow-core
description: "vibeknow CLI setup, authentication, profile management, and diagnostics. Use when: first-time setup, auth errors, switching environments, diagnosing connection issues."
version: 0.9.1
emoji: "🔧"
homepage: https://github.com/vibeknow/cli
allowed-tools: Bash(vibeknow:*)
metadata:
  openclaw:
    requires:
      bins: ["vibeknow"]
    install:
      - kind: node
        package: vibeknow-cli
        bins: [vibeknow]
    primaryEnv: VIBEKNOW_TOKEN
    envVars:
      - name: VIBEKNOW_TOKEN
        required: false
        description: "API token. Optional — if unset, the CLI uses OS keychain or encrypted file via `vibeknow auth login`. When set, takes priority over stored credentials."
      - name: VIBEKNOW_CONFIG_HOME
        required: false
        description: "Override the config directory (default: OS user config dir). Useful for portable/test setups."
      - name: VIBEKNOW_LANG
        required: false
        description: "Force CLI output language (e.g. `en`, `zh`). Falls back to `LANG`."
---

# vibeknow-core

## TRIGGER

- First-time vibeknow setup or onboarding
- Authentication errors or credential issues
- Switching between environments (dev/staging/prod)
- Diagnosing connection or endpoint problems
- Managing profiles or config values

## SKIP

- Video generation, status, download → use **vibeknow-create**
- Document upload or status → use **vibeknow-doc**

## Core Concepts

- **Profile**: A named environment config (endpoints + credential). Supports dev/staging/prod switching via `vibeknow profile use <name>`.
- **Credential priority**: `VIBEKNOW_TOKEN` env var > OS keychain > encrypted file. The env var overrides everything and cannot be cleared by `logout`.
- **Endpoint trust boundary**: Profiles pointing to localhost or non-official domains must set `--trust dev --is-production=false`, or the CLI refuses to start.
- **Doctor**: Runs endpoint reachability checks and token validation. Use it first when anything seems wrong.

## Quick Reference

| Command | Description |
|---------|-------------|
| `vibeknow auth status` | Show credential source and active profile |
| `vibeknow auth whoami` | Print current authenticated user |
| `vibeknow auth logout` | Clear stored credential for current profile |
| `vibeknow profile add NAME` | Add a new profile |
| `vibeknow profile use NAME` | Switch active profile |
| `vibeknow profile list` | List all profiles |
| `vibeknow profile show [NAME]` | Show profile details (default: current) |
| `vibeknow profile remove NAME` | Delete a profile |
| `vibeknow config get KEY` | Read a config value |
| `vibeknow config set KEY VALUE` | Write a config value |
| `vibeknow config list` | List all config keys |
| `vibeknow doctor` | Diagnose local setup and endpoint reachability |

For full flags and output examples, see [commands.md](references/commands.md).

## Common Tasks

### Check if vibeknow is working

```bash
vibeknow doctor
vibeknow auth status
```

### Set up a new dev environment

```bash
vibeknow profile add dev \
  --endpoint-figlens http://localhost:20067 \
  --trust dev \
  --is-production=false \
  --credential-ref vibeknow.dev
vibeknow profile use dev
vibeknow doctor
```

### Switch between environments

```bash
vibeknow profile list              # see available profiles
vibeknow profile use prod          # switch to prod
vibeknow auth status               # verify credentials
```

### Diagnose auth failure

```bash
vibeknow auth status               # check credential source
vibeknow auth whoami               # verify identity
vibeknow doctor                    # check endpoint reachability
```

## Exit Code Handling

| Exit | Meaning | Action |
|------|---------|--------|
| 0 | Success | — |
| 1 | General error | Check stderr message |
| 2 | Invalid arguments | Fix command syntax |
| 3 | Auth error | Run `vibeknow auth status`, check credential source |
| 130 | User interrupt (SIGINT) | — |

For full exit code and error code reference, see [errors.md](references/errors.md).

## Output Formats

All core commands support `--output text|json|ndjson`:

```bash
vibeknow auth status --output json    # structured output
vibeknow profile list --output json   # machine-readable profile list
```

## References

- [commands.md](references/commands.md) — Full flag reference for all 12 subcommands
- [errors.md](references/errors.md) — Exit codes, error codes, and Error Object schema

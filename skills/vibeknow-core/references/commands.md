# vibeknow-core Command Reference

## Global Flags

These flags are available on all commands:

| Flag | Description |
|------|-------------|
| `--output string` | Output format: `text\|json\|ndjson` (auto-selects based on TTY) |
| `--profile string` | Override active profile for this command |
| `--verbose` | Emit request/response summaries (credentials redacted) |

---

## auth status

Show credential source and active profile.

```
vibeknow auth status [flags]
```

No command-specific flags.

**Text output example:**
```
Profile:    dev
Source:     keychain (vibeknow.dev)
Endpoints:
  account:  https://account.vibeknow.com
  figlens:  http://localhost:20067
  vectoria: https://vectoria.vibeknow.com
  vibeknow: https://api.vibeknow.com
```

**JSON output example:**
```json
{"profile":"dev","source":"keychain","credential_ref":"vibeknow.dev"}
```

## auth whoami

Print the current authenticated user.

```
vibeknow auth whoami [flags]
```

No command-specific flags.

## auth logout

Clear stored credential for the current profile. Only clears persistent storage (keychain or file). Cannot clear `VIBEKNOW_TOKEN` env var — prints a warning instead.

```
vibeknow auth logout [flags]
```

No command-specific flags.

## profile add

Add a new profile.

```
vibeknow profile add NAME [flags]
```

| Flag | Description |
|------|-------------|
| `--credential-ref string` | Keychain entry name or `file://` path **(required)** |
| `--endpoint-account string` | go-account URL override (optional; default uses cloud) |
| `--endpoint-figlens string` | go-figlens URL override |
| `--endpoint-vectoria string` | go-vectoria URL override |
| `--endpoint-vibeknow string` | go-vibeknow URL override |
| `--default-project string` | Optional default project name |
| `--trust string` | `user\|dev` (default: `"user"`) |
| `--is-production` | Treat as production (default: `true`). Must be `false` for non-prod endpoint overrides |
| `--api-endpoint string` | **DEPRECATED**: alias for `--endpoint-vibeknow` |

**Trust boundary rule:** If any endpoint points to `localhost` / `127.0.0.1` / non-official domain, the profile must set `--trust dev --is-production=false`, otherwise the CLI refuses to load it.

## profile use

Switch active profile.

```
vibeknow profile use NAME [flags]
```

No command-specific flags.

## profile list

List all profiles.

```
vibeknow profile list [flags]
```

No command-specific flags.

## profile show

Show profile details. Defaults to current profile if NAME is omitted.

```
vibeknow profile show [NAME] [flags]
```

No command-specific flags.

## profile remove

Delete a profile.

```
vibeknow profile remove NAME [flags]
```

No command-specific flags.

## config get

Read a config value.

```
vibeknow config get KEY [flags]
```

No command-specific flags.

## config set

Write a config value.

```
vibeknow config set KEY VALUE [flags]
```

No command-specific flags.

## config list

List all config keys.

```
vibeknow config list [flags]
```

No command-specific flags.

## doctor

Diagnose local setup and endpoint reachability. Checks: endpoint connectivity, token validity, CLI version.

```
vibeknow doctor [flags]
```

No command-specific flags.

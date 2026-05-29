---
title: CLAUDE.md
---
# wakethemup

:-: var key="project.description"

## Architecture

Four packages, each with a single responsibility:

- **config** (`internal/config/`): TOML parsing and validation. `Parse` does strict schema checking (unknown keys rejected). `Validate` does runtime checks: calendar expression validation via `systemd-analyze`, exec path resolution, filesystem checks on `working_directory` and `env_file`.
- **systemd** (`internal/systemd/`): All systemd interaction -- detecting systemd, daemon-reload, enable/disable timers, linger management, timer listing, and status queries. Everything goes through the `Commander` interface.
- **unit** (`internal/unit/`): Unit file generation from Go templates (`service.tmpl`, `timer.tmpl`), systemd-specific escaping, and atomic file writes.
- **cmd/wake** (`cmd/wake/`): strictcli-based CLI with 4 commands (`install`, `remove`, `list`, `status`) and a check system for environment diagnostics.

## Commander interface

All systemd interaction goes through `systemd.Commander`:

```go
type Commander interface {
    Run(ctx context.Context, args ...string) (stdout string, stderr string, err error)
    LookPath(name string) (string, error)
}
```

`ExecCommander` is the production implementation (shells out to system binaries with a 30-second default timeout). `MockCommander` is the test implementation (records calls, returns predefined responses matched by args).

This pattern exists for testability: unit tests use `MockCommander` to verify systemctl invocations without a running systemd.

## Escaping rules

Systemd unit files require context-dependent escaping. The rules per directive:

| Directive | Context | Escapes | Rationale |
| --- | --- | --- | --- |
| `ExecStart` | direct (no shell) | `%` -> `%%`, `$` -> `$$` | Prevent specifier expansion and variable substitution |
| `ExecStart` | shell (`/bin/sh -c`) | `%` -> `%%` | `$` left alone for shell variable use; quotes handled by `resolveExec` |
| `Environment` | always | `%` -> `%%`, value always double-quoted | Quoting prevents systemd splitting; specifiers escaped |
| `Description`, `WorkingDirectory`, `EnvironmentFile`, `X-WakeConfig` | always | `%` -> `%%` | Only specifier expansion is a concern |

Functions: `EscapeSpecifiers` (% only), `EscapeExecDirect` (% and $), `EscapeExecShell` (% only), `QuoteEnvAssignment` (% escaped, double-quoted).

## TOML schema version

The config schema is versioned (`version = 1`). Key contracts:

- `Parse` rejects any `version` value other than `1`
- Unknown keys at any level cause a hard error (strict rejection via known-key maps)
- All schema changes require a version bump -- no backward-compatible silent additions

## Modules

:-: list-modules path="."

## Commands

:-: table-commands path="cmd/wake"

## Testing

- Unit tests use `MockCommander` to verify systemctl/loginctl/systemd-analyze invocations
- Config parsing tests cover: valid configs, missing required fields, unknown keys, invalid types, name validation
- Unit generation tests cover: escaping edge cases, template rendering, atomic file writes
- Build: `go build ./cmd/wake` produces a `wake` binary

## Release workflow

This project uses [rlsbl](https://github.com/smm-h/rlsbl) for release orchestration.

- Run `rlsbl release init` to scaffold the release file, set the bump type, then `rlsbl release run`
- CI handles publishing automatically via the publish workflow
- Never publish manually -- always use `rlsbl release run`

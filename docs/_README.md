---
title: README.md
---
# wakethemup

:-: var key="project.description"

## Install

From source:

```
go install github.com/smm-h/wakethemup/cmd/wake@latest
```

Or download a binary from [GitHub Releases](https://github.com/smm-h/wakethemup/releases).

## Quick start

Create a TOML config file (e.g. `backup.toml`):

```toml
version = 1

[schedule]
name = "backup"
description = "Daily database backup"
calendar = "*-*-* 03:00:00"

[command]
exec = "/usr/local/bin/backup.sh"
working_directory = "/var/backups"
```

Then use the CLI:

```
wake install backup.toml   # create and start the systemd timer
wake list                   # show all installed schedules
wake status backup          # show timer/service status and recent logs
wake remove backup          # stop and delete the schedule
```

## TOML schema

The config file version is currently `1`. Unknown keys are rejected.

### Top-level

| Key | Type | Required | Description |
| --- | --- | --- | --- |
| `version` | int | yes | Schema version, must be `1` |

### `[schedule]`

| Key | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Schedule identifier (alphanumeric, hyphens, underscores; max 64 chars). Used as the systemd unit name (`wake-<name>.timer`). |
| `description` | string | yes | Human-readable description, used in the unit's `Description=` field |
| `calendar` | string | yes | systemd calendar expression (validated via `systemd-analyze calendar`). Examples: `daily`, `*-*-* 03:00:00`, `Mon *-*-* 09:00:00`. |

### `[command]`

| Key | Type | Required | Description |
| --- | --- | --- | --- |
| `exec` | string | yes | Command to run. Binary names are resolved to absolute paths via `$PATH`. |
| `working_directory` | string | no | Working directory for the command. Must exist at install time. |
| `env_file` | string | no | Path to an environment file loaded by systemd (`EnvironmentFile=`). Must exist at install time. |

### `[env]`

Optional section. Each key is an environment variable name (must match `^[A-Za-z_][A-Za-z0-9_]*$`), and each value must be a string. Variables are set via systemd `Environment=` directives.

```toml
[env]
DATABASE_URL = "postgres://localhost/mydb"
LOG_LEVEL = "info"
```

## Commands

:-: table-commands path="cmd/wake"

## Behavior

- **Persistent timers.** All timers are created with `Persistent=true`. If the machine was off when a timer should have fired, systemd runs one catch-up execution on boot.
- **Shell detection.** Commands containing shell metacharacters (`|`, `&&`, `||`, `;`, `>`, `<`, `` ` ``, `$(`, `&`) are automatically wrapped in `/bin/sh -c "..."`. Commands without metacharacters are resolved to absolute binary paths and executed directly.
- **Linger.** On first install, `loginctl enable-linger` is run automatically so that user timers fire without an active login session.
- **Atomic writes.** Unit files are written to a temp file and renamed into place, preventing partial writes.
- **Ownership verification.** `wake remove` checks for the `X-WakeConfig=` marker in the service unit and refuses to remove units not created by wake.

## Packages

:-: list-modules path="."

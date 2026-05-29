package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockCommander implements Commander for testing.
type mockCommander struct {
	// runFunc is called for each Run invocation. If nil, returns success.
	runFunc func(ctx context.Context, args ...string) (string, string, error)
}

func (m *mockCommander) Run(ctx context.Context, args ...string) (string, string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, args...)
	}
	return "", "", nil
}

// --- Parse tests ---

func TestParse_ValidMinimal(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "my-task"
description = "A test task"
calendar = "daily"

[command]
exec = "/usr/bin/echo hello"
`
	cfg, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	if cfg.Schedule.Name != "my-task" {
		t.Errorf("name = %q, want %q", cfg.Schedule.Name, "my-task")
	}
	if cfg.Schedule.Description != "A test task" {
		t.Errorf("description = %q, want %q", cfg.Schedule.Description, "A test task")
	}
	if cfg.Schedule.Calendar != "daily" {
		t.Errorf("calendar = %q, want %q", cfg.Schedule.Calendar, "daily")
	}
	if cfg.Command.Exec != "/usr/bin/echo hello" {
		t.Errorf("exec = %q, want %q", cfg.Command.Exec, "/usr/bin/echo hello")
	}
	if cfg.Env != nil {
		t.Errorf("env = %v, want nil", cfg.Env)
	}
}

func TestParse_ValidFull(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "backup_db"
description = "Daily database backup"
calendar = "*-*-* 02:00:00"

[command]
exec = "/usr/local/bin/backup.sh"
working_directory = "/var/backups"
env_file = "/etc/backup.env"

[env]
DB_HOST = "localhost"
DB_PORT = "5432"
BACKUP_DIR = "/var/backups"
`
	cfg, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	if cfg.Schedule.Name != "backup_db" {
		t.Errorf("name = %q, want %q", cfg.Schedule.Name, "backup_db")
	}
	if cfg.Command.WorkingDirectory != "/var/backups" {
		t.Errorf("working_directory = %q, want %q", cfg.Command.WorkingDirectory, "/var/backups")
	}
	if cfg.Command.EnvFile != "/etc/backup.env" {
		t.Errorf("env_file = %q, want %q", cfg.Command.EnvFile, "/etc/backup.env")
	}
	if len(cfg.Env) != 3 {
		t.Errorf("env has %d entries, want 3", len(cfg.Env))
	}
	if cfg.Env["DB_HOST"] != "localhost" {
		t.Errorf("env[DB_HOST] = %q, want %q", cfg.Env["DB_HOST"], "localhost")
	}
	if cfg.Env["DB_PORT"] != "5432" {
		t.Errorf("env[DB_PORT] = %q, want %q", cfg.Env["DB_PORT"], "5432")
	}
}

func TestParse_UnknownTopLevelKey(t *testing.T) {
	toml := `
version = 1
bogus = "oops"

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for unknown top-level key")
	}
	if !strings.Contains(err.Error(), "unknown top-level key") || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error = %q, want mention of unknown top-level key 'bogus'", err)
	}
}

func TestParse_UnknownScheduleKey(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"
timezone = "UTC"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for unknown key in [schedule]")
	}
	if !strings.Contains(err.Error(), "unknown key in [schedule]") || !strings.Contains(err.Error(), "timezone") {
		t.Errorf("error = %q, want mention of unknown key 'timezone' in [schedule]", err)
	}
}

func TestParse_UnknownCommandKey(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
user = "root"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for unknown key in [command]")
	}
	if !strings.Contains(err.Error(), "unknown key in [command]") || !strings.Contains(err.Error(), "user") {
		t.Errorf("error = %q, want mention of unknown key 'user' in [command]", err)
	}
}

func TestParse_MissingVersion(t *testing.T) {
	toml := `
[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for missing version")
	}
	if !strings.Contains(err.Error(), "missing required key: version") {
		t.Errorf("error = %q, want mention of missing version", err)
	}
}

func TestParse_WrongVersion(t *testing.T) {
	toml := `
version = 2

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for wrong version")
	}
	if !strings.Contains(err.Error(), "unsupported version: 2") {
		t.Errorf("error = %q, want mention of unsupported version 2", err)
	}
}

func TestParse_VersionAsString(t *testing.T) {
	toml := `
version = "1"

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for version as string")
	}
	if !strings.Contains(err.Error(), "version must be an integer") {
		t.Errorf("error = %q, want mention of type mismatch", err)
	}
}

func TestParse_MissingName(t *testing.T) {
	toml := `
version = 1

[schedule]
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "schedule.name") {
		t.Errorf("error = %q, want mention of schedule.name", err)
	}
}

func TestParse_MissingDescription(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for missing description")
	}
	if !strings.Contains(err.Error(), "schedule.description") {
		t.Errorf("error = %q, want mention of schedule.description", err)
	}
}

func TestParse_MissingCalendar(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for missing calendar")
	}
	if !strings.Contains(err.Error(), "schedule.calendar") {
		t.Errorf("error = %q, want mention of schedule.calendar", err)
	}
}

func TestParse_MissingExec(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
working_directory = "/tmp"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for missing exec")
	}
	if !strings.Contains(err.Error(), "command.exec") {
		t.Errorf("error = %q, want mention of command.exec", err)
	}
}

func TestParse_NameTooLong(t *testing.T) {
	name := strings.Repeat("a", 65)
	toml := fmt.Sprintf(`
version = 1

[schedule]
name = "%s"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`, name)
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for name too long")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("error = %q, want mention of name being too long", err)
	}
}

func TestParse_NameWithSpaces(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "has spaces"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for name with spaces")
	}
	if !strings.Contains(err.Error(), "invalid characters") {
		t.Errorf("error = %q, want mention of invalid characters", err)
	}
}

func TestParse_NameWithDots(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "has.dots"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for name with dots")
	}
	if !strings.Contains(err.Error(), "invalid characters") {
		t.Errorf("error = %q, want mention of invalid characters", err)
	}
}

func TestParse_NameWithHyphensAndUnderscores(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "my-task_v2"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`
	cfg, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Schedule.Name != "my-task_v2" {
		t.Errorf("name = %q, want %q", cfg.Schedule.Name, "my-task_v2")
	}
}

func TestParse_NameExactly64Chars(t *testing.T) {
	name := strings.Repeat("a", 64)
	toml := fmt.Sprintf(`
version = 1

[schedule]
name = "%s"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"
`, name)
	cfg, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Schedule.Name) != 64 {
		t.Errorf("name length = %d, want 64", len(cfg.Schedule.Name))
	}
}

func TestParse_EnvValueAsInteger(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"

[env]
PORT = 8080
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for env value as integer")
	}
	if !strings.Contains(err.Error(), "must be a string") {
		t.Errorf("error = %q, want mention of string requirement", err)
	}
}

func TestParse_EnvValueAsBoolean(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"

[env]
DEBUG = true
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for env value as boolean")
	}
	if !strings.Contains(err.Error(), "must be a string") {
		t.Errorf("error = %q, want mention of string requirement", err)
	}
}

func TestParse_EnvKeyWithDots(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"

[env]
"my.var" = "value"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for env key with dots")
	}
	if !strings.Contains(err.Error(), "invalid env var name") {
		t.Errorf("error = %q, want mention of invalid env var name", err)
	}
}

func TestParse_EnvKeyStartingWithDigit(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"

[env]
"1BAD" = "value"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for env key starting with digit")
	}
	if !strings.Contains(err.Error(), "invalid env var name") {
		t.Errorf("error = %q, want mention of invalid env var name", err)
	}
}

func TestParse_ValidPOSIXEnvKeys(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"

[env]
HOME = "/home/user"
_PRIVATE = "secret"
MY_VAR_123 = "value"
x = "short"
`
	cfg, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Env) != 4 {
		t.Errorf("env has %d entries, want 4", len(cfg.Env))
	}
}

func TestParse_MissingScheduleSection(t *testing.T) {
	toml := `
version = 1

[command]
exec = "/bin/true"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for missing [schedule] section")
	}
	if !strings.Contains(err.Error(), "missing required section: [schedule]") {
		t.Errorf("error = %q, want mention of missing [schedule]", err)
	}
}

func TestParse_MissingCommandSection(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for missing [command] section")
	}
	if !strings.Contains(err.Error(), "missing required section: [command]") {
		t.Errorf("error = %q, want mention of missing [command]", err)
	}
}

func TestParse_EmptyExec(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = ""
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for empty exec")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("error = %q, want mention of empty exec", err)
	}
}

func TestParse_UnknownTopLevelTable(t *testing.T) {
	toml := `
version = 1

[schedule]
name = "x"
description = "x"
calendar = "daily"

[command]
exec = "/bin/true"

[logging]
level = "debug"
`
	_, err := Parse([]byte(toml))
	if err == nil {
		t.Fatal("expected error for unknown top-level table [logging]")
	}
	if !strings.Contains(err.Error(), "unknown top-level key") || !strings.Contains(err.Error(), "logging") {
		t.Errorf("error = %q, want mention of unknown top-level key 'logging'", err)
	}
}

// --- Validate tests ---

func TestValidate_ValidCalendar(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "/bin/echo hello",
		},
	}

	cmd := &mockCommander{
		runFunc: func(ctx context.Context, args ...string) (string, string, error) {
			// systemd-analyze calendar daily succeeds
			return "  Original form: daily\n  Normalized form: *-*-* 00:00:00\n", "", nil
		},
	}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidCalendar(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "notacalendar",
		},
		Command: CommandConfig{
			Exec: "/bin/echo hello",
		},
	}

	cmd := &mockCommander{
		runFunc: func(ctx context.Context, args ...string) (string, string, error) {
			return "", "Failed to parse calendar expression", fmt.Errorf("exit status 1")
		},
	}

	err := Validate(cfg, cmd)
	if err == nil {
		t.Fatal("expected error for invalid calendar expression")
	}
	if !strings.Contains(err.Error(), "invalid calendar expression") {
		t.Errorf("error = %q, want mention of invalid calendar expression", err)
	}
}

func TestValidate_ExecNoShellMetachars(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "echo hello world",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	// echo should be found via LookPath on most systems
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Command.IsShell {
		t.Error("IsShell = true, want false for simple command")
	}
	if !filepath.IsAbs(strings.Fields(cfg.Command.ResolvedExec)[0]) {
		t.Errorf("ResolvedExec first token is not absolute: %q", cfg.Command.ResolvedExec)
	}
	// Should preserve arguments.
	if !strings.HasSuffix(cfg.Command.ResolvedExec, " hello world") {
		t.Errorf("ResolvedExec = %q, want to end with ' hello world'", cfg.Command.ResolvedExec)
	}
}

func TestValidate_ExecWithPipe(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "cat /var/log/syslog | grep error",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for pipe")
	}
	if !strings.HasPrefix(cfg.Command.ResolvedExec, "/bin/sh -c ") {
		t.Errorf("ResolvedExec = %q, want /bin/sh -c prefix", cfg.Command.ResolvedExec)
	}
}

func TestValidate_ExecWithAnd(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "make build && make test",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for &&")
	}
}

func TestValidate_ExecWithSubshell(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "echo $(date)",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for $()")
	}
}

func TestValidate_ExecBinaryNotFound(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "nonexistent_binary_xyz_12345",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "exec binary not found") {
		t.Errorf("error = %q, want mention of binary not found", err)
	}
}

func TestValidate_WorkingDirectoryExists(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec:             "/bin/echo hello",
			WorkingDirectory: dir,
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_WorkingDirectoryNotExists(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec:             "/bin/echo hello",
			WorkingDirectory: "/nonexistent/path/xyz",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err == nil {
		t.Fatal("expected error for nonexistent working_directory")
	}
	if !strings.Contains(err.Error(), "working_directory does not exist") {
		t.Errorf("error = %q, want mention of working_directory not existing", err)
	}
}

func TestValidate_EnvFileExists(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "test.env")
	if err := os.WriteFile(envFile, []byte("KEY=val\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec:    "/bin/echo hello",
			EnvFile: envFile,
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_EnvFileNotExists(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec:    "/bin/echo hello",
			EnvFile: "/nonexistent/path/test.env",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err == nil {
		t.Fatal("expected error for nonexistent env_file")
	}
	if !strings.Contains(err.Error(), "env_file does not exist") {
		t.Errorf("error = %q, want mention of env_file not existing", err)
	}
}

func TestValidate_EnvFileIsDirectory(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec:    "/bin/echo hello",
			EnvFile: dir,
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err == nil {
		t.Fatal("expected error for env_file being a directory")
	}
	if !strings.Contains(err.Error(), "env_file is a directory") {
		t.Errorf("error = %q, want mention of env_file being a directory", err)
	}
}

func TestValidate_WorkingDirectoryIsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec:             "/bin/echo hello",
			WorkingDirectory: file,
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err == nil {
		t.Fatal("expected error for working_directory being a file")
	}
	if !strings.Contains(err.Error(), "working_directory is not a directory") {
		t.Errorf("error = %q, want mention of not a directory", err)
	}
}

func TestValidate_ExecWithRedirect(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "echo hello > /tmp/out.txt",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for >")
	}
}

func TestValidate_ExecWithInputRedirect(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "wc -l < /tmp/input.txt",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for <")
	}
}

func TestValidate_ExecWithSemicolon(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "cmd1; cmd2",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for ;")
	}
}

func TestValidate_ExecWithBacktick(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "echo `date`",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for backtick")
	}
}

func TestValidate_ExecWithBackgroundAmpersand(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "sleep 10 &",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for &")
	}
}

func TestValidate_ExecWithOr(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "cmd1 || cmd2",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Command.IsShell {
		t.Error("IsShell = false, want true for ||")
	}
}

func TestValidate_ExecAbsolutePath(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Schedule: ScheduleConfig{
			Name:        "test",
			Description: "test",
			Calendar:    "daily",
		},
		Command: CommandConfig{
			Exec: "/bin/echo hello",
		},
	}

	cmd := &mockCommander{}

	err := Validate(cfg, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Command.IsShell {
		t.Error("IsShell = true, want false for absolute path")
	}
	if cfg.Command.ResolvedExec != "/bin/echo hello" {
		t.Errorf("ResolvedExec = %q, want %q", cfg.Command.ResolvedExec, "/bin/echo hello")
	}
}

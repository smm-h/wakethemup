// Package config handles TOML schedule configuration parsing and validation.
package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	tomledit "github.com/smm-h/go-toml-edit"
)

// Config is the top-level parsed schedule configuration.
type Config struct {
	Version  int
	Schedule ScheduleConfig
	Command  CommandConfig
	Env      map[string]string // may be nil if [env] section absent
}

// ScheduleConfig describes the systemd timer schedule.
type ScheduleConfig struct {
	Name        string
	Description string
	Calendar    string
}

// CommandConfig describes the command to execute on schedule.
type CommandConfig struct {
	Exec             string
	WorkingDirectory string // optional, may be empty
	EnvFile          string // optional, may be empty
	IsShell          bool   // true if exec contains shell metacharacters
	ResolvedExec     string // absolute path version of exec (or sh -c wrapped)
}

// Commander runs external commands. Matches internal/systemd.Commander.
type Commander interface {
	Run(ctx context.Context, args ...string) (stdout string, stderr string, err error)
}

var (
	nameRegexp   = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	envKeyRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Known keys at each level for strict unknown-key rejection.
var (
	knownTopLevel  = map[string]bool{"version": true, "schedule": true, "command": true, "env": true}
	knownSchedule  = map[string]bool{"name": true, "description": true, "calendar": true}
	knownCommand   = map[string]bool{"exec": true, "working_directory": true, "env_file": true}
)

// Parse parses TOML data into a Config, performing strict validation.
// It does NOT resolve exec paths or validate calendar expressions -- use
// Validate for that.
func Parse(data []byte) (*Config, error) {
	// Unmarshal into generic map for strict key checking and type validation.
	var raw map[string]any
	if err := tomledit.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("TOML parse error: %w", err)
	}

	// Strict unknown-key rejection at top level.
	for key := range raw {
		if !knownTopLevel[key] {
			return nil, fmt.Errorf("unknown top-level key: %q", key)
		}
	}

	// Validate and extract version.
	rawVersion, ok := raw["version"]
	if !ok {
		return nil, fmt.Errorf("missing required key: version")
	}
	version64, ok := rawVersion.(int64)
	if !ok {
		return nil, fmt.Errorf("version must be an integer, got %T", rawVersion)
	}
	if version64 != 1 {
		return nil, fmt.Errorf("unsupported version: %d (expected 1)", version64)
	}

	// Validate [schedule] section.
	rawSchedule, ok := raw["schedule"]
	if !ok {
		return nil, fmt.Errorf("missing required section: [schedule]")
	}
	scheduleMap, ok := rawSchedule.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("[schedule] must be a table, got %T", rawSchedule)
	}
	for key := range scheduleMap {
		if !knownSchedule[key] {
			return nil, fmt.Errorf("unknown key in [schedule]: %q", key)
		}
	}

	// Validate [command] section.
	rawCommand, ok := raw["command"]
	if !ok {
		return nil, fmt.Errorf("missing required section: [command]")
	}
	commandMap, ok := rawCommand.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("[command] must be a table, got %T", rawCommand)
	}
	for key := range commandMap {
		if !knownCommand[key] {
			return nil, fmt.Errorf("unknown key in [command]: %q", key)
		}
	}

	// Validate [env] section if present.
	var envMap map[string]string
	if rawEnv, ok := raw["env"]; ok {
		envTable, ok := rawEnv.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("[env] must be a table, got %T", rawEnv)
		}
		envMap = make(map[string]string, len(envTable))
		for key, val := range envTable {
			if !envKeyRegexp.MatchString(key) {
				return nil, fmt.Errorf("invalid env var name: %q (must match %s)", key, envKeyRegexp.String())
			}
			strVal, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("env var %q must be a string, got %T", key, val)
			}
			envMap[key] = strVal
		}
	}

	// Extract schedule fields.
	schedule, err := extractSchedule(scheduleMap)
	if err != nil {
		return nil, err
	}

	// Extract command fields.
	command, err := extractCommand(commandMap)
	if err != nil {
		return nil, err
	}

	return &Config{
		Version:  int(version64),
		Schedule: schedule,
		Command:  command,
		Env:      envMap,
	}, nil
}

func extractSchedule(m map[string]any) (ScheduleConfig, error) {
	name, err := requireString(m, "name", "schedule")
	if err != nil {
		return ScheduleConfig{}, err
	}
	if len(name) > 64 {
		return ScheduleConfig{}, fmt.Errorf("schedule.name too long: %d chars (max 64)", len(name))
	}
	if !nameRegexp.MatchString(name) {
		return ScheduleConfig{}, fmt.Errorf("schedule.name contains invalid characters: %q (must match %s)", name, nameRegexp.String())
	}

	description, err := requireString(m, "description", "schedule")
	if err != nil {
		return ScheduleConfig{}, err
	}

	calendar, err := requireString(m, "calendar", "schedule")
	if err != nil {
		return ScheduleConfig{}, err
	}

	return ScheduleConfig{
		Name:        name,
		Description: description,
		Calendar:    calendar,
	}, nil
}

func extractCommand(m map[string]any) (CommandConfig, error) {
	execStr, err := requireString(m, "exec", "command")
	if err != nil {
		return CommandConfig{}, err
	}

	var workDir string
	if v, ok := m["working_directory"]; ok {
		s, ok := v.(string)
		if !ok {
			return CommandConfig{}, fmt.Errorf("command.working_directory must be a string, got %T", v)
		}
		workDir = s
	}

	var envFile string
	if v, ok := m["env_file"]; ok {
		s, ok := v.(string)
		if !ok {
			return CommandConfig{}, fmt.Errorf("command.env_file must be a string, got %T", v)
		}
		envFile = s
	}

	return CommandConfig{
		Exec:             execStr,
		WorkingDirectory: workDir,
		EnvFile:          envFile,
	}, nil
}

func requireString(m map[string]any, key, section string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("missing required key: %s.%s", section, key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s.%s must be a string, got %T", section, key, v)
	}
	if s == "" {
		return "", fmt.Errorf("%s.%s must not be empty", section, key)
	}
	return s, nil
}

// shellMetachars are characters that indicate the exec string needs shell
// interpretation rather than direct execution.
var shellMetachars = []string{"|", "&&", "||", ";", ">", "<", "`", "$(", "&"}

// Validate performs validations that require external tools or filesystem access.
// It validates the calendar expression via systemd-analyze, resolves the exec
// path, and checks that working_directory and env_file exist.
func Validate(cfg *Config, commander Commander) error {
	ctx := context.Background()

	// Validate calendar expression via systemd-analyze.
	_, stderr, err := commander.Run(ctx, "systemd-analyze", "calendar", cfg.Schedule.Calendar)
	if err != nil {
		return fmt.Errorf("invalid calendar expression %q: %s", cfg.Schedule.Calendar, strings.TrimSpace(stderr))
	}

	// Process exec string.
	if err := resolveExec(cfg); err != nil {
		return err
	}

	// Validate working_directory if set.
	if cfg.Command.WorkingDirectory != "" {
		info, err := os.Stat(cfg.Command.WorkingDirectory)
		if err != nil {
			return fmt.Errorf("working_directory does not exist: %s", cfg.Command.WorkingDirectory)
		}
		if !info.IsDir() {
			return fmt.Errorf("working_directory is not a directory: %s", cfg.Command.WorkingDirectory)
		}
	}

	// Validate env_file if set.
	if cfg.Command.EnvFile != "" {
		info, err := os.Stat(cfg.Command.EnvFile)
		if err != nil {
			return fmt.Errorf("env_file does not exist: %s", cfg.Command.EnvFile)
		}
		if info.IsDir() {
			return fmt.Errorf("env_file is a directory, not a file: %s", cfg.Command.EnvFile)
		}
	}

	return nil
}

func resolveExec(cfg *Config) error {
	execStr := cfg.Command.Exec

	// Check for shell metacharacters.
	for _, meta := range shellMetachars {
		if strings.Contains(execStr, meta) {
			cfg.Command.IsShell = true
			// Escape any double quotes in the command, then wrap in quotes
			// so systemd passes the entire string as a single argument to sh -c.
			escaped := strings.ReplaceAll(execStr, `"`, `\"`)
			cfg.Command.ResolvedExec = `/bin/sh -c "` + escaped + `"`
			return nil
		}
	}

	// No shell metacharacters: resolve the binary path.
	parts := strings.Fields(execStr)
	if len(parts) == 0 {
		return fmt.Errorf("exec is empty after splitting")
	}

	absPath, err := exec.LookPath(parts[0])
	if err != nil {
		return fmt.Errorf("exec binary not found: %s", parts[0])
	}

	parts[0] = absPath
	cfg.Command.ResolvedExec = strings.Join(parts, " ")
	return nil
}

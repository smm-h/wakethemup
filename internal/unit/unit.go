// Package unit generates systemd unit file content from schedule configs.
package unit

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"github.com/smm-h/wakethemup/internal/config"
	"github.com/smm-h/wakethemup/internal/systemd"
)

//go:embed service.tmpl
var serviceTmpl string

//go:embed timer.tmpl
var timerTmpl string

var (
	serviceTemplate = template.Must(template.New("service").Parse(serviceTmpl))
	timerTemplate   = template.Must(template.New("timer").Parse(timerTmpl))
)

// serviceData holds escaped values for the service template.
type serviceData struct {
	Description      string
	ConfigPath       string
	ExecStart        string
	WorkingDirectory string
	EnvFile          string
	EnvVars          []string // each is a quoted "KEY=value" from QuoteEnvAssignment
}

// timerData holds escaped values for the timer template.
type timerData struct {
	Description string
	Calendar    string
}

// GenerateService renders a systemd service unit from a parsed config.
// configPath is the absolute path to the TOML config file.
func GenerateService(cfg *config.Config, configPath string) (string, error) {
	data := serviceData{
		Description: EscapeSpecifiers(cfg.Schedule.Description),
		ConfigPath:  EscapeSpecifiers(configPath),
	}

	if cfg.Command.IsShell {
		data.ExecStart = EscapeExecShell(cfg.Command.ResolvedExec)
	} else {
		data.ExecStart = EscapeExecDirect(cfg.Command.ResolvedExec)
	}

	if cfg.Command.WorkingDirectory != "" {
		data.WorkingDirectory = EscapeSpecifiers(cfg.Command.WorkingDirectory)
	}

	if cfg.Command.EnvFile != "" {
		data.EnvFile = EscapeSpecifiers(cfg.Command.EnvFile)
	}

	if len(cfg.Env) > 0 {
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		data.EnvVars = make([]string, 0, len(keys))
		for _, k := range keys {
			data.EnvVars = append(data.EnvVars, QuoteEnvAssignment(k, cfg.Env[k]))
		}
	}

	var buf bytes.Buffer
	if err := serviceTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering service template: %w", err)
	}
	return buf.String(), nil
}

// GenerateTimer renders a systemd timer unit from a parsed config.
func GenerateTimer(cfg *config.Config) (string, error) {
	data := timerData{
		Description: EscapeSpecifiers(cfg.Schedule.Description),
		Calendar:    cfg.Schedule.Calendar,
	}

	var buf bytes.Buffer
	if err := timerTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering timer template: %w", err)
	}
	return buf.String(), nil
}

// UnitFilenames returns the service and timer filenames for a schedule name.
func UnitFilenames(name string) (service string, timer string) {
	return systemd.UnitPrefix + name + ".service", systemd.UnitPrefix + name + ".timer"
}

// WriteUnit atomically writes unit file content to dir/filename.
// It creates a temp file in the same directory, writes content, sets
// permissions to 0644, and renames to the final path.
func WriteUnit(dir, filename, content string) error {
	finalPath := filepath.Join(dir, filename)

	tmp, err := os.CreateTemp(dir, filename+".tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	// Clean up temp file on any error after creation.
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file %s: %w", tmpPath, err)
	}

	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("setting permissions on %s: %w", tmpPath, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, finalPath, err)
	}

	success = true
	return nil
}

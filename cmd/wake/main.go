package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/smm-h/strictcli/go/strictcli"
	"github.com/smm-h/wakethemup/internal/config"
	"github.com/smm-h/wakethemup/internal/systemd"
	"github.com/smm-h/wakethemup/internal/unit"
)

const version = "0.1.0"

func main() {
	app := strictcli.NewApp("wake", version, "Manage systemd user timers from TOML configs")

	// install: install a schedule from a TOML config file
	app.Command("install", "Install a schedule from a TOML config file", handleInstall,
		strictcli.WithArgs(
			strictcli.NewArg("config", "Path to the schedule TOML file"),
		),
		strictcli.WithFlags(
			strictcli.BoolFlag("dry-run", "Show generated units without installing"),
		),
	)

	// remove: remove an installed schedule
	app.Command("remove", "Remove an installed schedule", handleRemove,
		strictcli.WithArgs(
			strictcli.NewArg("name", "Schedule name"),
		),
		strictcli.WithFlags(
			strictcli.BoolFlag("dry-run", "Show what would be stopped and deleted"),
		),
	)

	// list: list installed schedules
	app.Command("list", "List installed schedules", handleList,
		strictcli.WithFlags(
			strictcli.BoolFlag("json", "Output as JSON array instead of table"),
		),
	)

	// status: show status of a schedule
	followFlag := strictcli.BoolFlag("follow", "Stream journal entries")
	jsonFlag := strictcli.BoolFlag("json", "Output status as JSON")

	app.Command("status", "Show status of a schedule", handleStatus,
		strictcli.WithArgs(
			strictcli.NewArg("name", "Schedule name"),
		),
		strictcli.WithFlags(
			strictcli.IntFlag("lines", "Number of journal entries to show", strictcli.Default(20)),
		),
		strictcli.WithMutex(strictcli.MutexGroup{
			Flags: []strictcli.Flag{followFlag, jsonFlag},
		}),
	)

	registerChecks(app)

	app.Run()
}

func handleInstall(args map[string]interface{}) int {
	configPath := args["config"].(string)
	dryRun, _ := args["dry-run"].(bool)

	// Read the TOML file.
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Parse configuration.
	cfg, err := config.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Create commander and detect systemd.
	cmd := systemd.NewExecCommander()

	if err := systemd.DetectSystemd(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Check and enable linger.
	hasLinger, err := systemd.CheckLinger(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	if !hasLinger {
		if err := systemd.EnableLinger(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "Enabled loginctl linger for user %s. This allows timers to fire without an active login session.\n", os.Getenv("USER"))
	}

	// Ensure unit directory exists.
	unitDir, err := systemd.EnsureUnitDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Get unit filenames and check for duplicates.
	serviceFilename, timerFilename := unit.UnitFilenames(cfg.Schedule.Name)
	timerPath := filepath.Join(unitDir, timerFilename)
	if _, err := os.Stat(timerPath); err == nil {
		fmt.Fprintf(os.Stderr, "error: schedule '%s' already installed. To update: wake remove %s && wake install %s\n",
			cfg.Schedule.Name, cfg.Schedule.Name, configPath)
		return 1
	}

	// Validate config (calendar expression, exec resolution, etc.).
	if err := config.Validate(cfg, cmd); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Resolve config path to absolute.
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Generate unit contents.
	serviceContent, err := unit.GenerateService(cfg, absConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	timerContent, err := unit.GenerateTimer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Dry run: print units and exit.
	if dryRun {
		fmt.Printf("=== %s ===\n%s\n", serviceFilename, serviceContent)
		fmt.Printf("=== %s ===\n%s\n", timerFilename, timerContent)
		return 0
	}

	// Write units atomically.
	servicePath := filepath.Join(unitDir, serviceFilename)
	if err := unit.WriteUnit(unitDir, serviceFilename, serviceContent); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	if err := unit.WriteUnit(unitDir, timerFilename, timerContent); err != nil {
		os.Remove(servicePath)
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Daemon reload.
	if err := systemd.DaemonReload(cmd); err != nil {
		os.Remove(servicePath)
		os.Remove(timerPath)
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Enable timer.
	if err := systemd.EnableTimer(cmd, cfg.Schedule.Name); err != nil {
		os.Remove(servicePath)
		os.Remove(timerPath)
		_ = systemd.DaemonReload(cmd)
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	fmt.Printf("Installed schedule '%s' (calendar: %s)\n", cfg.Schedule.Name, cfg.Schedule.Calendar)
	return 0
}

var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func handleRemove(args map[string]interface{}) int {
	name := args["name"].(string)
	dryRun, _ := args["dry-run"].(bool)

	// Validate name format.
	if len(name) > 64 || !nameRegexp.MatchString(name) {
		fmt.Fprintf(os.Stderr, "error: invalid schedule name %q (must match ^[a-zA-Z0-9_-]+$, max 64 chars)\n", name)
		return 1
	}

	// Get unit directory.
	unitDir, err := systemd.UnitDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Get filenames and check existence.
	serviceFilename, timerFilename := unit.UnitFilenames(name)
	servicePath := filepath.Join(unitDir, serviceFilename)
	timerPath := filepath.Join(unitDir, timerFilename)

	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: schedule '%s' not found\n", name)
		return 1
	}

	// Ownership verification: check for X-WakeConfig line.
	serviceData, err := os.ReadFile(servicePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	if !strings.Contains(string(serviceData), "X-WakeConfig=") {
		fmt.Fprintf(os.Stderr, "error: wake-%s was not installed by wake; refusing to remove\n", name)
		return 1
	}

	cmd := systemd.NewExecCommander()

	// Dry run: print what would be done.
	if dryRun {
		fmt.Printf("Would disable wake-%s.timer, delete unit files\n", name)
		return 0
	}

	// Disable timer (log error but continue).
	if err := systemd.DisableTimer(cmd, name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to disable timer: %s\n", err)
	}

	// Check if service is running.
	running, err := systemd.IsServiceRunning(cmd, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	if running {
		fmt.Println("Waiting for running service to finish...")
		if err := systemd.WaitForServiceStop(cmd, name, 60*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
	}

	// Delete unit files (ignore "not found" errors).
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	if err := os.Remove(timerPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Daemon reload (log error but continue).
	if err := systemd.DaemonReload(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "warning: daemon-reload failed: %s\n", err)
	}

	fmt.Printf("Removed schedule '%s'\n", name)
	return 0
}

func handleList(args map[string]interface{}) int {
	jsonOutput, _ := args["json"].(bool)

	cmd := systemd.NewExecCommander()

	timers, err := systemd.ListTimers(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	if jsonOutput {
		data, err := json.MarshalIndent(timers, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	if len(timers) == 0 {
		fmt.Println("No wake schedules installed.")
		return 0
	}

	// Compute column widths.
	headers := [5]string{"NAME", "CALENDAR", "NEXT", "LAST", "ACTIVE"}
	widths := [5]int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3]), len(headers[4])}
	for _, t := range timers {
		if len(t.Name) > widths[0] {
			widths[0] = len(t.Name)
		}
		if len(t.Calendar) > widths[1] {
			widths[1] = len(t.Calendar)
		}
		if len(t.NextFire) > widths[2] {
			widths[2] = len(t.NextFire)
		}
		if len(t.LastFire) > widths[3] {
			widths[3] = len(t.LastFire)
		}
		if len(t.Active) > widths[4] {
			widths[4] = len(t.Active)
		}
	}

	// Print header.
	fmtStr := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds\n",
		widths[0], widths[1], widths[2], widths[3], widths[4])
	fmt.Printf(fmtStr, headers[0], headers[1], headers[2], headers[3], headers[4])

	// Print rows.
	for _, t := range timers {
		fmt.Printf(fmtStr, t.Name, t.Calendar, t.NextFire, t.LastFire, t.Active)
	}

	return 0
}

// statusJSON is the JSON output structure for the status command.
type statusJSON struct {
	Name       string                `json:"name"`
	ConfigPath string                `json:"config_path"`
	Timer      *systemd.TimerStatus  `json:"timer"`
	Service    *systemd.ServiceStatus `json:"service"`
}

func handleStatus(args map[string]interface{}) int {
	name := args["name"].(string)
	lines := args["lines"].(int)
	follow, _ := args["follow"].(bool)
	jsonOutput, _ := args["json"].(bool)

	cmd := systemd.NewExecCommander()

	// Check unit files exist.
	unitDir, err := systemd.UnitDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	serviceFilename, _ := unit.UnitFilenames(name)
	servicePath := filepath.Join(unitDir, serviceFilename)
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: schedule '%s' not found\n", name)
		return 1
	}

	// Get timer and service status.
	timerStatus, err := systemd.GetTimerStatus(cmd, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	serviceStatus, err := systemd.GetServiceStatus(cmd, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Read X-WakeConfig from service unit file.
	configPath := readWakeConfigPath(servicePath)

	if follow {
		// Stream journal entries.
		unitName := systemd.UnitPrefix + name + ".service"
		journalCmd := exec.Command("journalctl", "--user", "-u", unitName, "-f", "--no-pager")
		journalCmd.Stdout = os.Stdout
		journalCmd.Stderr = os.Stderr
		if err := journalCmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		if err := journalCmd.Wait(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			return 1
		}
		return 0
	}

	if jsonOutput {
		data, err := json.MarshalIndent(statusJSON{
			Name:       name,
			ConfigPath: configPath,
			Timer:      timerStatus,
			Service:    serviceStatus,
		}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	// Default status display.
	timerLine := timerStatus.ActiveState
	if timerStatus.NextElapse != "" {
		timerLine += fmt.Sprintf(" (next: %s)", timerStatus.NextElapse)
	}

	serviceLine := serviceStatus.ActiveState
	if serviceStatus.ExecMainExitTimestamp != "" {
		serviceLine += fmt.Sprintf(" (last exit: %d)", serviceStatus.ExecMainStatus)
	}

	fmt.Printf("Schedule: %s\n", name)
	fmt.Printf("Config:   %s\n", configPath)
	fmt.Printf("Timer:    %s\n", timerLine)
	fmt.Printf("Service:  %s\n", serviceLine)

	// Print recent journal lines.
	unitName := systemd.UnitPrefix + name + ".service"
	journalCmd := exec.Command("journalctl", "--user", "-u", unitName, "-n", fmt.Sprintf("%d", lines), "--no-pager")
	journalCmd.Stdout = os.Stdout
	journalCmd.Stderr = os.Stderr
	_ = journalCmd.Run()

	return 0
}

// readWakeConfigPath reads the X-WakeConfig value from a service unit file.
func readWakeConfigPath(servicePath string) string {
	f, err := os.Open(servicePath)
	if err != nil {
		return "(unknown)"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "X-WakeConfig=") {
			path := strings.TrimPrefix(line, "X-WakeConfig=")
			if _, err := os.Stat(path); err != nil {
				return path + " (file not found)"
			}
			return path
		}
	}
	return "(unknown)"
}

// registerChecks registers check implementations if checks.toml was discovered.
func registerChecks(app *strictcli.App) {
	// Checks are only available when .strictcli/checks.toml exists in cwd
	if _, err := os.Stat(".strictcli/checks.toml"); err != nil {
		return
	}

	cmd := systemd.NewExecCommander()

	app.RegisterCheck("systemd-available", func(_ strictcli.CheckContext) strictcli.CheckResult {
		path, err := exec.LookPath("systemctl")
		if err != nil {
			return strictcli.CheckResult{Status: "fail", Message: "systemctl not found on PATH"}
		}
		return strictcli.CheckResult{Status: "pass", Message: fmt.Sprintf("systemctl found at %s", path)}
	})

	app.RegisterCheck("systemd-user-session", func(_ strictcli.CheckContext) strictcli.CheckResult {
		_, stderr, err := cmd.Run(context.Background(), "systemctl", "--user", "status")
		if err != nil {
			if strings.Contains(stderr, "Failed to connect to bus") ||
				strings.Contains(stderr, "No such file or directory") {
				return strictcli.CheckResult{
					Status:  "fail",
					Message: fmt.Sprintf("user systemd session not available: %s", strings.TrimSpace(stderr)),
				}
			}
		}
		return strictcli.CheckResult{Status: "pass", Message: "user systemd session is running"}
	})

	app.RegisterCheck("linger-enabled", func(_ strictcli.CheckContext) strictcli.CheckResult {
		enabled, err := systemd.CheckLinger(cmd)
		if err != nil {
			return strictcli.CheckResult{Status: "fail", Message: fmt.Sprintf("failed to check linger: %s", err)}
		}
		if !enabled {
			return strictcli.CheckResult{Status: "fail", Message: "linger is not enabled; run: loginctl enable-linger"}
		}
		return strictcli.CheckResult{
			Status:  "pass",
			Message: fmt.Sprintf("linger is enabled for user %s", os.Getenv("USER")),
		}
	})

	app.RegisterCheck("unit-dir-exists", func(_ strictcli.CheckContext) strictcli.CheckResult {
		dir, err := systemd.UnitDir()
		if err != nil {
			return strictcli.CheckResult{Status: "fail", Message: fmt.Sprintf("cannot determine unit directory: %s", err)}
		}
		if _, err := os.Stat(dir); err != nil {
			return strictcli.CheckResult{
				Status:  "fail",
				Message: fmt.Sprintf("unit directory does not exist: %s. It will be created on first install.", dir),
			}
		}
		return strictcli.CheckResult{Status: "pass", Message: fmt.Sprintf("unit directory exists: %s", dir)}
	})

	app.RegisterCheck("unit-dir-writable", func(_ strictcli.CheckContext) strictcli.CheckResult {
		dir, err := systemd.UnitDir()
		if err != nil {
			return strictcli.CheckResult{Status: "fail", Message: fmt.Sprintf("cannot determine unit directory: %s", err)}
		}
		f, err := os.CreateTemp(dir, ".wake-check-*")
		if err != nil {
			return strictcli.CheckResult{
				Status:  "fail",
				Message: fmt.Sprintf("unit directory is not writable: %s", err),
			}
		}
		name := f.Name()
		f.Close()
		os.Remove(name)
		return strictcli.CheckResult{Status: "pass", Message: "unit directory is writable"}
	})

	app.RegisterCheck("installed-units-healthy", func(_ strictcli.CheckContext) strictcli.CheckResult {
		timers, err := systemd.ListTimers(cmd)
		if err != nil {
			return strictcli.CheckResult{Status: "fail", Message: fmt.Sprintf("failed to list timers: %s", err)}
		}
		if len(timers) == 0 {
			return strictcli.CheckResult{Status: "pass", Message: "no wake schedules installed"}
		}

		var unhealthy []string
		hasFailed := false
		for _, t := range timers {
			timerBad := t.Active == "" || t.Active == "inactive"

			svcStatus, err := systemd.GetServiceStatus(cmd, t.Name)
			svcFailed := false
			if err == nil {
				svcFailed = svcStatus.Result == "failed" || svcStatus.ActiveState == "failed"
			}

			if timerBad || svcFailed {
				detail := fmt.Sprintf("wake-%s.timer", t.Name)
				if timerBad {
					detail += " (timer inactive)"
				}
				if svcFailed {
					detail += " (service failed)"
					hasFailed = true
				}
				unhealthy = append(unhealthy, detail)
			}
		}

		if len(unhealthy) > 0 {
			status := "warn"
			if hasFailed {
				status = "fail"
			}
			return strictcli.CheckResult{
				Status:  status,
				Message: fmt.Sprintf("%d of %d schedules unhealthy", len(unhealthy), len(timers)),
				Details: unhealthy,
			}
		}
		return strictcli.CheckResult{
			Status:  "pass",
			Message: fmt.Sprintf("all %d schedules healthy", len(timers)),
		}
	})

	app.SetCheckContext(func() strictcli.CheckContext {
		cwd, _ := os.Getwd()
		return &wakeCheckContext{root: cwd}
	})
}

// wakeCheckContext provides project context to check implementations.
type wakeCheckContext struct {
	root string
}

func (c *wakeCheckContext) ProjectRoot() string {
	return c.root
}

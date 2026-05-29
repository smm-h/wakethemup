package main

import (
	"os"

	"github.com/smm-h/strictcli/go/strictcli"
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
	return 0
}

func handleRemove(args map[string]interface{}) int {
	return 0
}

func handleList(args map[string]interface{}) int {
	return 0
}

func handleStatus(args map[string]interface{}) int {
	return 0
}

// registerChecks registers placeholder check implementations if checks.toml was discovered.
func registerChecks(app *strictcli.App) {
	// Checks are only available when .strictcli/checks.toml exists in cwd
	if _, err := os.Stat(".strictcli/checks.toml"); err != nil {
		return
	}

	skipResult := func(_ strictcli.CheckContext) strictcli.CheckResult {
		return strictcli.CheckResult{Status: "skip", Message: "not yet implemented"}
	}

	app.RegisterCheck("systemd-available", skipResult)
	app.RegisterCheck("systemd-user-session", skipResult)
	app.RegisterCheck("linger-enabled", skipResult)
	app.RegisterCheck("unit-dir-exists", skipResult)
	app.RegisterCheck("unit-dir-writable", skipResult)
	app.RegisterCheck("installed-units-healthy", skipResult)

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

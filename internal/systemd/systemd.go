//go:build linux

// Package systemd manages systemd user-level timer and service units.
package systemd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DetectSystemd checks that systemd is available and the user session is functional.
func DetectSystemd(cmd Commander) error {
	_, err := cmd.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found on PATH: systemd is required")
	}

	ctx := context.Background()
	_, stderr, err := cmd.Run(ctx, "systemctl", "--user", "status")
	if err != nil {
		// systemctl --user status returns non-zero when no units are failed,
		// but the command itself succeeds. Only fail if we can't connect to
		// the user session bus at all.
		if strings.Contains(stderr, "Failed to connect to bus") ||
			strings.Contains(stderr, "No such file or directory") {
			return fmt.Errorf("systemd user session not available: %s", strings.TrimSpace(stderr))
		}
	}
	return nil
}

// DaemonReload runs systemctl --user daemon-reload.
func DaemonReload(cmd Commander) error {
	ctx := context.Background()
	_, _, err := cmd.Run(ctx, "systemctl", "--user", "daemon-reload")
	return err
}

// EnableTimer enables and starts the wake timer for the given schedule name.
func EnableTimer(cmd Commander, name string) error {
	ctx := context.Background()
	_, _, err := cmd.Run(ctx, "systemctl", "--user", "enable", "--now", UnitPrefix+name+".timer")
	return err
}

// DisableTimer disables and stops the wake timer for the given schedule name.
func DisableTimer(cmd Commander, name string) error {
	ctx := context.Background()
	_, _, err := cmd.Run(ctx, "systemctl", "--user", "disable", "--now", UnitPrefix+name+".timer")
	return err
}

// IsServiceRunning checks whether the wake service for the given name is currently running.
// For Type=oneshot services, running means ActiveState=activating and SubState=start.
func IsServiceRunning(cmd Commander, name string) (bool, error) {
	ctx := context.Background()
	stdout, _, err := cmd.Run(ctx, "systemctl", "--user", "show",
		UnitPrefix+name+".service",
		"--property=ActiveState,SubState", "--no-pager")
	if err != nil {
		return false, err
	}

	props := parseKeyValue(stdout)
	return props["ActiveState"] == "activating" && props["SubState"] == "start", nil
}

// WaitForServiceStop polls IsServiceRunning until the service stops or the timeout is exceeded.
func WaitForServiceStop(cmd Commander, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		running, err := IsServiceRunning(cmd, name)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service still running after %ds; to force-terminate: systemctl --user stop %s%s.service",
				int(timeout.Seconds()), UnitPrefix, name)
		}
		time.Sleep(2 * time.Second)
	}
}

// GetTimerStatus returns the status of the wake timer for the given name.
func GetTimerStatus(cmd Commander, name string) (*TimerStatus, error) {
	ctx := context.Background()
	stdout, _, err := cmd.Run(ctx, "systemctl", "--user", "show",
		UnitPrefix+name+".timer",
		"--property=ActiveState,NextElapseUSecRealtime,LastTriggerUSec,Result", "--no-pager")
	if err != nil {
		return nil, err
	}

	props := parseKeyValue(stdout)
	return &TimerStatus{
		ActiveState: props["ActiveState"],
		NextElapse:  props["NextElapseUSecRealtime"],
		LastTrigger: props["LastTriggerUSec"],
		Result:      props["Result"],
	}, nil
}

// GetServiceStatus returns the status of the wake service for the given name.
func GetServiceStatus(cmd Commander, name string) (*ServiceStatus, error) {
	ctx := context.Background()
	stdout, _, err := cmd.Run(ctx, "systemctl", "--user", "show",
		UnitPrefix+name+".service",
		"--property=ActiveState,SubState,ExecMainStartTimestamp,ExecMainExitTimestamp,ExecMainStatus,Result",
		"--no-pager")
	if err != nil {
		return nil, err
	}

	props := parseKeyValue(stdout)
	exitStatus, _ := strconv.Atoi(props["ExecMainStatus"])
	return &ServiceStatus{
		ActiveState:            props["ActiveState"],
		SubState:               props["SubState"],
		ExecMainStartTimestamp: props["ExecMainStartTimestamp"],
		ExecMainExitTimestamp:  props["ExecMainExitTimestamp"],
		ExecMainStatus:         exitStatus,
		Result:                 props["Result"],
	}, nil
}

// ListTimers returns all wake-managed timers by parsing systemctl list-timers output.
func ListTimers(cmd Commander) ([]TimerInfo, error) {
	ctx := context.Background()
	stdout, _, err := cmd.Run(ctx, "systemctl", "--user", "list-timers", "wake-*", "--no-pager", "--all")
	if err != nil {
		return nil, err
	}

	return parseListTimers(stdout), nil
}

// parseListTimers parses the table output from systemctl list-timers.
// The format has columns: NEXT, LEFT, LAST, PASSED, UNIT, ACTIVATES
// with a header row, data rows, and a summary line at the end.
func parseListTimers(output string) []TimerInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil
	}

	// Find the header line to determine column positions
	headerIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "NEXT") && strings.Contains(line, "UNIT") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil
	}

	header := lines[headerIdx]
	unitCol := strings.Index(header, "UNIT")
	activatesCol := strings.Index(header, "ACTIVATES")
	nextCol := strings.Index(header, "NEXT")
	lastCol := strings.Index(header, "LAST")

	if unitCol < 0 {
		return nil
	}

	var timers []TimerInfo
	for _, line := range lines[headerIdx+1:] {
		// Skip empty lines and the summary line (e.g., "2 timers listed.")
		if line == "" || strings.Contains(line, " timers listed") {
			continue
		}

		// Extract UNIT column value
		var unitName string
		if activatesCol > 0 && len(line) > unitCol {
			end := activatesCol
			if end > len(line) {
				end = len(line)
			}
			unitName = strings.TrimSpace(line[unitCol:end])
		} else if len(line) > unitCol {
			unitName = strings.TrimSpace(line[unitCol:])
		}

		if unitName == "" {
			continue
		}

		// Strip wake- prefix and .timer suffix to get the schedule name
		name := unitName
		if strings.HasPrefix(name, UnitPrefix) {
			name = strings.TrimPrefix(name, UnitPrefix)
		}
		if strings.HasSuffix(name, ".timer") {
			name = strings.TrimSuffix(name, ".timer")
		}

		// Extract NEXT and LAST columns
		var nextFire, lastFire string
		if nextCol >= 0 && lastCol > nextCol && len(line) > nextCol {
			end := lastCol
			if end > len(line) {
				end = len(line)
			}
			nextFire = strings.TrimSpace(line[nextCol:end])
		}
		if lastCol >= 0 && unitCol > lastCol && len(line) > lastCol {
			end := unitCol
			// PASSED column is between LAST and UNIT
			passedCol := strings.Index(header, "PASSED")
			if passedCol > 0 {
				end = passedCol
			}
			if end > len(line) {
				end = len(line)
			}
			lastFire = strings.TrimSpace(line[lastCol:end])
		}

		timers = append(timers, TimerInfo{
			Name:     name,
			NextFire: nextFire,
			LastFire: lastFire,
		})
	}

	return timers
}

// ValidateCalendar checks whether a calendar expression is valid using systemd-analyze.
func ValidateCalendar(cmd Commander, expr string) error {
	ctx := context.Background()
	_, _, err := cmd.Run(ctx, "systemd-analyze", "calendar", expr)
	return err
}

// UnitDir returns the path to the systemd user unit directory without creating it.
func UnitDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "systemd", "user"), nil
}

// EnsureUnitDir returns the systemd user unit directory, creating it if necessary.
func EnsureUnitDir() (string, error) {
	dir, err := UnitDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create unit directory %s: %w", dir, err)
	}
	return dir, nil
}

// parseKeyValue parses systemctl show output (KEY=VALUE lines) into a map.
func parseKeyValue(output string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := line[:idx]
		val := line[idx+1:]
		props[key] = val
	}
	return props
}

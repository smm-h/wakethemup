//go:build linux

package systemd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectSystemd_Success(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user status", MockResponse{Stdout: "", Stderr: "", Err: nil})
	err := DetectSystemd(mock)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDetectSystemd_NonZeroButNoError(t *testing.T) {
	// systemctl --user status returns non-zero when no units are failed
	// but the user session is alive — this should not be treated as an error
	mock := &MockCommander{}
	mock.OnCall("systemctl --user status", MockResponse{
		Stdout: "",
		Stderr: "some non-bus error output",
		Err:    fmt.Errorf("systemctl: exit status 1 (stderr: some non-bus error output)"),
	})
	err := DetectSystemd(mock)
	if err != nil {
		t.Fatalf("expected no error for non-bus error, got: %v", err)
	}
}

func TestDetectSystemd_DeadUserSession(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user status", MockResponse{
		Stdout: "",
		Stderr: "Failed to connect to bus: No such file or directory",
		Err:    fmt.Errorf("systemctl: exit status 1 (stderr: Failed to connect to bus: No such file or directory)"),
	})
	err := DetectSystemd(mock)
	if err == nil {
		t.Fatal("expected error for dead user session")
	}
	if got := err.Error(); !contains(got, "user session not available") {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestDaemonReload_Success(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user daemon-reload", MockResponse{})
	err := DaemonReload(mock)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDaemonReload_Failure(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user daemon-reload", MockResponse{
		Err: fmt.Errorf("systemctl: failed"),
	})
	err := DaemonReload(mock)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnableTimer_Success(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user enable --now wake-backup.timer", MockResponse{})
	err := EnableTimer(mock, "backup")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
}

func TestEnableTimer_Failure(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user enable --now wake-backup.timer", MockResponse{
		Err: fmt.Errorf("failed to enable"),
	})
	err := EnableTimer(mock, "backup")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisableTimer_Success(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user disable --now wake-backup.timer", MockResponse{})
	err := DisableTimer(mock, "backup")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDisableTimer_Failure(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user disable --now wake-backup.timer", MockResponse{
		Err: fmt.Errorf("failed to disable"),
	})
	err := DisableTimer(mock, "backup")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsServiceRunning_Active(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user show wake-backup.service --property=ActiveState,SubState --no-pager", MockResponse{
		Stdout: "ActiveState=activating\nSubState=start\n",
	})
	running, err := IsServiceRunning(mock, "backup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !running {
		t.Fatal("expected service to be running")
	}
}

func TestIsServiceRunning_Inactive(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user show wake-backup.service --property=ActiveState,SubState --no-pager", MockResponse{
		Stdout: "ActiveState=inactive\nSubState=dead\n",
	})
	running, err := IsServiceRunning(mock, "backup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running {
		t.Fatal("expected service to not be running")
	}
}

func TestWaitForServiceStop_StopsWithinTimeout(t *testing.T) {
	// WaitForServiceStop polls IsServiceRunning repeatedly. We need
	// call-count-based behavior: first call returns running, second stopped.
	countingMock := &countingCommander{
		responses: []MockResponse{
			{Stdout: "ActiveState=activating\nSubState=start\n"},
			{Stdout: "ActiveState=inactive\nSubState=dead\n"},
		},
	}

	err := WaitForServiceStop(countingMock, "backup", 10*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestWaitForServiceStop_ExceedsTimeout(t *testing.T) {
	// Always return running
	alwaysRunning := &countingCommander{
		responses: []MockResponse{
			{Stdout: "ActiveState=activating\nSubState=start\n"},
		},
		repeat: true,
	}

	err := WaitForServiceStop(alwaysRunning, "backup", 1*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if got := err.Error(); !contains(got, "service still running") {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestGetTimerStatus(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user show wake-backup.timer --property=ActiveState,NextElapseUSecRealtime,LastTriggerUSec,Result --no-pager", MockResponse{
		Stdout: "ActiveState=active\nNextElapseUSecRealtime=Thu 2026-05-30 02:00:00 CEST\nLastTriggerUSec=Wed 2026-05-29 02:00:00 CEST\nResult=success\n",
	})

	status, err := GetTimerStatus(mock, "backup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ActiveState != "active" {
		t.Errorf("ActiveState = %q, want %q", status.ActiveState, "active")
	}
	if status.NextElapse != "Thu 2026-05-30 02:00:00 CEST" {
		t.Errorf("NextElapse = %q, want expected value", status.NextElapse)
	}
	if status.LastTrigger != "Wed 2026-05-29 02:00:00 CEST" {
		t.Errorf("LastTrigger = %q, want expected value", status.LastTrigger)
	}
	if status.Result != "success" {
		t.Errorf("Result = %q, want %q", status.Result, "success")
	}
}

func TestGetServiceStatus(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user show wake-backup.service --property=ActiveState,SubState,ExecMainStartTimestamp,ExecMainExitTimestamp,ExecMainStatus,Result --no-pager", MockResponse{
		Stdout: "ActiveState=inactive\nSubState=dead\nExecMainStartTimestamp=Wed 2026-05-29 02:00:00 CEST\nExecMainExitTimestamp=Wed 2026-05-29 02:05:00 CEST\nExecMainStatus=0\nResult=success\n",
	})

	status, err := GetServiceStatus(mock, "backup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ActiveState != "inactive" {
		t.Errorf("ActiveState = %q, want %q", status.ActiveState, "inactive")
	}
	if status.SubState != "dead" {
		t.Errorf("SubState = %q, want %q", status.SubState, "dead")
	}
	if status.ExecMainStartTimestamp != "Wed 2026-05-29 02:00:00 CEST" {
		t.Errorf("ExecMainStartTimestamp = %q, want expected value", status.ExecMainStartTimestamp)
	}
	if status.ExecMainExitTimestamp != "Wed 2026-05-29 02:05:00 CEST" {
		t.Errorf("ExecMainExitTimestamp = %q, want expected value", status.ExecMainExitTimestamp)
	}
	if status.ExecMainStatus != 0 {
		t.Errorf("ExecMainStatus = %d, want 0", status.ExecMainStatus)
	}
	if status.Result != "success" {
		t.Errorf("Result = %q, want %q", status.Result, "success")
	}
}

func TestListTimers(t *testing.T) {
	mock := &MockCommander{}
	// Realistic systemctl list-timers output
	output := `NEXT                         LEFT          LAST                         PASSED       UNIT                   ACTIVATES
Thu 2026-05-30 02:00:00 CEST 12h left      Wed 2026-05-29 02:00:00 CEST 10h ago      wake-backup.timer      wake-backup.service
Fri 2026-05-30 08:00:00 CEST 18h left      n/a                          n/a          wake-healthcheck.timer wake-healthcheck.service

2 timers listed.`
	mock.OnCall("systemctl --user list-timers wake-* --no-pager --all", MockResponse{
		Stdout: output,
	})

	timers, err := ListTimers(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(timers) != 2 {
		t.Fatalf("expected 2 timers, got %d", len(timers))
	}

	if timers[0].Name != "backup" {
		t.Errorf("timers[0].Name = %q, want %q", timers[0].Name, "backup")
	}
	if timers[1].Name != "healthcheck" {
		t.Errorf("timers[1].Name = %q, want %q", timers[1].Name, "healthcheck")
	}
}

func TestListTimers_Empty(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemctl --user list-timers wake-* --no-pager --all", MockResponse{
		Stdout: "0 timers listed.",
	})

	timers, err := ListTimers(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(timers) != 0 {
		t.Fatalf("expected 0 timers, got %d", len(timers))
	}
}

func TestValidateCalendar_Valid(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemd-analyze calendar *-*-* 02:00:00", MockResponse{
		Stdout: "  Original form: *-*-* 02:00:00\nNormalized form: *-*-* 02:00:00\n",
	})

	err := ValidateCalendar(mock, "*-*-* 02:00:00")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateCalendar_Invalid(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("systemd-analyze calendar not-a-calendar", MockResponse{
		Stderr: "Failed to parse calendar expression",
		Err:    fmt.Errorf("systemd-analyze: exit status 1 (stderr: Failed to parse calendar expression)"),
	})

	err := ValidateCalendar(mock, "not-a-calendar")
	if err == nil {
		t.Fatal("expected error for invalid calendar")
	}
}

func TestUnitDir_Default(t *testing.T) {
	// Clear XDG_CONFIG_HOME to test default path
	orig := os.Getenv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", orig)

	dir, err := UnitDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "systemd", "user")
	if dir != expected {
		t.Errorf("UnitDir() = %q, want %q", dir, expected)
	}
}

func TestUnitDir_CustomXDG(t *testing.T) {
	orig := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg-config")
	defer os.Setenv("XDG_CONFIG_HOME", orig)

	dir, err := UnitDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/tmp/test-xdg-config/systemd/user"
	if dir != expected {
		t.Errorf("UnitDir() = %q, want %q", dir, expected)
	}
}

func TestEnsureUnitDir(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", orig)

	dir, err := EnsureUnitDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(tmpDir, "systemd", "user")
	if dir != expected {
		t.Errorf("EnsureUnitDir() = %q, want %q", dir, expected)
	}

	// Verify directory was created
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}
}

func TestParseKeyValue(t *testing.T) {
	input := "ActiveState=active\nSubState=running\nResult=success\n"
	props := parseKeyValue(input)

	tests := map[string]string{
		"ActiveState": "active",
		"SubState":    "running",
		"Result":      "success",
	}
	for k, want := range tests {
		if got := props[k]; got != want {
			t.Errorf("parseKeyValue[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestParseKeyValue_ValueWithEquals(t *testing.T) {
	// Values can contain '=' (e.g., timestamps)
	input := "ExecMainStartTimestamp=Wed 2026-05-29 02:00:00 CEST\n"
	props := parseKeyValue(input)
	if got := props["ExecMainStartTimestamp"]; got != "Wed 2026-05-29 02:00:00 CEST" {
		t.Errorf("got %q, want timestamp", got)
	}
}

// contains is a helper for substring checks in error messages.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// countingCommander returns responses in order, one per call.
type countingCommander struct {
	responses []MockResponse
	callIdx   int
	repeat    bool // if true, repeat the last response forever
}

func (c *countingCommander) Run(_ context.Context, args ...string) (string, string, error) {
	idx := c.callIdx
	if idx >= len(c.responses) {
		if c.repeat {
			idx = len(c.responses) - 1
		} else {
			return "", "", fmt.Errorf("no more mock responses (call %d)", c.callIdx)
		}
	}
	c.callIdx++
	r := c.responses[idx]
	return r.Stdout, r.Stderr, r.Err
}

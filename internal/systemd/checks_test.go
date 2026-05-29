//go:build linux

package systemd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Tests for functions and patterns used by the check system in cmd/wake/main.go.
// The checks themselves are closures registered in main() and are integration-tested
// in Phase 8. These tests verify the underlying systemd functions that checks depend on.

func TestGetServiceStatus_Failed(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall(
		"systemctl --user show wake-broken.service --property=ActiveState,SubState,ExecMainStartTimestamp,ExecMainExitTimestamp,ExecMainStatus,Result --no-pager",
		MockResponse{
			Stdout: "ActiveState=failed\nSubState=failed\nExecMainStartTimestamp=Wed 2026-05-29 02:00:00 CEST\nExecMainExitTimestamp=Wed 2026-05-29 02:00:01 CEST\nExecMainStatus=1\nResult=exit-code\n",
		},
	)

	status, err := GetServiceStatus(mock, "broken")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ActiveState != "failed" {
		t.Errorf("ActiveState = %q, want %q", status.ActiveState, "failed")
	}
	if status.Result != "exit-code" {
		t.Errorf("Result = %q, want %q", status.Result, "exit-code")
	}
	if status.ExecMainStatus != 1 {
		t.Errorf("ExecMainStatus = %d, want 1", status.ExecMainStatus)
	}
}

func TestGetServiceStatus_ResultFailed(t *testing.T) {
	// Some failure modes set Result=failed rather than ActiveState=failed.
	mock := &MockCommander{}
	mock.OnCall(
		"systemctl --user show wake-timeout.service --property=ActiveState,SubState,ExecMainStartTimestamp,ExecMainExitTimestamp,ExecMainStatus,Result --no-pager",
		MockResponse{
			Stdout: "ActiveState=inactive\nSubState=dead\nExecMainStartTimestamp=Wed 2026-05-29 02:00:00 CEST\nExecMainExitTimestamp=Wed 2026-05-29 02:05:00 CEST\nExecMainStatus=143\nResult=failed\n",
		},
	)

	status, err := GetServiceStatus(mock, "timeout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Result != "failed" {
		t.Errorf("Result = %q, want %q", status.Result, "failed")
	}
}

func TestGetServiceStatus_CommandError(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall(
		"systemctl --user show wake-missing.service --property=ActiveState,SubState,ExecMainStartTimestamp,ExecMainExitTimestamp,ExecMainStatus,Result --no-pager",
		MockResponse{
			Err: fmt.Errorf("systemctl: unit not found"),
		},
	)

	_, err := GetServiceStatus(mock, "missing")
	if err == nil {
		t.Fatal("expected error for missing service")
	}
}

func TestUnitDir_Writable(t *testing.T) {
	// Simulate the unit-dir-writable check pattern: create temp file then remove it.
	tmpDir := t.TempDir()
	orig := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", orig)

	dir, err := EnsureUnitDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f, err := os.CreateTemp(dir, ".wake-check-*")
	if err != nil {
		t.Fatalf("unit directory not writable: %v", err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)

	// Verify temp file was cleaned up.
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("temp file was not cleaned up")
	}
}

func TestUnitDir_NotWritable(t *testing.T) {
	// Create a read-only directory to verify the writable check pattern fails.
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly", "systemd", "user")
	if err := os.MkdirAll(readOnlyDir, 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.Chmod(readOnlyDir, 0555); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0755) // restore so t.TempDir cleanup works

	_, err := os.CreateTemp(readOnlyDir, ".wake-check-*")
	if err == nil {
		t.Fatal("expected error writing to read-only directory")
	}
}

func TestListTimers_WithActiveState(t *testing.T) {
	// The installed-units-healthy check examines the Active field of each timer.
	// Verify that ListTimers produces timers we can inspect for active state.
	mock := &MockCommander{}
	output := `NEXT                         LEFT          LAST                         PASSED       UNIT                   ACTIVATES
Thu 2026-05-30 02:00:00 CEST 12h left      Wed 2026-05-29 02:00:00 CEST 10h ago      wake-backup.timer      wake-backup.service

1 timers listed.`
	mock.OnCall("systemctl --user list-timers wake-* --no-pager --all", MockResponse{
		Stdout: output,
	})

	timers, err := ListTimers(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(timers) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(timers))
	}
	if timers[0].Name != "backup" {
		t.Errorf("Name = %q, want %q", timers[0].Name, "backup")
	}
}

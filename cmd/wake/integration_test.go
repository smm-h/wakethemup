//go:build integration

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var wakeBinary string

func TestMain(m *testing.M) {
	// Build the binary once for all integration tests.
	tmp, err := os.MkdirTemp("", "wake-integration-*")
	if err != nil {
		panic("cannot create temp dir: " + err.Error())
	}
	wakeBinary = filepath.Join(tmp, "wake-test")

	cmd := exec.Command("go", "build", "-o", wakeBinary, ".")
	cmd.Dir = filepath.Join(projectRoot(), "cmd", "wake")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("build failed: " + err.Error() + "\n" + string(out))
	}

	code := m.Run()

	os.RemoveAll(tmp)
	os.Exit(code)
}

func projectRoot() string {
	// Walk up from this file's dir until we find go.mod.
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("cannot find project root (go.mod)")
		}
		dir = parent
	}
}

func writeTestTOML(t *testing.T, name string) string {
	t.Helper()
	content := `version = 1

[schedule]
name = "` + name + `"
description = "Integration test timer"
calendar = "*-*-* *:*:00/10"

[command]
exec = "/bin/true"
`
	path := filepath.Join(t.TempDir(), name+".toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing test TOML: %v", err)
	}
	return path
}

func unitDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot determine home dir: %v", err)
	}
	return filepath.Join(home, ".config", "systemd", "user")
}

func runWake(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(wakeBinary, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("exec error: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

func cleanup(t *testing.T, name string) {
	t.Helper()
	// Best-effort cleanup: remove the schedule if it exists.
	runWake(t, "remove", name)
}

func TestIntegration_InstallAndRemove(t *testing.T) {
	name := "integ-install-remove"
	tomlPath := writeTestTOML(t, name)
	defer cleanup(t, name)

	// Install
	stdout, stderr, code := runWake(t, "install", tomlPath)
	if code != 0 {
		t.Fatalf("install failed (exit %d): stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Installed schedule") {
		t.Errorf("install output missing confirmation: %q", stdout)
	}

	// Verify unit files exist
	udir := unitDir(t)
	servicePath := filepath.Join(udir, "wake-"+name+".service")
	timerPath := filepath.Join(udir, "wake-"+name+".timer")
	if _, err := os.Stat(servicePath); err != nil {
		t.Errorf("service file not found: %v", err)
	}
	if _, err := os.Stat(timerPath); err != nil {
		t.Errorf("timer file not found: %v", err)
	}

	// Verify service file contains X-WakeConfig
	data, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("reading service file: %v", err)
	}
	if !strings.Contains(string(data), "X-WakeConfig=") {
		t.Error("service file missing X-WakeConfig header")
	}

	// List should contain the schedule name
	stdout, _, code = runWake(t, "list")
	if code != 0 {
		t.Fatalf("list failed (exit %d)", code)
	}
	if !strings.Contains(stdout, name) {
		t.Errorf("list output does not contain %q: %q", name, stdout)
	}

	// List --json should produce valid JSON containing the name
	stdout, _, code = runWake(t, "list", "--json")
	if code != 0 {
		t.Fatalf("list --json failed (exit %d)", code)
	}
	var timers []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &timers); err != nil {
		t.Fatalf("list --json invalid JSON: %v\noutput: %q", err, stdout)
	}
	found := false
	for _, timer := range timers {
		if timer["Name"] == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("list --json does not contain timer %q", name)
	}

	// Remove
	stdout, stderr, code = runWake(t, "remove", name)
	if code != 0 {
		t.Fatalf("remove failed (exit %d): stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Removed schedule") {
		t.Errorf("remove output missing confirmation: %q", stdout)
	}

	// Verify files are gone
	if _, err := os.Stat(servicePath); !os.IsNotExist(err) {
		t.Error("service file still exists after remove")
	}
	if _, err := os.Stat(timerPath); !os.IsNotExist(err) {
		t.Error("timer file still exists after remove")
	}
}

func TestIntegration_DryRun(t *testing.T) {
	name := "integ-dry-run"
	tomlPath := writeTestTOML(t, name)
	defer cleanup(t, name)

	// Install with --dry-run
	stdout, stderr, code := runWake(t, "install", tomlPath, "--dry-run")
	if code != 0 {
		t.Fatalf("install --dry-run failed (exit %d): stdout=%q stderr=%q", code, stdout, stderr)
	}

	// Should print unit file contents
	if !strings.Contains(stdout, "[Unit]") {
		t.Errorf("dry-run output missing [Unit] section: %q", stdout)
	}
	if !strings.Contains(stdout, "[Service]") {
		t.Errorf("dry-run output missing [Service] section: %q", stdout)
	}
	if !strings.Contains(stdout, "[Timer]") {
		t.Errorf("dry-run output missing [Timer] section: %q", stdout)
	}

	// Should NOT have installed the timer
	if strings.Contains(stdout, "Installed schedule") {
		t.Error("dry-run should not install")
	}

	// Verify no files were created
	udir := unitDir(t)
	servicePath := filepath.Join(udir, "wake-"+name+".service")
	if _, err := os.Stat(servicePath); err == nil {
		t.Error("service file exists after dry-run -- dry-run should not create files")
		// Clean up since defer won't handle this case properly
		cleanup(t, name)
	}
}

func TestIntegration_DuplicateInstall(t *testing.T) {
	name := "integ-duplicate"
	tomlPath := writeTestTOML(t, name)
	defer cleanup(t, name)

	// First install should succeed
	_, _, code := runWake(t, "install", tomlPath)
	if code != 0 {
		t.Fatalf("first install failed (exit %d)", code)
	}

	// Second install should fail
	_, stderr, code := runWake(t, "install", tomlPath)
	if code == 0 {
		t.Fatal("duplicate install should have failed")
	}
	if !strings.Contains(stderr, "already installed") {
		t.Errorf("duplicate install error missing 'already installed': %q", stderr)
	}
}

func TestIntegration_RemoveNonExistent(t *testing.T) {
	name := "integ-nonexistent-xyz"

	_, stderr, code := runWake(t, "remove", name)
	if code == 0 {
		t.Fatal("remove of non-existent schedule should have failed")
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("remove error missing 'not found': %q", stderr)
	}
}

func TestIntegration_RemoveDryRun(t *testing.T) {
	name := "integ-remove-dry"
	tomlPath := writeTestTOML(t, name)
	defer cleanup(t, name)

	// Install first
	_, _, code := runWake(t, "install", tomlPath)
	if code != 0 {
		t.Fatalf("install failed (exit %d)", code)
	}

	// Remove --dry-run should not actually remove
	stdout, _, code := runWake(t, "remove", name, "--dry-run")
	if code != 0 {
		t.Fatalf("remove --dry-run failed (exit %d)", code)
	}
	if !strings.Contains(stdout, "Would disable") {
		t.Errorf("remove --dry-run output missing 'Would disable': %q", stdout)
	}

	// Timer should still exist
	udir := unitDir(t)
	servicePath := filepath.Join(udir, "wake-"+name+".service")
	if _, err := os.Stat(servicePath); err != nil {
		t.Error("service file missing after remove --dry-run -- dry-run should not delete files")
	}
}

func TestIntegration_StatusJSON(t *testing.T) {
	name := "integ-status-json"
	tomlPath := writeTestTOML(t, name)
	defer cleanup(t, name)

	// Install first
	_, _, code := runWake(t, "install", tomlPath)
	if code != 0 {
		t.Fatalf("install failed (exit %d)", code)
	}

	// Status --json should produce valid JSON
	stdout, _, code := runWake(t, "status", name, "--json")
	if code != 0 {
		t.Fatalf("status --json failed (exit %d)", code)
	}

	var status map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("status --json invalid JSON: %v\noutput: %q", err, stdout)
	}
	if status["name"] != name {
		t.Errorf("status name = %v, want %q", status["name"], name)
	}
	if status["timer"] == nil {
		t.Error("status timer is nil")
	}
	if status["service"] == nil {
		t.Error("status service is nil")
	}
}

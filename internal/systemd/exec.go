//go:build linux

package systemd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

const defaultTimeout = 30 * time.Second

// ExecCommander runs commands by shelling out to system binaries.
type ExecCommander struct{}

// NewExecCommander returns a new ExecCommander.
func NewExecCommander() *ExecCommander {
	return &ExecCommander{}
}

// Run executes an external command and returns its stdout, stderr, and any error.
func (c *ExecCommander) Run(ctx context.Context, args ...string) (string, string, error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("no command specified")
	}

	// Apply default timeout if context has no deadline
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.String(), stderr.String(), fmt.Errorf("%s: %w (stderr: %s)", args[0], err, stderr.String())
	}
	return stdout.String(), stderr.String(), nil
}

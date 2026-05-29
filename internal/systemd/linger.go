//go:build linux

package systemd

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// CheckLinger reports whether lingering is enabled for the current user.
func CheckLinger(cmd Commander) (bool, error) {
	ctx := context.Background()
	stdout, _, err := cmd.Run(ctx, "loginctl", "show-user", os.Getenv("USER"), "--property=Linger")
	if err != nil {
		return false, err
	}

	val := strings.TrimSpace(stdout)
	if val == "Linger=yes" {
		return true, nil
	}
	if val == "Linger=no" {
		return false, nil
	}
	return false, fmt.Errorf("unexpected linger value: %s", val)
}

// EnableLinger enables lingering for the current user.
func EnableLinger(cmd Commander) error {
	ctx := context.Background()
	_, _, err := cmd.Run(ctx, "loginctl", "enable-linger")
	return err
}

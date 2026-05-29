package systemd

import "context"

// Commander runs external commands (systemctl, loginctl, systemd-analyze).
type Commander interface {
	Run(ctx context.Context, args ...string) (stdout string, stderr string, err error)
	LookPath(name string) (string, error)
}

// UnitPrefix is the naming convention for wake-managed systemd units.
const UnitPrefix = "wake-"

// TimerInfo holds parsed output from systemctl list-timers.
type TimerInfo struct {
	Name     string // schedule name (without wake- prefix and .timer suffix)
	Calendar string // OnCalendar expression
	NextFire string // next elapse time
	LastFire string // last trigger time
	Active   string // active state
}

// ServiceStatus holds parsed output from systemctl show.
type ServiceStatus struct {
	ActiveState               string
	SubState                  string
	ExecMainStartTimestamp    string
	ExecMainExitTimestamp     string
	ExecMainStatus            int
	Result                    string
}

// TimerStatus holds parsed output from systemctl show for a timer.
type TimerStatus struct {
	ActiveState string
	NextElapse  string
	LastTrigger string
	Result      string
}

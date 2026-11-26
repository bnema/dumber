package port

import (
	"context"
	"time"
)

// SessionProcessStatus describes the process or thread currently holding a PID.
type SessionProcessStatus struct {
	// Running is false when no live process or thread holds the PID.
	Running bool
	// StartedAt is when the current holder started; set only when Running.
	StartedAt time.Time
}

// SessionProcessProbe inspects the PID recorded for a browser session.
type SessionProcessProbe interface {
	ProcessStatus(ctx context.Context, pid int) (SessionProcessStatus, error)
}

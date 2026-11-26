package port

import (
	"context"
	"time"
)

// SessionProcessStatus describes the process or thread currently holding a PID.
type SessionProcessStatus struct {
	// Running is false when no live process holds the PID; a thread ID is not a process.
	Running bool
	// StartedAt is when the current holder started; set only when Running.
	StartedAt time.Time
	// SameExecutable reports that the holder is a main process of the caller's binary,
	// not one of its subprocesses. It does not depend on the wall clock, unlike StartedAt.
	SameExecutable bool
}

// SessionProcessProbe inspects the PID recorded for a browser session.
type SessionProcessProbe interface {
	ProcessStatus(ctx context.Context, pid int) (SessionProcessStatus, error)
}

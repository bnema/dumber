package port

import "context"

// SessionProcessProbe checks whether a process recorded for a browser session is still alive.
type SessionProcessProbe interface {
	IsProcessAlive(ctx context.Context, pid int) (bool, error)
}

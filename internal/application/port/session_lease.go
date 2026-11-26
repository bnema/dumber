package port

import (
	"context"
	"io"

	"bnema.dev/dumber/internal/domain/entity"
)

// SessionLeaseState tells whether a browser session's owning process still runs.
type SessionLeaseState int

const (
	// SessionLeaseMissing means no lease file exists: the session predates leases,
	// its acquire failed, or the file was removed. Liveness must be inferred another way.
	SessionLeaseMissing SessionLeaseState = iota
	// SessionLeaseHeld means a live process still owns the session.
	SessionLeaseHeld
	// SessionLeaseFree means the owning process is gone.
	SessionLeaseFree
)

// SessionLeaseStore holds one lease per live browser session. The operating system
// releases a lease when its process exits, crash included, whatever its PID namespace.
type SessionLeaseStore interface {
	// Acquire takes the session's lease until the closer is closed or the process exits.
	Acquire(ctx context.Context, id entity.SessionID) (io.Closer, error)
	// Probe reports the state of the session's lease without keeping it.
	Probe(ctx context.Context, id entity.SessionID) (SessionLeaseState, error)
	// Remove deletes the lease of a session that has ended.
	Remove(ctx context.Context, id entity.SessionID) error
	// List returns the sessions that have a lease file, held or not.
	List(ctx context.Context) ([]entity.SessionID, error)
}

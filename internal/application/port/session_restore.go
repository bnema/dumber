package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// SessionRestoreLauncher delivers a restore to the profile host, or starts it.
type SessionRestoreLauncher interface {
	// The bool reports a cold browser start, not completed restoration.
	RestoreSession(context.Context, entity.SessionID) (started bool, err error)
}

// SessionRestoreRelay delivers restore intent without reducing it to a URL.
type SessionRestoreRelay interface {
	DeliverRestoreSession(context.Context, entity.SessionID) (bool, error)
}

// SessionRestoreOpener imports a snapshot into new windows in the host.
type SessionRestoreOpener interface {
	RestoreSession(context.Context, entity.SessionID) error
}

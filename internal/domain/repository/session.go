package repository

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

// SessionRepository persists session metadata.
type SessionRepository interface {
	Save(ctx context.Context, session *entity.Session) error
	FindByID(ctx context.Context, id entity.SessionID) (*entity.Session, error)

	// GetActive returns the most recent active browser session (if any).
	GetActive(ctx context.Context) (*entity.Session, error)
	GetRecent(ctx context.Context, limit int) ([]*entity.Session, error)
	MarkEnded(ctx context.Context, id entity.SessionID, endedAt time.Time) error

	// Delete removes a session record.
	Delete(ctx context.Context, id entity.SessionID) error

	// DeleteOldestExited deletes exited sessions beyond the keep limit.
	// Returns number of deleted sessions.
	DeleteOldestExited(ctx context.Context, keepCount int) (int64, error)

	// DeleteExitedBefore deletes exited sessions older than the cutoff time.
	// Returns number of deleted sessions.
	DeleteExitedBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

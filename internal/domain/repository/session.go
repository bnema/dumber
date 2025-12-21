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
}

package repository

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// SessionStateRepository persists session state snapshots.
type SessionStateRepository interface {
	// SaveSnapshot saves or updates a session state snapshot.
	SaveSnapshot(ctx context.Context, state *entity.SessionState) error

	// GetSnapshot returns the latest snapshot for a session.
	GetSnapshot(ctx context.Context, sessionID entity.SessionID) (*entity.SessionState, error)

	// DeleteSnapshot removes a session's snapshot.
	DeleteSnapshot(ctx context.Context, sessionID entity.SessionID) error

	// GetAllSnapshots returns all snapshots with summary info.
	// Used by ListSessionsUseCase.
	GetAllSnapshots(ctx context.Context) ([]*entity.SessionState, error)

	// GetTotalSnapshotsSize returns the total size of all session snapshots in bytes.
	GetTotalSnapshotsSize(ctx context.Context) (int64, error)
}

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"github.com/bnema/dumber/internal/logging"
)

type sessionStateRepo struct {
	db      *sql.DB
	queries *sqlc.Queries
}

// NewSessionStateRepository creates a new session state repository.
func NewSessionStateRepository(db *sql.DB) repository.SessionStateRepository {
	return &sessionStateRepo{
		db:      db,
		queries: sqlc.New(db),
	}
}

// SaveSnapshot saves or updates a session state snapshot.
func (r *sessionStateRepo) SaveSnapshot(ctx context.Context, state *entity.SessionState) error {
	log := logging.FromContext(ctx)
	if state == nil {
		return errors.New("session state cannot be nil")
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal session state")
		return err
	}

	log.Debug().
		Str("session_id", string(state.SessionID)).
		Int("tab_count", len(state.Tabs)).
		Int("pane_count", state.CountPanes()).
		Msg("saving session state snapshot")

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin snapshot transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Debug().Err(rollbackErr).Msg("snapshot rollback reported non-terminal error")
		}
	}()

	txQueries := r.queries.WithTx(tx)
	if err := txQueries.UpsertSessionState(ctx, sqlc.UpsertSessionStateParams{
		SessionID: string(state.SessionID),
		StateJson: string(stateJSON),
		Version:   int64(state.Version),
		TabCount:  int64(len(state.Tabs)),
		PaneCount: int64(state.CountPanes()),
		UpdatedAt: state.SavedAt,
	}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit snapshot transaction: %w", err)
	}

	return nil
}

// GetSnapshot returns the latest snapshot for a session.
func (r *sessionStateRepo) GetSnapshot(ctx context.Context, sessionID entity.SessionID) (*entity.SessionState, error) {
	row, err := r.queries.GetSessionState(ctx, string(sessionID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var state entity.SessionState
	if err := json.Unmarshal([]byte(row.StateJson), &state); err != nil {
		logging.FromContext(ctx).Error().Err(err).
			Str("session_id", string(sessionID)).
			Msg("failed to unmarshal session state")
		return nil, err
	}

	return &state, nil
}

// DeleteSnapshot removes a session's snapshot.
func (r *sessionStateRepo) DeleteSnapshot(ctx context.Context, sessionID entity.SessionID) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("session_id", string(sessionID)).Msg("deleting session state snapshot")
	return r.queries.DeleteSessionState(ctx, string(sessionID))
}

// GetAllSnapshots returns all snapshots with summary info.
func (r *sessionStateRepo) GetAllSnapshots(ctx context.Context) ([]*entity.SessionState, error) {
	rows, err := r.queries.GetAllSessionStates(ctx)
	if err != nil {
		return nil, err
	}

	states := make([]*entity.SessionState, 0, len(rows))
	for _, row := range rows {
		var state entity.SessionState
		if err := json.Unmarshal([]byte(row.StateJson), &state); err != nil {
			logging.FromContext(ctx).Warn().Err(err).
				Str("session_id", row.SessionID).
				Msg("skipping corrupted session state")
			continue
		}
		states = append(states, &state)
	}

	return states, nil
}

// GetTotalSnapshotsSize returns the total size of all session snapshots in bytes.
func (r *sessionStateRepo) GetTotalSnapshotsSize(ctx context.Context) (int64, error) {
	result, err := r.queries.GetTotalSessionStatesSize(ctx)
	if err != nil {
		return 0, err
	}
	// SQLite returns int64 for SUM, but COALESCE wraps it as interface{}
	switch v := result.(type) {
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	default:
		return 0, nil
	}
}

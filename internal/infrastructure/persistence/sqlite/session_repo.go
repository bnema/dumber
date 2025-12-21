package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"github.com/bnema/dumber/internal/logging"
)

type sessionRepo struct {
	queries *sqlc.Queries
}

func NewSessionRepository(db *sql.DB) repository.SessionRepository {
	return &sessionRepo{queries: sqlc.New(db)}
}

func (r *sessionRepo) Save(ctx context.Context, session *entity.Session) error {
	log := logging.FromContext(ctx)
	if err := session.Validate(); err != nil {
		return err
	}

	log.Debug().Str("session", string(session.ID)).Str("type", string(session.Type)).Msg("saving session")

	var endedAt sql.NullTime
	if session.EndedAt != nil {
		endedAt = sql.NullTime{Time: session.EndedAt.UTC(), Valid: true}
	}

	return r.queries.InsertSession(ctx, sqlc.InsertSessionParams{
		ID:        string(session.ID),
		Type:      string(session.Type),
		StartedAt: session.StartedAt.UTC(),
		EndedAt:   endedAt,
	})
}

func (r *sessionRepo) FindByID(ctx context.Context, id entity.SessionID) (*entity.Session, error) {
	row, err := r.queries.GetSessionByID(ctx, string(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return sessionFromRow(row), nil
}

func (r *sessionRepo) GetActive(ctx context.Context) (*entity.Session, error) {
	row, err := r.queries.GetActiveBrowserSession(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return sessionFromRow(row), nil
}

func (r *sessionRepo) GetRecent(ctx context.Context, limit int) ([]*entity.Session, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.queries.GetRecentSessions(ctx, int64(limit))
	if err != nil {
		return nil, err
	}

	sessions := make([]*entity.Session, len(rows))
	for i := range rows {
		sessions[i] = sessionFromRow(rows[i])
	}
	return sessions, nil
}

func (r *sessionRepo) MarkEnded(ctx context.Context, id entity.SessionID, endedAt time.Time) error {
	endedAt = endedAt.UTC()
	return r.queries.MarkSessionEnded(ctx, sqlc.MarkSessionEndedParams{
		EndedAt: sql.NullTime{Time: endedAt, Valid: true},
		ID:      string(id),
	})
}

func sessionFromRow(row sqlc.Session) *entity.Session {
	var endedAt *time.Time
	if row.EndedAt.Valid {
		t := row.EndedAt.Time.UTC()
		endedAt = &t
	}

	return &entity.Session{
		ID:        entity.SessionID(row.ID),
		Type:      entity.SessionType(row.Type),
		StartedAt: row.StartedAt.UTC(),
		EndedAt:   endedAt,
	}
}

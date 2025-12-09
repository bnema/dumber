package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"github.com/bnema/dumber/internal/logging"
)

type historyRepo struct {
	queries *sqlc.Queries
}

// NewHistoryRepository creates a new SQLite-backed history repository.
func NewHistoryRepository(db *sql.DB) repository.HistoryRepository {
	return &historyRepo{queries: sqlc.New(db)}
}

func (r *historyRepo) Save(ctx context.Context, entry *entity.HistoryEntry) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", entry.URL).Msg("saving history entry")

	return r.queries.UpsertHistory(ctx, sqlc.UpsertHistoryParams{
		Url:        entry.URL,
		Title:      sql.NullString{String: entry.Title, Valid: entry.Title != ""},
		FaviconUrl: sql.NullString{String: entry.FaviconURL, Valid: entry.FaviconURL != ""},
	})
}

func (r *historyRepo) FindByURL(ctx context.Context, url string) (*entity.HistoryEntry, error) {
	row, err := r.queries.GetHistoryByURL(ctx, url)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return historyFromRow(row), nil
}

func (r *historyRepo) Search(ctx context.Context, query string, limit int) ([]entity.HistoryMatch, error) {
	rows, err := r.queries.SearchHistory(ctx, sqlc.SearchHistoryParams{
		Column1: sql.NullString{String: query, Valid: true},
		Column2: sql.NullString{String: query, Valid: true},
		Limit:   int64(limit),
	})
	if err != nil {
		return nil, err
	}

	matches := make([]entity.HistoryMatch, len(rows))
	for i, row := range rows {
		matches[i] = entity.HistoryMatch{
			Entry: historyFromRow(row),
			Score: 1.0, // Basic score, could be enhanced with FTS ranking
		}
	}
	return matches, nil
}

func (r *historyRepo) GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	rows, err := r.queries.GetRecentHistory(ctx, sqlc.GetRecentHistoryParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i, row := range rows {
		entries[i] = historyFromRow(row)
	}
	return entries, nil
}

func (r *historyRepo) IncrementVisitCount(ctx context.Context, url string) error {
	return r.queries.IncrementVisitCount(ctx, url)
}

func (r *historyRepo) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteHistoryByID(ctx, id)
}

func (r *historyRepo) DeleteOlderThan(ctx context.Context, before time.Time) error {
	return r.queries.DeleteHistoryOlderThan(ctx, sql.NullTime{Time: before, Valid: true})
}

func (r *historyRepo) DeleteAll(ctx context.Context) error {
	return r.queries.DeleteAllHistory(ctx)
}

func historyFromRow(row sqlc.History) *entity.HistoryEntry {
	return &entity.HistoryEntry{
		ID:          row.ID,
		URL:         row.Url,
		Title:       row.Title.String,
		FaviconURL:  row.FaviconUrl.String,
		VisitCount:  row.VisitCount.Int64,
		LastVisited: row.LastVisited.Time,
		CreatedAt:   row.CreatedAt.Time,
	}
}

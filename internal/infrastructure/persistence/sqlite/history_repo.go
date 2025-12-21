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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return historyFromRow(row), nil
}

func (r *historyRepo) Search(ctx context.Context, query string, limit int) ([]entity.HistoryMatch, error) {
	log := logging.FromContext(ctx)

	// Use FTS5 full-text search for better accuracy
	// FTS5 query syntax: use * for prefix matching
	ftsQuery := query + "*"

	rows, err := r.queries.SearchHistoryFTS(ctx, sqlc.SearchHistoryFTSParams{
		Url:   ftsQuery,
		Limit: int64(limit),
	})
	if err != nil {
		log.Debug().Err(err).Str("query", query).Msg("FTS search failed, no results")
		// FTS5 returns error on no match or invalid query, return empty results
		return []entity.HistoryMatch{}, nil
	}

	matches := make([]entity.HistoryMatch, len(rows))
	for i := range rows {
		row := rows[i]
		matches[i] = entity.HistoryMatch{
			Entry: historyFromRow(row),
			Score: 1.0, // FTS5 already ranked by bm25
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
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
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

func (r *historyRepo) DeleteByDomain(ctx context.Context, domain string) error {
	return r.queries.DeleteHistoryByDomain(ctx, sqlc.DeleteHistoryByDomainParams{
		Column1: sql.NullString{String: domain, Valid: true},
		Column2: sql.NullString{String: domain, Valid: true},
		Column3: sql.NullString{String: domain, Valid: true},
		Column4: sql.NullString{String: domain, Valid: true},
	})
}

func (r *historyRepo) GetStats(ctx context.Context) (*entity.HistoryStats, error) {
	row, err := r.queries.GetHistoryStats(ctx)
	if err != nil {
		return nil, err
	}

	// Handle the interface{} type for total_visits
	var totalVisits int64
	switch v := row.TotalVisits.(type) {
	case int64:
		totalVisits = v
	case float64:
		totalVisits = int64(v)
	}

	return &entity.HistoryStats{
		TotalEntries: row.TotalEntries,
		TotalVisits:  totalVisits,
		UniqueDays:   row.UniqueDays,
	}, nil
}

func (r *historyRepo) GetDomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error) {
	rows, err := r.queries.GetDomainStats(ctx, int64(limit))
	if err != nil {
		return nil, err
	}

	stats := make([]*entity.DomainStat, len(rows))
	for i, row := range rows {
		var totalVisits int64
		if row.TotalVisits.Valid {
			totalVisits = int64(row.TotalVisits.Float64)
		}

		var lastVisit time.Time
		if v, ok := row.LastVisit.(string); ok && v != "" {
			lastVisit, _ = time.Parse("2006-01-02 15:04:05", v)
		}

		stats[i] = &entity.DomainStat{
			Domain:      row.Domain,
			PageCount:   row.PageCount,
			TotalVisits: totalVisits,
			LastVisit:   lastVisit,
		}
	}
	return stats, nil
}

func (r *historyRepo) GetHourlyDistribution(ctx context.Context) ([]*entity.HourlyDistribution, error) {
	rows, err := r.queries.GetHourlyDistribution(ctx)
	if err != nil {
		return nil, err
	}

	dist := make([]*entity.HourlyDistribution, len(rows))
	for i, row := range rows {
		dist[i] = &entity.HourlyDistribution{
			Hour:       int(row.Hour),
			VisitCount: row.VisitCount,
		}
	}
	return dist, nil
}

func (r *historyRepo) GetDailyVisitCount(ctx context.Context, daysAgo string) ([]*entity.DailyVisitCount, error) {
	rows, err := r.queries.GetDailyVisitCount(ctx, daysAgo)
	if err != nil {
		return nil, err
	}

	counts := make([]*entity.DailyVisitCount, len(rows))
	for i, row := range rows {
		var day string
		if v, ok := row.Day.(string); ok {
			day = v
		}

		var visits int64
		if row.Visits.Valid {
			visits = int64(row.Visits.Float64)
		}

		counts[i] = &entity.DailyVisitCount{
			Day:     day,
			Entries: row.Entries,
			Visits:  visits,
		}
	}
	return counts, nil
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

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

	// Split query into words and add prefix matching to each
	// This enables multi-word searches like "github issues" -> "github* issues*"
	words := strings.Fields(query)
	if len(words) == 0 {
		return []entity.HistoryMatch{}, nil
	}

	// Build FTS5 query: "word1* word2*" (implicit AND between terms)
	// Sanitize each word to remove FTS5 special characters
	parts := make([]string, len(words))
	for i, word := range words {
		parts[i] = sanitizeFTS5Word(word) + "*"
	}
	ftsQuery := strings.Join(parts, " ")

	// Search both URL and title columns, then merge results
	urlRows, urlErr := r.queries.SearchHistoryFTSUrl(ctx, sqlc.SearchHistoryFTSUrlParams{
		Query: ftsQuery,
		Limit: int64(limit),
	})
	if urlErr != nil {
		log.Debug().Err(urlErr).Str("query", query).Msg("FTS URL search failed")
		urlRows = []sqlc.History{}
	}

	titleRows, titleErr := r.queries.SearchHistoryFTSTitle(ctx, sqlc.SearchHistoryFTSTitleParams{
		Query: ftsQuery,
		Limit: int64(limit),
	})
	if titleErr != nil {
		log.Debug().Err(titleErr).Str("query", query).Msg("FTS title search failed")
		titleRows = []sqlc.History{}
	}

	// Merge results by interleaving URL and title matches for balanced results
	seen := make(map[int64]bool)
	var matches []entity.HistoryMatch

	i, j := 0, 0
	for len(matches) < limit && (i < len(urlRows) || j < len(titleRows)) {
		// Take next URL match if available
		if i < len(urlRows) {
			row := urlRows[i]
			i++
			if !seen[row.ID] {
				seen[row.ID] = true
				matches = append(matches, entity.HistoryMatch{
					Entry: historyFromRow(row),
					Score: 1.0,
				})
			}
		}

		// Take next title match if available
		if j < len(titleRows) && len(matches) < limit {
			row := titleRows[j]
			j++
			if !seen[row.ID] {
				seen[row.ID] = true
				matches = append(matches, entity.HistoryMatch{
					Entry: historyFromRow(row),
					Score: 1.0,
				})
			}
		}
	}

	return matches, nil
}

// sanitizeFTS5Word removes FTS5 special characters from a search word.
// FTS5 special chars: " * ( ) : ^ - AND OR NOT NEAR
func sanitizeFTS5Word(word string) string {
	// Remove characters that have special meaning in FTS5
	var result strings.Builder
	for _, r := range word {
		switch r {
		case '"', '*', '(', ')', ':', '^', '-':
			// Skip FTS5 special characters
		default:
			result.WriteRune(r)
		}
	}
	return result.String()
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

func (r *historyRepo) GetRecentSince(ctx context.Context, days int) ([]*entity.HistoryEntry, error) {
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive, got %d", days)
	}
	// Format: "-N days" for SQLite datetime modifier
	daysModifier := fmt.Sprintf("-%d days", days)
	rows, err := r.queries.GetRecentHistorySince(ctx, daysModifier)
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
	}
	return entries, nil
}

func (r *historyRepo) GetMostVisited(ctx context.Context, days int) ([]*entity.HistoryEntry, error) {
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive, got %d", days)
	}
	// Format: "-N days" for SQLite datetime modifier
	daysModifier := fmt.Sprintf("-%d days", days)
	rows, err := r.queries.GetMostVisited(ctx, daysModifier)
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
	}
	return entries, nil
}

func (r *historyRepo) GetAllRecentHistory(ctx context.Context) ([]*entity.HistoryEntry, error) {
	rows, err := r.queries.GetAllRecentHistory(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
	}
	return entries, nil
}

func (r *historyRepo) GetAllMostVisited(ctx context.Context) ([]*entity.HistoryEntry, error) {
	rows, err := r.queries.GetAllMostVisited(ctx)
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

package repository

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

// HistoryRepository defines operations for browsing history persistence.
type HistoryRepository interface {
	// Save creates or updates a history entry (upsert).
	Save(ctx context.Context, entry *entity.HistoryEntry) error

	// FindByURL retrieves a history entry by its URL.
	FindByURL(ctx context.Context, url string) (*entity.HistoryEntry, error)

	// Search performs a fuzzy search on history entries.
	Search(ctx context.Context, query string, limit int) ([]entity.HistoryMatch, error)

	// GetRecent retrieves recent history entries with pagination.
	GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error)

	// GetRecentSince retrieves history entries visited within the last N days.
	// days must be > 0.
	GetRecentSince(ctx context.Context, days int) ([]*entity.HistoryEntry, error)

	// GetMostVisited retrieves history entries sorted by visit count within the last N days.
	// days must be > 0.
	GetMostVisited(ctx context.Context, days int) ([]*entity.HistoryEntry, error)

	// GetAllRecentHistory retrieves all history entries sorted by recency.
	GetAllRecentHistory(ctx context.Context) ([]*entity.HistoryEntry, error)

	// GetAllMostVisited retrieves all history entries sorted by visit count.
	GetAllMostVisited(ctx context.Context) ([]*entity.HistoryEntry, error)

	// IncrementVisitCount increments the visit count for a URL.
	IncrementVisitCount(ctx context.Context, url string) error

	// Delete removes a single history entry by ID.
	Delete(ctx context.Context, id int64) error

	// DeleteOlderThan removes entries older than the given time.
	DeleteOlderThan(ctx context.Context, before time.Time) error

	// DeleteAll removes all history entries.
	DeleteAll(ctx context.Context) error

	// DeleteByDomain removes all history entries for a domain.
	DeleteByDomain(ctx context.Context, domain string) error

	// GetStats retrieves overall history statistics.
	GetStats(ctx context.Context) (*entity.HistoryStats, error)

	// GetDomainStats retrieves per-domain statistics.
	GetDomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error)

	// GetHourlyDistribution retrieves hourly visit distribution.
	GetHourlyDistribution(ctx context.Context) ([]*entity.HourlyDistribution, error)

	// GetDailyVisitCount retrieves daily visit counts for the given period.
	GetDailyVisitCount(ctx context.Context, daysAgo string) ([]*entity.DailyVisitCount, error)
}

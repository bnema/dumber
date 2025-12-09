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

	// IncrementVisitCount increments the visit count for a URL.
	IncrementVisitCount(ctx context.Context, url string) error

	// Delete removes a single history entry by ID.
	Delete(ctx context.Context, id int64) error

	// DeleteOlderThan removes entries older than the given time.
	DeleteOlderThan(ctx context.Context, before time.Time) error

	// DeleteAll removes all history entries.
	DeleteAll(ctx context.Context) error
}

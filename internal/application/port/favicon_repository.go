package port

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/domain/favicon"
)

// FaviconRepository persists favicon metadata and supports fallback-key lookups.
// Get returns metadata for one key, FindFirst returns the first matching key, Upsert writes metadata,
// UpdateLastChecked records refresh state, and Delete removes metadata.
type FaviconRepository interface {
	Get(ctx context.Context, key favicon.Key) (*favicon.Metadata, error)
	FindFirst(ctx context.Context, keys []favicon.Key) (*favicon.Metadata, error)
	Upsert(ctx context.Context, meta favicon.Metadata) error
	UpdateLastChecked(ctx context.Context, key favicon.Key, contentHash string, checkedAt time.Time) error
	Delete(ctx context.Context, key favicon.Key) error
}

package port

import (
	"context"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
)

// HistorySidebarHistory provides the narrow history operations needed by the
// native GTK history sidebar.
type HistorySidebarHistory interface {
	GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error)
	Search(ctx context.Context, input dto.HistorySearchInput) (*dto.HistorySearchOutput, error)
	Delete(ctx context.Context, id int64) error
}

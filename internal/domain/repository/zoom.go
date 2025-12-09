package repository

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// ZoomRepository defines operations for per-domain zoom level persistence.
type ZoomRepository interface {
	// Get retrieves the zoom level for a domain.
	// Returns nil if no custom zoom is set.
	Get(ctx context.Context, domain string) (*entity.ZoomLevel, error)

	// Set saves or updates the zoom level for a domain.
	Set(ctx context.Context, level *entity.ZoomLevel) error

	// Delete removes the custom zoom level for a domain.
	Delete(ctx context.Context, domain string) error

	// GetAll retrieves all custom zoom levels.
	GetAll(ctx context.Context) ([]*entity.ZoomLevel, error)
}

package cache

import (
	"context"

	"github.com/bnema/dumber/internal/cache/generic"
	"github.com/bnema/dumber/internal/db"
)

// ZoomDBOperations implements DatabaseOperations for zoom level cache.
// Handles loading, persisting, and deleting zoom levels from the database.
type ZoomDBOperations struct {
	queries db.DatabaseQuerier
}

// NewZoomDBOperations creates a new ZoomDBOperations instance.
func NewZoomDBOperations(queries db.DatabaseQuerier) *ZoomDBOperations {
	return &ZoomDBOperations{
		queries: queries,
	}
}

// LoadAll loads all zoom levels from the database.
// Returns a map of domain -> zoom factor.
func (z *ZoomDBOperations) LoadAll(ctx context.Context) (map[string]float64, error) {
	levels, err := z.queries.ListZoomLevels(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]float64, len(levels))
	for _, level := range levels {
		result[level.Domain] = level.ZoomFactor
	}

	return result, nil
}

// Persist saves a zoom level to the database.
// Uses UPSERT (INSERT OR UPDATE) to handle both new and existing entries.
func (z *ZoomDBOperations) Persist(ctx context.Context, domain string, zoomFactor float64) error {
	return z.queries.SetZoomLevel(ctx, domain, zoomFactor)
}

// Delete removes a zoom level from the database.
func (z *ZoomDBOperations) Delete(ctx context.Context, domain string) error {
	return z.queries.DeleteZoomLevel(ctx, domain)
}

// ZoomCache is a specialized cache for zoom levels.
// It wraps GenericCache with domain-specific helper methods.
type ZoomCache struct {
	*generic.GenericCache[string, float64]
}

// NewZoomCache creates a new zoom level cache.
func NewZoomCache(queries db.DatabaseQuerier) *ZoomCache {
	dbOps := NewZoomDBOperations(queries)
	return &ZoomCache{
		GenericCache: generic.NewGenericCache(dbOps),
	}
}

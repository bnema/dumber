package cache

import (
	"context"
	"database/sql"

	"github.com/bnema/dumber/internal/cache/generic"
	"github.com/bnema/dumber/internal/db"
)

// FavoritesDBOperations implements DatabaseOperations for favorites cache.
// Handles loading, persisting, and deleting favorites from the database.
type FavoritesDBOperations struct {
	queries db.DatabaseQuerier
}

// NewFavoritesDBOperations creates a new FavoritesDBOperations instance.
func NewFavoritesDBOperations(queries db.DatabaseQuerier) *FavoritesDBOperations {
	return &FavoritesDBOperations{
		queries: queries,
	}
}

// LoadAll loads all favorites from the database.
// Returns a map of URL -> Favorite.
func (f *FavoritesDBOperations) LoadAll(ctx context.Context) (map[string]db.Favorite, error) {
	favorites, err := f.queries.GetAllFavorites(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]db.Favorite, len(favorites))
	for _, fav := range favorites {
		result[fav.Url] = fav
	}

	return result, nil
}

// Persist saves a favorite to the database.
// Uses UPSERT logic: if URL exists, updates metadata; otherwise creates new.
func (f *FavoritesDBOperations) Persist(ctx context.Context, url string, favorite db.Favorite) error {
	// Check if favorite already exists
	_, err := f.queries.GetFavoriteByURL(ctx, url)
	if err == sql.ErrNoRows {
		// Create new favorite
		return f.queries.CreateFavorite(ctx, url, favorite.Title, favorite.FaviconUrl)
	} else if err != nil {
		return err
	}

	// Update existing favorite
	return f.queries.UpdateFavorite(ctx, favorite.Title, favorite.FaviconUrl, url)
}

// Delete removes a favorite from the database.
func (f *FavoritesDBOperations) Delete(ctx context.Context, url string) error {
	return f.queries.DeleteFavorite(ctx, url)
}

// FavoritesCache is a specialized cache for favorites.
// It wraps GenericCache with favorites-specific helper methods.
type FavoritesCache struct {
	*generic.GenericCache[string, db.Favorite]
}

// NewFavoritesCache creates a new favorites cache.
func NewFavoritesCache(queries db.DatabaseQuerier) *FavoritesCache {
	dbOps := NewFavoritesDBOperations(queries)
	return &FavoritesCache{
		GenericCache: generic.NewGenericCache(dbOps),
	}
}

// GetAll returns all favorites as a slice, ordered by position.
// This is a convenience method for the frontend.
func (f *FavoritesCache) GetAll() []db.Favorite {
	// Get all favorites from the cache using the List method
	favorites := f.GenericCache.List()

	// Sort by position
	// Note: favorites should already be ordered by position from DB load,
	// but this ensures consistency even if items were added at runtime.
	for i := 0; i < len(favorites)-1; i++ {
		for j := i + 1; j < len(favorites); j++ {
			if favorites[i].Position > favorites[j].Position {
				favorites[i], favorites[j] = favorites[j], favorites[i]
			}
		}
	}

	return favorites
}

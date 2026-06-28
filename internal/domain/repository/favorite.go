package repository

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// FavoriteRepository defines operations for bookmark persistence.
type FavoriteRepository interface {
	// Save creates or updates a favorite.
	Save(ctx context.Context, fav *entity.Favorite) error

	// FindByID retrieves a favorite by its ID.
	FindByID(ctx context.Context, id entity.FavoriteID) (*entity.Favorite, error)

	// FindByURL retrieves a favorite by its URL.
	FindByURL(ctx context.Context, url string) (*entity.Favorite, error)

	// GetAll retrieves all favorites.
	GetAll(ctx context.Context) ([]*entity.Favorite, error)

	// GetByTag retrieves favorites with a specific tag.
	GetByTag(ctx context.Context, tagID entity.TagID) ([]*entity.Favorite, error)

	// GetByShortcut retrieves the favorite with the given shortcut key (1-9).
	GetByShortcut(ctx context.Context, key int) (*entity.Favorite, error)

	// UpdatePosition updates a favorite's position.
	UpdatePosition(ctx context.Context, id entity.FavoriteID, position int) error

	// SetShortcut assigns a keyboard shortcut (1-9) or nil to remove.
	SetShortcut(ctx context.Context, id entity.FavoriteID, key *int) error

	// Delete removes a favorite by ID.
	Delete(ctx context.Context, id entity.FavoriteID) error
}

// TagRepository defines operations for tag persistence.
type TagRepository interface {
	// Save creates or updates a tag.
	Save(ctx context.Context, tag *entity.Tag) error

	// FindByID retrieves a tag by its ID.
	FindByID(ctx context.Context, id entity.TagID) (*entity.Tag, error)

	// FindByName retrieves a tag by its name.
	FindByName(ctx context.Context, name string) (*entity.Tag, error)

	// GetAll retrieves all tags.
	GetAll(ctx context.Context) ([]*entity.Tag, error)

	// AssignToFavorite associates a tag with a favorite.
	AssignToFavorite(ctx context.Context, tagID entity.TagID, favID entity.FavoriteID) error

	// RemoveFromFavorite removes a tag association from a favorite.
	RemoveFromFavorite(ctx context.Context, tagID entity.TagID, favID entity.FavoriteID) error

	// GetForFavorite retrieves all tags associated with a favorite.
	GetForFavorite(ctx context.Context, favID entity.FavoriteID) ([]*entity.Tag, error)

	// Delete removes a tag by ID.
	Delete(ctx context.Context, id entity.TagID) error
}

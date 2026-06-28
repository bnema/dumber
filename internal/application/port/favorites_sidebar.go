package port

import (
	"context"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
)

// FavoritesSidebarFavorites provides favorite and tag operations needed by the native favorites sidebar.
type FavoritesSidebarFavorites interface {
	GetAll(ctx context.Context) ([]*entity.Favorite, error)
	GetAllTags(ctx context.Context) ([]*entity.Tag, error)
	AddFavorite(ctx context.Context, input dto.FavoriteCreateInput) (*entity.Favorite, error)
	UpdateFavorite(ctx context.Context, input dto.FavoriteUpdateInput) (*entity.Favorite, error)
	DeleteFavorite(ctx context.Context, id entity.FavoriteID) error
	SetShortcut(ctx context.Context, id entity.FavoriteID, key *int) error
	TagFavorite(ctx context.Context, favID entity.FavoriteID, tagID entity.TagID) error
	UntagFavorite(ctx context.Context, favID entity.FavoriteID, tagID entity.TagID) error
}

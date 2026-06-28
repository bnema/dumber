package port

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
)

// HomepageHistory provides history operations needed by the WebUI homepage handlers.
type HomepageHistory interface {
	GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error)
	GetRecentByDomain(ctx context.Context, domain string, limit, offset int) ([]*entity.HistoryEntry, error)
	GetRecentWindow(ctx context.Context, before time.Time, beforeID int64, domain string) (*entity.HistoryWindow, error)
	Search(ctx context.Context, input dto.HistorySearchInput) (*dto.HistorySearchOutput, error)
	Delete(ctx context.Context, id int64) error
	ClearRange(ctx context.Context, rangeID string) error
	ClearAll(ctx context.Context) error
	ClearOlderThan(ctx context.Context, before time.Time) error
	GetStats(ctx context.Context) (*entity.HistoryStats, error)
	GetAnalytics(ctx context.Context) (*entity.HistoryAnalytics, error)
	GetDomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error)
	DeleteByDomain(ctx context.Context, domain string) error
}

// HomepageFavorites provides favorite and tag operations needed by the WebUI homepage handlers.
type HomepageFavorites interface {
	GetAll(ctx context.Context) ([]*entity.Favorite, error)
	AddFavorite(ctx context.Context, input dto.FavoriteCreateInput) (*entity.Favorite, error)
	UpdateFavorite(ctx context.Context, input dto.FavoriteUpdateInput) (*entity.Favorite, error)
	DeleteFavorite(ctx context.Context, id entity.FavoriteID) error
	SetShortcut(ctx context.Context, id entity.FavoriteID, key *int) error
	GetByShortcut(ctx context.Context, key int) (*entity.Favorite, error)
	GetAllTags(ctx context.Context) ([]*entity.Tag, error)
	AddTag(ctx context.Context, name, color string) (*entity.Tag, error)
	DeleteTag(ctx context.Context, id entity.TagID) error
	UpdateTag(ctx context.Context, id entity.TagID, name, color string) error
	TagFavorite(ctx context.Context, favID entity.FavoriteID, tagID entity.TagID) error
	UntagFavorite(ctx context.Context, favID entity.FavoriteID, tagID entity.TagID) error
}

package port

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

// HistorySearchInput holds search parameters (moved from usecase.SearchInput).
type HistorySearchInput struct {
	Query string
	Limit int
}

// HistorySearchOutput holds search results (moved from usecase.SearchOutput).
type HistorySearchOutput struct {
	Matches []entity.HistoryMatch
}

// HomepageHistory provides history operations needed by the WebUI homepage handlers.
type HomepageHistory interface {
	GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error)
	Search(ctx context.Context, input HistorySearchInput) (*HistorySearchOutput, error)
	Delete(ctx context.Context, id int64) error
	ClearRange(ctx context.Context, rangeID string) error
	ClearAll(ctx context.Context) error
	ClearOlderThan(ctx context.Context, before time.Time) error
	GetAnalytics(ctx context.Context) (*entity.HistoryAnalytics, error)
	GetDomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error)
	DeleteByDomain(ctx context.Context, domain string) error
}

// FavoriteCreateInput contains fields needed to create a favorite from a UI port.
type FavoriteCreateInput struct {
	URL        string           `json:"url"`
	Title      string           `json:"title"`
	FaviconURL string           `json:"favicon_url"`
	FolderID   *entity.FolderID `json:"folder_id"`
	Tags       []entity.TagID   `json:"tags"`
}

// FavoriteUpdateInput contains editable favorite metadata.
type FavoriteUpdateInput struct {
	ID          entity.FavoriteID `json:"id"`
	Title       string            `json:"title"`
	FaviconURL  string            `json:"favicon_url"`
	FolderID    *entity.FolderID  `json:"folder_id"`
	ShortcutKey *int              `json:"shortcut_key"`
}

// HomepageFavorites provides favorites/folders/tags operations
// needed by the WebUI homepage handlers.
type HomepageFavorites interface {
	GetAll(ctx context.Context) ([]*entity.Favorite, error)
	AddFavorite(ctx context.Context, input FavoriteCreateInput) (*entity.Favorite, error)
	UpdateFavorite(ctx context.Context, input FavoriteUpdateInput) (*entity.Favorite, error)
	DeleteFavorite(ctx context.Context, id entity.FavoriteID) error
	SetShortcut(ctx context.Context, id entity.FavoriteID, key *int) error
	GetByShortcut(ctx context.Context, key int) (*entity.Favorite, error)
	Move(ctx context.Context, id entity.FavoriteID, folderID *entity.FolderID) error
	GetAllFolders(ctx context.Context) ([]*entity.Folder, error)
	CreateFolder(ctx context.Context, name string, parentID *entity.FolderID) (*entity.Folder, error)
	DeleteFolder(ctx context.Context, id entity.FolderID) error
	UpdateFolder(ctx context.Context, id entity.FolderID, name, icon string) error
	GetAllTags(ctx context.Context) ([]*entity.Tag, error)
	AddTag(ctx context.Context, name, color string) (*entity.Tag, error)
	DeleteTag(ctx context.Context, id entity.TagID) error
	UpdateTag(ctx context.Context, id entity.TagID, name, color string) error
	TagFavorite(ctx context.Context, favID entity.FavoriteID, tagID entity.TagID) error
	UntagFavorite(ctx context.Context, favID entity.FavoriteID, tagID entity.TagID) error
}

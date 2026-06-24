package port

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
)

// SystemviewHistoryService exposes history operations for the systemviews history route.
type SystemviewHistoryService interface {
	Timeline(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error)
	TimelineByDomain(ctx context.Context, domain string, limit, offset int) ([]*entity.HistoryEntry, error)
	TimelineWindow(ctx context.Context, before time.Time, beforeID int64, domain string) (*entity.HistoryWindow, error)
	Search(ctx context.Context, query string, limit int) ([]*entity.HistoryEntry, error)
	DeleteEntry(ctx context.Context, id int64) error
	DeleteRange(ctx context.Context, rangeID string) error
	Stats(ctx context.Context) (*entity.HistoryStats, error)
	Analytics(ctx context.Context) (*entity.HistoryAnalytics, error)
	DomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error)
	DeleteDomain(ctx context.Context, domain string) error
}

// SystemviewFavoritesService exposes favorite, folder, and tag operations.
type SystemviewFavoritesService interface {
	List(ctx context.Context) ([]*entity.Favorite, error)
	CreateFavorite(ctx context.Context, input dto.FavoriteCreateInput) (*entity.Favorite, error)
	UpdateFavorite(ctx context.Context, input dto.FavoriteUpdateInput) (*entity.Favorite, error)
	DeleteFavorite(ctx context.Context, id int64) error
	ListFolders(ctx context.Context) ([]*entity.Folder, error)
	ListTags(ctx context.Context) ([]*entity.Tag, error)
	SetShortcut(ctx context.Context, favoriteID int64, shortcutKey *int) error
	SetFolder(ctx context.Context, favoriteID int64, folderID *int64) error
	CreateFolder(ctx context.Context, name, icon string, parentID *int64) (*entity.Folder, error)
	UpdateFolder(ctx context.Context, id int64, name, icon string) error
	DeleteFolder(ctx context.Context, id int64) error
	CreateTag(ctx context.Context, name, color string) (*entity.Tag, error)
	UpdateTag(ctx context.Context, id int64, name, color string) error
	DeleteTag(ctx context.Context, id int64) error
	AssignTag(ctx context.Context, favoriteID, tagID int64) error
	RemoveTag(ctx context.Context, favoriteID, tagID int64) error
}

// SystemviewConfigService exposes config and keybinding operations for the systemviews UI.
type SystemviewConfigService interface {
	Current(ctx context.Context) (dto.SystemviewConfigPayload, error)
	Default(ctx context.Context) (dto.SystemviewConfigPayload, error)
	Save(ctx context.Context, cfg dto.WebUIConfig) error
	GetKeybindings(ctx context.Context) (KeybindingsConfig, error)
	SetKeybinding(ctx context.Context, req SetKeybindingRequest) (SetKeybindingResponse, error)
	ResetKeybinding(ctx context.Context, req ResetKeybindingRequest) error
	ResetAllKeybindings(ctx context.Context) error
}

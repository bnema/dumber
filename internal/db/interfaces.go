package db

import (
	"context"
	"database/sql"
)

//go:generate mockgen -source=interfaces.go -destination=mocks/mock_db.go

// ZoomQuerier defines the interface for zoom-related database operations
type ZoomQuerier interface {
	GetZoomLevel(ctx context.Context, domain string) (float64, error)
	SetZoomLevel(ctx context.Context, domain string, zoomLevel float64) error
	DeleteZoomLevel(ctx context.Context, domain string) error
	ListZoomLevels(ctx context.Context) ([]ZoomLevel, error)
}

// HistoryQuerier defines the interface for history-related database operations
type HistoryQuerier interface {
	GetHistory(ctx context.Context, limit int64) ([]History, error)
	GetHistoryEntry(ctx context.Context, url string) (History, error)
	GetHistoryWithOffset(ctx context.Context, limit int64, offset int64) ([]History, error)
	GetMostVisited(ctx context.Context, limit int64) ([]History, error)
	SearchHistory(ctx context.Context, column1 sql.NullString, column2 sql.NullString, limit int64) ([]History, error)
	AddOrUpdateHistory(ctx context.Context, url string, title sql.NullString) error
	UpdateHistoryFavicon(ctx context.Context, faviconUrl sql.NullString, url string) error
	DeleteHistory(ctx context.Context, id int64) error
	DeleteAllHistory(ctx context.Context) error
}

// CertificateQuerier defines the interface for certificate validation-related database operations
type CertificateQuerier interface {
	ListCertificateValidations(ctx context.Context) ([]CertificateValidation, error)
	GetCertificateValidation(ctx context.Context, hostname string, certificateHash string) (CertificateValidation, error)
	GetCertificateValidationByHostname(ctx context.Context, hostname string) (CertificateValidation, error)
	StoreCertificateValidation(ctx context.Context, hostname string, certificateHash string, userDecision string, expiresAt sql.NullTime) error
	DeleteCertificateValidation(ctx context.Context, hostname string, certificateHash string) error
	DeleteExpiredCertificateValidations(ctx context.Context) error
}

// FavoritesQuerier defines the interface for favorites-related database operations
type FavoritesQuerier interface {
	GetAllFavorites(ctx context.Context) ([]Favorite, error)
	GetFavoriteByURL(ctx context.Context, url string) (Favorite, error)
	CreateFavorite(ctx context.Context, url string, title sql.NullString, faviconUrl sql.NullString) error
	UpdateFavorite(ctx context.Context, title sql.NullString, faviconUrl sql.NullString, url string) error
	DeleteFavorite(ctx context.Context, url string) error
	IsFavorite(ctx context.Context, url string) (int64, error)
	UpdateFavoritePosition(ctx context.Context, position int64, url string) error
	GetFavoriteCount(ctx context.Context) (int64, error)
}

// FolderQuerier defines the interface for favorite folder operations
type FolderQuerier interface {
	GetAllFolders(ctx context.Context) ([]FavoriteFolder, error)
	GetFolderByID(ctx context.Context, id int64) (FavoriteFolder, error)
	CreateFolder(ctx context.Context, name string, icon sql.NullString) (FavoriteFolder, error)
	UpdateFolder(ctx context.Context, name string, icon sql.NullString, id int64) error
	DeleteFolder(ctx context.Context, id int64) error
	GetFavoritesInFolder(ctx context.Context, folderID sql.NullInt64) ([]Favorite, error)
	GetFavoritesWithoutFolder(ctx context.Context) ([]Favorite, error)
	SetFavoriteFolder(ctx context.Context, folderID sql.NullInt64, id int64) error
	ClearFavoriteFolder(ctx context.Context, id int64) error
}

// TagQuerier defines the interface for favorite tag operations
type TagQuerier interface {
	GetAllTags(ctx context.Context) ([]FavoriteTag, error)
	GetTagByID(ctx context.Context, id int64) (FavoriteTag, error)
	GetTagByName(ctx context.Context, name string) (FavoriteTag, error)
	CreateTag(ctx context.Context, name string, color string) (FavoriteTag, error)
	UpdateTag(ctx context.Context, name string, color string, id int64) error
	DeleteTag(ctx context.Context, id int64) error
	AssignTag(ctx context.Context, favoriteID int64, tagID int64) error
	RemoveTag(ctx context.Context, favoriteID int64, tagID int64) error
	GetTagsForFavorite(ctx context.Context, favoriteID int64) ([]FavoriteTag, error)
	GetFavoritesWithTag(ctx context.Context, tagID int64) ([]Favorite, error)
	ClearTagsFromFavorite(ctx context.Context, favoriteID int64) error
}

// ShortcutQuerier defines the interface for favorite shortcut operations
type ShortcutQuerier interface {
	SetFavoriteShortcut(ctx context.Context, shortcutKey sql.NullInt64, id int64) error
	ClearFavoriteShortcut(ctx context.Context, id int64) error
	GetFavoriteByShortcut(ctx context.Context, shortcutKey sql.NullInt64) (Favorite, error)
	ClearShortcutFromOthers(ctx context.Context, shortcutKey sql.NullInt64) error
}

// HistoryExtendedQuerier defines the interface for extended history operations
type HistoryExtendedQuerier interface {
	GetHistoryTimeline(ctx context.Context, limit int64, offset int64) ([]GetHistoryTimelineRow, error)
	GetHistoryByDateRange(ctx context.Context, lastVisited sql.NullTime, lastVisited2 sql.NullTime) ([]History, error)
	GetHistoryDates(ctx context.Context, limit int64) ([]interface{}, error)
	GetHistoryStats(ctx context.Context) (GetHistoryStatsRow, error)
	GetDomainStats(ctx context.Context, limit int64) ([]GetDomainStatsRow, error)
	GetHourlyDistribution(ctx context.Context) ([]GetHourlyDistributionRow, error)
	GetDailyVisitCount(ctx context.Context, date interface{}) ([]GetDailyVisitCountRow, error)
	DeleteHistoryLastHour(ctx context.Context) error
	DeleteHistoryLastDay(ctx context.Context) error
	DeleteHistoryLastWeek(ctx context.Context) error
	DeleteHistoryLastMonth(ctx context.Context) error
	DeleteHistoryByDomain(ctx context.Context, col1 sql.NullString, col2 sql.NullString, col3 sql.NullString, col4 sql.NullString) error
	DeleteHistoryOlderThan(ctx context.Context, days string) error
}

// ContentWhitelistQuerier defines the interface for content filtering whitelist operations
type ContentWhitelistQuerier interface {
	GetAllWhitelistedDomains(ctx context.Context) ([]string, error)
	AddToWhitelist(ctx context.Context, domain string) error
	RemoveFromWhitelist(ctx context.Context, domain string) error
	IsWhitelisted(ctx context.Context, domain string) (int64, error)
}

// DatabaseQuerier combines all database operation interfaces
type DatabaseQuerier interface {
	ZoomQuerier
	HistoryQuerier
	CertificateQuerier
	FavoritesQuerier
	FolderQuerier
	TagQuerier
	ShortcutQuerier
	HistoryExtendedQuerier
	ContentWhitelistQuerier
}

// Ensure that *Queries implements DatabaseQuerier interface
var _ DatabaseQuerier = (*Queries)(nil)

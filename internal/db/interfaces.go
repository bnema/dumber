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

// DatabaseQuerier combines all database operation interfaces
type DatabaseQuerier interface {
	ZoomQuerier
	HistoryQuerier
	CertificateQuerier
	FavoritesQuerier
}

// Ensure that *Queries implements DatabaseQuerier interface
var _ DatabaseQuerier = (*Queries)(nil)

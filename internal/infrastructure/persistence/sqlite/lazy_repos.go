// Package sqlite provides SQLite implementations of domain repositories.
//
// # Lazy Repository Infrastructure
//
// This file contains lazy-loading repository wrappers that defer database initialization
// until first access. While currently unused (the application uses eager initialization
// via OpenDatabase in RunParallelDBWebKit), this infrastructure is kept for potential
// future optimization scenarios:
//
//   - CLI commands that may not need database access
//   - Further startup latency reduction by deferring DB init past first paint
//   - Testing with mock DatabaseProvider implementations
//
// The lazy wrappers implement the same repository interfaces as their eager counterparts,
// making them drop-in replacements when lazy initialization becomes beneficial.
package sqlite

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
)

// LazyHistoryRepository wraps a history repository with lazy database initialization.
type LazyHistoryRepository struct {
	provider port.DatabaseProvider
	repo     repository.HistoryRepository
	once     sync.Once
	initErr  error
}

// NewLazyHistoryRepository creates a lazy-loading history repository.
func NewLazyHistoryRepository(provider port.DatabaseProvider) repository.HistoryRepository {
	return &LazyHistoryRepository{provider: provider}
}

func (r *LazyHistoryRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewHistoryRepository(db)
	})
	return r.initErr
}

func (r *LazyHistoryRepository) Save(ctx context.Context, entry *entity.HistoryEntry) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Save(ctx, entry)
}

func (r *LazyHistoryRepository) FindByURL(ctx context.Context, url string) (*entity.HistoryEntry, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.FindByURL(ctx, url)
}

func (r *LazyHistoryRepository) Search(ctx context.Context, query string, limit int) ([]entity.HistoryMatch, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.Search(ctx, query, limit)
}

func (r *LazyHistoryRepository) GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetRecent(ctx, limit, offset)
}

func (r *LazyHistoryRepository) GetRecentSince(ctx context.Context, days int) ([]*entity.HistoryEntry, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetRecentSince(ctx, days)
}

func (r *LazyHistoryRepository) GetMostVisited(ctx context.Context, days int) ([]*entity.HistoryEntry, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetMostVisited(ctx, days)
}

func (r *LazyHistoryRepository) IncrementVisitCount(ctx context.Context, url string) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.IncrementVisitCount(ctx, url)
}

func (r *LazyHistoryRepository) Delete(ctx context.Context, id int64) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Delete(ctx, id)
}

func (r *LazyHistoryRepository) DeleteOlderThan(ctx context.Context, before time.Time) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.DeleteOlderThan(ctx, before)
}

func (r *LazyHistoryRepository) DeleteAll(ctx context.Context) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.DeleteAll(ctx)
}

func (r *LazyHistoryRepository) DeleteByDomain(ctx context.Context, domain string) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.DeleteByDomain(ctx, domain)
}

func (r *LazyHistoryRepository) GetStats(ctx context.Context) (*entity.HistoryStats, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetStats(ctx)
}

func (r *LazyHistoryRepository) GetDomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetDomainStats(ctx, limit)
}

func (r *LazyHistoryRepository) GetHourlyDistribution(ctx context.Context) ([]*entity.HourlyDistribution, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetHourlyDistribution(ctx)
}

func (r *LazyHistoryRepository) GetDailyVisitCount(ctx context.Context, daysAgo string) ([]*entity.DailyVisitCount, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetDailyVisitCount(ctx, daysAgo)
}

// LazyFavoriteRepository wraps a favorite repository with lazy database initialization.
type LazyFavoriteRepository struct {
	provider port.DatabaseProvider
	repo     repository.FavoriteRepository
	once     sync.Once
	initErr  error
}

// NewLazyFavoriteRepository creates a lazy-loading favorite repository.
func NewLazyFavoriteRepository(provider port.DatabaseProvider) repository.FavoriteRepository {
	return &LazyFavoriteRepository{provider: provider}
}

func (r *LazyFavoriteRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewFavoriteRepository(db)
	})
	return r.initErr
}

func (r *LazyFavoriteRepository) Save(ctx context.Context, fav *entity.Favorite) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Save(ctx, fav)
}

func (r *LazyFavoriteRepository) FindByID(ctx context.Context, id entity.FavoriteID) (*entity.Favorite, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.FindByID(ctx, id)
}

func (r *LazyFavoriteRepository) FindByURL(ctx context.Context, url string) (*entity.Favorite, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.FindByURL(ctx, url)
}

func (r *LazyFavoriteRepository) GetAll(ctx context.Context) ([]*entity.Favorite, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetAll(ctx)
}

func (r *LazyFavoriteRepository) GetByFolder(ctx context.Context, folderID *entity.FolderID) ([]*entity.Favorite, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetByFolder(ctx, folderID)
}

func (r *LazyFavoriteRepository) GetByShortcut(ctx context.Context, key int) (*entity.Favorite, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetByShortcut(ctx, key)
}

func (r *LazyFavoriteRepository) UpdatePosition(ctx context.Context, id entity.FavoriteID, position int) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.UpdatePosition(ctx, id, position)
}

func (r *LazyFavoriteRepository) SetFolder(ctx context.Context, id entity.FavoriteID, folderID *entity.FolderID) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.SetFolder(ctx, id, folderID)
}

func (r *LazyFavoriteRepository) SetShortcut(ctx context.Context, id entity.FavoriteID, key *int) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.SetShortcut(ctx, id, key)
}

func (r *LazyFavoriteRepository) Delete(ctx context.Context, id entity.FavoriteID) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Delete(ctx, id)
}

// LazyFolderRepository wraps a folder repository with lazy database initialization.
type LazyFolderRepository struct {
	provider port.DatabaseProvider
	repo     repository.FolderRepository
	once     sync.Once
	initErr  error
}

// NewLazyFolderRepository creates a lazy-loading folder repository.
func NewLazyFolderRepository(provider port.DatabaseProvider) repository.FolderRepository {
	return &LazyFolderRepository{provider: provider}
}

func (r *LazyFolderRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewFolderRepository(db)
	})
	return r.initErr
}

func (r *LazyFolderRepository) Save(ctx context.Context, folder *entity.Folder) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Save(ctx, folder)
}

func (r *LazyFolderRepository) FindByID(ctx context.Context, id entity.FolderID) (*entity.Folder, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.FindByID(ctx, id)
}

func (r *LazyFolderRepository) GetAll(ctx context.Context) ([]*entity.Folder, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetAll(ctx)
}

func (r *LazyFolderRepository) GetChildren(ctx context.Context, parentID *entity.FolderID) ([]*entity.Folder, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetChildren(ctx, parentID)
}

func (r *LazyFolderRepository) UpdatePosition(ctx context.Context, id entity.FolderID, position int) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.UpdatePosition(ctx, id, position)
}

func (r *LazyFolderRepository) Delete(ctx context.Context, id entity.FolderID) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Delete(ctx, id)
}

// LazyTagRepository wraps a tag repository with lazy database initialization.
type LazyTagRepository struct {
	provider port.DatabaseProvider
	repo     repository.TagRepository
	once     sync.Once
	initErr  error
}

// NewLazyTagRepository creates a lazy-loading tag repository.
func NewLazyTagRepository(provider port.DatabaseProvider) repository.TagRepository {
	return &LazyTagRepository{provider: provider}
}

func (r *LazyTagRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewTagRepository(db)
	})
	return r.initErr
}

func (r *LazyTagRepository) Save(ctx context.Context, tag *entity.Tag) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Save(ctx, tag)
}

func (r *LazyTagRepository) FindByID(ctx context.Context, id entity.TagID) (*entity.Tag, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.FindByID(ctx, id)
}

func (r *LazyTagRepository) FindByName(ctx context.Context, name string) (*entity.Tag, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.FindByName(ctx, name)
}

func (r *LazyTagRepository) GetAll(ctx context.Context) ([]*entity.Tag, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetAll(ctx)
}

func (r *LazyTagRepository) AssignToFavorite(ctx context.Context, tagID entity.TagID, favID entity.FavoriteID) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.AssignToFavorite(ctx, tagID, favID)
}

func (r *LazyTagRepository) RemoveFromFavorite(ctx context.Context, tagID entity.TagID, favID entity.FavoriteID) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.RemoveFromFavorite(ctx, tagID, favID)
}

func (r *LazyTagRepository) GetForFavorite(ctx context.Context, favID entity.FavoriteID) ([]*entity.Tag, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetForFavorite(ctx, favID)
}

func (r *LazyTagRepository) Delete(ctx context.Context, id entity.TagID) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Delete(ctx, id)
}

// LazyZoomRepository wraps a zoom repository with lazy database initialization.
type LazyZoomRepository struct {
	provider port.DatabaseProvider
	repo     repository.ZoomRepository
	once     sync.Once
	initErr  error
}

// NewLazyZoomRepository creates a lazy-loading zoom repository.
func NewLazyZoomRepository(provider port.DatabaseProvider) repository.ZoomRepository {
	return &LazyZoomRepository{provider: provider}
}

func (r *LazyZoomRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewZoomRepository(db)
	})
	return r.initErr
}

func (r *LazyZoomRepository) Get(ctx context.Context, domain string) (*entity.ZoomLevel, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.Get(ctx, domain)
}

func (r *LazyZoomRepository) Set(ctx context.Context, zoom *entity.ZoomLevel) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Set(ctx, zoom)
}

func (r *LazyZoomRepository) Delete(ctx context.Context, domain string) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Delete(ctx, domain)
}

func (r *LazyZoomRepository) GetAll(ctx context.Context) ([]*entity.ZoomLevel, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetAll(ctx)
}

// LazySessionRepository wraps a session repository with lazy database initialization.
type LazySessionRepository struct {
	provider port.DatabaseProvider
	repo     repository.SessionRepository
	once     sync.Once
	initErr  error
}

// NewLazySessionRepository creates a lazy-loading session repository.
func NewLazySessionRepository(provider port.DatabaseProvider) repository.SessionRepository {
	return &LazySessionRepository{provider: provider}
}

func (r *LazySessionRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewSessionRepository(db)
	})
	return r.initErr
}

func (r *LazySessionRepository) Save(ctx context.Context, session *entity.Session) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Save(ctx, session)
}

func (r *LazySessionRepository) FindByID(ctx context.Context, id entity.SessionID) (*entity.Session, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.FindByID(ctx, id)
}

func (r *LazySessionRepository) GetActive(ctx context.Context) (*entity.Session, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetActive(ctx)
}

func (r *LazySessionRepository) GetRecent(ctx context.Context, limit int) ([]*entity.Session, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetRecent(ctx, limit)
}

func (r *LazySessionRepository) MarkEnded(ctx context.Context, id entity.SessionID, endedAt time.Time) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.MarkEnded(ctx, id, endedAt)
}

func (r *LazySessionRepository) Delete(ctx context.Context, id entity.SessionID) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Delete(ctx, id)
}

func (r *LazySessionRepository) DeleteOldestExited(ctx context.Context, keepCount int) (int64, error) {
	if err := r.init(ctx); err != nil {
		return 0, err
	}
	return r.repo.DeleteOldestExited(ctx, keepCount)
}

func (r *LazySessionRepository) DeleteExitedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if err := r.init(ctx); err != nil {
		return 0, err
	}
	return r.repo.DeleteExitedBefore(ctx, cutoff)
}

// LazySessionStateRepository wraps a session state repository with lazy database initialization.
type LazySessionStateRepository struct {
	provider port.DatabaseProvider
	repo     repository.SessionStateRepository
	once     sync.Once
	initErr  error
}

// NewLazySessionStateRepository creates a lazy-loading session state repository.
func NewLazySessionStateRepository(provider port.DatabaseProvider) repository.SessionStateRepository {
	return &LazySessionStateRepository{provider: provider}
}

func (r *LazySessionStateRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewSessionStateRepository(db)
	})
	return r.initErr
}

func (r *LazySessionStateRepository) SaveSnapshot(ctx context.Context, state *entity.SessionState) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.SaveSnapshot(ctx, state)
}

func (r *LazySessionStateRepository) GetSnapshot(ctx context.Context, sessionID entity.SessionID) (*entity.SessionState, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetSnapshot(ctx, sessionID)
}

func (r *LazySessionStateRepository) DeleteSnapshot(ctx context.Context, sessionID entity.SessionID) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.DeleteSnapshot(ctx, sessionID)
}

func (r *LazySessionStateRepository) GetAllSnapshots(ctx context.Context) ([]*entity.SessionState, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetAllSnapshots(ctx)
}

func (r *LazySessionStateRepository) GetTotalSnapshotsSize(ctx context.Context) (int64, error) {
	if err := r.init(ctx); err != nil {
		return 0, err
	}
	return r.repo.GetTotalSnapshotsSize(ctx)
}

// LazyContentWhitelistRepository wraps a content whitelist repository with lazy database initialization.
type LazyContentWhitelistRepository struct {
	provider port.DatabaseProvider
	repo     repository.ContentWhitelistRepository
	once     sync.Once
	initErr  error
}

// NewLazyContentWhitelistRepository creates a lazy-loading content whitelist repository.
func NewLazyContentWhitelistRepository(provider port.DatabaseProvider) repository.ContentWhitelistRepository {
	return &LazyContentWhitelistRepository{provider: provider}
}

func (r *LazyContentWhitelistRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewContentWhitelistRepository(db)
	})
	return r.initErr
}

func (r *LazyContentWhitelistRepository) Add(ctx context.Context, domain string) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Add(ctx, domain)
}

func (r *LazyContentWhitelistRepository) Remove(ctx context.Context, domain string) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Remove(ctx, domain)
}

func (r *LazyContentWhitelistRepository) Contains(ctx context.Context, domain string) (bool, error) {
	if err := r.init(ctx); err != nil {
		return false, err
	}
	return r.repo.Contains(ctx, domain)
}

func (r *LazyContentWhitelistRepository) GetAll(ctx context.Context) ([]string, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.GetAll(ctx)
}

// LazyRepositories holds all lazy-loaded repositories.
type LazyRepositories struct {
	History      repository.HistoryRepository
	Favorite     repository.FavoriteRepository
	Folder       repository.FolderRepository
	Tag          repository.TagRepository
	Zoom         repository.ZoomRepository
	Session      repository.SessionRepository
	SessionState repository.SessionStateRepository
	Filter       repository.ContentWhitelistRepository
}

// NewLazyRepositories creates all lazy repositories from a database provider.
func NewLazyRepositories(provider port.DatabaseProvider) *LazyRepositories {
	return &LazyRepositories{
		History:      NewLazyHistoryRepository(provider),
		Favorite:     NewLazyFavoriteRepository(provider),
		Folder:       NewLazyFolderRepository(provider),
		Tag:          NewLazyTagRepository(provider),
		Zoom:         NewLazyZoomRepository(provider),
		Session:      NewLazySessionRepository(provider),
		SessionState: NewLazySessionStateRepository(provider),
		Filter:       NewLazyContentWhitelistRepository(provider),
	}
}

// GetDB returns the underlying *sql.DB for operations that need it directly.
// Returns nil if the database hasn't been initialized yet.
func GetDB(provider port.DatabaseProvider, ctx context.Context) *sql.DB {
	db, err := provider.DB(ctx)
	if err != nil {
		return nil
	}
	return db
}

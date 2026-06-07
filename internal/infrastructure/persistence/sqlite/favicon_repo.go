package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/favicon"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
)

type faviconRepo struct {
	queries *sqlc.Queries
}

func NewFaviconRepository(db *sql.DB) port.FaviconRepository {
	return &faviconRepo{queries: sqlc.New(db)}
}

type lazyFaviconRepository struct {
	provider port.DatabaseProvider
	repo     port.FaviconRepository
	once     sync.Once
	initErr  error
}

func NewLazyFaviconRepository(provider port.DatabaseProvider) port.FaviconRepository {
	return &lazyFaviconRepository{provider: provider}
}

func (r *lazyFaviconRepository) init(ctx context.Context) error {
	r.once.Do(func() {
		db, err := r.provider.DB(ctx)
		if err != nil {
			r.initErr = err
			return
		}
		r.repo = NewFaviconRepository(db)
	})
	return r.initErr
}

func (r *lazyFaviconRepository) Get(ctx context.Context, key favicon.Key) (*favicon.Metadata, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.Get(ctx, key)
}

func (r *lazyFaviconRepository) FindFirst(ctx context.Context, keys []favicon.Key) (*favicon.Metadata, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r.repo.FindFirst(ctx, keys)
}

func (r *lazyFaviconRepository) Upsert(ctx context.Context, meta favicon.Metadata) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Upsert(ctx, meta)
}

func (r *lazyFaviconRepository) UpdateLastChecked(ctx context.Context, key favicon.Key, contentHash string, checkedAt time.Time) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.UpdateLastChecked(ctx, key, contentHash, checkedAt)
}

func (r *lazyFaviconRepository) Delete(ctx context.Context, key favicon.Key) error {
	if err := r.init(ctx); err != nil {
		return err
	}
	return r.repo.Delete(ctx, key)
}

func (r *faviconRepo) Get(ctx context.Context, key favicon.Key) (*favicon.Metadata, error) {
	row, err := r.queries.GetFavicon(ctx, string(key))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return faviconFromRow(row), nil
}

func (r *faviconRepo) FindFirst(ctx context.Context, keys []favicon.Key) (*favicon.Metadata, error) {
	for _, key := range keys {
		meta, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if meta != nil {
			return meta, nil
		}
	}
	return nil, nil
}

func (r *faviconRepo) Upsert(ctx context.Context, meta favicon.Metadata) error {
	return r.queries.UpsertFavicon(ctx, sqlc.UpsertFaviconParams{
		Key:           string(meta.Key),
		SourceUrl:     nullString(meta.SourceURL),
		PageUrl:       nullString(meta.PageURL),
		Source:        string(meta.Source),
		ContentHash:   meta.ContentHash,
		ContentType:   nullString(meta.ContentType),
		UpdatedAt:     meta.UpdatedAt,
		LastCheckedAt: meta.LastCheckedAt,
	})
}

func (r *faviconRepo) UpdateLastChecked(ctx context.Context, key favicon.Key, contentHash string, checkedAt time.Time) error {
	return r.queries.UpdateFaviconLastChecked(ctx, sqlc.UpdateFaviconLastCheckedParams{
		Key:           string(key),
		ContentHash:   contentHash,
		LastCheckedAt: checkedAt,
	})
}

func (r *faviconRepo) Delete(ctx context.Context, key favicon.Key) error {
	return r.queries.DeleteFavicon(ctx, string(key))
}

func faviconFromRow(row sqlc.Favicon) *favicon.Metadata {
	return &favicon.Metadata{
		Key:           favicon.Key(row.Key),
		SourceURL:     row.SourceUrl.String,
		PageURL:       row.PageUrl.String,
		Source:        favicon.Source(row.Source),
		ContentHash:   row.ContentHash,
		ContentType:   row.ContentType.String,
		UpdatedAt:     row.UpdatedAt,
		LastCheckedAt: row.LastCheckedAt,
	}
}

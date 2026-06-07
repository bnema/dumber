package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/favicon"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func faviconTestCtx() context.Context {
	logger := logging.NewFromConfigValues("debug", "console")
	return logging.WithContext(context.Background(), logger)
}

func newFaviconRepo(t *testing.T) (context.Context, port.FaviconRepository) {
	t.Helper()
	ctx := faviconTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return ctx, sqlite.NewFaviconRepository(db)
}

func TestFaviconRepositoryUpsertAndGet(t *testing.T) {
	ctx, repo := newFaviconRepo(t)
	updatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	checkedAt := updatedAt.Add(time.Hour)

	meta := favicon.Metadata{
		Key:           favicon.Key("example.com"),
		SourceURL:     "https://example.com/favicon.ico",
		PageURL:       "https://example.com/page",
		Source:        favicon.SourceEngine,
		ContentHash:   "sha256:first",
		ContentType:   "image/png",
		UpdatedAt:     updatedAt,
		LastCheckedAt: checkedAt,
	}
	require.NoError(t, repo.Upsert(ctx, meta))

	got, err := repo.Get(ctx, favicon.Key("example.com"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, meta.Key, got.Key)
	assert.Equal(t, meta.SourceURL, got.SourceURL)
	assert.Equal(t, meta.PageURL, got.PageURL)
	assert.Equal(t, meta.Source, got.Source)
	assert.Equal(t, meta.ContentHash, got.ContentHash)
	assert.Equal(t, meta.ContentType, got.ContentType)
	assert.True(t, meta.UpdatedAt.Equal(got.UpdatedAt), "updated_at = %s", got.UpdatedAt)
	assert.True(t, meta.LastCheckedAt.Equal(got.LastCheckedAt), "last_checked_at = %s", got.LastCheckedAt)

	replacement := meta
	replacement.SourceURL = "https://cdn.example.com/icon.png"
	replacement.Source = favicon.SourcePageDiscovery
	replacement.ContentHash = "sha256:second"
	replacement.UpdatedAt = updatedAt.Add(2 * time.Hour)
	replacement.LastCheckedAt = checkedAt.Add(2 * time.Hour)
	require.NoError(t, repo.Upsert(ctx, replacement))

	got, err = repo.Get(ctx, favicon.Key("example.com"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, replacement.SourceURL, got.SourceURL)
	assert.Equal(t, replacement.Source, got.Source)
	assert.Equal(t, replacement.ContentHash, got.ContentHash)
	assert.True(t, replacement.UpdatedAt.Equal(got.UpdatedAt))
	assert.True(t, replacement.LastCheckedAt.Equal(got.LastCheckedAt))
}

func TestFaviconRepositoryGetNotFoundReturnsNil(t *testing.T) {
	ctx, repo := newFaviconRepo(t)

	got, err := repo.Get(ctx, favicon.Key("missing.example"))
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFaviconRepositoryFindFirstUsesCallerCandidateOrder(t *testing.T) {
	ctx, repo := newFaviconRepo(t)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, repo.Upsert(ctx, favicon.Metadata{Key: favicon.Key("example.com"), Source: favicon.SourceEngine, ContentHash: "parent", UpdatedAt: now, LastCheckedAt: now}))
	require.NoError(t, repo.Upsert(ctx, favicon.Metadata{Key: favicon.Key("docs.example.com"), Source: favicon.SourceEngine, ContentHash: "child", UpdatedAt: now, LastCheckedAt: now}))

	got, err := repo.FindFirst(ctx, []favicon.Key{favicon.Key("missing.example.com"), favicon.Key("example.com"), favicon.Key("docs.example.com")})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, favicon.Key("example.com"), got.Key)
	assert.Equal(t, "parent", got.ContentHash)

	got, err = repo.FindFirst(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFaviconRepositoryUpdateLastChecked(t *testing.T) {
	ctx, repo := newFaviconRepo(t)
	updatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	checkedAt := updatedAt.Add(24 * time.Hour)
	meta := favicon.Metadata{Key: favicon.Key("example.com"), Source: favicon.SourceEngine, ContentHash: "old", UpdatedAt: updatedAt, LastCheckedAt: updatedAt}
	require.NoError(t, repo.Upsert(ctx, meta))

	require.NoError(t, repo.UpdateLastChecked(ctx, favicon.Key("example.com"), "new", checkedAt))

	got, err := repo.Get(ctx, favicon.Key("example.com"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "new", got.ContentHash)
	assert.True(t, updatedAt.Equal(got.UpdatedAt), "updated_at should not change")
	assert.True(t, checkedAt.Equal(got.LastCheckedAt))
}

func TestFaviconRepositoryDelete(t *testing.T) {
	ctx, repo := newFaviconRepo(t)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, repo.Upsert(ctx, favicon.Metadata{Key: favicon.Key("example.com"), Source: favicon.SourceEngine, ContentHash: "hash", UpdatedAt: now, LastCheckedAt: now}))

	require.NoError(t, repo.Delete(ctx, favicon.Key("example.com")))

	got, err := repo.Get(ctx, favicon.Key("example.com"))
	require.NoError(t, err)
	assert.Nil(t, got)
}

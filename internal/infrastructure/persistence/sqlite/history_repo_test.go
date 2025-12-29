package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func historyTestCtx() context.Context {
	logger := logging.NewFromConfigValues("debug", "console")
	return logging.WithContext(context.Background(), logger)
}

func TestHistoryRepository_GetRecentSince(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save some history entries
	entries := []*entity.HistoryEntry{
		{URL: "https://example.com", Title: "Example"},
		{URL: "https://github.com", Title: "GitHub"},
		{URL: "https://google.com", Title: "Google"},
	}

	for _, e := range entries {
		require.NoError(t, repo.Save(ctx, e))
	}

	// Get recent since 30 days (all should be included)
	results, err := repo.GetRecentSince(ctx, 30)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Results should be sorted by last_visited DESC
	// Since they were all just inserted, order may vary
	urls := make(map[string]bool)
	for _, r := range results {
		urls[r.URL] = true
	}
	assert.True(t, urls["https://example.com"])
	assert.True(t, urls["https://github.com"])
	assert.True(t, urls["https://google.com"])
}

func TestHistoryRepository_GetRecentSince_EmptyResult(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Get recent with no data
	results, err := repo.GetRecentSince(ctx, 30)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHistoryRepository_GetMostVisited(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entries with different visit counts
	entry1 := &entity.HistoryEntry{URL: "https://example.com", Title: "Example"}
	entry2 := &entity.HistoryEntry{URL: "https://github.com", Title: "GitHub"}
	entry3 := &entity.HistoryEntry{URL: "https://google.com", Title: "Google"}

	require.NoError(t, repo.Save(ctx, entry1))
	require.NoError(t, repo.Save(ctx, entry2))
	require.NoError(t, repo.Save(ctx, entry3))

	// Increment visit counts to create different popularities
	// github: 5 visits, google: 3 visits, example: 1 visit
	for i := 0; i < 4; i++ {
		require.NoError(t, repo.IncrementVisitCount(ctx, "https://github.com"))
	}
	for i := 0; i < 2; i++ {
		require.NoError(t, repo.IncrementVisitCount(ctx, "https://google.com"))
	}

	// Get most visited
	results, err := repo.GetMostVisited(ctx, 30)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// First result should be github (5 visits)
	assert.Equal(t, "https://github.com", results[0].URL)
	assert.Equal(t, int64(5), results[0].VisitCount)

	// Second should be google (3 visits)
	assert.Equal(t, "https://google.com", results[1].URL)
	assert.Equal(t, int64(3), results[1].VisitCount)

	// Third should be example (1 visit)
	assert.Equal(t, "https://example.com", results[2].URL)
	assert.Equal(t, int64(1), results[2].VisitCount)
}

func TestHistoryRepository_GetMostVisited_EmptyResult(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Get most visited with no data
	results, err := repo.GetMostVisited(ctx, 30)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHistoryRepository_GetRecentSince_RespectsTimeFilter(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save an entry
	entry := &entity.HistoryEntry{URL: "https://example.com", Title: "Example"}
	require.NoError(t, repo.Save(ctx, entry))

	// Query with 1 day filter - should include recently added entry
	results, err := repo.GetRecentSince(ctx, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Query with very large days filter - should still work
	results, err = repo.GetRecentSince(ctx, 365)
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestHistoryRepository_CRUD(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Create
	entry := &entity.HistoryEntry{URL: "https://example.com", Title: "Example"}
	require.NoError(t, repo.Save(ctx, entry))

	// Read
	found, err := repo.FindByURL(ctx, "https://example.com")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "https://example.com", found.URL)
	assert.Equal(t, "Example", found.Title)
	assert.Equal(t, int64(1), found.VisitCount)

	// Update (save again increments visit count)
	require.NoError(t, repo.Save(ctx, entry))
	found2, err := repo.FindByURL(ctx, "https://example.com")
	require.NoError(t, err)
	assert.Equal(t, int64(2), found2.VisitCount)

	// Get recent
	recent, err := repo.GetRecent(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, recent, 1)

	// Delete
	require.NoError(t, repo.Delete(ctx, found.ID))

	// Verify deleted
	deleted, err := repo.FindByURL(ctx, "https://example.com")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

// Ensure the test uses an isolated timestamp for time-sensitive queries
func TestHistoryRepository_TimeFilterIntegrity(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entry
	entry := &entity.HistoryEntry{URL: "https://test.com", Title: "Test"}
	require.NoError(t, repo.Save(ctx, entry))

	// Verify entry was saved with recent timestamp
	found, err := repo.FindByURL(ctx, "https://test.com")
	require.NoError(t, err)
	require.NotNil(t, found)

	// LastVisited should be within the last minute
	timeDiff := time.Since(found.LastVisited)
	assert.Less(t, timeDiff, time.Minute, "LastVisited should be recent, got %v ago", timeDiff)
}

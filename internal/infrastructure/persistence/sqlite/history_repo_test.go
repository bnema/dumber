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

func TestHistoryRepository_GetRecentSince_RejectsNonPositiveDays(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// days = 0 should return error
	_, err = repo.GetRecentSince(ctx, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "days must be positive")

	// days < 0 should return error
	_, err = repo.GetRecentSince(ctx, -5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "days must be positive")
}

func TestHistoryRepository_GetMostVisited_RejectsNonPositiveDays(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// days = 0 should return error
	_, err = repo.GetMostVisited(ctx, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "days must be positive")

	// days < 0 should return error
	_, err = repo.GetMostVisited(ctx, -5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "days must be positive")
}

func TestHistoryRepository_GetAllRecentHistory(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save some entries
	entries := []*entity.HistoryEntry{
		{URL: "https://example.com", Title: "Example"},
		{URL: "https://github.com", Title: "GitHub"},
	}
	for _, e := range entries {
		require.NoError(t, repo.Save(ctx, e))
	}

	// Get all recent history
	results, err := repo.GetAllRecentHistory(ctx)
	require.NoError(t, err)
	require.Len(t, results, 2)
}

func TestHistoryRepository_GetAllMostVisited(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entries with different visit counts
	entry1 := &entity.HistoryEntry{URL: "https://example.com", Title: "Example"}
	entry2 := &entity.HistoryEntry{URL: "https://github.com", Title: "GitHub"}

	require.NoError(t, repo.Save(ctx, entry1))
	require.NoError(t, repo.Save(ctx, entry2))

	// Increment github visits
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.IncrementVisitCount(ctx, "https://github.com"))
	}

	// Get all most visited
	results, err := repo.GetAllMostVisited(ctx)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// First should be github (4 visits)
	assert.Equal(t, "https://github.com", results[0].URL)
	assert.Equal(t, int64(4), results[0].VisitCount)
}

func TestHistoryRepository_Search_SingleWord(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entries
	entries := []*entity.HistoryEntry{
		{URL: "https://github.com/user/repo", Title: "My GitHub Repository"},
		{URL: "https://gitlab.com/project", Title: "GitLab Project"},
		{URL: "https://example.com", Title: "Example Site"},
	}
	for _, e := range entries {
		require.NoError(t, repo.Save(ctx, e))
	}

	// Search for "github" - should find the github entry
	results, err := repo.Search(ctx, "github", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "https://github.com/user/repo", results[0].Entry.URL)
}

func TestHistoryRepository_Search_MultiWord(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entries
	entries := []*entity.HistoryEntry{
		{URL: "https://github.com/issues", Title: "GitHub Issues"},
		{URL: "https://github.com/pulls", Title: "GitHub Pull Requests"},
		{URL: "https://jira.com/issues", Title: "Jira Issues"},
	}
	for _, e := range entries {
		require.NoError(t, repo.Save(ctx, e))
	}

	// Search for "github issues" - should find only the github issues entry
	results, err := repo.Search(ctx, "github issues", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "https://github.com/issues", results[0].Entry.URL)
}

func TestHistoryRepository_Search_MatchesTitle(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entry with specific title that doesn't appear in URL
	entry := &entity.HistoryEntry{
		URL:   "https://docs.example.com/guide",
		Title: "Comprehensive Documentation Guide",
	}
	require.NoError(t, repo.Save(ctx, entry))

	// Search for "Documentation" - should find by title
	results, err := repo.Search(ctx, "Documentation", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "https://docs.example.com/guide", results[0].Entry.URL)
}

func TestHistoryRepository_Search_EmptyQuery(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save an entry
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://example.com",
		Title: "Example",
	}))

	// Empty query should return empty results, no error
	results, err := repo.Search(ctx, "", 10)
	require.NoError(t, err)
	assert.Empty(t, results)

	// Whitespace-only query should also return empty results
	results, err = repo.Search(ctx, "   ", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHistoryRepository_Search_NoResults(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save an entry
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://example.com",
		Title: "Example",
	}))

	// Search for something that doesn't exist
	results, err := repo.Search(ctx, "nonexistent", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHistoryRepository_Search_PrefixMatching(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entries
	entries := []*entity.HistoryEntry{
		{URL: "https://github.com", Title: "GitHub"},
		{URL: "https://gitlab.com", Title: "GitLab"},
		{URL: "https://example.com", Title: "Example"},
	}
	for _, e := range entries {
		require.NoError(t, repo.Save(ctx, e))
	}

	// Search for "git" - should find both github and gitlab (prefix match)
	results, err := repo.Search(ctx, "git", 10)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Verify both git* entries are found
	urls := make(map[string]bool)
	for _, r := range results {
		urls[r.Entry.URL] = true
	}
	assert.True(t, urls["https://github.com"])
	assert.True(t, urls["https://gitlab.com"])
}

func TestHistoryRepository_Search_DomainLikeQuery(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entries with domain-like URLs
	entries := []*entity.HistoryEntry{
		{URL: "https://gordon.bnema.dev/", Title: "Gordon - Self-hosted Deployment Platform"},
		{URL: "https://example.com", Title: "Example Site"},
		{URL: "https://other.gordon.io", Title: "Another Gordon Site"},
	}
	for _, e := range entries {
		require.NoError(t, repo.Save(ctx, e))
	}

	// Search with period in query (domain-like) - periods should be treated as separators
	// "gordon.bnem" should match "gordon.bnema.dev" (tokens: gordon, bnem -> gordon, bnema)
	results, err := repo.Search(ctx, "gordon.bnem", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "domain-like query 'gordon.bnem' should match gordon.bnema.dev")

	// Verify the gordon.bnema.dev entry is found
	found := false
	for _, r := range results {
		if r.Entry.URL == "https://gordon.bnema.dev/" {
			found = true
			break
		}
	}
	assert.True(t, found, "gordon.bnema.dev should be in results")
}

func TestHistoryRepository_Search_SpecialCharacters(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save entry
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://example.com/path",
		Title: "Example Page",
	}))

	// Search with FTS5 special characters - should not cause errors
	specialQueries := []string{
		"example:",     // colon
		"example*",     // asterisk
		"example()",    // parentheses
		`example"test`, // quotes
		"example^",     // caret
		"example-test", // hyphen
		"example/",     // slash
	}

	for _, q := range specialQueries {
		results, err := repo.Search(ctx, q, 10)
		require.NoError(t, err, "query %q should not cause error", q)
		// May or may not find results, but should not error
		_ = results
	}
}

func TestHistoryRepository_Search_SlashSeparatedQuery(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://github.com/bnema/dumber",
		Title: "Dumber",
	}))

	results, err := repo.Search(ctx, "github.com/bnema", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "slash-separated query should match history")
}

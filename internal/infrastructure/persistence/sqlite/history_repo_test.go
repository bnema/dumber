package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	repository "github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
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
	for range 4 {
		require.NoError(t, repo.IncrementVisitCount(ctx, "https://github.com"))
	}
	for range 2 {
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
	recent, err := repo.GetRecent(ctx, 10, 0, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, recent, 1)

	allRecent, err := repo.GetRecent(ctx, 0, 0, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, allRecent, 1)

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
	for range 3 {
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
	results, err := repo.Search(ctx, "github", 10, repository.HistoryScope{})
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
	results, err := repo.Search(ctx, "github issues", 10, repository.HistoryScope{})
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
	results, err := repo.Search(ctx, "Documentation", 10, repository.HistoryScope{})
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
	results, err := repo.Search(ctx, "", 10, repository.HistoryScope{})
	require.NoError(t, err)
	assert.Empty(t, results)

	// Whitespace-only query should also return empty results
	results, err = repo.Search(ctx, "   ", 10, repository.HistoryScope{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHistoryRepository_Search_NoValidTokens(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://example.com",
		Title: "Example",
	}))

	invalidQueries := []string{
		"!!!",
		"___",
		"AND OR NOT NEAR",
		`"***"`,
	}

	for _, q := range invalidQueries {
		results, err := repo.Search(ctx, q, 10, repository.HistoryScope{})
		require.NoError(t, err, "query %q should return early without SQL errors", q)
		assert.Empty(t, results, "query %q should have no valid tokens", q)
	}
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
	results, err := repo.Search(ctx, "nonexistent", 10, repository.HistoryScope{})
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
	results, err := repo.Search(ctx, "git", 10, repository.HistoryScope{})
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
	results, err := repo.Search(ctx, "gordon.bnem", 10, repository.HistoryScope{})
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
		results, err := repo.Search(ctx, q, 10, repository.HistoryScope{})
		require.NoError(t, err, "query %q should not cause error", q)
		// May or may not find results, but should not error
		_ = results
	}
}

func TestHistoryRepository_Search_IgnoresFTSOperatorsAsTokens(t *testing.T) {
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

	results, err := repo.Search(ctx, "AND github OR", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "https://github.com/bnema/dumber", results[0].Entry.URL)
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

	results, err := repo.Search(ctx, "github.com/bnema", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.NotEmpty(t, results, "slash-separated query should match history")
}

func TestHistoryRepository_Search_PrefersURLMatchOverTitleOnlyMatch(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://github.com/bnema/dumber/issues",
		Title: "Project board",
	}))
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://tracker.example.com/board",
		Title: "GitHub Issues Mirror",
	}))

	results, err := repo.Search(ctx, "github issues", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "https://github.com/bnema/dumber/issues", results[0].Entry.URL)
}

func TestHistoryRepository_Search_PrefersHostPrefixOverTitleContains(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://github.com",
		Title: "GitHub",
	}))
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://example.com/landing",
		Title: "GitHub documentation mirror",
	}))

	results, err := repo.Search(ctx, "git", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "https://github.com", results[0].Entry.URL)
}

func TestHistoryRepository_Search_PrefersShorterRootURLForHostQuery(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://github.com",
		Title: "GitHub",
	}))
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://github.com/bnema/dumber/issues/123",
		Title: "Issue 123",
	}))

	results, err := repo.Search(ctx, "github", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "https://github.com", results[0].Entry.URL)
}

func TestHistoryRepository_UpdateMetadata_DoesNotIncrementVisitCount(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repoIface := sqlite.NewHistoryRepository(db)
	repo, ok := repoIface.(interface {
		Save(context.Context, *entity.HistoryEntry) error
		FindByURL(context.Context, string) (*entity.HistoryEntry, error)
		UpdateMetadata(context.Context, *entity.HistoryEntry) error
	})
	require.True(t, ok)

	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://github.com/bnema/dumber",
		Title: "Old title",
	}))

	before, err := repo.FindByURL(ctx, "https://github.com/bnema/dumber")
	require.NoError(t, err)
	require.NotNil(t, before)
	require.Equal(t, int64(1), before.VisitCount)

	require.NoError(t, repo.UpdateMetadata(ctx, &entity.HistoryEntry{
		URL:   "https://github.com/bnema/dumber",
		Title: "New title",
	}))

	after, err := repo.FindByURL(ctx, "https://github.com/bnema/dumber")
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.Equal(t, int64(1), after.VisitCount)
	assert.Equal(t, "New title", after.Title)
}

func TestHistoryRepository_AboutBlank_CappedVisitCount(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save about:blank multiple times - visit count should stay at 1
	for range 5 {
		require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
			URL:   "about:blank",
			Title: "New Tab",
		}))
	}

	// Verify about:blank has visit_count = 1
	found, err := repo.FindByURL(ctx, "about:blank")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, int64(1), found.VisitCount, "about:blank should always have visit_count = 1")

	// Try incrementing visit count directly - should be ignored
	require.NoError(t, repo.IncrementVisitCount(ctx, "about:blank"))

	found2, err := repo.FindByURL(ctx, "about:blank")
	require.NoError(t, err)
	assert.Equal(t, int64(1), found2.VisitCount, "about:blank increment should be ignored")
}

func TestHistoryRepository_AboutBlank_NotDominant(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	// Save about:blank many times
	for range 10 {
		require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
			URL:   "about:blank",
			Title: "New Tab",
		}))
	}

	// Save a regular entry with multiple visits
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://example.com",
		Title: "Example",
	}))
	require.NoError(t, repo.IncrementVisitCount(ctx, "https://example.com"))
	require.NoError(t, repo.IncrementVisitCount(ctx, "https://example.com"))

	// Get most visited - example.com should be first (3 visits vs 1)
	results, err := repo.GetAllMostVisited(ctx)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "https://example.com", results[0].URL, "example.com should be first with higher visit count")
	assert.Equal(t, int64(3), results[0].VisitCount)

	assert.Equal(t, "about:blank", results[1].URL, "about:blank should be second with capped visit count")
	assert.Equal(t, int64(1), results[1].VisitCount)
}

func TestHistoryRepository_Search_ReturnsEmptyForNonPositiveLimit(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://github.com",
		Title: "GitHub",
	}))

	results, err := repo.Search(ctx, "git", 0, repository.HistoryScope{})
	require.NoError(t, err)
	assert.Empty(t, results)

	results, err = repo.Search(ctx, "git", -1, repository.HistoryScope{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHistoryRepository_RejectsEmptyCanonicalDomain(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)

	entries, err := repo.GetRecentByDomain(ctx, " ", 10, 0)
	require.Error(t, err)
	assert.Nil(t, entries)
	assert.Error(t, repo.DeleteByDomain(ctx, "/path"))
}

func TestHistoryRepository_DomainColumnPowersFilteringStatsAndDelete(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{URL: "https://www.example.com/a", Title: "Example A"}))
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{URL: "https://example.com/b", Title: "Example B"}))
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{URL: "https://other.test", Title: "Other"}))

	exampleEntries, err := repo.GetRecentByDomain(ctx, "www.example.com", 0, 0)
	require.NoError(t, err)
	require.Len(t, exampleEntries, 2)
	for _, entry := range exampleEntries {
		assert.Contains(t, entry.URL, "example.com")
	}

	stats, err := repo.GetDomainStats(ctx, 10)
	require.NoError(t, err)
	require.NotEmpty(t, stats)
	assert.Equal(t, "example.com", stats[0].Domain)
	assert.Equal(t, int64(2), stats[0].PageCount)

	require.NoError(t, repo.DeleteByDomain(ctx, "example.com"))

	exampleEntries, err = repo.GetRecentByDomain(ctx, "example.com", 10, 0)
	require.NoError(t, err)
	assert.Empty(t, exampleEntries)

	remaining, err := repo.GetAllRecentHistory(ctx)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "https://other.test", remaining[0].URL)
}
func TestHistoryRepository_GetRecentWindow_StableCursorWithTimestampTies(t *testing.T) {
	ctx := historyTestCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)
	tied := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	older := tied.Add(-time.Hour)
	for i := 1; i <= 5; i++ {
		_, err = db.ExecContext(ctx, `INSERT INTO history (url, title, domain, last_visited, created_at) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("https://example.com/%d", i), fmt.Sprintf("Entry %d", i), "example.com", tied, tied)
		require.NoError(t, err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO history (url, title, domain, last_visited, created_at) VALUES (?, ?, ?, ?, ?)`,
		"https://example.com/older", "Older", "example.com", older, older)
	require.NoError(t, err)

	page1, err := repo.GetRecentWindow(ctx, tied.Add(time.Second), 0, 3)
	require.NoError(t, err)
	require.Len(t, page1, 3)
	assert.Equal(t, []int64{5, 4, 3}, historyIDs(page1))

	page2, err := repo.GetRecentWindow(ctx, page1[len(page1)-1].LastVisited, page1[len(page1)-1].ID, 3)
	require.NoError(t, err)
	require.Len(t, page2, 3)
	assert.Equal(t, []int64{2, 1, 6}, historyIDs(page2))

	seen := map[int64]bool{}
	for _, entry := range append(page1, page2...) {
		if seen[entry.ID] {
			t.Fatalf("duplicate history id %d", entry.ID)
		}
		seen[entry.ID] = true
	}
}

func historyIDs(entries []*entity.HistoryEntry) []int64 {
	ids := make([]int64, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	return ids
}

// insertHistoryRow seeds a history row with an explicit visit count and
// last_visited so age-scope tests can use stable, non-now timestamps. The
// timestamp is written in the same UTC second-precision format that SQLite's
// CURRENT_TIMESTAMP uses in production.
func insertHistoryRow(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	url, title string,
	visitCount int,
	lastVisited time.Time,
) {
	t.Helper()
	timestamp := sqliteTimestamp(lastVisited)
	_, err := db.ExecContext(ctx,
		`INSERT INTO history (url, title, last_visited, created_at, visit_count) VALUES (?, ?, ?, ?, ?)`,
		url, title, timestamp, timestamp, visitCount)
	require.NoError(t, err)
}

// sqliteTimestamp formats a time the way SQLite's CURRENT_TIMESTAMP stores it:
// UTC at second precision without a timezone suffix.
func sqliteTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

func TestHistoryRepository_GetMostVisitedWithin_ExcludesOldHighCountURL(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -30)
	insertHistoryRow(t, ctx, db, "https://old.example.com", "Old Popular", 50, now.AddDate(0, 0, -40))
	insertHistoryRow(t, ctx, db, "https://recent.example.com", "Recent", 2, now.AddDate(0, 0, -1))

	results, err := repo.GetMostVisitedWithin(ctx, 10, repository.HistoryScope{Cutoff: cutoff})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "https://recent.example.com", results[0].URL)

	// A zero cutoff means all stored history, so the old popular URL returns.
	all, err := repo.GetMostVisitedWithin(ctx, 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, "https://old.example.com", all[0].URL)
	assert.Equal(t, int64(50), all[0].VisitCount)
}

func TestHistoryRepository_GetMostVisitedWithin_BoundsResults(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	insertHistoryRow(t, ctx, db, "https://one.example.com", "One", 5, now.Add(-time.Hour))
	insertHistoryRow(t, ctx, db, "https://two.example.com", "Two", 3, now.Add(-2*time.Hour))
	insertHistoryRow(t, ctx, db, "https://three.example.com", "Three", 1, now.Add(-3*time.Hour))

	results, err := repo.GetMostVisitedWithin(ctx, 2, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, []string{"https://one.example.com", "https://two.example.com"}, historyURLs(results))
}

func TestHistoryRepository_GetRecent_ScopeExcludesOldEntries(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -30)
	insertHistoryRow(t, ctx, db, "https://old.example.com", "Old", 1, now.AddDate(0, 0, -40))
	insertHistoryRow(t, ctx, db, "https://recent.example.com", "Recent", 1, now.AddDate(0, 0, -1))

	scoped, err := repo.GetRecent(ctx, 10, 0, repository.HistoryScope{Cutoff: cutoff})
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	assert.Equal(t, "https://recent.example.com", scoped[0].URL)

	all, err := repo.GetRecent(ctx, 10, 0, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, all, 2)

	bounded, err := repo.GetRecent(ctx, 1, 0, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, bounded, 1)
}

func TestHistoryRepository_Search_ScopeExcludesOldTitleOnlyMatch(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -30)
	// Matches only in the title, but is older than the window.
	insertHistoryRow(t, ctx, db, "https://old.example.com/ancient", "Widget Old Guide", 1, now.AddDate(0, 0, -40))
	// Matches in the title and is inside the window.
	insertHistoryRow(t, ctx, db, "https://recent.example.com/guide", "Widget Recent Guide", 1, now.AddDate(0, 0, -1))
	// Matches in the URL, but is older than the window.
	insertHistoryRow(t, ctx, db, "https://widget.example.com/page", "No match here", 1, now.AddDate(0, 0, -40))

	scoped, err := repo.Search(ctx, "widget", 10, repository.HistoryScope{Cutoff: cutoff})
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	assert.Equal(t, "https://recent.example.com/guide", scoped[0].Entry.URL)

	// A zero cutoff keeps the general, unrestricted search behavior.
	all, err := repo.Search(ctx, "widget", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, all, 3)
}

func historyURLs(entries []*entity.HistoryEntry) []string {
	urls := make([]string, len(entries))
	for i, entry := range entries {
		urls[i] = entry.URL
	}
	return urls
}

func TestHistoryRepository_ScopeCutoffIsInclusiveAtSecondPrecision(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	cutoff := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	insertHistoryRow(t, ctx, db, "https://exact.example.com", "Exact", 1, cutoff)
	insertHistoryRow(t, ctx, db, "https://before.example.com", "Before", 1, cutoff.Add(-time.Second))

	results, err := repo.GetRecent(ctx, 10, 0, repository.HistoryScope{Cutoff: cutoff})
	require.NoError(t, err)
	require.Len(t, results, 1, "the entry exactly on the cutoff must be included")
	assert.Equal(t, "https://exact.example.com", results[0].URL)
}

func TestHistoryRepository_UnboundedScopePreservesNullLastVisited(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	_, err = db.ExecContext(
		ctx,
		`INSERT INTO history (url, title, last_visited, visit_count) VALUES (?, ?, NULL, ?)`,
		"https://null-last-visited.example.com",
		"Legacy Null Row",
		1,
	)
	require.NoError(t, err)

	cutoff := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	recent, err := repo.GetRecent(ctx, 10, 0, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, recent, 1, "unbounded recent history must keep NULL last_visited rows")

	mostVisited, err := repo.GetMostVisitedWithin(ctx, 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, mostVisited, 1, "unbounded most-visited must keep NULL last_visited rows")

	matches, err := repo.Search(ctx, "legacy", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, matches, 1, "unbounded search must keep NULL last_visited rows")

	scoped, err := repo.GetRecent(ctx, 10, 0, repository.HistoryScope{Cutoff: cutoff})
	require.NoError(t, err)
	assert.Empty(t, scoped, "a bounded window must not include rows without a last_visited timestamp")
}

func TestHistoryRepository_Search_FiltersBeforeLimitOnURLMatch(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -30)
	// The old row outranks the recent row (higher visit count and domain boost),
	// so it would consume the single LIMIT slot if filtering happened after LIMIT.
	insertHistoryRow(t, ctx, db, "https://urlsecret.example.com/", "Old", 50, now.AddDate(0, 0, -40))
	insertHistoryRow(t, ctx, db, "https://urlsecret-recent.example.com/", "Recent", 1, now.AddDate(0, 0, -1))

	results, err := repo.Search(ctx, "urlsecret", 1, repository.HistoryScope{Cutoff: cutoff})
	require.NoError(t, err)
	require.Len(t, results, 1, "URL FTS must filter by age before applying LIMIT")
	assert.Equal(t, "https://urlsecret-recent.example.com/", results[0].Entry.URL)

	// Without a scope the old high-priority row wins the same query.
	all, err := repo.Search(ctx, "urlsecret", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, "https://urlsecret.example.com/", all[0].Entry.URL)
}

func TestHistoryRepository_Search_FiltersBeforeLimitOnTitleMatch(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -30)
	// The query only matches titles, so only the title FTS path is exercised.
	// The old row has the higher visit count and would win an unfiltered LIMIT.
	insertHistoryRow(t, ctx, db, "https://old.example.com/ancient", "Titlesecret Old Guide", 50, now.AddDate(0, 0, -40))
	insertHistoryRow(t, ctx, db, "https://recent.example.com/entry", "Titlesecret Recent Guide", 1, now.AddDate(0, 0, -1))

	results, err := repo.Search(ctx, "titlesecret", 1, repository.HistoryScope{Cutoff: cutoff})
	require.NoError(t, err)
	require.Len(t, results, 1, "title FTS must filter by age before applying LIMIT")
	assert.Equal(t, "https://recent.example.com/entry", results[0].Entry.URL)

	all, err := repo.Search(ctx, "titlesecret", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func TestHistoryRepository_GetRecent_BoundedLimitOffsetAndCutoff(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -30)
	insertHistoryRow(t, ctx, db, "https://one.example.com", "One", 1, now.Add(-time.Hour))
	insertHistoryRow(t, ctx, db, "https://two.example.com", "Two", 1, now.Add(-2*time.Hour))
	insertHistoryRow(t, ctx, db, "https://three.example.com", "Three", 1, now.Add(-3*time.Hour))
	insertHistoryRow(t, ctx, db, "https://stale.example.com", "Stale", 1, now.AddDate(0, 0, -40))

	scope := repository.HistoryScope{Cutoff: cutoff}

	page, err := repo.GetRecent(ctx, 1, 1, scope)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, "https://two.example.com", page[0].URL)

	// A non-positive limit means all eligible rows, with the offset applied and
	// the stale row excluded by the cutoff.
	rest, err := repo.GetRecent(ctx, 0, 1, scope)
	require.NoError(t, err)
	require.Len(t, rest, 2)
	assert.Equal(t, []string{"https://two.example.com", "https://three.example.com"}, historyURLs(rest))
}

func TestHistoryRepository_Search_BoundedScopeEmptyResult(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -30)
	insertHistoryRow(t, ctx, db, "https://old.example.com/ancient", "Ephemeral Secret", 1, now.AddDate(0, 0, -40))

	scoped, err := repo.Search(ctx, "ephemeral", 10, repository.HistoryScope{Cutoff: cutoff})
	require.NoError(t, err)
	assert.Empty(t, scoped, "a bounded search must not return age-excluded matches")

	all, err := repo.Search(ctx, "ephemeral", 10, repository.HistoryScope{})
	require.NoError(t, err)
	require.Len(t, all, 1)
}

func TestHistoryRepository_GetMostVisitedWithin_NonPositiveLimitIsEmpty(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	insertHistoryRow(t, ctx, db, "https://example.com", "Example", 5, now.Add(-time.Hour))

	unbounded, err := repo.GetMostVisitedWithin(ctx, 0, repository.HistoryScope{})
	require.NoError(t, err)
	assert.Empty(t, unbounded)

	bounded, err := repo.GetMostVisitedWithin(ctx, -5, repository.HistoryScope{Cutoff: now.AddDate(0, 0, -30)})
	require.NoError(t, err)
	assert.Empty(t, bounded)
}

// queryPlan records the actual driver's EXPLAIN QUERY PLAN output for a query.
func queryPlan(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) []string {
	t.Helper()

	rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notUsed, &detail))
		details = append(details, detail)
	}
	require.NoError(t, rows.Err())
	return details
}

func TestHistoryRepository_BoundedRecentPlanUsesCutoffIndex(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Exercise the exact generated statement that historyRepo.GetRecent runs for
	// a bounded scope (see GetRecentHistorySinceCutoff), passed verbatim including
	// sqlc's leading `-- name:` comment. This fails to compile if the bounded
	// variant disappears, and the plan assertions fail if it regresses to a
	// nullable `(? IS NULL OR last_visited >= ?)` predicate, which plans as a
	// SCAN using idx_history_last_visited rather than a SEARCH.
	cutoff := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	plan := queryPlan(t, ctx, db, sqlc.GetRecentHistorySinceCutoff, sql.NullTime{Time: cutoff.UTC(), Valid: true}, 0, 50)

	joined := strings.Join(plan, "\n")
	require.Contains(t, joined, "SEARCH", "bounded recent query must use an index search")
	require.Contains(t, joined, "idx_history_last_visited", "bounded recent query must use the last_visited index")
}

func TestHistoryRepository_ScopeCutoffSubSecondNormalization(t *testing.T) {
	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(t.TempDir(), "dumber.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewHistoryRepository(db)

	// Stored at exactly 12:00:05 with no fractional part, matching
	// CURRENT_TIMESTAMP.
	stored := time.Date(2026, 6, 24, 12, 0, 5, 0, time.UTC)
	insertHistoryRow(t, ctx, db, "https://subsecond.example.com/marker", "Subsecond Marker", 1, stored)
	scope := repository.HistoryScope{Cutoff: stored.Add(500 * time.Millisecond)}

	recent, err := repo.GetRecent(ctx, 10, 0, scope)
	require.NoError(t, err)
	require.Len(t, recent, 1, "a sub-second cutoff must normalize to the stored second and include the row")
	assert.Equal(t, "https://subsecond.example.com/marker", recent[0].URL)

	mostVisited, err := repo.GetMostVisitedWithin(ctx, 10, scope)
	require.NoError(t, err)
	require.Len(t, mostVisited, 1)

	matches, err := repo.Search(ctx, "subsecond", 10, scope)
	require.NoError(t, err)
	require.Len(t, matches, 1, "the shared FTS cutoff helper must apply the same normalization")
	assert.Equal(t, "https://subsecond.example.com/marker", matches[0].Entry.URL)
}

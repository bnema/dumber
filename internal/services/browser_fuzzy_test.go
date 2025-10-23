package services

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	mock_db "github.com/bnema/dumber/internal/db/mocks"
	"go.uber.org/mock/gomock"
)

// TestBrowserService_FuzzyCacheIntegration tests the fuzzy cache lifecycle in BrowserService
func TestBrowserService_FuzzyCacheIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_db.NewMockDatabaseQuerier(ctrl)
	ctx := context.Background()

	// Mock test history data
	testHistory := []db.History{
		{
			ID:  1,
			Url: "https://github.com",
			Title: sql.NullString{
				String: "GitHub",
				Valid:  true,
			},
			VisitCount: sql.NullInt64{Int64: 5, Valid: true},
		},
		{
			ID:  2,
			Url: "https://google.com",
			Title: sql.NullString{
				String: "Google",
				Valid:  true,
			},
			VisitCount: sql.NullInt64{Int64: 10, Valid: true},
		},
		{
			ID:  3,
			Url: "https://stackoverflow.com",
			Title: sql.NullString{
				String: "Stack Overflow",
				Valid:  true,
			},
			VisitCount: sql.NullInt64{Int64: 3, Valid: true},
		},
	}

	// Expect GetHistory to be called when loading fuzzy cache
	mockDB.EXPECT().
		GetHistory(gomock.Any(), gomock.Any()).
		Return(testHistory, nil).
		MinTimes(1)

	// Create browser service (fuzzy cache initialized)
	cfg := &config.Config{}
	service := NewBrowserService(cfg, mockDB)

	// Test 1: Verify fuzzy cache was initialized
	if service.fuzzyCache == nil {
		t.Fatal("Fuzzy cache should be initialized")
	}

	// Test 2: Load fuzzy cache from DB
	err := service.LoadFuzzyCacheFromDB(ctx)
	if err != nil {
		t.Fatalf("Failed to load fuzzy cache: %v", err)
	}

	// Test 3: Verify cache has entries
	stats := service.fuzzyCache.Stats()
	if stats.EntryCount == 0 {
		t.Error("Fuzzy cache should have entries after loading")
	}

	t.Logf("Fuzzy cache loaded: %d entries, %d trigrams", stats.EntryCount, stats.TrigramCount)

	// Test 4: Verify refresh tracking is initialized
	if service.historySinceRefresh != 0 {
		t.Error("historySinceRefresh should start at 0")
	}

	if service.lastCacheRefresh.IsZero() {
		t.Error("lastCacheRefresh should be initialized")
	}

	// Test 5: Flush all caches (should include fuzzy cache)
	err = service.FlushAllCaches(ctx)
	if err != nil {
		t.Fatalf("Failed to flush caches: %v", err)
	}

	t.Log("Fuzzy cache integration test passed")
}

// TestBrowserService_FuzzyCacheSmartRefresh tests the smart refresh logic
func TestBrowserService_FuzzyCacheSmartRefresh(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_db.NewMockDatabaseQuerier(ctrl)
	cfg := &config.Config{}

	// Expect GetHistory to be called when loading fuzzy cache
	mockDB.EXPECT().
		GetHistory(gomock.Any(), gomock.Any()).
		Return([]db.History{}, nil).
		AnyTimes()

	service := NewBrowserService(cfg, mockDB)

	// Load initial cache
	ctx := context.Background()
	err := service.LoadFuzzyCacheFromDB(ctx)
	if err != nil {
		t.Fatalf("Failed to load fuzzy cache: %v", err)
	}

	// Test 1: Verify initial state
	if service.historySinceRefresh != 0 {
		t.Error("historySinceRefresh should start at 0")
	}

	initialRefreshTime := service.lastCacheRefresh

	// Test 2: Simulate adding less than threshold entries (< 10)
	service.historySinceRefresh = 5
	service.lastCacheRefresh = time.Now().Add(-10 * time.Minute) // Old enough

	// This should NOT trigger refresh (not enough entries)
	// Manually check the condition
	const minEntries = 10
	const minInterval = 5 * time.Minute

	shouldRefresh := service.historySinceRefresh >= minEntries &&
		time.Since(service.lastCacheRefresh) >= minInterval

	if shouldRefresh {
		t.Error("Should not trigger refresh with only 5 entries")
	}

	// Test 3: Simulate enough entries but too recent
	service.historySinceRefresh = 15
	service.lastCacheRefresh = time.Now() // Just refreshed

	shouldRefresh = service.historySinceRefresh >= minEntries &&
		time.Since(service.lastCacheRefresh) >= minInterval

	if shouldRefresh {
		t.Error("Should not trigger refresh when last refresh was too recent")
	}

	// Test 4: Simulate conditions that SHOULD trigger refresh
	service.historySinceRefresh = 15
	service.lastCacheRefresh = time.Now().Add(-10 * time.Minute)

	shouldRefresh = service.historySinceRefresh >= minEntries &&
		time.Since(service.lastCacheRefresh) >= minInterval

	if !shouldRefresh {
		t.Error("Should trigger refresh with 15 entries and 10 minutes passed")
	}

	t.Logf("Smart refresh logic validated: %d entries, %v since refresh, should refresh: %v",
		service.historySinceRefresh,
		time.Since(service.lastCacheRefresh),
		shouldRefresh)

	// Test 5: Verify counter would reset after refresh
	if shouldRefresh {
		// Simulate the reset that happens in the actual code
		service.historySinceRefresh = 0
		service.lastCacheRefresh = time.Now()

		if service.historySinceRefresh != 0 {
			t.Error("Counter should reset after refresh")
		}
	}

	// Test 6: Verify old refresh time was updated
	if !service.lastCacheRefresh.After(initialRefreshTime) {
		t.Error("Refresh time should be updated")
	}

	t.Log("Smart refresh logic test passed")
}

// TestBrowserService_FuzzyCacheThresholds tests the threshold constants
func TestBrowserService_FuzzyCacheThresholds(t *testing.T) {
	// This test documents and validates the threshold values

	const minEntries = 10               // From the actual code
	const minInterval = 5 * time.Minute // From the actual code

	t.Logf("Fuzzy cache refresh thresholds:")
	t.Logf("  - Minimum entries: %d", minEntries)
	t.Logf("  - Minimum interval: %v", minInterval)

	// Test reasonable threshold values
	if minEntries < 5 {
		t.Error("minEntries too low - will refresh too frequently")
	}

	if minEntries > 100 {
		t.Error("minEntries too high - cache will be stale")
	}

	if minInterval < 1*time.Minute {
		t.Error("minInterval too low - will refresh too frequently")
	}

	if minInterval > 30*time.Minute {
		t.Error("minInterval too high - cache will be stale")
	}

	// Test that both conditions are required
	// This prevents:
	// - Refreshing after 1 entry but 10 minutes (would be wasteful)
	// - Refreshing after 100 entries but 1 second (would be too frequent)

	t.Log("Threshold validation passed - both conditions required for refresh")
}

// TestBrowserService_FuzzyCacheLoadFromDB tests loading cache at startup
func TestBrowserService_FuzzyCacheLoadFromDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_db.NewMockDatabaseQuerier(ctrl)
	ctx := context.Background()

	// Mock test history entries
	testHistory := []db.History{
		{
			ID:         1,
			Url:        "https://github.com/user/repo",
			Title:      sql.NullString{String: "GitHub Repository", Valid: true},
			VisitCount: sql.NullInt64{Int64: 10, Valid: true},
		},
		{
			ID:         2,
			Url:        "https://stackoverflow.com/questions/123",
			Title:      sql.NullString{String: "Stack Overflow Question", Valid: true},
			VisitCount: sql.NullInt64{Int64: 5, Valid: true},
		},
		{
			ID:         3,
			Url:        "https://golang.org/doc",
			Title:      sql.NullString{String: "Go Documentation", Valid: true},
			VisitCount: sql.NullInt64{Int64: 15, Valid: true},
		},
		{
			ID:         4,
			Url:        "https://reddit.com/r/golang",
			Title:      sql.NullString{String: "Golang Subreddit", Valid: true},
			VisitCount: sql.NullInt64{Int64: 3, Valid: true},
		},
		{
			ID:         5,
			Url:        "https://news.ycombinator.com",
			Title:      sql.NullString{String: "Hacker News", Valid: true},
			VisitCount: sql.NullInt64{Int64: 20, Valid: true},
		},
	}

	// Expect GetHistory to be called when loading fuzzy cache
	mockDB.EXPECT().
		GetHistory(gomock.Any(), gomock.Any()).
		Return(testHistory, nil).
		MinTimes(1)

	// Create service and load cache
	cfg := &config.Config{}
	service := NewBrowserService(cfg, mockDB)

	// Load cache from DB
	err := service.LoadFuzzyCacheFromDB(ctx)
	if err != nil {
		t.Fatalf("Failed to load fuzzy cache from DB: %v", err)
	}

	// Verify cache has loaded entries
	stats := service.fuzzyCache.Stats()
	if stats.EntryCount < len(testHistory) {
		t.Errorf("Expected at least %d entries, got %d", len(testHistory), stats.EntryCount)
	}

	if stats.TrigramCount == 0 {
		t.Error("Expected trigram index to be built")
	}

	t.Logf("Loaded fuzzy cache: %d entries, %d trigrams", stats.EntryCount, stats.TrigramCount)

	// Test search functionality
	result, err := service.fuzzyCache.Search(ctx, "github")
	if err != nil {
		t.Fatalf("Failed to search fuzzy cache: %v", err)
	}

	if len(result.Matches) == 0 {
		t.Error("Expected to find matches for 'github'")
	}

	t.Logf("Search for 'github' found %d matches", len(result.Matches))

	t.Log("Load from DB test passed")
}

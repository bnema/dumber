package cache

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	mock_cache "github.com/bnema/dumber/internal/cache/mocks"
	"github.com/bnema/dumber/internal/db"
	"go.uber.org/mock/gomock"
)

func createTestHistory() []db.History {
	now := time.Now()
	return []db.History{
		{
			ID:          1,
			Url:         "https://github.com",
			Title:       sql.NullString{String: "GitHub", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 25, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-1 * time.Hour), Valid: true},
		},
		{
			ID:          2,
			Url:         "https://stackoverflow.com",
			Title:       sql.NullString{String: "Stack Overflow", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 15, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
		},
		{
			ID:          3,
			Url:         "https://golang.org",
			Title:       sql.NullString{String: "The Go Programming Language", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 8, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-5 * time.Hour), Valid: true},
		},
		{
			ID:          4,
			Url:         "https://news.ycombinator.com",
			Title:       sql.NullString{String: "Hacker News", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 30, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-30 * time.Minute), Valid: true},
		},
		{
			ID:          5,
			Url:         "https://youtube.com",
			Title:       sql.NullString{String: "YouTube", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 50, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-10 * time.Minute), Valid: true},
		},
	}
}

func TestCacheManager_BuildCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQuerier := mock_cache.NewMockHistoryQuerier(ctrl)
	history := createTestHistory()

	// Mock both calls - one for building cache, one for calculating hash
	mockQuerier.EXPECT().GetHistory(gomock.Any(), int64(10000)).Return(history, nil)
	mockQuerier.EXPECT().GetHistory(gomock.Any(), int64(20)).Return(history, nil)

	tempDir, err := os.MkdirTemp("", "dumber_cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	config := DefaultCacheConfig()
	config.CacheFile = filepath.Join(tempDir, "test_cache.bin")
	config.MaxResults = 10

	manager := NewCacheManager(mockQuerier, config)
	ctx := context.Background()

	// Test cache building
	cache, err := manager.GetCache(ctx)
	if err != nil {
		t.Fatalf("Failed to get cache: %v", err)
	}

	if cache.entryCount != 5 {
		t.Errorf("Expected 5 entries, got %d", cache.entryCount)
	}

	// Test that indices are built
	if len(cache.trigramIndex) == 0 {
		t.Error("Trigram index should not be empty")
	}

	if cache.prefixTrie == nil || cache.prefixTrie.Root == nil {
		t.Error("Prefix trie should be initialized")
	}
}

// validateSimilarityScore is a helper function to validate similarity scores
func validateSimilarityScore(t *testing.T, result, expected, delta float64, testName string) {
	if result < 0.0 || result > 1.0 {
		t.Errorf("Score %f should be between 0.0 and 1.0", result)
	}

	diff := result - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > delta {
		t.Errorf("%s: Expected %f ±%f, got %f", testName, expected, delta, result)
	}
}

func TestFuzzySearcher_JaroWinklerSimilarity(t *testing.T) {
	config := DefaultCacheConfig()
	searcher := NewFuzzySearcher(config)

	tests := []struct {
		name     string
		s1       string
		s2       string
		expected float64
		delta    float64
	}{
		{"identical strings", "test", "test", 1.0, 0.0},
		{"empty strings", "", "", 1.0, 0.0},
		{"one empty", "", "test", 0.0, 0.0},
		{"similar strings", "github", "gihub", 0.88, 0.1},
		{"different strings", "github", "stackoverflow", 0.41, 0.05},
		{"prefix match", "programming", "program", 0.9, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := searcher.jaroWinklerSimilarity(tt.s1, tt.s2)
			validateSimilarityScore(t, result, tt.expected, tt.delta, tt.name)
		})
	}
}

func TestFuzzySearcher_SubstringSimilarity(t *testing.T) {
	config := DefaultCacheConfig()
	searcher := NewFuzzySearcher(config)

	tests := []struct {
		name     string
		query    string
		text     string
		expected float64
		delta    float64
	}{
		{"exact match", "github", "github", 1.0, 0.0},
		{"substring at start", "git", "github.com", 0.45, 0.2},
		{"substring at end", "com", "github.com", 0.3, 0.1},
		{"no match", "xyz", "github.com", 0.0, 0.0},
		{"empty query", "", "github.com", 0.0, 0.0},
		{"empty text", "git", "", 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := searcher.substringSimilarity(tt.query, tt.text)
			validateSimilarityScore(t, result, tt.expected, tt.delta, tt.name)
		})
	}
}

func TestFuzzySearcher_TokenizedSimilarity(t *testing.T) {
	config := DefaultCacheConfig()
	searcher := NewFuzzySearcher(config)

	tests := []struct {
		name     string
		query    string
		text     string
		expected float64
		delta    float64
	}{
		{"exact token match", "go programming", "The Go Programming Language", 0.8, 0.2},
		{"partial token match", "stack overflow", "Stack Overflow Questions", 0.8, 0.2},
		{"no token match", "python django", "JavaScript React Framework", 0.0, 0.1},
		{"single token", "github", "GitHub Repository", 0.78, 0.05},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := searcher.tokenizedSimilarity(tt.query, tt.text)

			if result < 0.0 || result > 1.0 {
				t.Errorf("Score %f should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("Expected %f ±%f, got %f", tt.expected, tt.delta, result)
			}
		})
	}
}

func TestFuzzySearcher_RecencyScore(t *testing.T) {
	config := DefaultCacheConfig()
	searcher := NewFuzzySearcher(config)

	now := time.Now()
	tests := []struct {
		name          string
		lastVisitDays uint32
		expected      float64
		delta         float64
	}{
		{"recent visit", DaysFromTime(now.Add(-1 * time.Hour)), 0.95, 0.1},
		{"one day ago", DaysFromTime(now.Add(-24 * time.Hour)), 0.88, 0.1},
		{"one week ago", DaysFromTime(now.Add(-7 * 24 * time.Hour)), 0.7, 0.1},
		{"one month ago", DaysFromTime(now.Add(-30 * 24 * time.Hour)), 0.37, 0.1},
		{"very old", DaysFromTime(now.Add(-365 * 24 * time.Hour)), 0.01, 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := searcher.calculateRecencyScore(tt.lastVisitDays)

			if result < 0.0 || result > 1.0 {
				t.Errorf("Score %f should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("Expected %f ±%f, got %f", tt.expected, tt.delta, result)
			}
		})
	}
}

func TestFuzzySearcher_VisitScore(t *testing.T) {
	config := DefaultCacheConfig()
	searcher := NewFuzzySearcher(config)

	tests := []struct {
		name       string
		visitCount uint16
		expected   float64
		delta      float64
	}{
		{"zero visits", 0, 0.0, 0.0},
		{"single visit", 1, 0.1, 0.05},
		{"moderate visits", 10, 0.35, 0.1},
		{"many visits", 100, 0.67, 0.1},
		{"max visits", 1000, 1.0, 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := searcher.calculateVisitScore(tt.visitCount)

			if result < 0.0 || result > 1.0 {
				t.Errorf("Score %f should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("Expected %f ±%f, got %f", tt.expected, tt.delta, result)
			}
		})
	}
}

func TestTextNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"basic url", "https://github.com", "github com"},
		{"with www", "https://www.google.com", "google com"},
		{"with path", "https://stackoverflow.com/questions/123", "stackoverflow com questions 123"},
		{"mixed case", "GitHub.COM/User", "github com user"},
		{"with special chars", "test-site_name.com", "test site name com"},
		{"unicode chars", "café-münü.com", "café münü com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeText(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTrigramExtraction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"short string", "ab", nil},
		{"exact length", "abc", []string{"abc"}},
		{"longer string", "github", []string{"git", "ith", "thu", "hub"}},
		{"with spaces", "go lang", []string{"go ", "o l", " la", "lan", "ang"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTrigrams(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d trigrams, got %d", len(tt.expected), len(result))
				return
			}
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected trigram %q at index %d, got %q", expected, i, result[i])
				}
			}
		})
	}
}

func TestCompactEntry_BaseScore(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		visitCount  uint16
		lastVisit   time.Time
		expectRange [2]uint16 // min, max expected score
	}{
		{"high visits, recent", 100, now.Add(-1 * time.Hour), [2]uint16{44000, 65535}},
		{"low visits, recent", 5, now.Add(-1 * time.Hour), [2]uint16{35000, 50000}},
		{"high visits, old", 100, now.Add(-90 * 24 * time.Hour), [2]uint16{5000, 25000}},
		{"low visits, old", 5, now.Add(-90 * 24 * time.Hour), [2]uint16{1000, 15000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := CompactEntry{
				VisitCount: tt.visitCount,
				LastVisit:  DaysFromTime(tt.lastVisit),
			}

			score := entry.calculateBaseScore()

			if score < tt.expectRange[0] || score > tt.expectRange[1] {
				t.Errorf("Expected score between %d and %d, got %d",
					tt.expectRange[0], tt.expectRange[1], score)
			}
		})
	}
}

func TestFuzzySearch_Integration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQuerier := mock_cache.NewMockHistoryQuerier(ctrl)
	history := createTestHistory()

	mockQuerier.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return(history, nil).AnyTimes()

	tempDir, err := os.MkdirTemp("", "dumber_search_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	config := DefaultCacheConfig()
	config.CacheFile = filepath.Join(tempDir, "search_cache.bin")
	config.ScoreThreshold = 0.1

	manager := NewCacheManager(mockQuerier, config)
	ctx := context.Background()

	// Critical fuzzy search tests
	searchTests := []struct {
		name            string
		query           string
		expectCount     int
		expectFirst     string
		expectMatchType MatchType
	}{
		{
			name:            "exact domain match",
			query:           "github",
			expectCount:     1,
			expectFirst:     "https://github.com",
			expectMatchType: MatchTypeExact,
		},
		{
			name:            "partial title match",
			query:           "programming",
			expectCount:     1,
			expectFirst:     "https://golang.org",
			expectMatchType: MatchTypeExact,
		},
		{
			name:            "fuzzy typo match",
			query:           "githb", // typo in github
			expectCount:     1,
			expectFirst:     "https://github.com",
			expectMatchType: MatchTypeFuzzy,
		},
		{
			name:            "prefix match",
			query:           "you",
			expectCount:     1,
			expectFirst:     "https://youtube.com",
			expectMatchType: MatchTypePrefix,
		},
		{
			name:        "no matches",
			query:       "nonexistentsite12345",
			expectCount: 0,
		},
	}

	for _, tt := range searchTests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.Search(ctx, tt.query)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			if len(result.Matches) < tt.expectCount {
				t.Errorf("Expected at least %d matches, got %d", tt.expectCount, len(result.Matches))
				return
			}

			if tt.expectCount == 0 {
				return // No further checks needed
			}

			// Check first result
			if result.Matches[0].Entry.URL != tt.expectFirst {
				t.Errorf("Expected first result %q, got %q",
					tt.expectFirst, result.Matches[0].Entry.URL)
			}

			// Verify results are sorted by score
			for i := 1; i < len(result.Matches); i++ {
				if result.Matches[i].Score > result.Matches[i-1].Score {
					t.Errorf("Results not sorted by score: %f > %f at position %d",
						result.Matches[i].Score, result.Matches[i-1].Score, i)
				}
			}

			// Verify all scores meet threshold
			for i, match := range result.Matches {
				if match.Score < config.ScoreThreshold {
					t.Errorf("Match %d score %f below threshold %f",
						i, match.Score, config.ScoreThreshold)
				}
			}

			// Verify query time is recorded
			if result.QueryTime <= 0 {
				t.Error("Query time should be positive")
			}
		})
	}
}

func TestCacheInvalidation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQuerier := mock_cache.NewMockHistoryQuerier(ctrl)

	// First call returns initial history
	firstHistory := createTestHistory()[:3] // Only 3 entries
	secondHistory := createTestHistory()    // All 5 entries

	// Set up multiple expectations for the different calls
	gomock.InOrder(
		mockQuerier.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return(firstHistory, nil).Times(2),    // Build + hash
		mockQuerier.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return(secondHistory, nil).AnyTimes(), // Subsequent calls
	)

	tempDir, err := os.MkdirTemp("", "dumber_invalidation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	config := DefaultCacheConfig()
	config.CacheFile = filepath.Join(tempDir, "invalidation_cache.bin")

	manager := NewCacheManager(mockQuerier, config)
	ctx := context.Background()

	// Initial cache build
	cache1, err := manager.GetCache(ctx)
	if err != nil {
		t.Fatalf("Failed to get initial cache: %v", err)
	}

	if cache1.entryCount != 3 {
		t.Errorf("Expected 3 entries in initial cache, got %d", cache1.entryCount)
	}

	// Invalidate and refresh - wait for completion to prevent mock access after test ends
	done := manager.InvalidateAndRefresh(ctx)
	<-done

	// Get updated cache
	cache2, err := manager.GetCache(ctx)
	if err != nil {
		t.Fatalf("Failed to get updated cache: %v", err)
	}

	if cache2.entryCount != 5 {
		t.Errorf("Expected 5 entries in updated cache, got %d", cache2.entryCount)
	}
}

// create40TestEntries creates a realistic dataset with 40 entries for benchmarking
func create40TestEntries() []db.History {
	now := time.Now()
	entries := make([]db.History, 40)

	urls := []string{
		"https://github.com", "https://stackoverflow.com", "https://golang.org", "https://news.ycombinator.com",
		"https://youtube.com", "https://google.com", "https://reddit.com", "https://twitter.com",
		"https://linkedin.com", "https://medium.com", "https://dev.to", "https://docs.python.org",
		"https://reactjs.org", "https://nodejs.org", "https://kubernetes.io", "https://docker.com",
		"https://aws.amazon.com", "https://cloud.google.com", "https://azure.microsoft.com", "https://heroku.com",
		"https://netlify.com", "https://vercel.com", "https://stripe.com", "https://shopify.com",
		"https://gitlab.com", "https://bitbucket.org", "https://atlassian.com", "https://slack.com",
		"https://discord.com", "https://telegram.org", "https://whatsapp.com", "https://facebook.com",
		"https://instagram.com", "https://tiktok.com", "https://netflix.com", "https://spotify.com",
		"https://apple.com", "https://microsoft.com", "https://amazon.com", "https://wikipedia.org",
	}

	titles := []string{
		"GitHub", "Stack Overflow", "Go Programming Language", "Hacker News",
		"YouTube", "Google", "Reddit", "Twitter",
		"LinkedIn", "Medium", "Dev.to", "Python Documentation",
		"React", "Node.js", "Kubernetes", "Docker",
		"AWS", "Google Cloud", "Microsoft Azure", "Heroku",
		"Netlify", "Vercel", "Stripe", "Shopify",
		"GitLab", "Bitbucket", "Atlassian", "Slack",
		"Discord", "Telegram", "WhatsApp", "Facebook",
		"Instagram", "TikTok", "Netflix", "Spotify",
		"Apple", "Microsoft", "Amazon", "Wikipedia",
	}

	for i := 0; i < 40; i++ {
		entries[i] = db.History{
			ID:          int64(i + 1),
			Url:         urls[i],
			Title:       sql.NullString{String: titles[i], Valid: true},
			VisitCount:  sql.NullInt64{Int64: int64(40 - i), Valid: true}, // Decreasing visit counts
			LastVisited: sql.NullTime{Time: now.Add(-time.Duration(i) * time.Hour), Valid: true},
		}
	}

	return entries
}

// BenchmarkGetTopEntries benchmarks the critical path for dmenu performance
func BenchmarkGetTopEntries(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockQuerier := mock_cache.NewMockHistoryQuerier(ctrl)

	// Setup 40 test entries
	testHistory := create40TestEntries()
	mockQuerier.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return(testHistory, nil).AnyTimes()

	// Create temporary directory for cache file
	tempDir, err := os.MkdirTemp("", "cache_benchmark")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Warning: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	config := DefaultCacheConfig()
	config.CacheFile = filepath.Join(tempDir, "benchmark_cache.bin")
	config.MaxResults = 50

	manager := NewCacheManager(mockQuerier, config)
	ctx := context.Background()

	// Pre-build cache once (this isn't part of the benchmark)
	_, err = manager.GetCache(ctx)
	if err != nil {
		b.Fatalf("Failed to pre-build cache: %v", err)
	}

	b.ResetTimer() // Start timing from here
	b.ReportAllocs()

	// This is the critical path that needs to be under 2ms
	for i := 0; i < b.N; i++ {
		result, err := manager.GetTopEntries(ctx)
		if err != nil {
			b.Fatalf("GetTopEntries failed: %v", err)
		}
		if len(result.Matches) == 0 {
			b.Fatalf("Expected matches, got none")
		}
	}
}

func TestGetBestPrefixMatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQuerier := mock_cache.NewMockHistoryQuerier(ctrl)
	history := createTestHistory()

	mockQuerier.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return(history, nil).AnyTimes()

	tempDir, err := os.MkdirTemp("", "dumber_prefix_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	config := DefaultCacheConfig()
	config.CacheFile = filepath.Join(tempDir, "prefix_cache.bin")

	manager := NewCacheManager(mockQuerier, config)
	ctx := context.Background()

	// Build cache first
	_, err = manager.GetCache(ctx)
	if err != nil {
		t.Fatalf("Failed to get cache: %v", err)
	}

	tests := []struct {
		name        string
		prefix      string
		expectURL   string
		expectEmpty bool
	}{
		{
			name:      "exact prefix match - github",
			prefix:    "https://github",
			expectURL: "https://github.com",
		},
		{
			name:      "partial prefix match - you",
			prefix:    "https://you",
			expectURL: "https://youtube.com",
		},
		{
			name:      "partial prefix match - stack",
			prefix:    "https://stack",
			expectURL: "https://stackoverflow.com",
		},
		{
			name:        "no match",
			prefix:      "https://nonexistent",
			expectEmpty: true,
		},
		{
			name:        "empty prefix",
			prefix:      "",
			expectEmpty: true,
		},
		{
			name:      "case insensitive match",
			prefix:    "https://GITHUB",
			expectURL: "https://github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.GetBestPrefixMatch(ctx, tt.prefix)

			if tt.expectEmpty {
				if result != "" {
					t.Errorf("Expected empty result, got %q", result)
				}
				return
			}

			if result != tt.expectURL {
				t.Errorf("Expected %q, got %q", tt.expectURL, result)
			}
		})
	}
}

func TestGetBestPrefixMatch_ReturnsHighestScore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQuerier := mock_cache.NewMockHistoryQuerier(ctrl)

	// Create history with multiple entries sharing the same prefix
	// "https://go" matches both golang.org and google.com
	now := time.Now()
	history := []db.History{
		{
			ID:          1,
			Url:         "https://golang.org",
			Title:       sql.NullString{String: "Go Programming Language", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 5, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-5 * time.Hour), Valid: true},
		},
		{
			ID:          2,
			Url:         "https://google.com",
			Title:       sql.NullString{String: "Google", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 100, Valid: true}, // Much higher visits
			LastVisited: sql.NullTime{Time: now.Add(-1 * time.Hour), Valid: true}, // More recent
		},
	}

	mockQuerier.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return(history, nil).AnyTimes()

	tempDir, err := os.MkdirTemp("", "dumber_prefix_score_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	config := DefaultCacheConfig()
	config.CacheFile = filepath.Join(tempDir, "prefix_score_cache.bin")

	manager := NewCacheManager(mockQuerier, config)
	ctx := context.Background()

	// Build cache first
	_, err = manager.GetCache(ctx)
	if err != nil {
		t.Fatalf("Failed to get cache: %v", err)
	}

	// Query with prefix that matches both
	result := manager.GetBestPrefixMatch(ctx, "https://go")

	// Should return google.com because it has higher score (more visits, more recent)
	if result != "https://google.com" {
		t.Errorf("Expected https://google.com (highest score), got %q", result)
	}
}

// BenchmarkGetTopEntriesParallel benchmarks concurrent access performance
func BenchmarkGetTopEntriesParallel(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockQuerier := mock_cache.NewMockHistoryQuerier(ctrl)

	// Setup 40 test entries
	testHistory := create40TestEntries()
	mockQuerier.EXPECT().GetHistory(gomock.Any(), gomock.Any()).Return(testHistory, nil).AnyTimes()

	// Create temporary directory for cache file
	tempDir, err := os.MkdirTemp("", "cache_benchmark_parallel")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Warning: failed to remove temp dir %s: %v", tempDir, err)
		}
	}()

	config := DefaultCacheConfig()
	config.CacheFile = filepath.Join(tempDir, "benchmark_parallel_cache.bin")
	config.MaxResults = 50

	manager := NewCacheManager(mockQuerier, config)
	ctx := context.Background()

	// Pre-build cache once (this isn't part of the benchmark)
	_, err = manager.GetCache(ctx)
	if err != nil {
		b.Fatalf("Failed to pre-build cache: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, err := manager.GetTopEntries(ctx)
			if err != nil {
				b.Fatalf("GetTopEntries failed: %v", err)
			}
			if len(result.Matches) == 0 {
				b.Fatalf("Expected matches, got none")
			}
		}
	})
}

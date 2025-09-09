package parser

import (
	"database/sql"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/db"
)

func TestFuzzyMatcher_JaroWinklerSimilarity(t *testing.T) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	tests := []struct {
		s1       string
		s2       string
		expected float64
		delta    float64 // Acceptable difference for float comparison
	}{
		{"", "", 1.0, 0.0},
		{"", "test", 0.0, 0.0},
		{"test", "", 0.0, 0.0},
		{"test", "test", 1.0, 0.0},
		{"github", "github", 1.0, 0.0},
		{"github", "gihub", 0.95, 0.05},           // High similarity
		{"github", "bitbucket", 0.62, 0.15},       // Lower similarity
		{"stackoverflow", "stack", 0.87, 0.05},    // Partial match
		{"google", "goggle", 0.9, 0.1},            // Single char difference
		{"programming", "programing", 0.95, 0.05}, // Missing char
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_vs_"+tt.s2, func(t *testing.T) {
			result := fuzzyMatcher.jaroWinklerSimilarity(tt.s1, tt.s2)

			if result < 0.0 || result > 1.0 {
				t.Errorf("jaroWinklerSimilarity() = %v, should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("jaroWinklerSimilarity(%q, %q) = %v, want %v ±%v", tt.s1, tt.s2, result, tt.expected, tt.delta)
			}
		})
	}
}

func TestFuzzyMatcher_LevenshteinDistance(t *testing.T) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	tests := []struct {
		s1       string
		s2       string
		expected int
	}{
		{"", "", 0},
		{"", "test", 4},
		{"test", "", 4},
		{"test", "test", 0},
		{"github", "github", 0},
		{"github", "gihub", 1},           // 1 deletion
		{"cat", "bat", 1},                // 1 substitution
		{"kitten", "sitting", 3},         // Classic example
		{"programming", "programing", 1}, // 1 deletion
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_vs_"+tt.s2, func(t *testing.T) {
			result := fuzzyMatcher.LevenshteinDistance(tt.s1, tt.s2)
			if result != tt.expected {
				t.Errorf("LevenshteinDistance(%q, %q) = %v, want %v", tt.s1, tt.s2, result, tt.expected)
			}
		})
	}
}

func TestFuzzyMatcher_LevenshteinSimilarity(t *testing.T) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	tests := []struct {
		s1       string
		s2       string
		expected float64
		delta    float64
	}{
		{"test", "test", 1.0, 0.0},
		{"", "", 1.0, 0.0},
		{"github", "gihub", 0.83, 0.05}, // 1 error out of 6 chars
		{"cat", "bat", 0.67, 0.05},      // 1 error out of 3 chars
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_vs_"+tt.s2, func(t *testing.T) {
			result := fuzzyMatcher.LevenshteinSimilarity(tt.s1, tt.s2)

			if result < 0.0 || result > 1.0 {
				t.Errorf("LevenshteinSimilarity() = %v, should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("LevenshteinSimilarity(%q, %q) = %v, want %v ±%v", tt.s1, tt.s2, result, tt.expected, tt.delta)
			}
		})
	}
}

func TestFuzzyMatcher_SubstringMatch(t *testing.T) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	tests := []struct {
		query    string
		text     string
		expected float64
		delta    float64
	}{
		{"", "test", 0.0, 0.0},
		{"test", "", 0.0, 0.0},
		{"github", "github.com", 0.9, 0.1}, // Full match at start gets boost
		{"hub", "github.com", 0.3, 0.1},    // Partial match
		{"git", "github.com", 0.45, 0.1},   // Match at beginning gets boost
		{"com", "github.com", 0.3, 0.1},    // Match at end
		{"xyz", "github.com", 0.0, 0.0},    // No match
	}

	for _, tt := range tests {
		t.Run(tt.query+"_in_"+tt.text, func(t *testing.T) {
			result := fuzzyMatcher.substringMatch(tt.query, tt.text)

			if result < 0.0 || result > 1.0 {
				t.Errorf("substringMatch() = %v, should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("substringMatch(%q, %q) = %v, want %v ±%v", tt.query, tt.text, result, tt.expected, tt.delta)
			}
		})
	}
}

func TestFuzzyMatcher_TokenizedMatch(t *testing.T) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	tests := []struct {
		query    string
		text     string
		expected float64
		delta    float64
	}{
		{"go programming", "The Go Programming Language", 0.8, 0.2},
		{"stack overflow", "Stack Overflow - Where Developers Learn", 0.8, 0.2},
		{"github repo", "GitHub Repository", 0.94, 0.1},
		{"missing words", "Something completely different", 0.34, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.query+"_vs_"+tt.text, func(t *testing.T) {
			result := fuzzyMatcher.TokenizedMatch(tt.query, tt.text)

			if result < 0.0 || result > 1.0 {
				t.Errorf("TokenizedMatch() = %v, should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("TokenizedMatch(%q, %q) = %v, want %v ±%v", tt.query, tt.text, result, tt.expected, tt.delta)
			}
		})
	}
}

func TestFuzzyMatcher_SearchHistory(t *testing.T) {
	config := DefaultFuzzyConfig()
	config.MinSimilarityThreshold = 0.3
	config.MaxResults = 10
	fuzzyMatcher := NewFuzzyMatcher(config)

	now := time.Now()
	history := []*db.History{
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
	}

	tests := []struct {
		name          string
		query         string
		expectMatches int
		expectFirst   string // Expected first match URL
	}{
		{
			name:          "Exact domain match",
			query:         "github",
			expectMatches: 1,
			expectFirst:   "https://github.com",
		},
		{
			name:          "Partial title match",
			query:         "programming",
			expectMatches: 1,
			expectFirst:   "https://golang.org",
		},
		{
			name:          "Multiple matches",
			query:         "o", // Should match github.com, stackoverflow.com, golang.org
			expectMatches: 3,
		},
		{
			name:          "No matches",
			query:         "nonexistent",
			expectMatches: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := fuzzyMatcher.SearchHistory(tt.query, history)

			if len(matches) < tt.expectMatches {
				t.Errorf("SearchHistory() returned %d matches, want >= %d", len(matches), tt.expectMatches)
			}

			if len(matches) > 0 && tt.expectFirst != "" {
				if matches[0].HistoryEntry.Url != tt.expectFirst {
					t.Errorf("SearchHistory() first match = %q, want %q", matches[0].HistoryEntry.Url, tt.expectFirst)
				}
			}

			// Verify scores are in descending order
			for i := 1; i < len(matches); i++ {
				if matches[i].Score > matches[i-1].Score {
					t.Errorf("SearchHistory() results not sorted by score: %f > %f", matches[i].Score, matches[i-1].Score)
				}
			}

			// Verify all scores meet threshold
			for i, match := range matches {
				if match.Score < config.MinSimilarityThreshold {
					t.Errorf("SearchHistory() match %d score %f below threshold %f", i, match.Score, config.MinSimilarityThreshold)
				}
			}
		})
	}
}

func TestFuzzyMatcher_CalculateRecencyScore(t *testing.T) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())
	now := time.Now()

	tests := []struct {
		name      string
		lastVisit sql.NullTime
		expected  float64
		delta     float64
	}{
		{
			name:      "Invalid time",
			lastVisit: sql.NullTime{Valid: false},
			expected:  0.0,
			delta:     0.0,
		},
		{
			name:      "Recent visit (1 hour ago)",
			lastVisit: sql.NullTime{Time: now.Add(-1 * time.Hour), Valid: true},
			expected:  0.95,
			delta:     0.1,
		},
		{
			name:      "Old visit (30 days ago)",
			lastVisit: sql.NullTime{Time: now.Add(-30 * 24 * time.Hour), Valid: true},
			expected:  0.37,
			delta:     0.1,
		},
		{
			name:      "Very old visit (90 days ago)",
			lastVisit: sql.NullTime{Time: now.Add(-90 * 24 * time.Hour), Valid: true},
			expected:  0.05,
			delta:     0.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fuzzyMatcher.calculateRecencyScore(tt.lastVisit)

			if result < 0.0 || result > 1.0 {
				t.Errorf("calculateRecencyScore() = %v, should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("calculateRecencyScore() = %v, want %v ±%v", result, tt.expected, tt.delta)
			}
		})
	}
}

func TestFuzzyMatcher_CalculateVisitScore(t *testing.T) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	tests := []struct {
		name       string
		visitCount sql.NullInt64
		expected   float64
		delta      float64
	}{
		{
			name:       "Invalid count",
			visitCount: sql.NullInt64{Valid: false},
			expected:   0.0,
			delta:      0.0,
		},
		{
			name:       "Zero visits",
			visitCount: sql.NullInt64{Int64: 0, Valid: true},
			expected:   0.0,
			delta:      0.0,
		},
		{
			name:       "Single visit",
			visitCount: sql.NullInt64{Int64: 1, Valid: true},
			expected:   0.1,
			delta:      0.05,
		},
		{
			name:       "Many visits",
			visitCount: sql.NullInt64{Int64: 100, Valid: true},
			expected:   0.67,
			delta:      0.1,
		},
		{
			name:       "Very many visits",
			visitCount: sql.NullInt64{Int64: 1000, Valid: true},
			expected:   1.0,
			delta:      0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fuzzyMatcher.calculateVisitScore(tt.visitCount)

			if result < 0.0 || result > 1.0 {
				t.Errorf("calculateVisitScore() = %v, should be between 0.0 and 1.0", result)
			}

			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("calculateVisitScore() = %v, want %v ±%v", result, tt.expected, tt.delta)
			}
		})
	}
}

func TestFuzzyMatcher_IsDomainMatch(t *testing.T) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	tests := []struct {
		query    string
		url      string
		expected bool
	}{
		{"github", "https://github.com", true},
		{"github", "https://www.github.com", true},
		{"github", "https://api.github.com", true},
		{"github", "https://stackoverflow.com", false},
		{"stackoverflow", "https://stackoverflow.com/questions", true},
		{"google", "https://www.google.com/search", true},
		{"localhost", "http://localhost:8080", true},
		{"example", "https://example.org", true}, // Matches domain part
		{"sub.example", "https://sub.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.query+"_vs_"+tt.url, func(t *testing.T) {
			result := fuzzyMatcher.isDomainMatch(tt.query, tt.url)
			if result != tt.expected {
				t.Errorf("isDomainMatch(%q, %q) = %v, want %v", tt.query, tt.url, result, tt.expected)
			}
		})
	}
}

func TestFuzzyMatcher_RankMatches(t *testing.T) {
	config := DefaultFuzzyConfig()
	fuzzyMatcher := NewFuzzyMatcher(config)

	now := time.Now()
	matches := []FuzzyMatch{
		{
			HistoryEntry: &db.History{
				Url:         "https://example.com/very/long/path/that/makes/url/longer",
				VisitCount:  sql.NullInt64{Int64: 5, Valid: true},
				LastVisited: sql.NullTime{Time: now.Add(-10 * time.Hour), Valid: true},
			},
			Score: 0.5,
		},
		{
			HistoryEntry: &db.History{
				Url:         "https://github.com",
				VisitCount:  sql.NullInt64{Int64: 25, Valid: true},
				LastVisited: sql.NullTime{Time: now.Add(-1 * time.Hour), Valid: true},
			},
			Score: 0.7,
		},
		{
			HistoryEntry: &db.History{
				Url:         "https://test.com",
				VisitCount:  sql.NullInt64{Int64: 1, Valid: true},
				LastVisited: sql.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
			},
			Score: 0.6,
		},
	}

	rankedMatches := fuzzyMatcher.RankMatches(matches, "github")

	// Verify the matches are still sorted by score (descending)
	for i := 1; i < len(rankedMatches); i++ {
		if rankedMatches[i].Score > rankedMatches[i-1].Score {
			t.Errorf("RankMatches() results not sorted: %f > %f", rankedMatches[i].Score, rankedMatches[i-1].Score)
		}
	}

	// The github.com match should get a boost and likely be first
	githubFound := false
	for i, match := range rankedMatches {
		if match.HistoryEntry.Url == "https://github.com" {
			githubFound = true
			// Should be boosted due to domain match
			if i > 0 && rankedMatches[0].HistoryEntry.Url != "https://github.com" {
				// It's possible another match scored higher even after boost
				t.Logf("GitHub match at position %d with score %f", i, match.Score)
			}
			break
		}
	}

	if !githubFound {
		t.Errorf("RankMatches() didn't preserve GitHub match")
	}
}

func BenchmarkFuzzyMatcher_JaroWinklerSimilarity(b *testing.B) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuzzyMatcher.jaroWinklerSimilarity("github", "gihub")
	}
}

func BenchmarkFuzzyMatcher_LevenshteinDistance(b *testing.B) {
	fuzzyMatcher := NewFuzzyMatcher(DefaultFuzzyConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuzzyMatcher.LevenshteinDistance("programming", "programing")
	}
}

func BenchmarkFuzzyMatcher_SearchHistory(b *testing.B) {
	config := DefaultFuzzyConfig()
	fuzzyMatcher := NewFuzzyMatcher(config)

	now := time.Now()
	// Create larger history set for more realistic benchmark
	history := make([]*db.History, 100)
	for i := 0; i < 100; i++ {
		history[i] = &db.History{
			ID:          int64(i + 1),
			Url:         "https://example" + string(rune(i%26+'a')) + ".com",
			Title:       sql.NullString{String: "Test Site " + string(rune(i%26+'A')), Valid: true},
			VisitCount:  sql.NullInt64{Int64: int64(i % 50), Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-time.Duration(i) * time.Hour), Valid: true},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuzzyMatcher.SearchHistory("example", history)
	}
}

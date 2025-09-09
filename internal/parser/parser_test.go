package parser

import (
	"database/sql"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
)

// MockHistoryProvider implements HistoryProvider for testing.
type MockHistoryProvider struct {
	history []*db.History
}

func (m *MockHistoryProvider) GetRecentHistory(limit int) ([]*db.History, error) {
	if limit >= len(m.history) {
		return m.history, nil
	}
	return m.history[:limit], nil
}

func (m *MockHistoryProvider) GetAllHistory() ([]*db.History, error) {
	return m.history, nil
}

func (m *MockHistoryProvider) SearchHistory(query string, limit int) ([]*db.History, error) {
	// Simple mock search - just return entries that contain the query
	result := make([]*db.History, 0)
	for _, entry := range m.history {
		if len(result) >= limit {
			break
		}
		// Simple contains check
		result = append(result, entry)
	}
	return result, nil
}

func (m *MockHistoryProvider) GetHistoryByURL(url string) (*db.History, error) {
	for _, entry := range m.history {
		if entry.Url == url {
			return entry, nil
		}
	}
	return nil, sql.ErrNoRows
}

// createMockHistory creates mock history data for testing.
func createMockHistory() []*db.History {
	now := time.Now()
	return []*db.History{
		{
			ID:          1,
			Url:         "https://github.com",
			Title:       sql.NullString{String: "GitHub", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 25, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-1 * time.Hour), Valid: true},
			CreatedAt:   sql.NullTime{Time: now.Add(-30 * 24 * time.Hour), Valid: true},
		},
		{
			ID:          2,
			Url:         "https://stackoverflow.com",
			Title:       sql.NullString{String: "Stack Overflow", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 15, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
			CreatedAt:   sql.NullTime{Time: now.Add(-20 * 24 * time.Hour), Valid: true},
		},
		{
			ID:          3,
			Url:         "https://reddit.com",
			Title:       sql.NullString{String: "Reddit", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 50, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-3 * time.Hour), Valid: true},
			CreatedAt:   sql.NullTime{Time: now.Add(-10 * 24 * time.Hour), Valid: true},
		},
		{
			ID:          4,
			Url:         "https://golang.org",
			Title:       sql.NullString{String: "The Go Programming Language", Valid: true},
			VisitCount:  sql.NullInt64{Int64: 8, Valid: true},
			LastVisited: sql.NullTime{Time: now.Add(-5 * time.Hour), Valid: true},
			CreatedAt:   sql.NullTime{Time: now.Add(-5 * 24 * time.Hour), Valid: true},
		},
	}
}

// createTestConfig creates a test configuration.
func createTestConfig() *config.Config {
	return &config.Config{
		SearchShortcuts: map[string]config.SearchShortcut{
			"g": {
				URL:         "https://www.google.com/search?q={query}",
				Description: "Google Search",
			},
			"gh": {
				URL:         "https://github.com/search?q={query}",
				Description: "GitHub Search",
			},
			"so": {
				URL:         "https://stackoverflow.com/search?q={query}",
				Description: "Stack Overflow Search",
			},
		},
	}
}

func TestParser_ParseInput_DirectURL(t *testing.T) {
	mockHistory := createMockHistory()
	provider := &MockHistoryProvider{history: mockHistory}
	config := createTestConfig()
	parser := NewParser(config, provider)

	tests := []struct {
		name       string
		input      string
		expectType InputType
		expectURL  string
		expectConf float64
	}{
		{
			name:       "HTTPS URL",
			input:      "https://example.com",
			expectType: InputTypeDirectURL,
			expectURL:  "https://example.com",
			expectConf: 1.0,
		},
		{
			name:       "HTTP URL",
			input:      "http://example.com",
			expectType: InputTypeDirectURL,
			expectURL:  "http://example.com",
			expectConf: 1.0,
		},
		{
			name:       "Domain only",
			input:      "example.com",
			expectType: InputTypeDirectURL,
			expectURL:  "https://example.com",
			expectConf: 1.0,
		},
		{
			name:       "Subdomain",
			input:      "www.example.com",
			expectType: InputTypeDirectURL,
			expectURL:  "https://www.example.com",
			expectConf: 1.0,
		},
		{
			name:       "Domain with path",
			input:      "example.com/path",
			expectType: InputTypeDirectURL,
			expectURL:  "https://example.com/path",
			expectConf: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseInput(tt.input)
			if err != nil {
				t.Fatalf("ParseInput() error = %v", err)
			}

			if result.Type != tt.expectType {
				t.Errorf("ParseInput() type = %v, want %v", result.Type, tt.expectType)
			}

			if result.URL != tt.expectURL {
				t.Errorf("ParseInput() URL = %v, want %v", result.URL, tt.expectURL)
			}

			if result.Confidence != tt.expectConf {
				t.Errorf("ParseInput() confidence = %v, want %v", result.Confidence, tt.expectConf)
			}
		})
	}
}

func TestParser_ParseInput_SearchShortcut(t *testing.T) {
	mockHistory := createMockHistory()
	provider := &MockHistoryProvider{history: mockHistory}
	config := createTestConfig()
	parser := NewParser(config, provider)

	tests := []struct {
		name        string
		input       string
		expectType  InputType
		expectURL   string
		expectKey   string
		expectQuery string
	}{
		{
			name:        "Google search",
			input:       "g: golang tutorial",
			expectType:  InputTypeSearchShortcut,
			expectURL:   "https://www.google.com/search?q=golang tutorial",
			expectKey:   "g",
			expectQuery: "golang tutorial",
		},
		{
			name:        "GitHub search",
			input:       "gh: cobra cli",
			expectType:  InputTypeSearchShortcut,
			expectURL:   "https://github.com/search?q=cobra cli",
			expectKey:   "gh",
			expectQuery: "cobra cli",
		},
		{
			name:        "Stack Overflow search",
			input:       "so: how to fuzzy search",
			expectType:  InputTypeSearchShortcut,
			expectURL:   "https://stackoverflow.com/search?q=how to fuzzy search",
			expectKey:   "so",
			expectQuery: "how to fuzzy search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseInput(tt.input)
			if err != nil {
				t.Fatalf("ParseInput() error = %v", err)
			}

			if result.Type != tt.expectType {
				t.Errorf("ParseInput() type = %v, want %v", result.Type, tt.expectType)
			}

			if result.URL != tt.expectURL {
				t.Errorf("ParseInput() URL = %v, want %v", result.URL, tt.expectURL)
			}

			if result.Shortcut == nil {
				t.Fatalf("ParseInput() shortcut is nil")
			}

			if result.Shortcut.Key != tt.expectKey {
				t.Errorf("ParseInput() shortcut key = %v, want %v", result.Shortcut.Key, tt.expectKey)
			}

			if result.Shortcut.Query != tt.expectQuery {
				t.Errorf("ParseInput() shortcut query = %v, want %v", result.Shortcut.Query, tt.expectQuery)
			}
		})
	}
}

func TestParser_ParseInput_HistorySearch(t *testing.T) {
	mockHistory := createMockHistory()
	provider := &MockHistoryProvider{history: mockHistory}
	config := createTestConfig()
	parser := NewParser(config, provider)

	tests := []struct {
		name          string
		input         string
		expectType    InputType
		expectMatches bool
		minConfidence float64
	}{
		{
			name:          "Exact domain match",
			input:         "github",
			expectType:    InputTypeHistorySearch,
			expectMatches: true,
			minConfidence: 0.5,
		},
		{
			name:          "Partial title match",
			input:         "stack",
			expectType:    InputTypeHistorySearch,
			expectMatches: true,
			minConfidence: 0.3,
		},
		{
			name:          "Programming language search",
			input:         "golang",
			expectType:    InputTypeHistorySearch,
			expectMatches: true,
			minConfidence: 0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseInput(tt.input)
			if err != nil {
				t.Fatalf("ParseInput() error = %v", err)
			}

			if result.Type != tt.expectType {
				t.Errorf("ParseInput() type = %v, want %v", result.Type, tt.expectType)
			}

			if tt.expectMatches && len(result.FuzzyMatches) == 0 {
				t.Errorf("ParseInput() expected fuzzy matches but got none")
			}

			if result.Confidence < tt.minConfidence {
				t.Errorf("ParseInput() confidence = %v, want >= %v", result.Confidence, tt.minConfidence)
			}
		})
	}
}

func TestParser_FuzzySearchHistory(t *testing.T) {
	mockHistory := createMockHistory()
	provider := &MockHistoryProvider{history: mockHistory}
	config := createTestConfig()
	parser := NewParser(config, provider)

	tests := []struct {
		name          string
		query         string
		threshold     float64
		expectMatches int
		expectFirst   string // Expected first match URL
	}{
		{
			name:          "GitHub search",
			query:         "github",
			threshold:     0.3,
			expectMatches: 1,
			expectFirst:   "https://github.com",
		},
		{
			name:          "Programming search",
			query:         "programming",
			threshold:     0.2,
			expectMatches: 1,
			expectFirst:   "https://golang.org",
		},
		{
			name:          "Stack search",
			query:         "stack",
			threshold:     0.3,
			expectMatches: 1,
			expectFirst:   "https://stackoverflow.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := parser.FuzzySearchHistory(tt.query, tt.threshold)
			if err != nil {
				t.Fatalf("FuzzySearchHistory() error = %v", err)
			}

			if len(matches) < tt.expectMatches {
				t.Errorf("FuzzySearchHistory() matches = %d, want >= %d", len(matches), tt.expectMatches)
			}

			if len(matches) > 0 && tt.expectFirst != "" {
				if matches[0].HistoryEntry.Url != tt.expectFirst {
					t.Errorf("FuzzySearchHistory() first match = %v, want %v", matches[0].HistoryEntry.Url, tt.expectFirst)
				}
			}
		})
	}
}

func TestParser_SuggestCompletions(t *testing.T) {
	mockHistory := createMockHistory()
	provider := &MockHistoryProvider{history: mockHistory}
	config := createTestConfig()
	parser := NewParser(config, provider)

	tests := []struct {
		name        string
		input       string
		limit       int
		expectCount int
	}{
		{
			name:        "GitHub completion",
			input:       "gith",
			limit:       5,
			expectCount: 1,
		},
		{
			name:        "Shortcut completion",
			input:       "g",
			limit:       5,
			expectCount: 2, // "g" and "gh" shortcuts
		},
		{
			name:        "Empty input",
			input:       "",
			limit:       5,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completions, err := parser.SuggestCompletions(tt.input, tt.limit)
			if err != nil {
				t.Fatalf("SuggestCompletions() error = %v", err)
			}

			if len(completions) != tt.expectCount {
				t.Errorf("SuggestCompletions() count = %d, want %d", len(completions), tt.expectCount)
			}
		})
	}
}

func TestParser_ProcessShortcut(t *testing.T) {
	config := createTestConfig()
	mockHistory := createMockHistory()
	provider := &MockHistoryProvider{history: mockHistory}
	parser := NewParser(config, provider)

	tests := []struct {
		name      string
		shortcut  string
		query     string
		expectURL string
		expectErr bool
	}{
		{
			name:      "Google shortcut",
			shortcut:  "g",
			query:     "test query",
			expectURL: "https://www.google.com/search?q=test query",
			expectErr: false,
		},
		{
			name:      "GitHub shortcut",
			shortcut:  "gh",
			query:     "golang",
			expectURL: "https://github.com/search?q=golang",
			expectErr: false,
		},
		{
			name:      "Unknown shortcut",
			shortcut:  "unknown",
			query:     "test",
			expectURL: "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := parser.ProcessShortcut(tt.shortcut, tt.query, config.SearchShortcuts)

			if tt.expectErr {
				if err == nil {
					t.Errorf("ProcessShortcut() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("ProcessShortcut() error = %v", err)
			}

			if url != tt.expectURL {
				t.Errorf("ProcessShortcut() URL = %v, want %v", url, tt.expectURL)
			}
		})
	}
}

func TestParser_ValidateConfig(t *testing.T) {
	mockHistory := createMockHistory()
	provider := &MockHistoryProvider{history: mockHistory}

	tests := []struct {
		name      string
		config    *config.Config
		fuzzy     *FuzzyConfig
		provider  HistoryProvider
		expectErr bool
	}{
		{
			name:      "Valid configuration",
			config:    createTestConfig(),
			fuzzy:     DefaultFuzzyConfig(),
			provider:  provider,
			expectErr: false,
		},
		{
			name:      "Nil config",
			config:    nil,
			fuzzy:     DefaultFuzzyConfig(),
			provider:  provider,
			expectErr: true,
		},
		{
			name:      "Nil fuzzy config",
			config:    createTestConfig(),
			fuzzy:     nil,
			provider:  provider,
			expectErr: true,
		},
		{
			name:      "Nil history provider",
			config:    createTestConfig(),
			fuzzy:     DefaultFuzzyConfig(),
			provider:  nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.config, tt.provider, WithFuzzyConfig(tt.fuzzy))

			err := parser.ValidateConfig()

			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateConfig() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateConfig() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestFuzzyConfig_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		config *FuzzyConfig
		expect bool
	}{
		{
			name:   "Default config",
			config: DefaultFuzzyConfig(),
			expect: true,
		},
		{
			name: "Invalid threshold (negative)",
			config: &FuzzyConfig{
				MinSimilarityThreshold: -0.1,
				MaxResults:             10,
				URLWeight:              0.4,
				TitleWeight:            0.3,
				RecencyWeight:          0.2,
				VisitWeight:            0.1,
				RecencyDecayDays:       30,
			},
			expect: false,
		},
		{
			name: "Invalid threshold (>1.0)",
			config: &FuzzyConfig{
				MinSimilarityThreshold: 1.1,
				MaxResults:             10,
				URLWeight:              0.4,
				TitleWeight:            0.3,
				RecencyWeight:          0.2,
				VisitWeight:            0.1,
				RecencyDecayDays:       30,
			},
			expect: false,
		},
		{
			name: "Invalid MaxResults",
			config: &FuzzyConfig{
				MinSimilarityThreshold: 0.3,
				MaxResults:             0,
				URLWeight:              0.4,
				TitleWeight:            0.3,
				RecencyWeight:          0.2,
				VisitWeight:            0.1,
				RecencyDecayDays:       30,
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsValid()
			if result != tt.expect {
				t.Errorf("FuzzyConfig.IsValid() = %v, want %v", result, tt.expect)
			}
		})
	}
}

func TestInputType_String(t *testing.T) {
	tests := []struct {
		inputType InputType
		expected  string
	}{
		{InputTypeDirectURL, "direct_url"},
		{InputTypeSearchShortcut, "search_shortcut"},
		{InputTypeHistorySearch, "history_search"},
		{InputTypeFallbackSearch, "fallback_search"},
		{InputType(99), "unknown"}, // Unknown type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.inputType.String()
			if result != tt.expected {
				t.Errorf("InputType.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

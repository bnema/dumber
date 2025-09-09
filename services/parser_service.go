package services

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/parser"
)

// ParserService wraps the parser functionality for Wails integration.
type ParserService struct {
	parser   *parser.Parser
	dbQueries *db.Queries
}

// NewParserService creates a new ParserService instance.
func NewParserService(cfg *config.Config, queries *db.Queries) *ParserService {
	// Create history provider from database
	historyProvider := &DatabaseHistoryProvider{queries: queries}
	
	// Create parser with configuration
	p := parser.NewParser(cfg, historyProvider)
	
	return &ParserService{
		parser:   p,
		dbQueries: queries,
	}
}

// ParseInput parses user input and returns navigation result.
func (s *ParserService) ParseInput(ctx context.Context, input string) (*parser.ParseResult, error) {
	if input == "" {
		return nil, fmt.Errorf("input cannot be empty")
	}
	
	return s.parser.ParseInput(input)
}

// GetCompletions provides URL/search completions based on input.
func (s *ParserService) GetCompletions(ctx context.Context, input string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10 // Default limit
	}
	
	return s.parser.SuggestCompletions(input, limit)
}

// FuzzySearchHistory performs fuzzy search on history entries.
func (s *ParserService) FuzzySearchHistory(ctx context.Context, query string, threshold float64) ([]parser.FuzzyMatch, error) {
	if threshold <= 0 {
		threshold = 0.3 // Default threshold
	}
	
	return s.parser.FuzzySearchHistory(query, threshold)
}

// GetSupportedShortcuts returns configured search shortcuts.
func (s *ParserService) GetSupportedShortcuts(ctx context.Context) (map[string]config.SearchShortcut, error) {
	return s.parser.GetSupportedShortcuts(), nil
}

// ProcessShortcut processes a shortcut and returns the resulting URL.
func (s *ParserService) ProcessShortcut(ctx context.Context, shortcut, query string) (string, error) {
	shortcuts := s.parser.GetSupportedShortcuts()
	return s.parser.ProcessShortcut(shortcut, query, shortcuts)
}

// CalculateSimilarity calculates similarity between two strings.
func (s *ParserService) CalculateSimilarity(ctx context.Context, s1, s2 string) (float64, error) {
	return s.parser.CalculateSimilarity(s1, s2), nil
}

// ValidateURL checks if the input represents a valid URL.
func (s *ParserService) ValidateURL(ctx context.Context, input string) (bool, error) {
	return s.parser.IsValidURL(input), nil
}

// PreviewParse provides a preview of what ParseInput would return.
func (s *ParserService) PreviewParse(ctx context.Context, input string) (*parser.ParseResult, error) {
	if input == "" {
		return nil, fmt.Errorf("input cannot be empty")
	}
	
	return s.parser.PreviewParse(input)
}

// GetFuzzyConfig returns current fuzzy matching configuration.
func (s *ParserService) GetFuzzyConfig(ctx context.Context) (*parser.FuzzyConfig, error) {
	return s.parser.GetFuzzyConfig(), nil
}

// UpdateFuzzyConfig updates the fuzzy matching configuration.
func (s *ParserService) UpdateFuzzyConfig(ctx context.Context, config *parser.FuzzyConfig) error {
	return s.parser.UpdateFuzzyConfig(config)
}

// DatabaseHistoryProvider implements HistoryProvider using the database.
type DatabaseHistoryProvider struct {
	queries *db.Queries
}

// GetRecentHistory returns recent history entries limited by count.
func (h *DatabaseHistoryProvider) GetRecentHistory(limit int) ([]*db.History, error) {
	ctx := context.Background()
	
	entries, err := h.queries.GetHistory(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	
	// Convert to pointer slice
	result := make([]*db.History, len(entries))
	for i, entry := range entries {
		entryCopy := entry
		result[i] = &entryCopy
	}
	
	return result, nil
}

// GetAllHistory returns all history entries for fuzzy search.
func (h *DatabaseHistoryProvider) GetAllHistory() ([]*db.History, error) {
	ctx := context.Background()
	
	// Use a large limit to get all history
	entries, err := h.queries.GetHistory(ctx, 10000)
	if err != nil {
		return nil, err
	}
	
	// Convert to pointer slice
	result := make([]*db.History, len(entries))
	for i, entry := range entries {
		entryCopy := entry
		result[i] = &entryCopy
	}
	
	return result, nil
}

// SearchHistory performs a basic text search in history.
func (h *DatabaseHistoryProvider) SearchHistory(query string, limit int) ([]*db.History, error) {
	ctx := context.Background()
	
	// Use SQL LIKE pattern for basic search
	urlPattern := sql.NullString{String: "%" + query + "%", Valid: true}
	titlePattern := sql.NullString{String: "%" + query + "%", Valid: true}
	
	entries, err := h.queries.SearchHistory(ctx, urlPattern, titlePattern, int64(limit))
	if err != nil {
		return nil, err
	}
	
	// Convert to pointer slice
	result := make([]*db.History, len(entries))
	for i, entry := range entries {
		entryCopy := entry
		result[i] = &entryCopy
	}
	
	return result, nil
}

// GetHistoryByURL retrieves history entry by exact URL match.
func (h *DatabaseHistoryProvider) GetHistoryByURL(url string) (*db.History, error) {
	ctx := context.Background()
	
	// Search for exact URL match using SearchHistory
	urlPattern := sql.NullString{String: url, Valid: true}
	emptyPattern := sql.NullString{Valid: false}
	
	entries, err := h.queries.SearchHistory(ctx, urlPattern, emptyPattern, 1)
	if err != nil {
		return nil, err
	}
	
	if len(entries) == 0 {
		return nil, fmt.Errorf("history entry not found for URL: %s", url)
	}
	
	return &entries[0], nil
}
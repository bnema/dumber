// Package parser provides URL parsing and fuzzy finding functionality for dumber.
package parser

import (
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
)

// InputType represents the type of user input detected.
type InputType int

const (
	// InputTypeDirectURL represents a direct URL input (e.g., "reddit.com", "https://example.com")
	InputTypeDirectURL InputType = iota
	// InputTypeSearchShortcut represents a search shortcut (e.g., "g: golang tutorial")
	InputTypeSearchShortcut
	// InputTypeHistorySearch represents a history search query
	InputTypeHistorySearch
	// InputTypeFallbackSearch represents a fallback web search
	InputTypeFallbackSearch
)

// ParseResult represents the result of parsing user input.
type ParseResult struct {
	// Type indicates the detected input type
	Type InputType `json:"type"`
	// URL is the final URL to navigate to
	URL string `json:"url"`
	// Query is the original user query
	Query string `json:"query"`
	// Confidence is the confidence score of the parsing (0.0-1.0)
	Confidence float64 `json:"confidence"`
	// FuzzyMatches contains fuzzy search results if applicable
	FuzzyMatches []FuzzyMatch `json:"fuzzy_matches,omitempty"`
	// Shortcut contains the detected shortcut information
	Shortcut *DetectedShortcut `json:"shortcut,omitempty"`
	// ProcessingTime is the time taken to process the input
	ProcessingTime time.Duration `json:"processing_time"`
}

// DetectedShortcut represents a detected search shortcut.
type DetectedShortcut struct {
	// Key is the shortcut key (e.g., "g", "gh")
	Key string `json:"key"`
	// Query is the search query part
	Query string `json:"query"`
	// URL is the constructed URL
	URL string `json:"url"`
	// Description is the shortcut description
	Description string `json:"description"`
}

// FuzzyMatch represents a fuzzy search match from history.
type FuzzyMatch struct {
	// HistoryEntry is the matched history entry
	HistoryEntry *db.History `json:"history_entry"`
	// Score is the similarity score (0.0-1.0, higher is better)
	Score float64 `json:"score"`
	// URLScore is the URL similarity score
	URLScore float64 `json:"url_score"`
	// TitleScore is the title similarity score
	TitleScore float64 `json:"title_score"`
	// RecencyScore is the recency score based on last visit
	RecencyScore float64 `json:"recency_score"`
	// VisitScore is the score based on visit count
	VisitScore float64 `json:"visit_score"`
	// MatchedField indicates which field matched (url, title, or both)
	MatchedField string `json:"matched_field"`
}

// HistoryProvider defines the interface for history data access.
type HistoryProvider interface {
	// GetRecentHistory returns recent history entries limited by count
	GetRecentHistory(limit int) ([]*db.History, error)
	// GetAllHistory returns all history entries for fuzzy search
	GetAllHistory() ([]*db.History, error)
	// SearchHistory performs a basic text search in history
	SearchHistory(query string, limit int) ([]*db.History, error)
	// GetHistoryByURL retrieves history entry by exact URL match
	GetHistoryByURL(url string) (*db.History, error)
}

// FuzzyConfig holds configuration for fuzzy matching algorithms.
type FuzzyConfig struct {
	// MinSimilarityThreshold is the minimum similarity score to consider a match (0.0-1.0)
	MinSimilarityThreshold float64 `json:"min_similarity_threshold"`
	// MaxResults is the maximum number of fuzzy results to return
	MaxResults int `json:"max_results"`
	// URLWeight is the weight for URL similarity in scoring
	URLWeight float64 `json:"url_weight"`
	// TitleWeight is the weight for title similarity in scoring
	TitleWeight float64 `json:"title_weight"`
	// RecencyWeight is the weight for recency in scoring
	RecencyWeight float64 `json:"recency_weight"`
	// VisitWeight is the weight for visit count in scoring
	VisitWeight float64 `json:"visit_weight"`
	// RecencyDecayDays is the number of days for recency score decay
	RecencyDecayDays int `json:"recency_decay_days"`
}

// Parser handles URL parsing and fuzzy finding operations.
type Parser struct {
	config          *config.Config
	fuzzyConfig     *FuzzyConfig
	historyProvider HistoryProvider
}

// ParserOption is a functional option for configuring the Parser.
type ParserOption func(*Parser)

// NewParser creates a new Parser instance with the given configuration.
func NewParser(cfg *config.Config, historyProvider HistoryProvider, opts ...ParserOption) *Parser {
	p := &Parser{
		config:          cfg,
		historyProvider: historyProvider,
		fuzzyConfig:     DefaultFuzzyConfig(),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// WithFuzzyConfig sets custom fuzzy matching configuration.
func WithFuzzyConfig(fuzzyConfig *FuzzyConfig) ParserOption {
	return func(p *Parser) {
		p.fuzzyConfig = fuzzyConfig
	}
}

// DefaultFuzzyConfig returns default fuzzy matching configuration.
func DefaultFuzzyConfig() *FuzzyConfig {
	return &FuzzyConfig{
		MinSimilarityThreshold: 0.3,
		MaxResults:             10,
		URLWeight:              0.4,
		TitleWeight:            0.3,
		RecencyWeight:          0.2,
		VisitWeight:            0.1,
		RecencyDecayDays:       30,
	}
}

// String returns a string representation of InputType.
func (t InputType) String() string {
	switch t {
	case InputTypeDirectURL:
		return "direct_url"
	case InputTypeSearchShortcut:
		return "search_shortcut"
	case InputTypeHistorySearch:
		return "history_search"
	case InputTypeFallbackSearch:
		return "fallback_search"
	default:
		return "unknown"
	}
}

// IsValid returns true if the FuzzyConfig has valid values.
func (fc *FuzzyConfig) IsValid() bool {
	return fc.MinSimilarityThreshold >= 0.0 && fc.MinSimilarityThreshold <= 1.0 &&
		fc.MaxResults > 0 &&
		fc.URLWeight >= 0.0 && fc.TitleWeight >= 0.0 &&
		fc.RecencyWeight >= 0.0 && fc.VisitWeight >= 0.0 &&
		fc.RecencyDecayDays > 0
}

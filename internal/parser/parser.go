package parser

import (
	"fmt"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/config"
)

// ParseInput parses user input and returns a ParseResult with URL and metadata.
//
// Learning Behavior: When users search with shortcuts like "g: viper cli" and visit
// results, those URLs are recorded in history. Future searches for "viper cli" will
// find the GitHub page in history via fuzzy matching, creating intelligent learning.
func (p *Parser) ParseInput(input string) (*ParseResult, error) {
	startTime := time.Now()

	// Sanitize input
	validator := NewURLValidator()
	cleanInput := validator.SanitizeInput(input)

	if cleanInput == "" {
		return &ParseResult{
			Type:           InputTypeFallbackSearch,
			URL:            p.buildSearchURL("", cleanInput),
			Query:          input,
			Confidence:     0.0,
			ProcessingTime: time.Since(startTime),
		}, nil
	}

	// Determine input type
	inputType := validator.GetURLType(cleanInput)

	switch inputType {
	case InputTypeDirectURL:
		return p.parseDirectURL(cleanInput, input, startTime)

	case InputTypeSearchShortcut:
		return p.parseSearchShortcut(cleanInput, input, startTime)

	case InputTypeHistorySearch:
		return p.parseHistorySearch(cleanInput, input, startTime)

	default:
		return p.parseFallbackSearch(cleanInput, input, startTime)
	}
}

// parseDirectURL handles direct URL inputs.
func (p *Parser) parseDirectURL(cleanInput, originalInput string, startTime time.Time) (*ParseResult, error) {
	validator := NewURLValidator()
	normalizedURL := validator.NormalizeURL(cleanInput)

	return &ParseResult{
		Type:           InputTypeDirectURL,
		URL:            normalizedURL,
		Query:          originalInput,
		Confidence:     1.0, // High confidence for direct URLs
		ProcessingTime: time.Since(startTime),
	}, nil
}

// parseSearchShortcut handles search shortcut inputs.
func (p *Parser) parseSearchShortcut(cleanInput, originalInput string, startTime time.Time) (*ParseResult, error) {
	validator := NewURLValidator()
	isShortcut, shortcutKey, query := validator.IsSearchShortcut(cleanInput)

	if !isShortcut {
		return nil, fmt.Errorf("input is not a valid search shortcut")
	}

	// Look up the shortcut in configuration
	shortcuts := p.config.SearchShortcuts
	shortcutConfig, exists := shortcuts[shortcutKey]
	if !exists {
		// Unknown shortcut, treat as search query
		return p.parseHistorySearch(cleanInput, originalInput, startTime)
	}

	// Build URL from shortcut template
	finalURL, err := p.processShortcut(shortcutKey, query, shortcutConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to process shortcut: %w", err)
	}

	return &ParseResult{
		Type:       InputTypeSearchShortcut,
		URL:        finalURL,
		Query:      originalInput,
		Confidence: 0.95, // High confidence for known shortcuts
		Shortcut: &DetectedShortcut{
			Key:         shortcutKey,
			Query:       query,
			URL:         finalURL,
			Description: shortcutConfig.Description,
		},
		ProcessingTime: time.Since(startTime),
	}, nil
}

// parseHistorySearch handles history search with fuzzy finding.
func (p *Parser) parseHistorySearch(cleanInput, originalInput string, startTime time.Time) (*ParseResult, error) {
	// Get history entries for fuzzy search
	history, err := p.historyProvider.GetAllHistory()
	if err != nil {
		// If history search fails, fall back to web search
		return p.parseFallbackSearch(cleanInput, originalInput, startTime)
	}

	// Perform fuzzy search
	fuzzyMatcher := NewFuzzyMatcher(p.fuzzyConfig)
	matches := fuzzyMatcher.SearchHistory(cleanInput, history)

	// Check if we have good matches
	if len(matches) > 0 && matches[0].Score >= p.fuzzyConfig.MinSimilarityThreshold {
		// Use the best match
		bestMatch := matches[0]

		return &ParseResult{
			Type:           InputTypeHistorySearch,
			URL:            bestMatch.HistoryEntry.Url,
			Query:          originalInput,
			Confidence:     bestMatch.Score,
			FuzzyMatches:   matches,
			ProcessingTime: time.Since(startTime),
		}, nil
	}

	// No good history matches, fall back to search
	fallbackResult, err := p.parseFallbackSearch(cleanInput, originalInput, startTime)
	if err != nil {
		return nil, err
	}

	// Include the fuzzy matches even though we're falling back
	fallbackResult.FuzzyMatches = matches

	return fallbackResult, nil
}

// parseFallbackSearch handles fallback web search.
func (p *Parser) parseFallbackSearch(cleanInput, originalInput string, startTime time.Time) (*ParseResult, error) {
	// Use default search shortcut (usually "g" for Google)
	searchURL := p.buildSearchURL("", cleanInput)

	return &ParseResult{
		Type:           InputTypeFallbackSearch,
		URL:            searchURL,
		Query:          originalInput,
		Confidence:     0.1, // Low confidence for fallback
		ProcessingTime: time.Since(startTime),
	}, nil
}

// processShortcut processes a search shortcut and builds the final URL.
func (p *Parser) processShortcut(shortcutKey, query string, shortcut config.SearchShortcut) (string, error) {
	urlTemplate := shortcut.URL

	// Replace placeholder with query
	finalURL := strings.ReplaceAll(urlTemplate, "{query}", query)

	// Validate the resulting URL
	validator := NewURLValidator()
	if !validator.IsValidURL(finalURL) && !strings.HasPrefix(finalURL, "http") {
		return "", fmt.Errorf("invalid URL generated from shortcut template: %s", finalURL)
	}

	return finalURL, nil
}

// buildSearchURL builds a search URL using the default search engine.
func (p *Parser) buildSearchURL(searchEngine, query string) string {
	// If no specific search engine, use Google as default
	if searchEngine == "" {
		defaultShortcut, exists := p.config.SearchShortcuts["g"]
		if exists {
			url := strings.ReplaceAll(defaultShortcut.URL, "{query}", query)
			return url
		}
		// Fallback to Google if no "g" shortcut configured
		return fmt.Sprintf("https://www.google.com/search?q=%s", query)
	}

	// Use specified search engine
	shortcut, exists := p.config.SearchShortcuts[searchEngine]
	if !exists {
		return fmt.Sprintf("https://www.google.com/search?q=%s", query)
	}

	return strings.ReplaceAll(shortcut.URL, "{query}", query)
}

// FuzzySearchHistory performs fuzzy search on history and returns ranked matches.
func (p *Parser) FuzzySearchHistory(query string, threshold float64) ([]FuzzyMatch, error) {
	if query == "" {
		return nil, nil
	}

	// Get all history entries
	history, err := p.historyProvider.GetAllHistory()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve history: %w", err)
	}

	// Create fuzzy config with custom threshold if provided
	fuzzyConfig := *p.fuzzyConfig
	if threshold > 0 && threshold <= 1.0 {
		fuzzyConfig.MinSimilarityThreshold = threshold
	}

	// Perform fuzzy search
	fuzzyMatcher := NewFuzzyMatcher(&fuzzyConfig)
	matches := fuzzyMatcher.SearchHistory(query, history)

	// Apply additional ranking
	matches = fuzzyMatcher.RankMatches(matches, query)

	return matches, nil
}

// CalculateSimilarity calculates similarity between two strings.
func (p *Parser) CalculateSimilarity(s1, s2 string) float64 {
	fuzzyMatcher := NewFuzzyMatcher(p.fuzzyConfig)
	return fuzzyMatcher.calculateSimilarity(s1, s2)
}

// RankMatches re-ranks fuzzy matches based on additional criteria.
func (p *Parser) RankMatches(matches []FuzzyMatch, query string) []FuzzyMatch {
	fuzzyMatcher := NewFuzzyMatcher(p.fuzzyConfig)
	return fuzzyMatcher.RankMatches(matches, query)
}

// IsValidURL checks if the input represents a valid URL.
func (p *Parser) IsValidURL(input string) bool {
	validator := NewURLValidator()
	return validator.IsValidURL(input)
}

// ProcessShortcut processes a shortcut and returns the resulting URL.
func (p *Parser) ProcessShortcut(shortcut, query string, shortcuts map[string]config.SearchShortcut) (string, error) {
	shortcutConfig, exists := shortcuts[shortcut]
	if !exists {
		return "", fmt.Errorf("shortcut '%s' not found", shortcut)
	}

	return p.processShortcut(shortcut, query, shortcutConfig)
}

// GetSupportedShortcuts returns a list of configured search shortcuts.
func (p *Parser) GetSupportedShortcuts() map[string]config.SearchShortcut {
	return p.config.SearchShortcuts
}

// UpdateFuzzyConfig updates the fuzzy matching configuration.
func (p *Parser) UpdateFuzzyConfig(newConfig *FuzzyConfig) error {
	if !newConfig.IsValid() {
		return fmt.Errorf("invalid fuzzy configuration")
	}

	p.fuzzyConfig = newConfig
	return nil
}

// GetFuzzyConfig returns the current fuzzy matching configuration.
func (p *Parser) GetFuzzyConfig() *FuzzyConfig {
	return p.fuzzyConfig
}

// PreviewParse provides a preview of what ParseInput would return without side effects.
func (p *Parser) PreviewParse(input string) (*ParseResult, error) {
	// This is essentially the same as ParseInput but could be extended
	// to avoid any side effects if needed in the future
	return p.ParseInput(input)
}

// SuggestCompletions provides URL/search completions based on input.
func (p *Parser) SuggestCompletions(input string, limit int) ([]string, error) {
	if input == "" || limit <= 0 {
		return nil, nil
	}

	// Get fuzzy matches from history
	matches, err := p.FuzzySearchHistory(input, 0.2) // Lower threshold for suggestions
	if err != nil {
		return nil, err
	}

	completions := make([]string, 0, limit)
	seen := make(map[string]bool)

	// Add URLs from fuzzy matches
	for _, match := range matches {
		if len(completions) >= limit {
			break
		}

		url := match.HistoryEntry.Url
		if !seen[url] {
			completions = append(completions, url)
			seen[url] = true
		}
	}

	// Add search shortcuts if input looks like it might be a shortcut
	if strings.Contains(input, ":") || len(input) <= 3 {
		for shortcut, config := range p.config.SearchShortcuts {
			if len(completions) >= limit {
				break
			}

			if strings.HasPrefix(shortcut, strings.ToLower(input)) {
				suggestion := fmt.Sprintf("%s: %s", shortcut, config.Description)
				if !seen[suggestion] {
					completions = append(completions, suggestion)
					seen[suggestion] = true
				}
			}
		}
	}

	return completions, nil
}

// ValidateConfig validates the parser configuration.
func (p *Parser) ValidateConfig() error {
	if p.config == nil {
		return fmt.Errorf("parser config is nil")
	}

	if p.fuzzyConfig == nil {
		return fmt.Errorf("fuzzy config is nil")
	}

	if !p.fuzzyConfig.IsValid() {
		return fmt.Errorf("invalid fuzzy config")
	}

	if p.historyProvider == nil {
		return fmt.Errorf("history provider is nil")
	}

	// Validate search shortcuts
	if len(p.config.SearchShortcuts) == 0 {
		return fmt.Errorf("no search shortcuts configured")
	}

	// Check for default search shortcut
	if _, hasGoogle := p.config.SearchShortcuts["g"]; !hasGoogle {
		// Warning: no default search configured
	}

	return nil
}

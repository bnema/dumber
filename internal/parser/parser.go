package parser

import (
	"fmt"
	neturl "net/url"
	"regexp"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/config"
)

// ParseInput parses user input and returns a ParseResult with URL and metadata.
// Now uses the proven CLI parsing logic for consistent behavior.
func (p *Parser) ParseInput(input string) (*ParseResult, error) {
	startTime := time.Now()

	if input == "" {
		return &ParseResult{
			Type:           InputTypeFallbackSearch,
			URL:            "https://www.google.com/search?q=",
			Query:          input,
			Confidence:     0.0,
			ProcessingTime: time.Since(startTime),
		}, nil
	}

	// Use the working CLI parsing logic
	finalURL, err := p.parseInputUsingCLILogic(input)
	if err != nil {
		// If parsing fails, fall back to search
		finalURL = fmt.Sprintf("https://www.google.com/search?q=%s", neturl.QueryEscape(input))
	}

	// Determine result type based on the URL
	var resultType InputType = InputTypeFallbackSearch
	confidence := 0.1

	if strings.Contains(finalURL, "google.com/search") {
		resultType = InputTypeFallbackSearch
		confidence = 0.1
	} else if strings.HasPrefix(finalURL, "http://") || strings.HasPrefix(finalURL, "https://") || strings.HasPrefix(finalURL, "dumb://") || strings.HasPrefix(finalURL, "file://") {
		resultType = InputTypeDirectURL
		confidence = 1.0
	}

	// Check if it was a shortcut
	if idx := strings.Index(input, ":"); idx > 0 && idx < 10 && !strings.Contains(input, "://") {
		shortcut := strings.TrimSpace(input[:idx])
		if _, exists := p.config.SearchShortcuts[shortcut]; exists {
			resultType = InputTypeSearchShortcut
			confidence = 0.95
		}
	}

	return &ParseResult{
		Type:           resultType,
		URL:            finalURL,
		Query:          input,
		Confidence:     confidence,
		ProcessingTime: time.Since(startTime),
	}, nil
}

// parseDirectURL handles direct URL inputs using CLI logic.
func (p *Parser) parseDirectURL(cleanInput, originalInput string, startTime time.Time) (*ParseResult, error) {
	// Use the working CLI parsing logic instead
	finalURL, err := p.parseInputUsingCLILogic(cleanInput)
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		Type:           InputTypeDirectURL,
		URL:            finalURL,
		Query:          originalInput,
		Confidence:     1.0,
		ProcessingTime: time.Since(startTime),
	}, nil
}

// parseInputUsingCLILogic implements the working CLI parsing logic
func (p *Parser) parseInputUsingCLILogic(input string) (string, error) {
	input = strings.TrimSpace(input)

	// 1. Check if it's a shortcut (format: "prefix:query")
	if idx := strings.Index(input, ":"); idx > 0 && idx < 10 && !strings.Contains(input, "://") {
		shortcut := strings.TrimSpace(input[:idx])
		query := strings.TrimSpace(input[idx+1:])

		if query != "" {
			// First check configuration-based shortcuts
			if shortcutCfg, exists := p.config.SearchShortcuts[shortcut]; exists {
				return fmt.Sprintf(shortcutCfg.URL, neturl.QueryEscape(query)), nil
			}

			return "", fmt.Errorf("unknown shortcut '%s'", shortcut)
		}
	}

	// 2. Already a full URL with protocol
	if regexp.MustCompile(`^https?://`).MatchString(input) {
		return input, nil
	}

	// 3. File protocol
	if strings.HasPrefix(input, "file://") {
		return input, nil
	}

	// 4. Dumb protocol for custom browser pages
	if strings.HasPrefix(input, "dumb://") {
		return input, nil
	}

	// 5. Localhost/development URLs
	if regexp.MustCompile(`^(localhost|127\.0\.0\.1|0\.0\.0\.0|::1)(:\d+)?(/.*)?$`).MatchString(input) {
		return "http://" + input, nil
	}

	// 6. Local network IPs
	if regexp.MustCompile(`^(192\.168|10\.|172\.(1[6-9]|2[0-9]|3[01]))\.\d+\.\d+(:\d+)?(/.*)?$`).MatchString(input) {
		return "http://" + input, nil
	}

	// 7. Domain with TLD (must have dot and valid TLD)
	// More comprehensive TLD pattern for better URL detection
	domainPattern := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}(/.*)?$`)
	if domainPattern.MatchString(input) {
		// Additional check for common TLDs to reduce false positives
		commonTLDs := regexp.MustCompile(`\.(com|org|net|edu|gov|io|co|uk|de|fr|jp|cn|in|br|au|ca|ru|nl|it|es|se|no|fi|dk|pl|ch|at|be|cz|gr|il|mx|nz|sg|kr|tw|hk|my|th|vn|id|ph|za|eg|ng|ke|dev|app|xyz|tech|site|online|store|blog|info|biz|name|pro)(/.*)?$`)
		if commonTLDs.MatchString(input) {
			return "https://" + input, nil
		}

		// If it has a dot but uncommon TLD, still try as URL
		return "https://" + input, nil
	}

	// 8. Everything else is a search query
	// This includes: single words, multi-word phrases, questions, special characters, etc.
	// Single words without TLDs should be treated as search queries, not hostnames
	return fmt.Sprintf("https://www.google.com/search?q=%s", neturl.QueryEscape(input)), nil
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
	q := neturl.QueryEscape(query)
	finalURL := strings.ReplaceAll(urlTemplate, "{query}", q)
	finalURL = strings.ReplaceAll(finalURL, "%s", q)

	// Validate the resulting URL
	validator := NewURLValidator()
	if !validator.IsValidURL(finalURL) && !strings.HasPrefix(finalURL, "http") {
		return "", fmt.Errorf("invalid URL generated from shortcut '%s' template: %s", shortcutKey, finalURL)
	}

	return finalURL, nil
}

// buildSearchURL builds a search URL using the default search engine.
func (p *Parser) buildSearchURL(searchEngine, query string) string {
	q := neturl.QueryEscape(query)
	// If no specific search engine, use Google as default
	if searchEngine == "" {
		defaultShortcut, exists := p.config.SearchShortcuts["g"]
		if exists {
			url := strings.ReplaceAll(defaultShortcut.URL, "{query}", q)
			url = strings.ReplaceAll(url, "%s", q)
			return url
		}
		// Fallback to Google if no "g" shortcut configured
		return fmt.Sprintf("https://www.google.com/search?q=%s", q)
	}

	// Use specified search engine
	shortcut, exists := p.config.SearchShortcuts[searchEngine]
	if !exists {
		return fmt.Sprintf("https://www.google.com/search?q=%s", q)
	}

	u := strings.ReplaceAll(shortcut.URL, "{query}", q)
	u = strings.ReplaceAll(u, "%s", q)
	return u
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

package parser

import (
	"fmt"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/db"
)

// HistorySearcher provides history search integration with fuzzy matching.
type HistorySearcher struct {
	provider     HistoryProvider
	fuzzyMatcher *FuzzyMatcher
	fuzzyConfig  *FuzzyConfig
}

// NewHistorySearcher creates a new HistorySearcher.
func NewHistorySearcher(provider HistoryProvider, fuzzyConfig *FuzzyConfig) *HistorySearcher {
	if fuzzyConfig == nil {
		fuzzyConfig = DefaultFuzzyConfig()
	}

	return &HistorySearcher{
		provider:     provider,
		fuzzyMatcher: NewFuzzyMatcher(fuzzyConfig),
		fuzzyConfig:  fuzzyConfig,
	}
}

// SearchWithFuzzy performs fuzzy search on history entries.
func (hs *HistorySearcher) SearchWithFuzzy(query string, options ...SearchOption) (*HistorySearchResult, error) {
	if query == "" {
		return &HistorySearchResult{}, nil
	}

	// Apply search options
	searchConfig := &SearchConfig{
		Limit:                  hs.fuzzyConfig.MaxResults,
		MinSimilarityThreshold: hs.fuzzyConfig.MinSimilarityThreshold,
		IncludeExactMatches:    true,
		IncludeFuzzyMatches:    true,
		SortByRelevance:        true,
	}

	for _, opt := range options {
		opt(searchConfig)
	}

	startTime := time.Now()

	// Get all history for fuzzy search
	allHistory, err := hs.provider.GetAllHistory()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve history: %w", err)
	}

	result := &HistorySearchResult{
		Query:        query,
		TotalEntries: len(allHistory),
		SearchTime:   0, // Will be set below
	}

	// Perform exact search first if requested
	if searchConfig.IncludeExactMatches {
		exactMatches, err := hs.provider.SearchHistory(query, searchConfig.Limit)
		if err == nil && len(exactMatches) > 0 {
			// Convert to FuzzyMatch format with perfect scores
			for _, entry := range exactMatches {
				match := FuzzyMatch{
					HistoryEntry: entry,
					Score:        1.0,
					URLScore:     1.0,
					TitleScore:   1.0,
					RecencyScore: hs.fuzzyMatcher.calculateRecencyScore(entry.LastVisited),
					VisitScore:   hs.fuzzyMatcher.calculateVisitScore(entry.VisitCount),
					MatchedField: "exact",
				}
				result.ExactMatches = append(result.ExactMatches, match)
			}
		}
	}

	// Perform fuzzy search if requested
	if searchConfig.IncludeFuzzyMatches {
		// Create temporary config with search options
		tempConfig := *hs.fuzzyConfig
		tempConfig.MinSimilarityThreshold = searchConfig.MinSimilarityThreshold
		tempConfig.MaxResults = searchConfig.Limit

		tempMatcher := NewFuzzyMatcher(&tempConfig)
		fuzzyMatches := tempMatcher.SearchHistory(query, allHistory)

		// Filter out matches that are already in exact matches
		exactURLs := make(map[string]bool)
		for _, exact := range result.ExactMatches {
			exactURLs[exact.HistoryEntry.Url] = true
		}

		for _, match := range fuzzyMatches {
			if !exactURLs[match.HistoryEntry.Url] {
				result.FuzzyMatches = append(result.FuzzyMatches, match)
			}
		}
	}

	// Combine and sort results if requested
	if searchConfig.SortByRelevance {
		result.RankedMatches = hs.combineAndRankMatches(result.ExactMatches, result.FuzzyMatches, query)
	}

	// Limit final results
	if len(result.RankedMatches) > searchConfig.Limit {
		result.RankedMatches = result.RankedMatches[:searchConfig.Limit]
	}

	result.MatchCount = len(result.RankedMatches)
	result.SearchTime = time.Since(startTime)

	return result, nil
}

// SearchByDomain searches history entries by domain with fuzzy matching.
func (hs *HistorySearcher) SearchByDomain(domain string, options ...SearchOption) (*HistorySearchResult, error) {
	// Apply search options
	searchConfig := &SearchConfig{
		Limit:                  hs.fuzzyConfig.MaxResults,
		MinSimilarityThreshold: hs.fuzzyConfig.MinSimilarityThreshold,
		IncludeExactMatches:    true,
		IncludeFuzzyMatches:    true,
		SortByRelevance:        true,
	}

	for _, opt := range options {
		opt(searchConfig)
	}

	// Get all history entries
	allHistory, err := hs.provider.GetAllHistory()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve history: %w", err)
	}

	// Filter by domain and apply fuzzy matching
	domainEntries := make([]*db.History, 0)
	domain = strings.ToLower(domain)

	for _, entry := range allHistory {
		entryDomain := hs.extractDomain(entry.Url)
		if strings.Contains(strings.ToLower(entryDomain), domain) {
			domainEntries = append(domainEntries, entry)
		}
	}

	// If we have domain matches, rank them by fuzzy score
	matches := make([]FuzzyMatch, 0)
	for _, entry := range domainEntries {
		match := hs.fuzzyMatcher.matchHistoryEntry(domain, entry)
		// Apply similarity threshold filter
		if match.Score >= searchConfig.MinSimilarityThreshold {
			matches = append(matches, match)
		}
	}

	// Sort by score if requested
	if searchConfig.SortByRelevance {
		matches = hs.fuzzyMatcher.RankMatches(matches, domain)
	}

	// Apply limit
	if searchConfig.Limit > 0 && len(matches) > searchConfig.Limit {
		matches = matches[:searchConfig.Limit]
	}

	return &HistorySearchResult{
		Query:         domain,
		TotalEntries:  len(allHistory),
		MatchCount:    len(matches),
		RankedMatches: matches,
		SearchTime:    0, // Set by caller if needed
	}, nil
}

// SearchByTimeRange searches history entries within a time range with fuzzy matching.
func (hs *HistorySearcher) SearchByTimeRange(query string, startTime, endTime time.Time) (*HistorySearchResult, error) {
	// Get all history entries
	allHistory, err := hs.provider.GetAllHistory()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve history: %w", err)
	}

	// Filter by time range
	filteredHistory := make([]*db.History, 0)
	for _, entry := range allHistory {
		if entry.LastVisited.Valid {
			visitTime := entry.LastVisited.Time
			if visitTime.After(startTime) && visitTime.Before(endTime) {
				filteredHistory = append(filteredHistory, entry)
			}
		}
	}

	// Perform fuzzy search on filtered entries
	matches := hs.fuzzyMatcher.SearchHistory(query, filteredHistory)

	return &HistorySearchResult{
		Query:         query,
		TotalEntries:  len(filteredHistory),
		MatchCount:    len(matches),
		RankedMatches: matches,
		SearchTime:    time.Since(time.Now()),
	}, nil
}

// GetTopVisited returns the most visited sites with optional fuzzy filtering.
func (hs *HistorySearcher) GetTopVisited(limit int, filterQuery string) ([]FuzzyMatch, error) {
	allHistory, err := hs.provider.GetAllHistory()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve history: %w", err)
	}

	// Sort by visit count
	matches := make([]FuzzyMatch, 0)
	for _, entry := range allHistory {
		// Apply filter if provided
		if filterQuery != "" {
			match := hs.fuzzyMatcher.matchHistoryEntry(filterQuery, entry)
			if match.Score < hs.fuzzyConfig.MinSimilarityThreshold {
				continue
			}
			matches = append(matches, match)
		} else {
			// No filter, just create basic match with visit score
			match := FuzzyMatch{
				HistoryEntry: entry,
				Score:        hs.fuzzyMatcher.calculateVisitScore(entry.VisitCount),
				VisitScore:   hs.fuzzyMatcher.calculateVisitScore(entry.VisitCount),
				RecencyScore: hs.fuzzyMatcher.calculateRecencyScore(entry.LastVisited),
				MatchedField: "visit_count",
			}
			matches = append(matches, match)
		}
	}

	// Sort by visit count (descending)
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			iVisits := int64(0)
			jVisits := int64(0)
			if matches[i].HistoryEntry.VisitCount.Valid {
				iVisits = matches[i].HistoryEntry.VisitCount.Int64
			}
			if matches[j].HistoryEntry.VisitCount.Valid {
				jVisits = matches[j].HistoryEntry.VisitCount.Int64
			}
			if jVisits > iVisits {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Limit results
	if len(matches) > limit {
		matches = matches[:limit]
	}

	return matches, nil
}

// GetRecentWithFuzzy returns recent entries with optional fuzzy filtering.
func (hs *HistorySearcher) GetRecentWithFuzzy(limit int, filterQuery string) ([]FuzzyMatch, error) {
	recentHistory, err := hs.provider.GetRecentHistory(limit * 2) // Get more to filter
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve recent history: %w", err)
	}

	if filterQuery == "" {
		// No filter, convert to FuzzyMatch format
		matches := make([]FuzzyMatch, 0, len(recentHistory))
		for _, entry := range recentHistory {
			match := FuzzyMatch{
				HistoryEntry: entry,
				Score:        1.0, // Recent entries get full score
				RecencyScore: hs.fuzzyMatcher.calculateRecencyScore(entry.LastVisited),
				VisitScore:   hs.fuzzyMatcher.calculateVisitScore(entry.VisitCount),
				MatchedField: "recent",
			}
			matches = append(matches, match)
		}

		if len(matches) > limit {
			matches = matches[:limit]
		}

		return matches, nil
	}

	// Apply fuzzy filter
	matches := hs.fuzzyMatcher.SearchHistory(filterQuery, recentHistory)
	if len(matches) > limit {
		matches = matches[:limit]
	}

	return matches, nil
}

// combineAndRankMatches combines exact and fuzzy matches with proper ranking.
func (hs *HistorySearcher) combineAndRankMatches(exactMatches, fuzzyMatches []FuzzyMatch, query string) []FuzzyMatch {
	combined := make([]FuzzyMatch, 0, len(exactMatches)+len(fuzzyMatches))

	// Add exact matches first (they get priority)
	combined = append(combined, exactMatches...)

	// Add fuzzy matches
	combined = append(combined, fuzzyMatches...)

	// Re-rank all matches
	return hs.fuzzyMatcher.RankMatches(combined, query)
}

// extractDomain extracts domain from URL for domain-based searches.
func (hs *HistorySearcher) extractDomain(url string) string {
	// Simple domain extraction
	domain := url
	if strings.HasPrefix(domain, "http://") {
		domain = domain[7:]
	} else if strings.HasPrefix(domain, "https://") {
		domain = domain[8:]
	}

	// Remove path
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	// Remove port
	if idx := strings.LastIndex(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	return domain
}

// HistorySearchResult represents the result of a history search operation.
type HistorySearchResult struct {
	Query         string        `json:"query"`
	TotalEntries  int           `json:"total_entries"`
	MatchCount    int           `json:"match_count"`
	ExactMatches  []FuzzyMatch  `json:"exact_matches,omitempty"`
	FuzzyMatches  []FuzzyMatch  `json:"fuzzy_matches,omitempty"`
	RankedMatches []FuzzyMatch  `json:"ranked_matches"`
	SearchTime    time.Duration `json:"search_time"`
}

// SearchConfig holds configuration for history search operations.
type SearchConfig struct {
	Limit                  int     `json:"limit"`
	MinSimilarityThreshold float64 `json:"min_similarity_threshold"`
	IncludeExactMatches    bool    `json:"include_exact_matches"`
	IncludeFuzzyMatches    bool    `json:"include_fuzzy_matches"`
	SortByRelevance        bool    `json:"sort_by_relevance"`
}

// SearchOption is a functional option for configuring search operations.
type SearchOption func(*SearchConfig)

// WithLimit sets the maximum number of results to return.
func WithLimit(limit int) SearchOption {
	return func(config *SearchConfig) {
		config.Limit = limit
	}
}

// WithSimilarityThreshold sets the minimum similarity threshold for matches.
func WithSimilarityThreshold(threshold float64) SearchOption {
	return func(config *SearchConfig) {
		config.MinSimilarityThreshold = threshold
	}
}

// WithExactMatches enables/disables exact match inclusion.
func WithExactMatches(include bool) SearchOption {
	return func(config *SearchConfig) {
		config.IncludeExactMatches = include
	}
}

// WithFuzzyMatches enables/disables fuzzy match inclusion.
func WithFuzzyMatches(include bool) SearchOption {
	return func(config *SearchConfig) {
		config.IncludeFuzzyMatches = include
	}
}

// WithRelevanceSort enables/disables relevance-based sorting.
func WithRelevanceSort(enable bool) SearchOption {
	return func(config *SearchConfig) {
		config.SortByRelevance = enable
	}
}

// GetBestMatch returns the single best match for a query.
func (hs *HistorySearcher) GetBestMatch(query string) (*FuzzyMatch, error) {
	result, err := hs.SearchWithFuzzy(query, WithLimit(1))
	if err != nil {
		return nil, err
	}

	if len(result.RankedMatches) == 0 {
		return nil, nil
	}

	return &result.RankedMatches[0], nil
}

// GetMatchesByScore returns matches above a specific score threshold.
func (hs *HistorySearcher) GetMatchesByScore(query string, minScore float64) ([]FuzzyMatch, error) {
	result, err := hs.SearchWithFuzzy(query, WithSimilarityThreshold(minScore))
	if err != nil {
		return nil, err
	}

	return result.RankedMatches, nil
}

// UpdateFuzzyConfig updates the fuzzy matching configuration.
func (hs *HistorySearcher) UpdateFuzzyConfig(config *FuzzyConfig) {
	hs.fuzzyConfig = config
	hs.fuzzyMatcher = NewFuzzyMatcher(config)
}

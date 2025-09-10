package cache

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// FuzzySearcher provides high-performance fuzzy search capabilities.
type FuzzySearcher struct {
	config     *CacheConfig
	resultPool sync.Pool
}

// NewFuzzySearcher creates a new fuzzy searcher with the given config.
func NewFuzzySearcher(config *CacheConfig) *FuzzySearcher {
	fs := &FuzzySearcher{
		config: config,
	}

	// Initialize result pool for zero-allocation queries
	fs.resultPool = sync.Pool{
		New: func() interface{} {
			return &FuzzyResult{
				Matches: make([]FuzzyMatch, 0, config.MaxResults),
			}
		},
	}

	return fs
}

// Search performs a fuzzy search on the cache and returns ranked results.
func (c *DmenuFuzzyCache) Search(query string, config *CacheConfig) *FuzzyResult {
	if len(query) == 0 {
		return c.getTopEntries(config)
	}

	startTime := time.Now()

	// Get result object from pool
	searcher := NewFuzzySearcher(config)
	result := searcher.resultPool.Get().(*FuzzyResult)
	result.Reset()

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Phase 1: Fast candidate selection using indices
	candidates := c.getAllCandidates(query)

	// Phase 2: Score candidates using fuzzy algorithms
	matches := make([]FuzzyMatch, 0, len(candidates))

	if len(candidates) == 0 {
		// Fallback: search all entries (should be rare)
		candidates = make([]uint32, len(c.entries))
		for i := range c.entries {
			candidates[i] = uint32(i)
		}
	}

	// Limit candidates to avoid excessive computation
	if len(candidates) > 1000 {
		candidates = candidates[:1000]
	}

	for _, entryID := range candidates {
		if int(entryID) >= len(c.entries) {
			continue
		}

		match := searcher.scoreEntry(query, &c.entries[entryID])
		if match.Score >= config.ScoreThreshold {
			matches = append(matches, match)
		}
	}

	// Phase 3: Sort by score and apply limits
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > config.MaxResults {
		matches = matches[:config.MaxResults]
	}

	result.Matches = matches
	result.QueryTime = time.Since(startTime)

	return result
}

// getTopEntries returns the top entries when no query is provided.
func (c *DmenuFuzzyCache) getTopEntries(config *CacheConfig) *FuzzyResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	startTime := time.Now()

	// Use pre-sorted index for O(1) access to top entries
	maxResults := config.MaxResults
	if maxResults > len(c.sortedIndex) {
		maxResults = len(c.sortedIndex)
	}

	matches := make([]FuzzyMatch, maxResults)
	for i := 0; i < maxResults; i++ {
		entryID := c.sortedIndex[i]
		matches[i] = FuzzyMatch{
			Entry:        &c.entries[entryID],
			Score:        float64(c.entries[entryID].Score) / 65535.0, // Normalize to 0-1
			RecencyScore: 1.0,                                         // No query, so full recency weight
			VisitScore:   1.0,                                         // No query, so full visit weight
			MatchType:    MatchTypeExact,
		}
	}

	return &FuzzyResult{
		Matches:   matches,
		QueryTime: time.Since(startTime),
	}
}

// scoreEntry calculates the fuzzy match score for a single entry.
func (fs *FuzzySearcher) scoreEntry(query string, entry *CompactEntry) FuzzyMatch {
	match := FuzzyMatch{
		Entry: entry,
	}

	queryNorm := normalizeText(query)
	urlNorm := normalizeText(entry.URL)
	titleNorm := normalizeText(entry.Title)

	// Calculate different types of similarity
	match.URLScore = fs.calculateTextSimilarity(queryNorm, urlNorm)
	match.TitleScore = fs.calculateTextSimilarity(queryNorm, titleNorm)

	// Boost exact matches
	if strings.Contains(urlNorm, queryNorm) {
		match.URLScore = math.Max(match.URLScore, 0.95)
		match.MatchType = MatchTypeExact
	}
	if strings.Contains(titleNorm, queryNorm) {
		match.TitleScore = math.Max(match.TitleScore, 0.95)
		match.MatchType = MatchTypeExact
	}

	// Boost prefix matches
	if strings.HasPrefix(urlNorm, queryNorm) {
		match.URLScore = math.Max(match.URLScore, 0.9)
		if match.MatchType != MatchTypeExact {
			match.MatchType = MatchTypePrefix
		}
	}
	if strings.HasPrefix(titleNorm, queryNorm) {
		match.TitleScore = math.Max(match.TitleScore, 0.9)
		if match.MatchType != MatchTypeExact {
			match.MatchType = MatchTypePrefix
		}
	}

	// Calculate recency and visit scores
	match.RecencyScore = fs.calculateRecencyScore(entry.LastVisit)
	match.VisitScore = fs.calculateVisitScore(entry.VisitCount)

	// Determine final match type if not already set
	if match.MatchType == 0 {
		if match.URLScore > 0.7 || match.TitleScore > 0.7 {
			match.MatchType = MatchTypeFuzzy
		} else {
			match.MatchType = MatchTypeTrigram
		}
	}

	// Calculate weighted final score
	match.Score = fs.config.URLWeight*match.URLScore +
		fs.config.TitleWeight*match.TitleScore +
		fs.config.RecencyWeight*match.RecencyScore +
		fs.config.VisitWeight*match.VisitScore

	return match
}

// calculateTextSimilarity computes similarity between query and text using multiple algorithms.
func (fs *FuzzySearcher) calculateTextSimilarity(query, text string) float64 {
	if query == text {
		return 1.0
	}

	if len(query) == 0 || len(text) == 0 {
		return 0.0
	}

	// Use different algorithms based on text length for optimal performance
	if len(query) <= 3 {
		// For short queries, use substring matching
		return fs.substringSimilarity(query, text)
	} else if len(query) <= 10 {
		// For medium queries, use Jaro-Winkler
		return fs.jaroWinklerSimilarity(query, text)
	} else {
		// For long queries, use tokenized matching
		return fs.tokenizedSimilarity(query, text)
	}
}

// substringSimilarity calculates similarity based on substring matches with position weighting.
func (fs *FuzzySearcher) substringSimilarity(query, text string) float64 {
	index := strings.Index(text, query)
	if index == -1 {
		return 0.0
	}

	// Base score
	baseScore := float64(len(query)) / float64(len(text))

	// Position bonus (earlier matches are better)
	positionBonus := 1.0
	if index == 0 {
		positionBonus = 1.5 // Start of string
	} else if index < len(text)/3 {
		positionBonus = 1.2 // First third
	}

	score := baseScore * positionBonus
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// jaroWinklerSimilarity calculates Jaro-Winkler similarity (optimized version).
func (fs *FuzzySearcher) jaroWinklerSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Calculate match window
	matchWindow := (max(len1, len2) / 2) - 1
	if matchWindow < 0 {
		matchWindow = 0
	}

	matches1 := make([]bool, len1)
	matches2 := make([]bool, len2)
	matches := 0

	// Find matches
	for i := 0; i < len1; i++ {
		start := max(0, i-matchWindow)
		end := min(i+matchWindow+1, len2)

		for j := start; j < end; j++ {
			if matches2[j] || s1[i] != s2[j] {
				continue
			}
			matches1[i] = true
			matches2[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	// Count transpositions
	transpositions := 0
	k := 0
	for i := 0; i < len1; i++ {
		if !matches1[i] {
			continue
		}
		for !matches2[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	// Calculate Jaro similarity
	jaro := (float64(matches)/float64(len1) +
		float64(matches)/float64(len2) +
		float64(matches-transpositions/2)/float64(matches)) / 3.0

	// Jaro-Winkler modification
	if jaro < 0.7 {
		return jaro
	}

	// Common prefix (up to 4 characters)
	prefix := 0
	for i := 0; i < min(min(len1, len2), 4); i++ {
		if s1[i] == s2[i] {
			prefix++
		} else {
			break
		}
	}

	return jaro + (0.1 * float64(prefix) * (1 - jaro))
}

// tokenizedSimilarity performs token-based matching for longer texts.
func (fs *FuzzySearcher) tokenizedSimilarity(query, text string) float64 {
	queryTokens := strings.Fields(query)
	textTokens := strings.Fields(text)

	if len(queryTokens) == 0 || len(textTokens) == 0 {
		return 0.0
	}

	totalScore := 0.0
	matchedTokens := 0

	for _, queryToken := range queryTokens {
		bestScore := 0.0
		for _, textToken := range textTokens {
			score := fs.jaroWinklerSimilarity(queryToken, textToken)
			if score > bestScore {
				bestScore = score
			}
		}
		if bestScore >= 0.6 { // Token match threshold
			totalScore += bestScore
			matchedTokens++
		}
	}

	if matchedTokens == 0 {
		return 0.0
	}

	// Average score weighted by matched token ratio
	avgScore := totalScore / float64(matchedTokens)
	tokenRatio := float64(matchedTokens) / float64(len(queryTokens))

	return avgScore * tokenRatio
}

// calculateRecencyScore calculates score based on how recently the entry was visited.
func (fs *FuzzySearcher) calculateRecencyScore(lastVisitDays uint32) float64 {
	now := time.Now()
	lastVisit := TimeFromDays(lastVisitDays)
	daysSince := now.Sub(lastVisit).Hours() / 24

	// Exponential decay over 30 days
	return math.Exp(-daysSince / 30.0)
}

// calculateVisitScore calculates score based on visit count with logarithmic scaling.
func (fs *FuzzySearcher) calculateVisitScore(visitCount uint16) float64 {
	if visitCount == 0 {
		return 0.0
	}

	// Logarithmic scaling to prevent highly visited sites from dominating
	score := math.Log1p(float64(visitCount)) / math.Log1p(1000.0)
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// Helper functions
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

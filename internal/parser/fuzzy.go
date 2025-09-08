package parser

import (
	"database/sql"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bnema/dumber/internal/db"
)

// FuzzyMatcher implements efficient fuzzy string matching algorithms.
type FuzzyMatcher struct {
	config *FuzzyConfig
}

// NewFuzzyMatcher creates a new FuzzyMatcher with the given configuration.
func NewFuzzyMatcher(config *FuzzyConfig) *FuzzyMatcher {
	return &FuzzyMatcher{
		config: config,
	}
}

// SearchHistory performs fuzzy search on history entries and returns ranked matches.
func (fm *FuzzyMatcher) SearchHistory(query string, history []*db.History) []FuzzyMatch {
	if len(history) == 0 || query == "" {
		return nil
	}

	query = strings.ToLower(strings.TrimSpace(query))
	matches := make([]FuzzyMatch, 0)

	for _, entry := range history {
		match := fm.matchHistoryEntry(query, entry)
		if match.Score >= fm.config.MinSimilarityThreshold {
			matches = append(matches, match)
		}
	}

	// Sort by score (descending)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Limit results
	if len(matches) > fm.config.MaxResults {
		matches = matches[:fm.config.MaxResults]
	}

	return matches
}

// matchHistoryEntry calculates fuzzy match score for a history entry.
func (fm *FuzzyMatcher) matchHistoryEntry(query string, entry *db.History) FuzzyMatch {
	match := FuzzyMatch{
		HistoryEntry: entry,
	}

	url := strings.ToLower(entry.Url)
	title := ""
	if entry.Title.Valid {
		title = strings.ToLower(entry.Title.String)
	}

	// Calculate URL similarity
	match.URLScore = fm.calculateSimilarity(query, url)
	urlSubstring := fm.substringMatch(query, url)
	if urlSubstring > match.URLScore {
		match.URLScore = urlSubstring
	}

	// Calculate title similarity
	if title != "" {
		match.TitleScore = fm.calculateSimilarity(query, title)
		titleSubstring := fm.substringMatch(query, title)
		if titleSubstring > match.TitleScore {
			match.TitleScore = titleSubstring
		}
	}

	// Calculate recency score
	match.RecencyScore = fm.calculateRecencyScore(entry.LastVisited)

	// Calculate visit count score
	match.VisitScore = fm.calculateVisitScore(entry.VisitCount)

	// Determine matched field
	if match.URLScore >= match.TitleScore {
		match.MatchedField = "url"
		if match.URLScore > 0 && match.TitleScore > 0 {
			match.MatchedField = "both"
		}
	} else {
		match.MatchedField = "title"
	}

	// Calculate weighted final score
	match.Score = fm.config.URLWeight*match.URLScore +
		fm.config.TitleWeight*match.TitleScore +
		fm.config.RecencyWeight*match.RecencyScore +
		fm.config.VisitWeight*match.VisitScore

	return match
}

// calculateSimilarity computes similarity using Jaro-Winkler distance.
func (fm *FuzzyMatcher) calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// Use Jaro-Winkler distance for better results
	return fm.jaroWinklerSimilarity(s1, s2)
}

// substringMatch calculates substring match score with position weighting.
func (fm *FuzzyMatcher) substringMatch(query, text string) float64 {
	if query == "" || text == "" {
		return 0.0
	}

	// Check for exact substring match
	index := strings.Index(text, query)
	if index == -1 {
		return 0.0
	}

	// Base score for substring match
	baseScore := float64(len(query)) / float64(len(text))

	// Boost score if match is at the beginning
	positionWeight := 1.0
	if index == 0 {
		positionWeight = 1.5
	} else if index < len(text)/3 {
		positionWeight = 1.2
	}

	score := baseScore * positionWeight
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// jaroWinklerSimilarity calculates Jaro-Winkler similarity between two strings.
func (fm *FuzzyMatcher) jaroWinklerSimilarity(s1, s2 string) float64 {
	jaroSim := fm.jaroSimilarity(s1, s2)

	if jaroSim < 0.7 {
		return jaroSim
	}

	// Calculate common prefix length (up to 4 characters)
	prefixLength := 0
	maxPrefix := 4
	minLen := len(s1)
	if len(s2) < minLen {
		minLen = len(s2)
	}
	if maxPrefix > minLen {
		maxPrefix = minLen
	}

	for i := 0; i < maxPrefix; i++ {
		if s1[i] == s2[i] {
			prefixLength++
		} else {
			break
		}
	}

	// Winkler modification
	return jaroSim + (0.1 * float64(prefixLength) * (1 - jaroSim))
}

// jaroSimilarity calculates Jaro similarity between two strings.
func (fm *FuzzyMatcher) jaroSimilarity(s1, s2 string) float64 {
	len1 := utf8.RuneCountInString(s1)
	len2 := utf8.RuneCountInString(s2)

	if len1 == 0 && len2 == 0 {
		return 1.0
	}
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Convert to rune slices for proper Unicode handling
	runes1 := []rune(s1)
	runes2 := []rune(s2)

	// Calculate match window
	matchWindow := (maxInt(len1, len2) / 2) - 1
	if matchWindow < 0 {
		matchWindow = 0
	}

	// Track matches
	matches1 := make([]bool, len1)
	matches2 := make([]bool, len2)
	matchCount := 0

	// Find matches
	for i := 0; i < len1; i++ {
		start := maxInt(0, i-matchWindow)
		end := minInt(i+matchWindow+1, len2)

		for j := start; j < end; j++ {
			if matches2[j] || runes1[i] != runes2[j] {
				continue
			}
			matches1[i] = true
			matches2[j] = true
			matchCount++
			break
		}
	}

	if matchCount == 0 {
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
		if runes1[i] != runes2[k] {
			transpositions++
		}
		k++
	}

	// Calculate Jaro similarity
	return (float64(matchCount)/float64(len1) +
		float64(matchCount)/float64(len2) +
		float64(matchCount-transpositions/2)/float64(matchCount)) / 3.0
}

// calculateRecencyScore calculates score based on how recently the entry was visited.
func (fm *FuzzyMatcher) calculateRecencyScore(lastVisited sql.NullTime) float64 {
	if !lastVisited.Valid {
		return 0.0
	}

	now := time.Now()
	daysSince := now.Sub(lastVisited.Time).Hours() / 24

	// Exponential decay based on days since last visit
	decayRate := float64(fm.config.RecencyDecayDays)
	score := math.Exp(-daysSince / decayRate)

	return score
}

// calculateVisitScore calculates score based on visit count with logarithmic scaling.
func (fm *FuzzyMatcher) calculateVisitScore(visitCount sql.NullInt64) float64 {
	if !visitCount.Valid || visitCount.Int64 <= 0 {
		return 0.0
	}

	// Logarithmic scaling to prevent highly visited sites from dominating
	score := math.Log1p(float64(visitCount.Int64)) / math.Log1p(1000) // Normalize to max 1000 visits
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// LevenshteinDistance calculates the Levenshtein distance between two strings.
func (fm *FuzzyMatcher) LevenshteinDistance(s1, s2 string) int {
	runes1 := []rune(s1)
	runes2 := []rune(s2)

	len1 := len(runes1)
	len2 := len(runes2)

	// Create a matrix to store distances
	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
	}

	// Initialize first row and column
	for i := 0; i <= len1; i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	// Fill the matrix
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 1
			if runes1[i-1] == runes2[j-1] {
				cost = 0
			}

			deletion := matrix[i-1][j] + 1
			insertion := matrix[i][j-1] + 1
			substitution := matrix[i-1][j-1] + cost

			matrix[i][j] = minInt(deletion, minInt(insertion, substitution))
		}
	}

	return matrix[len1][len2]
}

// LevenshteinSimilarity converts Levenshtein distance to similarity score (0.0-1.0).
func (fm *FuzzyMatcher) LevenshteinSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	distance := fm.LevenshteinDistance(s1, s2)
	maxLen := maxInt(utf8.RuneCountInString(s1), utf8.RuneCountInString(s2))

	if maxLen == 0 {
		return 1.0
	}

	return 1.0 - float64(distance)/float64(maxLen)
}

// TokenizedMatch performs token-based fuzzy matching for better phrase matching.
func (fm *FuzzyMatcher) TokenizedMatch(query, text string) float64 {
	queryTokens := strings.Fields(strings.ToLower(query))
	textTokens := strings.Fields(strings.ToLower(text))

	if len(queryTokens) == 0 || len(textTokens) == 0 {
		return 0.0
	}

	totalScore := 0.0
	matchedTokens := 0

	for _, queryToken := range queryTokens {
		bestScore := 0.0
		for _, textToken := range textTokens {
			score := fm.calculateSimilarity(queryToken, textToken)
			if score > bestScore {
				bestScore = score
			}
		}
		if bestScore >= 0.5 { // Threshold for token match
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

// RankMatches re-ranks fuzzy matches based on additional criteria.
func (fm *FuzzyMatcher) RankMatches(matches []FuzzyMatch, query string) []FuzzyMatch {
	if len(matches) == 0 {
		return matches
	}

	// Apply additional ranking factors
	for i := range matches {
		match := &matches[i]

		// Boost exact matches
		if strings.Contains(strings.ToLower(match.HistoryEntry.Url), strings.ToLower(query)) {
			match.Score *= 1.2
		}

		// Boost domain matches
		if fm.isDomainMatch(query, match.HistoryEntry.Url) {
			match.Score *= 1.15
		}

		// Boost short URLs (likely more relevant)
		if len(match.HistoryEntry.Url) < 50 {
			match.Score *= 1.05
		}
	}

	// Re-sort by updated scores
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches
}

// isDomainMatch checks if the query matches the domain of the URL.
func (fm *FuzzyMatcher) isDomainMatch(query, url string) bool {
	query = strings.ToLower(strings.TrimSpace(query))

	// Extract domain from URL
	domain := url
	if strings.HasPrefix(url, "http://") {
		domain = url[7:]
	} else if strings.HasPrefix(url, "https://") {
		domain = url[8:]
	}

	// Remove path
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	// Remove port
	if idx := strings.Index(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	domain = strings.ToLower(domain)

	// Check for exact domain match
	if domain == query {
		return true
	}
	
	// Check if query is part of domain (e.g., "github" in "github.com" or "www.github.com")  
	if strings.HasSuffix(domain, "."+query) || strings.HasPrefix(domain, query+".") {
		return true
	}
	
	// Check if query matches a significant part of domain (not just a substring)
	// Split domain by dots and check if any part matches query
	domainParts := strings.Split(domain, ".")
	for _, part := range domainParts {
		if part == query {
			return true
		}
	}
	
	return false
}

// Helper functions
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

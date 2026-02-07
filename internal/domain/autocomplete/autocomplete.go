// Package autocomplete provides domain types and logic for URL autocompletion.
package autocomplete

import (
	stdurl "net/url"
	"strings"
	"unicode/utf8"
)

// SuggestionSource indicates the origin of an autocomplete suggestion.
type SuggestionSource int

const (
	SourceHistory SuggestionSource = iota
	SourceFavorite
	SourceBangShortcut
)

// Suggestion represents a single autocomplete suggestion with its completion suffix.
type Suggestion struct {
	FullText string           // The complete text (URL or bang shortcut)
	Suffix   string           // The suffix to append after the user's input
	Source   SuggestionSource // Where this suggestion came from
	Title    string           // Display title (page title or description)
}

// ComputeCompletionSuffix returns the suffix if input is a case-insensitive prefix of fullText.
// Returns the suffix and true if input matches as a prefix, otherwise empty string and false.
func ComputeCompletionSuffix(input, fullText string) (string, bool) {
	if input == "" || fullText == "" {
		return "", false
	}

	inputLower := strings.ToLower(input)
	fullLower := strings.ToLower(fullText)

	if !strings.HasPrefix(fullLower, inputLower) {
		return "", false
	}

	// Return the original-case suffix from fullText
	suffix := fullText[len(input):]
	return suffix, suffix != ""
}

// StripProtocol removes http:// or https:// prefix from a URL for matching.
func StripProtocol(url string) string {
	if strings.HasPrefix(url, "https://") {
		return url[8:]
	}
	if strings.HasPrefix(url, "http://") {
		return url[7:]
	}
	return url
}

// ComputeURLCompletionSuffix computes the completion suffix for URLs,
// handling protocol stripping for better matching.
// It tries matching with and without protocol prefixes.
func ComputeURLCompletionSuffix(input, fullURL string) (suffix, matchedURL string, ok bool) {
	// First try direct match
	if suffix, ok := ComputeCompletionSuffix(input, fullURL); ok {
		return suffix, fullURL, true
	}

	// Try matching against URL without protocol
	strippedURL := StripProtocol(fullURL)
	if suffix, ok := ComputeCompletionSuffix(input, strippedURL); ok {
		return suffix, strippedURL, true
	}

	// Try matching if input has www. but URL doesn't (or vice versa)
	inputNoWWW := strings.TrimPrefix(input, "www.")
	strippedNoWWW := strings.TrimPrefix(strippedURL, "www.")
	if suffix, ok := ComputeCompletionSuffix(inputNoWWW, strippedNoWWW); ok {
		// Return suffix relative to input
		if strings.HasPrefix(strings.ToLower(strippedURL), strings.ToLower(input)) {
			return strippedURL[len(input):], strippedURL, true
		}
		return suffix, strippedNoWWW, true
	}

	return "", "", false
}

// BestURLCompletion returns the best completion candidate from a list of URLs.
// For host-like queries (for example "goo"), it prefers short host completions
// over deep redirect paths.
func BestURLCompletion(input string, urls []string) (suffix, matchedURL string, ok bool) {
	suffix, matchedURL, _, ok = BestURLCompletionWithIndex(input, urls)
	return suffix, matchedURL, ok
}

// BestURLCompletionWithIndex returns the best completion candidate and its source URL index.
func BestURLCompletionWithIndex(input string, urls []string) (suffix, matchedURL string, index int, ok bool) {
	bestScore := completionScore{
		valid: false,
	}
	bestSuffix := ""
	bestMatched := ""
	bestIndex := -1

	for idx, rawURL := range urls {
		if rawURL == "" {
			continue
		}

		candidateText, candidateSuffix, candidateOK := completionCandidateForURL(input, rawURL)
		if !candidateOK {
			continue
		}

		score := rankCompletion(candidateText, candidateSuffix, idx)
		if !bestScore.valid || score.less(bestScore) {
			bestScore = score
			bestSuffix = candidateSuffix
			bestMatched = candidateText
			bestIndex = idx
		}
	}

	if !bestScore.valid {
		return "", "", -1, false
	}
	return bestSuffix, bestMatched, bestIndex, true
}

func completionCandidateForURL(input, rawURL string) (matchedURL, suffix string, ok bool) {
	if looksLikeHostInput(input) {
		if host := hostForCompletion(rawURL); host != "" {
			for _, candidate := range []string{host, strings.TrimPrefix(host, "www.")} {
				if completionSuffix, completionOK := ComputeCompletionSuffix(input, candidate); completionOK {
					return candidate, completionSuffix, true
				}
			}
		}
	}

	completionSuffix, completionMatched, completionOK := ComputeURLCompletionSuffix(input, rawURL)
	if !completionOK {
		return "", "", false
	}
	return completionMatched, completionSuffix, true
}

func looksLikeHostInput(input string) bool {
	if input == "" || strings.HasPrefix(input, "!") {
		return false
	}
	return !strings.ContainsAny(input, "/?#=& ")
}

func hostForCompletion(raw string) string {
	if raw == "" {
		return ""
	}
	if parsed, err := stdurl.Parse(raw); err == nil && parsed.Host != "" {
		return strings.ToLower(parsed.Hostname())
	}

	trimmed := strings.ToLower(StripProtocol(raw))
	for _, sep := range []string{"/", "?", "#"} {
		if idx := strings.Index(trimmed, sep); idx >= 0 {
			trimmed = trimmed[:idx]
		}
	}
	return strings.TrimSpace(trimmed)
}

type completionScore struct {
	valid         bool
	hasPathOrArgs bool
	suffixLen     int
	totalLen      int
	index         int
}

func rankCompletion(matchedURL, suffix string, index int) completionScore {
	return completionScore{
		valid:         true,
		hasPathOrArgs: strings.ContainsAny(matchedURL, "/?#"),
		suffixLen:     utf8.RuneCountInString(suffix),
		totalLen:      utf8.RuneCountInString(matchedURL),
		index:         index,
	}
}

func (s completionScore) less(other completionScore) bool {
	if s.hasPathOrArgs != other.hasPathOrArgs {
		return !s.hasPathOrArgs
	}
	if s.index != other.index {
		return s.index < other.index
	}
	if s.suffixLen != other.suffixLen {
		return s.suffixLen < other.suffixLen
	}
	if s.totalLen != other.totalLen {
		return s.totalLen < other.totalLen
	}
	return false
}

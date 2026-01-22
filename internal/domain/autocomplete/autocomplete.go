// Package autocomplete provides domain types and logic for URL autocompletion.
package autocomplete

import "strings"

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
func ComputeURLCompletionSuffix(input, fullURL string) (suffix string, matchedURL string, ok bool) {
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

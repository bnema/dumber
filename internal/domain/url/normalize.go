// Package url provides URL manipulation utilities for the browser.
package url

import "strings"

// Normalize adds https:// prefix if missing for URL-like inputs.
// Returns the input unchanged if it already has a scheme or doesn't look like a URL.
func Normalize(input string) string {
	if input == "" {
		return ""
	}

	// Already has scheme
	switch {
	case strings.HasPrefix(input, "http://"):
		return input
	case strings.HasPrefix(input, "https://"):
		return input
	case strings.HasPrefix(input, "dumb://"):
		return input
	case strings.HasPrefix(input, "file://"):
		return input
	case strings.HasPrefix(input, "about:"):
		return input
	}

	// Looks like a URL (contains . and no spaces)
	if strings.Contains(input, ".") && !strings.Contains(input, " ") {
		return "https://" + input
	}

	return input
}

// LooksLikeURL checks if the input appears to be a URL (not a search query).
// Returns true for strings like "github.com", "google.com/search", etc.
func LooksLikeURL(input string) bool {
	if input == "" {
		return false
	}
	// Contains a dot and no spaces = likely a URL
	return strings.Contains(input, ".") && !strings.Contains(input, " ")
}

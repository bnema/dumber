// Package url provides URL manipulation utilities for the browser.
package url

import (
	"net/url"
	"strings"
)

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
// Also returns true for URLs with explicit schemes like "dumb://".
func LooksLikeURL(input string) bool {
	if input == "" {
		return false
	}

	// Explicit schemes should always be treated as URLs.
	switch {
	case strings.HasPrefix(input, "http://"):
		return true
	case strings.HasPrefix(input, "https://"):
		return true
	case strings.HasPrefix(input, "dumb://"):
		return true
	case strings.HasPrefix(input, "file://"):
		return true
	case strings.HasPrefix(input, "about:"):
		return true
	}

	// Contains a dot and no spaces = likely a URL
	return strings.Contains(input, ".") && !strings.Contains(input, " ")
}

// ExtractDomain extracts the normalized domain (host) from a URL string.
// Normalizes by stripping "www." prefix so youtube.com and www.youtube.com
// resolve to the same value.
func ExtractDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.TrimPrefix(parsed.Host, "www.")
}

// SanitizeDomainForFilename converts a domain to a safe filename with .ico extension.
// Replaces unsafe filesystem characters with underscores.
func SanitizeDomainForFilename(domain string) string {
	return sanitizeDomain(domain) + ".ico"
}

// SanitizeDomainForPNG converts a domain to a safe filename with .png extension.
// Used for favicon export for tools like rofi/fuzzel that require PNG format.
func SanitizeDomainForPNG(domain string) string {
	return sanitizeDomain(domain) + ".png"
}

// sanitizeDomain replaces unsafe filesystem characters with underscores.
func sanitizeDomain(domain string) string {
	replacer := strings.NewReplacer(
		":", "_",
		"/", "_",
		"\\", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(domain)
}

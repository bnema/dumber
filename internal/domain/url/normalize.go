// Package url provides URL manipulation utilities for the browser.
package url

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// expandHome expands ~ prefix to user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// looksLikeFilePath returns true if the input looks like a filesystem path.
// This includes absolute paths (/path), relative paths (./path, ../path),
// and home-relative paths (~/path).
func looksLikeFilePath(input string) bool {
	switch {
	case strings.HasPrefix(input, "/"):
		return true
	case strings.HasPrefix(input, "./"):
		return true
	case strings.HasPrefix(input, "../"):
		return true
	case strings.HasPrefix(input, "~/"):
		return true
	}
	return false
}

// Normalize adds https:// prefix if missing for URL-like inputs.
// Returns the input unchanged if it already has a scheme or doesn't look like a URL.
// If the input is an existing local file, returns a file:// URL.
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

	// Check if input looks like a file path
	if looksLikeFilePath(input) {
		expanded := expandHome(input)
		absPath, err := filepath.Abs(expanded)
		if err == nil {
			if _, statErr := os.Stat(absPath); statErr == nil {
				// File exists, convert to file:// URL
				return "file://" + absPath
			}
		}
		// Path-like input but file doesn't exist - return unchanged
		// Don't treat as URL to avoid malformed URLs like https:///path
		return input
	}

	// Check if input is an existing file in current directory (relative without ./ prefix)
	absPath, err := filepath.Abs(input)
	if err == nil {
		if _, statErr := os.Stat(absPath); statErr == nil {
			return "file://" + absPath
		}
	}

	// Looks like a URL (contains . and no spaces)
	if input == "localhost" || strings.HasPrefix(input, "localhost:") || strings.HasPrefix(input, "localhost/") {
		return "http://" + input
	}
	if looksLikeIPAddressWithOptionalPortOrPath(input) {
		return "http://" + input
	}
	if strings.Contains(input, ".") && !strings.Contains(input, " ") {
		return "https://" + input
	}

	return input
}

func looksLikeIPAddressWithOptionalPortOrPath(input string) bool {
	if input == "" || strings.Contains(input, " ") {
		return false
	}

	parsed, err := url.Parse("http://" + input)
	if err != nil {
		return false
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return false
	}

	return net.ParseIP(hostname) != nil
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
	if input == "localhost" || strings.HasPrefix(input, "localhost:") || strings.HasPrefix(input, "localhost/") {
		return true
	}
	if looksLikeIPAddressWithOptionalPortOrPath(input) {
		return true
	}
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

// SanitizeDomainForPNGSized converts a domain to a safe filename with size suffix.
// Example: "google.com" with size 32 -> "google.com.32.png"
// Used for normalized favicon export for tools like rofi/fuzzel.
func SanitizeDomainForPNGSized(domain string, size int) string {
	return fmt.Sprintf("%s.%d.png", sanitizeDomain(domain), size)
}

// TrimLeadingSpacesIfURL removes leading whitespace from input if the trimmed
// result looks like a URL. Returns the original input unchanged if it doesn't
// contain leading spaces or if the trimmed result is not a URL.
// This handles cases like pasting "  https://example.com" from clipboard.
func TrimLeadingSpacesIfURL(input string) string {
	trimmed := strings.TrimLeft(input, " \t")
	if trimmed != input && LooksLikeURL(trimmed) {
		return trimmed
	}
	return input
}

// IsExternalScheme returns true if the URL uses a scheme that should be
// launched externally (e.g., vscode://, vscode-insiders://, spotify://, steam://)
// rather than handled by the browser.
// URL schemes are case-insensitive per RFC 3986.
func IsExternalScheme(uri string) bool {
	if uri == "" {
		return false
	}

	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme == "" {
		return false
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "file", "dumb", "about", "data", "blob", "javascript":
		return false
	default:
		return true
	}
}

// ExtractOrigin extracts the origin (scheme://host) from a URI.
// This normalizes URIs to origins for permission storage and comparison.
// Canonicalizes by lowercasing scheme and host, and omitting default ports.
// Example: "https://example.com/path?query" -> "https://example.com"
// Example: "HTTPS://EXAMPLE.COM:443/" -> "https://example.com"
func ExtractOrigin(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("invalid URI: %w", err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("URI missing scheme or host: %s", uri)
	}

	// Canonicalize: lowercase scheme and hostname
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	if strings.Contains(hostname, ":") && !strings.HasPrefix(hostname, "[") {
		hostname = "[" + hostname + "]"
	}

	// Determine if we should include the port
	port := parsed.Port()
	if port != "" {
		// Omit default ports
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			port = ""
		}
	}

	// Build canonical origin
	origin := scheme + "://" + hostname
	if port != "" {
		origin = origin + ":" + port
	}

	return origin, nil
}

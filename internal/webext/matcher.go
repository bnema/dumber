package webext

import (
	"net/url"
	"regexp"
	"strings"
)

// MatchPattern represents a WebExtension match pattern
// Format: <scheme>://<host><path>
// Examples:
//   - https://*.example.com/*
//   - *://*.google.com/*
//   - <all_urls>
type MatchPattern struct {
	raw    string
	scheme *regexp.Regexp
	host   *regexp.Regexp
	path   *regexp.Regexp
}

// NewMatchPattern creates a new match pattern from a string
func NewMatchPattern(pattern string) (*MatchPattern, error) {
	// Handle special case
	if pattern == "<all_urls>" {
		return &MatchPattern{
			raw:    pattern,
			scheme: regexp.MustCompile("^(https?|file|ftp)$"),
			host:   regexp.MustCompile(".*"),
			path:   regexp.MustCompile(".*"),
		}, nil
	}

	// Parse pattern: <scheme>://<host><path>
	parts := strings.SplitN(pattern, "://", 2)
	if len(parts) != 2 {
		// Invalid pattern, but be lenient
		return nil, nil
	}

	schemePattern := parts[0]
	remainder := parts[1]

	// Split host and path
	hostPath := strings.SplitN(remainder, "/", 2)
	hostPattern := hostPath[0]
	pathPattern := "/"
	if len(hostPath) > 1 {
		pathPattern = "/" + hostPath[1]
	}

	// Convert scheme pattern to regex
	var schemeRe *regexp.Regexp
	switch schemePattern {
	case "*":
		schemeRe = regexp.MustCompile("^(https?|ftp)$")
	case "http":
		schemeRe = regexp.MustCompile("^http$")
	case "https":
		schemeRe = regexp.MustCompile("^https$")
	case "file":
		schemeRe = regexp.MustCompile("^file$")
	case "ftp":
		schemeRe = regexp.MustCompile("^ftp$")
	default:
		schemeRe = regexp.MustCompile("^" + regexp.QuoteMeta(schemePattern) + "$")
	}

	// Convert host pattern to regex
	hostRe := patternToRegex(hostPattern)

	// Convert path pattern to regex
	pathRe := patternToRegex(pathPattern)

	return &MatchPattern{
		raw:    pattern,
		scheme: schemeRe,
		host:   hostRe,
		path:   pathRe,
	}, nil
}

// Match checks if a URL matches this pattern
func (p *MatchPattern) Match(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Match scheme
	if !p.scheme.MatchString(u.Scheme) {
		return false
	}

	// Match host
	host := u.Hostname()
	if !p.host.MatchString(host) {
		return false
	}

	// Match path
	path := u.Path
	if path == "" {
		path = "/"
	}
	if !p.path.MatchString(path) {
		return false
	}

	return true
}

// patternToRegex converts a match pattern glob to a regex
func patternToRegex(pattern string) *regexp.Regexp {
	// Escape special regex chars except *
	escaped := regexp.QuoteMeta(pattern)

	// Replace escaped \* with .*
	escaped = strings.ReplaceAll(escaped, "\\*", ".*")

	// Anchor the pattern
	return regexp.MustCompile("^" + escaped + "$")
}

// MatchURL checks if a URL matches any of the given patterns
func MatchURL(urlStr string, patterns []string) bool {
	for _, pattern := range patterns {
		mp, err := NewMatchPattern(pattern)
		if err != nil || mp == nil {
			continue
		}
		if mp.Match(urlStr) {
			return true
		}
	}
	return false
}

// ExcludesURL checks if a URL is excluded by any of the given patterns
func ExcludesURL(urlStr string, excludePatterns []string) bool {
	return MatchURL(urlStr, excludePatterns)
}

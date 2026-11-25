package url

import "strings"

// ParseBangShortcut extracts a bang shortcut from input.
// Input must start with "!" followed by shortcut key and a space.
// Returns (shortcutKey, query, found).
//
// Examples:
//
//	"!g golang"      → ("g", "golang", true)
//	"!ddg test"      → ("ddg", "test", true)
//	"!gh repo name"  → ("gh", "repo name", true)
//	"!g"             → ("", "", false) - no query
//	"plain text"     → ("", "", false)
//	"test !g"        → ("", "", false) - bang not at start
func ParseBangShortcut(input string) (shortcut, query string, found bool) {
	shortcut, query, found = ParseBangPrefix(input)
	if !found || query == "" {
		return "", "", false
	}
	return shortcut, query, true
}

// ParseBangPrefix extracts a bang key from input, allowing an empty query.
// The key runs from after "!" to the first space; the query is the trimmed
// rest.
//
// Examples:
//
//	"!gh"         → ("gh", "", true)
//	"!gh dumber"  → ("gh", "dumber", true)
//	"! foo"       → ("", "", false) - empty key
//	"plain text"  → ("", "", false)
func ParseBangPrefix(input string) (key, query string, ok bool) {
	if !strings.HasPrefix(input, "!") {
		return "", "", false
	}
	rest := input[1:]
	if spaceIdx := strings.Index(rest, " "); spaceIdx >= 0 {
		key = rest[:spaceIdx]
		query = strings.TrimSpace(rest[spaceIdx+1:])
	} else {
		key = rest
	}
	if key == "" {
		return "", "", false
	}
	return key, query, true
}

// BuildSearchURL constructs a URL from user input, handling bang shortcuts.
// It checks for bang shortcuts first, then URL-like input, then falls back to default search.
//
// Parameters:
//   - input: user input (e.g., "!g golang", "example.com", "search query")
//   - shortcutURLs: map of shortcut keys to URL templates (e.g., {"g": "https://google.com/search?q=%s"})
//   - defaultSearch: default search engine URL template (e.g., "https://duckduckgo.com/?q=%s")
//
// Returns the resolved URL.
func BuildSearchURL(input string, shortcutURLs map[string]string, defaultSearch string) string {
	if input == "" {
		return ""
	}

	// Check for bang shortcut (e.g., "!g query")
	if shortcutKey, query, found := ParseBangShortcut(input); found {
		if urlTemplate, ok := shortcutURLs[shortcutKey]; ok {
			return strings.Replace(urlTemplate, "%s", query, 1)
		}
		// Unknown bang falls through to default search with original input
	}

	// Check if it looks like a URL
	if LooksLikeURL(input) {
		return Normalize(input)
	}

	// Use default search
	if defaultSearch != "" {
		return strings.Replace(defaultSearch, "%s", input, 1)
	}

	return input
}

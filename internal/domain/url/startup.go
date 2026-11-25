package url

import "strings"

const defaultBrowserStartupURL = "dumb://home"

// DefaultBrowserStartupURL is the user-facing destination used when the browser
// opens without an explicit or configured URL. Engine implementations may still
// use their own technical bootstrap URL (for example, about:blank) before
// navigating here.
func DefaultBrowserStartupURL() string {
	return defaultBrowserStartupURL
}

// ResolveBrowserStartupURL returns the explicit URL when provided, otherwise
// the configured startup URL, otherwise DefaultBrowserStartupURL.
func ResolveBrowserStartupURL(explicit, configured string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(configured); trimmed != "" {
		return trimmed
	}
	return defaultBrowserStartupURL
}

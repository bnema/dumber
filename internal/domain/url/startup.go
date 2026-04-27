package url

const defaultBrowserStartupURL = "dumb://history"

// DefaultBrowserStartupURL is the user-facing destination used when the browser
// opens without an explicit URL. Engine implementations may still use their own
// technical bootstrap URL (for example, about:blank) before navigating here.
func DefaultBrowserStartupURL() string {
	return defaultBrowserStartupURL
}

// ResolveBrowserStartupURL returns the explicit URL when provided, otherwise
// the global user-facing browser startup URL.
func ResolveBrowserStartupURL(rawURL string) string {
	if rawURL != "" {
		return rawURL
	}
	return defaultBrowserStartupURL
}

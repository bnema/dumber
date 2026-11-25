package url

import (
	neturl "net/url"
	"strings"
)

// systemviewPages maps every dumb:// page host, aliases included, to its
// canonical page.
var systemviewPages = map[string]string{
	"home":      "home",
	"history":   "history",
	"favorites": "favorites",
	"config":    "config",
	"settings":  "config",
}

// systemviewTitles are the stable titles of dumb:// pages, used for their
// history entries (the live page title carries changing counts).
var systemviewTitles = map[string]string{
	"home":      "Home",
	"history":   "History",
	"favorites": "Favorites",
	"config":    "Settings",
}

// SystemviewTitle returns the stable title of the dumb:// page at raw.
func SystemviewTitle(raw string) (string, bool) {
	if !IsSystemviewPageURL(raw) {
		return "", false
	}
	parsed, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	page, _ := SystemviewPage(parsed.Host)
	title, ok := systemviewTitles[page]
	return title, ok
}

// SystemviewPage returns the canonical page for a dumb:// host, resolving
// aliases ("settings" is "config"). ok is false for unknown hosts.
func SystemviewPage(host string) (page string, ok bool) {
	page, ok = systemviewPages[strings.ToLower(strings.TrimSpace(host))]
	return page, ok
}

// IsSystemviewPageURL reports whether raw is the root of a user-facing
// dumb:// page (dumb://history, dumb://config…). Sub-paths such as the
// crash page and internal APIs are not pages.
func IsSystemviewPageURL(raw string) bool {
	parsed, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "dumb") {
		return false
	}
	if _, ok := SystemviewPage(parsed.Host); !ok {
		return false
	}
	return strings.Trim(parsed.Path, "/") == ""
}

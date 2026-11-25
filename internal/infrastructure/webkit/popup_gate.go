package webkit

import (
	"net/url"
	"strings"
)

// internalHost is the CEF internal origin host; on WebKit it is never a real
// site, so popups to it are refused like dumb: ones.
const internalHost = "dumber.invalid"

// blockSystemViewPopup reports whether a popup to target must be refused.
// WebKit's create signal does not say which frame asked for the popup, so an
// iframe embedded in dumb://home looks the same as the page itself. No system
// view opens other system views in popups, so every popup to one is refused;
// the user still opens them from the omnibox, keyboard or CLI.
func blockSystemViewPopup(target string) bool {
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "dumb":
		return true
	case "https", "http":
		return strings.EqualFold(u.Hostname(), internalHost)
	default:
		return false
	}
}

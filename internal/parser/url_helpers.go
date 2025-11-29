package parser

import (
	neturl "net/url"
	"strings"
)

// NormalizeHistoryURL ensures history URLs have a canonical form so lookups match.
// Currently it only guarantees that http/https URLs always include a trailing slash
// for root paths, matching how WebKit reports final navigation URLs.
func NormalizeHistoryURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}

	u, err := neturl.Parse(trimmed)
	if err != nil {
		return trimmed
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return trimmed
	}

	if u.Path == "" {
		u.Path = "/"
	}

	return u.String()
}

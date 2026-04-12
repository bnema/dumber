package systemviews

import (
	"net/url"
	"strings"
)

func sanitizeHref(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "#"
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "#"
	}

	if parsed.Scheme == "" {
		if parsed.Host != "" {
			return "#"
		}
		return trimmed
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "dumb":
		return trimmed
	default:
		return "#"
	}
}

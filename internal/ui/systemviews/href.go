package systemviews

import (
	"net/url"
	"strings"
)

const httpsScheme = "https"

func sanitizeHref(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") {
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
	case httpScheme, httpsScheme:
		if parsed.Host == "" {
			return "#"
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.User = nil
		return parsed.String()
	case "dumb":
		if parsed.Host == "" && parsed.Opaque == "" {
			return "#"
		}
		return trimmed
	default:
		return "#"
	}
}

package cef

import (
	"net/url"
	"path"
	"strings"
)

const (
	conceptualInternalSchemePrefix = "dumb://"
	actualInternalScheme           = "https"
	actualInternalHost             = "dumber.invalid"
	actualInternalOrigin           = actualInternalScheme + "://" + actualInternalHost
	internalAPIPathPrefix          = "/api/"
	internalAPIPathPrefixTrimmed   = "api/"
)

func isConceptualInternalURL(raw string) bool {
	return strings.HasPrefix(raw, conceptualInternalSchemePrefix) || strings.HasPrefix(raw, "dumb:")
}

func isActualInternalURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, actualInternalScheme) && strings.EqualFold(parsed.Host, actualInternalHost)
}

func toActualInternalURL(raw string) string {
	if raw == "" || isActualInternalURL(raw) {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "dumb") {
		return raw
	}

	page := parsed.Host
	if page == "" {
		page = parsed.Opaque
	}
	if page == "" || !isInternalPageHost(page) {
		return raw
	}

	actualPath := "/" + page
	trimmedPath := strings.Trim(parsed.Path, "/")
	switch {
	case trimmedPath == "":
		// Keep page roots at /history instead of /history/ so relative assets
		// resolve from the origin root.
	case strings.HasPrefix(trimmedPath, internalAPIPathPrefixTrimmed):
		actualPath = "/" + trimmedPath
	case path.Ext(trimmedPath) != "":
		actualPath = "/" + trimmedPath
	default:
		actualPath = "/" + page + "/" + trimmedPath
	}

	actual := url.URL{
		Scheme:   actualInternalScheme,
		Host:     actualInternalHost,
		Path:     actualPath,
		RawQuery: parsed.RawQuery,
		Fragment: parsed.Fragment,
	}
	return actual.String()
}

func resolveAPIPath(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}

	if strings.EqualFold(u.Scheme, actualInternalScheme) && strings.EqualFold(u.Host, actualInternalHost) {
		if strings.HasPrefix(u.Path, internalAPIPathPrefix) {
			return u.Path, true
		}
		return "", false
	}

	if !strings.EqualFold(u.Scheme, "dumb") {
		return "", false
	}

	if strings.EqualFold(u.Host, "api") {
		if u.Path == "" {
			return "/api", true
		}
		return internalAPIPathPrefix[:len(internalAPIPathPrefix)-1] + u.Path, true
	}

	if strings.HasPrefix(u.Path, internalAPIPathPrefix) {
		return u.Path, true
	}

	return "", false
}

func toConceptualInternalURL(raw string) string {
	if raw == "" || !isActualInternalURL(raw) {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	trimmedPath := strings.Trim(parsed.Path, "/")
	if trimmedPath == "" {
		return raw
	}

	parts := strings.SplitN(trimmedPath, "/", 2)
	page := parts[0]
	if !isInternalPageHost(page) {
		return raw
	}

	conceptual := url.URL{
		Scheme:   "dumb",
		Host:     page,
		RawQuery: parsed.RawQuery,
		Fragment: parsed.Fragment,
	}
	if len(parts) == 2 && parts[1] != "" {
		conceptual.Path = "/" + parts[1]
	}
	return conceptual.String()
}

func isInternalPageHost(host string) bool {
	switch host {
	case historyPath, favoritesPath, configPath, errorPath:
		return true
	default:
		return false
	}
}

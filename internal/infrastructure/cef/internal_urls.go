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
	if page == "" {
		return raw
	}

	actualPath := "/" + page
	trimmedPath := strings.Trim(parsed.Path, "/")
	switch {
	case trimmedPath == "":
		// Keep page roots at /home instead of /home/ so relative assets resolve
		// from the origin root.
	case strings.HasPrefix(trimmedPath, "api/"):
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
	case homePath, configPath, webrtcPath, errorPath:
		return true
	default:
		return false
	}
}

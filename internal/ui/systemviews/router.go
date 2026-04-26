package systemviews

import (
	"net/url"
	"strings"
)

type Route string

const (
	RouteUnknown   Route = "unknown"
	RouteHistory   Route = "history"
	RouteFavorites Route = "favorites"
	RouteConfig    Route = "config"
)

func ParseRoute(uri string) Route {
	if uri == "" {
		return RouteUnknown
	}

	u, err := url.Parse(uri)
	if err != nil {
		return RouteUnknown
	}

	switch {
	case strings.EqualFold(u.Scheme, "dumb"):
		return routeFromPageHost(u.Host, u.Opaque)
	case strings.EqualFold(u.Scheme, "https") && strings.EqualFold(u.Host, "dumber.invalid"):
		path := strings.Trim(u.Path, "/")
		if path == "" {
			return RouteUnknown
		}
		if head, _, ok := strings.Cut(path, "/"); ok {
			path = head
		}
		return routeFromPageHost(path, "")
	default:
		return RouteUnknown
	}
}

func routeFromPageHost(host, opaque string) Route {
	page := strings.TrimSpace(host)
	if page == "" {
		page = strings.TrimSpace(opaque)
	}

	switch strings.ToLower(page) {
	case string(RouteHistory):
		return RouteHistory
	case string(RouteFavorites):
		return RouteFavorites
	case string(RouteConfig):
		return RouteConfig
	default:
		return RouteUnknown
	}
}

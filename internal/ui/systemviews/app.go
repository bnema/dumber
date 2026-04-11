package systemviews

import "errors"

type Dependencies struct {
	DOM         DOM
	LocationURI string
}

type App struct {
	deps         Dependencies
	currentRoute Route
}

func NewApp(deps Dependencies) *App {
	return &App{deps: deps}
}

func (a *App) Run() error {
	if a == nil {
		return errors.New("app is nil")
	}

	a.currentRoute = ParseRoute(a.deps.LocationURI)
	if a.deps.DOM == nil {
		return errors.New("DOM not configured")
	}

	return a.deps.DOM.Mount(placeholderHTML(a.currentRoute))
}

func (a *App) CurrentRoute() Route {
	if a == nil {
		return RouteUnknown
	}
	return a.currentRoute
}

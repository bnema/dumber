package systemviews

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

type Dependencies struct {
	DOM         DOM
	History     port.SystemviewHistoryService
	LocationURI string
}

type App struct {
	deps           Dependencies
	currentRoute   Route
	historyEntries []*entity.HistoryEntry
	renderedHTML   string
}

func NewApp(deps Dependencies) *App {
	return &App{deps: deps, currentRoute: RouteUnknown}
}

func (a *App) Run() error {
	if a == nil {
		return errors.New("app is nil")
	}

	a.currentRoute = ParseRoute(a.deps.LocationURI)
	if err := a.LoadInitial(context.Background()); err != nil {
		return err
	}
	if a.deps.DOM == nil {
		return errors.New("DOM not configured")
	}

	return a.deps.DOM.Mount(a.renderedHTML)
}

func (a *App) LoadInitial(ctx context.Context) error {
	if a == nil {
		return errors.New("app is nil")
	}
	if a.currentRoute == "" || a.currentRoute == RouteUnknown {
		a.currentRoute = ParseRoute(a.deps.LocationURI)
	}

	switch a.currentRoute {
	case RouteHistory:
		if a.deps.History == nil {
			a.historyEntries = nil
			a.renderedHTML = placeholderHTML(a.currentRoute)
			return nil
		}

		entries, err := a.deps.History.Timeline(ctx, 25, 0)
		if err != nil {
			return err
		}
		a.historyEntries = entries
		a.renderedHTML = historyHTML(entries)
		return nil
	default:
		a.historyEntries = nil
		a.renderedHTML = placeholderHTML(a.currentRoute)
		return nil
	}
}

func (a *App) CurrentRoute() Route {
	if a == nil {
		return RouteUnknown
	}
	return a.currentRoute
}

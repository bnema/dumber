package systemviews

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

type Dependencies struct {
	DOM         DOM
	Favorites   port.SystemviewFavoritesService
	History     port.SystemviewHistoryService
	Config      port.SystemviewConfigService
	LocationURI string
}

type App struct {
	deps           Dependencies
	currentRoute   Route
	shellTheme     shellTheme
	historyEntries []*entity.HistoryEntry
	favorites      []*entity.Favorite
	folders        []*entity.Folder
	tags           []*entity.Tag
	config         *port.SystemviewConfigPayload
	keybindings    any
	renderedHTML   string
}

const historyTimelineLimit = 25

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
	a.loadShellTheme(ctx)
	if a.currentRoute == "" || a.currentRoute == RouteUnknown {
		a.currentRoute = ParseRoute(a.deps.LocationURI)
	}

	var err error
	switch a.currentRoute {
	case RouteHistory:
		err = a.loadHistoryRoute(ctx)
	case RouteFavorites:
		err = a.loadFavoritesRoute(ctx)
	case RouteConfig:
		err = a.loadConfigRoute(ctx)
	default:
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			subtitle: string(a.currentRoute),
			body:     placeholderHTML(a.currentRoute),
		}, a.shellTheme)
		return nil
	}
	if err != nil {
		a.renderRouteError(err)
	}
	return nil
}

func (a *App) renderRouteError(err error) {
	a.resetRouteState()
	message := "unknown error"
	if err != nil {
		message = err.Error()
	}
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    a.currentRoute,
		subtitle: routeSubtitle(a.currentRoute),
		body:     errorStateHTML(message),
	}, a.shellTheme)
}

func routeSubtitle(route Route) string {
	switch route {
	case RouteHistory:
		return "Recent visits"
	case RouteFavorites:
		return "Saved bookmarks"
	case RouteConfig:
		return "Browser settings"
	default:
		return string(route)
	}
}

func (a *App) loadHistoryRoute(ctx context.Context) error {
	if a.deps.History == nil {
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			subtitle: "Recent visits",
			body:     placeholderHTML(a.currentRoute),
		}, a.shellTheme)
		return nil
	}

	entries, err := a.deps.History.Timeline(ctx, historyTimelineLimit, 0)
	if err != nil {
		return err
	}
	a.favorites = nil
	a.folders = nil
	a.tags = nil
	a.historyEntries = entries
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    RouteHistory,
		subtitle: "Recent visits",
		body:     historyHTML(entries),
	}, a.shellTheme)
	return nil
}

func (a *App) loadFavoritesRoute(ctx context.Context) error {
	if a.deps.Favorites == nil {
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			subtitle: "Saved bookmarks",
			body:     placeholderHTML(a.currentRoute),
		}, a.shellTheme)
		return nil
	}

	favorites, err := a.deps.Favorites.List(ctx)
	if err != nil {
		return err
	}
	folders, err := a.deps.Favorites.ListFolders(ctx)
	if err != nil {
		return err
	}
	tags, err := a.deps.Favorites.ListTags(ctx)
	if err != nil {
		return err
	}

	a.historyEntries = nil
	a.favorites = favorites
	a.folders = folders
	a.tags = tags
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    RouteFavorites,
		subtitle: "Saved bookmarks",
		body:     favoritesHTML(favorites, folders, tags),
	}, a.shellTheme)
	return nil
}

func (a *App) loadConfigRoute(ctx context.Context) error {
	if a.deps.Config == nil {
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			subtitle: "Browser settings",
			body:     placeholderHTML(a.currentRoute),
		}, a.shellTheme)
		return nil
	}

	config, err := a.deps.Config.Current(ctx)
	if err != nil {
		return err
	}
	keybindings, err := a.deps.Config.GetKeybindings(ctx)
	if err != nil {
		return err
	}

	a.historyEntries = nil
	a.favorites = nil
	a.folders = nil
	a.tags = nil
	a.config = &config
	a.keybindings = keybindings
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    RouteConfig,
		subtitle: "Browser settings",
		body:     configHTML(config, keybindings),
	}, a.shellTheme)
	return nil
}

func (a *App) loadShellTheme(ctx context.Context) {
	if a == nil {
		return
	}

	a.shellTheme = shellTheme{}
	if a.deps.Config == nil {
		return
	}

	config, err := a.deps.Config.Current(ctx)
	if err == nil {
		a.shellTheme = resolveShellTheme(config.Appearance)
	}
}

func (a *App) resetRouteState() {
	a.historyEntries = nil
	a.favorites = nil
	a.folders = nil
	a.tags = nil
	a.config = nil
	a.keybindings = nil
}

func (a *App) CurrentRoute() Route {
	if a == nil {
		return RouteUnknown
	}
	return a.currentRoute
}

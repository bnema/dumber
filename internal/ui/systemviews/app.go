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
	historyEntries []*entity.HistoryEntry
	favorites      []*entity.Favorite
	folders        []*entity.Folder
	tags           []*entity.Tag
	config         *port.SystemviewConfigPayload
	keybindings    any
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
			a.favorites = nil
			a.folders = nil
			a.tags = nil
			a.renderedHTML = placeholderHTML(a.currentRoute)
			return nil
		}

		entries, err := a.deps.History.Timeline(ctx, 25, 0)
		if err != nil {
			return err
		}
		a.favorites = nil
		a.folders = nil
		a.tags = nil
		a.historyEntries = entries
		a.renderedHTML = historyHTML(entries)
		return nil
	case RouteFavorites:
		if a.deps.Favorites == nil {
			a.historyEntries = nil
			a.favorites = nil
			a.folders = nil
			a.tags = nil
			a.renderedHTML = placeholderHTML(a.currentRoute)
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
		a.renderedHTML = favoritesHTML(favorites, folders, tags)
		return nil
	case RouteConfig:
		if a.deps.Config == nil {
			a.historyEntries = nil
			a.favorites = nil
			a.folders = nil
			a.tags = nil
			a.config = nil
			a.keybindings = nil
			a.renderedHTML = placeholderHTML(a.currentRoute)
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
		a.renderedHTML = configHTML(config, keybindings)
		return nil
	default:
		a.historyEntries = nil
		a.favorites = nil
		a.folders = nil
		a.tags = nil
		a.config = nil
		a.keybindings = nil
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

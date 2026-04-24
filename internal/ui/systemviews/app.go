package systemviews

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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
	mu                   sync.Mutex
	deps                 Dependencies
	currentRoute         Route
	shellTheme           shellTheme
	historyEntries       []*entity.HistoryEntry
	historyAnalytics     *entity.HistoryAnalytics
	historyDomainStats   []*entity.DomainStat
	historyQuery         string
	historyDomainFilter  string
	historyOffset        int
	historyNotice        string
	historyError         string
	favorites            []*entity.Favorite
	folders              []*entity.Folder
	tags                 []*entity.Tag
	favoriteFolderFilter *entity.FolderID
	favoriteTagFilter    *entity.TagID
	favoritesNotice      string
	favoritesError       string
	config               *port.SystemviewConfigPayload
	keybindings          port.KeybindingsConfig
	configNotice         string
	configError          string
	renderedHTML         string
	actionQueue          chan DOMAction
	actionCtx            context.Context
	actionCancel         context.CancelFunc
}

const historyTimelineLimit = 25

func NewApp(deps Dependencies) *App {
	return &App{deps: deps, currentRoute: RouteUnknown}
}

func (a *App) Run() error {
	return a.RunWithContext(context.Background())
}

func (a *App) RunWithContext(ctx context.Context) error {
	if a == nil {
		return errors.New("app is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	a.currentRoute = ParseRoute(a.deps.LocationURI)
	if err := a.LoadInitial(ctx); err != nil {
		return err
	}
	if a.deps.DOM == nil {
		return errors.New("DOM not configured")
	}

	if err := a.deps.DOM.Mount(a.renderedHTML); err != nil {
		return err
	}
	if binder, ok := a.deps.DOM.(DOMActionBinder); ok {
		return a.bindDOMActions(ctx, binder)
	}
	return nil
}

func (a *App) bindDOMActions(ctx context.Context, binder DOMActionBinder) error {
	if a.actionQueue == nil {
		workerCtx, cancel := context.WithCancel(ctx)
		a.actionQueue = make(chan DOMAction, 64)
		a.actionCtx = workerCtx
		a.actionCancel = cancel
		go a.runActionWorker(workerCtx)
	}
	return binder.BindActions(func(action DOMAction) {
		if !a.enqueueDOMAction(action) {
			go a.surfaceActionError(fmt.Errorf("systemview is busy; dropped action %q", action.Action))
		}
	})
}

func (a *App) enqueueDOMAction(action DOMAction) bool {
	if a == nil || a.actionQueue == nil {
		return false
	}
	if a.actionCtx == nil {
		return false
	}
	select {
	case <-a.actionCtx.Done():
		return false
	case a.actionQueue <- action:
		return true
	default:
		return false
	}
}

func (a *App) runActionWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case action, ok := <-a.actionQueue:
			if !ok {
				return
			}
			if err := a.HandleDOMAction(ctx, action); err != nil {
				a.surfaceActionError(err)
			}
		}
	}
}

func (a *App) surfaceActionError(err error) {
	if a == nil || err == nil {
		return
	}
	a.mu.Lock()
	a.renderRouteError(err)
	a.mu.Unlock()
	_ = a.mountRenderedHTML()
}

func (a *App) Close() {
	if a == nil {
		return
	}
	if a.actionCancel != nil {
		a.actionCancel()
		a.actionCancel = nil
	}
	a.actionCtx = nil
	a.actionQueue = nil
	if releaser, ok := a.deps.DOM.(interface{ Release() }); ok {
		releaser.Release()
	}
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

	entries, err := a.loadHistoryEntries(ctx)
	if err != nil {
		return err
	}
	analytics, analyticsErr := a.deps.History.Analytics(ctx)
	if analyticsErr != nil {
		analytics = nil
	}
	domains, domainsErr := a.deps.History.DomainStats(ctx, 10)
	if domainsErr != nil {
		domains = nil
	}
	a.favorites = nil
	a.folders = nil
	a.tags = nil
	a.historyEntries = entries
	a.historyAnalytics = analytics
	a.historyDomainStats = domains
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    RouteHistory,
		subtitle: "Recent visits",
		body: historyHTML(historyRenderData{
			Entries:      entries,
			Analytics:    analytics,
			Domains:      domains,
			Query:        a.historyQuery,
			DomainFilter: a.historyDomainFilter,
			Offset:       a.historyOffset,
			Limit:        historyTimelineLimit,
			Notice:       a.historyNotice,
			Error:        a.historyError,
		}),
	}, a.shellTheme)
	return nil
}

func (a *App) loadHistoryEntries(ctx context.Context) ([]*entity.HistoryEntry, error) {
	query := strings.TrimSpace(a.historyQuery)
	domain := strings.TrimSpace(a.historyDomainFilter)
	if query != "" {
		entries, err := a.deps.History.Search(ctx, query, historyTimelineLimit)
		if err != nil || domain == "" {
			return entries, err
		}
		return filterHistoryEntriesByDomain(entries, domain), nil
	}
	if domain != "" {
		return a.deps.History.TimelineByDomain(ctx, domain, historyTimelineLimit, a.historyOffset)
	}
	return a.deps.History.Timeline(ctx, historyTimelineLimit, a.historyOffset)
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
	favorites = filterFavorites(favorites, a.favoriteFolderFilter, a.favoriteTagFilter)
	a.favorites = favorites
	a.folders = folders
	a.tags = tags
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    RouteFavorites,
		subtitle: "Saved bookmarks",
		body: favoritesHTML(favoritesRenderData{
			Favorites:    favorites,
			Folders:      folders,
			Tags:         tags,
			FolderFilter: a.favoriteFolderFilter,
			TagFilter:    a.favoriteTagFilter,
			Notice:       a.favoritesNotice,
			Error:        a.favoritesError,
		}),
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
		body: configHTML(configRenderData{
			Config:      config,
			Keybindings: keybindings,
			Notice:      a.configNotice,
			Error:       a.configError,
		}),
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
	a.historyAnalytics = nil
	a.historyDomainStats = nil
	a.historyQuery = ""
	a.historyDomainFilter = ""
	a.historyOffset = 0
	a.historyNotice = ""
	a.historyError = ""
	a.favorites = nil
	a.folders = nil
	a.tags = nil
	a.favoriteFolderFilter = nil
	a.favoriteTagFilter = nil
	a.favoritesNotice = ""
	a.favoritesError = ""
	a.config = nil
	a.keybindings = port.KeybindingsConfig{}
	a.configNotice = ""
	a.configError = ""
}

func (a *App) CurrentRoute() Route {
	if a == nil {
		return RouteUnknown
	}
	return a.currentRoute
}

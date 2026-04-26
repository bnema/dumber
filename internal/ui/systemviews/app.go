package systemviews

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
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
	// mu protects rendered route state; actionMu protects worker channels.
	// The two locks must not be held at the same time.
	mu                   sync.Mutex
	actionMu             sync.Mutex
	deps                 Dependencies
	currentRoute         Route
	shellTheme           shellTheme
	historyEntries       []*entity.HistoryEntry
	historyStats         *entity.HistoryStats
	historyAnalytics     *entity.HistoryAnalytics
	historyDomainStats   []*entity.DomainStat
	historyQuery         string
	historyDomainFilter  string
	historyOffset        int
	historyWindowBefore  time.Time
	historyWindowAfter   time.Time
	historyHasMore       bool
	historyNotice        string
	historyError         string
	favorites            []*entity.Favorite
	folders              []*entity.Folder
	tags                 []*entity.Tag
	favoriteFolderFilter *entity.FolderID
	favoriteTagFilter    *entity.TagID
	favoritesNotice      string
	favoritesError       string
	config               *dto.SystemviewConfigPayload
	keybindings          port.KeybindingsConfig
	configNotice         string
	configError          string
	renderedHTML         string
	renderGeneration     uint64
	closed               bool
	actionQueue          chan DOMAction
	actionErrorQueue     chan error
	actionCtx            context.Context
	actionCancel         context.CancelFunc
	actionClosed         bool
	actionWG             sync.WaitGroup
}

const (
	historyTimelineLimit = 0
	historySearchLimit   = 100
)

func (a *App) lockAction() {
	// Lock order: actionMu is never acquired while App.mu is held.
	a.actionMu.Lock()
}

func (a *App) unlockAction() {
	a.actionMu.Unlock()
}

func (a *App) lockState() {
	// Lock order: App.mu is never held while acquiring actionMu.
	a.mu.Lock()
}

func (a *App) unlockState() {
	a.mu.Unlock()
}

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
		return errors.New("context is nil")
	}

	a.currentRoute = ParseRoute(a.deps.LocationURI)
	if a.deps.DOM == nil {
		return errors.New("DOM not configured")
	}
	asyncInitial := a.shouldLoadInitialRouteAsync()
	if asyncInitial {
		a.LoadInitialShell(ctx)
	} else if err := a.LoadInitial(ctx); err != nil {
		return err
	}

	if err := a.deps.DOM.Mount(a.renderedHTML); err != nil {
		return err
	}
	if binder, ok := a.deps.DOM.(DOMActionBinder); ok {
		if err := a.bindDOMActions(ctx, binder); err != nil {
			return err
		}
	}
	if asyncInitial {
		a.refreshCurrentRouteAsync(ctx)
	}
	return nil
}

func (a *App) shouldLoadInitialRouteAsync() bool {
	return a != nil && a.currentRoute == RouteHistory && a.deps.History != nil
}

func (a *App) refreshCurrentRouteAsync(ctx context.Context) {
	if a == nil {
		return
	}
	a.lockState()
	snapshot := a.currentHistoryRouteSnapshotLocked()
	generation := a.renderGeneration
	closed := a.closed
	a.unlockState()
	if closed || snapshot.route != RouteHistory {
		return
	}

	a.lockAction()
	asyncCtx := a.actionCtx
	if asyncCtx == nil {
		asyncCtx = ctx
	}
	ready := !a.actionClosed && asyncCtx != nil && asyncCtx.Err() == nil
	if ready {
		a.actionWG.Add(1)
	}
	a.unlockAction()
	if !ready {
		return
	}

	go func() {
		defer a.actionWG.Done()
		if asyncCtx.Err() != nil {
			return
		}
		result, err := a.renderHistoryRouteSnapshot(asyncCtx, snapshot)

		a.lockState()
		if a.closed || asyncCtx.Err() != nil || a.currentRoute != snapshot.route || a.renderGeneration != generation {
			a.unlockState()
			return
		}
		if err != nil {
			a.renderRouteError(err)
		} else {
			a.commitHistoryRouteResultLocked(result)
		}
		a.renderGeneration++
		mountGeneration := a.renderGeneration
		html := a.renderedHTML
		a.unlockState()

		if err := a.mountHTMLIfCurrent(asyncCtx, html, mountGeneration); err != nil {
			logActionMountError(asyncCtx, err, nil)
		}
	}()
}

func (a *App) bindDOMActions(ctx context.Context, binder DOMActionBinder) error {
	var (
		created    bool
		workerCtx  context.Context
		cancel     context.CancelFunc
		queue      chan DOMAction
		errorQueue chan error
	)

	a.lockAction()
	if a.actionClosed {
		a.unlockAction()
		return errors.New("systemview app is closed")
	}
	if a.actionQueue == nil {
		workerCtx, cancel = context.WithCancel(ctx)
		queue = make(chan DOMAction, 64)
		errorQueue = make(chan error, 1)
		a.actionQueue = queue
		a.actionErrorQueue = errorQueue
		a.actionCtx = workerCtx
		a.actionCancel = cancel
		created = true
	}
	a.unlockAction()

	if err := binder.BindActions(func(action DOMAction) {
		if !a.enqueueDOMAction(action) && a.actionWorkerActive() {
			a.enqueueActionError(fmt.Errorf("systemview is busy; dropped action %q", action.Action))
		}
	}); err != nil {
		if created {
			a.discardPendingActionWorker(workerCtx, cancel, queue, errorQueue)
		}
		return err
	}

	if created && !a.startActionWorkers(workerCtx, queue, errorQueue) {
		a.discardPendingActionWorker(workerCtx, cancel, queue, errorQueue)
		a.releaseDOMBindings()
		return errors.New("systemview action worker unavailable")
	}
	return nil
}

func (a *App) startActionWorkers(ctx context.Context, queue chan DOMAction, errorQueue chan error) bool {
	if a == nil || ctx == nil || queue == nil || errorQueue == nil {
		return false
	}
	a.lockAction()
	ready := !a.actionClosed && a.actionCtx == ctx && a.actionQueue == queue && a.actionErrorQueue == errorQueue && ctx.Err() == nil
	if ready {
		a.actionWG.Add(2)
	}
	a.unlockAction()
	if !ready {
		return false
	}
	go func() {
		defer a.actionWG.Done()
		a.runActionWorker(ctx, queue)
	}()
	go func() {
		defer a.actionWG.Done()
		a.runActionErrorWorker(ctx, errorQueue)
	}()
	return true
}

func (a *App) discardPendingActionWorker(ctx context.Context, cancel context.CancelFunc, queue chan DOMAction, errorQueue chan error) {
	if a == nil {
		return
	}
	a.lockAction()
	if a.actionCtx == ctx && a.actionQueue == queue {
		a.actionCancel = nil
		a.actionCtx = nil
		a.actionQueue = nil
		a.actionErrorQueue = nil
		if cancel != nil {
			cancel()
		}
		close(queue)
		close(errorQueue)
	}
	a.unlockAction()
}

func (a *App) actionWorkerActive() bool {
	if a == nil {
		return false
	}
	a.lockAction()
	closed := a.actionClosed
	queue := a.actionQueue
	ctx := a.actionCtx
	a.unlockAction()
	return !closed && queue != nil && ctx != nil && ctx.Err() == nil
}

func (a *App) enqueueDOMAction(action DOMAction) bool {
	if a == nil {
		return false
	}
	a.lockAction()
	defer a.unlockAction()
	queue := a.actionQueue
	ctx := a.actionCtx
	if a.actionClosed || queue == nil || ctx == nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case queue <- action:
		return true
	default:
		return false
	}
}

func (a *App) enqueueActionError(err error) {
	if a == nil || err == nil {
		return
	}
	a.lockAction()
	defer a.unlockAction()
	queue := a.actionErrorQueue
	ctx := a.actionCtx
	if a.actionClosed || queue == nil || ctx == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case queue <- err:
	default:
	}
}

func (a *App) runActionWorker(ctx context.Context, queue <-chan DOMAction) {
	for {
		select {
		case <-ctx.Done():
			return
		case action, ok := <-queue:
			if !ok || ctx.Err() != nil {
				return
			}
			if err := a.HandleDOMAction(ctx, action); err != nil {
				a.enqueueActionError(err)
			}
		}
	}
}

func (a *App) runActionErrorWorker(ctx context.Context, queue <-chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-queue:
			if !ok || ctx.Err() != nil {
				return
			}
			a.surfaceActionError(ctx, err)
		}
	}
}

func (a *App) surfaceActionError(ctx context.Context, err error) {
	if a == nil || err == nil {
		return
	}
	a.lockState()
	a.renderRouteError(err)
	a.renderGeneration++
	generation := a.renderGeneration
	html := a.renderedHTML
	a.unlockState()
	if mountErr := a.mountHTMLIfCurrent(ctx, html, generation); mountErr != nil {
		logActionMountError(ctx, mountErr, err)
	}
}

func (a *App) Close() {
	if a == nil {
		return
	}
	a.lockState()
	a.closed = true
	a.renderGeneration++
	a.unlockState()

	a.lockAction()
	cancel := a.actionCancel
	actionQueue := a.actionQueue
	actionErrorQueue := a.actionErrorQueue
	a.actionClosed = true
	a.actionCancel = nil
	a.actionCtx = nil
	a.actionQueue = nil
	a.actionErrorQueue = nil
	a.unlockAction()

	if cancel != nil {
		cancel()
	}
	a.actionWG.Wait()
	if actionQueue != nil {
		close(actionQueue)
	}
	if actionErrorQueue != nil {
		close(actionErrorQueue)
	}
	a.releaseDOMBindings()
}

func (a *App) releaseDOMBindings() {
	if a == nil {
		return
	}
	if releaser, ok := a.deps.DOM.(interface{ Release() }); ok {
		releaser.Release()
	}
}

func (a *App) LoadInitialShell(ctx context.Context) {
	if a == nil {
		return
	}
	a.loadShellTheme(ctx)
	if a.currentRoute == "" || a.currentRoute == RouteUnknown {
		a.currentRoute = ParseRoute(a.deps.LocationURI)
	}
	switch a.currentRoute {
	case RouteHistory:
		a.loadHistoryLoadingRoute()
	default:
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			title:    routeDocumentTitle(a.currentRoute),
			subtitle: routeSubtitle(a.currentRoute),
			body:     placeholderHTML(a.currentRoute),
		}, a.shellTheme)
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

	if err := a.loadCurrentRoute(ctx); err != nil {
		a.renderRouteError(err)
	}
	return nil
}

func (a *App) loadCurrentRoute(ctx context.Context) error {
	switch a.currentRoute {
	case RouteHistory:
		return a.loadHistoryRoute(ctx)
	case RouteFavorites:
		return a.loadFavoritesRoute(ctx)
	case RouteConfig:
		return a.loadConfigRoute(ctx)
	default:
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			title:    routeDocumentTitle(a.currentRoute),
			subtitle: string(a.currentRoute),
			body:     placeholderHTML(a.currentRoute),
		}, a.shellTheme)
		return nil
	}
}

func (a *App) renderRouteError(err error) {
	a.resetRouteState()
	message := "unknown error"
	if err != nil {
		message = err.Error()
	}
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    a.currentRoute,
		title:    routeDocumentTitle(a.currentRoute) + " — Error",
		subtitle: routeSubtitle(a.currentRoute),
		body:     errorStateHTML(message),
	}, a.shellTheme)
}

func routeDocumentTitle(route Route) string {
	switch route {
	case RouteHistory:
		return "History"
	case RouteFavorites:
		return "Favorites"
	case RouteConfig:
		return "Config"
	default:
		return "Dumber System View"
	}
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

func (a *App) loadHistoryLoadingRoute() {
	a.favorites = nil
	a.folders = nil
	a.tags = nil
	a.historyEntries = nil
	a.historyStats = nil
	a.historyAnalytics = nil
	a.historyDomainStats = nil
	a.historyNotice = ""
	a.historyError = ""
	data := historyRenderData{
		Query:        a.historyQuery,
		DomainFilter: a.historyDomainFilter,
		Offset:       a.historyOffset,
		Limit:        historyTimelineLimit,
		WindowBefore: a.historyWindowBefore,
		WindowAfter:  a.historyWindowAfter,
		HasMore:      a.historyHasMore,
		Notice:       a.historyNotice,
		Error:        a.historyError,
		Loading:      true,
	}
	title := historyDocumentTitle(data)
	a.renderedHTML = renderAppFrame(renderedPage{
		route:          RouteHistory,
		title:          title,
		subtitle:       "History",
		subtitleDetail: historyTitleDetail(data),
		body:           historyHTML(data),
	}, a.shellTheme)
}

func (a *App) loadHistoryRoute(ctx context.Context) error {
	if a.deps.History == nil {
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			title:    routeDocumentTitle(RouteHistory),
			subtitle: routeDocumentTitle(RouteHistory),
			body:     placeholderHTML(a.currentRoute),
		}, a.shellTheme)
		return nil
	}

	result, err := a.renderHistoryRouteSnapshot(ctx, a.currentHistoryRouteSnapshotLocked())
	if err != nil {
		return err
	}
	a.commitHistoryRouteResultLocked(result)
	return nil
}

type historyRouteSnapshot struct {
	route        Route
	history      port.SystemviewHistoryService
	shellTheme   shellTheme
	query        string
	domainFilter string
	offset       int
	windowBefore time.Time
	windowAfter  time.Time
	notice       string
	error        string
}

type historyRouteResult struct {
	entries      []*entity.HistoryEntry
	stats        *entity.HistoryStats
	domains      []*entity.DomainStat
	windowBefore time.Time
	windowAfter  time.Time
	hasMore      bool
	html         string
}

func (a *App) currentHistoryRouteSnapshotLocked() historyRouteSnapshot {
	return historyRouteSnapshot{
		route:        a.currentRoute,
		history:      a.deps.History,
		shellTheme:   a.shellTheme,
		query:        a.historyQuery,
		domainFilter: a.historyDomainFilter,
		offset:       a.historyOffset,
		windowBefore: a.historyWindowBefore,
		windowAfter:  a.historyWindowAfter,
		notice:       a.historyNotice,
		error:        a.historyError,
	}
}

func (a *App) renderHistoryRouteSnapshot(ctx context.Context, snapshot historyRouteSnapshot) (historyRouteResult, error) {
	if snapshot.history == nil {
		return historyRouteResult{
			html: renderAppFrame(renderedPage{
				route:    snapshot.route,
				title:    routeDocumentTitle(RouteHistory),
				subtitle: routeDocumentTitle(RouteHistory),
				body:     placeholderHTML(snapshot.route),
			}, snapshot.shellTheme),
		}, nil
	}

	window, entries, err := loadHistoryWindowSnapshot(ctx, snapshot)
	if err != nil {
		return historyRouteResult{}, err
	}
	stats, err := snapshot.history.Stats(ctx)
	if err != nil {
		return historyRouteResult{}, fmt.Errorf("failed to load history stats: %w", err)
	}
	domains, domainsErr := snapshot.history.DomainStats(ctx, 10)
	if domainsErr != nil {
		domains = nil
	}
	result := historyRouteResult{
		entries:      entries,
		stats:        stats,
		domains:      domains,
		windowBefore: snapshot.windowBefore,
		windowAfter:  snapshot.windowAfter,
	}
	if window != nil {
		result.windowBefore = window.Before
		result.windowAfter = window.After
		result.hasMore = window.HasMore
	}
	data := historyRenderData{
		Entries:      entries,
		Stats:        stats,
		Analytics:    nil,
		Domains:      domains,
		Query:        snapshot.query,
		DomainFilter: snapshot.domainFilter,
		Offset:       snapshot.offset,
		Limit:        historyTimelineLimit,
		WindowBefore: result.windowBefore,
		WindowAfter:  result.windowAfter,
		HasMore:      result.hasMore,
		Notice:       snapshot.notice,
		Error:        snapshot.error,
	}
	title := historyDocumentTitle(data)
	result.html = renderAppFrame(renderedPage{
		route:          RouteHistory,
		title:          title,
		subtitle:       "History",
		subtitleDetail: historyTitleDetail(data),
		body:           historyHTML(data),
	}, snapshot.shellTheme)
	return result, nil
}

func loadHistoryWindowSnapshot(ctx context.Context, snapshot historyRouteSnapshot) (*entity.HistoryWindow, []*entity.HistoryEntry, error) {
	query := strings.TrimSpace(snapshot.query)
	domain := strings.TrimSpace(snapshot.domainFilter)
	if query != "" {
		entries, err := snapshot.history.Search(ctx, query, historySearchLimit)
		return nil, entries, err
	}
	window, err := snapshot.history.TimelineWindow(ctx, snapshot.windowBefore, domain)
	if err != nil {
		return nil, nil, err
	}
	if window == nil {
		return nil, nil, nil
	}
	return window, window.Entries, nil
}

func (a *App) commitHistoryRouteResultLocked(result historyRouteResult) {
	a.favorites = nil
	a.folders = nil
	a.tags = nil
	a.historyEntries = result.entries
	a.historyStats = result.stats
	a.historyAnalytics = nil
	a.historyDomainStats = result.domains
	a.historyWindowBefore = result.windowBefore
	a.historyWindowAfter = result.windowAfter
	a.historyHasMore = result.hasMore
	a.renderedHTML = result.html
}

func (a *App) loadFavoritesRoute(ctx context.Context) error {
	if a.deps.Favorites == nil {
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			title:    routeDocumentTitle(RouteFavorites),
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
	data := favoritesRenderData{
		Favorites:    favorites,
		Folders:      folders,
		Tags:         tags,
		FolderFilter: a.favoriteFolderFilter,
		TagFilter:    a.favoriteTagFilter,
		Notice:       a.favoritesNotice,
		Error:        a.favoritesError,
	}
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    RouteFavorites,
		title:    favoritesDocumentTitle(data),
		subtitle: "Saved bookmarks",
		body:     favoritesHTML(data),
	}, a.shellTheme)
	return nil
}

func (a *App) loadConfigRoute(ctx context.Context) error {
	if a.deps.Config == nil {
		a.resetRouteState()
		a.renderedHTML = renderAppFrame(renderedPage{
			route:    a.currentRoute,
			title:    routeDocumentTitle(RouteConfig),
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
		title:    "Config — Dumber",
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
	a.historyStats = nil
	a.historyAnalytics = nil
	a.historyDomainStats = nil
	a.historyQuery = ""
	a.historyDomainFilter = ""
	a.historyOffset = 0
	a.historyWindowBefore = time.Time{}
	a.historyWindowAfter = time.Time{}
	a.historyHasMore = false
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

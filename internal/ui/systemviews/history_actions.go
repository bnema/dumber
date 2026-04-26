package systemviews

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	historyActionSearch       = "history.search"
	historyActionClear        = "history.clear"
	historyActionFilterDomain = "history.filterDomain"
	historyActionClearDomain  = "history.clearDomain"
	historyActionPage         = "history.page"
	historyActionLoadMore     = "history.loadMore"
	historyActionDeleteEntry  = "history.deleteEntry"
	historyActionDeleteRange  = "history.deleteRange"
	historyActionDeleteDomain = "history.deleteDomain"
)

// HandleDOMAction applies an action delegated from the browser DOM and refreshes
// the mounted view. It keeps UI orchestration in the systemview edge and calls
// only application ports for data changes.
func (a *App) HandleDOMAction(ctx context.Context, event DOMAction) error {
	if a == nil {
		return fmt.Errorf("app is nil")
	}
	a.lockState()
	if a.closed {
		a.unlockState()
		return nil
	}
	a.renderGeneration++
	if a.currentRoute == RouteUnknown || a.currentRoute == "" {
		a.currentRoute = ParseRoute(a.deps.LocationURI)
	}
	route := a.currentRoute
	if route == RouteHistory && event.Action == historyActionLoadMore {
		a.unlockState()
		return a.handleHistoryLoadMore(ctx, event)
	}
	defer a.unlockState()

	switch route {
	case RouteHistory:
		a.historyError = ""
		if err := a.handleHistoryAction(ctx, event); err != nil {
			a.historyNotice = ""
			a.historyError = err.Error()
		}
		if err := a.loadHistoryRoute(ctx); err != nil {
			a.renderRouteError(err)
			_ = a.mountRenderedHTML()
			return err
		}
		return a.mountRenderedHTML()
	case RouteFavorites:
		a.favoritesError = ""
		if err := a.handleFavoriteAction(ctx, event); err != nil {
			a.favoritesNotice = ""
			a.favoritesError = err.Error()
		}
		if err := a.loadFavoritesRoute(ctx); err != nil {
			a.renderRouteError(err)
			_ = a.mountRenderedHTML()
			return err
		}
		return a.mountRenderedHTML()
	case RouteConfig:
		a.configError = ""
		if err := a.handleConfigAction(ctx, event); err != nil {
			a.configNotice = ""
			a.configError = err.Error()
		}
		a.loadShellTheme(ctx)
		if err := a.loadConfigRoute(ctx); err != nil {
			a.renderRouteError(err)
			_ = a.mountRenderedHTML()
			return err
		}
		return a.mountRenderedHTML()
	default:
		return nil
	}
}

//nolint:gocyclo,funlen // Mechanical dispatcher keeps action routing in one place.
func (a *App) handleHistoryAction(ctx context.Context, event DOMAction) error {
	if a.deps.History == nil {
		return fmt.Errorf("history service not configured")
	}

	data := event.Data
	switch event.Action {
	case historyActionSearch:
		a.historyQuery = strings.TrimSpace(data["query"])
		a.historyDomainFilter = ""
		a.resetHistoryWindowState()
		a.historyNotice = ""
	case historyActionClear:
		a.historyQuery = ""
		a.historyDomainFilter = ""
		a.resetHistoryWindowState()
		a.historyNotice = ""
	case historyActionFilterDomain:
		domain := strings.TrimSpace(data["domain"])
		if domain == "" {
			return fmt.Errorf("domain is required")
		}
		a.historyDomainFilter = domain
		a.historyQuery = ""
		a.resetHistoryWindowState()
		a.historyNotice = ""
	case historyActionClearDomain:
		a.historyDomainFilter = ""
		a.resetHistoryWindowState()
		a.historyNotice = ""
	case historyActionPage:
		offset, err := strconv.Atoi(strings.TrimSpace(data["offset"]))
		if err != nil || offset < 0 {
			return fmt.Errorf("invalid history offset")
		}
		a.resetHistoryWindowState()
		a.historyOffset = offset
		a.historyNotice = ""
	case historyActionDeleteEntry:
		id, err := strconv.ParseInt(strings.TrimSpace(data["id"]), 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("invalid history entry id")
		}
		if err := a.deps.History.DeleteEntry(ctx, id); err != nil {
			return err
		}
		a.resetHistoryWindowState()
		a.historyNotice = "Deleted history entry"
	case historyActionDeleteRange:
		rangeID := strings.TrimSpace(data["range"])
		if rangeID == "" {
			return fmt.Errorf("history range is required")
		}
		if !isKnownHistoryCleanupRange(rangeID) {
			return fmt.Errorf("invalid history range")
		}
		if err := a.deps.History.DeleteRange(ctx, rangeID); err != nil {
			return err
		}
		a.resetHistoryWindowState()
		a.historyNotice = historyRangeNotice(rangeID)
	case historyActionDeleteDomain:
		domain := strings.TrimSpace(data["domain"])
		if domain == "" {
			return fmt.Errorf("domain is required")
		}
		if err := a.deps.History.DeleteDomain(ctx, domain); err != nil {
			return err
		}
		if strings.EqualFold(a.historyDomainFilter, domain) {
			a.historyDomainFilter = ""
		}
		a.resetHistoryWindowState()
		a.historyNotice = "Deleted history for " + domain
	default:
		return fmt.Errorf("unknown history action: %q", event.Action)
	}
	return nil
}

func (a *App) handleHistoryLoadMore(ctx context.Context, event DOMAction) error {
	a.lockState()
	if a.deps.History == nil {
		a.unlockState()
		return fmt.Errorf("history service not configured")
	}
	if strings.TrimSpace(a.historyQuery) != "" || !a.historyHasMore {
		a.unlockState()
		return nil
	}
	before := a.historyWindowAfter
	if requested := strings.TrimSpace(event.Data["before"]); requested != "" {
		parsed, err := time.Parse(time.RFC3339Nano, requested)
		if err != nil {
			a.unlockState()
			return fmt.Errorf("invalid history cursor")
		}
		if !before.IsZero() && !parsed.Equal(before) {
			a.unlockState()
			return nil
		}
		before = parsed
	}
	if before.IsZero() {
		before = time.Now()
	}
	domain := a.historyDomainFilter
	history := a.deps.History
	dom := a.deps.DOM
	a.unlockState()

	window, err := history.TimelineWindow(ctx, before, domain)
	if err != nil {
		return err
	}
	if window == nil {
		a.lockState()
		a.historyHasMore = false
		a.unlockState()
		return nil
	}

	a.lockState()
	appendSkipFirstDate := lastHistoryDateKey(a.historyEntries)
	a.historyEntries = append(a.historyEntries, window.Entries...)
	a.historyWindowBefore = window.Before
	a.historyWindowAfter = window.After
	a.historyHasMore = window.HasMore
	data := historyRenderData{
		Entries:             window.Entries,
		Stats:               a.historyStats,
		Domains:             a.historyDomainStats,
		Query:               a.historyQuery,
		DomainFilter:        a.historyDomainFilter,
		Offset:              a.historyOffset,
		Limit:               historyTimelineLimit,
		WindowBefore:        a.historyWindowBefore,
		WindowAfter:         a.historyWindowAfter,
		HasMore:             a.historyHasMore,
		AppendSkipFirstDate: appendSkipFirstDate,
		Notice:              a.historyNotice,
		Error:               a.historyError,
	}
	fragment := historyTimelineAppendHTML(data)
	var fallbackHTML string
	if _, ok := dom.(DOMHistoryTimelineAppender); !ok {
		fullData := data
		fullData.Entries = a.historyEntries
		title := historyDocumentTitle(fullData)
		a.renderedHTML = renderAppFrame(renderedPage{
			route:          RouteHistory,
			title:          title,
			subtitle:       "History",
			subtitleDetail: historyTitleDetail(fullData),
			body:           historyHTML(fullData),
		}, a.shellTheme)
		fallbackHTML = a.renderedHTML
	}
	a.unlockState()

	if appender, ok := dom.(DOMHistoryTimelineAppender); ok {
		return appender.AppendHistoryTimeline(fragment)
	}
	return a.mountHTML(fallbackHTML)
}

func (a *App) resetHistoryWindowState() {
	a.historyOffset = 0
	a.historyWindowBefore = time.Time{}
	a.historyWindowAfter = time.Time{}
	a.historyHasMore = false
}

func (a *App) mountRenderedHTML() error {
	return a.mountHTML(a.renderedHTML)
}

func (a *App) mountHTML(html string) error {
	if a == nil || a.deps.DOM == nil {
		return nil
	}
	return a.deps.DOM.Mount(html)
}

func (a *App) mountHTMLIfCurrent(ctx context.Context, html string, generation uint64) error {
	if a == nil || a.deps.DOM == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return nil
	}
	a.lockState()
	current := !a.closed && a.renderGeneration == generation
	a.unlockState()
	if !current {
		return nil
	}
	return a.deps.DOM.Mount(html)
}

func isKnownHistoryCleanupRange(rangeID string) bool {
	for _, item := range historyCleanupItems() {
		if item.RangeID == rangeID {
			return true
		}
	}
	return false
}

func historyRangeNotice(rangeID string) string {
	for _, item := range historyCleanupItems() {
		if item.RangeID == rangeID {
			return item.Notice
		}
	}
	return "Deleted history"
}

package systemviews

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	historyActionSearch       = "history.search"
	historyActionClear        = "history.clear"
	historyActionFilterDomain = "history.filterDomain"
	historyActionClearDomain  = "history.clearDomain"
	historyActionPage         = "history.page"
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
	if a.currentRoute == RouteUnknown || a.currentRoute == "" {
		a.currentRoute = ParseRoute(a.deps.LocationURI)
	}

	switch a.currentRoute {
	case RouteHistory:
		a.historyError = ""
		if err := a.handleHistoryAction(ctx, event); err != nil {
			a.historyNotice = ""
			a.historyError = err.Error()
		}
		if err := a.loadHistoryRoute(ctx); err != nil {
			a.renderRouteError(err)
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
		}
		return a.mountRenderedHTML()
	default:
		return nil
	}
}

func (a *App) handleHistoryAction(ctx context.Context, event DOMAction) error {
	if a.deps.History == nil {
		return fmt.Errorf("history service not configured")
	}

	data := event.Data
	switch event.Action {
	case historyActionSearch:
		a.historyQuery = strings.TrimSpace(data["query"])
		a.historyOffset = 0
		a.historyNotice = ""
	case historyActionClear:
		a.historyQuery = ""
		a.historyDomainFilter = ""
		a.historyOffset = 0
		a.historyNotice = ""
	case historyActionFilterDomain:
		domain := strings.TrimSpace(data["domain"])
		if domain == "" {
			return fmt.Errorf("domain is required")
		}
		a.historyDomainFilter = domain
		a.historyOffset = 0
		a.historyNotice = ""
	case historyActionClearDomain:
		a.historyDomainFilter = ""
		a.historyOffset = 0
		a.historyNotice = ""
	case historyActionPage:
		offset, err := strconv.Atoi(strings.TrimSpace(data["offset"]))
		if err != nil || offset < 0 {
			return fmt.Errorf("invalid history offset")
		}
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
		a.historyNotice = "Deleted history entry"
	case historyActionDeleteRange:
		rangeID := strings.TrimSpace(data["range"])
		if !validHistoryRange(rangeID) {
			return fmt.Errorf("invalid history cleanup range")
		}
		if err := a.deps.History.DeleteRange(ctx, rangeID); err != nil {
			return err
		}
		a.historyOffset = 0
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
		a.historyOffset = 0
		a.historyNotice = "Deleted history for " + domain
	}
	return nil
}

func (a *App) mountRenderedHTML() error {
	if a.deps.DOM == nil {
		return nil
	}
	return a.deps.DOM.Mount(a.renderedHTML)
}

func validHistoryRange(rangeID string) bool {
	switch rangeID {
	case "hour", "day", "week", "month", "all":
		return true
	default:
		return false
	}
}

func historyRangeNotice(rangeID string) string {
	switch rangeID {
	case "hour":
		return "Deleted history from the last hour"
	case "day":
		return "Deleted history from today"
	case "week":
		return "Deleted history from this week"
	case "month":
		return "Deleted history from this month"
	case "all":
		return "Deleted all history"
	default:
		return "Deleted history"
	}
}

package systemviews

import (
	"context"
	"strings"
)

// historyActionSearchLive is sent while the user types in the history search
// field. It is coalesced (latest wins) and only refreshes the result regions.
const historyActionSearchLive = "history.searchLive"

// coalesceLiveSearchLocked records the newest live query. It reports whether
// a live search action still needs to be queued; when one is already waiting,
// that queued action will pick up this query instead. Caller holds actionMu.
func (a *App) coalesceLiveSearchLocked(action DOMAction) bool {
	a.liveSearchQuery = action.Data["query"]
	return !a.liveSearchQueued
}

// takeLiveSearchQuery returns the newest coalesced query, or fallback when the
// action did not come through the queue (direct calls in tests).
func (a *App) takeLiveSearchQuery(fallback string) string {
	a.lockAction()
	defer a.unlockAction()
	if !a.liveSearchQueued {
		return fallback
	}
	a.liveSearchQueued = false
	return a.liveSearchQuery
}

// handleHistoryLiveSearch refreshes the timeline for query without reloading
// stats or domains, and replaces only the result regions when the DOM can.
func (a *App) handleHistoryLiveSearch(ctx context.Context, query string) error {
	query = strings.TrimSpace(query)

	a.lockState()
	if a.closed || a.currentRoute != RouteHistory || a.deps.History == nil {
		a.unlockState()
		return nil
	}
	if query == a.historyQuery && a.historyDomainFilter == "" {
		a.unlockState()
		return nil
	}
	a.renderGeneration++
	generation := a.renderGeneration
	a.historyQuery = query
	a.historyDomainFilter = ""
	a.resetHistoryWindowState()
	a.historyNotice = ""
	a.historyError = ""
	snapshot := a.currentHistoryRouteSnapshotLocked()
	loaded := a.historyLoaded
	a.unlockState()

	if !loaded {
		return a.renderFullHistoryForLiveSearch(ctx, snapshot, generation)
	}

	window, entries, loadErr := loadHistoryWindowSnapshot(ctx, snapshot)

	a.lockState()
	defer a.unlockState()
	if a.closed || generation != a.renderGeneration {
		return nil
	}
	if loadErr != nil {
		a.historyError = loadErr.Error()
		window, entries = nil, nil
	}
	a.historyEntries = a.withoutPendingDeletesLocked(entries)
	if window != nil {
		a.historyWindowBefore = window.Before
		a.historyWindowAfter, a.historyCursorID, a.historyHasMore = historyWindowCursor(window)
	}

	data := a.historyRenderDataLocked()
	a.renderedHTML = renderAppFrame(renderedPage{
		route:          RouteHistory,
		title:          historyDocumentTitle(data),
		subtitle:       "History",
		subtitleDetail: historyTitleDetail(data),
		body:           historyHTML(data),
	}, a.shellTheme)

	if updater, ok := a.deps.DOM.(DOMRegionUpdater); ok {
		return updater.UpdateRegions(RegionUpdate{Regions: historyLiveRegions(data)})
	}
	return a.mountRenderedHTML()
}

// renderFullHistoryForLiveSearch renders and mounts the whole history route
// when live search runs before the initial async load committed. The live
// search bumps renderGeneration, which discards that load, so this path must
// load stats and domains too.
func (a *App) renderFullHistoryForLiveSearch(ctx context.Context, snapshot historyRouteSnapshot, generation uint64) error {
	result, err := a.renderHistoryRouteSnapshot(ctx, snapshot)

	a.lockState()
	defer a.unlockState()
	if a.closed || generation != a.renderGeneration || a.currentRoute != snapshot.route {
		return nil
	}
	if err != nil {
		a.renderRouteError(err)
	} else {
		a.commitHistoryRouteResultLocked(result)
	}
	return a.mountRenderedHTML()
}

func (a *App) historyRenderDataLocked() historyRenderData {
	return historyRenderData{
		Entries:      a.historyEntries,
		Stats:        a.historyStats,
		Domains:      a.historyDomainStats,
		Query:        a.historyQuery,
		DomainFilter: a.historyDomainFilter,
		Offset:       a.historyOffset,
		Limit:        historyTimelineLimit,
		WindowBefore: a.historyWindowBefore,
		WindowAfter:  a.historyWindowAfter,
		HasMore:      a.historyHasMore,
		Notice:       a.historyNotice,
		Error:        a.historyError,
	}
}

package systemviews

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestParseRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uri  string
		want Route
	}{
		{name: "empty", uri: "", want: RouteUnknown},
		{name: "scheme only", uri: "://", want: RouteUnknown},
		{name: "dumb scheme only", uri: "dumb://", want: RouteUnknown},
		{name: "dumb unknown host", uri: "dumb://unknown", want: RouteUnknown},
		{name: "dumb opaque bogus", uri: "dumb:bogus", want: RouteUnknown},
		{name: "history host", uri: "dumb://history", want: RouteHistory},
		{name: "history opaque", uri: "dumb:history", want: RouteHistory},
		{name: "favorites host", uri: "dumb://favorites", want: RouteFavorites},
		{name: "favorites opaque", uri: "dumb:favorites", want: RouteFavorites},
		{name: "config host", uri: "dumb://config", want: RouteConfig},
		{name: "config opaque", uri: "dumb:config", want: RouteConfig},
		{name: "settings alias", uri: "dumb://settings", want: RouteConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseRoute(tt.uri); got != tt.want {
				t.Fatalf("ParseRoute(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestAppRunMountsPlaceholderAndRecordsRoute(t *testing.T) {
	t.Parallel()

	dom := &recordingDOM{}
	app := NewApp(Dependencies{
		DOM:         dom,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.Run())
	assert.Equal(t, RouteHistory, app.CurrentRoute())
	assert.True(t, dom.Mounted())
	assert.Contains(t, dom.HTML(), "history")
	assert.Contains(t, dom.HTML(), "systemviews")
}

func TestAppLoadInitialHistoryRouteUsesStyledSections(t *testing.T) {
	t.Parallel()

	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{
		URL:   "https://example.com",
		Title: "Example",
	}}})

	app := NewApp(Dependencies{
		History:     history.service,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Equal(t, RouteHistory, app.CurrentRoute())
	assert.Len(t, app.historyEntries, 1)
	assert.Contains(t, app.renderedHTML, "Example")
	assert.Contains(t, app.renderedHTML, "https://example.com")
	assert.True(t, history.called)
	// Analytics and Timeline have no expectation: the history route must not call them.

	// Shell frame present, no full document wrapper.
	assert.NotContains(t, app.renderedHTML, "<html")
	assert.NotContains(t, app.renderedHTML, "<head")
	assert.NotContains(t, app.renderedHTML, "<body")
	assert.Contains(t, app.renderedHTML, `class="sv-shell"`)
	assert.Contains(t, app.renderedHTML, `data-route="history"`)
	assert.Contains(t, app.renderedHTML, `data-page-title="History · 1 entry · 1 visit · 0 days"`)
	assert.Contains(t, app.renderedHTML, `sv-panel sv-history-panel`)
	assert.Contains(t, app.renderedHTML, `class="sv-rows"`)
}

func TestAppLoadInitialHistoryRouteShowsTotalStatsWithWindowedTimeline(t *testing.T) {
	t.Parallel()

	history := newHistoryStore(t, &historyStore{
		entries: []*entity.HistoryEntry{
			{ID: 1, URL: "https://older.example", Title: "Older", VisitCount: 2},
			{ID: 2, URL: "https://newer.example", Title: "Newer", VisitCount: 3},
		},
		stats: &entity.HistoryStats{TotalEntries: 8904, TotalVisits: 40947, UniqueDays: 121},
	})
	app := NewApp(Dependencies{History: history.service, LocationURI: "dumb://history"})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.True(t, history.statsCalled)
	assert.Contains(t, app.renderedHTML, `data-page-title="History · 8,904 entries · 40,947 visits · 121 days"`)
	assert.Contains(t, app.renderedHTML, `<h1 class="sv-title">History <span class="sv-title-detail">8,904 entries · 40,947 visits · 121 days</span></h1>`)
	assert.NotContains(t, app.renderedHTML, `sv-history-summary`)
	assert.NotContains(t, app.renderedHTML, `sv-stat-value`)
}

func TestAppRunMountsHistoryLoadingBeforeAsyncHydration(t *testing.T) {
	t.Parallel()

	dom := &recordingActionDOM{recordingDOM: recordingDOM{mounts: make(chan string, 4)}}
	history := newHistoryStore(t, &historyStore{
		entries:         []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Loaded entry"}},
		timelineStarted: make(chan struct{}),
		releaseTimeline: make(chan struct{}),
	})
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})

	require.NoError(t, app.RunWithContext(context.Background()))
	loadingHTML := receiveMount(t, dom.mounts)
	assert.Contains(t, loadingHTML, "Loading history")
	assert.NotContains(t, loadingHTML, "Loaded entry")

	select {
	case <-history.timelineStarted:
	case <-time.After(time.Second):
		t.Fatal("history timeline was not started asynchronously")
	}
	close(history.releaseTimeline)

	hydratedHTML := receiveMount(t, dom.mounts)
	assert.Contains(t, hydratedHTML, "Loaded entry")
}

func TestSurfaceActionErrorPreventsStaleAsyncHydrationMount(t *testing.T) {
	t.Parallel()

	dom := &recordingDOM{mounts: make(chan string, 4)}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	staleHTML := "<div>Stale hydrated entry</div>"
	staleGeneration := app.renderGeneration
	ctx := context.Background()

	app.surfaceActionError(ctx, errors.New("action worker failed"))
	errorHTML := receiveMount(t, dom.mounts)
	assert.Contains(t, errorHTML, "action worker failed")
	assert.NotContains(t, errorHTML, "Stale hydrated entry")

	require.NoError(t, app.mountHTMLIfCurrent(ctx, staleHTML, staleGeneration))
	assertNoMount(t, dom.mounts)
	assert.Contains(t, dom.HTML(), "action worker failed")
	assert.NotContains(t, dom.HTML(), "Stale hydrated entry")
}

func TestSurfaceActionErrorWhileAsyncHydrationBlockedPreventsStaleMount(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		dom := &recordingActionDOM{recordingDOM: recordingDOM{mounts: make(chan string, 4)}}
		history := newHistoryStore(t, &historyStore{
			entries:         []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Stale hydrated entry"}},
			timelineStarted: make(chan struct{}),
			releaseTimeline: make(chan struct{}),
		})
		app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})
		defer app.Close()
		ctx := context.Background()

		require.NoError(t, app.RunWithContext(ctx))
		loadingHTML := receiveMount(t, dom.mounts)
		assert.Contains(t, loadingHTML, "Loading history")

		<-history.timelineStarted

		app.surfaceActionError(ctx, errors.New("action worker failed"))
		errorHTML := receiveMount(t, dom.mounts)
		assert.Contains(t, errorHTML, "action worker failed")
		assert.NotContains(t, errorHTML, "Stale hydrated entry")

		close(history.releaseTimeline)
		synctest.Wait() // the hydration goroutine has finished or is durably blocked
		assertNoMount(t, dom.mounts)
		assert.Contains(t, dom.HTML(), "action worker failed")
		assert.NotContains(t, dom.HTML(), "Stale hydrated entry")
	})
}

func TestCurrentHistoryRouteSnapshotIncludesWindowAfter(t *testing.T) {
	cursor := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	app := NewApp(Dependencies{History: newHistoryStore(t, &historyStore{}).service, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	app.historyWindowAfter = cursor

	snapshot := app.currentHistoryRouteSnapshotLocked()
	assert.Equal(t, cursor, snapshot.windowAfter)
}

func TestAppHandleHistoryLoadMoreAppendsOlderWindow(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	history := newHistoryStore(t, &historyStore{window: &entity.HistoryWindow{
		Entries:           []*entity.HistoryEntry{{ID: 2, URL: "https://older.example", Title: "Older entry", LastVisited: cursor.Add(-time.Hour)}},
		Before:            cursor,
		CursorLastVisited: cursor.Add(-time.Hour),
		CursorID:          2,
		HasMore:           true,
	}})
	dom := &recordingActionDOM{}
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	app.historyEntries = []*entity.HistoryEntry{{ID: 1, URL: "https://newer.example", Title: "Newer entry", LastVisited: cursor}}
	app.historyWindowAfter = cursor
	app.historyCursorID = 1
	app.historyHasMore = true

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionLoadMore,
		Data:   map[string]string{"before": cursor.Format(time.RFC3339Nano)},
	}))
	assert.Equal(t, cursor, history.windowBefore)
	assert.Equal(t, int64(1), history.windowBeforeID)
	assert.Equal(t, int64(2), app.historyCursorID)
	assert.Equal(t, cursor.Add(-time.Hour), app.historyWindowAfter)
	assert.Len(t, app.historyEntries, 2)
	assert.Contains(t, dom.AppendedHTML(), "Older entry")
	assert.Contains(t, dom.AppendedHTML(), `data-sv-action="history.loadMore"`)
	assert.Empty(t, dom.HTML(), "load-more should append a fragment instead of remounting the whole page")
}

func TestAppHandleHistoryLoadMoreRemountsWhenAppendUnavailable(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	history := newHistoryStore(t, &historyStore{window: &entity.HistoryWindow{
		Entries:           []*entity.HistoryEntry{{ID: 2, URL: "https://older.example", Title: "Older entry", LastVisited: cursor.Add(-time.Hour)}},
		Before:            cursor,
		CursorLastVisited: cursor.Add(-time.Hour),
		CursorID:          2,
		HasMore:           true,
	}})
	dom := &recordingDOM{}
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	app.historyEntries = []*entity.HistoryEntry{{ID: 1, URL: "https://newer.example", Title: "Newer entry", LastVisited: cursor}}
	app.historyWindowAfter = cursor
	app.historyCursorID = 1
	app.historyHasMore = true

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionLoadMore,
		Data:   map[string]string{"before": cursor.Format(time.RFC3339Nano)},
	}))
	assert.Equal(t, cursor, history.windowBefore)
	assert.Equal(t, int64(1), history.windowBeforeID)
	assert.Len(t, app.historyEntries, 2)
	assert.Contains(t, dom.HTML(), "Older entry")
	assert.Contains(t, dom.HTML(), `data-sv-action="history.loadMore"`)
}

func TestAppHandleHistoryLoadMoreDisablesPartialNextCursor(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	history := newHistoryStore(t, &historyStore{window: &entity.HistoryWindow{
		Entries: []*entity.HistoryEntry{{ID: 2, URL: "https://older.example", Title: "Older entry", LastVisited: cursor.Add(-time.Hour)}},
		Before:  cursor,
		After:   cursor.Add(-24 * time.Hour),
		HasMore: true,
	}})
	dom := &recordingActionDOM{}
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	app.historyEntries = []*entity.HistoryEntry{{ID: 1, URL: "https://newer.example", Title: "Newer entry", LastVisited: cursor}}
	app.historyWindowAfter = cursor
	app.historyCursorID = 1
	app.historyHasMore = true

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionLoadMore,
		Data:   map[string]string{"before": cursor.Format(time.RFC3339Nano)},
	}))
	assert.False(t, app.historyHasMore)
	assert.Zero(t, app.historyCursorID)
	assert.True(t, app.historyWindowAfter.IsZero())
	assert.NotContains(t, dom.AppendedHTML(), `data-sv-action="history.loadMore"`)
}

func TestAppHandleHistoryLoadMoreIgnoresStaleCursor(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	history := newHistoryStore(t, &historyStore{window: &entity.HistoryWindow{Entries: []*entity.HistoryEntry{{ID: 2, URL: "https://older.example"}}}})
	dom := &recordingActionDOM{}
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	app.historyWindowAfter = cursor
	app.historyHasMore = true

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionLoadMore,
		Data:   map[string]string{"before": cursor.Add(-24 * time.Hour).Format(time.RFC3339Nano)},
	}))
	assert.False(t, history.called)
	assert.Empty(t, dom.AppendedHTML())
}

func TestAppLoadInitialHistoryRouteRendersManagementActions(t *testing.T) {
	t.Parallel()

	history := newHistoryStore(t, &historyStore{
		entries: []*entity.HistoryEntry{{
			ID:    42,
			URL:   "https://example.com/page",
			Title: "Example",
		}},
		domainStats: []*entity.DomainStat{{Domain: "www.example.com:8080", PageCount: 1, TotalVisits: 3}},
	})

	app := NewApp(Dependencies{
		History:     history.service,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, `data-sv-action="history.search"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="history.deleteEntry"`)
	assert.Contains(t, app.renderedHTML, `data-id="42"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="history.deleteRange"`)
	assert.Contains(t, app.renderedHTML, `data-range="hour"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="history.filterDomain"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="history.deleteDomain"`)
	assert.Contains(t, app.renderedHTML, `data-domain="example.com:8080"`)
	assert.Contains(t, app.renderedHTML, `>example.com</button>`)
	assert.Contains(t, app.renderedHTML, `data-sv-history-search data-sv-autofocus`)
	assert.NotContains(t, app.renderedHTML, "Keys:")
}

func TestAppHandleHistoryActionsRefreshesDOM(t *testing.T) {
	dom := &recordingDOM{}
	history := newHistoryStore(t, &historyStore{
		entries:       []*entity.HistoryEntry{{ID: 42, URL: "https://example.com", Title: "Example"}},
		searchEntries: []*entity.HistoryEntry{{ID: 7, URL: "https://search.example", Title: "Search result"}},
	})
	history.expectDeleteEntry(42)
	history.expectDeleteRange("week")
	history.expectDeleteDomain("example.com")
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionSearch,
		Data:   map[string]string{"query": " example "},
	}))
	assert.True(t, history.searchCalled)
	assert.Equal(t, "example", history.query)
	assert.Contains(t, dom.HTML(), "Search result")
	assert.Contains(t, dom.HTML(), "Showing 1 matching item")

	app.historyOffset = historyTimelineLimit
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteEntry,
		Data:   map[string]string{"id": "42"},
	}))
	assert.Equal(t, []int64{42}, history.deletedEntryIDs())
	assert.Equal(t, 0, app.historyOffset)
	assert.Contains(t, dom.HTML(), "Deleted history entry")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteRange,
		Data:   map[string]string{"range": "week"},
	}))
	assert.Contains(t, dom.HTML(), "Deleted history from this week")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: historyActionClear}))
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionFilterDomain,
		Data:   map[string]string{"domain": "example.com"},
	}))
	assert.True(t, history.called)
	assert.Equal(t, "example.com", history.windowDomain)
	assert.Empty(t, app.historyQuery)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteDomain,
		Data:   map[string]string{"domain": "example.com"},
	}))
	assert.Contains(t, dom.HTML(), "Deleted history for example.com")
}

func TestAppCloseStopsActionWorkerAndReleasesDOM(t *testing.T) {
	dom := &recordingActionDOM{}
	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}})
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})

	ctx := t.Context()
	require.NoError(t, app.RunWithContext(ctx))
	require.NotNil(t, dom.handler)

	app.Close()
	assert.True(t, dom.released)
	assert.False(t, app.enqueueDOMAction(DOMAction{Action: historyActionClear}))
	require.NotPanics(t, func() {
		dom.handler(DOMAction{Action: historyActionClear})
	})
}

func TestAppRunCleansActionWorkerWhenBindingFails(t *testing.T) {
	bindErr := errors.New("bind failed")
	dom := &failingActionDOM{bindErr: bindErr}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://history"})

	err := app.RunWithContext(context.Background())
	require.ErrorIs(t, err, bindErr)
	assert.Nil(t, app.actionQueue)
	assert.Nil(t, app.actionErrorQueue)
	assert.Nil(t, app.actionCtx)
	assert.False(t, app.actionWorkerActive())
	require.NotPanics(t, app.Close)
}

func TestAppRunReleasesDOMWhenActionWorkerCannotStart(t *testing.T) {
	dom := &recordingActionDOM{}
	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}})
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := app.RunWithContext(ctx)

	require.ErrorContains(t, err, "systemview action worker unavailable")
	assert.True(t, dom.released)
	assert.Nil(t, app.actionQueue)
	assert.Nil(t, app.actionErrorQueue)
	assert.Nil(t, app.actionCtx)
	assert.False(t, app.actionWorkerActive())
}

func TestAppLoadInitialHistoryRouteRendersErrorState(t *testing.T) {
	t.Parallel()

	history := newHistoryStore(t, &historyStore{err: errors.New("database unavailable")})
	app := NewApp(Dependencies{
		History:     history.service,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Equal(t, RouteHistory, app.CurrentRoute())
	assert.True(t, history.called)
	assert.Contains(t, app.renderedHTML, "Could not load this system view")
	assert.Contains(t, app.renderedHTML, "database unavailable")
	assert.Contains(t, app.renderedHTML, `role="alert"`)
}

func TestAppLoadInitialHistoryRouteAppliesThemeTokens(t *testing.T) {
	t.Parallel()

	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{
		URL:   "https://example.com",
		Title: "Example",
	}}})
	config := mocks.NewMockSystemviewConfigService(t)
	config.EXPECT().Current(mock.Anything).Return(dto.SystemviewConfigPayload{
		Appearance: dto.WebUIAppearanceConfig{
			ColorScheme: "prefer-light",
			LightPalette: dto.ColorPalette{
				Background:     "#ffffff",
				Surface:        "#fafafa",
				SurfaceVariant: "#eeeeee",
				Text:           "#111111",
				Muted:          "#666666",
				Accent:         "#0055ff",
				Border:         "#dddddd",
			},
			DarkPalette: dto.ColorPalette{
				Background:     "#111111",
				Surface:        "#1a1a1a",
				SurfaceVariant: "#2a2a2a",
				Text:           "#f5f5f5",
				Muted:          "#a0a0a0",
				Accent:         "#66aaff",
				Border:         "#333333",
			},
			SansFont:        "Inter",
			SerifFont:       "Georgia",
			MonospaceFont:   "JetBrains Mono",
			DefaultFontSize: 16,
		},
	}, nil)

	app := NewApp(Dependencies{
		Config:      config,
		History:     history.service,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, `class="sv-app sv-style-command sv-light"`)
	assert.Contains(t, app.renderedHTML, `--sv-background: #ffffff;`)
	assert.Contains(t, app.renderedHTML, `--sv-surface-variant: #eeeeee;`)
}

func TestAppLoadInitialFavoritesRouteRendersData(t *testing.T) {
	t.Parallel()

	favorites := newFavoritesStore(t, &favoritesStore{
		favorites: []*entity.Favorite{{URL: "https://example.com", Title: "Example"}},
		tags:      []*entity.Tag{{Name: "Go"}},
	})

	app := NewApp(Dependencies{
		Favorites:   favorites.service,
		LocationURI: "dumb://favorites",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Equal(t, RouteFavorites, app.CurrentRoute())
	assert.Len(t, app.favorites, 1)
	assert.Len(t, app.tags, 1)
	assert.Contains(t, app.renderedHTML, "Favorites")
	assert.Contains(t, app.renderedHTML, "Example")
	assert.Contains(t, app.renderedHTML, "Go")
	assert.True(t, favorites.calledList)
	assert.True(t, favorites.calledTags)

	// Shell frame present, no full document wrapper.
	assert.NotContains(t, app.renderedHTML, "<html")
	assert.NotContains(t, app.renderedHTML, "<head")
	assert.NotContains(t, app.renderedHTML, "<body")
	assert.Contains(t, app.renderedHTML, `class="sv-shell"`)
	assert.Contains(t, app.renderedHTML, `data-route="favorites"`)
	assert.Contains(t, app.renderedHTML, `data-page-title="Favorites — 1 bookmark"`)
	assert.Contains(t, app.renderedHTML, `data-sv-favorites`)
	assert.Contains(t, app.renderedHTML, `data-sv-region="favorites-list"`)
}

func TestAppLoadInitialFavoritesRouteRendersCRUDControls(t *testing.T) {
	t.Parallel()

	shortcut := 3
	favorites := newFavoritesStore(t, &favoritesStore{
		favorites: []*entity.Favorite{{ID: 42, URL: "https://example.com", Title: "Example", ShortcutKey: &shortcut}},
		tags:      []*entity.Tag{{ID: 7, Name: "Go", Color: "#00add8"}},
	})

	app := NewApp(Dependencies{Favorites: favorites.service, LocationURI: "dumb://favorites"})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, `data-sv-action="favorite.create"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="favorite.save"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="favorite.delete"`)
	assert.NotContains(t, app.renderedHTML, `data-sv-action="folder.create"`)
	assert.NotContains(t, app.renderedHTML, `data-sv-action="folder.update"`)
	assert.NotContains(t, app.renderedHTML, `data-sv-action="folder.delete"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="tag.create"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="tag.update"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="tag.delete"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="favorite.filterUntagged"`)
	assert.Contains(t, app.renderedHTML, `name="tags"`)
	assert.Contains(t, app.renderedHTML, `value="7"`)
	assert.Contains(t, app.renderedHTML, "Go")
	assert.Contains(t, app.renderedHTML, `title="Shortcut 3"`)
}

func TestAppHandleFavoriteActionsRefreshesDOM(t *testing.T) {
	dom := &recordingDOM{}
	favorites := newFavoritesStore(t, &favoritesStore{
		favorites: []*entity.Favorite{{ID: 42, URL: "https://example.com", Title: "Example"}},
		tags:      []*entity.Tag{{ID: 7, Name: "Go", Color: "#00add8"}},
	})
	favorites.expectCreateFavorite("https://new.example")
	favorites.expectUpdateFavorite(42)
	favorites.expectAssignTag(42, 7)
	favorites.expectDeleteFavorite(42)
	app := NewApp(Dependencies{DOM: dom, Favorites: favorites.service, LocationURI: "dumb://favorites"})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "https://new.example", "title": "New", "tags": "7"},
	}))
	assert.Equal(t, "https://new.example", favorites.createdFavorite.URL)
	assert.Equal(t, []entity.TagID{7}, favorites.createdFavorite.Tags)
	assert.Contains(t, dom.HTML(), "Added “New”")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionSave,
		Data:   map[string]string{"id": "42", "title": "Updated", "favicon_url": "https://icon.example/favicon.ico", "shortcut_key": "4", "tags": ",7"},
	}))
	assert.Equal(t, entity.FavoriteID(42), favorites.updatedFavorite.ID)
	assert.Equal(t, "Updated", favorites.updatedFavorite.Title)
	assert.True(t, favorites.updatedFavorite.ShortcutKeySet)
	require.NotNil(t, favorites.updatedFavorite.ShortcutKey)
	assert.Equal(t, 4, *favorites.updatedFavorite.ShortcutKey)
	assert.Equal(t, [][2]int64{{42, 7}}, favorites.assigned)
	assert.Contains(t, dom.HTML(), "Saved “Updated”")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionFilterTag,
		Data:   map[string]string{"tagId": "7"},
	}))
	require.NotNil(t, app.favoriteTagFilter)
	assert.Equal(t, entity.TagID(7), *app.favoriteTagFilter)
	assert.False(t, app.favoriteUntaggedFilter)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionDelete,
		Data:   map[string]string{"id": "42"},
	}))
	assert.Equal(t, []int64{42}, favorites.deletedFavoriteIDs())
	assert.Contains(t, dom.HTML(), "Deleted favorite")
}

func TestAppLoadInitialConfigRouteRendersData(t *testing.T) {
	t.Parallel()

	config := dto.SystemviewConfigPayload{
		EngineType: "webkit",
		Appearance: dto.WebUIAppearanceConfig{
			ColorScheme:     "prefer-dark",
			SansFont:        "Inter",
			SerifFont:       "Georgia",
			MonospaceFont:   "JetBrains Mono",
			DefaultFontSize: 16,
			LightPalette: dto.ColorPalette{
				Background:     "#ffffff",
				Surface:        "#fafafa",
				SurfaceVariant: "#eeeeee",
				Text:           "#111111",
				Muted:          "#666666",
				Accent:         "#0055ff",
				Border:         "#dddddd",
			},
			DarkPalette: dto.ColorPalette{
				Background:     "#111111",
				Surface:        "#1a1a1a",
				SurfaceVariant: "#2a2a2a",
				Text:           "#f5f5f5",
				Muted:          "#a0a0a0",
				Accent:         "#66aaff",
				Border:         "#333333",
			},
			ExternalTheme: dto.WebUIExternalThemeConfig{
				Enabled:  true,
				Provider: "noctalia",
				Format:   "dumber-json",
				Path:     "/tmp/noctalia-dumber.json",
			},
		},
		Performance: dto.SystemviewPerformancePayload{
			Profile: "balanced",
			Custom: dto.SystemviewCustomPerformancePayload{
				SkiaCPUThreads:         4,
				SkiaGPUThreads:         2,
				WebProcessMemoryMB:     512,
				NetworkProcessMemoryMB: 128,
				WebViewPoolPrewarm:     1,
			},
			Hardware: dto.SystemviewHardwarePayload{
				CPUCores:   8,
				CPUThreads: 16,
				TotalRAMMB: 32768,
				GPUVendor:  "NVIDIA",
				GPUName:    "RTX 4060",
				VRAMMB:     8192,
			},
		},
		DefaultSearchEngine: "https://duckduckgo.com/?q=%s",
		SearchShortcuts: map[string]dto.SearchShortcut{
			"ddg": {
				URL:         "https://duckduckgo.com/?q=%s",
				Description: "DuckDuckGo",
			},
		},
	}
	service := mocks.NewMockSystemviewConfigService(t)
	service.EXPECT().Current(mock.Anything).Return(config, nil)
	service.EXPECT().GetKeybindings(mock.Anything).Return(port.KeybindingsConfig{Groups: []port.KeybindingGroup{
		{
			Mode:        "default",
			DisplayName: "Default",
			Bindings: []port.KeybindingEntry{
				{Action: "open", Description: "Open", Keys: []string{"ctrl+o"}, DefaultKeys: []string{"ctrl+o"}},
				{Action: "new-tab", Description: "New tab", Keys: []string{"ctrl+t"}, DefaultKeys: []string{"ctrl+shift+t"}, IsCustom: true},
			},
		},
		{
			Mode:        "search",
			DisplayName: "Search",
			Bindings: []port.KeybindingEntry{
				{Action: "find", Description: "Find", Keys: []string{"ctrl+f"}, DefaultKeys: []string{"ctrl+f"}},
			},
		},
	}}, nil)

	app := NewApp(Dependencies{
		Config:      service,
		LocationURI: "dumb://config",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Equal(t, RouteConfig, app.CurrentRoute())
	require.NotNil(t, app.config)
	assert.Equal(t, "webkit", app.config.EngineType)
	assert.Contains(t, app.renderedHTML, "webkit")
	assert.Contains(t, app.renderedHTML, "https://duckduckgo.com/?q=%s")
	assert.Contains(t, app.renderedHTML, "appearance.color_scheme")
	assert.Contains(t, app.renderedHTML, "prefer-dark")
	assert.Contains(t, app.renderedHTML, "appearance.sans_font")
	assert.Contains(t, app.renderedHTML, "Inter")
	assert.Contains(t, app.renderedHTML, "appearance.light_palette.background")
	assert.Contains(t, app.renderedHTML, "#ffffff")
	assert.Contains(t, app.renderedHTML, "appearance.external_theme.enabled")
	assert.Contains(t, app.renderedHTML, "appearance.external_theme.provider")
	assert.Contains(t, app.renderedHTML, "appearance.external_theme.format")
	assert.Contains(t, app.renderedHTML, "appearance.external_theme.path")
	assert.Contains(t, app.renderedHTML, "noctalia")
	assert.Contains(t, app.renderedHTML, "dumber-json")
	assert.Contains(t, app.renderedHTML, "/tmp/noctalia-dumber.json")
	assert.Contains(t, app.renderedHTML, "performance.profile")
	assert.Contains(t, app.renderedHTML, "balanced")
	assert.Contains(t, app.renderedHTML, "performance.custom.skia_cpu_threads")
	assert.Contains(t, app.renderedHTML, "4")
	assert.Contains(t, app.renderedHTML, "performance.hardware.gpu_vendor")
	assert.Contains(t, app.renderedHTML, "NVIDIA")
	assert.Contains(t, app.renderedHTML, "search_shortcuts.ddg.url")
	assert.Contains(t, app.renderedHTML, "Default")
	assert.Contains(t, app.renderedHTML, "Search")
	assert.Contains(t, app.renderedHTML, "Open")
	assert.Contains(t, app.renderedHTML, "ctrl+o")
	assert.Contains(t, app.renderedHTML, "New tab")
	assert.Contains(t, app.renderedHTML, "ctrl+t")
	assert.Contains(t, app.renderedHTML, "ctrl+shift+t")
	assert.Contains(t, app.renderedHTML, "Find")
	assert.Contains(t, app.renderedHTML, "open")
	assert.Contains(t, app.renderedHTML, "default")
	assert.Contains(t, app.renderedHTML, "custom")
	assert.NotContains(t, app.renderedHTML, "groups[0].bindings[0].action")
	service.AssertCalled(t, "Current", mock.Anything)
	service.AssertCalled(t, "GetKeybindings", mock.Anything)

	// Shell frame present, no full document wrapper.
	assert.NotContains(t, app.renderedHTML, "<html")
	assert.NotContains(t, app.renderedHTML, "<head")
	assert.NotContains(t, app.renderedHTML, "<body")
	assert.Contains(t, app.renderedHTML, `class="sv-shell"`)
	assert.Contains(t, app.renderedHTML, `data-route="config"`)
	assert.Contains(t, app.renderedHTML, `data-page-title="Settings — Dumber"`)
	assert.Contains(t, app.renderedHTML, `sv-config-section`)
	assert.Contains(t, app.renderedHTML, `data-sv-config-form`)
}

func TestAppLoadInitialConfigRouteUsesResolvedAppearanceOnlyForShellTheme(t *testing.T) {
	t.Parallel()

	cfg := testConfigPayload()
	resolved := cfg.Appearance
	resolved.DarkPalette.Background = "#222222"
	resolved.DarkPalette.SurfaceVariant = "#444444"
	cfg.ResolvedAppearance = &resolved
	service := mocks.NewMockSystemviewConfigService(t)
	service.EXPECT().Current(mock.Anything).Return(cfg, nil)
	service.EXPECT().GetKeybindings(mock.Anything).Return(port.KeybindingsConfig{}, nil)

	app := NewApp(Dependencies{Config: service, LocationURI: "dumb://config"})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, `--sv-background: #222222;`)
	assert.Contains(t, app.renderedHTML, `--sv-surface-variant: #444444;`)
	assert.Contains(t, app.renderedHTML, `value="#111111"`)
	assert.Contains(t, app.renderedHTML, `value="#2a2a2a"`)
}

func testConfigPayload() dto.SystemviewConfigPayload {
	return dto.SystemviewConfigPayload{
		EngineType: "webkit",
		Appearance: dto.WebUIAppearanceConfig{
			ColorScheme:     "prefer-dark",
			SansFont:        "Inter",
			SerifFont:       "Georgia",
			MonospaceFont:   "JetBrains Mono",
			DefaultFontSize: 16,
			LightPalette:    dto.ColorPalette{Background: "#ffffff", Surface: "#fafafa", SurfaceVariant: "#eeeeee", Text: "#111111", Muted: "#666666", Accent: "#0055ff", Border: "#dddddd"},
			DarkPalette:     dto.ColorPalette{Background: "#111111", Surface: "#1a1a1a", SurfaceVariant: "#2a2a2a", Text: "#f5f5f5", Muted: "#a0a0a0", Accent: "#66aaff", Border: "#333333"},
			ExternalTheme:   dto.WebUIExternalThemeConfig{Enabled: true, Provider: "noctalia", Format: "dumber-json", Path: "/tmp/noctalia-dumber.json"},
		},
		DefaultUIScale:      1,
		DefaultSearchEngine: "https://duckduckgo.com/?q=%s",
		SearchShortcuts: map[string]dto.SearchShortcut{
			"ddg": {URL: "https://duckduckgo.com/?q=%s", Description: "DuckDuckGo"},
		},
		Performance: dto.SystemviewPerformancePayload{
			Profile:  "default",
			Custom:   dto.SystemviewCustomPerformancePayload{SkiaCPUThreads: 2, SkiaGPUThreads: 1, WebProcessMemoryMB: 1024, NetworkProcessMemoryMB: 256, WebViewPoolPrewarm: 2},
			Hardware: dto.SystemviewHardwarePayload{CPUCores: 4, CPUThreads: 8, TotalRAMMB: 16384, GPUVendor: "AMD", GPUName: "Radeon", VRAMMB: 4096},
		},
	}
}

func testDefaultConfigPayload() dto.SystemviewConfigPayload {
	cfg := testConfigPayload()
	cfg.Appearance.DefaultFontSize = 14
	cfg.DefaultUIScale = 1
	return cfg
}

// Recording fixture to capture DOM mounts for stateful render assertions.
type recordingDOM struct {
	mountCount   int
	mu           sync.Mutex
	mounted      bool
	html         string
	appendedHTML string
	mounts       chan string
}

func (d *recordingDOM) Mount(markup string) error {
	d.mu.Lock()
	d.mounted = true
	d.html = markup
	d.mountCount++
	mounts := d.mounts
	d.mu.Unlock()
	if mounts != nil {
		select {
		case mounts <- markup:
		default:
		}
	}
	return nil
}

func (d *recordingDOM) Mounted() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.mounted
}

func (d *recordingDOM) HTML() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.html
}

func (d *recordingDOM) AppendedHTML() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.appendedHTML
}

func receiveMount(t *testing.T, mounts <-chan string) string {
	t.Helper()
	select {
	case html := <-mounts:
		return html
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for DOM mount")
		return ""
	}
}

// assertNoMount checks that no mount is queued. Callers reach a sync point
// first (a synchronous call, or synctest.Wait for background work).
func assertNoMount(t *testing.T, mounts <-chan string) {
	t.Helper()
	select {
	case html := <-mounts:
		t.Fatalf("unexpected DOM mount: %s", html)
	default:
	}
}

type recordingActionDOM struct {
	recordingDOM
	handler  DOMActionHandler
	released bool
}

func (d *recordingActionDOM) BindActions(handler DOMActionHandler) error {
	d.handler = handler
	return nil
}

func (d *recordingActionDOM) AppendHistoryTimeline(markup string) error {
	d.mu.Lock()
	d.appendedHTML += markup
	d.mu.Unlock()
	return nil
}

func (d *recordingActionDOM) Release() {
	d.released = true
}

type failingActionDOM struct {
	recordingDOM
	bindErr error
}

func (d *failingActionDOM) BindActions(DOMActionHandler) error {
	return d.bindErr
}

func TestHandleFavoriteActionValidatesCreateURL(t *testing.T) {
	app := NewApp(Dependencies{Favorites: newFavoritesStore(t, &favoritesStore{}).service})

	err := app.handleFavoriteAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "  ", "title": "Blank"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "favorite URL is required")

	err = app.handleFavoriteAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "example.com", "title": "Relative"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "favorite URL must be absolute")

	err = app.handleFavoriteAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "javascript:alert(1)", "title": "Script"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "favorite URL must use http, https, or dumb scheme")
}

func TestHandleFavoriteCreateAcceptsInternalDumbRoutes(t *testing.T) {
	favorites := newFavoritesStore(t, &favoritesStore{})
	favorites.expectCreateFavorite("dumb:history")
	favorites.expectCreateFavorite("dumb://config")
	app := NewApp(Dependencies{Favorites: favorites.service})

	require.NoError(t, app.handleFavoriteAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "dumb:history", "title": "History"},
	}))
	assert.Equal(t, "dumb:history", favorites.createdFavorite.URL)

	require.NoError(t, app.handleFavoriteAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "dumb://config", "title": "Config"},
	}))
	assert.Equal(t, "dumb://config", favorites.createdFavorite.URL)
}

func TestHandleFavoriteCreateRejectsUnknownDumbRoutes(t *testing.T) {
	favorites := newFavoritesStore(t, &favoritesStore{})
	app := NewApp(Dependencies{Favorites: favorites.service})

	err := app.handleFavoriteAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "dumb:unknown", "title": "Unknown"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "known systemview route")
}

func TestParsePositiveInt64DistinguishesParseAndRangeErrors(t *testing.T) {
	_, err := parsePositiveInt64("abc", "tag id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tag id")
	assert.Contains(t, err.Error(), "invalid syntax")

	_, err = parsePositiveInt64("0", "tag id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag id must be a positive integer")
}

func TestHandleHistorySearchClearsDomainFilter(t *testing.T) {
	dom := &recordingDOM{}
	history := newHistoryStore(t, &historyStore{searchEntries: []*entity.HistoryEntry{{ID: 7, URL: "https://search.example", Title: "Search result"}}})
	app := NewApp(Dependencies{DOM: dom, History: history.service, LocationURI: "dumb://history"})
	app.historyDomainFilter = "example.com"

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionSearch,
		Data:   map[string]string{"query": "search"},
	}))

	assert.Equal(t, "search", app.historyQuery)
	assert.Empty(t, app.historyDomainFilter)
	assert.True(t, history.searchCalled)
	// TimelineByDomain has no expectation: search must not filter by domain.
}

func TestAppRunWithContextRejectsNilContext(t *testing.T) {
	app := NewApp(Dependencies{DOM: &recordingDOM{}, LocationURI: "dumb://history"})

	var ctx context.Context
	err := app.RunWithContext(ctx)

	require.ErrorContains(t, err, "context is nil")
}

func TestAppRunWithContextRequiresDOMBeforeLoading(t *testing.T) {
	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}})
	app := NewApp(Dependencies{History: history.service, LocationURI: "dumb://history"})

	err := app.RunWithContext(context.Background())

	require.ErrorContains(t, err, "DOM not configured")
	assert.False(t, history.called)
}

func TestHandleActionsRejectUnknownAction(t *testing.T) {
	app := NewApp(Dependencies{
		History:   newHistoryStore(t, &historyStore{}).service,
		Favorites: newFavoritesStore(t, &favoritesStore{}).service,
		Config:    mocks.NewMockSystemviewConfigService(t),
	})

	require.ErrorContains(t, app.handleHistoryAction(context.Background(), DOMAction{Action: "history.unknown"}), "unknown history action")
	require.ErrorContains(t, app.handleFavoriteAction(context.Background(), DOMAction{Action: "favorite.unknown"}), "unknown favorite action")
	require.ErrorContains(t, app.handleConfigAction(context.Background(), DOMAction{Action: "config.unknown"}), "unknown config action")
}

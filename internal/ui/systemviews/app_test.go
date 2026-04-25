package systemviews

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
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

	dom := &fakeDOM{}
	app := NewApp(Dependencies{
		DOM:         dom,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.Run())
	assert.Equal(t, RouteHistory, app.CurrentRoute())
	assert.True(t, dom.mounted)
	assert.Contains(t, dom.html, "history")
	assert.Contains(t, dom.html, "systemviews")
}

func TestAppLoadInitialHistoryRouteUsesStyledSections(t *testing.T) {
	t.Parallel()

	history := &fakeHistoryService{entries: []*entity.HistoryEntry{{
		URL:   "https://example.com",
		Title: "Example",
	}}}

	app := NewApp(Dependencies{
		History:     history,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Equal(t, RouteHistory, app.CurrentRoute())
	assert.Len(t, app.historyEntries, 1)
	assert.Contains(t, app.renderedHTML, "Example")
	assert.Contains(t, app.renderedHTML, "https://example.com")
	assert.True(t, history.called)
	assert.False(t, history.analyticsCalled)
	assert.Equal(t, 0, history.limit)
	assert.Equal(t, 0, history.offset)

	// Shell frame present, no full document wrapper.
	assert.NotContains(t, app.renderedHTML, "<html")
	assert.NotContains(t, app.renderedHTML, "<head")
	assert.NotContains(t, app.renderedHTML, "<body")
	assert.Contains(t, app.renderedHTML, `class="sv-shell"`)
	assert.Contains(t, app.renderedHTML, `data-route="history"`)
	assert.Contains(t, app.renderedHTML, `data-page-title="History — 1 entry"`)
	assert.Contains(t, app.renderedHTML, `sv-section`)
	assert.Contains(t, app.renderedHTML, `class="sv-list"`)
}

func TestAppLoadInitialHistoryRouteShowsTotalStatsWithWindowedTimeline(t *testing.T) {
	t.Parallel()

	history := &fakeHistoryService{
		entries: []*entity.HistoryEntry{
			{ID: 1, URL: "https://older.example", Title: "Older", VisitCount: 2},
			{ID: 2, URL: "https://newer.example", Title: "Newer", VisitCount: 3},
		},
		stats: &entity.HistoryStats{TotalEntries: 8904, TotalVisits: 40947, UniqueDays: 121},
	}
	app := NewApp(Dependencies{History: history, LocationURI: "dumb://history"})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.True(t, history.statsCalled)
	assert.Contains(t, app.renderedHTML, "Loaded 2 items")
	assert.Contains(t, app.renderedHTML, `data-page-title="History — 8904 entries"`)
	assert.Contains(t, app.renderedHTML, `<span class="sv-stat-value">8904</span><span class="sv-stat-label">Entries</span>`)
	assert.Contains(t, app.renderedHTML, `<span class="sv-stat-value">40947</span><span class="sv-stat-label">Visits</span>`)
	assert.Contains(t, app.renderedHTML, `<span class="sv-stat-value">121</span><span class="sv-stat-label">Days</span>`)
}

func TestAppRunMountsHistoryLoadingBeforeAsyncHydration(t *testing.T) {
	t.Parallel()

	dom := &fakeActionDOM{fakeDOM: fakeDOM{mounts: make(chan string, 4)}}
	history := &fakeHistoryService{
		entries:         []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Loaded entry"}},
		timelineStarted: make(chan struct{}),
		releaseTimeline: make(chan struct{}),
	}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})

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
	assert.Contains(t, hydratedHTML, "Loaded 1 item")
}

func TestAppHandleHistoryLoadMoreAppendsOlderWindow(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	history := &fakeHistoryService{window: &entity.HistoryWindow{
		Entries: []*entity.HistoryEntry{{ID: 2, URL: "https://older.example", Title: "Older entry", LastVisited: cursor.Add(-time.Hour)}},
		Before:  cursor,
		After:   cursor.Add(-24 * time.Hour),
		HasMore: true,
	}}
	dom := &fakeActionDOM{}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	app.historyEntries = []*entity.HistoryEntry{{ID: 1, URL: "https://newer.example", Title: "Newer entry", LastVisited: cursor}}
	app.historyWindowAfter = cursor
	app.historyHasMore = true

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionLoadMore,
		Data:   map[string]string{"before": cursor.Format(time.RFC3339Nano)},
	}))
	assert.Equal(t, cursor, history.windowBefore)
	assert.Len(t, app.historyEntries, 2)
	assert.Contains(t, dom.appendedHTML, "Older entry")
	assert.Contains(t, dom.appendedHTML, `data-sv-action="history.loadMore"`)
	assert.Empty(t, dom.html, "load-more should append a fragment instead of remounting the whole page")
}

func TestAppHandleHistoryLoadMoreRemountsWhenAppendUnavailable(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	history := &fakeHistoryService{window: &entity.HistoryWindow{
		Entries: []*entity.HistoryEntry{{ID: 2, URL: "https://older.example", Title: "Older entry", LastVisited: cursor.Add(-time.Hour)}},
		Before:  cursor,
		After:   cursor.Add(-24 * time.Hour),
		HasMore: true,
	}}
	dom := &fakeDOM{}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	app.historyEntries = []*entity.HistoryEntry{{ID: 1, URL: "https://newer.example", Title: "Newer entry", LastVisited: cursor}}
	app.historyWindowAfter = cursor
	app.historyHasMore = true

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionLoadMore,
		Data:   map[string]string{"before": cursor.Format(time.RFC3339Nano)},
	}))
	assert.Equal(t, cursor, history.windowBefore)
	assert.Len(t, app.historyEntries, 2)
	assert.Contains(t, dom.html, "Older entry")
	assert.Contains(t, dom.html, `data-sv-action="history.loadMore"`)
}

func TestAppHandleHistoryLoadMoreIgnoresStaleCursor(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	history := &fakeHistoryService{window: &entity.HistoryWindow{Entries: []*entity.HistoryEntry{{ID: 2, URL: "https://older.example"}}}}
	dom := &fakeActionDOM{}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})
	app.currentRoute = RouteHistory
	app.historyWindowAfter = cursor
	app.historyHasMore = true

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionLoadMore,
		Data:   map[string]string{"before": cursor.Add(-24 * time.Hour).Format(time.RFC3339Nano)},
	}))
	assert.False(t, history.called)
	assert.Empty(t, dom.appendedHTML)
}

func TestAppLoadInitialHistoryRouteRendersManagementActions(t *testing.T) {
	t.Parallel()

	history := &fakeHistoryService{
		entries: []*entity.HistoryEntry{{
			ID:    42,
			URL:   "https://example.com/page",
			Title: "Example",
		}},
		domainStats: []*entity.DomainStat{{Domain: "www.example.com:8080", PageCount: 1, TotalVisits: 3}},
	}

	app := NewApp(Dependencies{
		History:     history,
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
	assert.Contains(t, app.renderedHTML, "Keys:")
	assert.Contains(t, app.renderedHTML, "Enter")
}

func TestAppHandleHistoryActionsRefreshesDOM(t *testing.T) {
	dom := &fakeDOM{}
	history := &fakeHistoryService{
		entries:       []*entity.HistoryEntry{{ID: 42, URL: "https://example.com", Title: "Example"}},
		searchEntries: []*entity.HistoryEntry{{ID: 7, URL: "https://search.example", Title: "Search result"}},
	}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionSearch,
		Data:   map[string]string{"query": " example "},
	}))
	assert.True(t, history.searchCalled)
	assert.Equal(t, "example", history.query)
	assert.Contains(t, dom.html, "Search result")
	assert.Contains(t, dom.html, "Query: example")

	app.historyOffset = historyTimelineLimit
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteEntry,
		Data:   map[string]string{"id": "42"},
	}))
	assert.Equal(t, int64(42), history.deletedEntryID)
	assert.Equal(t, 0, app.historyOffset)
	assert.Contains(t, dom.html, "Deleted history entry")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteRange,
		Data:   map[string]string{"range": "week"},
	}))
	assert.Equal(t, "week", history.deletedRangeID)
	assert.Contains(t, dom.html, "Deleted history from this week")

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
	assert.Equal(t, "example.com", history.deletedDomain)
	assert.Contains(t, dom.html, "Deleted history for example.com")
}

func TestAppCloseStopsActionWorkerAndReleasesDOM(t *testing.T) {
	dom := &fakeActionDOM{}
	history := &fakeHistoryService{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	dom := &fakeActionDOM{}
	history := &fakeHistoryService{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})

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

	history := &fakeHistoryService{err: errors.New("database unavailable")}
	app := NewApp(Dependencies{
		History:     history,
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

	history := &fakeHistoryService{entries: []*entity.HistoryEntry{{
		URL:   "https://example.com",
		Title: "Example",
	}}}
	config := &fakeConfigService{current: dto.SystemviewConfigPayload{
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
	}}

	app := NewApp(Dependencies{
		Config:      config,
		History:     history,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, `class="sv-app sv-light"`)
	assert.Contains(t, app.renderedHTML, `--sv-background: #ffffff;`)
	assert.Contains(t, app.renderedHTML, `--sv-surface-variant: #eeeeee;`)
}

func TestAppLoadInitialFavoritesRouteRendersData(t *testing.T) {
	t.Parallel()

	favorites := &fakeFavoritesService{
		favorites: []*entity.Favorite{{URL: "https://example.com", Title: "Example"}},
		folders:   []*entity.Folder{{Name: "Read Later"}},
		tags:      []*entity.Tag{{Name: "Go"}},
	}

	app := NewApp(Dependencies{
		Favorites:   favorites,
		LocationURI: "dumb://favorites",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Equal(t, RouteFavorites, app.CurrentRoute())
	assert.Len(t, app.favorites, 1)
	assert.Len(t, app.folders, 1)
	assert.Len(t, app.tags, 1)
	assert.Contains(t, app.renderedHTML, "Favorites")
	assert.Contains(t, app.renderedHTML, "Example")
	assert.Contains(t, app.renderedHTML, "Read Later")
	assert.Contains(t, app.renderedHTML, "Go")
	assert.True(t, favorites.calledList)
	assert.True(t, favorites.calledFolders)
	assert.True(t, favorites.calledTags)

	// Shell frame present, no full document wrapper.
	assert.NotContains(t, app.renderedHTML, "<html")
	assert.NotContains(t, app.renderedHTML, "<head")
	assert.NotContains(t, app.renderedHTML, "<body")
	assert.Contains(t, app.renderedHTML, `class="sv-shell"`)
	assert.Contains(t, app.renderedHTML, `data-route="favorites"`)
	assert.Contains(t, app.renderedHTML, `data-page-title="Favorites — 1 bookmark"`)
	assert.Contains(t, app.renderedHTML, `sv-section`)
	assert.Contains(t, app.renderedHTML, `class="sv-meta"`)
}

func TestAppLoadInitialFavoritesRouteRendersCRUDControls(t *testing.T) {
	t.Parallel()

	folderID := entity.FolderID(1)
	shortcut := 3
	favorites := &fakeFavoritesService{
		favorites: []*entity.Favorite{{ID: 42, URL: "https://example.com", Title: "Example", FolderID: &folderID, ShortcutKey: &shortcut}},
		folders:   []*entity.Folder{{ID: folderID, Name: "Read Later", Icon: "📚"}},
		tags:      []*entity.Tag{{ID: 7, Name: "Go", Color: "#00add8"}},
	}

	app := NewApp(Dependencies{Favorites: favorites, LocationURI: "dumb://favorites"})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, `data-sv-action="favorite.create"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="favorite.update"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="favorite.delete"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="folder.create"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="folder.update"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="folder.delete"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="tag.create"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="tag.update"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="tag.delete"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="tag.assign"`)
	assert.Contains(t, app.renderedHTML, `name="tags"`)
	assert.Contains(t, app.renderedHTML, `value="7"`)
	assert.Contains(t, app.renderedHTML, "Read Later")
	assert.Contains(t, app.renderedHTML, "Go")
	assert.Contains(t, app.renderedHTML, "Shortcut 3")
}

func TestAppHandleFavoriteActionsRefreshesDOM(t *testing.T) {
	dom := &fakeDOM{}
	favorites := &fakeFavoritesService{
		favorites: []*entity.Favorite{{ID: 42, URL: "https://example.com", Title: "Example"}},
		folders:   []*entity.Folder{{ID: 5, Name: "Read Later"}},
		tags:      []*entity.Tag{{ID: 7, Name: "Go", Color: "#00add8"}},
	}
	app := NewApp(Dependencies{DOM: dom, Favorites: favorites, LocationURI: "dumb://favorites"})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "https://new.example", "title": "New", "folder_id": "5", "tags": "7"},
	}))
	require.NotNil(t, favorites.createdFavorite.FolderID)
	assert.Equal(t, "https://new.example", favorites.createdFavorite.URL)
	assert.Equal(t, entity.FolderID(5), *favorites.createdFavorite.FolderID)
	assert.Equal(t, []entity.TagID{7}, favorites.createdFavorite.Tags)
	assert.Contains(t, dom.html, "Added favorite New")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionUpdate,
		Data:   map[string]string{"id": "42", "title": "Updated", "favicon_url": "https://icon.example/favicon.ico", "shortcut_key": "4"},
	}))
	assert.Equal(t, entity.FavoriteID(42), favorites.updatedFavorite.ID)
	assert.Equal(t, "Updated", favorites.updatedFavorite.Title)
	require.NotNil(t, favorites.updatedFavorite.ShortcutKey)
	assert.Equal(t, 4, *favorites.updatedFavorite.ShortcutKey)
	assert.Contains(t, dom.html, "Saved favorite Updated")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: tagActionAssign,
		Data:   map[string]string{"favoriteId": "42", "tagId": "7"},
	}))
	assert.Equal(t, int64(42), favorites.assignedFavoriteID)
	assert.Equal(t, int64(7), favorites.assignedTagID)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionFilterFolder,
		Data:   map[string]string{"folderId": "5"},
	}))
	require.NotNil(t, app.favoriteFolderFilter)
	assert.Equal(t, entity.FolderID(5), *app.favoriteFolderFilter)
	assert.Nil(t, app.favoriteTagFilter)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionFilterTag,
		Data:   map[string]string{"tagId": "7"},
	}))
	require.NotNil(t, app.favoriteTagFilter)
	assert.Equal(t, entity.TagID(7), *app.favoriteTagFilter)
	assert.Nil(t, app.favoriteFolderFilter)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: folderActionCreate,
		Data:   map[string]string{"name": "Reading", "icon": "📚"},
	}))
	assert.Equal(t, "Reading", favorites.createdFolder)
	assert.Equal(t, "📚", favorites.createdFolderIcon)
	assert.Contains(t, dom.html, "Created folder 📚 Reading")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionDelete,
		Data:   map[string]string{"id": "42"},
	}))
	assert.Equal(t, int64(42), favorites.deletedFavorite)
	assert.Contains(t, dom.html, "Deleted favorite")
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
	service := &fakeConfigService{
		current: config,
		keybindings: port.KeybindingsConfig{Groups: []port.KeybindingGroup{
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
		}},
	}

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
	assert.True(t, service.calledCurrent)
	assert.True(t, service.calledKeybindings)

	// Shell frame present, no full document wrapper.
	assert.NotContains(t, app.renderedHTML, "<html")
	assert.NotContains(t, app.renderedHTML, "<head")
	assert.NotContains(t, app.renderedHTML, "<body")
	assert.Contains(t, app.renderedHTML, `class="sv-shell"`)
	assert.Contains(t, app.renderedHTML, `data-route="config"`)
	assert.Contains(t, app.renderedHTML, `data-page-title="Config — Dumber"`)
	assert.Contains(t, app.renderedHTML, `sv-section`)
	assert.Contains(t, app.renderedHTML, `class="sv-meta"`)
}

func TestAppLoadInitialConfigRouteRendersEditControls(t *testing.T) {
	t.Parallel()

	service := &fakeConfigService{
		current: testConfigPayload(),
		keybindings: port.KeybindingsConfig{Groups: []port.KeybindingGroup{{
			Mode:        "default",
			DisplayName: "Default",
			Bindings: []port.KeybindingEntry{{
				Action:      "toggle_history_systemview",
				Description: "Toggle history",
				Keys:        []string{"ctrl+h"},
				DefaultKeys: []string{"ctrl+h"},
			}},
		}}},
	}
	app := NewApp(Dependencies{Config: service, LocationURI: "dumb://config"})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.appearance.save"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.appearance.reset"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.search.save"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.searchShortcut.create"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.searchShortcut.update"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.searchShortcut.delete"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.performance.save"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.keybinding.set"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.keybinding.reset"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="config.keybinding.resetAll"`)
	assert.Contains(t, app.renderedHTML, "may require restarting")
}

func TestAppHandleConfigActionsRefreshesDOM(t *testing.T) {
	dom := &fakeDOM{}
	service := &fakeConfigService{
		current:    testConfigPayload(),
		defaultCfg: testDefaultConfigPayload(),
		keybindings: port.KeybindingsConfig{Groups: []port.KeybindingGroup{{
			Mode:        "default",
			DisplayName: "Default",
			Bindings: []port.KeybindingEntry{{
				Action:      "toggle_history_systemview",
				Description: "Toggle history",
				Keys:        []string{"ctrl+h"},
				DefaultKeys: []string{"ctrl+h"},
			}},
		}}},
		setResp: port.SetKeybindingResponse{},
	}
	app := NewApp(Dependencies{DOM: dom, Config: service, LocationURI: "dumb://config"})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: configActionSaveAppearance,
		Data: map[string]string{
			"sans_font": "Inter", "serif_font": "Georgia", "monospace_font": "JetBrains Mono",
			"default_font_size": "18", "default_ui_scale": "1.25", "color_scheme": "prefer-light",
			"light_background": "#ffffff", "light_surface": "#fafafa", "light_surface_variant": "#eeeeee", "light_text": "#111111", "light_muted": "#666666", "light_accent": "#0055ff", "light_border": "#dddddd",
			"dark_background": "#111111", "dark_surface": "#1a1a1a", "dark_surface_variant": "#2a2a2a", "dark_text": "#f5f5f5", "dark_muted": "#a0a0a0", "dark_accent": "#66aaff", "dark_border": "#333333",
		},
	}))
	assert.True(t, service.calledSave)
	assert.Equal(t, 18, service.savedConfig.Appearance.DefaultFontSize)
	assert.Equal(t, 1.25, service.savedConfig.DefaultUIScale)
	assert.Contains(t, dom.html, "Saved appearance settings")
	assert.Contains(t, dom.html, `class="sv-app sv-light"`)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: configActionCreateSearchShortcut,
		Data:   map[string]string{"key": "g", "url": "https://google.com/search?q=%s", "description": "Google"},
	}))
	assert.Equal(t, "Google", service.savedConfig.SearchShortcuts["g"].Description)
	assert.Contains(t, dom.html, "Created search shortcut g")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: configActionSavePerformance,
		Data: map[string]string{
			"profile": "custom", "skia_cpu_threads": "4", "skia_gpu_threads": "2",
			"web_process_memory_mb": "2048", "network_process_memory_mb": "512", "webview_pool_prewarm": "4",
		},
	}))
	assert.Equal(t, "custom", service.savedConfig.Performance.Profile)
	assert.Equal(t, 2048, service.savedConfig.Performance.Custom.WebProcessMemoryMB)
	assert.Contains(t, dom.html, "Saved performance settings")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: configActionSavePerformance,
		Data:   map[string]string{"profile": "default"},
	}))
	assert.Equal(t, "default", service.savedConfig.Performance.Profile)
	assert.Contains(t, dom.html, "Saved performance settings")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: configActionSetKeybinding,
		Data:   map[string]string{"mode": "default", "action": "toggle_history_systemview", "keys": "ctrl+h, alt+h"},
	}))
	assert.True(t, service.calledSet)
	assert.Equal(t, []string{"ctrl+h", "alt+h"}, service.setReq.Keys)
	assert.Contains(t, service.setReq.RequestID, "systemviews-config-")
	assert.Contains(t, dom.html, "Saved keybinding toggle_history_systemview")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: configActionResetKeybinding,
		Data:   map[string]string{"mode": "default", "action": "toggle_history_systemview"},
	}))
	assert.True(t, service.calledReset)
	assert.Equal(t, "toggle_history_systemview", service.resetReq.Action)
	assert.Contains(t, service.resetReq.RequestID, "systemviews-config-")
	assert.NotEqual(t, service.setReq.RequestID, service.resetReq.RequestID)
	assert.Contains(t, dom.html, "Reset keybinding toggle_history_systemview")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: configActionResetAllKeybindings}))
	assert.True(t, service.calledResetAll)
	assert.Contains(t, dom.html, "Reset all keybindings to defaults")
}

func TestAppRejectsSearchURLsWithoutPlaceholder(t *testing.T) {
	t.Parallel()

	service := &fakeConfigService{current: testConfigPayload(), defaultCfg: testDefaultConfigPayload()}
	app := NewApp(Dependencies{Config: service, LocationURI: "dumb://config"})

	err := app.handleConfigAction(context.Background(), DOMAction{
		Action: configActionSaveSearch,
		Data:   map[string]string{"default_search_engine": "https://example.com/search"},
	})
	require.Error(t, err)
	assert.False(t, service.calledSave)

	err = app.handleConfigAction(context.Background(), DOMAction{
		Action: configActionCreateSearchShortcut,
		Data:   map[string]string{"key": "bad", "url": "https://example.com/search", "description": "Bad"},
	})
	require.Error(t, err)
	assert.False(t, service.calledSave)
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

// Handwritten fake to capture DOM mounts for stateful render assertions.
type fakeDOM struct {
	mu           sync.Mutex
	mounted      bool
	html         string
	appendedHTML string
	mounts       chan string
}

func (d *fakeDOM) Mount(markup string) error {
	d.mu.Lock()
	d.mounted = true
	d.html = markup
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

type fakeActionDOM struct {
	fakeDOM
	handler  DOMActionHandler
	released bool
}

func (d *fakeActionDOM) BindActions(handler DOMActionHandler) error {
	d.handler = handler
	return nil
}

func (d *fakeActionDOM) AppendHistoryTimeline(markup string) error {
	d.mu.Lock()
	d.appendedHTML += markup
	d.mu.Unlock()
	return nil
}

func (d *fakeActionDOM) Release() {
	d.released = true
}

type failingActionDOM struct {
	fakeDOM
	bindErr error
}

func (d *failingActionDOM) BindActions(DOMActionHandler) error {
	return d.bindErr
}

// Handwritten fake to capture history state for stateful render assertions.
type fakeHistoryService struct {
	called       bool
	limit        int
	offset       int
	entries      []*entity.HistoryEntry
	err          error
	domainCalled bool
	domain       string

	searchCalled  bool
	query         string
	searchLimit   int
	searchEntries []*entity.HistoryEntry

	deletedEntryID  int64
	deletedRangeID  string
	deletedDomain   string
	domainStats     []*entity.DomainStat
	stats           *entity.HistoryStats
	statsCalled     bool
	window          *entity.HistoryWindow
	windowBefore    time.Time
	windowDomain    string
	analyticsCalled bool

	timelineStarted chan struct{}
	releaseTimeline chan struct{}
	startOnce       sync.Once
}

func (s *fakeHistoryService) Timeline(_ context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	s.called = true
	s.limit = limit
	s.offset = offset
	if s.timelineStarted != nil {
		s.startOnce.Do(func() { close(s.timelineStarted) })
	}
	if s.releaseTimeline != nil {
		<-s.releaseTimeline
	}
	return s.entries, s.err
}

func (s *fakeHistoryService) TimelineByDomain(_ context.Context, domain string, limit, offset int) ([]*entity.HistoryEntry, error) {
	s.domainCalled = true
	s.domain = domain
	s.limit = limit
	s.offset = offset
	return s.entries, s.err
}

func (s *fakeHistoryService) TimelineWindow(_ context.Context, before time.Time, domain string) (*entity.HistoryWindow, error) {
	s.called = true
	s.windowBefore = before
	s.windowDomain = domain
	if s.timelineStarted != nil {
		s.startOnce.Do(func() { close(s.timelineStarted) })
	}
	if s.releaseTimeline != nil {
		<-s.releaseTimeline
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.window != nil {
		return s.window, nil
	}
	return &entity.HistoryWindow{Entries: s.entries, Before: before, After: before.Add(-24 * time.Hour)}, nil
}

func (s *fakeHistoryService) Search(_ context.Context, query string, limit int) ([]*entity.HistoryEntry, error) {
	s.searchCalled = true
	s.query = query
	s.searchLimit = limit
	if s.searchEntries != nil {
		return s.searchEntries, nil
	}
	return s.entries, nil
}

func (s *fakeHistoryService) DeleteEntry(_ context.Context, id int64) error {
	s.deletedEntryID = id
	return nil
}

func (s *fakeHistoryService) DeleteRange(_ context.Context, rangeID string) error {
	s.deletedRangeID = rangeID
	return nil
}

func (s *fakeHistoryService) Stats(context.Context) (*entity.HistoryStats, error) {
	s.statsCalled = true
	return s.stats, nil
}

func (s *fakeHistoryService) Analytics(context.Context) (*entity.HistoryAnalytics, error) {
	s.analyticsCalled = true
	return nil, nil
}

func (s *fakeHistoryService) DomainStats(context.Context, int) ([]*entity.DomainStat, error) {
	return s.domainStats, nil
}

func (s *fakeHistoryService) DeleteDomain(_ context.Context, domain string) error {
	s.deletedDomain = domain
	return nil
}

// Handwritten fake to capture favorites state for stateful render assertions.
type fakeFavoritesService struct {
	calledList    bool
	calledFolders bool
	calledTags    bool
	favorites     []*entity.Favorite
	folders       []*entity.Folder
	tags          []*entity.Tag

	createdFavorite    port.FavoriteCreateInput
	updatedFavorite    port.FavoriteUpdateInput
	deletedFavorite    int64
	createdFolder      string
	createdFolderIcon  string
	updatedFolderID    int64
	deletedFolderID    int64
	createdTag         string
	updatedTagID       int64
	deletedTagID       int64
	assignedFavoriteID int64
	assignedTagID      int64
	removedFavoriteID  int64
	removedTagID       int64
}

func (s *fakeFavoritesService) List(context.Context) ([]*entity.Favorite, error) {
	s.calledList = true
	return s.favorites, nil
}

func (s *fakeFavoritesService) ListFolders(context.Context) ([]*entity.Folder, error) {
	s.calledFolders = true
	return s.folders, nil
}

func (s *fakeFavoritesService) ListTags(context.Context) ([]*entity.Tag, error) {
	s.calledTags = true
	return s.tags, nil
}

func (s *fakeFavoritesService) CreateFavorite(_ context.Context, input port.FavoriteCreateInput) (*entity.Favorite, error) {
	s.createdFavorite = input
	return &entity.Favorite{ID: 99, URL: input.URL, Title: input.Title, FolderID: input.FolderID}, nil
}

func (s *fakeFavoritesService) UpdateFavorite(_ context.Context, input port.FavoriteUpdateInput) (*entity.Favorite, error) {
	s.updatedFavorite = input
	return &entity.Favorite{ID: input.ID, URL: "https://example.com", Title: input.Title, FaviconURL: input.FaviconURL, FolderID: input.FolderID, ShortcutKey: input.ShortcutKey}, nil
}

func (s *fakeFavoritesService) DeleteFavorite(_ context.Context, id int64) error {
	s.deletedFavorite = id
	return nil
}

func (s *fakeFavoritesService) SetShortcut(context.Context, int64, *int) error { return nil }

func (s *fakeFavoritesService) SetFolder(context.Context, int64, *int64) error { return nil }

func (s *fakeFavoritesService) CreateFolder(_ context.Context, name, icon string, _ *int64) (*entity.Folder, error) {
	s.createdFolder = name
	s.createdFolderIcon = icon
	return &entity.Folder{ID: 77, Name: name, Icon: icon}, nil
}

func (s *fakeFavoritesService) UpdateFolder(_ context.Context, id int64, _, _ string) error {
	s.updatedFolderID = id
	return nil
}

func (s *fakeFavoritesService) DeleteFolder(_ context.Context, id int64) error {
	s.deletedFolderID = id
	return nil
}

func (s *fakeFavoritesService) CreateTag(_ context.Context, name, color string) (*entity.Tag, error) {
	s.createdTag = name
	return &entity.Tag{ID: 55, Name: name, Color: color}, nil
}

func (s *fakeFavoritesService) UpdateTag(_ context.Context, id int64, _, _ string) error {
	s.updatedTagID = id
	return nil
}

func (s *fakeFavoritesService) DeleteTag(_ context.Context, id int64) error {
	s.deletedTagID = id
	return nil
}

func (s *fakeFavoritesService) AssignTag(_ context.Context, favoriteID, tagID int64) error {
	s.assignedFavoriteID = favoriteID
	s.assignedTagID = tagID
	return nil
}

func (s *fakeFavoritesService) RemoveTag(_ context.Context, favoriteID, tagID int64) error {
	s.removedFavoriteID = favoriteID
	s.removedTagID = tagID
	return nil
}

// Handwritten fake to capture config state for stateful render assertions.
type fakeConfigService struct {
	calledCurrent     bool
	calledDefault     bool
	calledSave        bool
	calledKeybindings bool
	calledSet         bool
	calledReset       bool
	calledResetAll    bool

	current     dto.SystemviewConfigPayload
	defaultCfg  dto.SystemviewConfigPayload
	keybindings port.KeybindingsConfig
	savedConfig dto.WebUIConfig
	setReq      port.SetKeybindingRequest
	setResp     port.SetKeybindingResponse
	resetReq    port.ResetKeybindingRequest
}

func (s *fakeConfigService) Current(context.Context) (dto.SystemviewConfigPayload, error) {
	s.calledCurrent = true
	return s.current, nil
}

func (s *fakeConfigService) Default(context.Context) (dto.SystemviewConfigPayload, error) {
	s.calledDefault = true
	return s.defaultCfg, nil
}

func (s *fakeConfigService) Save(_ context.Context, cfg dto.WebUIConfig) error {
	s.calledSave = true
	s.savedConfig = cfg
	s.current.Appearance = cfg.Appearance
	s.current.DefaultUIScale = cfg.DefaultUIScale
	s.current.DefaultSearchEngine = cfg.DefaultSearchEngine
	s.current.SearchShortcuts = cfg.SearchShortcuts
	s.current.Performance.Profile = cfg.Performance.Profile
	s.current.Performance.Custom = dto.SystemviewCustomPerformancePayload{
		SkiaCPUThreads:         cfg.Performance.Custom.SkiaCPUThreads,
		SkiaGPUThreads:         cfg.Performance.Custom.SkiaGPUThreads,
		WebProcessMemoryMB:     cfg.Performance.Custom.WebProcessMemoryMB,
		NetworkProcessMemoryMB: cfg.Performance.Custom.NetworkProcessMemoryMB,
		WebViewPoolPrewarm:     cfg.Performance.Custom.WebViewPoolPrewarm,
	}
	return nil
}

func (s *fakeConfigService) GetKeybindings(context.Context) (port.KeybindingsConfig, error) {
	s.calledKeybindings = true
	return s.keybindings, nil
}

func (s *fakeConfigService) SetKeybinding(_ context.Context, req port.SetKeybindingRequest) (port.SetKeybindingResponse, error) {
	s.calledSet = true
	s.setReq = req
	return s.setResp, nil
}

func (s *fakeConfigService) ResetKeybinding(_ context.Context, req port.ResetKeybindingRequest) error {
	s.calledReset = true
	s.resetReq = req
	return nil
}

func (s *fakeConfigService) ResetAllKeybindings(context.Context) error {
	s.calledResetAll = true
	return nil
}

func TestHandleFavoriteActionValidatesCreateURL(t *testing.T) {
	app := NewApp(Dependencies{Favorites: &fakeFavoritesService{}})

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
	favorites := &fakeFavoritesService{}
	app := NewApp(Dependencies{Favorites: favorites})

	require.NoError(t, app.handleFavoriteAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "dumb:history", "title": "History"},
	}))
	assert.Equal(t, "dumb://history", favorites.createdFavorite.URL)

	require.NoError(t, app.handleFavoriteAction(context.Background(), DOMAction{
		Action: favoriteActionCreate,
		Data:   map[string]string{"url": "dumb://config", "title": "Config"},
	}))
	assert.Equal(t, "dumb://config", favorites.createdFavorite.URL)
}

func TestHandleFavoriteTagActionsAcceptSnakeCaseIDs(t *testing.T) {
	favorites := &fakeFavoritesService{}
	app := NewApp(Dependencies{Favorites: favorites})

	require.NoError(t, app.handleFavoriteAction(context.Background(), DOMAction{
		Action: tagActionAssign,
		Data:   map[string]string{"favorite_id": "42", "tag_id": "7"},
	}))
	assert.Equal(t, int64(42), favorites.assignedFavoriteID)
	assert.Equal(t, int64(7), favorites.assignedTagID)

	require.NoError(t, app.handleFavoriteAction(context.Background(), DOMAction{
		Action: tagActionRemove,
		Data:   map[string]string{"favorite_id": "42", "tag_id": "7"},
	}))
	assert.Equal(t, int64(42), favorites.removedFavoriteID)
	assert.Equal(t, int64(7), favorites.removedTagID)
}

func TestHandleHistorySearchClearsDomainFilter(t *testing.T) {
	dom := &fakeDOM{}
	history := &fakeHistoryService{searchEntries: []*entity.HistoryEntry{{ID: 7, URL: "https://search.example", Title: "Search result"}}}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})
	app.historyDomainFilter = "example.com"

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionSearch,
		Data:   map[string]string{"query": "search"},
	}))

	assert.Equal(t, "search", app.historyQuery)
	assert.Empty(t, app.historyDomainFilter)
	assert.True(t, history.searchCalled)
	assert.False(t, history.domainCalled)
}

func TestAppRunWithContextRejectsNilContext(t *testing.T) {
	app := NewApp(Dependencies{DOM: &fakeDOM{}, LocationURI: "dumb://history"})

	err := app.RunWithContext(nil)

	require.ErrorContains(t, err, "context is nil")
}

func TestAppRunWithContextRequiresDOMBeforeLoading(t *testing.T) {
	history := &fakeHistoryService{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}}
	app := NewApp(Dependencies{History: history, LocationURI: "dumb://history"})

	err := app.RunWithContext(context.Background())

	require.ErrorContains(t, err, "DOM not configured")
	assert.False(t, history.called)
}

func TestHandleActionsRejectUnknownAction(t *testing.T) {
	app := NewApp(Dependencies{
		History:   &fakeHistoryService{},
		Favorites: &fakeFavoritesService{},
		Config:    &fakeConfigService{current: testConfigPayload()},
	})

	require.ErrorContains(t, app.handleHistoryAction(context.Background(), DOMAction{Action: "history.unknown"}), "unknown history action")
	require.ErrorContains(t, app.handleFavoriteAction(context.Background(), DOMAction{Action: "favorite.unknown"}), "unknown favorite action")
	require.ErrorContains(t, app.handleConfigAction(context.Background(), DOMAction{Action: "config.unknown"}), "unknown config action")
}

func TestSavePerformanceConfigRejectsOutOfRangeValues(t *testing.T) {
	service := &fakeConfigService{current: testConfigPayload()}
	app := NewApp(Dependencies{Config: service})

	err := app.handleConfigAction(context.Background(), DOMAction{
		Action: configActionSavePerformance,
		Data: map[string]string{
			"profile": "custom", "skia_cpu_threads": "65", "skia_gpu_threads": "2",
			"web_process_memory_mb": "2048", "network_process_memory_mb": "512", "webview_pool_prewarm": "4",
		},
	})

	require.ErrorContains(t, err, "Skia CPU threads must be between 0 and 8")
	assert.False(t, service.calledSave)
}

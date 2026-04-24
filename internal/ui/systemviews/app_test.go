package systemviews

import (
	"context"
	"errors"
	"testing"

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
	assert.Equal(t, 25, history.limit)
	assert.Equal(t, 0, history.offset)

	// Shell frame present, no full document wrapper.
	assert.NotContains(t, app.renderedHTML, "<html")
	assert.NotContains(t, app.renderedHTML, "<head")
	assert.NotContains(t, app.renderedHTML, "<body")
	assert.Contains(t, app.renderedHTML, `class="sv-shell"`)
	assert.Contains(t, app.renderedHTML, `data-route="history"`)
	assert.Contains(t, app.renderedHTML, `sv-section`)
	assert.Contains(t, app.renderedHTML, `class="sv-list"`)
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
	assert.Contains(t, app.renderedHTML, `data-domain="www.example.com:8080"`)
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

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteEntry,
		Data:   map[string]string{"id": "42"},
	}))
	assert.Equal(t, int64(42), history.deletedEntryID)
	assert.Contains(t, dom.html, "Deleted history entry")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteRange,
		Data:   map[string]string{"range": "week"},
	}))
	assert.Equal(t, "week", history.deletedRangeID)
	assert.Contains(t, dom.html, "Deleted history from this week")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteDomain,
		Data:   map[string]string{"domain": "example.com"},
	}))
	assert.Equal(t, "example.com", history.deletedDomain)
	assert.Contains(t, dom.html, "Deleted history for example.com")
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
	config := &fakeConfigService{current: port.SystemviewConfigPayload{
		Appearance: port.WebUIAppearanceConfig{
			ColorScheme: "prefer-light",
			LightPalette: port.ColorPalette{
				Background:     "#ffffff",
				Surface:        "#fafafa",
				SurfaceVariant: "#eeeeee",
				Text:           "#111111",
				Muted:          "#666666",
				Accent:         "#0055ff",
				Border:         "#dddddd",
			},
			DarkPalette: port.ColorPalette{
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
		Data:   map[string]string{"url": "https://new.example", "title": "New", "folder_id": "5"},
	}))
	require.NotNil(t, favorites.createdFavorite.FolderID)
	assert.Equal(t, "https://new.example", favorites.createdFavorite.URL)
	assert.Equal(t, entity.FolderID(5), *favorites.createdFavorite.FolderID)
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
		Action: favoriteActionDelete,
		Data:   map[string]string{"id": "42"},
	}))
	assert.Equal(t, int64(42), favorites.deletedFavorite)
	assert.Contains(t, dom.html, "Deleted favorite")
}

func TestAppLoadInitialConfigRouteRendersData(t *testing.T) {
	t.Parallel()

	config := port.SystemviewConfigPayload{
		EngineType: "webkit",
		Appearance: port.WebUIAppearanceConfig{
			ColorScheme:     "prefer-dark",
			SansFont:        "Inter",
			SerifFont:       "Georgia",
			MonospaceFont:   "JetBrains Mono",
			DefaultFontSize: 16,
			LightPalette: port.ColorPalette{
				Background:     "#ffffff",
				Surface:        "#fafafa",
				SurfaceVariant: "#eeeeee",
				Text:           "#111111",
				Muted:          "#666666",
				Accent:         "#0055ff",
				Border:         "#dddddd",
			},
			DarkPalette: port.ColorPalette{
				Background:     "#111111",
				Surface:        "#1a1a1a",
				SurfaceVariant: "#2a2a2a",
				Text:           "#f5f5f5",
				Muted:          "#a0a0a0",
				Accent:         "#66aaff",
				Border:         "#333333",
			},
		},
		Performance: port.SystemviewPerformancePayload{
			Profile: "balanced",
			Custom: port.SystemviewCustomPerformancePayload{
				SkiaCPUThreads:         4,
				SkiaGPUThreads:         2,
				WebProcessMemoryMB:     512,
				NetworkProcessMemoryMB: 128,
				WebViewPoolPrewarm:     1,
			},
			Hardware: port.SystemviewHardwarePayload{
				CPUCores:   8,
				CPUThreads: 16,
				TotalRAMMB: 32768,
				GPUVendor:  "NVIDIA",
				GPUName:    "RTX 4060",
				VRAMMB:     8192,
			},
		},
		DefaultSearchEngine: "https://duckduckgo.com/?q=%s",
		SearchShortcuts: map[string]port.SearchShortcut{
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
		Action: configActionSetKeybinding,
		Data:   map[string]string{"mode": "default", "action": "toggle_history_systemview", "keys": "ctrl+h, alt+h"},
	}))
	assert.True(t, service.calledSet)
	assert.Equal(t, []string{"ctrl+h", "alt+h"}, service.setReq.Keys)
	assert.Contains(t, dom.html, "Saved keybinding toggle_history_systemview")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: configActionResetKeybinding,
		Data:   map[string]string{"mode": "default", "action": "toggle_history_systemview"},
	}))
	assert.True(t, service.calledReset)
	assert.Equal(t, "toggle_history_systemview", service.resetReq.Action)
	assert.Contains(t, dom.html, "Reset keybinding toggle_history_systemview")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: configActionResetAllKeybindings}))
	assert.True(t, service.calledResetAll)
	assert.Contains(t, dom.html, "Reset all keybindings to defaults")
}

func testConfigPayload() port.SystemviewConfigPayload {
	return port.SystemviewConfigPayload{
		EngineType: "webkit",
		Appearance: port.WebUIAppearanceConfig{
			ColorScheme:     "prefer-dark",
			SansFont:        "Inter",
			SerifFont:       "Georgia",
			MonospaceFont:   "JetBrains Mono",
			DefaultFontSize: 16,
			LightPalette:    port.ColorPalette{Background: "#ffffff", Surface: "#fafafa", SurfaceVariant: "#eeeeee", Text: "#111111", Muted: "#666666", Accent: "#0055ff", Border: "#dddddd"},
			DarkPalette:     port.ColorPalette{Background: "#111111", Surface: "#1a1a1a", SurfaceVariant: "#2a2a2a", Text: "#f5f5f5", Muted: "#a0a0a0", Accent: "#66aaff", Border: "#333333"},
		},
		DefaultUIScale:      1,
		DefaultSearchEngine: "https://duckduckgo.com/?q=%s",
		SearchShortcuts: map[string]port.SearchShortcut{
			"ddg": {URL: "https://duckduckgo.com/?q=%s", Description: "DuckDuckGo"},
		},
		Performance: port.SystemviewPerformancePayload{
			Profile:  "default",
			Custom:   port.SystemviewCustomPerformancePayload{SkiaCPUThreads: 2, SkiaGPUThreads: 1, WebProcessMemoryMB: 1024, NetworkProcessMemoryMB: 256, WebViewPoolPrewarm: 2},
			Hardware: port.SystemviewHardwarePayload{CPUCores: 4, CPUThreads: 8, TotalRAMMB: 16384, GPUVendor: "AMD", GPUName: "Radeon", VRAMMB: 4096},
		},
	}
}

func testDefaultConfigPayload() port.SystemviewConfigPayload {
	cfg := testConfigPayload()
	cfg.Appearance.DefaultFontSize = 14
	cfg.DefaultUIScale = 1
	return cfg
}

type fakeDOM struct {
	mounted bool
	html    string
}

func (d *fakeDOM) Mount(html string) error {
	d.mounted = true
	d.html = html
	return nil
}

type fakeHistoryService struct {
	called  bool
	limit   int
	offset  int
	entries []*entity.HistoryEntry
	err     error

	searchCalled  bool
	query         string
	searchLimit   int
	searchEntries []*entity.HistoryEntry

	deletedEntryID int64
	deletedRangeID string
	deletedDomain  string
	domainStats    []*entity.DomainStat
}

func (s *fakeHistoryService) Timeline(_ context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	s.called = true
	s.limit = limit
	s.offset = offset
	return s.entries, s.err
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

func (s *fakeHistoryService) Analytics(context.Context) (*entity.HistoryAnalytics, error) {
	return nil, nil
}

func (s *fakeHistoryService) DomainStats(context.Context, int) ([]*entity.DomainStat, error) {
	return s.domainStats, nil
}

func (s *fakeHistoryService) DeleteDomain(_ context.Context, domain string) error {
	s.deletedDomain = domain
	return nil
}

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

func (s *fakeFavoritesService) CreateFolder(_ context.Context, name string, _ *int64) (*entity.Folder, error) {
	s.createdFolder = name
	return &entity.Folder{ID: 77, Name: name}, nil
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

type fakeConfigService struct {
	calledCurrent     bool
	calledDefault     bool
	calledSave        bool
	calledKeybindings bool
	calledSet         bool
	calledReset       bool
	calledResetAll    bool

	current     port.SystemviewConfigPayload
	defaultCfg  port.SystemviewConfigPayload
	keybindings port.KeybindingsConfig
	savedConfig port.WebUIConfig
	setReq      port.SetKeybindingRequest
	setResp     port.SetKeybindingResponse
	resetReq    port.ResetKeybindingRequest
}

func (s *fakeConfigService) Current(context.Context) (port.SystemviewConfigPayload, error) {
	s.calledCurrent = true
	return s.current, nil
}

func (s *fakeConfigService) Default(context.Context) (port.SystemviewConfigPayload, error) {
	s.calledDefault = true
	return s.defaultCfg, nil
}

func (s *fakeConfigService) Save(_ context.Context, cfg port.WebUIConfig) error {
	s.calledSave = true
	s.savedConfig = cfg
	s.current.Appearance = cfg.Appearance
	s.current.DefaultUIScale = cfg.DefaultUIScale
	s.current.DefaultSearchEngine = cfg.DefaultSearchEngine
	s.current.SearchShortcuts = cfg.SearchShortcuts
	s.current.Performance.Profile = cfg.Performance.Profile
	s.current.Performance.Custom = port.SystemviewCustomPerformancePayload{
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

package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/dispatcher"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// History sidebar integration tests
// =============================================================================

// TestHistorySidebarConfig_OnNavigateNavigatesActivePaneAndKeepsSidebar verifies
// that the default OnNavigate callback targets the owning browser window's
// active pane and keeps the sidebar visible.
func TestHistorySidebarConfig_OnNavigateNavigatesActivePaneAndKeepsSidebar(t *testing.T) {
	ctx := t.Context()

	// Build two browser windows with independent tabs and panes.
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))

	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	// Create fake webviews, one per pane.
	fakeWv1 := &fakeRecordingWebView{id: 1}
	fakeWv2 := &fakeRecordingWebView{id: 2}

	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), fakeWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), fakeWv2)

	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id,
		contentCoord:        contentCoord,
		navCoord:            navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab1.ID: {},
			tab2.ID: {},
		},
	}
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	second.mainWindow = &window.MainWindow{}
	second.historySidebar = &component.HistorySidebar{}
	second.sidebarVisible = true

	cfg := app.buildHistorySidebarConfig(second)
	navigateURL := "https://example.com"
	err := cfg.OnNavigate(ctx, navigateURL)
	require.NoError(t, err, "OnNavigate should succeed")

	// The second window's webview must have received the navigation.
	assert.True(t, fakeWv2.loadURICalled, "second window webview should receive navigation")
	assert.Equal(t, navigateURL, fakeWv2.loadURILastURI)
	assert.True(t, second.sidebarVisible, "default history activation should keep the sidebar visible")

	// The first window's webview must NOT have been touched (stale-focus guard).
	assert.False(t, fakeWv1.loadURICalled, "first window webview must not receive navigation from second window callback")
}

// TestHistorySidebarConfig_OnNavigateKeepOpenNavigatesWithoutClosing verifies
// that the OnNavigateKeepOpen callback (Ctrl+Enter) navigates the owning window's
// active pane and keeps the sidebar visible.
func TestHistorySidebarConfig_OnNavigateKeepOpenNavigatesWithoutClosing(t *testing.T) {
	ctx := t.Context()

	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	bw := &browserWindow{
		id:             "window-1",
		tabs:           bwTabs,
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: true,
	}

	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	fakeWv := &fakeRecordingWebView{id: 1}
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), fakeWv)
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
		contentCoord:   contentCoord,
		navCoord:       navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab.ID: {},
		},
	}
	app.tabs.Add(tab)

	cfg := app.buildHistorySidebarConfig(bw)
	navigateURL := "https://keep-open.com"
	err := cfg.OnNavigateKeepOpen(ctx, navigateURL)
	require.NoError(t, err)
	assert.True(t, fakeWv.loadURICalled)
	assert.Equal(t, navigateURL, fakeWv.loadURILastURI, "Ctrl+Enter navigation should go to the URL")
	assert.True(t, bw.sidebarVisible, "Ctrl+Enter navigation should keep the sidebar visible")
}

// TestHistorySidebar_OwnershipOnMultiWindowNavigation verifies that when
// multiple browser windows have history sidebars, navigation targets the
// correct owning window's active pane. This tests the stale-focus scenario
// where a different window is globally focused.
func TestHistorySidebar_OwnershipOnMultiWindowNavigation(t *testing.T) {
	ctx := t.Context()

	// Two windows, each with their own tab and pane.
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))

	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	fakeWv1 := &fakeRecordingWebView{id: 1}
	fakeWv2 := &fakeRecordingWebView{id: 2}

	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), fakeWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), fakeWv2)

	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id, // STALE: first is globally focused
		contentCoord:        contentCoord,
		navCoord:            navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab1.ID: {},
			tab2.ID: {},
		},
	}
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	// Navigation from the SECOND window should target pane-2 even though
	// the first window is globally focused (stale focus).
	err := app.navigateFromBrowserWindow(ctx, second, "https://second-window.com")
	require.NoError(t, err)

	// Second window's webview must receive the navigation.
	assert.True(t, fakeWv2.loadURICalled, "second window webview should receive navigation")
	assert.Equal(t, "https://second-window.com", fakeWv2.loadURILastURI)

	// First window's webview must NOT have been touched.
	assert.False(t, fakeWv1.loadURICalled, "first window webview should NOT receive navigation when second was targeted")
}

// =============================================================================
// History sidebar toggle state tests
// =============================================================================

// TestBrowserWindow_HistorySidebarToggle_NilIsNoOp verifies that when
// browserWindow.historySidebar is nil, toggleHistorySidebar is a safe
// no-op and sidebarVisible stays / remains false.
func TestBrowserWindow_HistorySidebarToggle_NilIsNoOp(t *testing.T) {
	t.Parallel()

	bw := &browserWindow{id: "test-window", sidebarVisible: false}
	require.Nil(t, bw.historySidebar, "historySidebar must be nil for this test")

	// Should not panic even though historySidebar is nil.
	bw.toggleHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebarVisible must remain false when historySidebar is nil")

	// Calling again also safe.
	bw.toggleHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebarVisible must remain false on second toggle")
}

// TestBrowserWindow_HistorySidebarToggle_FlipsSidebarVisible verifies that
// toggleHistorySidebar correctly flips sidebarVisible when the sidebar
// has been set. Uses a zero-value HistorySidebar (all nil-checked methods
// are safe to call).
func TestBrowserWindow_HistorySidebarToggle_FlipsSidebarVisible(t *testing.T) {
	t.Parallel()

	bw := &browserWindow{
		id:             "test-window",
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: false,
	}

	bw.toggleHistorySidebar()
	assert.True(t, bw.sidebarVisible, "sidebarVisible must be true after first toggle")

	bw.toggleHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebarVisible must be false after second toggle")

	bw.toggleHistorySidebar()
	assert.True(t, bw.sidebarVisible, "sidebarVisible must be true after third toggle")
}

// TestBrowserWindow_HistorySidebarShowHide_TransitionsSidebarVisible
// verifies that showHistorySidebar and hideHistorySidebar independently
// set sidebarVisible to true and false respectively.
func TestBrowserWindow_HistorySidebarShowHide_TransitionsSidebarVisible(t *testing.T) {
	t.Parallel()

	bw := &browserWindow{
		id:             "test-window",
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: false,
	}

	// Show sets visible
	bw.showHistorySidebar()
	assert.True(t, bw.sidebarVisible, "sidebarVisible must be true after show")

	// Hide clears visible
	bw.hideHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebarVisible must be false after hide")

	// Redundant hide is idempotent
	bw.hideHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebarVisible must remain false after redundant hide")

	// Show again
	bw.showHistorySidebar()
	assert.True(t, bw.sidebarVisible, "sidebarVisible must be true after second show")
}

// TestBrowserWindow_HistorySidebarShowHide_NilSidebarIsNoOp verifies that
// show/hide do not panic when historySidebar is nil.
func TestBrowserWindow_HistorySidebarShowHide_NilSidebarIsNoOp(t *testing.T) {
	t.Parallel()

	bw := &browserWindow{id: "test-window"}
	require.Nil(t, bw.historySidebar)

	// Should not panic even though historySidebar is nil.
	bw.showHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebarVisible must remain false when nil")

	bw.hideHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebarVisible must remain false when nil")
}

// =============================================================================
// App wiring: toggle handler uses lastFocusedBrowserWindow
// =============================================================================

// TestApp_HistorySidebarToggleHandlerUsesLastFocusedWindow verifies that
// the toggle handler wired in App.wireKeyboardActions picks the
// lastFocusedBrowserWindow and calls toggleHistorySidebar on it.
func TestApp_HistorySidebarToggleHandlerUsesLastFocusedWindow(t *testing.T) {
	// Two windows, only the focused one has a history sidebar.
	focusedBW := &browserWindow{
		id:             "focused",
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: false,
	}
	otherBW := &browserWindow{
		id:         "other",
		mainWindow: &window.MainWindow{},
	}

	app := &App{
		browserWindows: map[string]*browserWindow{
			focusedBW.id: focusedBW,
			otherBW.id:   otherBW,
		},
		lastFocusedWindowID: focusedBW.id,
	}

	// Simulate the toggle handler that wireKeyboardActions registers:
	// a.kbDispatcher.SetOnToggleHistorySidebar(func(ctx context.Context) error {
	//     bw := a.lastFocusedBrowserWindow()
	//     ...
	// })
	bw := app.lastFocusedBrowserWindow()
	require.NotNil(t, bw, "lastFocusedBrowserWindow must not be nil")
	require.Equal(t, focusedBW.id, bw.id, "must return the focused window")
	require.NotNil(t, bw.historySidebar, "focused window must have a history sidebar")

	// Toggle on the focused window.
	bw.toggleHistorySidebar()
	assert.True(t, bw.sidebarVisible, "sidebar on focused window must become visible")

	// The other window must remain untouched.
	assert.False(t, otherBW.sidebarVisible, "other window sidebar must remain invisible")

	// Toggle again: focused window sidebar hides.
	bw.toggleHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebar on focused window must hide on second toggle")
}

// TestApp_HistorySidebarToggleHandler_NilFocusedWindowIsNoOp verifies
// that the toggle handler is safe when lastFocusedBrowserWindow returns nil.
func TestApp_HistorySidebarToggleHandler_NilFocusedWindowIsNoOp(t *testing.T) {
	app := &App{
		browserWindows:      make(map[string]*browserWindow),
		lastFocusedWindowID: "missing",
	}

	// The handler should return early without error when bw is nil.
	bw := app.lastFocusedBrowserWindow()
	require.Nil(t, bw, "lastFocusedBrowserWindow should return nil for missing window")
}

// =============================================================================
// History sidebar config callbacks: focus restoration on close
// =============================================================================

// TestHistorySidebarConfig_OnCloseHidesSidebar verifies that the OnClose
// callback hides the sidebar for the owning browser window.
func TestHistorySidebarConfig_OnCloseHidesSidebar(t *testing.T) {
	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	bw := &browserWindow{
		id:             "window-1",
		tabs:           bwTabs,
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: true,
	}

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{tab.ID: {}},
	}
	app.tabs.Add(tab)

	cfg := app.buildHistorySidebarConfig(bw)
	cfg.OnClose()

	assert.False(t, bw.sidebarVisible, "sidebar must be hidden by OnClose")
}

// =============================================================================
// Dispatcher-backed Ctrl+H integration test
// =============================================================================

// TestApp_HistorySidebar_ToggleThroughKeyboardDispatcher wires the real toggle
// handler from App.wireKeyboardActions through the KeyboardDispatcher and
// dispatches ActionToggleHistorySystemView, asserting the focused browser
// window's sidebar visibility toggles.
func TestApp_HistorySidebar_ToggleThroughKeyboardDispatcher(t *testing.T) {
	ctx := t.Context()

	focusedBW := &browserWindow{
		id:             "focused",
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: false,
	}
	otherBW := &browserWindow{
		id:         "other",
		mainWindow: &window.MainWindow{},
	}

	app := &App{
		browserWindows: map[string]*browserWindow{
			focusedBW.id: focusedBW,
			otherBW.id:   otherBW,
		},
		lastFocusedWindowID: focusedBW.id,
	}

	// Create a KeyboardDispatcher and wire the production toggle handler.
	kbDispatcher := dispatcher.NewKeyboardDispatcher(
		ctx,
		&coordinator.WorkspaceCoordinator{},
		&coordinator.NavigationCoordinator{},
		nil, nil,
		dispatcher.KeyboardActions{},
		func(context.Context) entity.PaneID { return "" },
	)

	kbDispatcher.SetOnToggleHistorySidebar(app.toggleHistorySidebarAction)

	// First dispatch: toggle ON.
	err := kbDispatcher.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.NoError(t, err)
	assert.True(t, focusedBW.sidebarVisible, "focused window sidebar must be visible after toggle")
	assert.False(t, otherBW.sidebarVisible, "other window sidebar must remain invisible")

	// Second dispatch: toggle OFF.
	err = kbDispatcher.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.NoError(t, err)
	assert.False(t, focusedBW.sidebarVisible, "focused window sidebar must be hidden after second toggle")
}

// TestApp_HistorySidebar_ToggleThroughDispatcher_UnavailableReturnsError verifies that
// when the focused window has no history sidebar, Ctrl+H returns a clean error.
func TestApp_HistorySidebar_ToggleThroughDispatcher_UnavailableReturnsError(t *testing.T) {
	ctx := t.Context()

	bw := &browserWindow{
		id:         "no-sidebar",
		mainWindow: &window.MainWindow{},
		// historySidebar is nil
	}

	app := &App{
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}

	kbDispatcher := dispatcher.NewKeyboardDispatcher(
		ctx,
		&coordinator.WorkspaceCoordinator{},
		&coordinator.NavigationCoordinator{},
		nil, nil,
		dispatcher.KeyboardActions{},
		func(context.Context) entity.PaneID { return "" },
	)

	kbDispatcher.SetOnToggleHistorySidebar(app.toggleHistorySidebarAction)

	err := kbDispatcher.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.Error(t, err)
	require.ErrorContains(t, err, "history sidebar unavailable")
	assert.False(t, bw.sidebarVisible, "sidebar must remain invisible when not wired")
}

// TestApp_HistorySidebar_ToggleThroughDispatcher_NilFocusedReturnsError verifies
// that the toggle handler returns a clean error when there is no focused window.
func TestApp_HistorySidebar_ToggleThroughDispatcher_NilFocusedReturnsError(t *testing.T) {
	ctx := t.Context()

	app := &App{
		browserWindows:      make(map[string]*browserWindow),
		lastFocusedWindowID: "missing",
	}

	kbDispatcher := dispatcher.NewKeyboardDispatcher(
		ctx,
		&coordinator.WorkspaceCoordinator{},
		&coordinator.NavigationCoordinator{},
		nil, nil,
		dispatcher.KeyboardActions{},
		func(context.Context) entity.PaneID { return "" },
	)

	kbDispatcher.SetOnToggleHistorySidebar(app.toggleHistorySidebarAction)

	err := kbDispatcher.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.Error(t, err)
	require.ErrorContains(t, err, "history sidebar unavailable")
}

// =============================================================================
// buildHistorySidebarConfig callback seam tests
// =============================================================================

// TestApp_HistorySidebarConfig_NavigateCallback verifies that the OnNavigate
// callback from buildHistorySidebarConfig navigates the owning browser window's
// active pane to the given URL.
func TestApp_HistorySidebarConfig_NavigateCallbackNavigates(t *testing.T) {
	ctx := t.Context()

	paneID := entity.PaneID("pane-1")
	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(paneID))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	bw := &browserWindow{
		id:             "window-1",
		tabs:           bwTabs,
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: true,
	}

	fakeWv := &fakeRecordingWebView{id: 1}
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(paneID, fakeWv)
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
		contentCoord:   contentCoord,
		navCoord:       navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab.ID: {},
		},
	}
	app.tabs.Add(tab)

	// Build the config using the extracted seam.
	cfg := app.buildHistorySidebarConfig(bw)
	require.NotNil(t, cfg.OnNavigate, "OnNavigate callback must be non-nil")

	// Invoke the OnNavigate callback.
	navigateURL := "https://navigated.com"
	err := cfg.OnNavigate(ctx, navigateURL)
	require.NoError(t, err)

	// Verify the navigation reached the correct webview.
	assert.True(t, fakeWv.loadURICalled, "webview must receive navigation")
	assert.Equal(t, navigateURL, fakeWv.loadURILastURI)
	assert.True(t, bw.sidebarVisible, "default history navigation should keep the sidebar open")
}

func TestApp_NavigateHistorySidebarSelection_KeepsSidebarVisible(t *testing.T) {
	ctx := t.Context()

	paneID := entity.PaneID("pane-1")
	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(paneID))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	bw := &browserWindow{
		id:             "window-1",
		tabs:           bwTabs,
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: true,
	}

	fakeWv := &fakeRecordingWebView{id: 1}
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(paneID, fakeWv)
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
		contentCoord:   contentCoord,
		navCoord:       navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab.ID: {},
		},
	}
	app.tabs.Add(tab)

	err := app.navigateHistorySidebarSelection(ctx, bw, "https://open.com")
	require.NoError(t, err)
	assert.True(t, fakeWv.loadURICalled)
	assert.Equal(t, "https://open.com", fakeWv.loadURILastURI)
	assert.True(t, bw.sidebarVisible, "history selection navigation should not hide the sidebar")
}

// TestApp_HistorySidebarConfig_NavigateCallbackOwnership verifies that
// OnNavigate targets the callback's owning window, not the globally focused
// window, when they differ.
func TestApp_HistorySidebarConfig_NavigateCallbackOwnership(t *testing.T) {
	ctx := t.Context()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))

	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	fakeWv1 := &fakeRecordingWebView{id: 1}
	fakeWv2 := &fakeRecordingWebView{id: 2}

	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), fakeWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), fakeWv2)
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id, // STALE: first is globally focused
		contentCoord:        contentCoord,
		navCoord:            navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab1.ID: {},
			tab2.ID: {},
		},
	}
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	// Build config for the SECOND window, even though first is globally focused.
	cfg := app.buildHistorySidebarConfig(second)

	// Invoke OnNavigate — should navigate through second window's pane-2.
	err := cfg.OnNavigate(ctx, "https://ownership.com")
	require.NoError(t, err)

	// Second window's webview must receive navigation.
	assert.True(t, fakeWv2.loadURICalled, "second window webview must receive navigation")
	assert.Equal(t, "https://ownership.com", fakeWv2.loadURILastURI)

	// First window's webview must NOT be touched.
	assert.False(t, fakeWv1.loadURICalled, "first window webview must not receive navigation")
}

// TestApp_HistorySidebarConfig_KeepOpenCallback verifies that
// OnNavigateKeepOpen navigates the owning window without hiding the sidebar.
func TestApp_HistorySidebarConfig_KeepOpenCallback(t *testing.T) {
	ctx := t.Context()

	paneID := entity.PaneID("pane-1")
	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(paneID))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	bw := &browserWindow{
		id:             "window-1",
		tabs:           bwTabs,
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: true,
	}

	fakeWv := &fakeRecordingWebView{id: 1}
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(paneID, fakeWv)
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
		contentCoord:   contentCoord,
		navCoord:       navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab.ID: {},
		},
	}
	app.tabs.Add(tab)

	cfg := app.buildHistorySidebarConfig(bw)

	// OnNavigateKeepOpen navigates but does NOT close the sidebar.
	navigateURL := "https://keep-open.com"
	err := cfg.OnNavigateKeepOpen(ctx, navigateURL)
	require.NoError(t, err)

	assert.True(t, fakeWv.loadURICalled, "webview must receive navigation")
	assert.Equal(t, navigateURL, fakeWv.loadURILastURI)

	// Sidebar must remain visible (keep-open contract).
	assert.True(t, bw.sidebarVisible, "sidebar must stay visible after keep-open navigation")
}

// TestApp_HistorySidebarConfig_OpenInNewPaneCallback verifies that
// OnOpenInNewPane activates the owning browser window and creates a split
// with the target URL.
func TestApp_HistorySidebarConfig_OpenInNewPaneCallback(t *testing.T) {
	ctx := t.Context()

	paneID := entity.PaneID("pane-1")
	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(paneID))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	ws := entity.NewWorkspace("ws-1", entity.NewPane(paneID))
	ws.ActivePaneID = paneID
	tab.Workspace = ws

	bw := &browserWindow{id: "window-1", tabs: bwTabs}

	panesUC := usecase.NewManagePanesUseCase(func() string { return "pane-2" })
	wsCoord := coordinator.NewWorkspaceCoordinator(ctx, coordinator.WorkspaceCoordinatorConfig{
		PanesUC: panesUC,
		GetActiveWS: func() (*entity.Workspace, *component.WorkspaceView) {
			return ws, nil
		},
	})

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
		wsCoord:        wsCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab.ID: {},
		},
	}
	app.tabs.Add(tab)

	cfg := app.buildHistorySidebarConfig(bw)

	// OnOpenInNewPane should activate the owning window and split with URL.
	splitURL := "https://shift-enter.com"
	err := cfg.OnOpenInNewPane(ctx, splitURL)
	require.NoError(t, err)

	// After split, workspace should have 2 panes.
	require.Equal(t, 2, ws.PaneCount(), "workspace should have 2 panes after split")

	// The new pane should have the split URL.
	allPanes := ws.AllPanes()
	var newPane *entity.Pane
	for _, p := range allPanes {
		if p != nil && p.ID != paneID {
			newPane = p
			break
		}
	}
	require.NotNil(t, newPane, "new pane must exist after split")
	assert.Equal(t, splitURL, newPane.URI, "new pane must have the split URL")
}

// TestApp_HistorySidebarConfig_CloseCallback verifies that OnClose hides the
// sidebar for the owning browser window and restores focus to the active pane.
func TestApp_HistorySidebarConfig_CloseCallback(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(paneID))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	ws := entity.NewWorkspace("ws-1", entity.NewPane(paneID))
	ws.ActivePaneID = paneID
	tab.Workspace = ws

	bw := &browserWindow{
		id:             "window-1",
		tabs:           bwTabs,
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: true,
	}

	// Create a minimal workspace view.
	wsView := &component.WorkspaceView{}
	// We set up the app so that hideAndRestoreFocusForBrowserWindow
	// can find the wsView and call FocusPane on it.
	// Since FocusPane is a method on WorkspaceView that requires GTK,
	// we verify the state changes that happen before FocusPane:
	// sidebarVisible must be toggled to false.

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab.ID: wsView,
		},
	}
	cfg := app.buildHistorySidebarConfig(bw)

	// OnClose hides the sidebar.
	cfg.OnClose()

	assert.False(t, bw.sidebarVisible, "sidebar must be hidden after close")
}

// TestApp_HistorySidebarConfig_CloseWithNoSidebarIsSafe verifies that
// OnClose (hideAndRestoreFocusForBrowserWindow) is safe when the browser
// window has no sidebar or is nil.
func TestApp_HistorySidebarConfig_CloseWithNoSidebarIsSafe(t *testing.T) {
	bw := &browserWindow{
		id:         "no-sidebar",
		mainWindow: &window.MainWindow{},
		// historySidebar is nil
	}

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
	}

	cfg := app.buildHistorySidebarConfig(bw)

	// Should not panic even with nil sidebar.
	require.NotPanics(t, func() { cfg.OnClose() })
}

// TestApp_HideAndRestoreFocusForBrowserWindow_HidesSidebar verifies that
// hideAndRestoreFocusForBrowserWindow hides the sidebar.
func TestApp_HideAndRestoreFocusForBrowserWindow_HidesSidebar(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(paneID))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	ws := entity.NewWorkspace("ws-1", entity.NewPane(paneID))
	ws.ActivePaneID = paneID
	tab.Workspace = ws

	bw := &browserWindow{
		id:             "window-1",
		tabs:           bwTabs,
		mainWindow:     &window.MainWindow{},
		historySidebar: &component.HistorySidebar{},
		sidebarVisible: true,
	}

	wsView := &component.WorkspaceView{}

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab.ID: wsView,
		},
	}
	app.tabs.Add(tab)

	// Call hideAndRestoreFocusForBrowserWindow directly.
	app.hideAndRestoreFocusForBrowserWindow(bw)

	// Sidebar must be hidden.
	assert.False(t, bw.sidebarVisible, "sidebar must be hidden")
}

// TestApp_HideAndRestoreFocusForBrowserWindow_NilBWIsSafe verifies that
// hideAndRestoreFocusForBrowserWindow handles nil browser window safely.
func TestApp_HideAndRestoreFocusForBrowserWindow_NilBWIsSafe(t *testing.T) {
	app := &App{}
	require.NotPanics(t, func() { app.hideAndRestoreFocusForBrowserWindow(nil) })
}

// =============================================================================
// Sidebar width config tests
// =============================================================================

func TestHistorySidebarWidthConfig_ConfigValue(t *testing.T) {
	cfg := historySidebarWidthConfig(350)
	assert.Equal(t, 350, cfg.WidthPx, "should apply config-backed width of 350px")
	assert.Equal(t, 280, cfg.MinPx, "should keep default min clamp")
	assert.Equal(t, 380, cfg.MaxPx, "should keep default max clamp")
}

func TestHistorySidebarWidthConfig_DefaultValue(t *testing.T) {
	cfg := historySidebarWidthConfig(0)
	assert.Equal(t, 320, cfg.WidthPx, "should use default width of 320px when config is 0")
	assert.Equal(t, 280, cfg.MinPx, "should keep default min clamp")
	assert.Equal(t, 380, cfg.MaxPx, "should keep default max clamp")
}

// TestBrowserWindow_ApplySidebarWidthConfig_NilMainWindowIsSafe verifies
// that applySidebarWidthConfig handles nil mainWindow without panic.
func TestBrowserWindow_ApplySidebarWidthConfig_NilMainWindowIsSafe(t *testing.T) {
	bw := &browserWindow{mainWindow: nil}
	app := &App{deps: &Dependencies{Config: &config.Config{SidebarWidth: 300}}}
	require.NotPanics(t, func() { bw.applySidebarWidthConfig(app) })
}

// TestBrowserWindow_ApplySidebarWidthConfig_NilDepsIsSafe verifies
// that applySidebarWidthConfig handles nil deps without panic.
func TestBrowserWindow_ApplySidebarWidthConfig_NilDepsIsSafe(t *testing.T) {
	mw := &window.MainWindow{}
	bw := &browserWindow{mainWindow: mw}
	app := &App{deps: nil}
	require.NotPanics(t, func() { bw.applySidebarWidthConfig(app) })
}

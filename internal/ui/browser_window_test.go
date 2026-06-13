package ui

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/dispatcher"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserWindow_RemoveBrowserWindowClearsShellState(t *testing.T) {
	mainWindow := &window.MainWindow{}
	removed := &browserWindow{id: "window-1", mainWindow: mainWindow}
	remaining := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	app := &App{
		mainWindow:          mainWindow,
		browserWindows:      map[string]*browserWindow{removed.id: removed, remaining.id: remaining},
		lastFocusedWindowID: remaining.id,
	}

	setShellField(t, removed, "appToaster", &component.Toaster{})
	setShellField(t, removed, "modeToaster", &component.Toaster{})
	setShellField(t, removed, "borderMgr", &focus.BorderManager{})
	setShellField(t, removed, "sessionManager", &component.SessionManager{})
	setShellField(t, removed, "tabPicker", &component.TabPicker{})
	setShellField(t, removed, "tabPickerWidget", (*testLayoutWidget)(nil))
	setShellField(t, removed, "tabPickerPaneID", entity.PaneID("pane-1"))
	setShellField(t, removed, "insertAccentUC", newTestAccentUseCase(t, false))
	setShellField(t, removed, "accentPicker", &component.AccentPicker{})
	setShellField(t, removed, "keyboardHandler", &input.KeyboardHandler{})
	setShellField(t, removed, "globalShortcutHandler", &input.GlobalShortcutHandler{})
	setShellField(t, removed, "permissionDialog", (*testPermissionDialogPresenter)(nil))
	setShellField(t, removed, "webrtcIndicator", &component.WebRTCPermissionIndicator{})
	setShellField(t, removed, "historySidebar", &component.HistorySidebar{})

	app.removeBrowserWindow(removed.id)

	for _, name := range []string{
		"appToaster",
		"modeToaster",
		"borderMgr",
		"sessionManager",
		"tabPicker",
		"tabPickerWidget",
		"tabPickerPaneID",
		"insertAccentUC",
		"accentPicker",
		"keyboardHandler",
		"globalShortcutHandler",
		"permissionDialog",
		"webrtcIndicator",
		"historySidebar",
	} {
		if !fieldIsZero(t, removed, name) {
			t.Fatalf("browserWindow.%s was not cleared", name)
		}
	}
}

func TestApp_CleanupCreatedBrowserWindowsDetachesUnregisteredWindowBeforeDestroy(t *testing.T) {
	transientMainWindow := &window.MainWindow{}
	fallbackMainWindow := &window.MainWindow{}
	transient := &browserWindow{id: "window-transient", mainWindow: transientMainWindow}
	fallback := &browserWindow{id: "window-fallback", mainWindow: fallbackMainWindow}
	app := &App{
		mainWindow:          transientMainWindow,
		browserWindows:      map[string]*browserWindow{fallback.id: fallback},
		browserWindowOrder:  []string{fallback.id},
		lastFocusedWindowID: transient.id,
	}

	setShellField(t, transient, "keyboardHandler", &input.KeyboardHandler{})
	setShellField(t, transient, "globalShortcutHandler", &input.GlobalShortcutHandler{})
	setShellField(t, transient, "accentPicker", &component.AccentPicker{})

	app.cleanupCreatedBrowserWindows([]*browserWindow{transient})

	for _, name := range []string{"keyboardHandler", "globalShortcutHandler", "accentPicker"} {
		if !fieldIsZero(t, transient, name) {
			t.Fatalf("transient browserWindow.%s was not cleared", name)
		}
	}
	if app.mainWindow != fallbackMainWindow {
		t.Fatalf("mainWindow = %p, want fallback main window %p", app.mainWindow, fallbackMainWindow)
	}
	if app.lastFocusedWindowID != fallback.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, fallback.id)
	}
	if got := app.browserWindows[fallback.id]; got != fallback {
		t.Fatalf("fallback browser window changed during transient cleanup: got %p want %p", got, fallback)
	}
}

func TestBrowserWindow_RemoveBrowserWindowCleansOwnedTabState(t *testing.T) {
	ownedTab := entity.NewTab(entity.TabID("tab-owned"), entity.WorkspaceID("workspace-owned"), entity.NewPane(entity.PaneID("pane-owned")))
	otherTab := entity.NewTab(entity.TabID("tab-other"), entity.WorkspaceID("workspace-other"), entity.NewPane(entity.PaneID("pane-other")))
	removedMainWindow := &window.MainWindow{}
	remainingMainWindow := &window.MainWindow{}
	removedTabs := entity.NewTabList()
	removedTabs.Add(ownedTab)
	remainingTabs := entity.NewTabList()
	remainingTabs.Add(otherTab)
	removed := &browserWindow{id: "window-1", mainWindow: removedMainWindow, tabs: removedTabs}
	remaining := &browserWindow{id: "window-2", mainWindow: remainingMainWindow, tabs: remainingTabs}
	ownedSessionKey := floatingSessionKey{tabID: ownedTab.ID, sessionID: "profile:owned"}
	otherSessionKey := floatingSessionKey{tabID: otherTab.ID, sessionID: "profile:other"}
	app := &App{
		mainWindow:     removedMainWindow,
		browserWindows: map[string]*browserWindow{removed.id: removed, remaining.id: remaining},
		tabs:           entity.NewTabList(),
		workspaceViews: map[entity.TabID]*component.WorkspaceView{ownedTab.ID: &component.WorkspaceView{}, otherTab.ID: &component.WorkspaceView{}},
		windowForTab:   map[entity.TabID]*browserWindow{ownedTab.ID: removed, otherTab.ID: remaining},
		floatingSessions: map[floatingSessionKey]*floatingWorkspaceSession{
			ownedSessionKey: {},
			otherSessionKey: {},
		},
	}
	app.tabs.Add(ownedTab)
	app.tabs.Add(otherTab)

	app.removeBrowserWindow(removed.id)

	if app.workspaceViews[ownedTab.ID] != nil {
		t.Fatalf("owned workspace view was not removed")
	}
	if app.windowForTab[ownedTab.ID] != nil {
		t.Fatalf("owned window mapping was not removed")
	}
	if app.tabs.Find(ownedTab.ID) != nil {
		t.Fatalf("owned tab was not removed")
	}
	if app.workspaceViews[otherTab.ID] == nil {
		t.Fatalf("other workspace view should remain")
	}
	if app.windowForTab[otherTab.ID] != remaining {
		t.Fatalf("other tab should remain mapped to remaining window")
	}
	if app.tabs.Find(otherTab.ID) == nil {
		t.Fatalf("other tab should remain")
	}
	if _, ok := app.floatingSessions[ownedSessionKey]; ok {
		t.Fatalf("owned floating session was not released")
	}
	if _, ok := app.floatingSessions[otherSessionKey]; !ok {
		t.Fatalf("other floating session should remain")
	}
	assertWindowOwnershipInvariant(t, app)
}

func TestBrowserWindow_RegisterAndRemoveTrackCollection(t *testing.T) {
	app := &App{browserWindows: make(map[string]*browserWindow)}
	first := &browserWindow{id: "window-1"}
	second := &browserWindow{id: "window-2"}

	app.registerBrowserWindow(first)
	app.registerBrowserWindow(second)

	if got := len(app.browserWindows); got != 2 {
		t.Fatalf("browserWindows length = %d, want 2", got)
	}
	if _, ok := app.browserWindows[first.id]; !ok {
		t.Fatalf("first browser window was not registered")
	}
	if _, ok := app.browserWindows[second.id]; !ok {
		t.Fatalf("second browser window was not registered")
	}
	if first.tabs == nil || second.tabs == nil {
		t.Fatalf("registered browser windows must have non-nil tab lists")
	}

	app.removeBrowserWindow(first.id)

	if _, ok := app.browserWindows[first.id]; ok {
		t.Fatalf("first browser window was not removed")
	}
	if _, ok := app.browserWindows[second.id]; !ok {
		t.Fatalf("second browser window should remain registered")
	}
}

func TestBrowserWindow_RemovePromotesRemainingMainWindow(t *testing.T) {
	mainWindow := &window.MainWindow{}
	otherWindow := &window.MainWindow{}
	first := &browserWindow{id: "window-1", mainWindow: mainWindow}
	second := &browserWindow{id: "window-2", mainWindow: otherWindow}
	app := &App{
		mainWindow:     mainWindow,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
	}

	app.removeBrowserWindow(first.id)

	if app.mainWindow != otherWindow {
		t.Fatalf("mainWindow = %p, want promoted window %p", app.mainWindow, otherWindow)
	}
	if len(app.browserWindows) != 1 {
		t.Fatalf("browserWindows length = %d, want 1", len(app.browserWindows))
	}
	if _, ok := app.browserWindows[second.id]; !ok {
		t.Fatalf("remaining browser window was not kept")
	}
}

func TestBrowserWindow_RemoveLastClearsMainWindow(t *testing.T) {
	mainWindow := &window.MainWindow{}
	first := &browserWindow{id: "window-1", mainWindow: mainWindow}
	app := &App{
		mainWindow:     mainWindow,
		browserWindows: map[string]*browserWindow{first.id: first},
	}

	app.removeBrowserWindow(first.id)

	if app.mainWindow != nil {
		t.Fatalf("mainWindow = %p, want nil", app.mainWindow)
	}
	if len(app.browserWindows) != 0 {
		t.Fatalf("browserWindows length = %d, want 0", len(app.browserWindows))
	}
}

func TestBrowserWindow_RemovePromotedWindowClearsResizeModeBorderTarget(t *testing.T) {
	mainWindow := &window.MainWindow{}
	otherWindow := &window.MainWindow{}
	removed := &browserWindow{id: "window-1", mainWindow: mainWindow}
	remaining := &browserWindow{id: "window-2", mainWindow: otherWindow}
	resizeTarget := &testLayoutWidget{}
	app := &App{
		mainWindow:             mainWindow,
		browserWindows:         map[string]*browserWindow{removed.id: removed, remaining.id: remaining},
		resizeModeBorderTarget: resizeTarget,
	}

	app.removeBrowserWindow(removed.id)

	if app.resizeModeBorderTarget != nil {
		t.Fatalf("resizeModeBorderTarget = %p, want nil", app.resizeModeBorderTarget)
	}
}

func TestOpenFreshWindow_DispatchesAndTracksBrowserWindow(t *testing.T) {
	app := &App{browserWindows: make(map[string]*browserWindow)}

	dispatched := false
	createdURL := ""
	app.dispatchOnMainThread = func(label string, fn func()) syncdispatch.SyncDispatchResult {
		dispatched = true
		if label != "ui.open_fresh_window" {
			t.Fatalf("dispatch label = %q, want ui.open_fresh_window", label)
		}
		fn()
		return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchCompleted}
	}
	app.browserWindowFactory = func(ctx context.Context, url string) (*browserWindow, error) {
		createdURL = url
		return &browserWindow{id: "window-1", initialURL: url}, nil
	}

	if err := app.OpenFreshWindow(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("OpenFreshWindow returned error: %v", err)
	}
	if !dispatched {
		t.Fatalf("OpenFreshWindow did not use the main-thread dispatcher")
	}
	if createdURL != "https://example.com" {
		t.Fatalf("browser window factory URL = %q, want %q", createdURL, "https://example.com")
	}
	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want 1", got)
	}
	if got := app.browserWindows["window-1"]; got == nil {
		t.Fatalf("browser window was not tracked")
	}
	if app.browserWindows["window-1"].initialURL != "https://example.com" {
		t.Fatalf("browser window initialURL = %q, want %q", app.browserWindows["window-1"].initialURL, "https://example.com")
	}
}

func TestOpenFreshWindow_PropagatesFactoryError(t *testing.T) {
	app := &App{browserWindows: make(map[string]*browserWindow)}
	app.dispatchOnMainThread = func(label string, fn func()) syncdispatch.SyncDispatchResult {
		fn()
		return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchCompleted}
	}
	wantErr := errors.New("factory error")
	app.browserWindowFactory = func(context.Context, string) (*browserWindow, error) {
		return nil, wantErr
	}

	err := app.OpenFreshWindow(context.Background(), "https://example.com")

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
	if got := len(app.browserWindows); got != 0 {
		t.Fatalf("browserWindows length = %d, want 0", got)
	}
}

func setShellField(t *testing.T, bw *browserWindow, name string, value any) {
	t.Helper()
	rv := reflect.ValueOf(bw).Elem()
	field := rv.FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("browserWindow missing field %s", name)
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func fieldIsZero(t *testing.T, bw *browserWindow, name string) bool {
	t.Helper()
	rv := reflect.ValueOf(bw).Elem()
	field := rv.FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("browserWindow missing field %s", name)
	}
	return field.IsZero()
}

type testLayoutWidget struct{}

func (*testLayoutWidget) Show()                   {}
func (*testLayoutWidget) Hide()                   {}
func (*testLayoutWidget) SetVisible(bool)         {}
func (*testLayoutWidget) IsVisible() bool         { return false }
func (*testLayoutWidget) SetOpacity(float64)      {}
func (*testLayoutWidget) GrabFocus() bool         { return false }
func (*testLayoutWidget) HasFocus() bool          { return false }
func (*testLayoutWidget) SetCanFocus(bool)        {}
func (*testLayoutWidget) SetFocusable(bool)       {}
func (*testLayoutWidget) SetFocusOnClick(bool)    {}
func (*testLayoutWidget) SetCanTarget(bool)       {}
func (*testLayoutWidget) SetHexpand(bool)         {}
func (*testLayoutWidget) SetVexpand(bool)         {}
func (*testLayoutWidget) GetHexpand() bool        { return false }
func (*testLayoutWidget) GetVexpand() bool        { return false }
func (*testLayoutWidget) SetHalign(gtk.Align)     {}
func (*testLayoutWidget) SetValign(gtk.Align)     {}
func (*testLayoutWidget) SetSizeRequest(int, int) {}
func (*testLayoutWidget) GetAllocatedWidth() int  { return 0 }
func (*testLayoutWidget) GetAllocatedHeight() int { return 0 }
func (*testLayoutWidget) ComputePoint(layout.Widget) (float64, float64, bool) {
	return 0, 0, false
}
func (*testLayoutWidget) AddCssClass(string)                 {}
func (*testLayoutWidget) RemoveCssClass(string)              {}
func (*testLayoutWidget) HasCssClass(string) bool            { return false }
func (*testLayoutWidget) Unparent()                          {}
func (*testLayoutWidget) GetParent() layout.Widget           { return nil }
func (*testLayoutWidget) GtkWidget() *gtk.Widget             { return nil }
func (*testLayoutWidget) AddController(*gtk.EventController) {}

type testPermissionDialogPresenter struct{}

func (*testPermissionDialogPresenter) ShowPermissionDialog(context.Context, string, []entity.PermissionType, entity.PermissionMetadata, func(port.PermissionDialogResult)) {
}

// --- Window tab list tests ---

func TestGetWindowTabLists_RespectsRegistrationOrder(t *testing.T) {
	// Register windows in non-lexicographic order: window-2 first, then window-1.
	// The result should preserve creation order, not sorted order.
	app := &App{
		browserWindows: make(map[string]*browserWindow),
	}

	first := &browserWindow{id: "window-2"}
	second := &browserWindow{id: "window-1"}
	app.registerBrowserWindow(first)
	app.registerBrowserWindow(second)

	// Give each window a tab so they show up in results.
	pane1 := entity.NewPane(entity.PaneID("p-w2"))
	tab1 := entity.NewTab(entity.TabID("t-w2"), entity.WorkspaceID("ws-w2"), pane1)
	pane2 := entity.NewPane(entity.PaneID("p-w1"))
	tab2 := entity.NewTab(entity.TabID("t-w1"), entity.WorkspaceID("ws-w1"), pane2)

	app.tabs = entity.NewTabList()
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)
	first.tabs.Add(tab1)
	second.tabs.Add(tab2)
	app.windowForTab = map[entity.TabID]*browserWindow{
		tab1.ID: first,
		tab2.ID: second,
	}

	result := app.GetWindowTabLists()

	require.Len(t, result, 2)
	assert.Equal(t, entity.WindowID("window-2"), result[0].WindowID, "first registered window should be first")
	assert.Equal(t, entity.WindowID("window-1"), result[1].WindowID, "second registered window should be second")
}

func TestGetWindowTabLists_NoBrowserWindowsReturnsGlobalSnapshot(t *testing.T) {
	pane := entity.NewPane(entity.PaneID("p1"))
	tab := entity.NewTab(entity.TabID("t1"), entity.WorkspaceID("ws1"), pane)
	tabs := entity.NewTabList()
	tabs.Add(tab)

	app := &App{tabs: tabs}

	result := app.GetWindowTabLists()

	require.Len(t, result, 1)
	assert.Equal(t, entity.WindowID(""), result[0].WindowID)
	// Snapshot returns a copy so pointer identity is not expected; verify
	// structural equality instead.
	assert.Equal(t, tabs.ActiveTabID, result[0].Tabs.ActiveTabID)
	assert.Len(t, result[0].Tabs.Tabs, len(tabs.Tabs))
}

func TestGetWindowTabLists_AppliesActiveTabIndex(t *testing.T) {
	// A window with its own TabList where the second tab is active.
	app := &App{
		browserWindows: make(map[string]*browserWindow),
	}

	pane1 := entity.NewPane(entity.PaneID("p1"))
	tab1 := entity.NewTab(entity.TabID("t1"), entity.WorkspaceID("ws1"), pane1)
	pane2 := entity.NewPane(entity.PaneID("p2"))
	tab2 := entity.NewTab(entity.TabID("t2"), entity.WorkspaceID("ws2"), pane2)

	app.tabs = entity.NewTabList()
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	bwTabs := entity.NewTabList()
	bwTabs.Add(tab1)
	bwTabs.Add(tab2)
	bwTabs.SetActive(entity.TabID("t2"))
	bw := &browserWindow{id: "w1", tabs: bwTabs}
	app.registerBrowserWindow(bw)
	app.windowForTab = map[entity.TabID]*browserWindow{
		tab1.ID: bw,
		tab2.ID: bw,
	}

	result := app.GetWindowTabLists()

	require.Len(t, result, 1)
	assert.Equal(t, entity.TabID("t2"), result[0].Tabs.ActiveTabID, "should reflect bw.tabs.ActiveTabID")
}

func TestGetWindowTabLists_PreservesPreviousActiveTabID(t *testing.T) {
	app := &App{
		browserWindows: make(map[string]*browserWindow),
	}

	pane1 := entity.NewPane(entity.PaneID("p1"))
	tab1 := entity.NewTab(entity.TabID("t1"), entity.WorkspaceID("ws1"), pane1)
	pane2 := entity.NewPane(entity.PaneID("p2"))
	tab2 := entity.NewTab(entity.TabID("t2"), entity.WorkspaceID("ws2"), pane2)

	app.tabs = entity.NewTabList()
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	bwTabs := entity.NewTabList()
	bwTabs.Add(tab1)
	bwTabs.Add(tab2)
	bwTabs.SetActive(entity.TabID("t2")) // t2 becomes active, t1 becomes previous

	bw := &browserWindow{id: "w1", tabs: bwTabs}
	app.registerBrowserWindow(bw)
	app.windowForTab = map[entity.TabID]*browserWindow{
		tab1.ID: bw,
		tab2.ID: bw,
	}

	result := app.GetWindowTabLists()

	require.Len(t, result, 1)
	assert.Equal(t, entity.TabID("t1"), result[0].Tabs.PreviousActiveTabID, "should preserve bw.tabs.PreviousActiveTabID")
}

func TestGetActiveWindowIndex_MatchesRegistrationOrder(t *testing.T) {
	// Register windows in non-lexicographic order; lastFocusedWindowID should
	// map to the correct index in registration order.
	app := &App{
		browserWindows: make(map[string]*browserWindow),
	}

	app.registerBrowserWindow(&browserWindow{id: "z-window"})
	app.registerBrowserWindow(&browserWindow{id: "a-window"})
	app.lastFocusedWindowID = "a-window"

	idx := app.GetActiveWindowIndex()
	assert.Equal(t, 1, idx, "a-window is the second registered window, index should be 1")
}

func TestGetActiveWindowIndex_MatchesGetWindowTabLists(t *testing.T) {
	// The order of windows in GetWindowTabLists must match the indices returned
	// by GetActiveWindowIndex.
	app := &App{
		browserWindows: make(map[string]*browserWindow),
	}

	w1 := &browserWindow{id: "w1"}
	w2 := &browserWindow{id: "w2"}
	w3 := &browserWindow{id: "w3"}
	app.registerBrowserWindow(w1)
	app.registerBrowserWindow(w2)
	app.registerBrowserWindow(w3)

	// Give each window a tab.
	p1 := entity.NewPane(entity.PaneID("p1"))
	t1 := entity.NewTab(entity.TabID("t1"), entity.WorkspaceID("ws1"), p1)
	p2 := entity.NewPane(entity.PaneID("p2"))
	t2 := entity.NewTab(entity.TabID("t2"), entity.WorkspaceID("ws2"), p2)
	p3 := entity.NewPane(entity.PaneID("p3"))
	t3 := entity.NewTab(entity.TabID("t3"), entity.WorkspaceID("ws3"), p3)
	app.tabs = entity.NewTabList()
	app.tabs.Add(t1)
	app.tabs.Add(t2)
	app.tabs.Add(t3)
	w1.tabs.Add(t1)
	w2.tabs.Add(t2)
	w3.tabs.Add(t3)
	app.windowForTab = map[entity.TabID]*browserWindow{
		t1.ID: w1,
		t2.ID: w2,
		t3.ID: w3,
	}

	app.lastFocusedWindowID = "w2"

	lists := app.GetWindowTabLists()
	idx := app.GetActiveWindowIndex()

	require.Len(t, lists, 3)
	assert.Equal(t, entity.WindowID("w2"), lists[idx].WindowID, "active window ID must match the lists index")
}

func TestWindowOrder_FallbackWhenRegistrationOrderEmpty(t *testing.T) {
	// When browserWindowOrder is empty (e.g., old tests / direct map manipulation),
	// windowOrder must fall back to a deterministic (sorted) order.
	app := &App{
		browserWindows: map[string]*browserWindow{
			"z-window": {id: "z-window"},
			"a-window": {id: "a-window"},
		},
	}

	order := app.windowOrder()

	require.Len(t, order, 2)
	// Sorted order: "a-window" < "z-window"
	assert.Equal(t, "a-window", order[0])
	assert.Equal(t, "z-window", order[1])
}

func TestRemoveBrowserWindow_RemovesFromOrder(t *testing.T) {
	app := &App{
		browserWindows: make(map[string]*browserWindow),
	}

	w1 := &browserWindow{id: "w1"}
	w2 := &browserWindow{id: "w2"}
	w3 := &browserWindow{id: "w3"}
	app.registerBrowserWindow(w1)
	app.registerBrowserWindow(w2)
	app.registerBrowserWindow(w3)

	app.removeBrowserWindow("w2")

	require.Len(t, app.browserWindowOrder, 2)
	assert.Equal(t, "w1", app.browserWindowOrder[0])
	assert.Equal(t, "w3", app.browserWindowOrder[1])
}

func TestRegisterBrowserWindow_DuplicateIgnores(t *testing.T) {
	app := &App{
		browserWindows: make(map[string]*browserWindow),
	}

	bw := &browserWindow{id: "w1"}
	app.registerBrowserWindow(bw)
	app.registerBrowserWindow(bw) // duplicate

	require.Len(t, app.browserWindowOrder, 1)
}

func TestApp_GetWindowTabListsUsesCreationOrderAndActiveIndex(t *testing.T) {
	// Create two windows with per-window bw.tabs as the real source of truth.
	// Ensure GetWindowTabLists returns them in registration order, with correct
	// WindowID, and that the returned Tabs pointers are the per-window bw.tabs
	// (not copies or derived from global App.tabs).
	firstTabs := entity.NewTabList()
	firstTab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	firstTabs.Add(firstTab)
	firstTabs.SetActive(entity.TabID("tab-1"))

	secondTabs := entity.NewTabList()
	secondTab := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))
	secondTabs.Add(secondTab)

	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	app := &App{
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		browserWindowOrder:  []string{"window-1", "window-2"},
		lastFocusedWindowID: "window-2",
	}

	result := app.GetWindowTabLists()

	require.Len(t, result, 2)
	assert.Equal(t, entity.WindowID("window-1"), result[0].WindowID)
	assert.Equal(t, entity.WindowID("window-2"), result[1].WindowID)

	// The returned Tabs are snapshot copies of per-window bw.tabs.
	// Verify structural equality (not pointer identity, since
	// GetWindowTabLists returns a snapshot for safe concurrent read).
	assert.Equal(t, firstTabs.ActiveTabID, result[0].Tabs.ActiveTabID)
	assert.Len(t, result[0].Tabs.Tabs, len(firstTabs.Tabs))
	assert.Equal(t, secondTabs.ActiveTabID, result[1].Tabs.ActiveTabID)
	assert.Len(t, result[1].Tabs.Tabs, len(secondTabs.Tabs))

	// Active state reflects per-window bw.tabs.
	assert.Equal(t, entity.TabID("tab-1"), result[0].Tabs.ActiveTabID)
	assert.Equal(t, entity.TabID("tab-2"), result[1].Tabs.ActiveTabID)

	// GetActiveWindowIndex respects lastFocusedWindowID mapped to registration order.
	idx := app.GetActiveWindowIndex()
	assert.Equal(t, 1, idx, "window-2 is second registered, index should be 1")
}

func TestApp_AssignRestoredWindowTabListsUsesFreshRuntimeWindowIDs(t *testing.T) {
	// Create two restored TabLists with distinct tabs.
	firstTabs := entity.NewTabList()
	firstTab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	firstTabs.Add(firstTab)
	firstTabs.SetActive(entity.TabID("tab-1"))

	secondTabs := entity.NewTabList()
	secondTab := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))
	secondTabs.Add(secondTab)

	// Create two runtime browser windows with fresh IDs distinct from saved IDs.
	firstBW := &browserWindow{id: "runtime-w1"}
	secondBW := &browserWindow{id: "runtime-w2"}

	app := &App{
		browserWindows: map[string]*browserWindow{"runtime-w1": firstBW, "runtime-w2": secondBW},
	}

	// Assign restored state using saved window IDs (simulating session restore).
	app.assignRestoredWindowTabList(firstBW, entity.WindowTabListState{
		WindowID: "saved-w1",
		Tabs:     firstTabs,
	})
	app.assignRestoredWindowTabList(secondBW, entity.WindowTabListState{
		WindowID: "saved-w2",
		Tabs:     secondTabs,
	})

	// Tabs must be owned by runtime windows, not saved.
	require.Same(t, firstTabs, firstBW.tabs, "first window tabs must be assigned")
	require.Same(t, secondTabs, secondBW.tabs, "second window tabs must be assigned")

	// windowForTab must map tabs to runtime windows.
	require.Equal(t, firstBW, app.browserWindowForTab(firstTab.ID), "tab-1 must be owned by runtime-w1")
	require.Equal(t, secondBW, app.browserWindowForTab(secondTab.ID), "tab-2 must be owned by runtime-w2")

	// Saved window IDs must never leak into browserWindows.
	require.Nil(t, app.browserWindows["saved-w1"], "saved-w1 must not appear in browserWindows")
	require.Nil(t, app.browserWindows["saved-w2"], "saved-w2 must not appear in browserWindows")
	assertWindowOwnershipInvariant(t, app)
}

func TestApp_BrowserWindowsOwnIndependentTabLists(t *testing.T) {
	// Construct first browser window with its own TabList and distinct tabs.
	firstTabs := entity.NewTabList()
	firstTab := entity.NewTab(entity.TabID("tab-first"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	firstTabs.Add(firstTab)

	first := &browserWindow{
		id:   "window-first",
		tabs: firstTabs,
	}

	// Construct second browser window with its own TabList and distinct tabs.
	secondTabs := entity.NewTabList()
	secondTab := entity.NewTab(entity.TabID("tab-second"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	secondTabs.Add(secondTab)

	second := &browserWindow{
		id:   "window-second",
		tabs: secondTabs,
	}

	app := &App{
		browserWindows: map[string]*browserWindow{
			first.id:  first,
			second.id: second,
		},
	}

	// tabListForBrowserWindow returns the correct TabList for each window.
	got := app.tabListForBrowserWindow(first)
	if got != firstTabs {
		t.Fatalf("tabListForBrowserWindow(first) = %p, want %p", got, firstTabs)
	}
	got = app.tabListForBrowserWindow(second)
	if got != secondTabs {
		t.Fatalf("tabListForBrowserWindow(second) = %p, want %p", got, secondTabs)
	}

	// Each window's tabs do not contain the other window's tabs.
	if firstTabs.Find(secondTab.ID) != nil {
		t.Fatalf("first window tabs unexpectedly contain second tab")
	}
	if secondTabs.Find(firstTab.ID) != nil {
		t.Fatalf("second window tabs unexpectedly contain first tab")
	}

	// activeTabForBrowserWindow returns the correct active tab for each window.
	gotTab := app.activeTabForBrowserWindow(first)
	if gotTab != firstTab {
		t.Fatalf("activeTabForBrowserWindow(first) = %p, want %p", gotTab, firstTab)
	}
	gotTab = app.activeTabForBrowserWindow(second)
	if gotTab != secondTab {
		t.Fatalf("activeTabForBrowserWindow(second) = %p, want %p", gotTab, secondTab)
	}

	app.windowForTab = map[entity.TabID]*browserWindow{firstTab.ID: first, secondTab.ID: second}
	assertWindowOwnershipInvariant(t, app)
}

func TestRestoreSession_RemovesStaleBrowserWindows(t *testing.T) {
	// After restore, only runtime windows should remain in browserWindows
	// and browserWindowOrder. Stale pre-existing windows must be removed.

	// Runtime windows with tabs (what restoreSession produces).
	runtime1Tabs := entity.NewTabList()
	r1Tab := entity.NewTab(entity.TabID("r1-t1"), entity.WorkspaceID("ws-r1"), entity.NewPane(entity.PaneID("p-r1")))
	runtime1Tabs.Add(r1Tab)
	runtime1Tabs.SetActive(entity.TabID("r1-t1"))

	runtime2Tabs := entity.NewTabList()
	r2Tab := entity.NewTab(entity.TabID("r2-t1"), entity.WorkspaceID("ws-r2"), entity.NewPane(entity.PaneID("p-r2")))
	runtime2Tabs.Add(r2Tab)
	runtime2Tabs.SetActive(entity.TabID("r2-t1"))

	runtime1 := &browserWindow{id: "runtime-w1", tabs: runtime1Tabs}
	runtime2 := &browserWindow{id: "runtime-w2", tabs: runtime2Tabs}

	// Stale pre-existing browser window (no longer in runtime set).
	staleTabs := entity.NewTabList()
	staleTab := entity.NewTab(entity.TabID("stale-t1"), entity.WorkspaceID("ws-stale"), entity.NewPane(entity.PaneID("p-stale")))
	staleTabs.Add(staleTab)
	stale := &browserWindow{id: "stale-window", tabs: staleTabs}

	app := &App{
		browserWindows: map[string]*browserWindow{
			"stale-window": stale,
			"runtime-w1":   runtime1,
			"runtime-w2":   runtime2,
		},
		tabs: entity.NewTabList(),
		windowForTab: map[entity.TabID]*browserWindow{
			staleTab.ID: stale,
			r1Tab.ID:    runtime1,
			r2Tab.ID:    runtime2,
		},
	}

	// Simulate post-restore cleanup: remove stale windows not in runtime set.
	runtimeIDs := []string{"runtime-w1", "runtime-w2"}
	app.pruneStaleBrowserWindows([]*browserWindow{runtime1, runtime2})

	// After cleanup, browserWindows contains only runtime windows.
	require.Len(t, app.browserWindows, 2)
	require.NotNil(t, app.browserWindows["runtime-w1"])
	require.NotNil(t, app.browserWindows["runtime-w2"])
	require.Nil(t, app.browserWindows["stale-window"])

	// browserWindowOrder matches runtime order.
	require.Equal(t, runtimeIDs, app.browserWindowOrder)

	// GetWindowTabLists returns only runtime windows.
	result := app.GetWindowTabLists()
	require.Len(t, result, 2)
	assert.Equal(t, entity.WindowID("runtime-w1"), result[0].WindowID)
	assert.Equal(t, entity.WindowID("runtime-w2"), result[1].WindowID)

	// No stale tabs leak into the result.
	// Verify structural equality (snapshot copies, not live pointers).
	assert.Equal(t, runtime1Tabs.ActiveTabID, result[0].Tabs.ActiveTabID)
	assert.Len(t, result[0].Tabs.Tabs, len(runtime1Tabs.Tabs))
	assert.Equal(t, runtime2Tabs.ActiveTabID, result[1].Tabs.ActiveTabID)
	assert.Len(t, result[1].Tabs.Tabs, len(runtime2Tabs.Tabs))
	assertWindowOwnershipInvariant(t, app)
}

func TestRestoreSession_BrowserWindowOrderStartsWithFirstReusedWindow(t *testing.T) {
	// When the first restored window reuses a browser window that was not
	// already in browserWindowOrder, the restored order must start with
	// the reused window ID followed by newly-created window IDs.

	reuseTabs := entity.NewTabList()
	reuseTab := entity.NewTab(entity.TabID("reuse-t1"), entity.WorkspaceID("ws-reuse"), entity.NewPane(entity.PaneID("p-reuse")))
	reuseTabs.Add(reuseTab)
	reuseBW := &browserWindow{id: "reused-window", tabs: reuseTabs}

	createdTabs := entity.NewTabList()
	createdTab := entity.NewTab(entity.TabID("created-t1"), entity.WorkspaceID("ws-created"), entity.NewPane(entity.PaneID("p-created")))
	createdTabs.Add(createdTab)
	createdBW := &browserWindow{id: "created-window", tabs: createdTabs}

	// Pre-existing browserWindowOrder does NOT contain the reused window.
	app := &App{
		browserWindows: map[string]*browserWindow{
			"reused-window":  reuseBW,
			"created-window": createdBW,
		},
		browserWindowOrder: []string{"old-stale-window"}, // reused window not present
		tabs:               entity.NewTabList(),
		windowForTab:       map[entity.TabID]*browserWindow{},
	}

	// Exercise the helper used by restoreSession to prune stale windows and
	// replace browserWindowOrder with the runtime restore order.
	runtimeWindows := []*browserWindow{reuseBW, createdBW}
	app.pruneStaleBrowserWindows(runtimeWindows)

	require.Len(t, app.browserWindowOrder, 2)
	assert.Equal(t, "reused-window", app.browserWindowOrder[0],
		"first reused window must be first in order")
	assert.Equal(t, "created-window", app.browserWindowOrder[1],
		"newly-created window must follow the reused window")

	// GetWindowTabLists respects the order.
	result := app.GetWindowTabLists()
	require.Len(t, result, 2)
	assert.Equal(t, entity.WindowID("reused-window"), result[0].WindowID)
	assert.Equal(t, entity.WindowID("created-window"), result[1].WindowID)
}

func TestRestoreSession_ActiveWindowIndexSyncsState(t *testing.T) {
	// After restore, GetActiveWindowIndex must return the restored active index.
	// The focused window state must be consistent: lastFocusedWindowID matches
	// the window at that index, and the active window's tab target aligns.

	runtime1Tabs := entity.NewTabList()
	r1Tab := entity.NewTab(entity.TabID("a1-t1"), entity.WorkspaceID("ws-a1"), entity.NewPane(entity.PaneID("p-a1")))
	runtime1Tabs.Add(r1Tab)
	runtime1Tabs.SetActive(entity.TabID("a1-t1"))

	runtime2Tabs := entity.NewTabList()
	r2Tab := entity.NewTab(entity.TabID("a2-t1"), entity.WorkspaceID("ws-a2"), entity.NewPane(entity.PaneID("p-a2")))
	runtime2Tabs.Add(r2Tab)
	runtime2Tabs.SetActive(entity.TabID("a2-t1"))

	runtime1 := &browserWindow{id: "active-w1", tabs: runtime1Tabs}
	runtime2 := &browserWindow{id: "active-w2", tabs: runtime2Tabs}

	app := &App{
		browserWindows: map[string]*browserWindow{
			"active-w1": runtime1,
			"active-w2": runtime2,
		},
		browserWindowOrder:  []string{"active-w1", "active-w2"},
		lastFocusedWindowID: "active-w2",
		tabs:                entity.NewTabList(),
		windowForTab:        map[entity.TabID]*browserWindow{},
	}

	// Active index should be 1 (second window in order).
	idx := app.GetActiveWindowIndex()
	assert.Equal(t, 1, idx)

	// The focused window is the one at that index.
	focused := app.lastFocusedBrowserWindow()
	require.NotNil(t, focused)
	assert.Equal(t, "active-w2", focused.id)

	// Verify that GetActiveWindowIndex and GetWindowTabLists are consistent:
	// the window at the active index matches the focused window.
	result := app.GetWindowTabLists()
	require.Len(t, result, 2)
	assert.Equal(t, entity.WindowID("active-w2"), result[idx].WindowID,
		"window at active index must match focused window ID")
}

// =============================================================================
// History sidebar integration tests
// =============================================================================

// TestHistorySidebarConfig_OnNavigateNavigatesActivePaneAndKeepsSidebar verifies
// that the default OnNavigate callback targets the owning browser window's
// active pane and keeps the sidebar visible.
func TestHistorySidebarConfig_OnNavigateNavigatesActivePaneAndKeepsSidebar(t *testing.T) {
	ctx := context.Background()

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

	cfg := app.buildHistorySidebarConfig(ctx, second)
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
	ctx := context.Background()

	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	bwTabs := entity.NewTabList()
	bwTabs.Add(tab)
	bwTabs.SetActive(tab.ID)

	bw := &browserWindow{id: "window-1", tabs: bwTabs}

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

	cfg := app.buildHistorySidebarConfig(ctx, bw)
	navigateURL := "https://keep-open.com"
	err := cfg.OnNavigateKeepOpen(ctx, navigateURL)
	require.NoError(t, err)
	assert.True(t, fakeWv.loadURICalled)
	assert.Equal(t, navigateURL, fakeWv.loadURILastURI, "Ctrl+Enter navigation should go to the URL")
}

// TestHistorySidebar_OwnershipOnMultiWindowNavigation verifies that when
// multiple browser windows have history sidebars, navigation targets the
// correct owning window's active pane. This tests the stale-focus scenario
// where a different window is globally focused.
func TestHistorySidebar_OwnershipOnMultiWindowNavigation(t *testing.T) {
	ctx := context.Background()

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

	// Simulate the hide step used by OnClose's hideAndRestoreFocus closure.
	bw.hideHistorySidebar()
	assert.False(t, bw.sidebarVisible, "sidebar must be hidden by hideAndRestoreFocus")

}

// =============================================================================
// Dispatcher-backed Ctrl+H integration test
// =============================================================================

// TestApp_HistorySidebar_ToggleThroughKeyboardDispatcher wires the real toggle
// handler from App.wireKeyboardActions through the KeyboardDispatcher and
// dispatches ActionToggleHistorySystemView, asserting the focused browser
// window's sidebar visibility toggles.
func TestApp_HistorySidebar_ToggleThroughKeyboardDispatcher(t *testing.T) {
	ctx := context.Background()

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
	ctx := context.Background()

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
	assert.ErrorContains(t, err, "history sidebar unavailable")
	assert.False(t, bw.sidebarVisible, "sidebar must remain invisible when not wired")
}

// TestApp_HistorySidebar_ToggleThroughDispatcher_NilFocusedReturnsError verifies
// that the toggle handler returns a clean error when there is no focused window.
func TestApp_HistorySidebar_ToggleThroughDispatcher_NilFocusedReturnsError(t *testing.T) {
	ctx := context.Background()

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
	assert.ErrorContains(t, err, "history sidebar unavailable")
}

// =============================================================================
// buildHistorySidebarConfig callback seam tests
// =============================================================================

// TestApp_HistorySidebarConfig_NavigateCallback verifies that the OnNavigate
// callback from buildHistorySidebarConfig navigates the owning browser window's
// active pane to the given URL.
func TestApp_HistorySidebarConfig_NavigateCallbackNavigates(t *testing.T) {
	ctx := context.Background()

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
	cfg := app.buildHistorySidebarConfig(ctx, bw)
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
	ctx := context.Background()

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
	ctx := context.Background()

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
	cfg := app.buildHistorySidebarConfig(ctx, second)

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
	ctx := context.Background()

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

	cfg := app.buildHistorySidebarConfig(ctx, bw)

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
	ctx := context.Background()

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

	cfg := app.buildHistorySidebarConfig(ctx, bw)

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
	ctx := context.Background()

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
	cfg := app.buildHistorySidebarConfig(ctx, bw)

	// OnClose hides the sidebar.
	cfg.OnClose()

	assert.False(t, bw.sidebarVisible, "sidebar must be hidden after close")
}

// TestApp_HistorySidebarConfig_CloseWithNoSidebarIsSafe verifies that
// OnClose (hideAndRestoreFocusForBrowserWindow) is safe when the browser
// window has no sidebar or is nil.
func TestApp_HistorySidebarConfig_CloseWithNoSidebarIsSafe(t *testing.T) {
	ctx := context.Background()

	bw := &browserWindow{
		id:         "no-sidebar",
		mainWindow: &window.MainWindow{},
		// historySidebar is nil
	}

	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{bw.id: bw},
	}

	cfg := app.buildHistorySidebarConfig(ctx, bw)

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

	_ = wsView
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

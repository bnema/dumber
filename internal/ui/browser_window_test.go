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
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
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

type recordingWebViewPool struct {
	released []port.WebView
}

func (p *recordingWebViewPool) Acquire(context.Context) (port.WebView, error) {
	return nil, errors.New("not implemented")
}
func (p *recordingWebViewPool) Release(wv port.WebView)           { p.released = append(p.released, wv) }
func (p *recordingWebViewPool) Prewarm(int)                       {}
func (p *recordingWebViewPool) PrewarmAsync(context.Context, int) {}
func (p *recordingWebViewPool) Size() int                         { return 0 }
func (p *recordingWebViewPool) Close()                            {}

func TestTabCoordinator_CloseReleasesClosedTabWorkspaceWebViews(t *testing.T) {
	ctx := context.Background()
	closedTab := entity.NewTab(entity.TabID("tab-x"), entity.WorkspaceID("workspace-x"), entity.NewPane(entity.PaneID("pane-x")))
	survivingTab := entity.NewTab(entity.TabID("tab-survivor"), entity.WorkspaceID("workspace-survivor"), entity.NewPane(entity.PaneID("pane-survivor")))
	tabs := entity.NewTabList()
	tabs.Add(closedTab)
	tabs.Add(survivingTab)
	tabs.SetActive(closedTab.ID)

	pool := &recordingWebViewPool{}
	contentCoord := contentcoord.NewCoordinator(ctx, pool, nil, nil, nil, nil, nil, nil)
	closedWV := &recordingWebView{id: 101}
	survivingWV := &recordingWebView{id: 102}
	contentCoord.RegisterPopupWebView(closedTab.Workspace.ActivePaneID, closedWV)
	contentCoord.RegisterPopupWebView(survivingTab.Workspace.ActivePaneID, survivingWV)

	bw := &browserWindow{id: "window-1", tabs: tabs}
	app := &App{
		deps:             &Dependencies{},
		runtimeConfig:    runtimeConfigStateForTest(config.DefaultConfig()),
		tabsUC:           usecase.NewManageTabsUseCase(counterIDGen(), nil),
		contentCoord:     contentCoord,
		browserWindows:   map[string]*browserWindow{bw.id: bw},
		tabs:             entity.NewTabList(),
		workspaceViews:   map[entity.TabID]*component.WorkspaceView{closedTab.ID: {}, survivingTab.ID: {}},
		windowForTab:     map[entity.TabID]*browserWindow{closedTab.ID: bw, survivingTab.ID: bw},
		floatingSessions: map[floatingSessionKey]*floatingWorkspaceSession{},
	}
	app.tabs.Add(closedTab)
	app.tabs.Add(survivingTab)
	app.initTabCoordinator(ctx)

	require.NoError(t, app.tabCoord.Close(ctx, app.ensureTabTargetForBrowserWindow(bw)))

	require.Len(t, pool.released, 1)
	assert.Same(t, closedWV, pool.released[0])
	assert.Nil(t, contentCoord.GetWebView(closedTab.Workspace.ActivePaneID))
	assert.Same(t, survivingWV, contentCoord.GetWebView(survivingTab.Workspace.ActivePaneID))
	assert.Nil(t, app.workspaceViews[closedTab.ID])
	assert.Nil(t, app.windowForTab[closedTab.ID])
	assert.Nil(t, app.tabs.Find(closedTab.ID))
	assert.Equal(t, survivingTab.ID, tabs.ActiveTabID)
}

func TestTabCoordinator_SwitchDoesNotReleaseWorkspaceWebViews(t *testing.T) {
	ctx := context.Background()
	firstTab := entity.NewTab(entity.TabID("tab-first"), entity.WorkspaceID("workspace-first"), entity.NewPane(entity.PaneID("pane-first")))
	secondTab := entity.NewTab(entity.TabID("tab-second"), entity.WorkspaceID("workspace-second"), entity.NewPane(entity.PaneID("pane-second")))
	tabs := entity.NewTabList()
	tabs.Add(firstTab)
	tabs.Add(secondTab)
	tabs.SetActive(firstTab.ID)

	pool := &recordingWebViewPool{}
	contentCoord := contentcoord.NewCoordinator(ctx, pool, nil, nil, nil, nil, nil, nil)
	firstWV := &recordingWebView{id: 111}
	secondWV := &recordingWebView{id: 112}
	contentCoord.RegisterPopupWebView(firstTab.Workspace.ActivePaneID, firstWV)
	contentCoord.RegisterPopupWebView(secondTab.Workspace.ActivePaneID, secondWV)

	bw := &browserWindow{id: "window-1", tabs: tabs}
	app := &App{
		deps:             &Dependencies{},
		runtimeConfig:    runtimeConfigStateForTest(config.DefaultConfig()),
		tabsUC:           usecase.NewManageTabsUseCase(counterIDGen(), nil),
		contentCoord:     contentCoord,
		browserWindows:   map[string]*browserWindow{bw.id: bw},
		tabs:             entity.NewTabList(),
		workspaceViews:   map[entity.TabID]*component.WorkspaceView{firstTab.ID: {}, secondTab.ID: {}},
		windowForTab:     map[entity.TabID]*browserWindow{firstTab.ID: bw, secondTab.ID: bw},
		floatingSessions: map[floatingSessionKey]*floatingWorkspaceSession{},
	}
	app.tabs.Add(firstTab)
	app.tabs.Add(secondTab)
	app.initTabCoordinator(ctx)

	require.NoError(t, app.tabCoord.Switch(ctx, app.ensureTabTargetForBrowserWindow(bw), secondTab.ID))

	assert.Empty(t, pool.released)
	assert.Same(t, firstWV, contentCoord.GetWebView(firstTab.Workspace.ActivePaneID))
	assert.Same(t, secondWV, contentCoord.GetWebView(secondTab.Workspace.ActivePaneID))
}

func TestTabCoordinator_CloseLastTabReleasesWorkspaceBeforeWindowRemoval(t *testing.T) {
	ctx := context.Background()
	closedTab := entity.NewTab(entity.TabID("tab-only"), entity.WorkspaceID("workspace-only"), entity.NewPane(entity.PaneID("pane-only")))
	tabs := entity.NewTabList()
	tabs.Add(closedTab)
	tabs.SetActive(closedTab.ID)

	pool := &recordingWebViewPool{}
	contentCoord := contentcoord.NewCoordinator(ctx, pool, nil, nil, nil, nil, nil, nil)
	closedWV := &recordingWebView{id: 121}
	contentCoord.RegisterPopupWebView(closedTab.Workspace.ActivePaneID, closedWV)

	bw := &browserWindow{id: "window-1", tabs: tabs}
	app := &App{
		deps:             &Dependencies{},
		runtimeConfig:    runtimeConfigStateForTest(config.DefaultConfig()),
		tabsUC:           usecase.NewManageTabsUseCase(counterIDGen(), nil),
		contentCoord:     contentCoord,
		browserWindows:   map[string]*browserWindow{bw.id: bw},
		tabs:             entity.NewTabList(),
		workspaceViews:   map[entity.TabID]*component.WorkspaceView{closedTab.ID: {}},
		windowForTab:     map[entity.TabID]*browserWindow{closedTab.ID: bw},
		floatingSessions: map[floatingSessionKey]*floatingWorkspaceSession{},
	}
	app.tabs.Add(closedTab)
	app.initTabCoordinator(ctx)

	require.NoError(t, app.tabCoord.Close(ctx, app.ensureTabTargetForBrowserWindow(bw)))

	require.Len(t, pool.released, 1)
	assert.Same(t, closedWV, pool.released[0])
	assert.Nil(t, contentCoord.GetWebView(closedTab.Workspace.ActivePaneID))
	assert.Empty(t, app.browserWindows)
	assert.Nil(t, app.workspaceViews[closedTab.ID])
	assert.Nil(t, app.windowForTab[closedTab.ID])
}

func TestBrowserWindow_RemoveBrowserWindowReleasesOwnedTabWorkspaceWebViews(t *testing.T) {
	ctx := context.Background()
	ownedTab := entity.NewTab(entity.TabID("tab-owned"), entity.WorkspaceID("workspace-owned"), entity.NewPane(entity.PaneID("pane-owned")))
	otherTab := entity.NewTab(entity.TabID("tab-other"), entity.WorkspaceID("workspace-other"), entity.NewPane(entity.PaneID("pane-other")))
	removed := &browserWindow{id: "window-1", tabs: entity.NewTabList()}
	remaining := &browserWindow{id: "window-2", tabs: entity.NewTabList()}
	removed.tabs.Add(ownedTab)
	remaining.tabs.Add(otherTab)

	pool := &recordingWebViewPool{}
	contentCoord := contentcoord.NewCoordinator(ctx, pool, nil, nil, nil, nil, nil, nil)
	ownedWV := &recordingWebView{id: 201}
	otherWV := &recordingWebView{id: 202}
	contentCoord.RegisterPopupWebView(ownedTab.Workspace.ActivePaneID, ownedWV)
	contentCoord.RegisterPopupWebView(otherTab.Workspace.ActivePaneID, otherWV)

	app := &App{
		contentCoord:     contentCoord,
		browserWindows:   map[string]*browserWindow{removed.id: removed, remaining.id: remaining},
		tabs:             entity.NewTabList(),
		workspaceViews:   map[entity.TabID]*component.WorkspaceView{ownedTab.ID: {}, otherTab.ID: {}},
		windowForTab:     map[entity.TabID]*browserWindow{ownedTab.ID: removed, otherTab.ID: remaining},
		floatingSessions: map[floatingSessionKey]*floatingWorkspaceSession{},
	}
	app.tabs.Add(ownedTab)
	app.tabs.Add(otherTab)

	app.removeBrowserWindow(removed.id)

	require.Len(t, pool.released, 1)
	assert.Same(t, ownedWV, pool.released[0])
	assert.Nil(t, contentCoord.GetWebView(ownedTab.Workspace.ActivePaneID))
	assert.Same(t, otherWV, contentCoord.GetWebView(otherTab.Workspace.ActivePaneID))
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

func TestOpenFreshWindow_FirstWindowRegistersShellWithoutCreatingTab(t *testing.T) {
	app := &App{
		tabs:           entity.NewTabList(),
		tabsUC:         usecase.NewManageTabsUseCase(func() string { return "unexpected-tab" }, nil),
		browserWindows: make(map[string]*browserWindow),
	}
	app.dispatchOnMainThread = func(label string, fn func()) syncdispatch.SyncDispatchResult {
		fn()
		return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchCompleted}
	}
	app.browserWindowFactory = func(context.Context, string) (*browserWindow, error) {
		return &browserWindow{id: "window-1", tabs: entity.NewTabList()}, nil
	}

	if err := app.OpenFreshWindow(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("OpenFreshWindow returned error: %v", err)
	}
	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want 1", got)
	}
	if app.lastFocusedWindowID != "window-1" {
		t.Fatalf("lastFocusedWindowID = %q, want window-1", app.lastFocusedWindowID)
	}
	if got := app.tabs.Count(); got != 0 {
		t.Fatalf("tabs count = %d, want 0; first shell should wait for createInitialTab", got)
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

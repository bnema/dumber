package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/dispatcher"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	layoutmocks "github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/gtk"
)

type testBrowserLaunchRelay struct {
	listenCalls int
	closer      *testCloser
}

func (r *testBrowserLaunchRelay) DeliverOpenFreshWindow(context.Context, string) (bool, error) {
	return false, nil
}

func (r *testBrowserLaunchRelay) Listen(_ context.Context, opener port.BrowserWindowOpener) (io.Closer, error) {
	r.listenCalls++
	_ = opener
	return r.closer, nil
}

type testCloser struct {
	closed bool
}

func (c *testCloser) Close() error {
	c.closed = true
	return nil
}

type fakeSessionStateRepo struct {
	state *entity.SessionState
}

var _ repository.SessionStateRepository = (*fakeSessionStateRepo)(nil)

func (r *fakeSessionStateRepo) SaveSnapshot(context.Context, *entity.SessionState) error { return nil }

func (r *fakeSessionStateRepo) GetSnapshot(context.Context, entity.SessionID) (*entity.SessionState, error) {
	return r.state, nil
}

func (r *fakeSessionStateRepo) DeleteSnapshot(context.Context, entity.SessionID) error { return nil }

func (r *fakeSessionStateRepo) GetAllSnapshots(context.Context) ([]*entity.SessionState, error) {
	return nil, nil
}

func (r *fakeSessionStateRepo) GetTotalSnapshotsSize(context.Context) (int64, error) { return 0, nil }

func testHasUsableGTKDisplay() bool {
	if waylandDisplay := os.Getenv("WAYLAND_DISPLAY"); waylandDisplay != "" {
		candidates := []string{waylandDisplay}
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" && !filepath.IsAbs(waylandDisplay) {
			candidates = append([]string{filepath.Join(runtimeDir, waylandDisplay)}, candidates...)
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return true
			}
		}
	}

	if display := os.Getenv("DISPLAY"); display != "" {
		if strings.HasPrefix(display, ":") {
			displayNum := strings.TrimPrefix(display, ":")
			displayNum = strings.SplitN(displayNum, ".", 2)[0]
			if displayNum == "" {
				return false
			}
			x11SocketCandidates := []string{filepath.Join(os.TempDir(), ".X11-unix", "X"+displayNum)}
			if fallback := "/tmp/.X11-unix/X" + displayNum; fallback != x11SocketCandidates[0] {
				x11SocketCandidates = append(x11SocketCandidates, fallback)
			}
			for _, candidate := range x11SocketCandidates {
				if _, err := os.Stat(candidate); err == nil {
					return true
				}
			}
			return false
		}
		return true
	}

	return false
}

func requireGTKDisplayApp(t *testing.T) *gtk.Application {
	t.Helper()
	if !testHasUsableGTKDisplay() {
		t.Skip("GTK display not available")
	}
	EnsureAdwaitaInitialized()
	if gdk.DisplayGetDefault() == nil {
		t.Skip("GTK display not available")
	}
	appID := AppID
	gtkApp := gtk.NewApplication(&appID, gio.GApplicationNonUniqueValue)
	if gtkApp == nil {
		t.Fatal("gtk application creation failed")
	}
	return gtkApp
}

// setDependencyField uses reflection to reach unexported dependency fields for tests.
// Keep this test-only so production visibility stays narrow.
func setDependencyField(t *testing.T, deps *Dependencies, field string, value any) {
	t.Helper()

	rv := reflect.ValueOf(deps).Elem()
	fv := rv.FieldByName(field)
	if !fv.IsValid() {
		t.Fatalf("Dependencies missing %s field", field)
	}
	fv.Set(reflect.ValueOf(value))
}

// windowForTabCount uses reflection to inspect unexported app state in tests.
// Keep this test-only so production visibility stays narrow.
func windowForTabCount(t *testing.T, app *App) int {
	t.Helper()

	rv := reflect.ValueOf(app).Elem()
	fv := rv.FieldByName("windowForTab")
	if !fv.IsValid() {
		t.Fatalf("App missing windowForTab field")
	}
	return fv.Len()
}

func assertWindowOwnershipInvariant(t *testing.T, app *App) {
	t.Helper()

	seen := make(map[entity.TabID]string)
	for windowID, bw := range app.browserWindows {
		if bw == nil {
			continue
		}
		if bw.tabs == nil {
			t.Errorf("browser window %s has nil tabs", windowID)
			continue
		}
		for _, tab := range bw.tabs.Tabs {
			if tab == nil {
				continue
			}
			if previous, exists := seen[tab.ID]; exists {
				t.Errorf("tab %s is owned by both %s and %s", tab.ID, previous, windowID)
				continue
			}
			seen[tab.ID] = windowID
			if got := app.windowForTab[tab.ID]; got != bw {
				t.Errorf("windowForTab[%s] = %p, want owner window %s (%p)", tab.ID, got, windowID, bw)
			}
		}
	}
	for tabID, bw := range app.windowForTab {
		if bw == nil || bw.tabs == nil || bw.tabs.Find(tabID) == nil {
			t.Errorf("windowForTab[%s] points to stale or non-owning window %p", tabID, bw)
		}
	}
}

// tabCoordinatorMainWindowPtr uses reflection to inspect unexported coordinator state in tests.
// Keep this test-only so production visibility stays narrow.
func tabCoordinatorMainWindowPtr(t *testing.T, tc *coordinator.TabCoordinator) uintptr {
	t.Helper()

	rv := reflect.ValueOf(tc).Elem()
	fv := rv.FieldByName("mainWindow")
	if !fv.IsValid() {
		t.Fatalf("TabCoordinator missing mainWindow field")
	}
	return fv.Pointer()
}

func setWindowTabBar(t *testing.T, mw *window.MainWindow, tabBar *component.TabBar) {
	t.Helper()

	rv := reflect.ValueOf(mw).Elem()
	fv := rv.FieldByName("tabBar")
	if !fv.IsValid() {
		t.Fatalf("MainWindow missing tabBar field")
	}
	reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().Set(reflect.ValueOf(tabBar))
}

func omniboxNavigateCallbackForTest(t *testing.T, omnibox *component.Omnibox) func(string) {
	t.Helper()
	if omnibox == nil {
		t.Fatal("omnibox is nil")
	}

	rv := reflect.ValueOf(omnibox).Elem()
	fv := rv.FieldByName("onNavigate")
	if !fv.IsValid() {
		t.Fatal("Omnibox missing onNavigate field")
	}
	cb, ok := reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().Interface().(func(string))
	if !ok || cb == nil {
		t.Fatal("omnibox onNavigate callback is nil or has unexpected type")
	}
	return cb
}

func newTestTabBarShell(t *testing.T, tabIDs ...entity.TabID) *component.TabBar {
	t.Helper()

	tabBar := &component.TabBar{}
	buttons := make(map[entity.TabID]*component.TabButton, len(tabIDs))
	for _, tabID := range tabIDs {
		buttons[tabID] = &component.TabButton{}
	}

	rv := reflect.ValueOf(tabBar).Elem()
	fv := rv.FieldByName("buttons")
	if !fv.IsValid() {
		t.Fatalf("TabBar missing buttons field")
	}
	reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().Set(reflect.ValueOf(buttons))

	return tabBar
}

func windowTabBarActiveID(t *testing.T, mw *window.MainWindow) entity.TabID {
	t.Helper()
	if mw == nil || mw.TabBar() == nil {
		return ""
	}
	return mw.TabBar().ActiveTabID()
}

func windowTitle(t *testing.T, mw *window.MainWindow) string {
	t.Helper()
	if mw == nil || mw.Window() == nil {
		return ""
	}
	return mw.Window().GetTitle()
}

func windowTabBarVisible(t *testing.T, mw *window.MainWindow) bool {
	t.Helper()
	if mw == nil || mw.TabBar() == nil || mw.TabBar().Box() == nil {
		return false
	}
	return mw.TabBar().Box().GetVisible()
}

func assertWindowTabBarAutoHidden(t *testing.T, mw *window.MainWindow, wantHidden bool) {
	t.Helper()
	if mw == nil || mw.TabBar() == nil || mw.TabBar().Box() == nil {
		t.Fatal("missing tab bar")
	}
	box := mw.TabBar().Box()
	if got := box.GetVisible(); !got {
		t.Fatalf("tab bar visible = %v, want true so allocation is preserved", got)
	}
	if wantHidden {
		if got := box.GetOpacity(); got != 0.0 {
			t.Fatalf("tab bar opacity = %v, want 0", got)
		}
		if got := box.GetCanTarget(); got {
			t.Fatalf("tab bar can target = %v, want false while auto-hidden", got)
		}
		return
	}
	if got := box.GetOpacity(); got != 1.0 {
		t.Fatalf("tab bar opacity = %v, want 1", got)
	}
	if got := box.GetCanTarget(); !got {
		t.Fatalf("tab bar can target = %v, want true while shown", got)
	}
}

func stackedViewOnActivateIsNil(t *testing.T, sv *layout.StackedView) bool {
	t.Helper()
	if sv == nil {
		return true
	}

	rv := reflect.ValueOf(sv).Elem()
	fv := rv.FieldByName("onActivate")
	if !fv.IsValid() {
		t.Fatalf("StackedView missing onActivate field")
	}
	return reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().IsNil()
}

func newTestShellToaster(t *testing.T) (*component.Toaster, *layoutmocks.MockBoxWidget, *layoutmocks.MockLabelWidget) {
	t.Helper()

	factory := layoutmocks.NewMockWidgetFactory(t)
	box := layoutmocks.NewMockBoxWidget(t)
	label := layoutmocks.NewMockLabelWidget(t)

	factory.EXPECT().NewBox(layout.OrientationHorizontal, 0).Return(box).Once()
	factory.EXPECT().NewLabel("").Return(label).Once()
	box.EXPECT().AddCssClass("toast").Once()
	box.EXPECT().AddCssClass("toast-info").Once()
	box.EXPECT().SetHalign(gtk.AlignStartValue).Twice()
	box.EXPECT().SetValign(gtk.AlignStartValue).Twice()
	box.EXPECT().SetHexpand(false).Once()
	box.EXPECT().SetVexpand(false).Once()
	box.EXPECT().SetCanTarget(false).Once()
	box.EXPECT().SetCanFocus(false).Once()
	box.EXPECT().SetVisible(false).Once()
	box.EXPECT().Append(label).Once()
	label.EXPECT().SetCanTarget(false).Once()
	label.EXPECT().SetCanFocus(false).Once()

	return component.NewToaster(factory), box, label
}

func TestApp_ShowFilterStatusUsesLastFocusedBrowserWindowToaster(t *testing.T) {
	ctx := context.Background()
	toaster, box, label := newTestShellToaster(t)
	box.EXPECT().SetVisible(true).Once()
	label.EXPECT().SetText("Ad blocker loading").Once()

	bw := &browserWindow{id: "window-1", appToaster: toaster}
	app := &App{
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}

	app.showFilterStatus(ctx, port.FilterStatus{State: port.FilterStateLoading, Message: "Ad blocker loading"})
}

func TestApp_CheckConfigMigrationUsesLastFocusedBrowserWindowToaster(t *testing.T) {
	ctx := context.Background()
	toaster, box, label := newTestShellToaster(t)
	box.EXPECT().SetVisible(true).Once()
	label.EXPECT().SetText("Config has 1 new settings. Run 'dumber config migrate'").Once()

	migrator := portmocks.NewMockConfigMigrator(t)
	migrator.EXPECT().CheckMigration().Return(&port.MigrationResult{MissingKeys: []string{"update.notify_on_new_settings"}}, nil).Once()

	bw := &browserWindow{id: "window-1", appToaster: toaster}
	app := &App{
		deps: &Dependencies{
			Config: &config.Config{
				Update: config.UpdateConfig{NotifyOnNewSettings: true},
			},
			ConfigMigrator: migrator,
		},
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}

	app.checkConfigMigration(ctx)
}

func TestApp_FinalizeActivationStartsBrowserLaunchRelayOnceAndClosesOnShutdown(t *testing.T) {
	relay := &testBrowserLaunchRelay{closer: &testCloser{}}
	deps := &Dependencies{}
	setDependencyField(t, deps, "BrowserLaunchRelay", relay)

	app := &App{deps: deps, cancel: func(error) {}}

	app.finalizeActivation(context.Background())
	app.finalizeActivation(context.Background())

	if relay.listenCalls != 1 {
		t.Fatalf("Listen calls = %d, want 1", relay.listenCalls)
	}

	app.onShutdown(context.Background())

	if !relay.closer.closed {
		t.Fatalf("relay listener closer was not closed")
	}
}

func TestApp_OpenFreshWindowRecordsTabOwnership(t *testing.T) {
	existingTab := entity.NewTab(entity.TabID("existing-tab"), entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane")))
	existingTabs := entity.NewTabList()
	existingTabs.Add(existingTab)
	existingTabs.SetActive(existingTab.ID)
	existingWindow := &browserWindow{id: "existing-window", tabs: existingTabs}
	created := &browserWindow{id: "window-1", tabs: entity.NewTabList()}
	app := &App{
		tabs:           entity.NewTabList(),
		tabsUC:         usecase.NewManageTabsUseCase(func() string { return "id-1" }),
		browserWindows: map[string]*browserWindow{existingWindow.id: existingWindow},
		windowForTab:   map[entity.TabID]*browserWindow{existingTab.ID: existingWindow},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{entity.TabID("id-1"): &component.WorkspaceView{}},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return created, nil
		},
	}

	if err := app.OpenFreshWindow(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("OpenFreshWindow returned error: %v", err)
	}
	if got := windowForTabCount(t, app); got != 2 {
		t.Fatalf("windowForTab length = %d, want 2 (existing + new tab)", got)
	}
	assertWindowOwnershipInvariant(t, app)
}

func TestApp_RestoreSessionHandlesEmptyWindowSnapshotAsSuccess(t *testing.T) {
	ctx := context.Background()
	sessionID := entity.SessionID("session-empty-windows")
	staleTab := entity.NewTab(entity.TabID("stale-tab"), entity.WorkspaceID("stale-workspace"), entity.NewPane(entity.PaneID("stale-pane")))
	staleTabs := entity.NewTabList()
	staleTabs.Add(staleTab)
	staleWindow := &browserWindow{id: "window-stale", tabs: staleTabs}
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{}, 0, time.Unix(123, 0))

	app := &App{
		deps: &Dependencies{
			Config:           &config.Config{},
			SessionStateRepo: &fakeSessionStateRepo{state: state},
		},
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{staleWindow.id: staleWindow},
		lastFocusedWindowID: staleWindow.id,
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{staleTab.ID: {}},
		windowForTab:        map[entity.TabID]*browserWindow{staleTab.ID: staleWindow},
	}
	app.tabs.Add(staleTab)

	err := app.restoreSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("restoreSession returned error for valid v2 empty window snapshot: %v", err)
	}
	// Stale state should be cleared and stale window should have an empty tab list.
	if _, ok := app.workspaceViews[staleTab.ID]; ok {
		t.Fatal("stale workspace view was not cleared")
	}
	if _, ok := app.windowForTab[staleTab.ID]; ok {
		t.Fatal("stale windowForTab entry was not cleared")
	}
	if got := app.tabs.Count(); got != 0 {
		t.Fatalf("app.tabs.Count() = %d, want 0 after empty restore", got)
	}
	if got := staleWindow.tabs.Count(); got != 0 {
		t.Fatalf("staleWindow.tabs.Count() = %d, want 0 after empty restore", got)
	}
}

func TestApp_CreateInitialTabNoFallbackAfterEmptyWindowRestore(t *testing.T) {
	ctx := context.Background()
	sessionID := entity.SessionID("session-empty-windows-fallback")
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{}, 0, time.Unix(123, 0))
	tabsUC := usecase.NewManageTabsUseCase(func() string { return "fallback-tab" })
	windowTabs := entity.NewTabList()
	bw := &browserWindow{id: "window-1", tabs: windowTabs}
	app := &App{
		deps: &Dependencies{
			Config:           &config.Config{},
			RestoreSessionID: string(sessionID),
			InitialURL:       "https://fallback.example",
			SessionStateRepo: &fakeSessionStateRepo{state: state},
		},
		tabs:                entity.NewTabList(),
		tabsUC:              tabsUC,
		tabCoord:            coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{TabsUC: tabsUC, Tabs: entity.NewTabList()}),
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}

	app.createInitialTab(ctx)

	// For valid v2 empty-window restore, no fallback tab should be created.
	if created := windowTabs.Find(entity.TabID("fallback-tab")); created != nil {
		t.Fatal("fallback tab was created despite valid v2 empty-window restore")
	}
	if got := app.tabs.Count(); got != 0 {
		t.Fatalf("app.tabs.Count() = %d, want 0 after empty restore", got)
	}
}

func TestApp_RestoreSessionClearsStaleUIStateBeforeApplyingRestoredTabs(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	staleTab := entity.NewTab(entity.TabID("stale-tab"), entity.WorkspaceID("stale-workspace"), entity.NewPane(entity.PaneID("stale-pane")))
	staleWindow := &browserWindow{id: "window-stale", mainWindow: mainWindow}
	staleSessionKey := floatingSessionKey{tabID: staleTab.ID, sessionID: "profile-stale"}
	restoredSessionID := entity.SessionID("session-restore")
	restoredTabs := entity.NewTabList()
	restoredTabs.Add(entity.NewTab(entity.TabID("restored-tab"), entity.WorkspaceID("restored-workspace"), entity.NewPane(entity.PaneID("restored-pane"))))

	app := &App{
		deps: &Dependencies{
			Config:           &config.Config{},
			SessionStateRepo: &fakeSessionStateRepo{state: entity.SnapshotFromTabList(restoredSessionID, restoredTabs)},
		},
		mainWindow:     mainWindow,
		widgetFactory:  layout.NewGtkWidgetFactory(),
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{staleWindow.id: staleWindow},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{staleTab.ID: &component.WorkspaceView{}},
		windowForTab:   map[entity.TabID]*browserWindow{staleTab.ID: staleWindow},
		floatingSessions: map[floatingSessionKey]*floatingWorkspaceSession{
			staleSessionKey: {},
		},
	}
	app.tabs.Add(staleTab)
	mainWindow.TabBar().AddTab(staleTab)

	if err := app.restoreSession(context.Background(), restoredSessionID); err != nil {
		t.Fatalf("restoreSession returned error: %v", err)
	}

	if got := app.tabs.Count(); got != 1 {
		t.Fatalf("tabs.Count() = %d, want 1", got)
	}
	if got := mainWindow.TabBar().Count(); got != 1 {
		t.Fatalf("tab bar count = %d, want 1", got)
	}
	if _, ok := app.workspaceViews[staleTab.ID]; ok {
		t.Fatalf("stale workspace view was not removed")
	}
	if _, ok := app.windowForTab[staleTab.ID]; ok {
		t.Fatalf("stale windowForTab entry was not removed")
	}
	if _, ok := app.floatingSessions[staleSessionKey]; ok {
		t.Fatalf("stale floating session was not removed")
	}
	assertWindowOwnershipInvariant(t, app)
}

func TestApp_RestoreSessionWiresStackedPaneTitleBarCallbacks(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	stackedSessionID := entity.SessionID("session-stacked-restore")
	stackedTabs := entity.NewTabList()
	stackedTabs.Add(entity.NewTab(entity.TabID("restored-tab"), entity.WorkspaceID("restored-workspace"), entity.NewPane(entity.PaneID("restored-pane-1"))))
	stackedTabs.Tabs[0].Workspace = &entity.Workspace{
		ID: entity.WorkspaceID("restored-workspace"),
		Root: &entity.PaneNode{
			ID:        "restored-stack-root",
			IsStacked: true,
			Children: []*entity.PaneNode{
				{ID: "restored-child-1", Pane: entity.NewPane(entity.PaneID("restored-pane-1"))},
				{ID: "restored-child-2", Pane: entity.NewPane(entity.PaneID("restored-pane-2"))},
			},
			ActiveStackIndex: 0,
		},
		ActivePaneID: entity.PaneID("restored-pane-1"),
	}

	app := &App{
		deps: &Dependencies{
			Config:           &config.Config{},
			SessionStateRepo: &fakeSessionStateRepo{state: entity.SnapshotFromTabList(stackedSessionID, stackedTabs)},
		},
		mainWindow:     mainWindow,
		widgetFactory:  layout.NewGtkWidgetFactory(),
		contentCoord:   &contentcoord.Coordinator{},
		wsCoord:        coordinator.NewWorkspaceCoordinator(context.Background(), coordinator.WorkspaceCoordinatorConfig{ContentCoord: &contentcoord.Coordinator{}}),
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{},
		windowForTab:   map[entity.TabID]*browserWindow{},
	}

	if err := app.restoreSession(context.Background(), stackedSessionID); err != nil {
		t.Fatalf("restoreSession returned error: %v", err)
	}

	restoredTab := app.tabs.ActiveTab()
	if restoredTab == nil {
		t.Fatal("restored active tab missing")
	}
	wv := app.workspaceViews[restoredTab.ID]
	if wv == nil {
		t.Fatal("restored workspace view missing")
	}
	tr := wv.TreeRenderer()
	if tr == nil {
		t.Fatal("restored tree renderer missing")
	}
	if restoredTab.Workspace == nil || restoredTab.Workspace.Root == nil || len(restoredTab.Workspace.Root.Children) == 0 {
		t.Fatal("restored stacked workspace missing children")
	}
	stackedView := tr.GetStackedViewForPane(string(restoredTab.Workspace.Root.Children[0].Pane.ID))
	if stackedView == nil {
		t.Fatal("restored stacked view missing")
	}
	if stackedViewOnActivateIsNil(t, stackedView) {
		t.Fatal("restored stacked view onActivate callback is nil")
	}
}

func TestApp_RemoveBrowserWindowRebindsPromotedTabCoordinatorWindow(t *testing.T) {
	oldWindow := &browserWindow{id: "window-1", mainWindow: &window.MainWindow{}}
	newWindow := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	tc := coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{
		MainWindow: oldWindow.mainWindow,
	})
	app := &App{
		mainWindow:     oldWindow.mainWindow,
		browserWindows: map[string]*browserWindow{oldWindow.id: oldWindow, newWindow.id: newWindow},
		tabCoord:       tc,
	}

	app.removeBrowserWindow(oldWindow.id)

	if app.mainWindow != newWindow.mainWindow {
		t.Fatalf("mainWindow = %p, want %p", app.mainWindow, newWindow.mainWindow)
	}
	if got := tabCoordinatorMainWindowPtr(t, tc); got != reflect.ValueOf(newWindow.mainWindow).Pointer() {
		t.Fatalf("tab coordinator mainWindow = %x, want %x", got, reflect.ValueOf(newWindow.mainWindow).Pointer())
	}
}

func TestApp_OpenFreshWindowRollsBackOnTabCreationFailure(t *testing.T) {
	created := &browserWindow{id: "window-1", tabs: entity.NewTabList()}
	originalWindow := &window.MainWindow{}
	tabBar := &component.TabBar{}
	setWindowTabBar(t, originalWindow, tabBar)
	existingTabID := entity.TabID("existing-tab")
	staleTabID := entity.TabID("stale-tab")
	tabBar.SetActive(staleTabID)

	existingTab := entity.NewTab(existingTabID, entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane")))
	existingTabs := entity.NewTabList()
	existingTabs.Add(existingTab)
	existingTabs.SetActive(existingTab.ID)
	existingWindow := &browserWindow{id: "existing-window", tabs: existingTabs, mainWindow: originalWindow}

	app := &App{
		tabs:           entity.NewTabList(),
		tabsUC:         usecase.NewManageTabsUseCase(func() string { return "id-1" }),
		browserWindows: map[string]*browserWindow{existingWindow.id: existingWindow},
		windowForTab:   map[entity.TabID]*browserWindow{existingTab.ID: existingWindow},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return created, nil
		},
		mainWindow: originalWindow,
	}
	app.tabCoord = coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{
		TabsUC:     app.tabsUC,
		Tabs:       entity.NewTabList(),
		MainWindow: &window.MainWindow{},
	})

	if err := app.OpenFreshWindow(context.Background(), "https://example.com/fail"); err == nil {
		t.Fatalf("OpenFreshWindow = nil error, want failure")
	}
	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want 1 (existing window only)", got)
	}
	if app.browserWindows[existingWindow.id] == nil {
		t.Fatal("existing window should remain after rollback")
	}
	if app.browserWindows["window-1"] != nil {
		t.Fatal("created window should be removed after rollback")
	}
	if got := windowForTabCount(t, app); got != 1 {
		t.Fatalf("windowForTab length = %d, want 1 (existing tab only)", got)
	}
	if got := windowTabBarActiveID(t, originalWindow); got != staleTabID {
		t.Fatalf("tab bar active tab = %q, want %q (original window unchanged)", got, staleTabID)
	}
}

func TestApp_OpenFreshWindowTargetsNewWindowTabBar(t *testing.T) {
	existingTabID := entity.TabID("existing-tab")
	createdTabID := entity.TabID("id-1")
	oldWindow := &window.MainWindow{}
	newWindow := &window.MainWindow{}
	setWindowTabBar(t, oldWindow, newTestTabBarShell(t))
	setWindowTabBar(t, newWindow, newTestTabBarShell(t, createdTabID))
	tabs := entity.NewTabList()
	tabsUC := usecase.NewManageTabsUseCase(func() string { return string(createdTabID) })

	existingTab := entity.NewTab(existingTabID, entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane")))
	existingTabs := entity.NewTabList()
	existingTabs.Add(existingTab)
	existingTabs.SetActive(existingTab.ID)
	existingWindow := &browserWindow{id: "existing-window", tabs: existingTabs, mainWindow: oldWindow}

	app := &App{
		tabs:           tabs,
		tabsUC:         tabsUC,
		browserWindows: map[string]*browserWindow{existingWindow.id: existingWindow},
		windowForTab:   map[entity.TabID]*browserWindow{existingTab.ID: existingWindow},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return &browserWindow{id: "window-1", mainWindow: newWindow}, nil
		},
		mainWindow: oldWindow,
		tabCoord: coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{
			TabsUC:                  tabsUC,
			Tabs:                    tabs,
			MainWindow:              oldWindow,
			HideTabBarWhenSingleTab: true,
		}),
		workspaceViews: make(map[entity.TabID]*component.WorkspaceView),
	}
	app.tabCoord.SetOnTabCreated(func(ctx context.Context, target coordinator.TabTarget, tab *entity.Tab) {
		app.workspaceViews[tab.ID] = &component.WorkspaceView{}
	})

	if err := app.OpenFreshWindow(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("OpenFreshWindow returned error: %v", err)
	}

	gotCreatedTabID := app.tabs.ActiveTabID
	if gotCreatedTabID == "" {
		t.Fatalf("created tab id = %q, want non-empty", gotCreatedTabID)
	}
	if got := windowTabBarActiveID(t, newWindow); got != gotCreatedTabID {
		t.Fatalf("new window tab bar active tab = %q, want %q", got, gotCreatedTabID)
	}
	if got := windowTabBarActiveID(t, oldWindow); got != "" {
		t.Fatalf("old window tab bar active tab = %q, want empty", got)
	}
	// The lightweight test tab bar shell has no GTK box, so allocation state is
	// covered by TestApp_UpdateBrowserWindowTabBarVisibilityAutoHidesSingleTabButPreservesAllocation.
}

func TestApp_ActivateBrowserWindowSwitchesActiveWorkspace(t *testing.T) {
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	ws1 := &component.WorkspaceView{}
	ws2 := &component.WorkspaceView{}
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs, mainWindow: &window.MainWindow{}}
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs, mainWindow: &window.MainWindow{}}
	kh := &input.KeyboardHandler{}
	gs := &input.GlobalShortcutHandler{}
	second.keyboardHandler = kh
	second.globalShortcutHandler = gs
	app := &App{
		tabs:           tabs,
		mainWindow:     first.mainWindow,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:   map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{tab1.ID: ws1, tab2.ID: ws2},
	}

	app.activateBrowserWindow(second)

	if app.mainWindow != second.mainWindow {
		t.Fatalf("mainWindow = %p, want %p", app.mainWindow, second.mainWindow)
	}
	if second.tabs.ActiveTabID != tab2.ID {
		t.Fatalf("active tab = %q, want %q", second.tabs.ActiveTabID, tab2.ID)
	}
	if app.activeWorkspaceView() != ws2 {
		t.Fatalf("active workspace view = %p, want %p", app.activeWorkspaceView(), ws2)
	}
	if app.keyboardHandler != kh {
		t.Fatalf("keyboardHandler = %p, want %p", app.keyboardHandler, kh)
	}
	if app.globalShortcutHandler != gs {
		t.Fatalf("globalShortcutHandler = %p, want %p", app.globalShortcutHandler, gs)
	}
}

func TestApp_BrowserWindowForPaneFindsOwningWindow(t *testing.T) {
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	app := &App{
		tabs:           tabs,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:   map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
	}

	got := app.browserWindowForPane(entity.PaneID("pane-2"))

	if got != second {
		t.Fatalf("browserWindowForPane = %p, want %p", got, second)
	}
}

func TestApp_OwnerOrLastFocusedBrowserWindowPrefersPaneOwner(t *testing.T) {
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		lastFocusedWindowID: first.id,
	}

	got := app.ownerOrLastFocusedBrowserWindow("", entity.PaneID("pane-2"))

	if got != second {
		t.Fatalf("ownerOrLastFocusedBrowserWindow = %p, want %p", got, second)
	}
}

func TestApp_HandlePaneWindowTitleChangedTargetsOwningWindow(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	firstMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("first window creation failed: %v", err)
	}
	defer firstMainWindow.Destroy()
	secondMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("second window creation failed: %v", err)
	}
	defer secondMainWindow.Destroy()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow, tabs: firstTabs}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow, tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		lastFocusedWindowID: first.id,
	}

	app.handlePaneWindowTitleChanged(entity.PaneID("pane-2"), "Pane Two")

	if got := windowTitle(t, firstMainWindow); got != "Dumber" {
		t.Fatalf("first window title = %q, want %q", got, "Dumber")
	}
	if got := windowTitle(t, secondMainWindow); got != "Pane Two - Dumber" {
		t.Fatalf("second window title = %q, want %q", got, "Pane Two - Dumber")
	}
}

func TestApp_ActivateBrowserWindowResyncsTitleFromBackgroundPane(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	firstMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("first window creation failed: %v", err)
	}
	defer firstMainWindow.Destroy()
	secondMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("second window creation failed: %v", err)
	}
	defer secondMainWindow.Destroy()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow, tabs: firstTabs}
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow, tabs: secondTabs}
	contentCoord := &contentcoord.Coordinator{}
	contentTitles := reflect.ValueOf(contentCoord).Elem().FieldByName("paneTitles")
	reflect.NewAt(contentTitles.Type(), unsafe.Pointer(contentTitles.UnsafeAddr())).Elem().Set(reflect.ValueOf(map[entity.PaneID]string{tab2.Workspace.ActivePaneID: "Pane Two"}))
	app := &App{
		tabs:           tabs,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:   map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		contentCoord:   contentCoord,
	}

	app.handlePaneWindowTitleChanged(tab2.Workspace.ActivePaneID, "Pane Two")
	secondMainWindow.SetTitle("Dumber")

	app.activateBrowserWindow(second)

	if got := windowTitle(t, secondMainWindow); got != "Pane Two - Dumber" {
		t.Fatalf("second window title = %q, want %q", got, "Pane Two - Dumber")
	}
	if got := windowTitle(t, firstMainWindow); got != "Dumber" {
		t.Fatalf("first window title = %q, want %q", got, "Dumber")
	}
}

func TestApp_HandlePaneFullscreenChangedTargetsOwningWindow(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	firstMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("first window creation failed: %v", err)
	}
	defer firstMainWindow.Destroy()
	secondMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("second window creation failed: %v", err)
	}
	defer secondMainWindow.Destroy()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	secondMainWindow.TabBar().AddTab(tab1)
	secondMainWindow.TabBar().AddTab(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow, tabs: firstTabs}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow, tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		lastFocusedWindowID: first.id,
	}

	app.handlePaneFullscreenChanged(entity.PaneID("pane-2"), true)

	if windowTabBarVisible(t, firstMainWindow) != true {
		t.Fatalf("first window tab bar visibility changed unexpectedly")
	}
	if got := windowTabBarVisible(t, secondMainWindow); got {
		t.Fatalf("second window tab bar visible = %v, want false", got)
	}

	app.handlePaneFullscreenChanged(entity.PaneID("pane-2"), false)

	if got := windowTabBarVisible(t, secondMainWindow); !got {
		t.Fatalf("second window tab bar visible = %v, want true", got)
	}
}

func TestApp_HandlePaneFullscreenChangedIgnoresUnresolvedPane(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	firstMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("first window creation failed: %v", err)
	}
	defer firstMainWindow.Destroy()
	secondMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("second window creation failed: %v", err)
	}
	defer secondMainWindow.Destroy()
	firstMainWindow.TabBar().SetVisible(true)
	secondMainWindow.TabBar().SetVisible(true)
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow}
	app := &App{
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id,
	}

	app.handlePaneFullscreenChanged(entity.PaneID("missing-pane"), true)

	if got := windowTabBarVisible(t, firstMainWindow); !got {
		t.Fatalf("first window tab bar visible = %v, want true", got)
	}
	if got := windowTabBarVisible(t, secondMainWindow); !got {
		t.Fatalf("second window tab bar visible = %v, want true", got)
	}
}

func TestApp_HandleAccentKeyPressDelegatesToOwningWindow(t *testing.T) {
	first := newTestAccentUseCase(t, true)
	second := newTestAccentUseCase(t, false)
	firstWindow := &browserWindow{id: "window-1", insertAccentUC: first}
	secondWindow := &browserWindow{id: "window-2", insertAccentUC: second}
	app := &App{
		browserWindows:      map[string]*browserWindow{firstWindow.id: firstWindow, secondWindow.id: secondWindow},
		lastFocusedWindowID: secondWindow.id,
	}

	if got := app.handleAccentKeyPress(context.Background(), uint('e'), 0); got {
		t.Fatalf("handleAccentKeyPress returned %v, want false from active shell handler", got)
	}
	if got := first.IsPickerVisible(); !got {
		t.Fatalf("first shell accent handler should remain visible")
	}
	if got := second.IsPickerVisible(); got {
		t.Fatalf("second shell accent handler should remain hidden")
	}
}

func TestApp_MoveActivePaneToTabFromBrowserWindowAnchorsOwningWindow(t *testing.T) {
	tabs := entity.NewTabList()
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		movePaneToTabUC:     usecase.NewMovePaneToTabUseCase(func() string { return "new-tab" }),
		deps:                &Dependencies{Config: &config.Config{}},
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: second.id,
	}

	if err := app.moveActivePaneToTabFromBrowserWindow(context.Background(), first, ""); err != nil {
		t.Fatalf("moveActivePaneToTabFromBrowserWindow returned error: %v", err)
	}

	if tabs.Find(tab1.ID) != nil {
		t.Fatalf("tab-1 should have been moved from owning window")
	}
	if tabs.Find(tab2.ID) == nil {
		t.Fatalf("tab-2 should remain when owning window is preserved")
	}
}

func TestApp_MoveActivePaneToTabFromBrowserWindowActivatesTargetOwnerForCrossWindowTab(t *testing.T) {
	tabs := entity.NewTabList()
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tab3 := entity.NewTab(entity.TabID("tab-3"), entity.WorkspaceID("workspace-3"), entity.NewPane(entity.PaneID("pane-3")))
	tabs.Add(tab1)
	tabs.Add(tab2)
	tabs.Add(tab3)
	tabs.SetActive(tab1.ID)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		movePaneToTabUC:     usecase.NewMovePaneToTabUseCase(func() string { return "generated" }),
		deps:                &Dependencies{Config: &config.Config{}},
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second, tab3.ID: second},
		lastFocusedWindowID: first.id,
	}

	if err := app.moveActivePaneToTabFromBrowserWindow(context.Background(), first, tab3.ID); err != nil {
		t.Fatalf("moveActivePaneToTabFromBrowserWindow returned error: %v", err)
	}

	if tabs.Find(tab1.ID) != nil {
		t.Fatalf("tab-1 should be removed after its active pane moves away")
	}
	if tabs.Find(tab2.ID) == nil {
		t.Fatalf("tab-2 should remain in the target owner window")
	}
	if tabs.Find(tab3.ID) == nil {
		t.Fatalf("tab-3 should remain as the move target")
	}
	if app.tabs.ActiveTabID != tab3.ID {
		t.Fatalf("active tab = %q, want %q", app.tabs.ActiveTabID, tab3.ID)
	}
	if app.lastFocusedWindowID != second.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, second.id)
	}
}

func TestApp_MoveActivePaneToTabFromBrowserWindowCrossWindowDerivedMirror(t *testing.T) {
	// app.tabs initially contains ONLY source tab (not target tab) to simulate
	// a target created only in browserWindow.tabs (the real-world scenario that
	// CodeRabbit flagged). The fix ensures buildMovePaneToTabInput syncs the
	// derived global mirror before passing a.tabs to the move usecase.
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab3 := entity.NewTab(entity.TabID("tab-3"), entity.WorkspaceID("workspace-3"), entity.NewPane(entity.PaneID("pane-3")))

	// app.tabs contains only tab1 — tab3 is absent from the mirror.
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.SetActive(tab1.ID)

	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTabs := entity.NewTabList()
	secondTabs.Add(tab3)
	secondTabs.SetActive(tab3.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	initialPaneCount := tab3.Workspace.PaneCount()

	app := &App{
		tabs:                tabs,
		movePaneToTabUC:     usecase.NewMovePaneToTabUseCase(func() string { return "generated" }),
		deps:                &Dependencies{Config: &config.Config{}},
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab3.ID: second},
		lastFocusedWindowID: first.id,
	}

	if err := app.moveActivePaneToTabFromBrowserWindow(context.Background(), first, tab3.ID); err != nil {
		t.Fatalf("moveActivePaneToTabFromBrowserWindow returned error: %v", err)
	}

	// The derived mirror must now contain tab3 (synced by the fix).
	if app.tabs.Find(tab3.ID) == nil {
		t.Fatalf("tab-3 should have been synced into the derived global mirror")
	}

	// No new tab should have been generated — the move should target tab3.
	if app.tabs.Find(entity.TabID("generated")) != nil {
		t.Fatalf("a new tab was incorrectly generated; the move should target tab-3")
	}

	// The moved pane should be in tab3's workspace.
	if got := tab3.Workspace.PaneCount(); got != initialPaneCount+1 {
		t.Fatalf("tab-3 pane count = %d, want %d (original + moved pane)", got, initialPaneCount+1)
	}

	// windowForTab for tab3 must remain second.
	if app.windowForTab[tab3.ID] != second {
		t.Fatalf("windowForTab[tab-3] = %p, want second window %p", app.windowForTab[tab3.ID], second)
	}
}

func TestApp_InitKeyboardHandlerDoesNotReattachExistingWindowInput(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	sentinelKeyboardHandler := &input.KeyboardHandler{}
	sentinelShortcutHandler := &input.GlobalShortcutHandler{}
	bw := &browserWindow{
		id:                    "window-1",
		mainWindow:            mainWindow,
		keyboardHandler:       sentinelKeyboardHandler,
		globalShortcutHandler: sentinelShortcutHandler,
	}
	app := &App{
		deps:           &Dependencies{Config: &config.Config{}},
		mainWindow:     mainWindow,
		browserWindows: map[string]*browserWindow{bw.id: bw},
		kbDispatcher:   dispatcher.NewKeyboardDispatcher(context.Background(), nil, nil, nil, nil, dispatcher.KeyboardActions{}, func(context.Context) entity.PaneID { return "" }),
	}

	app.initKeyboardHandler(context.Background())

	if bw.keyboardHandler != sentinelKeyboardHandler {
		t.Fatalf("keyboardHandler was reattached")
	}
	if bw.globalShortcutHandler != sentinelShortcutHandler {
		t.Fatalf("globalShortcutHandler was reattached")
	}
}

func newTestAccentUseCase(t *testing.T, pickerVisible bool) *usecase.InsertAccentUseCase {
	t.Helper()
	uc := usecase.NewInsertAccentUseCase(&testFocusedInputProvider{}, nil, nil)
	if uc == nil {
		t.Fatal("failed to create accent use case")
	}
	setAccentUseCaseBoolField(t, uc, "pickerVisible", pickerVisible)
	return uc
}

func setAccentUseCaseBoolField(t *testing.T, uc *usecase.InsertAccentUseCase, field string, value bool) {
	t.Helper()
	rv := reflect.ValueOf(uc).Elem()
	fv := rv.FieldByName(field)
	if !fv.IsValid() {
		t.Fatalf("InsertAccentUseCase missing %s field", field)
	}
	reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().SetBool(value)
}

type testFocusedInputProvider struct{}

func (*testFocusedInputProvider) GetFocusedInput() port.TextInputTarget { return nil }
func (*testFocusedInputProvider) SetFocusedInput(port.TextInputTarget)  {}

func TestApp_UpdateBrowserWindowTabBarVisibilityAutoHidesSingleTabButPreservesAllocation(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow := &window.MainWindow{}
	tabBar := component.NewTabBar()
	if tabBar == nil {
		t.Fatal("tab bar creation failed")
	}
	setWindowTabBar(t, mainWindow, tabBar)
	bw := &browserWindow{id: "window-1", mainWindow: mainWindow}
	app := &App{
		deps: &Dependencies{Config: &config.Config{}},
	}
	app.deps.Config.Workspace.HideTabBarWhenSingleTab = true

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabBar.AddTab(tab1)

	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, true)

	tabBar.AddTab(tab2)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, false)

	tabBar.SetVisible(false)
	app.updateBrowserWindowTabBarVisibility(bw)
	if got := tabBar.Box().GetVisible(); got {
		t.Fatalf("tab bar visible = %v, want false while explicitly hidden", got)
	}

	tabBar.SetVisible(true)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, false)

	tabBar.RemoveTab(tab2.ID)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, true)
}

func TestTabBarSetVisibleDoesNotClearAutoHiddenInteractionState(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	tabBar := component.NewTabBar()
	if tabBar == nil {
		t.Fatal("tab bar creation failed")
	}

	tabBar.SetAutoHidden(true)
	tabBar.SetVisible(false)
	tabBar.SetVisible(true)

	box := tabBar.Box()
	if !box.GetVisible() {
		t.Fatal("tab bar should stay mounted when explicitly visible")
	}
	if got := box.GetOpacity(); got != 0.0 {
		t.Fatalf("tab bar opacity = %v, want 0 while auto-hidden", got)
	}
	if got := box.GetCanTarget(); got {
		t.Fatalf("tab bar can target = %v, want false while auto-hidden", got)
	}
	if got := box.GetFocusable(); got {
		t.Fatalf("tab bar focusable = %v, want false while auto-hidden", got)
	}
}

func TestApp_UpdateBrowserWindowTabBarVisibilityHonorsHideWhenSingleTabDisabled(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow := &window.MainWindow{}
	tabBar := component.NewTabBar()
	if tabBar == nil {
		t.Fatal("tab bar creation failed")
	}
	setWindowTabBar(t, mainWindow, tabBar)
	bw := &browserWindow{id: "window-1", mainWindow: mainWindow}
	app := &App{
		deps: &Dependencies{Config: &config.Config{}},
	}
	app.deps.Config.Workspace.HideTabBarWhenSingleTab = false

	app.updateBrowserWindowTabBarVisibility(bw)

	assertWindowTabBarAutoHidden(t, mainWindow, false)
}

func TestApp_ActivePaneIDForNilBrowserWindowIgnoresStaleOverride(t *testing.T) {
	ctx := context.Background()
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.SetActivePaneOverride(entity.PaneID("stale-pane"))
	app := &App{contentCoord: contentCoord}

	if got := app.activePaneIDForBrowserWindow(nil); got != "" {
		t.Fatalf("active pane for nil browser window = %q, want empty", got)
	}
}

func TestApp_BuildRestoredWindowUIUpdatesEmptyWindowTabBarVisibility(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mw, err := window.New(context.Background(), gtkApp, "bottom")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mw.Destroy()

	mw.SetTabBarContentInsetVisible(true)
	bw := &browserWindow{id: "window-1", mainWindow: mw, tabs: entity.NewTabList()}
	app := &App{deps: &Dependencies{Config: &config.Config{}}}
	app.deps.Config.Workspace.HideTabBarWhenSingleTab = true

	app.buildRestoredWindowUI(context.Background(), []*browserWindow{bw})

	assertWindowTabBarAutoHidden(t, mw, true)
	if mw.HasTabBarContentInset() {
		t.Fatal("empty restored bottom window should not keep tab bar content inset")
	}
}

func TestApp_UpdateBrowserWindowTabBarVisibility_BottomInset(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	// Use a real bottom MainWindow so SetTabBarContentInsetVisible is effective.
	mainWindow, err := window.New(context.Background(), gtkApp, "bottom")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	bw := &browserWindow{id: "window-1", mainWindow: mainWindow}
	app := &App{
		deps: &Dependencies{Config: &config.Config{}},
	}
	app.deps.Config.Workspace.HideTabBarWhenSingleTab = true

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))

	// One tab + hide enabled: auto-hidden and no inset
	mainWindow.TabBar().AddTab(tab1)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, true)
	if mainWindow.HasTabBarContentInset() {
		t.Fatal("one tab: expected no content area inset")
	}

	// Two tabs: visible and inset present
	mainWindow.TabBar().AddTab(tab2)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, false)
	if !mainWindow.HasTabBarContentInset() {
		t.Fatal("two tabs: expected content area inset")
	}

	// Remove back to one tab: auto-hidden and no inset
	mainWindow.TabBar().RemoveTab(tab2.ID)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, true)
	if mainWindow.HasTabBarContentInset() {
		t.Fatal("back to one tab: expected no content area inset")
	}
}

func TestApp_HandlePaneFullscreenChanged_ClearsBottomInset(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	// Use a bottom MainWindow so SetTabBarContentInsetVisible is effective.
	mainWindow, err := window.New(context.Background(), gtkApp, "bottom")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	mainWindow.TabBar().AddTab(tab1)
	mainWindow.TabBar().AddTab(tab2)

	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	tabs.SetActive(tab2.ID)

	bw := &browserWindow{id: "window-1", mainWindow: mainWindow, tabs: tabs}
	app := &App{
		tabs:                tabs,
		deps:                &Dependencies{Config: &config.Config{}},
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}
	app.deps.Config.Workspace.HideTabBarWhenSingleTab = true

	// Start with two tabs: inset should be present
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, false)
	if !mainWindow.HasTabBarContentInset() {
		t.Fatal("two tabs: expected content area inset before fullscreen")
	}

	// Enter fullscreen: tab bar hidden, inset cleared
	app.handlePaneFullscreenChanged(entity.PaneID("pane-2"), true)
	if windowTabBarVisible(t, mainWindow) {
		t.Fatal("fullscreen: tab bar should be not visible")
	}
	if mainWindow.HasTabBarContentInset() {
		t.Fatal("fullscreen: expected no content area inset")
	}

	// Exit fullscreen: tab bar restored, inset restored
	app.handlePaneFullscreenChanged(entity.PaneID("pane-2"), false)
	if !windowTabBarVisible(t, mainWindow) {
		t.Fatal("exited fullscreen: tab bar should be visible")
	}
	if !mainWindow.HasTabBarContentInset() {
		t.Fatal("exited fullscreen: expected content area inset to be restored")
	}
}

func TestApp_RemoveBrowserWindowPromotesDeterministicFallbackWithMainWindow(t *testing.T) {
	for i := 0; i < 20; i++ {
		removed := &browserWindow{id: "window-z", mainWindow: &window.MainWindow{}}
		nilWindow := &browserWindow{id: "window-a"}
		firstValid := &browserWindow{id: "window-b", mainWindow: &window.MainWindow{}}
		secondValid := &browserWindow{id: "window-c", mainWindow: &window.MainWindow{}}
		app := &App{
			mainWindow: removed.mainWindow,
			browserWindows: map[string]*browserWindow{
				removed.id:     removed,
				nilWindow.id:   nilWindow,
				firstValid.id:  firstValid,
				secondValid.id: secondValid,
			},
			browserWindowOrder:  []string{removed.id, nilWindow.id, secondValid.id, firstValid.id},
			lastFocusedWindowID: removed.id,
		}

		app.removeBrowserWindow(removed.id)

		if app.mainWindow != secondValid.mainWindow {
			t.Fatalf("mainWindow = %p, want %p", app.mainWindow, secondValid.mainWindow)
		}
		if app.lastFocusedWindowID != secondValid.id {
			t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, secondValid.id)
		}
	}
}

func TestApp_BrowserWindowActivationHookUpdatesLastFocusedWindowID(t *testing.T) {
	first := &browserWindow{id: "window-1", mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	app := &App{
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id,
	}

	app.handleBrowserWindowActivationChanged(second, true)

	if app.lastFocusedWindowID != second.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, second.id)
	}
}

func TestApp_ActivateBrowserWindowTracksLastFocusedWindow(t *testing.T) {
	first := &browserWindow{id: "window-1", mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	app := &App{
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
	}

	app.activateBrowserWindow(second)

	if app.lastFocusedWindowID != second.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, second.id)
	}
}

func TestApp_LastFocusedBrowserWindowFallsBackWhenOwnerMissing(t *testing.T) {
	first := &browserWindow{id: "window-1"}
	second := &browserWindow{id: "window-2"}
	app := &App{
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: second.id,
	}

	got := app.lastFocusedBrowserWindow()

	if got != second {
		t.Fatalf("lastFocusedBrowserWindow = %p, want %p", got, second)
	}
}

func TestApp_CreatePopupTabUsesParentPaneOwnerWhenFocusIsStale(t *testing.T) {
	ctx := context.Background()
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	secondTab := entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	globalTabs := entity.NewTabList()
	globalTabs.Add(firstTab)
	globalTabs.Add(secondTab)
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	first := &browserWindow{id: "window-1", tabs: firstTabs, mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", tabs: secondTabs, mainWindow: &window.MainWindow{}}
	app := &App{
		deps:                &Dependencies{Config: &config.Config{}},
		tabs:                globalTabs,
		tabsUC:              usecase.NewManageTabsUseCase(func() string { return "popup-tab" }),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{firstTab.ID: first, secondTab.ID: second},
		workspaceViews:      make(map[entity.TabID]*component.WorkspaceView),
		lastFocusedWindowID: first.id,
		mainWindow:          first.mainWindow,
	}
	app.initTabCoordinator(ctx)

	err := app.createPopupTab(ctx, contentcoord.InsertPopupInput{
		ParentPaneID: secondTab.Workspace.ActivePaneID,
		PopupPane:    entity.NewPane(entity.PaneID("popup-pane")),
		TargetURI:    "https://example.com/popup",
	})
	if err != nil {
		t.Fatalf("createPopupTab returned error: %v", err)
	}

	if first.tabs.Find(entity.TabID("popup-tab")) != nil {
		t.Fatalf("popup tab was added to stale focused window")
	}
	created := second.tabs.Find(entity.TabID("popup-tab"))
	if created == nil {
		t.Fatalf("popup tab was not added to parent pane owner window")
	}
	if got := app.windowForTab[created.ID]; got != second {
		t.Fatalf("popup tab owner = %p, want %p", got, second)
	}
}

func TestApp_RemoveBrowserWindowClearsLastFocusedFallback(t *testing.T) {
	first := &browserWindow{id: "window-1", mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	app := &App{
		mainWindow:          second.mainWindow,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: second.id,
	}

	app.removeBrowserWindow(second.id)

	if app.lastFocusedWindowID != first.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, first.id)
	}
}

func TestApp_TabCoordinatorCallbacksUseTargetWindowWhenFocusIsStale(t *testing.T) {
	ctx := context.Background()
	firstTabs := entity.NewTabList()
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	firstTabs.Add(firstTab)
	secondTabs := entity.NewTabList()
	secondTab := entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	secondTabs.Add(secondTab)

	first := &browserWindow{id: "window-1", tabs: firstTabs, mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", tabs: secondTabs, mainWindow: &window.MainWindow{}}
	app := &App{
		deps:                &Dependencies{Config: &config.Config{}},
		tabs:                entity.NewTabList(),
		tabsUC:              usecase.NewManageTabsUseCase(func() string { return "created-tab" }),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id,
		mainWindow:          first.mainWindow,
		windowForTab:        map[entity.TabID]*browserWindow{firstTab.ID: first, secondTab.ID: second},
		workspaceViews:      make(map[entity.TabID]*component.WorkspaceView),
	}
	app.tabs.Add(firstTab)
	app.tabs.Add(secondTab)
	app.initTabCoordinator(ctx)

	created, err := app.tabCoord.Create(ctx, app.tabTargetForBrowserWindow(second), "https://example.com")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got := app.windowForTab[created.ID]; got != second {
		t.Fatalf("created tab owner = %p, want second window %p", got, second)
	}
	if app.mainWindow != second.mainWindow {
		t.Fatalf("mainWindow after create switch = %p, want %p", app.mainWindow, second.mainWindow)
	}

	app.mainWindow = first.mainWindow
	app.lastFocusedWindowID = first.id
	if err := app.tabCoord.Switch(ctx, app.tabTargetForBrowserWindow(second), secondTab.ID); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}
	if app.mainWindow != second.mainWindow {
		t.Fatalf("mainWindow after target switch = %p, want %p", app.mainWindow, second.mainWindow)
	}
}

func TestApp_ActivateBrowserWindowAddsGlobalSnapshotWithoutChangingWindowTabPosition(t *testing.T) {
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	secondTab := entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	secondTabs.SetActive(secondTab.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs, mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", tabs: secondTabs, mainWindow: &window.MainWindow{}}
	globalTabs := entity.NewTabList()
	globalTabs.Add(firstTab)
	app := &App{
		tabs:           globalTabs,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
	}

	app.activateBrowserWindow(second)

	if secondTab.Position != 0 {
		t.Fatalf("window tab position = %d, want 0", secondTab.Position)
	}
	globalSecond := app.tabs.Find(secondTab.ID)
	if globalSecond == nil {
		t.Fatalf("global tab snapshot was not added")
	}
	if globalSecond == secondTab {
		t.Fatalf("global tab should be a snapshot, not the live per-window tab")
	}
	if globalSecond.Position != 1 {
		t.Fatalf("global tab position = %d, want 1", globalSecond.Position)
	}
}

func TestApp_ActiveWorkspaceRequiresFocusedBrowserWindow(t *testing.T) {
	globalTab := entity.NewTab(entity.TabID("global-tab"), entity.WorkspaceID("ws-global"), entity.NewPane(entity.PaneID("pane-global")))
	globalTabs := entity.NewTabList()
	globalTabs.Add(globalTab)
	globalTabs.SetActive(globalTab.ID)
	app := &App{
		tabs:           globalTabs,
		browserWindows: map[string]*browserWindow{},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{globalTab.ID: {}},
	}

	if got := app.activeWorkspace(); got != nil {
		t.Fatalf("activeWorkspace() = %p, want nil without focused browser window", got)
	}
	if got := app.activeWorkspaceView(); got != nil {
		t.Fatalf("activeWorkspaceView() = %p, want nil without focused browser window", got)
	}
}

func TestApp_ActiveWorkspaceUsesLastFocusedWindowTabsNotGlobalActiveTab(t *testing.T) {
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	secondTab := entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	firstTabs.SetActive(firstTab.ID)
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	secondTabs.SetActive(secondTab.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	globalTabs := entity.NewTabList()
	globalTabs.Add(firstTab)
	globalTabs.Add(secondTab)
	globalTabs.SetActive(firstTab.ID)
	secondView := &component.WorkspaceView{}
	app := &App{
		tabs:                globalTabs,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: second.id,
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{secondTab.ID: secondView},
	}

	if got := app.activeWorkspace(); got != secondTab.Workspace {
		t.Fatalf("activeWorkspace() = %p, want second workspace %p", got, secondTab.Workspace)
	}
	if got := app.activeWorkspaceView(); got != secondView {
		t.Fatalf("activeWorkspaceView() = %p, want second view %p", got, secondView)
	}
}

// fakeRecordingWebView is a complete port.WebView that records which navigation
// methods were called and with what arguments. It does not use reflect or unsafe.
type fakeRecordingWebView struct {
	id port.WebViewID

	loadURICalled          bool
	loadURILastURI         string
	reloadCalls            int
	reloadBypassCacheCalls int
	stopCalls              int
	goBackCalls            int
	goForwardCalls         int
	loadHTMLCalls          int
	setZoomLevelCalls      int
	setZoomLevelLastLevel  float64
	openDevToolsCalls      int
	printPageCalls         int
}

func (f *fakeRecordingWebView) ID() port.WebViewID { return f.id }

func (f *fakeRecordingWebView) LoadURI(_ context.Context, uri string) error {
	f.loadURICalled = true
	f.loadURILastURI = uri
	return nil
}

func (f *fakeRecordingWebView) LoadHTML(_ context.Context, _, _ string) error {
	f.loadHTMLCalls++
	return nil
}

func (f *fakeRecordingWebView) Reload(_ context.Context) error {
	f.reloadCalls++
	return nil
}

func (f *fakeRecordingWebView) ReloadBypassCache(_ context.Context) error {
	f.reloadBypassCacheCalls++
	return nil
}

func (f *fakeRecordingWebView) Stop(_ context.Context) error {
	f.stopCalls++
	return nil
}

func (f *fakeRecordingWebView) GoBack(_ context.Context) error {
	f.goBackCalls++
	return nil
}

func (f *fakeRecordingWebView) GoForward(_ context.Context) error {
	f.goForwardCalls++
	return nil
}

func (f *fakeRecordingWebView) State() port.WebViewState { return port.WebViewState{} }

func (f *fakeRecordingWebView) URI() string   { return f.loadURILastURI }
func (f *fakeRecordingWebView) Title() string { return "" }

func (f *fakeRecordingWebView) IsLoading() bool            { return false }
func (f *fakeRecordingWebView) EstimatedProgress() float64 { return 0 }
func (f *fakeRecordingWebView) CanGoBack() bool            { return false }
func (f *fakeRecordingWebView) CanGoForward() bool         { return false }

func (f *fakeRecordingWebView) SetZoomLevel(_ context.Context, level float64) error {
	f.setZoomLevelCalls++
	f.setZoomLevelLastLevel = level
	return nil
}

func (f *fakeRecordingWebView) OpenDevTools() { f.openDevToolsCalls++ }
func (f *fakeRecordingWebView) PrintPage()    { f.printPageCalls++ }

func (f *fakeRecordingWebView) GetZoomLevel() float64                     { return 1.0 }
func (f *fakeRecordingWebView) GetFindController() port.FindController    { return nil }
func (f *fakeRecordingWebView) SetCallbacks(_ *port.WebViewCallbacks)     {}
func (f *fakeRecordingWebView) RunJavaScript(_ context.Context, _ string) {}
func (f *fakeRecordingWebView) SetBackgroundColor(_, _, _, _ float64)     {}
func (f *fakeRecordingWebView) ResetBackgroundToDefault()                 {}
func (f *fakeRecordingWebView) Favicon() port.Texture                     { return nil }
func (f *fakeRecordingWebView) Generation() uint64                        { return 0 }
func (f *fakeRecordingWebView) IsFullscreen() bool                        { return false }
func (f *fakeRecordingWebView) IsPlayingAudio() bool                      { return false }
func (f *fakeRecordingWebView) IsDestroyed() bool                         { return false }
func (f *fakeRecordingWebView) Destroy()                                  {}

type fakeZoomRepo struct{}

func (f *fakeZoomRepo) Get(context.Context, string) (*entity.ZoomLevel, error) { return nil, nil }
func (f *fakeZoomRepo) Set(context.Context, *entity.ZoomLevel) error           { return nil }
func (f *fakeZoomRepo) Delete(context.Context, string) error                   { return nil }
func (f *fakeZoomRepo) GetAll(context.Context) ([]*entity.ZoomLevel, error)    { return nil, nil }

func TestApp_BrowserWindowWebViewActionsIgnoreStaleFocusedWindow(t *testing.T) {
	ctx := context.Background()

	// Build two browser windows with independent bw.tabs and active tabs/panes.
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

	// Create fake recording webviews.
	fakeWv1 := &fakeRecordingWebView{id: 1}
	fakeWv2 := &fakeRecordingWebView{id: 2}

	// Create content coordinator with initialized internal maps to avoid
	// nil-map panics in SetNavigationOrigin during NavigateWebView calls.
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), fakeWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), fakeWv2)

	// Create NavigationCoordinator (no navigateUC needed for direct webview calls).
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id, // stale! should NOT be used
		contentCoord:        contentCoord,
		navCoord:            navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab1.ID: &component.WorkspaceView{},
			tab2.ID: &component.WorkspaceView{},
		},
	}
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	// Call window-scoped app helpers for the second window.
	if err := app.navigateFromBrowserWindow(ctx, second, "https://google.com"); err != nil {
		t.Fatalf("navigateFromBrowserWindow returned error: %v", err)
	}
	if err := app.reloadBrowserWindow(ctx, second, false); err != nil {
		t.Fatalf("reloadBrowserWindow (soft) returned error: %v", err)
	}
	if err := app.reloadBrowserWindow(ctx, second, true); err != nil {
		t.Fatalf("reloadBrowserWindow (hard) returned error: %v", err)
	}
	if err := app.stopBrowserWindow(ctx, second); err != nil {
		t.Fatalf("stopBrowserWindow returned error: %v", err)
	}
	if err := app.goBackBrowserWindow(ctx, second); err != nil {
		t.Fatalf("goBackBrowserWindow returned error: %v", err)
	}
	if err := app.goForwardBrowserWindow(ctx, second); err != nil {
		t.Fatalf("goForwardBrowserWindow returned error: %v", err)
	}

	assertRecordingWebViewIdle(t, "first", fakeWv1)
	assertRecordingWebViewActions(t, "second", fakeWv2, "https://google.com")
}

func assertRecordingWebViewIdle(t *testing.T, name string, wv *fakeRecordingWebView) {
	t.Helper()
	if wv.loadURICalled || wv.loadURILastURI != "" {
		t.Errorf("%s webview LoadURI was called, expected zero calls", name)
	}
	if wv.reloadCalls > 0 {
		t.Errorf("%s webview Reload calls = %d, want 0", name, wv.reloadCalls)
	}
	if wv.reloadBypassCacheCalls > 0 {
		t.Errorf("%s webview ReloadBypassCache calls = %d, want 0", name, wv.reloadBypassCacheCalls)
	}
	if wv.stopCalls > 0 {
		t.Errorf("%s webview Stop calls = %d, want 0", name, wv.stopCalls)
	}
	if wv.goBackCalls > 0 {
		t.Errorf("%s webview GoBack calls = %d, want 0", name, wv.goBackCalls)
	}
	if wv.goForwardCalls > 0 {
		t.Errorf("%s webview GoForward calls = %d, want 0", name, wv.goForwardCalls)
	}
}

func assertRecordingWebViewActions(t *testing.T, name string, wv *fakeRecordingWebView, wantURI string) {
	t.Helper()
	if !wv.loadURICalled {
		t.Errorf("%s webview LoadURI was not called", name)
	}
	if wv.loadURILastURI != wantURI {
		t.Errorf("%s webview LoadURI URI = %q, want %q", name, wv.loadURILastURI, wantURI)
	}
	if wv.reloadCalls != 1 {
		t.Errorf("%s webview Reload calls = %d, want 1", name, wv.reloadCalls)
	}
	if wv.reloadBypassCacheCalls != 1 {
		t.Errorf("%s webview ReloadBypassCache calls = %d, want 1", name, wv.reloadBypassCacheCalls)
	}
	if wv.stopCalls != 1 {
		t.Errorf("%s webview Stop calls = %d, want 1", name, wv.stopCalls)
	}
	if wv.goBackCalls != 1 {
		t.Errorf("%s webview GoBack calls = %d, want 1", name, wv.goBackCalls)
	}
	if wv.goForwardCalls != 1 {
		t.Errorf("%s webview GoForward calls = %d, want 1", name, wv.goForwardCalls)
	}
}

func assertRecordingWebViewBrowserActionsIdle(t *testing.T, name string, wv *fakeRecordingWebView) {
	t.Helper()
	if wv.reloadCalls > 0 {
		t.Errorf("%s webview Reload calls = %d, want 0", name, wv.reloadCalls)
	}
	if wv.reloadBypassCacheCalls > 0 {
		t.Errorf("%s webview ReloadBypassCache calls = %d, want 0", name, wv.reloadBypassCacheCalls)
	}
	if wv.stopCalls > 0 {
		t.Errorf("%s webview Stop calls = %d, want 0", name, wv.stopCalls)
	}
	if wv.goBackCalls > 0 {
		t.Errorf("%s webview GoBack calls = %d, want 0", name, wv.goBackCalls)
	}
	if wv.goForwardCalls > 0 {
		t.Errorf("%s webview GoForward calls = %d, want 0", name, wv.goForwardCalls)
	}
	if wv.printPageCalls > 0 {
		t.Errorf("%s webview PrintPage calls = %d, want 0", name, wv.printPageCalls)
	}
	if wv.openDevToolsCalls > 0 {
		t.Errorf("%s webview OpenDevTools calls = %d, want 0", name, wv.openDevToolsCalls)
	}
	if wv.setZoomLevelCalls > 0 {
		t.Errorf("%s webview SetZoomLevel calls = %d, want 0", name, wv.setZoomLevelCalls)
	}
}

func assertRecordingWebViewBrowserActionsCalledOnce(t *testing.T, name string, wv *fakeRecordingWebView) {
	t.Helper()
	if wv.reloadCalls != 1 {
		t.Errorf("%s webview Reload calls = %d, want 1", name, wv.reloadCalls)
	}
	if wv.reloadBypassCacheCalls != 1 {
		t.Errorf("%s webview ReloadBypassCache calls = %d, want 1", name, wv.reloadBypassCacheCalls)
	}
	if wv.stopCalls != 1 {
		t.Errorf("%s webview Stop calls = %d, want 1", name, wv.stopCalls)
	}
	if wv.goBackCalls != 1 {
		t.Errorf("%s webview GoBack calls = %d, want 1", name, wv.goBackCalls)
	}
	if wv.goForwardCalls != 1 {
		t.Errorf("%s webview GoForward calls = %d, want 1", name, wv.goForwardCalls)
	}
	if wv.printPageCalls != 1 {
		t.Errorf("%s webview PrintPage calls = %d, want 1", name, wv.printPageCalls)
	}
	if wv.openDevToolsCalls != 1 {
		t.Errorf("%s webview OpenDevTools calls = %d, want 1", name, wv.openDevToolsCalls)
	}
	if wv.setZoomLevelCalls != 1 {
		t.Errorf("%s webview SetZoomLevel calls = %d, want 1", name, wv.setZoomLevelCalls)
	}
}

func TestApp_OmniboxNavigateCallbackCapturesBrowserWindow(t *testing.T) {
	second := &browserWindow{id: "window-2"}

	var capturedBW *browserWindow
	var capturedURL string
	testNavigate := func(_ context.Context, bw *browserWindow, url string) error {
		capturedBW = bw
		capturedURL = url
		return nil
	}

	ctx := context.Background()
	cb := omniboxNavigateForBrowserWindow(ctx, second, testNavigate)
	cb("https://google.com")

	if capturedBW != second {
		capturedID := "<nil>"
		if capturedBW != nil {
			capturedID = capturedBW.id
		}
		t.Errorf("captured browser window = %p (id=%s), want %p (id=%s)", capturedBW, capturedID, second, second.id)
	}
	if capturedURL != "https://google.com" {
		t.Errorf("captured URL = %q, want %q", capturedURL, "https://google.com")
	}
}

func TestApp_DispatchBrowserWindowActionUsesSourceWindow(t *testing.T) {
	ctx := context.Background()

	// Build two browser windows with independent bw.tabs and active panes.
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

	// Create fake recording webviews.
	fakeWv1 := &fakeRecordingWebView{id: 1, loadURILastURI: "https://first.example"}
	fakeWv2 := &fakeRecordingWebView{id: 2, loadURILastURI: "https://second.example"}

	// Register fake webviews in content coordinator.
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), fakeWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), fakeWv2)

	// Create navCoord (no navigateUC needed for direct webview calls).
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id, // stale! should NOT be used
		contentCoord:        contentCoord,
		navCoord:            navCoord,
		deps:                &Dependencies{ZoomUC: usecase.NewManageZoomUseCase(&fakeZoomRepo{}, 1.0, nil)},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab1.ID: &component.WorkspaceView{},
			tab2.ID: &component.WorkspaceView{},
		},
	}
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	// Dispatch browser window actions targeting the second window.
	for _, a := range []input.Action{
		input.ActionReload,
		input.ActionHardReload,
		input.ActionStop,
		input.ActionGoBack,
		input.ActionGoForward,
		input.ActionPrintPage,
		input.ActionOpenDevTools,
		input.ActionZoomIn,
	} {
		if err := app.dispatchBrowserWindowAction(ctx, second, a); err != nil {
			t.Fatalf("dispatchBrowserWindowAction (%s) returned error: %v", a, err)
		}
	}

	// Assert first fake webview receives zero calls (stale focus is ignored).
	assertRecordingWebViewBrowserActionsIdle(t, "first", fakeWv1)

	// Assert second fake webview receives exactly one call per action.
	assertRecordingWebViewBrowserActionsCalledOnce(t, "second", fakeWv2)
}

func counterIDGen() func() string {
	var counter int
	return func() string {
		counter++
		return strconv.Itoa(counter)
	}
}

func TestApp_DispatchBrowserWindowActionSwitchTabIndexCreatesInSourceWindow(t *testing.T) {
	ctx := context.Background()

	// Two windows, each with one tab.
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	firstTabs.SetActive(firstTab.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTab := entity.NewTab(entity.TabID("second-tab-1"), entity.WorkspaceID("ws-second-1"), entity.NewPane(entity.PaneID("pane-second-1")))
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	secondTabs.SetActive(secondTab.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	app := &App{
		deps: &Dependencies{
			Config: &config.Config{
				Workspace: config.WorkspaceConfig{
					NewPaneURL: "about:blank",
				},
			},
		},
		tabsUC:              usecase.NewManageTabsUseCase(counterIDGen()),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{firstTab.ID: first, secondTab.ID: second},
		lastFocusedWindowID: first.id, // stale — should NOT be used
	}
	app.initTabCoordinator(ctx)

	// Dispatch ActionSwitchTabIndex4 targeting the second window.
	err := app.dispatchBrowserWindowAction(ctx, second, input.ActionSwitchTabIndex4)
	if err != nil {
		t.Fatalf("dispatchBrowserWindowAction returned error: %v", err)
	}

	// secondTabs should have 4 tabs (1 existing + 3 new).
	if got := secondTabs.Count(); got != 4 {
		t.Fatalf("second target tab count = %d, want 4", got)
	}
	if got := secondTabs.ActiveTabID; got != secondTabs.Tabs[3].ID {
		t.Fatalf("active tab = %q, want %q (index 3)", got, secondTabs.Tabs[3].ID)
	}

	// firstTabs must be completely unchanged.
	if got := firstTabs.Count(); got != 1 {
		t.Fatalf("first target tab count = %d, want 1", got)
	}

	// windowForTab should map all second window tabs to second.
	for _, tab := range secondTabs.Tabs {
		if got := app.windowForTab[tab.ID]; got != second {
			t.Fatalf("windowForTab[%s] = %p, want second window %p", tab.ID, got, second)
		}
	}
}

func TestApp_WorkspaceOmniboxNavigateUsesOwnerWindow(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	ctx := context.Background()
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	fakeWv1 := &fakeRecordingWebView{id: 1}
	fakeWv2 := &fakeRecordingWebView{id: 2}
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), fakeWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), fakeWv2)

	app := &App{
		deps:                &Dependencies{Config: &config.Config{}},
		widgetFactory:       layout.NewGtkWidgetFactory(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		lastFocusedWindowID: first.id,
		contentCoord:        contentCoord,
		navCoord:            coordinator.NewNavigationCoordinator(ctx, nil, contentCoord),
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{},
	}

	app.createWorkspaceViewWithoutAttach(ctx, tab2)
	wsView := app.workspaceViews[tab2.ID]
	if wsView == nil {
		t.Fatal("workspace view was not created for second tab")
	}
	wsView.ShowOmnibox(ctx, "")
	cb := omniboxNavigateCallbackForTest(t, wsView.GetOmnibox())
	cb("https://example.com")

	if fakeWv1.loadURICalled {
		t.Errorf("first window webview navigated to %q, want no navigation", fakeWv1.loadURILastURI)
	}
	if !fakeWv2.loadURICalled || fakeWv2.loadURILastURI != "https://example.com" {
		t.Errorf("second window webview navigation = called %v uri %q, want https://example.com", fakeWv2.loadURICalled, fakeWv2.loadURILastURI)
	}
}

func TestApp_FloatingOmniboxNavigateUsesSessionPane(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	ctx := context.Background()
	factory := layout.NewGtkWidgetFactory()
	overlay := factory.NewOverlay()

	loadedURL := ""
	pane := component.NewFloatingPane(overlay, component.FloatingPaneOptions{
		OnNavigate: func(_ context.Context, url string) error {
			loadedURL = url
			return nil
		},
	})
	session := &floatingWorkspaceSession{
		paneID:  entity.PaneID("floating-pane"),
		pane:    pane,
		overlay: overlay,
	}
	app := &App{widgetFactory: factory}

	app.showFloatingOmnibox(ctx, session)
	cb := omniboxNavigateCallbackForTest(t, session.omnibox)
	cb("https://example.com")

	if loadedURL != "https://example.com" {
		t.Fatalf("floating pane loaded URL = %q, want https://example.com", loadedURL)
	}
}

// TestApp_RestoreSessionDoesNotLeakStaleWindowsIntoTabMerge verifies that existing
// browserWindows not participating in the restore do not leak their tabs into the
// restored global tab list or UI aggregation. Only runtimeWindows (the windows
// actually restored) contribute.
func TestApp_RestoreSessionDoesNotLeakStaleWindowsIntoTabMerge(t *testing.T) {
	// Two stale browserWindows exist in the map; only one window is restored.
	staleBW1 := &browserWindow{id: "stale-w1"}
	staleBW2 := &browserWindow{id: "stale-w2"}

	staleTab1 := entity.NewTab(entity.TabID("stale-tab"), entity.WorkspaceID("stale-ws"), entity.NewPane(entity.PaneID("stale-pane")))
	staleTab1.Name = "StaleTab"
	staleBW1.tabs = entity.NewTabList()
	staleBW1.tabs.Add(staleTab1)

	staleBW2.tabs = entity.NewTabList()

	// One restored window with its own tab. Tab names survive the
	// snapshot/restore cycle even though IDs are regenerated.
	restoredTabs := entity.NewTabList()
	restoredTab := entity.NewTab(entity.TabID("restored-tab"), entity.WorkspaceID("restored-ws"), entity.NewPane(entity.PaneID("restored-pane")))
	restoredTab.Name = "RestoredTab"
	restoredTabs.Add(restoredTab)

	sessionID := entity.SessionID("test-session")
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{
		{WindowID: "saved-w1", Tabs: restoredTabs},
	}, 0, time.Unix(123, 0))

	mainWindow := &window.MainWindow{}
	runtimeBW := &browserWindow{id: "runtime-w1", mainWindow: mainWindow}

	app := &App{
		deps: &Dependencies{
			Config:           &config.Config{},
			SessionStateRepo: &fakeSessionStateRepo{state: state},
		},
		mainWindow: mainWindow,
		browserWindows: map[string]*browserWindow{
			staleBW1.id:  staleBW1,
			staleBW2.id:  staleBW2,
			runtimeBW.id: runtimeBW,
		},
		tabs: entity.NewTabList(),
	}
	// Seed global tabs with a stale tab to verify it is replaced.
	app.tabs.Add(staleTab1)

	if err := app.restoreSession(context.Background(), sessionID); err != nil {
		t.Fatalf("restoreSession returned error: %v", err)
	}

	// Global tabs must only contain the restored tab, not stale tabs.
	if got := app.tabs.Count(); got != 1 {
		t.Fatalf("tabs.Count() = %d, want 1 (only restored tab)", got)
	}
	// Verify by name since IDs are regenerated during restore.
	restored := app.tabs.Tabs[0]
	if restored == nil || restored.Name != "RestoredTab" {
		t.Fatalf("restored tab name = %q, want RestoredTab", firstTabName(app.tabs))
	}
	if app.tabs.Find(staleTab1.ID) != nil {
		t.Fatal("stale tab leaked into global tabs")
	}
}

func firstTabName(tl *entity.TabList) string {
	if len(tl.Tabs) == 0 || tl.Tabs[0] == nil {
		return ""
	}
	return tl.Tabs[0].Name
}

// TestApp_RestoreSessionHonorsActiveWindowIndex verifies that when restoring a
// v2 multi-window session, the ActiveWindowIndex from the snapshot is used to
// determine the focused window and global active tab.
func TestApp_RestoreSessionHonorsActiveWindowIndex(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	// Window 0 has a tab named "Tab-W0", window 1 has "Tab-W1" (active).
	// Tab names survive snapshot/restore; IDs are regenerated.
	pane0 := entity.NewPane("pane-w0")
	tabW0 := entity.NewTab("tab-w0", "ws-w0", pane0)
	tabW0.Name = "Tab-W0"
	tabs0 := entity.NewTabList()
	tabs0.Add(tabW0)

	pane1 := entity.NewPane("pane-w1")
	tabW1 := entity.NewTab("tab-w1", "ws-w1", pane1)
	tabW1.Name = "Tab-W1"
	tabs1 := entity.NewTabList()
	tabs1.Add(tabW1)
	tabs1.SetActive("tab-w1")

	sessionID := entity.SessionID("test-sess")
	// ActiveWindowIndex = 1: second window should be focused.
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{
		{WindowID: "saved-w0", Tabs: tabs0},
		{WindowID: "saved-w1", Tabs: tabs1},
	}, 1, time.Unix(123, 0))

	firstBW := &browserWindow{id: "runtime-w0", mainWindow: mainWindow}
	// Factory creates the second runtime window; share the mainWindow so the UI
	// restore loop can walk its tab bar without needing a second GTK window.
	var runtimeW1 *browserWindow
	app := &App{
		deps: &Dependencies{
			Config:           &config.Config{},
			SessionStateRepo: &fakeSessionStateRepo{state: state},
		},
		mainWindow:     mainWindow,
		widgetFactory:  layout.NewGtkWidgetFactory(),
		browserWindows: map[string]*browserWindow{firstBW.id: firstBW},
		tabs:           entity.NewTabList(),
		windowForTab:   map[entity.TabID]*browserWindow{},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{},
		browserWindowFactory: func(ctx context.Context, url string) (*browserWindow, error) {
			runtimeW1 = &browserWindow{id: "runtime-w1", mainWindow: mainWindow}
			return runtimeW1, nil
		},
	}

	if err := app.restoreSession(context.Background(), sessionID); err != nil {
		t.Fatalf("restoreSession returned error: %v", err)
	}

	// lastFocusedWindowID must point to the active window (index 1).
	if app.lastFocusedWindowID != "runtime-w1" {
		t.Errorf("lastFocusedWindowID = %s, want runtime-w1 (active window index 1)", app.lastFocusedWindowID)
	}

	// Global active tab must come from the active window (Tab-W1), not the first (Tab-W0).
	activeTab := app.tabs.ActiveTab()
	if activeTab == nil {
		t.Fatal("active tab is nil")
	}
	if activeTab.Name != "Tab-W1" {
		t.Errorf("active tab name = %q, want Tab-W1 (from active window)", activeTab.Name)
	}
}

// TestApp_RestoreSessionFailsOnAdditionalWindowCreationError verifies that
// restoreSession returns an error (not silent partial restore) when creating
// a browser window for an additional restored window fails.
func TestApp_RestoreSessionFailsOnAdditionalWindowCreationError(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	pane0 := entity.NewPane("pane-w0")
	tab0 := entity.NewTab("tab-w0", "ws-w0", pane0)
	tabs0 := entity.NewTabList()
	tabs0.Add(tab0)

	pane1 := entity.NewPane("pane-w1")
	tab1 := entity.NewTab("tab-w1", "ws-w1", pane1)
	tabs1 := entity.NewTabList()
	tabs1.Add(tab1)

	sessionID := entity.SessionID("test-sess")
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{
		{WindowID: "saved-w0", Tabs: tabs0},
		{WindowID: "saved-w1", Tabs: tabs1},
	}, 0, time.Unix(123, 0))

	firstBW := &browserWindow{id: "runtime-w0", mainWindow: mainWindow}
	factoryErr := errors.New("window creation failed")
	app := &App{
		deps: &Dependencies{
			Config:           &config.Config{},
			SessionStateRepo: &fakeSessionStateRepo{state: state},
		},
		mainWindow:     mainWindow,
		browserWindows: map[string]*browserWindow{firstBW.id: firstBW},
		tabs:           entity.NewTabList(),
		windowForTab:   map[entity.TabID]*browserWindow{},
		browserWindowFactory: func(ctx context.Context, url string) (*browserWindow, error) {
			return nil, factoryErr
		},
	}

	gotErr := app.restoreSession(context.Background(), sessionID)
	if gotErr == nil {
		t.Fatal("restoreSession returned nil error, want failure when additional window creation fails")
	}
	if !strings.Contains(gotErr.Error(), "create browser window 1 for restore") {
		t.Errorf("error message should mention window creation for restore, got: %v", gotErr)
	}
	if !errors.Is(gotErr, factoryErr) {
		t.Errorf("error should wrap factoryErr, got: %v", gotErr)
	}
}

func TestMainWindow_BottomTabBar_ContentAreaHasInset(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mw, err := window.New(context.Background(), gtkApp, "bottom")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mw.Destroy()

	// Bottom window should start WITHOUT the inset
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("bottom-tab-bar content area: should NOT have inset at window creation")
	}

	// HasTabBarContentInset must return false initially
	if mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return false initially")
	}

	// SetTabBarContentInsetVisible(true) must add the class
	mw.SetTabBarContentInsetVisible(true)
	if !mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("bottom-tab-bar content area: expected content-area-tabbar-inset-bottom CSS class after SetTabBarContentInsetVisible(true)")
	}
	if !mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return true after SetTabBarContentInsetVisible(true)")
	}

	// SetTabBarContentInsetVisible(false) must remove the class
	mw.SetTabBarContentInsetVisible(false)
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("bottom-tab-bar content area: should NOT have inset after SetTabBarContentInsetVisible(false)")
	}
	if mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return false after SetTabBarContentInsetVisible(false)")
	}

	// Verify tab bar remains a non-measured overlay regardless of inset state
	if mw.ContentOverlay().GetMeasureOverlay(mw.TabBar().Widget()) {
		t.Fatal("bottom-tab-bar: expected tab bar to remain a non-measured overlay")
	}
}

func TestMainWindow_TopTabBar_ContentAreaHasTopInset(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mw, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mw.Destroy()

	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-top") {
		t.Fatal("top-tab-bar content area: should NOT have inset at window creation")
	}
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("top-tab-bar content area: should NOT have content-area-tabbar-inset-bottom CSS class")
	}

	mw.SetTabBarContentInsetVisible(true)
	if !mw.ContentArea().HasCssClass("content-area-tabbar-inset-top") {
		t.Fatal("top-tab-bar content area: expected content-area-tabbar-inset-top CSS class after SetTabBarContentInsetVisible(true)")
	}
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("top-tab-bar content area: should NOT have bottom inset after SetTabBarContentInsetVisible(true)")
	}
	if !mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return true for top inset")
	}

	mw.SetTabBarContentInsetVisible(false)
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-top") {
		t.Fatal("top-tab-bar content area: should NOT have inset after SetTabBarContentInsetVisible(false)")
	}
	if mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return false after removing top inset")
	}

	if mw.ContentOverlay().GetMeasureOverlay(mw.TabBar().Widget()) {
		t.Fatal("top-tab-bar: expected tab bar to remain a non-measured overlay")
	}
}

package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
			if _, err := os.Stat(filepath.Join("/tmp/.X11-unix", "X"+displayNum)); err == nil {
				return true
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
	created := &browserWindow{id: "window-1"}
	app := &App{
		tabs:           entity.NewTabList(),
		tabsUC:         usecase.NewManageTabsUseCase(func() string { return "id-1" }),
		browserWindows: make(map[string]*browserWindow),
		workspaceViews: map[entity.TabID]*component.WorkspaceView{entity.TabID("id-1"): &component.WorkspaceView{}},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return created, nil
		},
	}
	app.tabs.Add(entity.NewTab(entity.TabID("existing-tab"), entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane"))))

	if err := app.OpenFreshWindow(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("OpenFreshWindow returned error: %v", err)
	}
	if got := windowForTabCount(t, app); got != 1 {
		t.Fatalf("windowForTab length = %d, want 1", got)
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
	created := &browserWindow{id: "window-1"}
	originalWindow := &window.MainWindow{}
	tabBar := &component.TabBar{}
	setWindowTabBar(t, originalWindow, tabBar)
	existingTabID := entity.TabID("existing-tab")
	staleTabID := entity.TabID("stale-tab")
	tabBar.SetActive(staleTabID)
	app := &App{
		tabs:           entity.NewTabList(),
		tabsUC:         usecase.NewManageTabsUseCase(func() string { return "id-1" }),
		browserWindows: make(map[string]*browserWindow),
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return created, nil
		},
		mainWindow: originalWindow,
	}
	app.tabs.Add(entity.NewTab(existingTabID, entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane"))))
	app.tabCoord = coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{
		TabsUC:     app.tabsUC,
		Tabs:       app.tabs,
		MainWindow: &window.MainWindow{},
	})

	if err := app.OpenFreshWindow(context.Background(), "https://example.com/fail"); err == nil {
		t.Fatalf("OpenFreshWindow = nil error, want failure")
	}
	if got := len(app.browserWindows); got != 0 {
		t.Fatalf("browserWindows length = %d, want 0", got)
	}
	if got := windowForTabCount(t, app); got != 0 {
		t.Fatalf("windowForTab length = %d, want 0", got)
	}
	if got := windowTabBarActiveID(t, originalWindow); got != existingTabID {
		t.Fatalf("tab bar active tab = %q, want %q", got, existingTabID)
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

	app := &App{
		tabs:           tabs,
		tabsUC:         tabsUC,
		browserWindows: make(map[string]*browserWindow),
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
	app.tabs.Add(entity.NewTab(existingTabID, entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane"))))
	app.tabCoord.SetOnTabCreated(func(ctx context.Context, tab *entity.Tab) {
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
	if got := windowTabBarVisible(t, newWindow); got {
		t.Fatalf("new window tab bar visible = %v, want false", got)
	}
}

func TestApp_ActivateBrowserWindowSwitchesActiveWorkspace(t *testing.T) {
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	ws1 := &component.WorkspaceView{}
	ws2 := &component.WorkspaceView{}
	first := &browserWindow{id: "window-1", activeTabID: tab1.ID, mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", activeTabID: tab2.ID, mainWindow: &window.MainWindow{}}
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
	if app.tabs.ActiveTabID != tab2.ID {
		t.Fatalf("active tab = %q, want %q", app.tabs.ActiveTabID, tab2.ID)
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
	first := &browserWindow{id: "window-1"}
	second := &browserWindow{id: "window-2"}
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
	first := &browserWindow{id: "window-1"}
	second := &browserWindow{id: "window-2"}
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
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow}
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
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow, activeTabID: tab1.ID}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow, activeTabID: tab2.ID}
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
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow}
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
	first := &browserWindow{id: "window-1", activeTabID: tab1.ID}
	second := &browserWindow{id: "window-2", activeTabID: tab2.ID}
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
	first := &browserWindow{id: "window-1", activeTabID: tab1.ID}
	second := &browserWindow{id: "window-2", activeTabID: tab2.ID}
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

	if tabs.Find(tab1.ID) == nil {
		t.Fatalf("tab-1 should remain when target owner is activated first")
	}
	if tabs.Find(tab2.ID) != nil {
		t.Fatalf("tab-2 should have been moved from the target owner window")
	}
	if tabs.Find(tab3.ID) == nil {
		t.Fatalf("tab-3 should remain as the move target")
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
		kbDispatcher:   dispatcher.NewKeyboardDispatcher(context.Background(), nil, nil, nil, nil, nil, "", func(context.Context) entity.PaneID { return "" }),
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

func TestApp_UpdateBrowserWindowTabBarVisibilityHonorsHideWhenSingleTabDisabled(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow := &window.MainWindow{}
	tabBar := component.NewTabBar()
	if tabBar == nil {
		t.Fatal("tab bar creation failed")
	}
	tabBar.SetVisible(false)
	setWindowTabBar(t, mainWindow, tabBar)
	bw := &browserWindow{id: "window-1", mainWindow: mainWindow}
	app := &App{
		deps: &Dependencies{Config: &config.Config{}},
	}
	app.deps.Config.Workspace.HideTabBarWhenSingleTab = false

	app.updateBrowserWindowTabBarVisibility(bw)

	if got := windowTabBarVisible(t, mainWindow); !got {
		t.Fatalf("tab bar visible = %v, want true", got)
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
			lastFocusedWindowID: removed.id,
		}

		app.removeBrowserWindow(removed.id)

		if app.mainWindow != firstValid.mainWindow {
			t.Fatalf("mainWindow = %p, want %p", app.mainWindow, firstValid.mainWindow)
		}
		if app.lastFocusedWindowID != firstValid.id {
			t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, firstValid.id)
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

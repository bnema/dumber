package ui

import (
	"context"
	"io"
	"reflect"
	"testing"
	"unsafe"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/window"
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

func setDependencyField(t *testing.T, deps *Dependencies, field string, value any) {
	t.Helper()

	rv := reflect.ValueOf(deps).Elem()
	fv := rv.FieldByName(field)
	if !fv.IsValid() {
		t.Fatalf("Dependencies missing %s field", field)
	}
	fv.Set(reflect.ValueOf(value))
}

func windowForTabCount(t *testing.T, app *App) int {
	t.Helper()

	rv := reflect.ValueOf(app).Elem()
	fv := rv.FieldByName("windowForTab")
	if !fv.IsValid() {
		t.Fatalf("App missing windowForTab field")
	}
	return fv.Len()
}

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

func windowTabBarActiveID(t *testing.T, mw *window.MainWindow) entity.TabID {
	t.Helper()
	if mw == nil || mw.TabBar() == nil {
		return ""
	}
	return mw.TabBar().ActiveTabID()
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
	EnsureAdwaitaInitialized()
	appID := AppID
	gtkApp := gtk.NewApplication(&appID, gio.GApplicationNonUniqueValue)
	if gtkApp == nil {
		t.Fatal("gtk application creation failed")
	}
	defer gtkApp.Unref()

	existingTabID := entity.TabID("existing-tab")
	oldWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("old window creation failed: %v", err)
	}
	newWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("new window creation failed: %v", err)
	}
	tabs := entity.NewTabList()
	tabsUC := usecase.NewManageTabsUseCase(func() string { return "id-1" })

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

	createdTabID := app.tabs.ActiveTabID
	if createdTabID == "" {
		t.Fatalf("created tab id = %q, want non-empty", createdTabID)
	}
	if got := windowTabBarActiveID(t, newWindow); got != createdTabID {
		t.Fatalf("new window tab bar active tab = %q, want %q", got, createdTabID)
	}
	if got := windowTabBarActiveID(t, oldWindow); got != "" {
		t.Fatalf("old window tab bar active tab = %q, want empty", got)
	}
	if got := newWindow.TabBar().Box().GetVisible(); got {
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

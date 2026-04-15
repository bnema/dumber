package ui

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gtk"
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
	setShellField(t, removed, "permissionDialog", (*testPermissionDialogPresenter)(nil))
	setShellField(t, removed, "webrtcIndicator", &component.WebRTCPermissionIndicator{})

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
	} {
		if !fieldIsZero(t, removed, name) {
			t.Fatalf("browserWindow.%s was not cleared", name)
		}
	}
}

func TestBrowserWindow_RemoveBrowserWindowCleansOwnedTabState(t *testing.T) {
	ownedTab := entity.NewTab(entity.TabID("tab-owned"), entity.WorkspaceID("workspace-owned"), entity.NewPane(entity.PaneID("pane-owned")))
	otherTab := entity.NewTab(entity.TabID("tab-other"), entity.WorkspaceID("workspace-other"), entity.NewPane(entity.PaneID("pane-other")))
	removedMainWindow := &window.MainWindow{}
	remainingMainWindow := &window.MainWindow{}
	removed := &browserWindow{id: "window-1", mainWindow: removedMainWindow}
	remaining := &browserWindow{id: "window-2", mainWindow: remainingMainWindow}
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
	app.dispatchOnMainThread = func(fn func()) {
		dispatched = true
		fn()
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
	app.dispatchOnMainThread = func(fn func()) { fn() }
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

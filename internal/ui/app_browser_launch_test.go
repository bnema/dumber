package ui

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/window"
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

type fakeWidgetFactory struct{}

func (fakeWidgetFactory) NewPaned(layout.Orientation) layout.PanedWidget  { return nil }
func (fakeWidgetFactory) NewBox(layout.Orientation, int) layout.BoxWidget { return nil }
func (fakeWidgetFactory) NewOverlay() layout.OverlayWidget                { return nil }
func (fakeWidgetFactory) NewLabel(string) layout.LabelWidget              { return nil }
func (fakeWidgetFactory) NewButton() layout.ButtonWidget                  { return nil }
func (fakeWidgetFactory) NewImage() layout.ImageWidget                    { return nil }
func (fakeWidgetFactory) NewProgressBar() layout.ProgressBarWidget        { return nil }
func (fakeWidgetFactory) NewSpinner() layout.SpinnerWidget                { return nil }
func (fakeWidgetFactory) WrapWidget(*gtk.Widget) layout.Widget            { return nil }

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
	app := &App{
		tabs:           entity.NewTabList(),
		tabsUC:         usecase.NewManageTabsUseCase(func() string { return "id-1" }),
		browserWindows: make(map[string]*browserWindow),
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return created, nil
		},
		tabCoord: coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{
			TabsUC:     usecase.NewManageTabsUseCase(func() string { return "id-2" }),
			Tabs:       nil,
			MainWindow: &window.MainWindow{},
		}),
		widgetFactory: fakeWidgetFactory{},
	}
	app.tabs.Add(entity.NewTab(entity.TabID("existing-tab"), entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane"))))

	if err := app.OpenFreshWindow(context.Background(), "https://example.com/fail"); err == nil {
		t.Fatalf("OpenFreshWindow = nil error, want failure")
	}
	if got := len(app.browserWindows); got != 0 {
		t.Fatalf("browserWindows length = %d, want 0", got)
	}
	if got := windowForTabCount(t, app); got != 0 {
		t.Fatalf("windowForTab length = %d, want 0", got)
	}
}

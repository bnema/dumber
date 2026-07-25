package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/coordinator/content"
)

func newPopupAdoptionApp(t *testing.T, attach func(context.Context, entity.TabID, *entity.Pane, port.WebView) error) (*App, *browserWindow) {
	t.Helper()
	bw := &browserWindow{id: "detached-window", tabs: entity.NewTabList()}
	app := &App{
		tabs: entity.NewTabList(), tabsUC: usecase.NewManageTabsUseCase(func() string { return "adopted-tab" }, nil),
		browserWindows: map[string]*browserWindow{}, windowForTab: map[entity.TabID]*browserWindow{},
		workspaceViews:       map[entity.TabID]*component.WorkspaceView{},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) { return bw, nil },
	}
	app.tabCoord = coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{TabsUC: app.tabsUC, Tabs: app.tabs})
	app.tabCoord.SetOnAttachPopupToTab(attach)
	app.tabCoord.SetOnTabClosed(func(_ context.Context, _ coordinator.TabTarget, tab *entity.Tab) {
		delete(app.windowForTab, tab.ID)
		delete(app.workspaceViews, tab.ID)
		app.tabs.Remove(tab.ID)
	})
	app.tabCoord.SetOnCurrentWindowEmpty(func(context.Context, coordinator.TabTarget) {
		app.cleanupEjectTargetWindow(bw)
	})
	return app, bw
}

type popupLifecycleWebView struct {
	*portmocks.MockWebView
	onReady   func()
	onClose   func()
	showCalls int
}

func (w *popupLifecycleWebView) SetOnReadyToShow(fn func())  { w.onReady = fn }
func (w *popupLifecycleWebView) SetOnClose(fn func())        { w.onClose = fn }
func (w *popupLifecycleWebView) Show()                       { w.showCalls++ }
func (w *popupLifecycleWebView) PrimePopupNavigation(string) {}

func TestOpenPopupBrowserWindowAdoptsProvidedWebView(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	var adopted port.WebView
	app, bw := newPopupAdoptionApp(t, func(_ context.Context, _ entity.TabID, _ *entity.Pane, got port.WebView) error {
		adopted = got
		return nil
	})

	result, err := app.openPopupBrowserWindow(context.Background(), content.BrowserWindowInput{
		PopupPane: entity.NewPane("popup-pane"), PopupWebView: wv, TargetURI: "https://example.com", Ready: true,
	})

	require.NoError(t, err)
	assert.Same(t, wv, adopted)
	assert.Equal(t, bw.id, result.WindowID)
	assert.Same(t, bw, app.windowForTab["adopted-tab"])
}

func TestOpenPopupBrowserWindowWaitsForReadySignalAndShowsExactlyOnce(t *testing.T) {
	previousShow := showPopupBrowserWindow
	showCalls := 0
	showPopupBrowserWindow = func(*browserWindow) { showCalls++ }
	t.Cleanup(func() { showPopupBrowserWindow = previousShow })

	wv := &popupLifecycleWebView{MockWebView: portmocks.NewMockWebView(t)}
	app, _ := newPopupAdoptionApp(t, func(context.Context, entity.TabID, *entity.Pane, port.WebView) error { return nil })

	_, err := app.openPopupBrowserWindow(context.Background(), content.BrowserWindowInput{
		PopupPane: entity.NewPane("popup-pane"), PopupWebView: wv, Ready: false,
	})

	require.NoError(t, err)
	require.NotNil(t, wv.onReady)
	assert.NotNil(t, wv.onClose)
	assert.Zero(t, showCalls)
	wv.onReady()
	wv.onReady()
	assert.Equal(t, 1, showCalls)
}

func TestOpenPopupBrowserWindowShowsImmediatelyWhenAlreadyReady(t *testing.T) {
	previousShow := showPopupBrowserWindow
	showCalls := 0
	showPopupBrowserWindow = func(*browserWindow) { showCalls++ }
	t.Cleanup(func() { showPopupBrowserWindow = previousShow })

	wv := &popupLifecycleWebView{MockWebView: portmocks.NewMockWebView(t)}
	app, _ := newPopupAdoptionApp(t, func(context.Context, entity.TabID, *entity.Pane, port.WebView) error { return nil })

	_, err := app.openPopupBrowserWindow(context.Background(), content.BrowserWindowInput{
		PopupPane: entity.NewPane("popup-pane"), PopupWebView: wv, Ready: true,
	})

	require.NoError(t, err)
	assert.Nil(t, wv.onReady)
	assert.NotNil(t, wv.onClose)
	assert.Equal(t, 1, showCalls)
}

func TestOpenPopupBrowserWindowTransferredWebViewIsNotDestroyed(t *testing.T) {
	wv := &recordingWebView{id: 99}
	app, _ := newPopupAdoptionApp(t, func(context.Context, entity.TabID, *entity.Pane, port.WebView) error { return nil })

	_, err := app.openPopupBrowserWindow(context.Background(), content.BrowserWindowInput{
		PopupPane: entity.NewPane("popup-pane"), PopupWebView: wv, Ready: true,
	})

	require.NoError(t, err)
	assert.Zero(t, wv.destroyCalls)
}

func TestOpenPopupBrowserWindowCloseRequestClosesAdoptedLastTabAndWindow(t *testing.T) {
	wv := &popupLifecycleWebView{MockWebView: portmocks.NewMockWebView(t)}
	app, bw := newPopupAdoptionApp(t, func(context.Context, entity.TabID, *entity.Pane, port.WebView) error { return nil })

	_, err := app.openPopupBrowserWindow(context.Background(), content.BrowserWindowInput{
		PopupPane: entity.NewPane("popup-pane"), PopupWebView: wv, Ready: true,
	})
	require.NoError(t, err)
	require.NotNil(t, wv.onClose)

	wv.onClose()

	assert.Zero(t, bw.tabs.Count())
	assert.False(t, app.hasBrowserWindow(bw))
	assert.Empty(t, app.windowForTab)
}

func TestOpenPopupBrowserWindowRollsBackShellWhenCreateWithPaneFails(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	app, bw := newPopupAdoptionApp(t, func(context.Context, entity.TabID, *entity.Pane, port.WebView) error {
		return errors.New("attachment failed")
	})

	_, err := app.openPopupBrowserWindow(context.Background(), content.BrowserWindowInput{
		PopupPane: entity.NewPane("popup-pane"), PopupWebView: wv, TargetURI: "https://example.com", Ready: true,
	})

	require.Error(t, err)
	assert.False(t, app.hasBrowserWindow(bw))
	assert.Zero(t, bw.tabs.Count())
	assert.Empty(t, app.windowForTab)
	assert.Empty(t, app.workspaceViews)
}

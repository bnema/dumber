package content

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/layout"
	layoutmocks "github.com/bnema/dumber/internal/ui/layout/mocks"
)

// newMinimalCoordinator returns a Coordinator with the maps required by
// ReleaseWebView initialized so that calls to clearPendingAppearance,
// titleMu / navOriginMu locks do not panic.
func newMinimalCoordinator() *Coordinator {
	return &Coordinator{
		webViews:   make(map[entity.PaneID]port.WebView),
		paneTitles: make(map[entity.PaneID]string),
		navOrigins: make(map[entity.PaneID]string),
	}
}

// ---------------------------------------------------------------------------
// EnsureWebView
// ---------------------------------------------------------------------------

func TestLifecycle_EnsureWebView_ReusesExistingNonDestroyed(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false)
	wv.EXPECT().ID().Return(port.WebViewID(101)).Once()

	pool := mocks.NewMockWebViewPool(t)
	// pool.Acquire must NOT be called

	c := newMinimalCoordinator()
	c.pool = pool
	c.webViews[entity.PaneID("pane-1")] = wv

	got, err := c.EnsureWebView(context.Background(), "pane-1")

	require.NoError(t, err)
	assert.Equal(t, wv, got)
}

func TestLifecycle_EnsureWebView_AcquiresFromPoolWhenNoneExists(t *testing.T) {
	t.Parallel()

	newWV := mocks.NewMockWebView(t)
	newWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	newWV.EXPECT().Generation().Return(uint64(0)).Maybe()
	newWV.EXPECT().ID().Return(port.WebViewID(102)).Maybe()

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Acquire(mock.Anything).Return(newWV, nil)

	c := newMinimalCoordinator()
	c.pool = pool

	got, err := c.EnsureWebView(context.Background(), "pane-1")

	require.NoError(t, err)
	assert.Equal(t, newWV, got)
	assert.Equal(t, newWV, c.webViews[entity.PaneID("pane-1")])
}

func TestLifecycle_EnsureWebView_AcquiresFromPoolWhenExistingIsDestroyed(t *testing.T) {
	t.Parallel()

	oldWV := mocks.NewMockWebView(t)
	oldWV.EXPECT().IsDestroyed().Return(true)

	newWV := mocks.NewMockWebView(t)
	newWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	newWV.EXPECT().Generation().Return(uint64(0)).Maybe()
	newWV.EXPECT().ID().Return(port.WebViewID(103)).Maybe()

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Acquire(mock.Anything).Return(newWV, nil)

	c := newMinimalCoordinator()
	c.pool = pool
	c.webViews[entity.PaneID("pane-1")] = oldWV

	got, err := c.EnsureWebView(context.Background(), "pane-1")

	require.NoError(t, err)
	assert.Equal(t, newWV, got)
}

func TestLifecycle_EnsureWebView_ErrorWhenPoolIsNil(t *testing.T) {
	t.Parallel()

	c := newMinimalCoordinator()
	// pool remains nil

	_, err := c.EnsureWebView(context.Background(), "pane-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "webview pool not configured")
}

func TestLifecycle_EnsureWebView_ErrorWhenAcquireFails(t *testing.T) {
	t.Parallel()

	acquireErr := errors.New("pool exhausted")

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Acquire(mock.Anything).Return(nil, acquireErr)

	c := newMinimalCoordinator()
	c.pool = pool

	_, err := c.EnsureWebView(context.Background(), "pane-1")

	require.Error(t, err)
	assert.Equal(t, acquireErr, err)
}

func TestLifecycle_AttachToWorkspace_ReusesRegisteredWebViewByPaneID(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().ID().Return(port.WebViewID(104)).Once()
	wv.EXPECT().URI().Return("").Maybe()

	pool := mocks.NewMockWebViewPool(t)
	widgetFactory := layoutmocks.NewMockWidgetFactory(t)
	ctx, wsView := newAttachWorkspaceView(t, widgetFactory)
	workspace := &entity.Workspace{
		ID:           "workspace-1",
		Root:         &entity.PaneNode{ID: "node-1", Pane: entity.NewPane(paneID)},
		ActivePaneID: paneID,
	}

	c := newMinimalCoordinator()
	c.pool = pool
	c.widgetFactory = widgetFactory
	c.webViews[paneID] = wv

	c.AttachToWorkspace(ctx, workspace, wsView)

	assert.Same(t, wv, c.webViews[paneID])
}

func newAttachWorkspaceView(t *testing.T, factory *layoutmocks.MockWidgetFactory) (context.Context, *component.WorkspaceView) {
	t.Helper()
	ctx := context.Background()
	container := layoutmocks.NewMockBoxWidget(t)
	overlay := layoutmocks.NewMockOverlayWidget(t)

	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(container).Once()
	container.EXPECT().SetHexpand(true).Once()
	container.EXPECT().SetVexpand(true).Once()
	container.EXPECT().SetVisible(true).Once()
	factory.EXPECT().NewOverlay().Return(overlay).Once()
	overlay.EXPECT().SetHexpand(true).Once()
	overlay.EXPECT().SetVexpand(true).Once()
	overlay.EXPECT().SetChild(container).Once()
	overlay.EXPECT().SetVisible(true).Once()

	return ctx, component.NewWorkspaceView(ctx, factory)
}

// ---------------------------------------------------------------------------
// ReleaseWebView
// ---------------------------------------------------------------------------

func TestLifecycle_ReleaseWebView_ReleasesToPool(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(11)).Maybe()
	// idleInhibitor is nil so these won't be called
	wv.EXPECT().IsFullscreen().Return(false).Maybe()
	wv.EXPECT().IsPlayingAudio().Return(false).Maybe()

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Release(wv)

	c := newMinimalCoordinator()
	c.pool = pool
	c.webViews[entity.PaneID("pane-1")] = wv

	c.ReleaseWebView(context.Background(), "pane-1")

	_, exists := c.webViews[entity.PaneID("pane-1")]
	assert.False(t, exists, "webview should be removed from map after release")
}

func TestLifecycle_ReleaseWebView_ClearsNamedBrowsingContextRegistration(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	windowID := "window-1"
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(15)).Maybe()
	wv.EXPECT().IsDestroyed().Return(false).Maybe()
	wv.EXPECT().IsFullscreen().Return(false).Maybe()
	wv.EXPECT().IsPlayingAudio().Return(false).Maybe()

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Release(wv)

	c := newMinimalCoordinator()
	c.pool = pool
	c.popups = newPopupManager()
	c.popups.setWindowIDResolver(func(id entity.PaneID) (string, bool) {
		if id == paneID {
			return windowID, true
		}
		return "", false
	})
	c.webViews[paneID] = wv
	c.popups.namedContexts.Register(windowID, "named-popup", paneID, wv.ID())

	_, _, okBefore := c.popups.namedContexts.Lookup(windowID, "named-popup", c.getWebViewLocked, c.popups.windowIDForPane)
	require.True(t, okBefore)

	c.ReleaseWebView(context.Background(), paneID)

	_, _, okAfter := c.popups.namedContexts.Lookup(windowID, "named-popup", c.getWebViewLocked, c.popups.windowIDForPane)
	assert.False(t, okAfter)
}

func TestLifecycle_ReleaseWebView_NoopForUnknownPane(t *testing.T) {
	t.Parallel()

	pool := mocks.NewMockWebViewPool(t)
	// pool.Release must NOT be called

	c := newMinimalCoordinator()
	c.pool = pool

	// Should not panic
	c.ReleaseWebView(context.Background(), "unknown-pane")
}

func TestLifecycle_ReleaseWebView_UninhibitsIdleIfFullscreen(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(12)).Maybe()
	wv.EXPECT().IsFullscreen().Return(true)
	wv.EXPECT().IsPlayingAudio().Return(false)

	inhibitor := mocks.NewMockIdleInhibitor(t)
	inhibitor.EXPECT().Uninhibit(mock.Anything).Return(nil)

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Release(wv)

	c := newMinimalCoordinator()
	c.pool = pool
	c.idleInhibitor = inhibitor
	c.webViews[entity.PaneID("pane-1")] = wv

	c.ReleaseWebView(context.Background(), "pane-1")
}

func TestLifecycle_ReleaseWebView_UninhibitsIdleIfPlayingAudio(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(13)).Maybe()
	wv.EXPECT().IsFullscreen().Return(false)
	wv.EXPECT().IsPlayingAudio().Return(true)

	inhibitor := mocks.NewMockIdleInhibitor(t)
	inhibitor.EXPECT().Uninhibit(mock.Anything).Return(nil)

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Release(wv)

	c := newMinimalCoordinator()
	c.pool = pool
	c.idleInhibitor = inhibitor
	c.webViews[entity.PaneID("pane-1")] = wv

	c.ReleaseWebView(context.Background(), "pane-1")
}

func TestLifecycle_ReleaseWebView_DestroysWebViewWhenPoolIsNil(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(14)).Maybe()
	// idleInhibitor is nil so these won't be called
	wv.EXPECT().IsFullscreen().Return(false).Maybe()
	wv.EXPECT().IsPlayingAudio().Return(false).Maybe()
	wv.EXPECT().Destroy()

	c := newMinimalCoordinator()
	// pool remains nil
	c.webViews[entity.PaneID("pane-1")] = wv

	c.ReleaseWebView(context.Background(), "pane-1")
}

// ---------------------------------------------------------------------------
// GetWebView / RegisterPopupWebView
// ---------------------------------------------------------------------------

func TestLifecycle_GetWebView_ReturnsRegisteredWebView(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)

	c := newMinimalCoordinator()
	c.webViews[entity.PaneID("pane-1")] = wv

	got := c.GetWebView("pane-1")

	assert.Equal(t, wv, got)
}

func TestLifecycle_RegisterPopupWebView_StoresWebView(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)

	c := newMinimalCoordinator()

	c.RegisterPopupWebView("popup-1", wv)

	assert.Equal(t, wv, c.GetWebView("popup-1"))
}

func TestLifecycle_RegisterPopupWebView_IgnoresNil(t *testing.T) {
	t.Parallel()

	c := newMinimalCoordinator()

	// Should not panic
	c.RegisterPopupWebView("popup-1", nil)

	assert.Nil(t, c.GetWebView("popup-1"))
}

type nativeWebViewStub struct {
	port.WebView
	ptr uintptr
}

func (s *nativeWebViewStub) NativeWidget() uintptr {
	return s.ptr
}

func TestLifecycle_WrapWidget_ReturnsNilForNilWebView(t *testing.T) {
	t.Parallel()

	factory := layoutmocks.NewMockWidgetFactory(t)
	c := newMinimalCoordinator()
	c.widgetFactory = factory

	assert.Nil(t, c.WrapWidget(context.Background(), nil))
}

func TestLifecycle_WrapWidget_ReturnsNilForMissingNativeProvider(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	factory := layoutmocks.NewMockWidgetFactory(t)
	c := newMinimalCoordinator()
	c.widgetFactory = factory

	assert.Nil(t, c.WrapWidget(context.Background(), wv))
}

func TestLifecycle_WrapWidget_ReturnsNilForZeroNativePointer(t *testing.T) {
	t.Parallel()

	base := mocks.NewMockWebView(t)
	wv := &nativeWebViewStub{WebView: base}
	factory := layoutmocks.NewMockWidgetFactory(t)
	c := newMinimalCoordinator()
	c.widgetFactory = factory

	assert.Nil(t, c.WrapWidget(context.Background(), wv))
}

func TestLifecycle_WrapWidget_ReturnsNilForNilWrappedWidget(t *testing.T) {
	t.Parallel()

	base := mocks.NewMockWebView(t)
	wv := &nativeWebViewStub{WebView: base, ptr: 123}
	factory := layoutmocks.NewMockWidgetFactory(t)
	factory.EXPECT().WrapNativeWidget(uintptr(123)).Return(nil).Once()
	c := newMinimalCoordinator()
	c.widgetFactory = factory

	assert.Nil(t, c.WrapWidget(context.Background(), wv))
}

func TestLifecycle_WrapWidget_RequestsGtkWidgetForGestureSetup(t *testing.T) {
	t.Parallel()

	base := mocks.NewMockWebView(t)
	wv := &nativeWebViewStub{WebView: base, ptr: 456}
	factory := layoutmocks.NewMockWidgetFactory(t)
	widget := layoutmocks.NewMockWidget(t)
	factory.EXPECT().WrapNativeWidget(uintptr(456)).Return(widget).Once()
	widget.EXPECT().GtkWidget().Return(nil).Once()
	c := newMinimalCoordinator()
	c.widgetFactory = factory

	assert.Same(t, widget, c.WrapWidget(context.Background(), wv))
}

type syncViewportContextKey struct{}

type syncViewportCapableStub struct {
	port.WebView
	called bool
	ctx    context.Context
	reason string
}

func (s *syncViewportCapableStub) SyncViewport(ctx context.Context, reason string) {
	s.called = true
	s.ctx = ctx
	s.reason = reason
}

func TestLifecycle_SyncWebViewViewport_DelegatesWhenCapabilityPresent(t *testing.T) {
	t.Parallel()

	base := mocks.NewMockWebView(t)
	wv := &syncViewportCapableStub{WebView: base}
	c := newMinimalCoordinator()
	c.webViews[entity.PaneID("pane-1")] = wv

	ctx := context.WithValue(context.Background(), syncViewportContextKey{}, "marker")
	c.SyncWebViewViewport(ctx, "pane-1", "unit-test")

	require.True(t, wv.called)
	require.Equal(t, "unit-test", wv.reason)
	require.Same(t, ctx, wv.ctx)
}

func TestLifecycle_SyncWebViewViewport_NoopWithoutCapability(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	c := newMinimalCoordinator()
	c.webViews[entity.PaneID("pane-1")] = wv

	c.SyncWebViewViewport(context.Background(), "pane-1", "unit-test")
}

type nativeLifecycleWebView struct {
	port.WebView
	widget uintptr
}

func (w *nativeLifecycleWebView) NativeWidget() uintptr { return w.widget }

func TestLifecycle_AttachToWorkspace_RebuildPropagatesRevealStateToFreshPaneView(t *testing.T) {
	for _, tc := range []struct {
		name     string
		revealed bool
	}{
		{name: "revealed current identity hides and stops skeleton", revealed: true},
		{name: "unrevealed current identity shows and starts skeleton", revealed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paneID := entity.PaneID("pane-1")
			ctx := context.Background()
			factory := layoutmocks.NewMockWidgetFactory(t)
			wsView, workspace, paneOverlay, loadingContainer, spinner := newLifecycleRebuildWorkspaceView(t, ctx, factory, paneID)
			wrappedWidget := layoutmocks.NewMockWidget(t)
			webViewMock := mocks.NewMockWebView(t)
			webViewMock.EXPECT().ID().Return(port.WebViewID(201)).Maybe()
			webViewMock.EXPECT().Generation().Return(uint64(3)).Maybe()
			webViewMock.EXPECT().IsDestroyed().Return(false).Once()
			webView := &nativeLifecycleWebView{WebView: webViewMock, widget: 0x351}

			factory.EXPECT().WrapNativeWidget(uintptr(0x351)).Return(wrappedWidget).Once()
			wrappedWidget.EXPECT().GtkWidget().Return(nil).Once()
			wrappedWidget.EXPECT().GetAllocatedWidth().Return(0).Times(3)
			wrappedWidget.EXPECT().GetAllocatedHeight().Return(0).Times(3)
			wrappedWidget.EXPECT().GetParent().Return(nil).Once()
			wrappedWidget.EXPECT().IsVisible().Return(true).Once()
			paneOverlay.EXPECT().GetAllocatedWidth().Return(0).Twice()
			paneOverlay.EXPECT().GetAllocatedHeight().Return(0).Twice()
			paneOverlay.EXPECT().SetChild(wrappedWidget).Once()
			loadingContainer.EXPECT().SetVisible(!tc.revealed).Once()
			if tc.revealed {
				spinner.EXPECT().Stop().Once()
			} else {
				spinner.EXPECT().Start().Once()
			}

			coordinator := newRevealTestCoordinator()
			coordinator.widgetFactory = factory
			coordinator.setWebViewLocked(paneID, webView)
			if tc.revealed {
				identity, ok := identityForWebView(webView)
				require.True(t, ok)
				coordinator.markWebViewRevealed(paneID, identity)
			}

			coordinator.AttachToWorkspace(ctx, workspace, wsView)

			assert.Same(t, wrappedWidget, wsView.GetPaneView(paneID).WebViewWidget())
		})
	}
}

func newLifecycleRebuildWorkspaceView(
	t *testing.T,
	ctx context.Context,
	factory *layoutmocks.MockWidgetFactory,
	paneID entity.PaneID,
) (*component.WorkspaceView, *entity.Workspace, *layoutmocks.MockOverlayWidget, *layoutmocks.MockBoxWidget, *layoutmocks.MockSpinnerWidget) {
	t.Helper()
	workspaceContainer := layoutmocks.NewMockBoxWidget(t)
	workspaceOverlay := layoutmocks.NewMockOverlayWidget(t)
	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(workspaceContainer).Once()
	workspaceContainer.EXPECT().SetHexpand(true).Once()
	workspaceContainer.EXPECT().SetVexpand(true).Once()
	workspaceContainer.EXPECT().SetVisible(true).Once()
	factory.EXPECT().NewOverlay().Return(workspaceOverlay).Once()
	workspaceOverlay.EXPECT().SetHexpand(true).Once()
	workspaceOverlay.EXPECT().SetVexpand(true).Once()
	workspaceOverlay.EXPECT().SetChild(workspaceContainer).Once()
	workspaceOverlay.EXPECT().SetVisible(true).Once()
	wsView := component.NewWorkspaceView(ctx, factory)

	paneOverlay := layoutmocks.NewMockOverlayWidget(t)
	loadingContainer := layoutmocks.NewMockBoxWidget(t)
	loadingContent := layoutmocks.NewMockBoxWidget(t)
	spinner := layoutmocks.NewMockSpinnerWidget(t)
	logo := layoutmocks.NewMockImageWidget(t)
	version := layoutmocks.NewMockLabelWidget(t)
	border := layoutmocks.NewMockBoxWidget(t)

	factory.EXPECT().NewOverlay().Return(paneOverlay).Once()
	paneOverlay.EXPECT().SetHexpand(true).Once()
	paneOverlay.EXPECT().SetVexpand(true).Once()
	paneOverlay.EXPECT().SetVisible(true).Once()
	paneOverlay.EXPECT().AddCssClass("pane-overlay").Once()
	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(loadingContainer).Once()
	loadingContainer.EXPECT().SetHexpand(true).Once()
	loadingContainer.EXPECT().SetVexpand(true).Once()
	loadingContainer.EXPECT().SetHalign(mock.Anything).Once()
	loadingContainer.EXPECT().SetValign(mock.Anything).Once()
	loadingContainer.EXPECT().SetCanFocus(false).Times(2)
	loadingContainer.EXPECT().SetCanTarget(false).Times(2)
	loadingContainer.EXPECT().AddCssClass("loading-skeleton").Once()
	factory.EXPECT().NewBox(layout.OrientationVertical, 6).Return(loadingContent).Once()
	loadingContent.EXPECT().SetHalign(mock.Anything).Once()
	loadingContent.EXPECT().SetValign(mock.Anything).Once()
	loadingContent.EXPECT().SetCanFocus(false).Once()
	loadingContent.EXPECT().SetCanTarget(false).Once()
	loadingContent.EXPECT().AddCssClass("loading-skeleton-content").Once()
	factory.EXPECT().NewImage().Return(logo).Once()
	logo.EXPECT().SetHalign(mock.Anything).Once()
	logo.EXPECT().SetValign(mock.Anything).Once()
	logo.EXPECT().SetCanFocus(false).Once()
	logo.EXPECT().SetCanTarget(false).Once()
	logo.EXPECT().SetSizeRequest(256, 256).Once()
	logo.EXPECT().SetPixelSize(256).Once()
	logo.EXPECT().AddCssClass("loading-skeleton-logo").Once()
	logo.EXPECT().SetFromPaintable(mock.Anything).Maybe()
	factory.EXPECT().NewSpinner().Return(spinner).Once()
	spinner.EXPECT().SetHalign(mock.Anything).Once()
	spinner.EXPECT().SetValign(mock.Anything).Once()
	spinner.EXPECT().SetCanFocus(false).Once()
	spinner.EXPECT().SetCanTarget(false).Once()
	spinner.EXPECT().SetSizeRequest(32, 32).Once()
	spinner.EXPECT().AddCssClass("loading-skeleton-spinner").Once()
	factory.EXPECT().NewLabel(mock.Anything).Return(version).Once()
	version.EXPECT().SetHalign(mock.Anything).Once()
	version.EXPECT().SetValign(mock.Anything).Once()
	version.EXPECT().SetCanFocus(false).Once()
	version.EXPECT().SetCanTarget(false).Once()
	version.EXPECT().SetMaxWidthChars(mock.Anything).Once()
	version.EXPECT().SetEllipsize(mock.Anything).Once()
	version.EXPECT().AddCssClass("loading-skeleton-version").Once()
	loadingContent.EXPECT().Append(logo).Once()
	loadingContent.EXPECT().Append(spinner).Once()
	loadingContent.EXPECT().Append(version).Once()
	loadingContainer.EXPECT().Append(loadingContent).Once()
	loadingContainer.EXPECT().SetVisible(true).Once()
	spinner.EXPECT().Start().Once()
	paneOverlay.EXPECT().AddOverlay(loadingContainer).Once()
	paneOverlay.EXPECT().SetClipOverlay(loadingContainer, false).Once()
	paneOverlay.EXPECT().SetMeasureOverlay(loadingContainer, false).Once()
	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(border).Once()
	border.EXPECT().SetCanFocus(false).Once()
	border.EXPECT().SetCanTarget(false).Once()
	border.EXPECT().AddCssClass("pane-border").Once()
	border.EXPECT().SetHexpand(true).Once()
	border.EXPECT().SetVexpand(true).Once()
	paneOverlay.EXPECT().AddOverlay(border).Once()
	paneOverlay.EXPECT().SetClipOverlay(border, false).Once()
	paneOverlay.EXPECT().SetMeasureOverlay(border, false).Once()

	stackBox := layoutmocks.NewMockBoxWidget(t)
	titleBar := layoutmocks.NewMockBoxWidget(t)
	favicon := layoutmocks.NewMockImageWidget(t)
	title := layoutmocks.NewMockLabelWidget(t)
	closeButton := layoutmocks.NewMockButtonWidget(t)
	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(stackBox).Once()
	stackBox.EXPECT().SetHexpand(true).Once()
	stackBox.EXPECT().SetVexpand(true).Once()
	factory.EXPECT().NewBox(layout.OrientationHorizontal, 4).Return(titleBar).Once()
	titleBar.EXPECT().AddCssClass("stacked-pane-titlebar").Once()
	titleBar.EXPECT().AddCssClass("stacked-pane-title-clickable").Once()
	titleBar.EXPECT().SetVexpand(false).Once()
	titleBar.EXPECT().SetHexpand(true).Once()
	factory.EXPECT().NewImage().Return(favicon).Once()
	favicon.EXPECT().SetFromIconName(mock.Anything).Once()
	favicon.EXPECT().SetPixelSize(16).Once()
	titleBar.EXPECT().Append(favicon).Once()
	factory.EXPECT().NewLabel(mock.Anything).Return(title).Once()
	title.EXPECT().SetEllipsize(layout.EllipsizeEnd).Once()
	title.EXPECT().SetMaxWidthChars(30).Once()
	title.EXPECT().SetHexpand(true).Once()
	title.EXPECT().SetXalign(float32(0)).Once()
	titleBar.EXPECT().Append(title).Once()
	factory.EXPECT().NewButton().Return(closeButton).Once()
	closeButton.EXPECT().SetIconName("window-close-symbolic").Once()
	closeButton.EXPECT().AddCssClass("stacked-pane-close-button").Once()
	closeButton.EXPECT().SetFocusOnClick(false).Once()
	closeButton.EXPECT().SetVexpand(false).Once()
	closeButton.EXPECT().SetHexpand(false).Once()
	titleBar.EXPECT().Append(closeButton).Once()
	titleBar.EXPECT().AddController(mock.Anything).Once()
	closeButton.EXPECT().ConnectClicked(mock.Anything).Return(uint(2)).Once()
	closeButton.EXPECT().GtkWidget().Return(nil).Maybe()
	stackBox.EXPECT().Append(titleBar).Once()
	stackBox.EXPECT().Append(paneOverlay).Once()
	titleBar.EXPECT().SetVisible(false).Once()
	paneOverlay.EXPECT().SetVisible(true).Once()
	titleBar.EXPECT().AddCssClass("active").Once()
	paneOverlay.EXPECT().GtkWidget().Return(nil).Maybe()
	stackBox.EXPECT().SetVisible(true).Once()
	workspaceContainer.EXPECT().Append(stackBox).Once()
	workspaceContainer.EXPECT().AddCssClass("single-pane").Once()
	border.EXPECT().AddCssClass("pane-active").Once()

	workspace := &entity.Workspace{
		ID:           "workspace-1",
		Root:         &entity.PaneNode{ID: "node-1", Pane: entity.NewPane(paneID)},
		ActivePaneID: paneID,
	}
	require.NoError(t, wsView.SetWorkspace(ctx, workspace))
	return wsView, workspace, paneOverlay, loadingContainer, spinner
}

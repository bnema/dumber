package ui

import (
	"context"
	"testing"
	"unsafe"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	layoutmocks "github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecorateFloatingOverlay_AddsThemeClasses(t *testing.T) {
	overlay := layoutmocks.NewMockOverlayWidget(t)
	overlay.EXPECT().SetHexpand(false).Once()
	overlay.EXPECT().SetVexpand(false).Once()
	overlay.EXPECT().AddCssClass("floating-pane-container").Once()
	overlay.EXPECT().AddCssClass("pane-border").Once()
	overlay.EXPECT().AddCssClass("pane-active").Once()

	decorateFloatingOverlay(overlay)
}

func TestConfigureFloatingOverlayMeasurement_DoesNotMeasureOverlay(t *testing.T) {
	workspaceOverlay := layoutmocks.NewMockOverlayWidget(t)
	floatingOverlay := layoutmocks.NewMockWidget(t)

	workspaceOverlay.EXPECT().SetMeasureOverlay(floatingOverlay, false).Once()
	workspaceOverlay.EXPECT().SetClipOverlay(floatingOverlay, false).Once()

	configureFloatingOverlayMeasurement(workspaceOverlay, floatingOverlay)
}

func TestShowFloatingWidget_AddsVisibleClassAndEnablesInteraction(t *testing.T) {
	widget := layoutmocks.NewMockWidget(t)

	widget.EXPECT().AddCssClass(floatingPaneVisibleClass).Once()
	widget.EXPECT().SetCanTarget(true).Once()
	widget.EXPECT().SetCanFocus(true).Once()

	showFloatingWidget(widget)
}

func TestHideFloatingWidget_RemovesVisibleClassAndDisablesInteraction(t *testing.T) {
	widget := layoutmocks.NewMockWidget(t)

	widget.EXPECT().RemoveCssClass(floatingPaneVisibleClass).Once()
	widget.EXPECT().SetCanTarget(false).Once()
	widget.EXPECT().SetCanFocus(false).Once()

	hideFloatingWidget(widget)
}

func TestFloatingAllocationRect_CentersRequestedSize(t *testing.T) {
	x, y, width, height, ok := floatingAllocationRect(1000, 700, 820, 504)

	require.True(t, ok)
	assert.Equal(t, 90, x)
	assert.Equal(t, 98, y)
	assert.Equal(t, 820, width)
	assert.Equal(t, 504, height)
}

func TestFloatingAllocationRect_ClampsToOverlayBounds(t *testing.T) {
	x, y, width, height, ok := floatingAllocationRect(1000, 700, 1200, 900)

	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 0, y)
	assert.Equal(t, 1000, width)
	assert.Equal(t, 700, height)
}

func TestWriteOverlayAllocation_WritesRectangleFields(t *testing.T) {
	type cGtkAllocation struct {
		X      int32
		Y      int32
		Width  int32
		Height int32
	}

	rect := cGtkAllocation{}
	allocationPtr := (*uintptr)(unsafe.Pointer(&rect))

	ok := writeOverlayAllocation(allocationPtr, 11, 22, 33, 44)

	require.True(t, ok)
	assert.Equal(t, int32(11), rect.X)
	assert.Equal(t, int32(22), rect.Y)
	assert.Equal(t, int32(33), rect.Width)
	assert.Equal(t, int32(44), rect.Height)
}

func TestFloatingPane_ToggleActiveWorkspaceSession(t *testing.T) {
	app, tabID, session := newFloatingPaneTestApp(t)

	err := app.ToggleFloatingPane(context.Background())
	require.NoError(t, err)
	assert.True(t, session.pane.IsVisible())
	assert.Equal(t, "about:blank", session.pane.CurrentURL())

	err = app.ToggleFloatingPane(context.Background())
	require.NoError(t, err)
	assert.False(t, session.pane.IsVisible())
	assert.Equal(t, "about:blank", session.pane.CurrentURL())

	_, gotTabID := app.activeFloatingSession()
	assert.Equal(t, tabID, gotTabID)
}

func TestFloatingPane_ProfileShortcutOpensConfiguredURL(t *testing.T) {
	app, _, session := newFloatingPaneTestApp(t)

	err := app.OpenFloatingPaneURL(context.Background(), "https://google.com")
	require.NoError(t, err)

	assert.True(t, session.pane.IsVisible())
	assert.False(t, session.pane.IsOmniboxVisible())
	assert.Equal(t, "https://google.com", session.pane.CurrentURL())
}

func TestFloatingPane_ActivePaneSwitchPreservesSession(t *testing.T) {
	app, tabID, session := newFloatingPaneTestApp(t)

	err := app.OpenFloatingPaneURL(context.Background(), "https://example.com/start")
	require.NoError(t, err)

	activeTab := app.tabs.Find(tabID)
	require.NotNil(t, activeTab)
	activeTab.Workspace.ActivePaneID = entity.PaneID("pane-two")

	err = app.ToggleFloatingPane(context.Background())
	require.NoError(t, err)
	err = app.ToggleFloatingPane(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "https://example.com/start", session.pane.CurrentURL())
}

func TestFloatingPane_OmniboxNavigationTargetsFloatingSession(t *testing.T) {
	app, _, session := newFloatingPaneTestApp(t)

	require.NoError(t, session.pane.ShowToggle(context.Background()))
	require.True(t, session.pane.IsOmniboxVisible())

	err := app.navigateFromOmnibox(context.Background(), "https://example.com/omnibox")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/omnibox", session.pane.CurrentURL())
}

func TestFloatingPane_HandleViewportTick_RecalculatesOnWorkspaceResize(t *testing.T) {
	ctx := context.Background()
	overlay := layoutmocks.NewMockOverlayWidget(t)
	width := 1000
	height := 700
	overlay.EXPECT().GetAllocatedWidth().RunAndReturn(func() int { return width }).Maybe()
	overlay.EXPECT().GetAllocatedHeight().RunAndReturn(func() int { return height }).Maybe()

	widget := layoutmocks.NewMockWidget(t)
	pane := component.NewFloatingPane(overlay, component.FloatingPaneOptions{
		WidthPct:       0.82,
		HeightPct:      0.72,
		FallbackWidth:  floatingPaneFallbackWidth,
		FallbackHeight: floatingPaneFallbackHeight,
	})
	require.NoError(t, pane.ShowURL(ctx, "https://example.com"))

	session := &floatingWorkspaceSession{pane: pane, overlay: overlay, widget: widget}
	app := &App{}

	widget.EXPECT().SetSizeRequest(820, 504).Once()
	assert.True(t, app.handleFloatingViewportTick(session))

	width = 1200
	height = 900
	widget.EXPECT().SetSizeRequest(983, 648).Once()
	assert.True(t, app.handleFloatingViewportTick(session))

	pane.Hide(ctx)
	assert.False(t, app.handleFloatingViewportTick(session))
}

func TestFloatingPane_HideShowAfterURIUpdateKeepsOmniboxHidden(t *testing.T) {
	app, _, session := newFloatingPaneTestApp(t)

	require.NoError(t, app.ToggleFloatingPane(context.Background()))
	require.True(t, session.pane.IsOmniboxVisible())

	app.updateFloatingSessionURI(session.paneID, "https://google.com")

	require.NoError(t, app.ToggleFloatingPane(context.Background()))
	require.NoError(t, app.ToggleFloatingPane(context.Background()))

	assert.Equal(t, "https://google.com", session.pane.CurrentURL())
	assert.False(t, session.pane.IsOmniboxVisible())
}

func TestFloatingPane_ProfileSessionsRemainIndependent(t *testing.T) {
	app, tabID, _ := newFloatingPaneTestApp(t)

	gmailSession := newFloatingPaneSession(tabID, "profile:gmail")
	blankSession := newFloatingPaneSession(tabID, "profile:blank")
	app.floatingSessions[floatingSessionKey{tabID: tabID, sessionID: "profile:gmail"}] = gmailSession
	app.floatingSessions[floatingSessionKey{tabID: tabID, sessionID: "profile:blank"}] = blankSession

	require.NoError(t, app.OpenFloatingPaneProfileURL(context.Background(), "profile:gmail", "https://mail.google.com"))
	require.NoError(t, app.ToggleFloatingPane(context.Background()))
	require.NoError(t, app.OpenFloatingPaneProfileURL(context.Background(), "profile:blank", "about:blank"))

	assert.Equal(t, "https://mail.google.com", gmailSession.pane.CurrentURL())
	assert.False(t, gmailSession.pane.IsVisible())
	assert.Equal(t, "about:blank", blankSession.pane.CurrentURL())
	assert.True(t, blankSession.pane.IsVisible())
	assert.NotEqual(t, gmailSession.paneID, blankSession.paneID)
}

func TestFloatingPane_ProfileShortcutTogglesAndPreservesSessionState(t *testing.T) {
	app, tabID, _ := newFloatingPaneTestApp(t)
	session := newFloatingPaneSession(tabID, "profile:gmail")
	app.floatingSessions[floatingSessionKey{tabID: tabID, sessionID: "profile:gmail"}] = session

	baseURL := "https://mail.google.com"
	draftURL := "https://mail.google.com/mail/u/0/#inbox?compose=new"

	// First open: navigates to base URL
	require.NoError(t, app.OpenFloatingPaneProfileURL(context.Background(), "profile:gmail", baseURL))
	assert.True(t, session.pane.IsVisible())
	assert.Equal(t, baseURL, session.pane.CurrentURL())

	// Simulate user navigating within gmail (compose draft etc.)
	app.updateFloatingSessionURI(session.paneID, draftURL)
	assert.Equal(t, draftURL, session.pane.CurrentURL())

	// Second press: hides (toggle off), preserves draft URL
	require.NoError(t, app.OpenFloatingPaneProfileURL(context.Background(), "profile:gmail", baseURL))
	assert.False(t, session.pane.IsVisible())
	assert.Equal(t, draftURL, session.pane.CurrentURL())

	// Third press: shows again without re-navigating, draft URL preserved
	require.NoError(t, app.OpenFloatingPaneProfileURL(context.Background(), "profile:gmail", baseURL))
	assert.True(t, session.pane.IsVisible())
	assert.Equal(t, draftURL, session.pane.CurrentURL())
}

func TestFloatingPane_ProfileRapidToggleNeverShowsAboutBlank(t *testing.T) {
	app, tabID, _ := newFloatingPaneTestApp(t)
	session := newFloatingPaneSession(tabID, "profile:gmail")
	app.floatingSessions[floatingSessionKey{tabID: tabID, sessionID: "profile:gmail"}] = session

	baseURL := "https://mail.google.com"

	// Rapid toggle 10 times: should never end up on about:blank
	for i := 0; i < 10; i++ {
		require.NoError(t, app.OpenFloatingPaneProfileURL(context.Background(), "profile:gmail", baseURL))
		assert.NotEqual(t, "about:blank", session.pane.CurrentURL(),
			"iteration %d: profile session should never show about:blank", i)
		if session.pane.IsVisible() {
			assert.Equal(t, baseURL, session.pane.CurrentURL())
		}
	}
}

func TestFloatingPane_DefaultSessionStillUsesAboutBlank(t *testing.T) {
	app, _, session := newFloatingPaneTestApp(t)

	require.NoError(t, app.ToggleFloatingPane(context.Background()))
	assert.True(t, session.pane.IsVisible())
	assert.Equal(t, "about:blank", session.pane.CurrentURL())
}

func TestFloatingPane_CloseActiveFloatingSession(t *testing.T) {
	app, tabID, _ := newFloatingPaneTestApp(t)
	session := newFloatingPaneSession(tabID, "profile:gmail")
	app.floatingSessions[floatingSessionKey{tabID: tabID, sessionID: "profile:gmail"}] = session

	require.NoError(t, app.OpenFloatingPaneProfileURL(context.Background(), "profile:gmail", "https://mail.google.com"))

	handled := app.closeActiveFloatingPane(context.Background())
	assert.True(t, handled)
	assert.False(t, session.pane.IsVisible())

	handled = app.closeActiveFloatingPane(context.Background())
	assert.False(t, handled)
}

func TestFloatingPane_OpenFloatingPaneSession_OptionalURL(t *testing.T) {
	app, _, session := newFloatingPaneTestApp(t)

	require.NoError(t, app.openFloatingPaneSession(context.Background(), floatingSessionIDDefault))
	assert.True(t, session.pane.IsVisible())
	assert.Equal(t, "about:blank", session.pane.CurrentURL())

	require.NoError(t, app.openFloatingPaneSession(context.Background(), floatingSessionIDDefault, "https://example.com"))
	assert.True(t, session.pane.IsVisible())
	assert.Equal(t, "https://example.com", session.pane.CurrentURL())
}

func TestFloatingPane_EnsureFloatingSession_CachedAllowsNilWorkspaceView(t *testing.T) {
	app, tabID, session := newFloatingPaneTestApp(t)

	got, err := app.ensureFloatingSession(context.Background(), tabID, floatingSessionIDDefault, nil)
	require.NoError(t, err)
	assert.Same(t, session, got)
}

func TestFloatingPane_HideFloatingSession_StopsResizeWatcher(t *testing.T) {
	session := newFloatingPaneSession(entity.TabID("tab-1"), "profile:test")
	session.resizeWatcherActive = true
	session.resizeTickID = 42
	session.appliedWidth = 960
	session.appliedHeight = 640

	app := &App{}
	app.hideFloatingSession(context.Background(), session)

	assert.False(t, session.resizeWatcherActive)
	assert.Equal(t, uint(0), session.resizeTickID)
	assert.Equal(t, 0, session.appliedWidth)
	assert.Equal(t, 0, session.appliedHeight)
}

func newFloatingPaneTestApp(t *testing.T) (*App, entity.TabID, *floatingWorkspaceSession) {
	t.Helper()

	tabID := entity.TabID("tab-1")
	workspace := &entity.Workspace{ActivePaneID: entity.PaneID("pane-one")}
	tabs := entity.NewTabList()
	tabs.Add(&entity.Tab{ID: tabID, Workspace: workspace})

	session := newFloatingPaneSession(tabID, floatingSessionIDDefault)

	app := &App{
		tabs:           tabs,
		workspaceViews: make(map[entity.TabID]*component.WorkspaceView),
		floatingSessions: map[floatingSessionKey]*floatingWorkspaceSession{
			{tabID: tabID, sessionID: floatingSessionIDDefault}: session,
		},
	}

	return app, tabID, session
}

func newFloatingPaneSession(tabID entity.TabID, sessionID string) *floatingWorkspaceSession {
	return &floatingWorkspaceSession{
		paneID: floatingPaneIDForSession(tabID, sessionID),
		pane: component.NewFloatingPane(nil, component.FloatingPaneOptions{
			WidthPct:       0.82,
			HeightPct:      0.72,
			FallbackWidth:  floatingPaneFallbackWidth,
			FallbackHeight: floatingPaneFallbackHeight,
			OnNavigate: func(context.Context, string) error {
				return nil
			},
		}),
	}
}

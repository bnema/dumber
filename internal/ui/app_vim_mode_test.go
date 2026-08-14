package ui

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/puregotk/v4/glib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/bnema/dumber/internal/ui/theme"
)

// ---------------------------------------------------------------------------
// Mock-based PaneView and WorkspaceView creation helpers
//
// These are used by vimModeTestFixture to build a minimal test environment
// without any GTK infrastructure.
// ---------------------------------------------------------------------------

// newLoadingSkeletonMocks sets up the mock expectations for the loading
// skeleton that NewPaneView creates internally.
func newLoadingSkeletonMocks(
	t *testing.T,
	factory *mocks.MockWidgetFactory,
	overlay *mocks.MockOverlayWidget,
) {
	t.Helper()

	loadingContainer := mocks.NewMockBoxWidget(t)
	loadingContent := mocks.NewMockBoxWidget(t)
	spinner := mocks.NewMockSpinnerWidget(t)
	logo := mocks.NewMockImageWidget(t)
	version := mocks.NewMockLabelWidget(t)

	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(loadingContainer).Once()
	loadingContainer.EXPECT().SetHexpand(true).Maybe()
	loadingContainer.EXPECT().SetVexpand(true).Maybe()
	loadingContainer.EXPECT().SetHalign(mock.Anything).Maybe()
	loadingContainer.EXPECT().SetValign(mock.Anything).Maybe()
	loadingContainer.EXPECT().SetCanFocus(false).Maybe()
	loadingContainer.EXPECT().SetCanTarget(false).Maybe()
	loadingContainer.EXPECT().AddCssClass(mock.Anything).Maybe()
	loadingContainer.EXPECT().SetVisible(mock.Anything).Maybe()

	factory.EXPECT().NewBox(layout.OrientationVertical, 6).Return(loadingContent).Once()
	loadingContent.EXPECT().SetHalign(mock.Anything).Maybe()
	loadingContent.EXPECT().SetValign(mock.Anything).Maybe()
	loadingContent.EXPECT().SetCanFocus(false).Maybe()
	loadingContent.EXPECT().SetCanTarget(false).Maybe()
	loadingContent.EXPECT().AddCssClass(mock.Anything).Maybe()

	factory.EXPECT().NewImage().Return(logo).Once()
	logo.EXPECT().SetHalign(mock.Anything).Maybe()
	logo.EXPECT().SetValign(mock.Anything).Maybe()
	logo.EXPECT().SetCanFocus(false).Maybe()
	logo.EXPECT().SetCanTarget(false).Maybe()
	logo.EXPECT().SetSizeRequest(mock.Anything, mock.Anything).Maybe()
	logo.EXPECT().SetPixelSize(mock.Anything).Maybe()
	logo.EXPECT().AddCssClass(mock.Anything).Maybe()
	logo.EXPECT().SetFromPaintable(mock.Anything).Maybe()

	factory.EXPECT().NewSpinner().Return(spinner).Once()
	spinner.EXPECT().SetHalign(mock.Anything).Maybe()
	spinner.EXPECT().SetValign(mock.Anything).Maybe()
	spinner.EXPECT().SetCanFocus(false).Maybe()
	spinner.EXPECT().SetCanTarget(false).Maybe()
	spinner.EXPECT().SetSizeRequest(mock.Anything, mock.Anything).Maybe()
	spinner.EXPECT().AddCssClass(mock.Anything).Maybe()
	spinner.EXPECT().Start().Maybe()
	spinner.EXPECT().Stop().Maybe()

	factory.EXPECT().NewLabel(mock.Anything).Return(version).Once()
	version.EXPECT().SetHalign(mock.Anything).Maybe()
	version.EXPECT().SetValign(mock.Anything).Maybe()
	version.EXPECT().SetCanFocus(false).Maybe()
	version.EXPECT().SetCanTarget(false).Maybe()
	version.EXPECT().SetMaxWidthChars(mock.Anything).Maybe()
	version.EXPECT().SetEllipsize(mock.Anything).Maybe()
	version.EXPECT().AddCssClass(mock.Anything).Maybe()

	loadingContent.EXPECT().Append(logo).Maybe()
	loadingContent.EXPECT().Append(spinner).Maybe()
	loadingContent.EXPECT().Append(version).Maybe()
	loadingContainer.EXPECT().Append(loadingContent).Maybe()

	overlay.EXPECT().AddOverlay(loadingContainer).Once()
	overlay.EXPECT().SetClipOverlay(loadingContainer, false).Once()
	overlay.EXPECT().SetMeasureOverlay(loadingContainer, false).Once()
}

// createTestPaneView builds a mock-backed PaneView.  Returns the PaneView and
// its overlay mock so subsequent vim-mode indicator / pulse expectations can
// be set up by the caller.
func createTestPaneView(
	t *testing.T,
	factory *mocks.MockWidgetFactory,
	paneID entity.PaneID,
) (*component.PaneView, *mocks.MockOverlayWidget) {
	t.Helper()

	overlay := mocks.NewMockOverlayWidget(t)
	borderBox := mocks.NewMockBoxWidget(t)

	// Overlay
	factory.EXPECT().NewOverlay().Return(overlay).Once()
	overlay.EXPECT().SetHexpand(true).Once()
	overlay.EXPECT().SetVexpand(true).Once()
	overlay.EXPECT().SetVisible(true).Once()
	overlay.EXPECT().AddCssClass("pane-overlay").Once()

	// Loading skeleton
	newLoadingSkeletonMocks(t, factory, overlay)

	// Border box
	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(borderBox).Once()
	borderBox.EXPECT().SetCanFocus(false).Once()
	borderBox.EXPECT().SetCanTarget(false).Once()
	borderBox.EXPECT().AddCssClass("pane-border").Once()
	borderBox.EXPECT().SetHexpand(true).Once()
	borderBox.EXPECT().SetVexpand(true).Once()
	overlay.EXPECT().AddOverlay(borderBox).Once()
	overlay.EXPECT().SetClipOverlay(borderBox, false).Once()
	overlay.EXPECT().SetMeasureOverlay(borderBox, false).Once()

	pv := component.NewPaneView(context.Background(), factory, paneID, nil)
	require.NotNil(t, pv)

	return pv, overlay
}

// enterVimMode marks the owning pane with the pane-local visual accent.
func enterVimMode(
	t *testing.T,
	app *App,
	overlay *mocks.MockOverlayWidget,
	bw *browserWindow,
) {
	t.Helper()
	overlay.EXPECT().AddCssClass("vim-mode-active").Once()
	app.handleVimModeOwnership(context.Background(), bw, input.ModeVim, input.ModeNormal)
}

func expectVimModeAccentHidden(overlay *mocks.MockOverlayWidget) {
	overlay.EXPECT().RemoveCssClass("vim-mode-active").Once()
}

type accessibilityEnablingWebView struct {
	*portmocks.MockWebView
	enableAccessibilityCalls int
}

func newAccessibilityEnablingWebView(t *testing.T, id port.WebViewID) *accessibilityEnablingWebView {
	t.Helper()
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(id).Once()
	return &accessibilityEnablingWebView{MockWebView: wv}
}

func (wv *accessibilityEnablingWebView) EnableAccessibility() {
	wv.enableAccessibilityCalls++
}

func newVimModeAccessibilityFixture(t *testing.T) (*App, *browserWindow, entity.PaneID) {
	t.Helper()
	paneID := entity.PaneID("pane-a")
	ws := &entity.Workspace{
		ID:           "ws-1",
		ActivePaneID: paneID,
		Root: &entity.PaneNode{
			ID:   "ws-1-root",
			Pane: entity.NewPane(paneID),
		},
	}
	tab := &entity.Tab{ID: "tab-1", Workspace: ws}
	tabs := entity.NewTabList()
	tabs.Add(tab)
	tabs.SetActive(tab.ID)
	bw := &browserWindow{id: "win-1", tabs: tabs}
	return &App{
		contentCoord: contentcoord.NewCoordinator(context.Background(), nil, nil, nil, nil, nil, nil, nil),
	}, bw, paneID
}

// setupWorkspaceViewMocks creates a mock-backed WorkspaceView.
func setupWorkspaceViewMocks(
	t *testing.T,
	factory *mocks.MockWidgetFactory,
) *component.WorkspaceView {
	t.Helper()

	box := mocks.NewMockBoxWidget(t)
	overlay := mocks.NewMockOverlayWidget(t)

	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(box).Once()
	box.EXPECT().SetHexpand(true).Once()
	box.EXPECT().SetVexpand(true).Once()
	box.EXPECT().SetVisible(true).Once()

	factory.EXPECT().NewOverlay().Return(overlay).Once()
	overlay.EXPECT().SetHexpand(true).Once()
	overlay.EXPECT().SetVexpand(true).Once()
	overlay.EXPECT().SetChild(box).Once()
	overlay.EXPECT().SetVisible(true).Once()

	wv := component.NewWorkspaceView(context.Background(), factory)
	require.NotNil(t, wv)

	return wv
}

// newKeyboardHandlerInVimMode creates a KeyboardHandler whose Mode() returns
// ModeVim — no GTK widgets required.
func newKeyboardHandlerInVimMode(t *testing.T) *input.KeyboardHandler {
	t.Helper()
	kh := input.NewKeyboardHandler(
		context.Background(),
		&entity.WorkspaceConfig{},
		&entity.SessionConfig{},
	)
	kh.EnterVimMode()
	return kh
}

func bindVimModeKeyboardHandler(t *testing.T, app *App, bw *browserWindow) *input.KeyboardHandler {
	t.Helper()
	kh := newKeyboardHandlerInVimMode(t)
	kh.SetOnModeChange(func(from, to input.Mode) {
		app.handleModeChange(context.Background(), bw, from, to)
	})
	bw.keyboardHandler = kh
	return kh
}

// ---------------------------------------------------------------------------
// vimModeTestFixture — complete, minimal App + workspace environment
//
// Provides two browser windows, each with a two-pane workspace.  The active
// pane on both workspaces starts at "pane-a".
// ---------------------------------------------------------------------------

type vimModeTestFixture struct {
	app  *App
	bw1  *browserWindow
	bw2  *browserWindow
	pv1A *component.PaneView // bw1 / pane-a (active)
	pv1B *component.PaneView // bw1 / pane-b
	pv2A *component.PaneView // bw2 / pane-a (active)
	pv2B *component.PaneView // bw2 / pane-b

	factory   *mocks.MockWidgetFactory
	overlay1A *mocks.MockOverlayWidget
	overlay1B *mocks.MockOverlayWidget
	overlay2A *mocks.MockOverlayWidget
	overlay2B *mocks.MockOverlayWidget
}

func newVimModeTestFixture(t *testing.T) *vimModeTestFixture {
	t.Helper()
	factory := mocks.NewMockWidgetFactory(t)

	// ---- Browser window 1 ----
	pv1A, ol1A := createTestPaneView(t, factory, "pane-a")
	pv1B, ol1B := createTestPaneView(t, factory, "pane-b")

	ws1 := &entity.Workspace{
		ID:           "ws-1",
		ActivePaneID: "pane-a",
		Root: &entity.PaneNode{
			ID: "ws-1-root",
			Children: []*entity.PaneNode{
				{ID: "ws-1-pane-a", Pane: entity.NewPane("pane-a")},
				{ID: "ws-1-pane-b", Pane: entity.NewPane("pane-b")},
			},
			SplitDir: entity.SplitHorizontal,
		},
	}
	tab1 := &entity.Tab{ID: "tab-1", Workspace: ws1}
	tabs1 := entity.NewTabList()
	tabs1.Add(tab1)
	tabs1.SetActive(tab1.ID)

	wv1 := setupWorkspaceViewMocks(t, factory)
	wv1.SetPaneViewsForTest(map[entity.PaneID]*component.PaneView{
		"pane-a": pv1A, "pane-b": pv1B,
	})

	bw1 := &browserWindow{id: "win-1", tabs: tabs1}

	// ---- Browser window 2 ----
	pv2A, ol2A := createTestPaneView(t, factory, "pane-a")
	pv2B, ol2B := createTestPaneView(t, factory, "pane-b")

	ws2 := &entity.Workspace{
		ID:           "ws-2",
		ActivePaneID: "pane-a",
		Root: &entity.PaneNode{
			ID: "ws-2-root",
			Children: []*entity.PaneNode{
				{ID: "ws-2-pane-a", Pane: entity.NewPane("pane-a")},
				{ID: "ws-2-pane-b", Pane: entity.NewPane("pane-b")},
			},
			SplitDir: entity.SplitHorizontal,
		},
	}
	tab2 := &entity.Tab{ID: "tab-2", Workspace: ws2}
	tabs2 := entity.NewTabList()
	tabs2.Add(tab2)
	tabs2.SetActive(tab2.ID)

	wv2 := setupWorkspaceViewMocks(t, factory)
	wv2.SetPaneViewsForTest(map[entity.PaneID]*component.PaneView{
		"pane-a": pv2A, "pane-b": pv2B,
	})

	bw2 := &browserWindow{id: "win-2", tabs: tabs2}

	app := &App{
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			"tab-1": wv1, "tab-2": wv2,
		},
		browserWindows: map[string]*browserWindow{
			"win-1": bw1, "win-2": bw2,
		},
		lastFocusedWindowID: "win-1",
	}

	return &vimModeTestFixture{
		app: app, bw1: bw1, bw2: bw2,
		pv1A: pv1A, pv1B: pv1B, pv2A: pv2A, pv2B: pv2B,
		factory:   factory,
		overlay1A: ol1A, overlay1B: ol1B,
		overlay2A: ol2A, overlay2B: ol2B,
	}
}

func newSingleWindowVimModeFixture(t *testing.T) *vimModeTestFixture {
	t.Helper()
	f := newVimModeTestFixture(t)
	delete(f.app.browserWindows, "win-2")
	delete(f.app.workspaceViews, entity.TabID("tab-2"))
	f.bw2 = nil
	f.pv2A = nil
	f.pv2B = nil
	f.overlay2A = nil
	f.overlay2B = nil
	return f
}

// ============================================================================
// 1. Entering Vim mode marks only the active pane
// ============================================================================

func TestVimMode_Enter_SetsOwnershipOnActivePane(t *testing.T) {
	f := newVimModeTestFixture(t)

	enterVimMode(t, f.app, f.overlay1A, f.bw1)

	assert.Equal(t, entity.PaneID("pane-a"), f.bw1.vimModePaneID,
		"entering vim mode sets ownership on the active pane")
	assert.True(t, f.pv1A.IsVimMode(),
		"the active pane view enters vim mode")
	assert.False(t, f.pv1B.IsVimMode(),
		"the inactive pane is NOT in vim mode")
	assert.Empty(t, f.bw2.vimModePaneID,
		"second window is untouched")
	assert.False(t, f.pv2A.IsVimMode(),
		"second window active pane is NOT in vim mode")
}

func TestVimMode_Enter_MarksCorrectWindow(t *testing.T) {
	f := newVimModeTestFixture(t)

	enterVimMode(t, f.app, f.overlay2A, f.bw2)

	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.vimModePaneID)
	assert.True(t, f.pv2A.IsVimMode())
	assert.Empty(t, f.bw1.vimModePaneID)
	assert.False(t, f.pv1A.IsVimMode())
}

func TestVimMode_Enter_EnablesAccessibilityForSupportedActiveWebView(t *testing.T) {
	app, bw, paneID := newVimModeAccessibilityFixture(t)
	wv := newAccessibilityEnablingWebView(t, 1)
	app.contentCoord.RegisterPopupWebView(paneID, wv)

	app.handleModeChange(context.Background(), bw, input.ModeNormal, input.ModeVim)

	assert.Equal(t, 1, wv.enableAccessibilityCalls)
}

func TestVimMode_Enter_AccessibilityNoopsForUnsupportedActiveWebView(t *testing.T) {
	app, bw, paneID := newVimModeAccessibilityFixture(t)
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(2)).Once()
	app.contentCoord.RegisterPopupWebView(paneID, wv)

	app.handleModeChange(context.Background(), bw, input.ModeNormal, input.ModeVim)
}

func TestVimMode_Enter_AccessibilityNoopsWhenActiveWebViewMissing(t *testing.T) {
	app, bw, _ := newVimModeAccessibilityFixture(t)

	app.handleModeChange(context.Background(), bw, input.ModeNormal, input.ModeVim)
}

func TestVimMode_AccessibilityOnlyEnabledOnEnteringVimMode(t *testing.T) {
	app, bw, paneID := newVimModeAccessibilityFixture(t)
	wv := newAccessibilityEnablingWebView(t, 3)
	app.contentCoord.RegisterPopupWebView(paneID, wv)

	app.handleModeChange(context.Background(), bw, input.ModeNormal, input.ModeTab)
	app.handleModeChange(context.Background(), bw, input.ModeVim, input.ModeNormal)
	app.handleModeChange(context.Background(), bw, input.ModeVim, input.ModeVim)
	assert.Equal(t, 0, wv.enableAccessibilityCalls)

	app.handleModeChange(context.Background(), bw, input.ModeNormal, input.ModeVim)
	assert.Equal(t, 1, wv.enableAccessibilityCalls)

	app.handleModeChange(context.Background(), bw, input.ModePane, input.ModeVim)
	assert.Equal(t, 2, wv.enableAccessibilityCalls)
}

// ============================================================================
// 2. Leaving Vim mode clears ownership
// ============================================================================

func TestVimMode_Leave_ClearsOwnershipFromOwningPane(t *testing.T) {
	f := newVimModeTestFixture(t)

	// Enter first
	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.vimModePaneID)

	// Leave — SetVimMode(false) hides indicator and removes the CSS class
	expectVimModeAccentHidden(f.overlay1A)
	f.app.handleVimModeOwnership(context.Background(), f.bw1, input.ModeNormal, input.ModeVim)

	assert.Empty(t, f.bw1.vimModePaneID,
		"leaving vim mode clears ownership field")
	assert.False(t, f.pv1A.IsVimMode(),
		"the previously owning pane exits vim mode")
}

// ============================================================================
// 3. Transfer within the same window
// ============================================================================

func TestVimMode_Transfer_MovesOwnershipToNewPane(t *testing.T) {
	f := newVimModeTestFixture(t)

	// Enter vim mode on bw1 pane-a — save label for hide expectation
	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.vimModePaneID)

	// Give bw1 a keyboard handler in vim mode so transfer performs the switch
	f.bw1.keyboardHandler = newKeyboardHandlerInVimMode(t)

	// transferVimModeOwnershipToPane calls SetVimMode(false) on the old
	// pane before SetVimMode(true) on the new pane.  Set up both.
	expectVimModeAccentHidden(f.overlay1A)

	// Set up the pane-local accent for pane-b.
	f.overlay1B.EXPECT().AddCssClass("vim-mode-active").Once()

	// Transfer ownership to pane-b
	f.app.transferVimModeOwnershipToPane(context.Background(), f.bw1, entity.PaneID("pane-b"))

	assert.Equal(t, entity.PaneID("pane-b"), f.bw1.vimModePaneID,
		"ownership transferred to new pane")
	assert.False(t, f.pv1A.IsVimMode(),
		"previous owning pane deactivated")
	assert.True(t, f.pv1B.IsVimMode(),
		"new pane is now in vim mode")
}

func TestVimMode_Transfer_NoVimModeClearsStale(t *testing.T) {
	f := newVimModeTestFixture(t)

	f.bw1.vimModePaneID = entity.PaneID("pane-a")
	f.bw1.keyboardHandler = input.NewKeyboardHandler(
		context.Background(),
		&entity.WorkspaceConfig{},
		&entity.SessionConfig{},
	)
	// No EnterVimMode — handler stays in ModeNormal.

	f.app.transferVimModeOwnershipToPane(context.Background(), f.bw1, entity.PaneID("pane-b"))

	assert.Empty(t, f.bw1.vimModePaneID,
		"stale ownership cleared when window is not in vim mode")
	assert.False(t, f.pv1A.IsVimMode())
	assert.False(t, f.pv1B.IsVimMode())
}

func TestVimMode_Transfer_NilBwDoesNotCrash(t *testing.T) {
	app := &App{}
	app.transferVimModeOwnershipToPane(context.Background(), nil, entity.PaneID("pane-1"))
}

func TestVimMode_Transfer_SamePaneIsNoop(t *testing.T) {
	app := &App{}
	paneID := entity.PaneID("pane-a")
	bw := &browserWindow{vimModePaneID: paneID}
	app.transferVimModeOwnershipToPane(context.Background(), bw, paneID)
	assert.Equal(t, paneID, bw.vimModePaneID)
}

func TestVimMode_Transfer_EmptyOwnershipIsNoop(t *testing.T) {
	app := &App{}
	bw := &browserWindow{}
	app.transferVimModeOwnershipToPane(context.Background(), bw, entity.PaneID("pane-a"))
	assert.Empty(t, bw.vimModePaneID)
}

// ============================================================================
// 4. Pulse targeting — normal vs fast
// ============================================================================

func setUpNormalPulse(overlay *mocks.MockOverlayWidget) {
	overlay.EXPECT().RemoveCssClass("vim-mode-pulse").Once()
	overlay.EXPECT().RemoveCssClass("vim-mode-pulse-fast").Once()
	overlay.EXPECT().RemoveCssClass("vim-mode-pulse-cycle-a").Once()
	overlay.EXPECT().RemoveCssClass("vim-mode-pulse-cycle-b").Once()
	overlay.EXPECT().AddCssClass("vim-mode-pulse").Once()
	overlay.EXPECT().AddCssClass("vim-mode-pulse-cycle-a").Once()
}

func setUpFastPulse(overlay *mocks.MockOverlayWidget) {
	overlay.EXPECT().RemoveCssClass("vim-mode-pulse").Once()
	overlay.EXPECT().RemoveCssClass("vim-mode-pulse-fast").Once()
	overlay.EXPECT().RemoveCssClass("vim-mode-pulse-cycle-a").Once()
	overlay.EXPECT().RemoveCssClass("vim-mode-pulse-cycle-b").Once()
	overlay.EXPECT().AddCssClass("vim-mode-pulse-fast").Once()
	overlay.EXPECT().AddCssClass("vim-mode-pulse-cycle-a").Once()
}

func TestVimMode_Pulse_NormalTriggersOnOwningPane(t *testing.T) {
	f := newVimModeTestFixture(t)

	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.vimModePaneID)

	// Pulse needs the indicator to exist — it was created lazily during
	// enterVimMode, but we also need to set up pulse expectations.
	// (enterVimMode created it, so we just set up pulse mocks.)
	setUpNormalPulse(f.overlay1A)

	f.app.triggerVimModePulse(context.Background(), false)
}

func TestVimMode_Pulse_FastTriggersOnOwningPane(t *testing.T) {
	f := newVimModeTestFixture(t)

	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.vimModePaneID)

	setUpFastPulse(f.overlay1A)

	f.app.triggerVimModePulse(context.Background(), true)
}

func TestVimMode_Pulse_NoOwnerIsNoop(t *testing.T) {
	f := newVimModeTestFixture(t)
	f.app.triggerVimModePulse(context.Background(), false)
	f.app.triggerVimModePulse(context.Background(), true)
	// No pane has ownership — no pulse expectations needed.
}

// ============================================================================
// 5. Multi-window isolation
// ============================================================================

func TestVimMode_MultiWindow_TwoWindowsEnterLeave(t *testing.T) {
	f := newVimModeTestFixture(t)

	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	enterVimMode(t, f.app, f.overlay2A, f.bw2)

	assert.Equal(t, entity.PaneID("pane-a"), f.bw1.vimModePaneID)
	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.vimModePaneID)

	// Leave bw1 only
	expectVimModeAccentHidden(f.overlay1A)
	f.app.handleVimModeOwnership(context.Background(), f.bw1, input.ModeNormal, input.ModeVim)

	assert.Empty(t, f.bw1.vimModePaneID)
	assert.False(t, f.pv1A.IsVimMode())

	// bw2 unchanged
	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.vimModePaneID)
	assert.True(t, f.pv2A.IsVimMode())
}

func TestVimMode_MultiWindow_TransferIsolated(t *testing.T) {
	f := newVimModeTestFixture(t)

	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	enterVimMode(t, f.app, f.overlay2A, f.bw2)

	f.bw1.keyboardHandler = newKeyboardHandlerInVimMode(t)

	// transferVimModeOwnershipToPane calls SetVimMode(false) on old pane first
	expectVimModeAccentHidden(f.overlay1A)

	// Transfer bw1 to pane-b.
	f.overlay1B.EXPECT().AddCssClass("vim-mode-active").Once()
	f.app.transferVimModeOwnershipToPane(context.Background(), f.bw1, entity.PaneID("pane-b"))

	assert.Equal(t, entity.PaneID("pane-b"), f.bw1.vimModePaneID)
	assert.True(t, f.pv1B.IsVimMode())
	assert.False(t, f.pv1A.IsVimMode())

	// bw2 unchanged
	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.vimModePaneID)
	assert.True(t, f.pv2A.IsVimMode())
}

func TestVimMode_MultiWindow_PulseIsolated(t *testing.T) {
	f := newVimModeTestFixture(t)

	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	enterVimMode(t, f.app, f.overlay2A, f.bw2)

	// Pulse on bw1 (lastFocusedWindowID = "win-1")
	setUpNormalPulse(f.overlay1A)
	f.app.triggerVimModePulse(context.Background(), false)

	// bw2 pane still in vim mode
	assert.True(t, f.pv2A.IsVimMode())
}

// ============================================================================
// Edge cases — nil / no-workspace / startup guards
// ============================================================================

func TestVimMode_Enter_NoWorkspaceIsNoop(t *testing.T) {
	app := &App{}
	bw := &browserWindow{id: "win-1"}
	app.handleVimModeOwnership(context.Background(), bw, input.ModeVim, input.ModeNormal)
	assert.Empty(t, bw.vimModePaneID)
}

func TestVimMode_Clear_NilBwDoesNotCrash(t *testing.T) {
	(&App{}).clearVimModeOwnership(context.Background(), nil)
}

func TestVimMode_Clear_NoPaneID(t *testing.T) {
	app := &App{}
	bw := &browserWindow{id: "test-win"}
	app.clearVimModeOwnership(context.Background(), bw)
	assert.Empty(t, bw.vimModePaneID)
}

func TestVimMode_Clear_StalePaneWithoutWorkspace(t *testing.T) {
	app := &App{}
	bw := &browserWindow{id: "test-win", vimModePaneID: entity.PaneID("stale-pane")}
	app.clearVimModeOwnership(context.Background(), bw)
	assert.Empty(t, bw.vimModePaneID)
}

func TestVimMode_HandleModeChange_NilBwDoesNotCrash(t *testing.T) {
	(&App{}).handleModeChange(context.Background(), nil, input.ModeNormal, input.ModeVim)
}

func TestVimMode_EditableFocusOnActivePaneExitsVimMode(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindVimModeKeyboardHandler(t, f.app, f.bw1)

	expectVimModeAccentHidden(f.overlay1A)
	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-a"), true)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.vimModePaneID)
	assert.False(t, f.pv1A.IsVimMode())
}

func TestVimMode_EditableFocusOnInactivePaneDoesNotExitVimMode(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindVimModeKeyboardHandler(t, f.app, f.bw1)

	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-b"), true)

	assert.Equal(t, input.ModeVim, kh.Mode())
	assert.Equal(t, entity.PaneID("pane-a"), f.bw1.vimModePaneID)
	assert.True(t, f.pv1A.IsVimMode())
}

func TestVimMode_PaneSwitchToEditablePaneExitsVimMode(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindVimModeKeyboardHandler(t, f.app, f.bw1)
	f.app.pageEditableFocusByPane = map[entity.PaneID]bool{
		"pane-b": true,
	}

	expectVimModeAccentHidden(f.overlay1A)
	f.app.transferVimModeOwnershipToPane(context.Background(), f.bw1, entity.PaneID("pane-b"))

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.vimModePaneID)
	assert.False(t, f.pv1A.IsVimMode())
	assert.False(t, f.pv1B.IsVimMode())
}

func TestVimMode_BackgroundWindowEditableFocusDoesNotExitFocusedWindowVimMode(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	pv1, overlay1 := createTestPaneView(t, factory, "pane-a")
	pv2, _ := createTestPaneView(t, factory, "pane-x")

	ws1 := &entity.Workspace{ID: "ws-1", ActivePaneID: "pane-a", Root: &entity.PaneNode{ID: "ws-1-pane-a", Pane: entity.NewPane("pane-a")}}
	ws2 := &entity.Workspace{ID: "ws-2", ActivePaneID: "pane-x", Root: &entity.PaneNode{ID: "ws-2-pane-x", Pane: entity.NewPane("pane-x")}}
	bw1 := &browserWindow{id: "win-1", tabs: entity.NewTabList()}
	bw2 := &browserWindow{id: "win-2", tabs: entity.NewTabList()}
	tab1 := &entity.Tab{ID: "tab-1", Workspace: ws1}
	tab2 := &entity.Tab{ID: "tab-2", Workspace: ws2}
	bw1.tabs.Add(tab1)
	bw1.tabs.SetActive(tab1.ID)
	bw2.tabs.Add(tab2)
	bw2.tabs.SetActive(tab2.ID)
	view1 := setupWorkspaceViewMocks(t, factory)
	view1.SetPaneViewsForTest(map[entity.PaneID]*component.PaneView{"pane-a": pv1})
	view2 := setupWorkspaceViewMocks(t, factory)
	view2.SetPaneViewsForTest(map[entity.PaneID]*component.PaneView{"pane-x": pv2})

	app := &App{
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{"tab-1": view1, "tab-2": view2},
		browserWindows:      map[string]*browserWindow{"win-1": bw1, "win-2": bw2},
		lastFocusedWindowID: "win-1",
	}

	enterVimMode(t, app, overlay1, bw1)
	kh := bindVimModeKeyboardHandler(t, app, bw1)

	app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-x"), true)

	assert.Equal(t, input.ModeVim, kh.Mode())
	assert.Equal(t, entity.PaneID("pane-a"), bw1.vimModePaneID)
	assert.True(t, pv1.IsVimMode())
}

func TestVimMode_OmniboxFocusExitsVimMode(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindVimModeKeyboardHandler(t, f.app, f.bw1)

	expectVimModeAccentHidden(f.overlay1A)
	f.app.handleVimModeFocusTrigger(context.Background(), f.bw1, usecase.VimModePolicyTriggerOmniboxFocus)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.vimModePaneID)
}

func TestVimMode_FindBarFocusExitsVimMode(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindVimModeKeyboardHandler(t, f.app, f.bw1)

	expectVimModeAccentHidden(f.overlay1A)
	f.app.handleVimModeFocusTrigger(context.Background(), f.bw1, usecase.VimModePolicyTriggerFindBarFocus)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.vimModePaneID)
}

func TestVimMode_TabSwitchExitsAndClearsOldAccent(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindVimModeKeyboardHandler(t, f.app, f.bw1)

	tab2 := &entity.Tab{ID: "tab-2b", Workspace: &entity.Workspace{
		ID:           "ws-2b",
		ActivePaneID: "pane-c",
		Root:         &entity.PaneNode{ID: "ws-2b-pane-c", Pane: entity.NewPane("pane-c")},
	}}
	f.bw1.tabs.Add(tab2)
	f.bw1.tabs.SetActive(tab2.ID)
	f.app.workspaceViews[tab2.ID] = setupWorkspaceViewMocks(t, f.factory)

	expectVimModeAccentHidden(f.overlay1A)
	f.app.handleVimModeTabSwitch(context.Background(), f.bw1)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.vimModePaneID)
	assert.False(t, f.pv1A.IsVimMode())
}

func TestVimMode_ActivationBypassWhenActivePageIsEditable(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	f.app.pageEditableFocusByPane = map[entity.PaneID]bool{
		"pane-a": true,
	}

	assert.True(t, f.app.shouldBypassVimModeActivation(f.bw1))
}

func TestVimMode_ActivationBypassClearsWhenEditableFocusLeaves(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)

	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-a"), true)
	assert.True(t, f.app.shouldBypassVimModeActivation(f.bw1))

	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-a"), false)
	assert.False(t, f.app.shouldBypassVimModeActivation(f.bw1))
}

func TestVimMode_ClearEditableFocusStateRemovesStoredBypass(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	f.app.pageEditableFocusByPane = map[entity.PaneID]bool{"pane-a": true}

	f.app.clearPageEditableFocusState(entity.PaneID("pane-a"))

	assert.False(t, f.app.shouldBypassVimModeActivation(f.bw1))
}

// ============================================================================
// CSS class constants
// ============================================================================

func TestVimMode_CSSClassConstants(t *testing.T) {
	assert.Equal(t, "vim-mode-active", component.VimModeActiveClass)
	assert.Equal(t, "vim-mode-pulse", component.VimModePulseClass)
	assert.Equal(t, "vim-mode-pulse-fast", component.VimModeFastPulseClass)
}

// ============================================================================
// Mode toaster
// ============================================================================

func newModeToasterForVimModeTest(t *testing.T, factory *mocks.MockWidgetFactory) (*component.Toaster, *mocks.MockBoxWidget, *mocks.MockLabelWidget) {
	t.Helper()

	container := mocks.NewMockBoxWidget(t)
	label := mocks.NewMockLabelWidget(t)
	factory.EXPECT().NewBox(layout.OrientationHorizontal, 0).Return(container).Once()
	container.EXPECT().AddCssClass("toast").Once()
	container.EXPECT().AddCssClass("toast-info").Once()
	container.EXPECT().SetHalign(mock.Anything).Once()
	container.EXPECT().SetValign(mock.Anything).Once()
	container.EXPECT().SetHexpand(false).Once()
	container.EXPECT().SetVexpand(false).Once()
	container.EXPECT().SetCanTarget(false).Once()
	container.EXPECT().SetCanFocus(false).Once()
	container.EXPECT().SetVisible(false).Once()
	factory.EXPECT().NewLabel("").Return(label).Once()
	label.EXPECT().SetCanTarget(false).Once()
	label.EXPECT().SetCanFocus(false).Once()
	container.EXPECT().Append(label).Once()

	return component.NewToaster(factory), container, label
}

func expectVimModeToastShown(container *mocks.MockBoxWidget, label *mocks.MockLabelWidget) {
	container.EXPECT().RemoveCssClass("toast-info").Once()
	container.EXPECT().AddCssClass("toast-vim-mode").Once()
	container.EXPECT().SetHalign(mock.Anything).Once()
	container.EXPECT().SetValign(mock.Anything).Once()
	label.EXPECT().SetText("VIM MODE").Once()
	container.EXPECT().SetVisible(true).Once()
}

func TestVimMode_ToasterRemainsVisibleUntilModeExit(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForVimModeTest(t, factory)
	expectVimModeToastShown(container, label)
	// These optional calls are made only by the old brief auto-dismiss path.
	container.EXPECT().RemoveCssClass("toast-vim-mode").Maybe()
	container.EXPECT().SetVisible(false).Maybe()

	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	})}
	bw := &browserWindow{id: "win-1", modeToaster: toaster}
	app.browserWindows = map[string]*browserWindow{bw.id: bw}
	app.lastFocusedWindowID = bw.id

	app.updateModeIndicatorToaster(context.Background(), bw, input.ModeVim)
	require.True(t, toaster.IsVisible())

	deadline := time.Now().Add(time.Duration(component.ToastBriefDurationMs+100) * time.Millisecond)
	mainContext := glib.MainContextDefault()
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		for mainContext.Pending() {
			mainContext.Iteration(false)
		}
	}

	assert.True(t, toaster.IsVisible(), "Vim Mode toast must remain visible until mode exit")
}

func TestVimMode_ModeExitHidesToaster(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForVimModeTest(t, factory)
	expectVimModeToastShown(container, label)
	container.EXPECT().RemoveCssClass("toast-custom").Once()
	container.EXPECT().RemoveCssClass("toast-vim-mode").Once()
	container.EXPECT().SetVisible(false).Once()

	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	})}
	bw := &browserWindow{id: "win-1", modeToaster: toaster}

	app.updateModeIndicatorToaster(context.Background(), bw, input.ModeVim)
	require.True(t, toaster.IsVisible())
	app.updateModeIndicatorToaster(context.Background(), bw, input.ModeNormal)

	assert.False(t, toaster.IsVisible(), "leaving Vim Mode must hide its persistent toast")
}

func TestVimMode_DisabledModeIndicatorPreferenceSuppressesToaster(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, _, _ := newModeToasterForVimModeTest(t, factory)
	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{})}
	bw := &browserWindow{id: "win-1", modeToaster: toaster}

	app.updateModeIndicatorToaster(context.Background(), bw, input.ModeVim)

	assert.False(t, toaster.IsVisible())
}

func TestVimMode_MultiWindowToastersExitIndependently(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster1, container1, label1 := newModeToasterForVimModeTest(t, factory)
	toaster2, container2, label2 := newModeToasterForVimModeTest(t, factory)
	expectVimModeToastShown(container1, label1)
	expectVimModeToastShown(container2, label2)
	container1.EXPECT().RemoveCssClass("toast-custom").Once()
	container1.EXPECT().RemoveCssClass("toast-vim-mode").Once()
	container1.EXPECT().SetVisible(false).Once()

	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	})}
	bw1 := &browserWindow{id: "win-1", modeToaster: toaster1}
	bw2 := &browserWindow{id: "win-2", modeToaster: toaster2}

	app.updateModeIndicatorToaster(context.Background(), bw1, input.ModeVim)
	app.updateModeIndicatorToaster(context.Background(), bw2, input.ModeVim)
	app.updateModeIndicatorToaster(context.Background(), bw1, input.ModeNormal)

	assert.False(t, toaster1.IsVisible())
	assert.True(t, toaster2.IsVisible(), "exiting one window must not hide another window's Vim Mode toast")
}

func TestVimMode_RuntimePreferenceDisableHidesPersistentToaster(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForVimModeTest(t, factory)
	expectVimModeToastShown(container, label)
	container.EXPECT().RemoveCssClass("toast-custom").Once()
	container.EXPECT().RemoveCssClass("toast-vim-mode").Once()
	container.EXPECT().SetVisible(false).Once()

	enabled := entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	}
	keyboardHandler := newKeyboardHandlerInVimMode(t)
	bw := &browserWindow{id: "win-1", modeToaster: toaster, keyboardHandler: keyboardHandler}
	app := &App{
		runtimeConfig:  runtimeConfigStateFromSnapshotForTest(enabled),
		browserWindows: map[string]*browserWindow{bw.id: bw},
	}

	app.updateModeIndicatorToaster(context.Background(), bw, input.ModeVim)
	require.True(t, toaster.IsVisible())
	app.applyRuntimeConfigChange(context.Background(), entity.RuntimeConfigSnapshot{})

	assert.False(t, toaster.IsVisible(), "runtime preference disable must reconcile an existing persistent toast")
}

func TestVimMode_RuntimePreferenceEnableShowsPersistentToaster(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForVimModeTest(t, factory)
	expectVimModeToastShown(container, label)

	keyboardHandler := newKeyboardHandlerInVimMode(t)
	bw := &browserWindow{id: "win-1", modeToaster: toaster, keyboardHandler: keyboardHandler}
	app := &App{
		runtimeConfig:  runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{}),
		browserWindows: map[string]*browserWindow{bw.id: bw},
	}
	enabled := entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	}

	app.applyRuntimeConfigChange(context.Background(), enabled)

	assert.True(t, toaster.IsVisible(), "runtime preference enable must reconcile active Vim Mode")
}

// ============================================================================
// BorderManager — Vim mode must NOT use the global border overlay
// ============================================================================

func TestVimMode_GlobalBorderOverlayStaysOff(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetCanFocus(false).Once()
	mockBox.EXPECT().SetCanTarget(false).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()
	mockBox.EXPECT().SetVisible(false).Once()

	bm := focus.NewBorderManager(mockFactory)

	mockBox.EXPECT().SetVisible(false).Once()
	bm.OnModeChange(context.Background(), input.ModeNormal, input.ModeVim)

	mockBox.EXPECT().SetVisible(false).Once()
	bm.OnModeChange(context.Background(), input.ModeVim, input.ModeNormal)
}

// ============================================================================
// Pulse Debounce
// ============================================================================

func TestVimMode_Pulse_DebounceSkipsRapidConsecutiveCalls(t *testing.T) {
	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)

	// Set up expectations for exactly ONE normal pulse.
	setUpNormalPulse(f.overlay1A)

	// First call fires the pulse (debounce timer is zero).
	f.app.triggerVimModePulse(context.Background(), false)

	// Second call within the same frame should be debounced — no
	// mock expectations set up for it, so the test fails if it fires.
	f.app.triggerVimModePulse(context.Background(), false)
}

func TestVimMode_Pulse_DebounceAllowsSpacedCalls(t *testing.T) {
	// Verify that the debounce timer starts at zero on a fresh App,
	// so the first pulse after entering vim mode always fires.
	// The exit-and-re-enter path is covered by the
	// TestVimMode_Pulse_DebounceResetOnClearOwnership test below.

	f := newSingleWindowVimModeFixture(t)

	// Fresh app: debounce timer is zero.
	assert.True(t, f.app.vimModePulseLastTime.IsZero(),
		"fresh app should have zero pulse timer")

	enterVimMode(t, f.app, f.overlay1A, f.bw1)

	// Pulse #1 fires because timer is zero (time.Since(zero) is huge).
	setUpNormalPulse(f.overlay1A)
	f.app.triggerVimModePulse(context.Background(), false)

	// Timer is now set, proving the first pulse registered.
	assert.False(t, f.app.vimModePulseLastTime.IsZero(),
		"pulse timer should be non-zero after first pulse")
}

func TestVimMode_Pulse_DebounceResetOnClearOwnership(t *testing.T) {
	// Verify that clearVimModeOwnership resets the debounce timer
	// so a subsequent pulse in a new vim mode session is not skipped.
	// We check the timer state directly rather than triggering a second
	// pulse (which would need pulse-cycle alternation handling).

	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)

	// Pulse #1 fires and sets the timer.
	setUpNormalPulse(f.overlay1A)
	f.app.triggerVimModePulse(context.Background(), false)
	assert.False(t, f.app.vimModePulseLastTime.IsZero(),
		"pulse timer should be non-zero after first pulse")

	// Clear ownership — this resets the debounce timer.
	expectVimModeAccentHidden(f.overlay1A)
	f.app.clearVimModeOwnership(context.Background(), f.bw1)
	assert.Empty(t, f.bw1.vimModePaneID)

	// Debounce timer should now be back to zero.
	assert.True(t, f.app.vimModePulseLastTime.IsZero(),
		"pulse timer should be reset after clearVimModeOwnership")
}

func TestVimMode_Pulse_DebounceFirstPulseAlwaysFires(t *testing.T) {
	// Even if triggerVimModePulse is called rapidly on a fresh
	// App (e.g., first keystroke after vim mode entry), the first
	// call must always fire. This is guaranteed by the zero value
	// of time.Time producing a time.Since result much larger than
	// the debounce interval.
	f := newSingleWindowVimModeFixture(t)
	enterVimMode(t, f.app, f.overlay1A, f.bw1)

	// Rapid double pulse — only first should fire regardless of order.
	// Set up just one set of expectations.
	setUpNormalPulse(f.overlay1A)
	f.app.triggerVimModePulse(context.Background(), false)
	f.app.triggerVimModePulse(context.Background(), false)
}

// ============================================================================
// UI Scale: Vim mode CSS uses relative (em) units
// ============================================================================

func TestVimMode_CSSUsesEms(t *testing.T) {
	css := theme.GenerateCSS(theme.DefaultDarkPalette())

	assert.Contains(t, css, "box-shadow: inset 0 0 0 0.125em",
		"vim mode overlay box-shadow should use em for scaling")
	assert.Contains(t, css, "0.2em",
		"vim mode overlay pulse glow radius uses em")
	assert.Contains(t, css, "0.26em",
		"vim mode overlay fast-pulse glow radius uses em")
	assert.NotContains(t, css, "vim-mode-indicator",
		"removed PAGE label styling must not remain")
}

func TestVimMode_CSSPulseTimingUsesDefaultTransitionDuration(t *testing.T) {
	// The pulse animation durations should be derived from the
	// default transition duration (normal = 3x, fast = 6x).
	// Default = 120ms → normal = 360ms, fast = 720ms.
	css := theme.GenerateCSS(theme.DefaultDarkPalette())

	assert.Contains(t, css, "360ms",
		"normal pulse duration should be 3x transition duration (360ms for 120ms base)")
	assert.Contains(t, css, "720ms",
		"fast pulse duration should be 6x transition duration (720ms for 120ms base)")
}

func TestVimMode_CSSWithCustomTransitionDuration(t *testing.T) {
	// Custom transition duration (e.g., 200ms) should produce
	// adjusted pulse timings: normal = 600ms, fast = 1200ms.
	css := theme.GenerateCSSFullWithTiming(
		theme.DefaultDarkPalette(),
		1.0,
		theme.DefaultFontConfig(),
		theme.DefaultModeColors(),
		200,
	)

	assert.Contains(t, css, "600ms",
		"normal pulse should be 3x custom transition duration (200ms -> 600ms)")
	assert.Contains(t, css, "1200ms",
		"fast pulse should be 6x custom transition duration (200ms -> 1200ms)")
}

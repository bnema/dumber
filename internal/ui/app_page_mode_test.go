package ui

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/puregotk/v4/glib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/bnema/dumber/internal/ui/theme"
)

// ---------------------------------------------------------------------------
// Mock-based PaneView and WorkspaceView creation helpers
//
// These are used by pageModeTestFixture to build a minimal test environment
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
// its overlay mock so subsequent page-mode indicator / pulse expectations can
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

// enterPageMode marks the owning pane with the pane-local visual accent.
func enterPageMode(
	t *testing.T,
	app *App,
	overlay *mocks.MockOverlayWidget,
	bw *browserWindow,
) {
	t.Helper()
	overlay.EXPECT().AddCssClass("page-mode-active").Once()
	app.handlePageModeOwnership(context.Background(), bw, input.ModePage, input.ModeNormal)
}

func expectPageModeAccentHidden(overlay *mocks.MockOverlayWidget) {
	overlay.EXPECT().RemoveCssClass("page-mode-active").Once()
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

// newKeyboardHandlerInPageMode creates a KeyboardHandler whose Mode() returns
// ModePage — no GTK widgets required.
func newKeyboardHandlerInPageMode(t *testing.T) *input.KeyboardHandler {
	t.Helper()
	kh := input.NewKeyboardHandler(
		context.Background(),
		&entity.WorkspaceConfig{},
		&entity.SessionConfig{},
	)
	kh.EnterPageMode()
	return kh
}

func bindPageModeKeyboardHandler(t *testing.T, app *App, bw *browserWindow) *input.KeyboardHandler {
	t.Helper()
	kh := newKeyboardHandlerInPageMode(t)
	kh.SetOnModeChange(func(from, to input.Mode) {
		app.handleModeChange(context.Background(), bw, from, to)
	})
	bw.keyboardHandler = kh
	return kh
}

// ---------------------------------------------------------------------------
// pageModeTestFixture — complete, minimal App + workspace environment
//
// Provides two browser windows, each with a two-pane workspace.  The active
// pane on both workspaces starts at "pane-a".
// ---------------------------------------------------------------------------

type pageModeTestFixture struct {
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

func newPageModeTestFixture(t *testing.T) *pageModeTestFixture {
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

	return &pageModeTestFixture{
		app: app, bw1: bw1, bw2: bw2,
		pv1A: pv1A, pv1B: pv1B, pv2A: pv2A, pv2B: pv2B,
		factory:   factory,
		overlay1A: ol1A, overlay1B: ol1B,
		overlay2A: ol2A, overlay2B: ol2B,
	}
}

func newSingleWindowPageModeFixture(t *testing.T) *pageModeTestFixture {
	t.Helper()
	f := newPageModeTestFixture(t)
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
// 1. Entering Page mode marks only the active pane
// ============================================================================

func TestPageMode_Enter_SetsOwnershipOnActivePane(t *testing.T) {
	f := newPageModeTestFixture(t)

	enterPageMode(t, f.app, f.overlay1A, f.bw1)

	assert.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID,
		"entering page mode sets ownership on the active pane")
	assert.True(t, f.pv1A.IsPageMode(),
		"the active pane view enters page mode")
	assert.False(t, f.pv1B.IsPageMode(),
		"the inactive pane is NOT in page mode")
	assert.Empty(t, f.bw2.pageModePaneID,
		"second window is untouched")
	assert.False(t, f.pv2A.IsPageMode(),
		"second window active pane is NOT in page mode")
}

func TestPageMode_Enter_MarksCorrectWindow(t *testing.T) {
	f := newPageModeTestFixture(t)

	enterPageMode(t, f.app, f.overlay2A, f.bw2)

	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.pageModePaneID)
	assert.True(t, f.pv2A.IsPageMode())
	assert.Empty(t, f.bw1.pageModePaneID)
	assert.False(t, f.pv1A.IsPageMode())
}

// ============================================================================
// 2. Leaving Page mode clears ownership
// ============================================================================

func TestPageMode_Leave_ClearsOwnershipFromOwningPane(t *testing.T) {
	f := newPageModeTestFixture(t)

	// Enter first
	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)

	// Leave — SetPageMode(false) hides indicator and removes the CSS class
	expectPageModeAccentHidden(f.overlay1A)
	f.app.handlePageModeOwnership(context.Background(), f.bw1, input.ModeNormal, input.ModePage)

	assert.Empty(t, f.bw1.pageModePaneID,
		"leaving page mode clears ownership field")
	assert.False(t, f.pv1A.IsPageMode(),
		"the previously owning pane exits page mode")
}

// ============================================================================
// 3. Transfer within the same window
// ============================================================================

func TestPageMode_Transfer_MovesOwnershipToNewPane(t *testing.T) {
	f := newPageModeTestFixture(t)

	// Enter page mode on bw1 pane-a — save label for hide expectation
	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)

	// Give bw1 a keyboard handler in page mode so transfer performs the switch
	f.bw1.keyboardHandler = newKeyboardHandlerInPageMode(t)

	// transferPageModeOwnershipToPane calls SetPageMode(false) on the old
	// pane before SetPageMode(true) on the new pane.  Set up both.
	expectPageModeAccentHidden(f.overlay1A)

	// Set up the pane-local accent for pane-b.
	f.overlay1B.EXPECT().AddCssClass("page-mode-active").Once()

	// Transfer ownership to pane-b
	f.app.transferPageModeOwnershipToPane(context.Background(), f.bw1, entity.PaneID("pane-b"))

	assert.Equal(t, entity.PaneID("pane-b"), f.bw1.pageModePaneID,
		"ownership transferred to new pane")
	assert.False(t, f.pv1A.IsPageMode(),
		"previous owning pane deactivated")
	assert.True(t, f.pv1B.IsPageMode(),
		"new pane is now in page mode")
}

func TestPageMode_Transfer_NoPageModeClearsStale(t *testing.T) {
	f := newPageModeTestFixture(t)

	f.bw1.pageModePaneID = entity.PaneID("pane-a")
	f.bw1.keyboardHandler = input.NewKeyboardHandler(
		context.Background(),
		&entity.WorkspaceConfig{},
		&entity.SessionConfig{},
	)
	// No EnterPageMode — handler stays in ModeNormal.

	f.app.transferPageModeOwnershipToPane(context.Background(), f.bw1, entity.PaneID("pane-b"))

	assert.Empty(t, f.bw1.pageModePaneID,
		"stale ownership cleared when window is not in page mode")
	assert.False(t, f.pv1A.IsPageMode())
	assert.False(t, f.pv1B.IsPageMode())
}

func TestPageMode_Transfer_NilBwDoesNotCrash(t *testing.T) {
	app := &App{}
	app.transferPageModeOwnershipToPane(context.Background(), nil, entity.PaneID("pane-1"))
}

func TestPageMode_Transfer_SamePaneIsNoop(t *testing.T) {
	app := &App{}
	paneID := entity.PaneID("pane-a")
	bw := &browserWindow{pageModePaneID: paneID}
	app.transferPageModeOwnershipToPane(context.Background(), bw, paneID)
	assert.Equal(t, paneID, bw.pageModePaneID)
}

func TestPageMode_Transfer_EmptyOwnershipIsNoop(t *testing.T) {
	app := &App{}
	bw := &browserWindow{}
	app.transferPageModeOwnershipToPane(context.Background(), bw, entity.PaneID("pane-a"))
	assert.Empty(t, bw.pageModePaneID)
}

// ============================================================================
// 4. Pulse targeting — normal vs fast
// ============================================================================

func setUpNormalPulse(overlay *mocks.MockOverlayWidget) {
	overlay.EXPECT().RemoveCssClass("page-mode-pulse").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-fast").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-cycle-a").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-cycle-b").Once()
	overlay.EXPECT().AddCssClass("page-mode-pulse").Once()
	overlay.EXPECT().AddCssClass("page-mode-pulse-cycle-a").Once()
}

func setUpFastPulse(overlay *mocks.MockOverlayWidget) {
	overlay.EXPECT().RemoveCssClass("page-mode-pulse").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-fast").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-cycle-a").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-cycle-b").Once()
	overlay.EXPECT().AddCssClass("page-mode-pulse-fast").Once()
	overlay.EXPECT().AddCssClass("page-mode-pulse-cycle-a").Once()
}

func TestPageMode_Pulse_NormalTriggersOnOwningPane(t *testing.T) {
	f := newPageModeTestFixture(t)

	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)

	// Pulse needs the indicator to exist — it was created lazily during
	// enterPageMode, but we also need to set up pulse expectations.
	// (enterPageMode created it, so we just set up pulse mocks.)
	setUpNormalPulse(f.overlay1A)

	f.app.triggerPageModePulse(context.Background(), false)
}

func TestPageMode_Pulse_FastTriggersOnOwningPane(t *testing.T) {
	f := newPageModeTestFixture(t)

	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)

	setUpFastPulse(f.overlay1A)

	f.app.triggerPageModePulse(context.Background(), true)
}

func TestPageMode_Pulse_NoOwnerIsNoop(t *testing.T) {
	f := newPageModeTestFixture(t)
	f.app.triggerPageModePulse(context.Background(), false)
	f.app.triggerPageModePulse(context.Background(), true)
	// No pane has ownership — no pulse expectations needed.
}

// ============================================================================
// 5. Multi-window isolation
// ============================================================================

func TestPageMode_MultiWindow_TwoWindowsEnterLeave(t *testing.T) {
	f := newPageModeTestFixture(t)

	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	enterPageMode(t, f.app, f.overlay2A, f.bw2)

	assert.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)
	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.pageModePaneID)

	// Leave bw1 only
	expectPageModeAccentHidden(f.overlay1A)
	f.app.handlePageModeOwnership(context.Background(), f.bw1, input.ModeNormal, input.ModePage)

	assert.Empty(t, f.bw1.pageModePaneID)
	assert.False(t, f.pv1A.IsPageMode())

	// bw2 unchanged
	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.pageModePaneID)
	assert.True(t, f.pv2A.IsPageMode())
}

func TestPageMode_MultiWindow_TransferIsolated(t *testing.T) {
	f := newPageModeTestFixture(t)

	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	enterPageMode(t, f.app, f.overlay2A, f.bw2)

	f.bw1.keyboardHandler = newKeyboardHandlerInPageMode(t)

	// transferPageModeOwnershipToPane calls SetPageMode(false) on old pane first
	expectPageModeAccentHidden(f.overlay1A)

	// Transfer bw1 to pane-b.
	f.overlay1B.EXPECT().AddCssClass("page-mode-active").Once()
	f.app.transferPageModeOwnershipToPane(context.Background(), f.bw1, entity.PaneID("pane-b"))

	assert.Equal(t, entity.PaneID("pane-b"), f.bw1.pageModePaneID)
	assert.True(t, f.pv1B.IsPageMode())
	assert.False(t, f.pv1A.IsPageMode())

	// bw2 unchanged
	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.pageModePaneID)
	assert.True(t, f.pv2A.IsPageMode())
}

func TestPageMode_MultiWindow_PulseIsolated(t *testing.T) {
	f := newPageModeTestFixture(t)

	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	enterPageMode(t, f.app, f.overlay2A, f.bw2)

	// Pulse on bw1 (lastFocusedWindowID = "win-1")
	setUpNormalPulse(f.overlay1A)
	f.app.triggerPageModePulse(context.Background(), false)

	// bw2 pane still in page mode
	assert.True(t, f.pv2A.IsPageMode())
}

// ============================================================================
// Edge cases — nil / no-workspace / startup guards
// ============================================================================

func TestPageMode_Enter_NoWorkspaceIsNoop(t *testing.T) {
	app := &App{}
	bw := &browserWindow{id: "win-1"}
	app.handlePageModeOwnership(context.Background(), bw, input.ModePage, input.ModeNormal)
	assert.Empty(t, bw.pageModePaneID)
}

func TestPageMode_Clear_NilBwDoesNotCrash(t *testing.T) {
	(&App{}).clearPageModeOwnership(context.Background(), nil)
}

func TestPageMode_Clear_NoPaneID(t *testing.T) {
	app := &App{}
	bw := &browserWindow{id: "test-win"}
	app.clearPageModeOwnership(context.Background(), bw)
	assert.Empty(t, bw.pageModePaneID)
}

func TestPageMode_Clear_StalePaneWithoutWorkspace(t *testing.T) {
	app := &App{}
	bw := &browserWindow{id: "test-win", pageModePaneID: entity.PaneID("stale-pane")}
	app.clearPageModeOwnership(context.Background(), bw)
	assert.Empty(t, bw.pageModePaneID)
}

func TestPageMode_HandleModeChange_NilBwDoesNotCrash(t *testing.T) {
	(&App{}).handleModeChange(context.Background(), nil, input.ModeNormal, input.ModePage)
}

func TestPageMode_EditableFocusOnActivePaneExitsPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	expectPageModeAccentHidden(f.overlay1A)
	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-a"), true)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.pageModePaneID)
	assert.False(t, f.pv1A.IsPageMode())
}

func TestPageMode_EditableFocusOnInactivePaneDoesNotExitPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-b"), true)

	assert.Equal(t, input.ModePage, kh.Mode())
	assert.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)
	assert.True(t, f.pv1A.IsPageMode())
}

func TestPageMode_PaneSwitchToEditablePaneExitsPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)
	f.app.pageEditableFocusByPane = map[entity.PaneID]bool{
		"pane-b": true,
	}

	expectPageModeAccentHidden(f.overlay1A)
	f.app.transferPageModeOwnershipToPane(context.Background(), f.bw1, entity.PaneID("pane-b"))

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.pageModePaneID)
	assert.False(t, f.pv1A.IsPageMode())
	assert.False(t, f.pv1B.IsPageMode())
}

func TestPageMode_BackgroundWindowEditableFocusDoesNotExitFocusedWindowPageMode(t *testing.T) {
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

	enterPageMode(t, app, overlay1, bw1)
	kh := bindPageModeKeyboardHandler(t, app, bw1)

	app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-x"), true)

	assert.Equal(t, input.ModePage, kh.Mode())
	assert.Equal(t, entity.PaneID("pane-a"), bw1.pageModePaneID)
	assert.True(t, pv1.IsPageMode())
}

func TestPageMode_OmniboxFocusExitsPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	expectPageModeAccentHidden(f.overlay1A)
	f.app.handlePageModeFocusTrigger(context.Background(), f.bw1, usecase.PageModePolicyTriggerOmniboxFocus)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.pageModePaneID)
}

func TestPageMode_FindBarFocusExitsPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	expectPageModeAccentHidden(f.overlay1A)
	f.app.handlePageModeFocusTrigger(context.Background(), f.bw1, usecase.PageModePolicyTriggerFindBarFocus)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.pageModePaneID)
}

func TestPageMode_TabSwitchExitsAndClearsOldAccent(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	tab2 := &entity.Tab{ID: "tab-2b", Workspace: &entity.Workspace{
		ID:           "ws-2b",
		ActivePaneID: "pane-c",
		Root:         &entity.PaneNode{ID: "ws-2b-pane-c", Pane: entity.NewPane("pane-c")},
	}}
	f.bw1.tabs.Add(tab2)
	f.bw1.tabs.SetActive(tab2.ID)
	f.app.workspaceViews[tab2.ID] = setupWorkspaceViewMocks(t, f.factory)

	expectPageModeAccentHidden(f.overlay1A)
	f.app.handlePageModeTabSwitch(context.Background(), f.bw1)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.pageModePaneID)
	assert.False(t, f.pv1A.IsPageMode())
}

func TestPageMode_ActivationBypassWhenActivePageIsEditable(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	f.app.pageEditableFocusByPane = map[entity.PaneID]bool{
		"pane-a": true,
	}

	assert.True(t, f.app.shouldBypassPageModeActivation(f.bw1))
}

func TestPageMode_ActivationBypassClearsWhenEditableFocusLeaves(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)

	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-a"), true)
	assert.True(t, f.app.shouldBypassPageModeActivation(f.bw1))

	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-a"), false)
	assert.False(t, f.app.shouldBypassPageModeActivation(f.bw1))
}

func TestPageMode_ClearEditableFocusStateRemovesStoredBypass(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	f.app.pageEditableFocusByPane = map[entity.PaneID]bool{"pane-a": true}

	f.app.clearPageEditableFocusState(entity.PaneID("pane-a"))

	assert.False(t, f.app.shouldBypassPageModeActivation(f.bw1))
}

// ============================================================================
// CSS class constants
// ============================================================================

func TestPageMode_CSSClassConstants(t *testing.T) {
	assert.Equal(t, "page-mode-active", component.PageModeActiveClass)
	assert.Equal(t, "page-mode-pulse", component.PageModePulseClass)
	assert.Equal(t, "page-mode-pulse-fast", component.PageModeFastPulseClass)
}

// ============================================================================
// Mode toaster
// ============================================================================

func newModeToasterForPageModeTest(t *testing.T, factory *mocks.MockWidgetFactory) (*component.Toaster, *mocks.MockBoxWidget, *mocks.MockLabelWidget) {
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

func expectPageModeToastShown(container *mocks.MockBoxWidget, label *mocks.MockLabelWidget) {
	container.EXPECT().RemoveCssClass("toast-info").Once()
	container.EXPECT().AddCssClass("toast-pane-mode").Once()
	container.EXPECT().SetHalign(mock.Anything).Once()
	container.EXPECT().SetValign(mock.Anything).Once()
	label.EXPECT().SetText("PAGE MODE").Once()
	container.EXPECT().SetVisible(true).Once()
}

func TestPageMode_ToasterRemainsVisibleUntilModeExit(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForPageModeTest(t, factory)
	expectPageModeToastShown(container, label)
	// These optional calls are made only by the old brief auto-dismiss path.
	container.EXPECT().RemoveCssClass("toast-pane-mode").Maybe()
	container.EXPECT().SetVisible(false).Maybe()

	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	})}
	bw := &browserWindow{id: "win-1", modeToaster: toaster}
	app.browserWindows = map[string]*browserWindow{bw.id: bw}
	app.lastFocusedWindowID = bw.id

	app.updateModeIndicatorToaster(context.Background(), bw, input.ModePage)
	require.True(t, toaster.IsVisible())

	deadline := time.Now().Add(time.Duration(component.ToastBriefDurationMs+100) * time.Millisecond)
	mainContext := glib.MainContextDefault()
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		for mainContext.Pending() {
			mainContext.Iteration(false)
		}
	}

	assert.True(t, toaster.IsVisible(), "Page Mode toast must remain visible until mode exit")
}

func TestPageMode_ModeExitHidesToaster(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForPageModeTest(t, factory)
	expectPageModeToastShown(container, label)
	container.EXPECT().RemoveCssClass("toast-custom").Once()
	container.EXPECT().RemoveCssClass("toast-pane-mode").Once()
	container.EXPECT().SetVisible(false).Once()

	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	})}
	bw := &browserWindow{id: "win-1", modeToaster: toaster}

	app.updateModeIndicatorToaster(context.Background(), bw, input.ModePage)
	require.True(t, toaster.IsVisible())
	app.updateModeIndicatorToaster(context.Background(), bw, input.ModeNormal)

	assert.False(t, toaster.IsVisible(), "leaving Page Mode must hide its persistent toast")
}

func TestPageMode_DisabledModeIndicatorPreferenceSuppressesToaster(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, _, _ := newModeToasterForPageModeTest(t, factory)
	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{})}
	bw := &browserWindow{id: "win-1", modeToaster: toaster}

	app.updateModeIndicatorToaster(context.Background(), bw, input.ModePage)

	assert.False(t, toaster.IsVisible())
}

func TestPageMode_MultiWindowToastersExitIndependently(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster1, container1, label1 := newModeToasterForPageModeTest(t, factory)
	toaster2, container2, label2 := newModeToasterForPageModeTest(t, factory)
	expectPageModeToastShown(container1, label1)
	expectPageModeToastShown(container2, label2)
	container1.EXPECT().RemoveCssClass("toast-custom").Once()
	container1.EXPECT().RemoveCssClass("toast-pane-mode").Once()
	container1.EXPECT().SetVisible(false).Once()

	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	})}
	bw1 := &browserWindow{id: "win-1", modeToaster: toaster1}
	bw2 := &browserWindow{id: "win-2", modeToaster: toaster2}

	app.updateModeIndicatorToaster(context.Background(), bw1, input.ModePage)
	app.updateModeIndicatorToaster(context.Background(), bw2, input.ModePage)
	app.updateModeIndicatorToaster(context.Background(), bw1, input.ModeNormal)

	assert.False(t, toaster1.IsVisible())
	assert.True(t, toaster2.IsVisible(), "exiting one window must not hide another window's Page Mode toast")
}

func TestPageMode_RuntimePreferenceDisableHidesPersistentToaster(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForPageModeTest(t, factory)
	expectPageModeToastShown(container, label)
	container.EXPECT().RemoveCssClass("toast-custom").Once()
	container.EXPECT().RemoveCssClass("toast-pane-mode").Once()
	container.EXPECT().SetVisible(false).Once()

	enabled := entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	}
	keyboardHandler := newKeyboardHandlerInPageMode(t)
	bw := &browserWindow{id: "win-1", modeToaster: toaster, keyboardHandler: keyboardHandler}
	app := &App{
		runtimeConfig:  runtimeConfigStateFromSnapshotForTest(enabled),
		browserWindows: map[string]*browserWindow{bw.id: bw},
	}

	app.updateModeIndicatorToaster(context.Background(), bw, input.ModePage)
	require.True(t, toaster.IsVisible())
	app.applyRuntimeConfigChange(context.Background(), entity.RuntimeConfigSnapshot{})

	assert.False(t, toaster.IsVisible(), "runtime preference disable must reconcile an existing persistent toast")
}

func TestPageMode_RuntimePreferenceEnableShowsPersistentToaster(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForPageModeTest(t, factory)
	expectPageModeToastShown(container, label)

	keyboardHandler := newKeyboardHandlerInPageMode(t)
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

	assert.True(t, toaster.IsVisible(), "runtime preference enable must reconcile active Page Mode")
}

// ============================================================================
// BorderManager — Page mode must NOT use the global border overlay
// ============================================================================

func TestPageMode_GlobalBorderOverlayStaysOff(t *testing.T) {
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
	bm.OnModeChange(context.Background(), input.ModeNormal, input.ModePage)

	mockBox.EXPECT().SetVisible(false).Once()
	bm.OnModeChange(context.Background(), input.ModePage, input.ModeNormal)
}

// ============================================================================
// Pulse Debounce
// ============================================================================

func TestPageMode_Pulse_DebounceSkipsRapidConsecutiveCalls(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)

	// Set up expectations for exactly ONE normal pulse.
	setUpNormalPulse(f.overlay1A)

	// First call fires the pulse (debounce timer is zero).
	f.app.triggerPageModePulse(context.Background(), false)

	// Second call within the same frame should be debounced — no
	// mock expectations set up for it, so the test fails if it fires.
	f.app.triggerPageModePulse(context.Background(), false)
}

func TestPageMode_Pulse_DebounceAllowsSpacedCalls(t *testing.T) {
	// Verify that the debounce timer starts at zero on a fresh App,
	// so the first pulse after entering page mode always fires.
	// The exit-and-re-enter path is covered by the
	// TestPageMode_Pulse_DebounceResetOnClearOwnership test below.

	f := newSingleWindowPageModeFixture(t)

	// Fresh app: debounce timer is zero.
	assert.True(t, f.app.pageModePulseLastTime.IsZero(),
		"fresh app should have zero pulse timer")

	enterPageMode(t, f.app, f.overlay1A, f.bw1)

	// Pulse #1 fires because timer is zero (time.Since(zero) is huge).
	setUpNormalPulse(f.overlay1A)
	f.app.triggerPageModePulse(context.Background(), false)

	// Timer is now set, proving the first pulse registered.
	assert.False(t, f.app.pageModePulseLastTime.IsZero(),
		"pulse timer should be non-zero after first pulse")
}

func TestPageMode_Pulse_DebounceResetOnClearOwnership(t *testing.T) {
	// Verify that clearPageModeOwnership resets the debounce timer
	// so a subsequent pulse in a new page mode session is not skipped.
	// We check the timer state directly rather than triggering a second
	// pulse (which would need pulse-cycle alternation handling).

	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)

	// Pulse #1 fires and sets the timer.
	setUpNormalPulse(f.overlay1A)
	f.app.triggerPageModePulse(context.Background(), false)
	assert.False(t, f.app.pageModePulseLastTime.IsZero(),
		"pulse timer should be non-zero after first pulse")

	// Clear ownership — this resets the debounce timer.
	expectPageModeAccentHidden(f.overlay1A)
	f.app.clearPageModeOwnership(context.Background(), f.bw1)
	assert.Empty(t, f.bw1.pageModePaneID)

	// Debounce timer should now be back to zero.
	assert.True(t, f.app.pageModePulseLastTime.IsZero(),
		"pulse timer should be reset after clearPageModeOwnership")
}

func TestPageMode_Pulse_DebounceFirstPulseAlwaysFires(t *testing.T) {
	// Even if triggerPageModePulse is called rapidly on a fresh
	// App (e.g., first keystroke after page mode entry), the first
	// call must always fire. This is guaranteed by the zero value
	// of time.Time producing a time.Since result much larger than
	// the debounce interval.
	f := newSingleWindowPageModeFixture(t)
	enterPageMode(t, f.app, f.overlay1A, f.bw1)

	// Rapid double pulse — only first should fire regardless of order.
	// Set up just one set of expectations.
	setUpNormalPulse(f.overlay1A)
	f.app.triggerPageModePulse(context.Background(), false)
	f.app.triggerPageModePulse(context.Background(), false)
}

// ============================================================================
// UI Scale: Page mode CSS uses relative (em) units
// ============================================================================

func TestPageMode_CSSUsesEms(t *testing.T) {
	css := theme.GenerateCSS(theme.DefaultDarkPalette())

	assert.Contains(t, css, "box-shadow: inset 0 0 0 0.125em",
		"page mode overlay box-shadow should use em for scaling")
	assert.Contains(t, css, "0.2em",
		"page mode overlay pulse glow radius uses em")
	assert.Contains(t, css, "0.26em",
		"page mode overlay fast-pulse glow radius uses em")
	assert.NotContains(t, css, "page-mode-indicator",
		"removed PAGE label styling must not remain")
}

func TestPageMode_CSSPulseTimingUsesDefaultTransitionDuration(t *testing.T) {
	// The pulse animation durations should be derived from the
	// default transition duration (normal = 3x, fast = 6x).
	// Default = 120ms → normal = 360ms, fast = 720ms.
	css := theme.GenerateCSS(theme.DefaultDarkPalette())

	assert.Contains(t, css, "360ms",
		"normal pulse duration should be 3x transition duration (360ms for 120ms base)")
	assert.Contains(t, css, "720ms",
		"fast pulse duration should be 6x transition duration (720ms for 120ms base)")
}

func TestPageMode_CSSWithCustomTransitionDuration(t *testing.T) {
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

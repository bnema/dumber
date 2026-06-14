package ui

import (
	"context"
	"testing"

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

// setupPageModeIndicatorMocks sets up mock expectations for the one-time lazy
// creation of the page mode indicator label widget and returns the label mock
// so callers can layer on expectations for SetPageMode / pulse.
func setupPageModeIndicatorMocks(
	t *testing.T,
	factory *mocks.MockWidgetFactory,
	overlay *mocks.MockOverlayWidget,
) *mocks.MockLabelWidget {
	t.Helper()

	label := mocks.NewMockLabelWidget(t)
	factory.EXPECT().NewLabel("PAGE").Return(label).Once()
	label.EXPECT().SetCanFocus(false).Once()
	label.EXPECT().SetCanTarget(false).Once()
	label.EXPECT().AddCssClass("page-mode-indicator").Once()
	label.EXPECT().SetVisible(false).Once() // indicator starts hidden
	label.EXPECT().SetHalign(mock.Anything).Once()
	label.EXPECT().SetValign(mock.Anything).Once()
	overlay.EXPECT().AddOverlay(label).Once()
	overlay.EXPECT().SetClipOverlay(label, false).Once()
	overlay.EXPECT().SetMeasureOverlay(label, false).Once()

	return label
}

// showIndicatorMocks sets up the mock expectations that happen when
// SetPageMode(true) is called — the indicator is shown and the CSS class
// is added.
func showIndicatorMocks(label *mocks.MockLabelWidget, overlay *mocks.MockOverlayWidget) {
	label.EXPECT().SetVisible(true).Once()
	overlay.EXPECT().AddCssClass("page-mode-active").Once()
}

// hideIndicatorMocks sets up the expectations for SetPageMode(false).
func hideIndicatorMocks(label *mocks.MockLabelWidget, overlay *mocks.MockOverlayWidget) {
	label.EXPECT().SetVisible(false).Once()
	overlay.EXPECT().RemoveCssClass("page-mode-active").Once()
}

// enterPageMode is a convenience helper that sets up all the mock expectations
// for entering page mode on the given browser window and pane overlay, then
// calls handlePageModeOwnership.
func enterPageMode(
	t *testing.T,
	app *App,
	factory *mocks.MockWidgetFactory,
	overlay *mocks.MockOverlayWidget,
	bw *browserWindow,
) *mocks.MockLabelWidget {
	t.Helper()
	label := setupPageModeIndicatorMocks(t, factory, overlay)
	showIndicatorMocks(label, overlay)
	app.handlePageModeOwnership(context.Background(), bw, input.ModePage, input.ModeNormal)
	return label
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

	enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)

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

	enterPageMode(t, f.app, f.factory, f.overlay2A, f.bw2)

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
	label := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)

	// Leave — SetPageMode(false) hides indicator and removes the CSS class
	hideIndicatorMocks(label, f.overlay1A)
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
	labelA := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)

	// Give bw1 a keyboard handler in page mode so transfer performs the switch
	f.bw1.keyboardHandler = newKeyboardHandlerInPageMode(t)

	// transferPageModeOwnershipToPane calls SetPageMode(false) on the old
	// pane before SetPageMode(true) on the new pane.  Set up both.
	hideIndicatorMocks(labelA, f.overlay1A)

	// Set up indicator for pane-b (lazy creation via SetPageMode(true))
	labelB := setupPageModeIndicatorMocks(t, f.factory, f.overlay1B)
	showIndicatorMocks(labelB, f.overlay1B)

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

// setUpNormalPulse sets up mock expectations for a normal (slow) pulse on
// the given overlay, assuming the indicator label has already been created.
func setUpNormalPulse(label *mocks.MockLabelWidget, overlay *mocks.MockOverlayWidget) {
	label.EXPECT().RemoveCssClass("page-mode-indicator-pulse").Once()
	label.EXPECT().RemoveCssClass("page-mode-indicator-pulse-fast").Once()
	label.EXPECT().AddCssClass("page-mode-indicator-pulse").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-fast").Once()
	overlay.EXPECT().AddCssClass("page-mode-pulse").Once()
}

func setUpFastPulse(label *mocks.MockLabelWidget, overlay *mocks.MockOverlayWidget) {
	label.EXPECT().RemoveCssClass("page-mode-indicator-pulse").Once()
	label.EXPECT().RemoveCssClass("page-mode-indicator-pulse-fast").Once()
	label.EXPECT().AddCssClass("page-mode-indicator-pulse-fast").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-fast").Once()
	overlay.EXPECT().AddCssClass("page-mode-pulse-fast").Once()
}

func TestPageMode_Pulse_NormalTriggersOnOwningPane(t *testing.T) {
	f := newPageModeTestFixture(t)

	label := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)

	// Pulse needs the indicator to exist — it was created lazily during
	// enterPageMode, but we also need to set up pulse expectations.
	// (enterPageMode created it, so we just set up pulse mocks.)
	setUpNormalPulse(label, f.overlay1A)

	f.app.triggerPageModePulse(context.Background(), false)
}

func TestPageMode_Pulse_FastTriggersOnOwningPane(t *testing.T) {
	f := newPageModeTestFixture(t)

	label := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	require.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)

	setUpFastPulse(label, f.overlay1A)

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

	label1 := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	_ = enterPageMode(t, f.app, f.factory, f.overlay2A, f.bw2)

	assert.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)
	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.pageModePaneID)

	// Leave bw1 only
	hideIndicatorMocks(label1, f.overlay1A)
	f.app.handlePageModeOwnership(context.Background(), f.bw1, input.ModeNormal, input.ModePage)

	assert.Empty(t, f.bw1.pageModePaneID)
	assert.False(t, f.pv1A.IsPageMode())

	// bw2 unchanged
	assert.Equal(t, entity.PaneID("pane-a"), f.bw2.pageModePaneID)
	assert.True(t, f.pv2A.IsPageMode())
}

func TestPageMode_MultiWindow_TransferIsolated(t *testing.T) {
	f := newPageModeTestFixture(t)

	label1 := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	_ = enterPageMode(t, f.app, f.factory, f.overlay2A, f.bw2)

	f.bw1.keyboardHandler = newKeyboardHandlerInPageMode(t)

	// transferPageModeOwnershipToPane calls SetPageMode(false) on old pane first
	hideIndicatorMocks(label1, f.overlay1A)

	// Transfer bw1 to pane-b
	labelB := setupPageModeIndicatorMocks(t, f.factory, f.overlay1B)
	showIndicatorMocks(labelB, f.overlay1B)
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

	label1 := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	_ = enterPageMode(t, f.app, f.factory, f.overlay2A, f.bw2)

	// Pulse on bw1 (lastFocusedWindowID = "win-1")
	setUpNormalPulse(label1, f.overlay1A)
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
	label := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	hideIndicatorMocks(label, f.overlay1A)
	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-a"), true)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.pageModePaneID)
	assert.False(t, f.pv1A.IsPageMode())
}

func TestPageMode_EditableFocusOnInactivePaneDoesNotExitPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	_ = enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	f.app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-b"), true)

	assert.Equal(t, input.ModePage, kh.Mode())
	assert.Equal(t, entity.PaneID("pane-a"), f.bw1.pageModePaneID)
	assert.True(t, f.pv1A.IsPageMode())
}

func TestPageMode_PaneSwitchToEditablePaneExitsPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	label := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)
	f.app.pageEditableFocusByPane = map[entity.PaneID]bool{
		"pane-b": true,
	}

	hideIndicatorMocks(label, f.overlay1A)
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

	label := enterPageMode(t, app, factory, overlay1, bw1)
	_ = label
	kh := bindPageModeKeyboardHandler(t, app, bw1)

	app.handlePageEditableFocusChanged(context.Background(), entity.PaneID("pane-x"), true)

	assert.Equal(t, input.ModePage, kh.Mode())
	assert.Equal(t, entity.PaneID("pane-a"), bw1.pageModePaneID)
	assert.True(t, pv1.IsPageMode())
}

func TestPageMode_OmniboxFocusExitsPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	label := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	hideIndicatorMocks(label, f.overlay1A)
	f.app.handlePageModeFocusTrigger(context.Background(), f.bw1, usecase.PageModePolicyTriggerOmniboxFocus)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.pageModePaneID)
}

func TestPageMode_FindBarFocusExitsPageMode(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	label := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	hideIndicatorMocks(label, f.overlay1A)
	f.app.handlePageModeFocusTrigger(context.Background(), f.bw1, usecase.PageModePolicyTriggerFindBarFocus)

	assert.Equal(t, input.ModeNormal, kh.Mode())
	assert.Empty(t, f.bw1.pageModePaneID)
}

func TestPageMode_TabSwitchExitsAndClearsOldIndicator(t *testing.T) {
	f := newSingleWindowPageModeFixture(t)
	label := enterPageMode(t, f.app, f.factory, f.overlay1A, f.bw1)
	kh := bindPageModeKeyboardHandler(t, f.app, f.bw1)

	tab2 := &entity.Tab{ID: "tab-2b", Workspace: &entity.Workspace{
		ID:           "ws-2b",
		ActivePaneID: "pane-c",
		Root:         &entity.PaneNode{ID: "ws-2b-pane-c", Pane: entity.NewPane("pane-c")},
	}}
	f.bw1.tabs.Add(tab2)
	f.bw1.tabs.SetActive(tab2.ID)
	f.app.workspaceViews[tab2.ID] = setupWorkspaceViewMocks(t, f.factory)

	hideIndicatorMocks(label, f.overlay1A)
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
	assert.Equal(t, "page-mode-indicator", component.PageModeIndicatorClass)
	assert.Equal(t, "page-mode-active", component.PageModeActiveClass)
	assert.Equal(t, "page-mode-indicator-pulse", component.PageModeIndicatorPulseClass)
	assert.Equal(t, "page-mode-indicator-pulse-fast", component.PageModeIndicatorFastPulseClass)
	assert.Equal(t, "page-mode-pulse", component.PageModePulseClass)
	assert.Equal(t, "page-mode-pulse-fast", component.PageModeFastPulseClass)
}

// ============================================================================
// Toast constant
// ============================================================================

func TestPageMode_ToastIsBrief(t *testing.T) {
	assert.Equal(t, 800, component.ToastBriefDurationMs)
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

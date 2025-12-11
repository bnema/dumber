package component_test

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

func TestNewWorkspaceView_CreatesEmptyContainer(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	// Act
	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Assert
	require.NotNil(t, wv)
	assert.Equal(t, 0, wv.PaneCount())
	assert.Equal(t, mockBox, wv.Container())
}

func TestSetWorkspace_NilWorkspace_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	err := wv.SetWorkspace(nil)

	// Assert
	assert.ErrorIs(t, err, component.ErrNilWorkspace)
}

func TestSetWorkspace_SinglePane_CreatesPaneView(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	// Container creation
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// PaneView creation (via paneViewFactoryAdapter)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	// No SetChild since WebView is nil

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()

	// Widget appended to container
	mockBox.EXPECT().Append(mockOverlay).Once()

	// Active state set
	mockBorderBox.EXPECT().AddCssClass("active-pane").Once()

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	ws := &entity.Workspace{
		ID:           "ws-1",
		Root:         node,
		ActivePaneID: pane.ID,
	}

	// Act
	err := wv.SetWorkspace(ws)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, wv.PaneCount())
	assert.Equal(t, pane.ID, wv.GetActivePaneID())
	assert.NotNil(t, wv.GetPaneView(pane.ID))
}

func TestSetActivePaneID_UpdatesStyling(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Setup two panes
	pane1 := entity.NewPane(entity.PaneID("pane-1"))
	pane2 := entity.NewPane(entity.PaneID("pane-2"))
	node1 := &entity.PaneNode{ID: "node-1", Pane: pane1}
	node2 := &entity.PaneNode{ID: "node-2", Pane: pane2}

	// Create mock widgets for split
	mockPaned := mocks.NewMockPanedWidget(t)
	mockOverlay1 := mocks.NewMockOverlayWidget(t)
	mockOverlay2 := mocks.NewMockOverlayWidget(t)
	mockBorderBox1 := mocks.NewMockBoxWidget(t)
	mockBorderBox2 := mocks.NewMockBoxWidget(t)

	// First pane view creation
	mockFactory.EXPECT().NewOverlay().Return(mockOverlay1).Once()
	mockOverlay1.EXPECT().SetHexpand(true).Once()
	mockOverlay1.EXPECT().SetVexpand(true).Once()
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox1).Once()
	mockBorderBox1.EXPECT().SetCanFocus(false).Once()
	mockBorderBox1.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox1.EXPECT().SetHexpand(true).Once()
	mockBorderBox1.EXPECT().SetVexpand(true).Once()
	mockOverlay1.EXPECT().AddOverlay(mockBorderBox1).Once()
	mockOverlay1.EXPECT().SetClipOverlay(mockBorderBox1, false).Once()
	mockOverlay1.EXPECT().SetMeasureOverlay(mockBorderBox1, false).Once()

	// Second pane view creation
	mockFactory.EXPECT().NewOverlay().Return(mockOverlay2).Once()
	mockOverlay2.EXPECT().SetHexpand(true).Once()
	mockOverlay2.EXPECT().SetVexpand(true).Once()
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox2).Once()
	mockBorderBox2.EXPECT().SetCanFocus(false).Once()
	mockBorderBox2.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox2.EXPECT().SetHexpand(true).Once()
	mockBorderBox2.EXPECT().SetVexpand(true).Once()
	mockOverlay2.EXPECT().AddOverlay(mockBorderBox2).Once()
	mockOverlay2.EXPECT().SetClipOverlay(mockBorderBox2, false).Once()
	mockOverlay2.EXPECT().SetMeasureOverlay(mockBorderBox2, false).Once()

	// Split view creation
	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetShrinkStartChild(false).Once()
	mockPaned.EXPECT().SetShrinkEndChild(false).Once()
	mockPaned.EXPECT().SetStartChild(mockOverlay1).Once()
	mockPaned.EXPECT().SetEndChild(mockOverlay2).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	mockBox.EXPECT().Append(mockPaned).Once()

	// Initial active pane is pane-1
	mockBorderBox1.EXPECT().AddCssClass("active-pane").Once()

	splitNode := &entity.PaneNode{
		ID:         "split",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{node1, node2},
	}

	ws := &entity.Workspace{
		ID:           "ws-1",
		Root:         splitNode,
		ActivePaneID: pane1.ID,
	}

	err := wv.SetWorkspace(ws)
	require.NoError(t, err)

	// Now change active pane
	mockBorderBox1.EXPECT().RemoveCssClass("active-pane").Once()
	mockBorderBox2.EXPECT().AddCssClass("active-pane").Once()

	// Act
	err = wv.SetActivePaneID(pane2.ID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, pane2.ID, wv.GetActivePaneID())
}

func TestSetActivePaneID_InvalidID_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	err := wv.SetActivePaneID(entity.PaneID("non-existent"))

	// Assert
	assert.ErrorIs(t, err, component.ErrPaneNotFound)
}

func TestGetPaneView_Existing(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// PaneView creation
	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()
	mockBox.EXPECT().Append(mockOverlay).Once()
	mockBorderBox.EXPECT().AddCssClass("active-pane").Once()

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}
	ws := &entity.Workspace{
		ID:           "ws-1",
		Root:         node,
		ActivePaneID: pane.ID,
	}

	_ = wv.SetWorkspace(ws)

	// Act
	result := wv.GetPaneView(pane.ID)

	// Assert
	assert.NotNil(t, result)
}

func TestGetPaneView_NonExistent(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	result := wv.GetPaneView(entity.PaneID("non-existent"))

	// Assert
	assert.Nil(t, result)
}

func TestGetPaneIDs_ReturnsAllIDs(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockOverlay1 := mocks.NewMockOverlayWidget(t)
	mockOverlay2 := mocks.NewMockOverlayWidget(t)
	mockBorderBox1 := mocks.NewMockBoxWidget(t)
	mockBorderBox2 := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// First pane view
	mockFactory.EXPECT().NewOverlay().Return(mockOverlay1).Once()
	mockOverlay1.EXPECT().SetHexpand(true).Once()
	mockOverlay1.EXPECT().SetVexpand(true).Once()
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox1).Once()
	mockBorderBox1.EXPECT().SetCanFocus(false).Once()
	mockBorderBox1.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox1.EXPECT().SetHexpand(true).Once()
	mockBorderBox1.EXPECT().SetVexpand(true).Once()
	mockOverlay1.EXPECT().AddOverlay(mockBorderBox1).Once()
	mockOverlay1.EXPECT().SetClipOverlay(mockBorderBox1, false).Once()
	mockOverlay1.EXPECT().SetMeasureOverlay(mockBorderBox1, false).Once()

	// Second pane view
	mockFactory.EXPECT().NewOverlay().Return(mockOverlay2).Once()
	mockOverlay2.EXPECT().SetHexpand(true).Once()
	mockOverlay2.EXPECT().SetVexpand(true).Once()
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox2).Once()
	mockBorderBox2.EXPECT().SetCanFocus(false).Once()
	mockBorderBox2.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox2.EXPECT().SetHexpand(true).Once()
	mockBorderBox2.EXPECT().SetVexpand(true).Once()
	mockOverlay2.EXPECT().AddOverlay(mockBorderBox2).Once()
	mockOverlay2.EXPECT().SetClipOverlay(mockBorderBox2, false).Once()
	mockOverlay2.EXPECT().SetMeasureOverlay(mockBorderBox2, false).Once()

	// Split view
	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetShrinkStartChild(false).Once()
	mockPaned.EXPECT().SetShrinkEndChild(false).Once()
	mockPaned.EXPECT().SetStartChild(mockOverlay1).Once()
	mockPaned.EXPECT().SetEndChild(mockOverlay2).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	mockBox.EXPECT().Append(mockPaned).Once()
	mockBorderBox1.EXPECT().AddCssClass("active-pane").Once()

	pane1 := entity.NewPane(entity.PaneID("pane-1"))
	pane2 := entity.NewPane(entity.PaneID("pane-2"))
	node1 := &entity.PaneNode{ID: "node-1", Pane: pane1}
	node2 := &entity.PaneNode{ID: "node-2", Pane: pane2}

	splitNode := &entity.PaneNode{
		ID:         "split",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{node1, node2},
	}

	ws := &entity.Workspace{
		ID:           "ws-1",
		Root:         splitNode,
		ActivePaneID: pane1.ID,
	}

	_ = wv.SetWorkspace(ws)

	// Act
	ids := wv.GetPaneIDs()

	// Assert
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, pane1.ID)
	assert.Contains(t, ids, pane2.ID)
}

func TestWidget_ReturnsContainer(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	widget := wv.Widget()

	// Assert
	assert.Equal(t, mockBox, widget)
}

func TestSetOnPaneFocused_SetsCallback(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	callback := func(paneID entity.PaneID) {}

	// Act
	wv.SetOnPaneFocused(callback)

	// Assert - callback is set (verified by no panic)
	assert.NotNil(t, wv)
}

func TestSetWebViewWidget_AttachesToPane(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// PaneView creation
	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()
	mockBox.EXPECT().Append(mockOverlay).Once()
	mockBorderBox.EXPECT().AddCssClass("active-pane").Once()

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}
	ws := &entity.Workspace{
		ID:           "ws-1",
		Root:         node,
		ActivePaneID: pane.ID,
	}

	_ = wv.SetWorkspace(ws)

	// Expect WebView to be set
	mockOverlay.EXPECT().SetChild(mockWebView).Once()

	// Act
	err := wv.SetWebViewWidget(pane.ID, mockWebView)

	// Assert
	require.NoError(t, err)
}

func TestSetWebViewWidget_NonExistentPane_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	err := wv.SetWebViewWidget(entity.PaneID("non-existent"), mockWebView)

	// Assert
	assert.ErrorIs(t, err, component.ErrPaneNotFound)
}

func TestFocusPane_DelegatesToPaneView(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// PaneView creation
	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()
	mockBox.EXPECT().Append(mockOverlay).Once()
	mockBorderBox.EXPECT().AddCssClass("active-pane").Once()

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}
	ws := &entity.Workspace{
		ID:           "ws-1",
		Root:         node,
		ActivePaneID: pane.ID,
	}

	_ = wv.SetWorkspace(ws)

	// Act - FocusPane with nil WebView returns false
	result := wv.FocusPane(pane.ID)

	// Assert
	assert.False(t, result) // No WebView attached, so GrabFocus fails
}

func TestFocusPane_NonExistentPane_ReturnsFalse(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	result := wv.FocusPane(entity.PaneID("non-existent"))

	// Assert
	assert.False(t, result)
}

func TestRebuild_NilWorkspace_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	err := wv.Rebuild()

	// Assert
	assert.ErrorIs(t, err, component.ErrNilWorkspace)
}

func TestTreeRenderer_ReturnsRenderer(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	renderer := wv.TreeRenderer()

	// Assert
	assert.NotNil(t, renderer)
}

func TestWorkspace_ReturnsCurrentWorkspace(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// PaneView creation
	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()
	mockBox.EXPECT().Append(mockOverlay).Once()
	mockBorderBox.EXPECT().AddCssClass("active-pane").Once()

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}
	ws := &entity.Workspace{
		ID:           "ws-1",
		Root:         node,
		ActivePaneID: pane.ID,
	}

	_ = wv.SetWorkspace(ws)

	// Act
	result := wv.Workspace()

	// Assert
	assert.Equal(t, ws, result)
}

func TestWorkspace_NilBeforeSet(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	wv := component.NewWorkspaceView(mockFactory, zerolog.Nop())

	// Act
	result := wv.Workspace()

	// Assert
	assert.Nil(t, result)
}

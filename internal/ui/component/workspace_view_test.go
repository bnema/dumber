package component_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

func setupLoadingSkeletonMocksWorkspace(
	t *testing.T,
	mockFactory *mocks.MockWidgetFactory,
	mockOverlay *mocks.MockOverlayWidget,
	mockLoadingContainer *mocks.MockBoxWidget,
	mockLoadingContent *mocks.MockBoxWidget,
	mockLoadingSpinner *mocks.MockSpinnerWidget,
	mockLoadingLogo *mocks.MockImageWidget,
) {
	mockLoadingVersion := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockLoadingContainer).Once()
	mockLoadingContainer.EXPECT().SetHexpand(true).Maybe()
	mockLoadingContainer.EXPECT().SetVexpand(true).Maybe()
	mockLoadingContainer.EXPECT().SetHalign(mock.Anything).Maybe()
	mockLoadingContainer.EXPECT().SetValign(mock.Anything).Maybe()
	mockLoadingContainer.EXPECT().SetCanFocus(false).Maybe()
	mockLoadingContainer.EXPECT().SetCanTarget(false).Maybe()
	mockLoadingContainer.EXPECT().AddCssClass("loading-skeleton").Maybe()
	mockLoadingContainer.EXPECT().SetVisible(true).Maybe()

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 6).Return(mockLoadingContent).Once()
	mockLoadingContent.EXPECT().SetHalign(mock.Anything).Maybe()
	mockLoadingContent.EXPECT().SetValign(mock.Anything).Maybe()
	mockLoadingContent.EXPECT().SetCanFocus(false).Maybe()
	mockLoadingContent.EXPECT().SetCanTarget(false).Maybe()
	mockLoadingContent.EXPECT().AddCssClass("loading-skeleton-content").Maybe()

	mockFactory.EXPECT().NewImage().Return(mockLoadingLogo).Once()
	mockLoadingLogo.EXPECT().SetHalign(mock.Anything).Maybe()
	mockLoadingLogo.EXPECT().SetValign(mock.Anything).Maybe()
	mockLoadingLogo.EXPECT().SetCanFocus(false).Maybe()
	mockLoadingLogo.EXPECT().SetCanTarget(false).Maybe()
	mockLoadingLogo.EXPECT().SetSizeRequest(mock.Anything, mock.Anything).Maybe()
	mockLoadingLogo.EXPECT().SetPixelSize(mock.Anything).Maybe()
	mockLoadingLogo.EXPECT().AddCssClass("loading-skeleton-logo").Maybe()
	mockLoadingLogo.EXPECT().SetFromPaintable(mock.Anything).Maybe()

	mockFactory.EXPECT().NewSpinner().Return(mockLoadingSpinner).Once()
	mockLoadingSpinner.EXPECT().SetHalign(mock.Anything).Maybe()
	mockLoadingSpinner.EXPECT().SetValign(mock.Anything).Maybe()
	mockLoadingSpinner.EXPECT().SetCanFocus(false).Maybe()
	mockLoadingSpinner.EXPECT().SetCanTarget(false).Maybe()
	mockLoadingSpinner.EXPECT().SetSizeRequest(mock.Anything, mock.Anything).Maybe()
	mockLoadingSpinner.EXPECT().AddCssClass("loading-skeleton-spinner").Maybe()
	mockLoadingSpinner.EXPECT().Start().Maybe()

	mockFactory.EXPECT().NewLabel(mock.Anything).Return(mockLoadingVersion).Once()
	mockLoadingVersion.EXPECT().SetHalign(mock.Anything).Maybe()
	mockLoadingVersion.EXPECT().SetValign(mock.Anything).Maybe()
	mockLoadingVersion.EXPECT().SetCanFocus(false).Maybe()
	mockLoadingVersion.EXPECT().SetCanTarget(false).Maybe()
	mockLoadingVersion.EXPECT().AddCssClass("loading-skeleton-version").Maybe()

	mockLoadingContent.EXPECT().Append(mockLoadingLogo).Maybe()
	mockLoadingContent.EXPECT().Append(mockLoadingSpinner).Maybe()
	mockLoadingContent.EXPECT().Append(mockLoadingVersion).Maybe()
	mockLoadingContainer.EXPECT().Append(mockLoadingContent).Maybe()

	mockOverlay.EXPECT().AddOverlay(mockLoadingContainer).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockLoadingContainer, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockLoadingContainer, false).Once()
}

func setupWorkspaceViewBase(t *testing.T, mockFactory *mocks.MockWidgetFactory) (context.Context, *mocks.MockBoxWidget) {
	ctx := context.Background()
	mockContainer := mocks.NewMockBoxWidget(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockContainer).Once()
	mockContainer.EXPECT().SetHexpand(true).Once()
	mockContainer.EXPECT().SetVexpand(true).Once()
	mockContainer.EXPECT().SetVisible(true).Once()

	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().SetChild(mockContainer).Once()
	mockOverlay.EXPECT().SetVisible(true).Once()

	return ctx, mockContainer
}

func setupWorkspacePaneViewMocks(t *testing.T, mockFactory *mocks.MockWidgetFactory) (*mocks.MockOverlayWidget, *mocks.MockBoxWidget) {
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)

	mockLoadingContainer := mocks.NewMockBoxWidget(t)
	mockLoadingContent := mocks.NewMockBoxWidget(t)
	mockLoadingSpinner := mocks.NewMockSpinnerWidget(t)
	mockLoadingLogo := mocks.NewMockImageWidget(t)

	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("pane-overlay").Once()

	setupLoadingSkeletonMocksWorkspace(t, mockFactory, mockOverlay, mockLoadingContainer, mockLoadingContent, mockLoadingSpinner, mockLoadingLogo)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().SetCanTarget(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()

	// GtkWidget is called when attaching hover handler - return nil for tests
	mockOverlay.EXPECT().GtkWidget().Return(nil).Once()

	return mockOverlay, mockBorderBox
}

func setupStackedLeafMocks(
	t *testing.T,
	mockFactory *mocks.MockWidgetFactory,
	container *mocks.MockOverlayWidget,
) *mocks.MockBoxWidget {
	mockStackBox := mocks.NewMockBoxWidget(t)
	mockTitleBar := mocks.NewMockBoxWidget(t)
	mockFavicon := mocks.NewMockImageWidget(t)
	mockLabel := mocks.NewMockLabelWidget(t)
	mockCloseButton := mocks.NewMockButtonWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockStackBox).Once()
	mockStackBox.EXPECT().SetHexpand(true).Once()
	mockStackBox.EXPECT().SetVexpand(true).Once()

	// Title bar creation - now directly used without button wrapper
	mockFactory.EXPECT().NewBox(layout.OrientationHorizontal, 4).Return(mockTitleBar).Once()
	mockTitleBar.EXPECT().AddCssClass("stacked-pane-titlebar").Once()
	mockTitleBar.EXPECT().AddCssClass("stacked-pane-title-clickable").Once()
	mockTitleBar.EXPECT().SetVexpand(false).Once()
	mockTitleBar.EXPECT().SetHexpand(true).Once()

	mockFactory.EXPECT().NewImage().Return(mockFavicon).Once()
	mockFavicon.EXPECT().SetFromIconName(mock.Anything).Once()
	mockFavicon.EXPECT().SetPixelSize(16).Once()
	mockTitleBar.EXPECT().Append(mockFavicon).Once()

	mockFactory.EXPECT().NewLabel(mock.Anything).Return(mockLabel).Once()
	mockLabel.EXPECT().SetEllipsize(layout.EllipsizeEnd).Once()
	mockLabel.EXPECT().SetMaxWidthChars(30).Once()
	mockLabel.EXPECT().SetHexpand(true).Once()
	mockLabel.EXPECT().SetXalign(float32(0.0)).Once()
	mockTitleBar.EXPECT().Append(mockLabel).Once()

	// Close button (uses SetIconName directly instead of child image)
	mockFactory.EXPECT().NewButton().Return(mockCloseButton).Once()
	mockCloseButton.EXPECT().SetIconName("window-close-symbolic").Once()
	mockCloseButton.EXPECT().AddCssClass("stacked-pane-close-button").Once()
	mockCloseButton.EXPECT().SetFocusOnClick(false).Once()
	mockCloseButton.EXPECT().SetVexpand(false).Once()
	mockCloseButton.EXPECT().SetHexpand(false).Once()
	mockTitleBar.EXPECT().Append(mockCloseButton).Once()

	// GestureClick is added to titleBar via AddController
	mockTitleBar.EXPECT().AddController(mock.Anything).Once()

	// Close button click handler
	mockCloseButton.EXPECT().ConnectClicked(mock.Anything).Return(uint(2)).Once()

	// Signal disconnection calls GtkWidget() - return nil to skip actual GTK operations in tests
	mockCloseButton.EXPECT().GtkWidget().Return(nil).Maybe()

	// Adding to main box - now titleBar is added directly (no button wrapper)
	mockStackBox.EXPECT().Append(mockTitleBar).Once()
	mockStackBox.EXPECT().Append(container).Once()

	// Visibility updates for active pane - titleBar is now directly in box, SetVisible called on it
	mockTitleBar.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	container.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	return mockStackBox
}

func TestNewWorkspaceView_CreatesEmptyContainer(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	ctx, mockBox := setupWorkspaceViewBase(t, mockFactory)

	// Act
	wv := component.NewWorkspaceView(ctx, mockFactory)

	// Assert
	require.NotNil(t, wv)
	assert.Equal(t, 0, wv.PaneCount())
	assert.Equal(t, mockBox, wv.Container())
}

func TestSetWorkspace_NilWorkspace_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	ctx, _ := setupWorkspaceViewBase(t, mockFactory)

	wv := component.NewWorkspaceView(ctx, mockFactory)

	// Act
	err := wv.SetWorkspace(ctx, nil)

	// Assert
	assert.ErrorIs(t, err, component.ErrNilWorkspace)
}

func TestSetWorkspace_SinglePane_CreatesPaneView(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	ctx, mockBox := setupWorkspaceViewBase(t, mockFactory)

	wv := component.NewWorkspaceView(ctx, mockFactory)

	mockOverlay, mockBorderBox := setupWorkspacePaneViewMocks(t, mockFactory)
	mockStackBox := setupStackedLeafMocks(t, mockFactory, mockOverlay)

	mockStackBox.EXPECT().SetVisible(true).Once()
	mockBox.EXPECT().Append(mockStackBox).Once()
	mockBox.EXPECT().AddCssClass("single-pane").Once()
	mockBorderBox.EXPECT().AddCssClass("pane-active").Once()

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	ws := &entity.Workspace{
		ID:           "ws-1",
		Root:         node,
		ActivePaneID: pane.ID,
	}

	// Act
	err := wv.SetWorkspace(ctx, ws)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, wv.PaneCount())
	assert.Equal(t, pane.ID, wv.GetActivePaneID())
	assert.NotNil(t, wv.GetPaneView(pane.ID))
}

func TestSetActivePaneID_UpdatesStyling(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	ctx, mockBox := setupWorkspaceViewBase(t, mockFactory)

	wv := component.NewWorkspaceView(ctx, mockFactory)

	// Setup two panes
	pane1 := entity.NewPane(entity.PaneID("pane-1"))
	pane2 := entity.NewPane(entity.PaneID("pane-2"))
	node1 := &entity.PaneNode{ID: "node-1", Pane: pane1}
	node2 := &entity.PaneNode{ID: "node-2", Pane: pane2}

	// Create mock widgets for split
	mockPaned := mocks.NewMockPanedWidget(t)
	mockOverlay1, mockBorderBox1 := setupWorkspacePaneViewMocks(t, mockFactory)
	mockStackBox1 := setupStackedLeafMocks(t, mockFactory, mockOverlay1)
	mockOverlay2, mockBorderBox2 := setupWorkspacePaneViewMocks(t, mockFactory)
	mockStackBox2 := setupStackedLeafMocks(t, mockFactory, mockOverlay2)

	// Split view creation
	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetVisible(true).Twice()
	mockStackBox1.EXPECT().SetVisible(true).Once()
	mockStackBox2.EXPECT().SetVisible(true).Once()
	mockPaned.EXPECT().SetStartChild(mockStackBox1).Once()
	mockPaned.EXPECT().SetEndChild(mockStackBox2).Once()
	mockPaned.EXPECT().ConnectNotifyPosition(mock.Anything).Return(uint(0)).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	mockBox.EXPECT().Append(mockPaned).Once()

	// Initial active pane is pane-1
	mockBox.EXPECT().RemoveCssClass("single-pane").Once()
	mockBorderBox1.EXPECT().AddCssClass("pane-active").Once()

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

	err := wv.SetWorkspace(ctx, ws)
	require.NoError(t, err)

	// Now change active pane
	mockBorderBox1.EXPECT().RemoveCssClass("pane-active").Once()
	mockBorderBox2.EXPECT().AddCssClass("pane-active").Once()

	// Act
	err = wv.SetActivePaneID(pane2.ID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, pane2.ID, wv.GetActivePaneID())
}

func TestSetActivePaneID_InvalidID_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	ctx, _ := setupWorkspaceViewBase(t, mockFactory)

	wv := component.NewWorkspaceView(ctx, mockFactory)

	// Act
	err := wv.SetActivePaneID(entity.PaneID("non-existent"))

	// Assert
	assert.ErrorIs(t, err, component.ErrPaneNotFound)
}

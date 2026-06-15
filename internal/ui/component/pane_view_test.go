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

func TestNewPaneView_CreatesOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	mockLoadingContainer := mocks.NewMockBoxWidget(t)
	mockLoadingContent := mocks.NewMockBoxWidget(t)
	mockLoadingSpinner := mocks.NewMockSpinnerWidget(t)
	mockLoadingLogo := mocks.NewMockImageWidget(t)

	paneID := entity.PaneID("pane-1")

	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("pane-overlay").Once() // Theme background
	mockOverlay.EXPECT().SetChild(mockWebView).Once()

	setupLoadingSkeletonMocks(t, mockFactory, mockOverlay, mockLoadingContainer, mockLoadingContent, mockLoadingSpinner, mockLoadingLogo)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().SetCanTarget(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()

	// Act
	pv := component.NewPaneView(context.Background(), mockFactory, paneID, mockWebView)

	// Assert
	require.NotNil(t, pv)
	assert.Equal(t, paneID, pv.PaneID())
	assert.Equal(t, mockWebView, pv.WebViewWidget())
}

func TestNewPaneView_NilWebView(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)

	mockLoadingContainer := mocks.NewMockBoxWidget(t)
	mockLoadingContent := mocks.NewMockBoxWidget(t)
	mockLoadingSpinner := mocks.NewMockSpinnerWidget(t)
	mockLoadingLogo := mocks.NewMockImageWidget(t)

	paneID := entity.PaneID("pane-1")

	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("pane-overlay").Once() // Theme background
	// SetChild should NOT be called when webview is nil

	setupLoadingSkeletonMocks(t, mockFactory, mockOverlay, mockLoadingContainer, mockLoadingContent, mockLoadingSpinner, mockLoadingLogo)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().SetCanTarget(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()

	// Act
	pv := component.NewPaneView(context.Background(), mockFactory, paneID, nil)

	// Assert
	require.NotNil(t, pv)
	assert.Nil(t, pv.WebViewWidget())
}

func TestSetActive_True_AddsCSSClass(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Expect CSS class to be added
	mockBorderBox.EXPECT().AddCssClass("pane-active").Once()

	// Act
	pv.SetActive(true)

	// Assert
	assert.True(t, pv.IsActive())
}

func TestSetActive_False_RemovesCSSClass(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// First activate
	mockBorderBox.EXPECT().AddCssClass("pane-active").Once()
	pv.SetActive(true)

	// Then deactivate
	mockBorderBox.EXPECT().RemoveCssClass("pane-active").Once()

	// Act
	pv.SetActive(false)

	// Assert
	assert.False(t, pv.IsActive())
}

func TestSetActive_NoChangeWhenSameState(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Act - setting false when already false should not call RemoveCssClass
	pv.SetActive(false)

	// Assert - no mock expectations for CSS changes
	assert.False(t, pv.IsActive())
}

func TestPaneID_ReturnsPaneID(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	paneID := entity.PaneID("test-pane-123")
	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, paneID, mockWebView)

	// Act
	result := pv.PaneID()

	// Assert
	assert.Equal(t, paneID, result)
}

func TestWebViewWidget_ReturnsWebView(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Act
	result := pv.WebViewWidget()

	// Assert
	assert.Equal(t, mockWebView, result)
}

func TestSetWebViewWidget_ReplacesWidget(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockOldWebView := mocks.NewMockWidget(t)
	mockNewWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockOldWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockOldWebView)

	// Expect removal of old widget and addition of new
	mockOverlay.EXPECT().SetChild(nil).Once()
	mockOverlay.EXPECT().GetAllocatedWidth().Return(0).Twice()
	mockOverlay.EXPECT().GetAllocatedHeight().Return(0).Twice()
	mockNewWebView.EXPECT().GetParent().Return(nil).Once()
	mockNewWebView.EXPECT().IsVisible().Return(false).Once()
	mockNewWebView.EXPECT().GetAllocatedWidth().Return(0).Twice()
	mockNewWebView.EXPECT().GetAllocatedHeight().Return(0).Twice()
	mockOverlay.EXPECT().SetChild(mockNewWebView).Once()

	// Act
	pv.SetWebViewWidget(mockNewWebView)

	// Assert
	assert.Equal(t, mockNewWebView, pv.WebViewWidget())
}

func TestSetWebViewWidget_ReparentsWidgetWithExistingParent(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockOldWebView := mocks.NewMockWidget(t)
	mockNewWebView := mocks.NewMockWidget(t)
	mockParent := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockOldWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockOldWebView)

	mockOverlay.EXPECT().SetChild(nil).Once()
	mockOverlay.EXPECT().GetAllocatedWidth().Return(0).Twice()
	mockOverlay.EXPECT().GetAllocatedHeight().Return(0).Twice()
	mockNewWebView.EXPECT().GetAllocatedWidth().Return(0).Twice()
	mockNewWebView.EXPECT().GetAllocatedHeight().Return(0).Twice()
	mockNewWebView.EXPECT().GetParent().Return(mockParent).Once()
	mockNewWebView.EXPECT().IsVisible().Return(true).Once()
	mockNewWebView.EXPECT().Unparent().Once()
	mockOverlay.EXPECT().SetChild(mockNewWebView).Once()
	mockNewWebView.EXPECT().SetVisible(true).Once()

	// Loading skeleton is hidden immediately for a reparented, already-visible widget.

	// Act
	pv.SetWebViewWidget(mockNewWebView)

	// Assert
	assert.Equal(t, mockNewWebView, pv.WebViewWidget())
}

func TestSetWebViewWidget_FromNil(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockNewWebView := mocks.NewMockWidget(t)

	setupPaneViewMocksNoWebView(t, mockFactory, mockOverlay, mockBorderBox)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), nil)

	// Expect only setting new child (no removal since old was nil)
	mockOverlay.EXPECT().GetAllocatedWidth().Return(0).Twice()
	mockOverlay.EXPECT().GetAllocatedHeight().Return(0).Twice()
	mockNewWebView.EXPECT().GetParent().Return(nil).Once()
	mockNewWebView.EXPECT().IsVisible().Return(false).Once()
	mockNewWebView.EXPECT().GetAllocatedWidth().Return(0).Twice()
	mockNewWebView.EXPECT().GetAllocatedHeight().Return(0).Twice()
	mockOverlay.EXPECT().SetChild(mockNewWebView).Once()

	// Act
	pv.SetWebViewWidget(mockNewWebView)

	// Assert
	assert.Equal(t, mockNewWebView, pv.WebViewWidget())
}

func TestGrabFocus_DelegatesToWebView(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	mockWebView.EXPECT().GrabFocus().Return(true).Once()

	// Act
	result := pv.GrabFocus()

	// Assert
	assert.True(t, result)
}

func TestGrabFocus_NilWebView_ReturnsFalse(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)

	setupPaneViewMocksNoWebView(t, mockFactory, mockOverlay, mockBorderBox)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), nil)

	// Act
	result := pv.GrabFocus()

	// Assert
	assert.False(t, result)
}

func TestHasFocus_DelegatesToWebView(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	mockWebView.EXPECT().HasFocus().Return(true).Once()

	// Act
	result := pv.HasFocus()

	// Assert
	assert.True(t, result)
}

func TestHasFocus_NilWebView_ReturnsFalse(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)

	setupPaneViewMocksNoWebView(t, mockFactory, mockOverlay, mockBorderBox)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), nil)

	// Act
	result := pv.HasFocus()

	// Assert
	assert.False(t, result)
}

func TestWidget_ReturnsOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Act
	result := pv.Widget()

	// Assert
	assert.Equal(t, mockOverlay, result)
}

func TestOverlay_ReturnsOverlayWidget(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Act
	result := pv.Overlay()

	// Assert
	assert.Equal(t, mockOverlay, result)
}

func TestShow_DelegatesToOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	mockOverlay.EXPECT().Show().Once()

	// Act
	pv.Show()

	// Assert - verified by mock expectations
}

func TestHide_DelegatesToOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	mockOverlay.EXPECT().Hide().Once()

	// Act
	pv.Hide()

	// Assert - verified by mock expectations
}

func TestSetVisible_DelegatesToOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	mockOverlay.EXPECT().SetVisible(true).Once()

	// Act
	pv.SetVisible(true)

	// Assert - verified by mock expectations
}

func TestIsVisible_DelegatesToOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	mockOverlay.EXPECT().IsVisible().Return(true).Once()

	// Act
	result := pv.IsVisible()

	// Assert
	assert.True(t, result)
}

func TestAddCssClass_DelegatesToOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	mockOverlay.EXPECT().AddCssClass("custom-class").Once()

	// Act
	pv.AddCssClass("custom-class")

	// Assert - verified by mock expectations
}

func TestRemoveCssClass_DelegatesToOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	mockOverlay.EXPECT().RemoveCssClass("custom-class").Once()

	// Act
	pv.RemoveCssClass("custom-class")

	// Assert - verified by mock expectations
}

func TestSetOnFocusIn_SetsCallback(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	callback := func(paneID entity.PaneID) {}

	// Act
	pv.SetOnFocusIn(callback)

	// Assert - callback is set (we can't directly verify without triggering focus)
	assert.NotNil(t, pv)
}

func TestSetOnFocusOut_SetsCallback(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	callback := func(paneID entity.PaneID) {}

	// Act
	pv.SetOnFocusOut(callback)

	// Assert - callback is set (we can't directly verify without triggering focus)
	assert.NotNil(t, pv)
}

// Helper function to setup common mock expectations for PaneView creation
func setupLoadingSkeletonMocks(
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
	mockLoadingContainer.EXPECT().SetVisible(mock.Anything).Maybe()

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
	mockLoadingLogo.EXPECT().SetSizeRequest(256, 256).Maybe()
	mockLoadingLogo.EXPECT().SetPixelSize(256).Maybe()
	mockLoadingLogo.EXPECT().AddCssClass("loading-skeleton-logo").Maybe()
	mockLoadingLogo.EXPECT().SetFromPaintable(mock.Anything).Maybe()

	mockFactory.EXPECT().NewSpinner().Return(mockLoadingSpinner).Once()
	mockLoadingSpinner.EXPECT().SetHalign(mock.Anything).Maybe()
	mockLoadingSpinner.EXPECT().SetValign(mock.Anything).Maybe()
	mockLoadingSpinner.EXPECT().SetCanFocus(false).Maybe()
	mockLoadingSpinner.EXPECT().SetCanTarget(false).Maybe()
	mockLoadingSpinner.EXPECT().SetSizeRequest(32, 32).Maybe()
	mockLoadingSpinner.EXPECT().AddCssClass("loading-skeleton-spinner").Maybe()
	mockLoadingSpinner.EXPECT().Start().Maybe()
	mockLoadingSpinner.EXPECT().Stop().Maybe()

	mockFactory.EXPECT().NewLabel(mock.Anything).Return(mockLoadingVersion).Once()
	mockLoadingVersion.EXPECT().SetHalign(mock.Anything).Maybe()
	mockLoadingVersion.EXPECT().SetValign(mock.Anything).Maybe()
	mockLoadingVersion.EXPECT().SetCanFocus(false).Maybe()
	mockLoadingVersion.EXPECT().SetCanTarget(false).Maybe()
	mockLoadingVersion.EXPECT().SetMaxWidthChars(mock.Anything).Maybe()
	mockLoadingVersion.EXPECT().SetEllipsize(mock.Anything).Maybe()
	mockLoadingVersion.EXPECT().AddCssClass("loading-skeleton-version").Maybe()

	mockLoadingContent.EXPECT().Append(mockLoadingLogo).Maybe()
	mockLoadingContent.EXPECT().Append(mockLoadingSpinner).Maybe()
	mockLoadingContent.EXPECT().Append(mockLoadingVersion).Maybe()
	mockLoadingContainer.EXPECT().Append(mockLoadingContent).Maybe()

	mockOverlay.EXPECT().AddOverlay(mockLoadingContainer).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockLoadingContainer, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockLoadingContainer, false).Once()
}

// Helper function to setup common mock expectations for PaneView creation
func setupPaneViewMocks(
	t *testing.T,
	mockFactory *mocks.MockWidgetFactory,
	mockOverlay *mocks.MockOverlayWidget,
	mockBorderBox *mocks.MockBoxWidget,
	mockWebView *mocks.MockWidget,
) {
	mockLoadingContainer := mocks.NewMockBoxWidget(t)
	mockLoadingContent := mocks.NewMockBoxWidget(t)
	mockLoadingSpinner := mocks.NewMockSpinnerWidget(t)
	mockLoadingLogo := mocks.NewMockImageWidget(t)

	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("pane-overlay").Once() // Theme background
	mockOverlay.EXPECT().SetChild(mockWebView).Once()

	setupLoadingSkeletonMocks(t, mockFactory, mockOverlay, mockLoadingContainer, mockLoadingContent, mockLoadingSpinner, mockLoadingLogo)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().SetCanTarget(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()
}

func setupPaneViewMocksNoWebView(
	t *testing.T,
	mockFactory *mocks.MockWidgetFactory,
	mockOverlay *mocks.MockOverlayWidget,
	mockBorderBox *mocks.MockBoxWidget,
) {
	mockLoadingContainer := mocks.NewMockBoxWidget(t)
	mockLoadingContent := mocks.NewMockBoxWidget(t)
	mockLoadingSpinner := mocks.NewMockSpinnerWidget(t)
	mockLoadingLogo := mocks.NewMockImageWidget(t)

	mockFactory.EXPECT().NewOverlay().Return(mockOverlay).Once()
	mockOverlay.EXPECT().SetHexpand(true).Once()
	mockOverlay.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("pane-overlay").Once() // Theme background

	setupLoadingSkeletonMocks(t, mockFactory, mockOverlay, mockLoadingContainer, mockLoadingContent, mockLoadingSpinner, mockLoadingLogo)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBorderBox).Once()
	mockBorderBox.EXPECT().SetCanFocus(false).Once()
	mockBorderBox.EXPECT().SetCanTarget(false).Once()
	mockBorderBox.EXPECT().AddCssClass("pane-border").Once()
	mockBorderBox.EXPECT().SetHexpand(true).Once()
	mockBorderBox.EXPECT().SetVexpand(true).Once()
	mockOverlay.EXPECT().AddOverlay(mockBorderBox).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockBorderBox, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockBorderBox, false).Once()
}

// Helper to set up mock expectations for page mode indicator creation.
// The returned mockLabel must be used for subsequent indicator expectations.
func setupPageModeIndicatorMocks(
	t *testing.T,
	mockFactory *mocks.MockWidgetFactory,
	mockOverlay *mocks.MockOverlayWidget,
) *mocks.MockLabelWidget {
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()
	// Indicator positioning: anchored at top-left corner of the pane
	mockLabel.EXPECT().SetHalign(mock.Anything).Once()
	mockLabel.EXPECT().SetValign(mock.Anything).Once()
	mockOverlay.EXPECT().AddOverlay(mockLabel).Once()
	mockOverlay.EXPECT().SetClipOverlay(mockLabel, false).Once()
	mockOverlay.EXPECT().SetMeasureOverlay(mockLabel, false).Once()

	return mockLabel
}

func TestSetPageMode_True_ShowsIndicatorAndAddsCSSClass(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Set up indicator creation expectations
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)

	// SetPageMode(true) expectations:
	// 1. Indicator is shown
	mockLabel.EXPECT().SetVisible(true).Once()
	// 2. Overlay gets page-mode-active class
	mockOverlay.EXPECT().AddCssClass("page-mode-active").Once()

	// Act
	pv.SetPageMode(true)

	// Assert
	require.True(t, pv.IsPageMode())
}

func TestSetPageMode_False_HidesIndicatorAndRemovesCSSClass(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Activate page mode first
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)
	mockLabel.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("page-mode-active").Once()
	pv.SetPageMode(true)

	// Then deactivate
	mockLabel.EXPECT().SetVisible(false).Once()
	mockOverlay.EXPECT().RemoveCssClass("page-mode-active").Once()

	// Act
	pv.SetPageMode(false)

	// Assert
	require.False(t, pv.IsPageMode())
}

func TestSetPageMode_NoChangeWhenSameState(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Act - setting false when already false should not create indicator or toggle classes
	pv.SetPageMode(false)

	// Assert
	require.False(t, pv.IsPageMode())
}

func TestSetPageMode_TrueThenTrueIsIdempotent(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// First activate
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)
	mockLabel.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("page-mode-active").Once()
	pv.SetPageMode(true)

	// Second activation should be no-op (no additional mock calls)
	// Act
	pv.SetPageMode(true)

	// Assert
	require.True(t, pv.IsPageMode())
}

func expectPageModeIndicatorPulse(label *mocks.MockLabelWidget, fast bool, cycle string) {
	label.EXPECT().RemoveCssClass("page-mode-indicator-pulse").Once()
	label.EXPECT().RemoveCssClass("page-mode-indicator-pulse-fast").Once()
	label.EXPECT().RemoveCssClass("page-mode-pulse-cycle-a").Once()
	label.EXPECT().RemoveCssClass("page-mode-pulse-cycle-b").Once()
	if fast {
		label.EXPECT().AddCssClass("page-mode-indicator-pulse-fast").Once()
	} else {
		label.EXPECT().AddCssClass("page-mode-indicator-pulse").Once()
	}
	label.EXPECT().AddCssClass(cycle).Once()
}

func expectPageModeOverlayPulse(overlay *mocks.MockOverlayWidget, fast bool, cycle string) {
	overlay.EXPECT().RemoveCssClass("page-mode-pulse").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-fast").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-cycle-a").Once()
	overlay.EXPECT().RemoveCssClass("page-mode-pulse-cycle-b").Once()
	if fast {
		overlay.EXPECT().AddCssClass("page-mode-pulse-fast").Once()
	} else {
		overlay.EXPECT().AddCssClass("page-mode-pulse").Once()
	}
	overlay.EXPECT().AddCssClass(cycle).Once()
}

func TestTriggerPageModePulse_PulsesIndicatorAndOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// First activate page mode to create indicator
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)
	mockLabel.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("page-mode-active").Once()
	pv.SetPageMode(true)

	expectPageModeIndicatorPulse(mockLabel, false, "page-mode-pulse-cycle-a")
	expectPageModeOverlayPulse(mockOverlay, false, "page-mode-pulse-cycle-a")

	// Act
	pv.TriggerPageModePulse()
}

func TestTriggerPageModePulseFast_PulsesIndicatorAndOverlay(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// First activate page mode to create indicator
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)
	mockLabel.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("page-mode-active").Once()
	pv.SetPageMode(true)

	expectPageModeIndicatorPulse(mockLabel, true, "page-mode-pulse-cycle-a")
	expectPageModeOverlayPulse(mockOverlay, true, "page-mode-pulse-cycle-a")

	// Act
	pv.TriggerPageModePulseFast()
}

func TestTriggerPageModePulse_CreatesIndicatorLazily(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Trigger page mode pulse WITHOUT having called SetPageMode first.
	// This should lazily create the indicator (but NOT show it).
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)

	expectPageModeIndicatorPulse(mockLabel, false, "page-mode-pulse-cycle-a")
	expectPageModeOverlayPulse(mockOverlay, false, "page-mode-pulse-cycle-a")

	// Act - should not panic or error
	pv.TriggerPageModePulse()
}

func TestTriggerPageModePulse_RepeatedCallsReTriggerAnimation(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Activate page mode to create indicator
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)
	mockLabel.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("page-mode-active").Once()
	pv.SetPageMode(true)

	// First pulse (normal)
	expectPageModeIndicatorPulse(mockLabel, false, "page-mode-pulse-cycle-a")
	expectPageModeOverlayPulse(mockOverlay, false, "page-mode-pulse-cycle-a")
	pv.TriggerPageModePulse()

	// Second pulse — fast. Alternate cycle class to re-arm the GTK animation.
	expectPageModeIndicatorPulse(mockLabel, true, "page-mode-pulse-cycle-b")
	expectPageModeOverlayPulse(mockOverlay, true, "page-mode-pulse-cycle-b")
	pv.TriggerPageModePulseFast()

	// Third pulse — normal again, cycling back to the first variant.
	expectPageModeIndicatorPulse(mockLabel, false, "page-mode-pulse-cycle-a")
	expectPageModeOverlayPulse(mockOverlay, false, "page-mode-pulse-cycle-a")
	pv.TriggerPageModePulse()
}

func TestPageModeIndicator_DoesNotLeakInCleanup(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Create the indicator by activating page mode
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)
	mockLabel.EXPECT().SetVisible(true).Once()
	mockOverlay.EXPECT().AddCssClass("page-mode-active").Once()
	pv.SetPageMode(true)

	// Cleanup should remove the indicator overlay
	mockOverlay.EXPECT().RemoveOverlay(mockLabel).Once()

	// Cleanup also cleans up existing widgets - WebView, etc.
	mockOverlay.EXPECT().SetChild(nil).Once()

	// Act
	pv.Cleanup()
}

func TestPageModeIndicator_NewPaneView_HiddenByDefault(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	// Act
	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)

	// Assert
	require.False(t, pv.IsPageMode())
}

func TestPageModeIndicator_ReturnsIndicator(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockOverlay := mocks.NewMockOverlayWidget(t)
	mockBorderBox := mocks.NewMockBoxWidget(t)
	mockWebView := mocks.NewMockWidget(t)

	setupPaneViewMocks(t, mockFactory, mockOverlay, mockBorderBox, mockWebView)

	pv := component.NewPaneView(context.Background(), mockFactory, entity.PaneID("pane-1"), mockWebView)
	_ = pv

	// Access PageModeIndicator without activating page mode first
	// This should lazily create the indicator.

	// The existing helper expects the full creation sequence.
	// For this test we need to create the indicator via PageModeIndicator() getter.
	mockLabel := setupPageModeIndicatorMocks(t, mockFactory, mockOverlay)

	// Act
	indicator := pv.PageModeIndicator()

	// Assert
	require.NotNil(t, indicator)
	require.Equal(t, mockLabel, indicator.Widget())
}

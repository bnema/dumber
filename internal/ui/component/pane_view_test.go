package component_test

import (
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
	pv := component.NewPaneView(mockFactory, paneID, mockWebView)

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
	pv := component.NewPaneView(mockFactory, paneID, nil)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, paneID, mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockOldWebView)

	// Expect removal of old widget and addition of new
	mockOverlay.EXPECT().SetChild(nil).Once()
	mockNewWebView.EXPECT().GetParent().Return(nil).Once()
	mockOverlay.EXPECT().SetChild(mockNewWebView).Once()

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), nil)

	// Expect only setting new child (no removal since old was nil)
	mockNewWebView.EXPECT().GetParent().Return(nil).Once()
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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), nil)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), nil)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

	pv := component.NewPaneView(mockFactory, entity.PaneID("pane-1"), mockWebView)

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

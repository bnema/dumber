package layout_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

// setupMockFactory creates a factory that returns configured mocks for stacked view tests.
func setupMockFactory(t *testing.T) (*mocks.MockWidgetFactory, *mocks.MockBoxWidget) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockBox := mocks.NewMockBoxWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockBox).Once()
	mockBox.EXPECT().SetHexpand(true).Once()
	mockBox.EXPECT().SetVexpand(true).Once()

	return mockFactory, mockBox
}

// setupPaneMocks creates mocks needed for AddPane
// Note: The stacked view now uses GestureClick on the titleBar directly instead of wrapping it in a button.
// The GestureClick is added via AddController which we mock to accept any EventController.
func setupPaneMocks(t *testing.T, mockFactory *mocks.MockWidgetFactory, mockBox *mocks.MockBoxWidget) (
	*mocks.MockBoxWidget, *mocks.MockImageWidget, *mocks.MockLabelWidget, *mocks.MockWidget,
) {
	mockTitleBar := mocks.NewMockBoxWidget(t)
	mockFavicon := mocks.NewMockImageWidget(t)
	mockLabel := mocks.NewMockLabelWidget(t)
	mockCloseButton := mocks.NewMockButtonWidget(t)
	mockContainer := mocks.NewMockWidget(t)

	// Title bar creation - now directly used without button wrapper
	mockFactory.EXPECT().NewBox(layout.OrientationHorizontal, 4).Return(mockTitleBar).Once()
	mockTitleBar.EXPECT().AddCssClass("stacked-pane-titlebar").Once()
	mockTitleBar.EXPECT().AddCssClass("stacked-pane-title-clickable").Once()
	mockTitleBar.EXPECT().SetVexpand(false).Once()
	mockTitleBar.EXPECT().SetHexpand(true).Once()

	// Favicon
	mockFactory.EXPECT().NewImage().Return(mockFavicon).Once()
	mockFavicon.EXPECT().SetFromIconName(mock.Anything).Once()
	mockFavicon.EXPECT().SetPixelSize(16).Once()
	mockTitleBar.EXPECT().Append(mockFavicon).Once()

	// Label
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
	mockCloseButton.EXPECT().ConnectClicked(mock.Anything).Return(uint32(2)).Once()

	// Signal disconnection calls GtkWidget() - return nil to skip actual GTK operations in tests
	mockCloseButton.EXPECT().GtkWidget().Return(nil).Maybe()

	// Adding to main box - now titleBar is added directly (no button wrapper)
	mockBox.EXPECT().Append(mockTitleBar).Once()
	mockBox.EXPECT().Append(mockContainer).Once()

	return mockTitleBar, mockFavicon, mockLabel, mockContainer
}

func TestNewStackedView_EmptyStack(t *testing.T) {
	// Arrange
	mockFactory, _ := setupMockFactory(t)

	// Act
	sv := layout.NewStackedView(mockFactory)

	// Assert
	require.NotNil(t, sv)
	assert.Equal(t, 0, sv.Count())
	assert.Equal(t, -1, sv.ActiveIndex())
}

func TestAddPane_SinglePane_BecomesActive(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, mockFavicon, mockLabel, mockContainer := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon
	_ = mockLabel

	// Visibility updates for active pane - titleBar is now directly in box, SetVisible called on it
	mockTitleBar.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)

	// Act
	index := sv.AddPane(ctx, "pane-1", "Test Page", "web-browser-symbolic", mockContainer)

	// Assert
	assert.Equal(t, 0, index)
	assert.Equal(t, 1, sv.Count())
	assert.Equal(t, 0, sv.ActiveIndex())
}

func TestAddPane_MultiplePanes_LastBecomesActive(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	// First pane
	mockTitleBar1, mockFavicon1, mockLabel1, mockContainer1 := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon1
	_ = mockLabel1
	mockTitleBar1.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer1.EXPECT().SetVisible(true).Once()
	mockTitleBar1.EXPECT().AddCssClass("active").Once()

	// Second pane - first pane becomes inactive
	mockTitleBar2, mockFavicon2, mockLabel2, mockContainer2 := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon2
	_ = mockLabel2

	// When second pane is added, update visibility for both
	mockTitleBar1.EXPECT().SetVisible(true).Once() // First pane becomes inactive, show its title bar
	mockContainer1.EXPECT().SetVisible(false).Once()
	mockTitleBar1.EXPECT().RemoveCssClass("active").Once()

	mockTitleBar2.EXPECT().SetVisible(false).Once() // Second pane is active, hide its title bar
	mockContainer2.EXPECT().SetVisible(true).Once()
	mockTitleBar2.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)

	// Act
	sv.AddPane(ctx, "pane-1", "Page 1", "", mockContainer1)
	index := sv.AddPane(ctx, "pane-2", "Page 2", "", mockContainer2)

	// Assert
	assert.Equal(t, 1, index)
	assert.Equal(t, 2, sv.Count())
	assert.Equal(t, 1, sv.ActiveIndex())
}

func TestRemovePane_MiddlePane(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	// Setup 3 panes (simplified - just track the key behaviors)
	containers := make([]*mocks.MockWidget, 3)
	titleBars := make([]*mocks.MockBoxWidget, 3)

	for i := 0; i < 3; i++ {
		titleBars[i], _, _, containers[i] = setupPaneMocks(t, mockFactory, mockBox)
	}

	// Allow any visibility calls during setup and removal
	for i := 0; i < 3; i++ {
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page 1", "", containers[0])
	sv.AddPane(ctx, "pane-2", "Page 2", "", containers[1])
	sv.AddPane(ctx, "pane-3", "Page 3", "", containers[2])

	// Remove middle pane (index 1)
	// The titleBar is now directly in the box (no button wrapper)
	mockBox.EXPECT().Remove(titleBars[1]).Once()
	mockBox.EXPECT().Remove(containers[1]).Once()

	// Act
	err := sv.RemovePane(ctx, 1)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, sv.Count())
}

func TestRemovePane_LastPane_ReturnsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, mockFavicon, mockLabel, mockContainer := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon
	_ = mockLabel

	mockTitleBar.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Only Page", "", mockContainer)

	// Act
	err := sv.RemovePane(ctx, 0)

	// Assert
	require.ErrorIs(t, err, layout.ErrCannotRemoveLastPane)
	assert.Equal(t, 1, sv.Count())
}

func TestRemovePane_EmptyStack_ReturnsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.RemovePane(ctx, 0)

	// Assert
	require.ErrorIs(t, err, layout.ErrStackEmpty)
}

func TestRemovePane_IndexOutOfBounds(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, mockFavicon, mockLabel, mockContainer := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon
	_ = mockLabel

	mockTitleBar.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page", "", mockContainer)

	// Act
	err := sv.RemovePane(ctx, 5)

	// Assert
	require.ErrorIs(t, err, layout.ErrIndexOutOfBounds)
}

func TestSetActive_ValidIndex(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	containers := make([]*mocks.MockWidget, 2)
	titleBars := make([]*mocks.MockBoxWidget, 2)

	for i := 0; i < 2; i++ {
		titleBar, mockFavicon, mockLabel, container := setupPaneMocks(t, mockFactory, mockBox)
		_ = mockFavicon
		_ = mockLabel

		titleBars[i] = titleBar
		containers[i] = container

		titleBars[i].EXPECT().SetVisible(mock.Anything).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page 1", "", containers[0])
	sv.AddPane(ctx, "pane-2", "Page 2", "", containers[1])

	// Act - switch back to first pane
	err := sv.SetActive(ctx, 0)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 0, sv.ActiveIndex())
}

func TestSetActive_OutOfBounds(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, mockFavicon, mockLabel, mockContainer := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon
	_ = mockLabel

	mockTitleBar.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page", "", mockContainer)

	// Act
	err := sv.SetActive(ctx, 5)

	// Assert
	require.ErrorIs(t, err, layout.ErrIndexOutOfBounds)
}

func TestSetActive_EmptyStack(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.SetActive(ctx, 0)

	// Assert
	require.ErrorIs(t, err, layout.ErrStackEmpty)
}

func TestUpdateTitle(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, _, mockLabel, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	mockTitleBar.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Old Title", "", mockContainer)

	mockLabel.EXPECT().SetText("New Title").Once()

	// Act
	err := sv.UpdateTitle(0, "New Title")

	// Assert
	require.NoError(t, err)
}

func TestUpdateTitle_InvalidIndex(t *testing.T) {
	// Arrange
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.UpdateTitle(0, "Title")

	// Assert
	require.ErrorIs(t, err, layout.ErrIndexOutOfBounds)
}

func TestUpdateFavicon(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, mockFavicon, _, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	mockTitleBar.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page", "", mockContainer)

	mockFavicon.EXPECT().SetFromIconName("new-icon").Once()

	// Act
	err := sv.UpdateFavicon(0, "new-icon")

	// Assert
	require.NoError(t, err)
}

func TestGetContainer(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, mockFavicon, mockLabel, mockContainer := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon
	_ = mockLabel

	mockTitleBar.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page", "", mockContainer)

	// Act
	container, err := sv.GetContainer(0)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, mockContainer, container)
}

func TestGetContainer_InvalidIndex(t *testing.T) {
	// Arrange
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	container, err := sv.GetContainer(0)

	// Assert
	require.ErrorIs(t, err, layout.ErrIndexOutOfBounds)
	assert.Nil(t, container)
}

func TestWidget_ReturnsBoxWidget(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	widget := sv.Widget()

	// Assert
	assert.Equal(t, mockBox, widget)
}

func TestNavigateNext_WrapsAround(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	containers := make([]*mocks.MockWidget, 2)
	titleBars := make([]*mocks.MockBoxWidget, 2)

	for i := 0; i < 2; i++ {
		titleBar, mockFavicon, mockLabel, container := setupPaneMocks(t, mockFactory, mockBox)
		_ = mockFavicon
		_ = mockLabel

		titleBars[i] = titleBar
		containers[i] = container

		titleBars[i].EXPECT().SetVisible(mock.Anything).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page 1", "", containers[0])
	sv.AddPane(ctx, "pane-2", "Page 2", "", containers[1])

	// Currently at index 1 (last added)
	assert.Equal(t, 1, sv.ActiveIndex())

	// Act - navigate next should wrap to 0
	err := sv.NavigateNext(ctx)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 0, sv.ActiveIndex())
}

func TestNavigatePrevious_WrapsAround(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	containers := make([]*mocks.MockWidget, 2)
	titleBars := make([]*mocks.MockBoxWidget, 2)

	for i := 0; i < 2; i++ {
		titleBar, mockFavicon, mockLabel, container := setupPaneMocks(t, mockFactory, mockBox)
		_ = mockFavicon
		_ = mockLabel

		titleBars[i] = titleBar
		containers[i] = container

		titleBars[i].EXPECT().SetVisible(mock.Anything).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page 1", "", containers[0])
	sv.AddPane(ctx, "pane-2", "Page 2", "", containers[1])

	// Set to first pane
	sv.SetActive(ctx, 0)

	// Act - navigate previous should wrap to 1
	err := sv.NavigatePrevious(ctx)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, sv.ActiveIndex())
}

func TestNavigateNext_EmptyStack(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.NavigateNext(ctx)

	// Assert
	require.ErrorIs(t, err, layout.ErrStackEmpty)
}

func TestNavigatePrevious_EmptyStack(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.NavigatePrevious(ctx)

	// Assert
	require.ErrorIs(t, err, layout.ErrStackEmpty)
}

func TestSetOnActivate_Callback(t *testing.T) {
	// Arrange
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	callback := func(index int) {
		_ = index // Used to test callback registration
	}

	// Act
	sv.SetOnActivate(callback)

	// We can't easily test the callback being triggered since it requires
	// simulating a button click. The test verifies the method doesn't panic.
	// Integration tests would verify the actual callback invocation.
}

// setupInsertPaneMocks creates mocks needed for InsertPaneAfter with position-aware insertion.
// Returns only the titleBar and container mocks that are needed by test assertions.
func setupInsertPaneMocks(
	t *testing.T,
	mockFactory *mocks.MockWidgetFactory,
	mockBox *mocks.MockBoxWidget,
	siblingContainer layout.Widget,
) (*mocks.MockBoxWidget, *mocks.MockWidget) {
	mockTitleBar := mocks.NewMockBoxWidget(t)
	mockFavicon := mocks.NewMockImageWidget(t)
	mockLabel := mocks.NewMockLabelWidget(t)
	mockCloseButton := mocks.NewMockButtonWidget(t)
	mockContainer := mocks.NewMockWidget(t)

	// Title bar creation - now directly used without button wrapper
	mockFactory.EXPECT().NewBox(layout.OrientationHorizontal, 4).Return(mockTitleBar).Once()
	mockTitleBar.EXPECT().AddCssClass("stacked-pane-titlebar").Once()
	mockTitleBar.EXPECT().AddCssClass("stacked-pane-title-clickable").Once()
	mockTitleBar.EXPECT().SetVexpand(false).Once()
	mockTitleBar.EXPECT().SetHexpand(true).Once()

	// Favicon
	mockFactory.EXPECT().NewImage().Return(mockFavicon).Once()
	mockFavicon.EXPECT().SetFromIconName(mock.Anything).Once()
	mockFavicon.EXPECT().SetPixelSize(16).Once()
	mockTitleBar.EXPECT().Append(mockFavicon).Once()

	// Label
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
	mockCloseButton.EXPECT().ConnectClicked(mock.Anything).Return(uint32(2)).Once()

	// Signal disconnection calls GtkWidget() - return nil to skip actual GTK operations in tests
	mockCloseButton.EXPECT().GtkWidget().Return(nil).Maybe()

	// Position-aware insertion using InsertChildAfter - now using titleBar directly
	if siblingContainer != nil {
		mockBox.EXPECT().InsertChildAfter(mockTitleBar, siblingContainer).Once()
		mockBox.EXPECT().InsertChildAfter(mockContainer, mockTitleBar).Once()
	} else {
		// Insert at beginning using Prepend
		mockBox.EXPECT().Prepend(mockContainer).Once()
		mockBox.EXPECT().Prepend(mockTitleBar).Once()
	}

	return mockTitleBar, mockContainer
}

func TestInsertPaneAfter_AtBeginning(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	// First pane - appended normally
	mockTitleBar1, mockFavicon1, mockLabel1, mockContainer1 := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon1
	_ = mockLabel1
	mockTitleBar1.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer1.EXPECT().SetVisible(true).Once()
	mockTitleBar1.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page 1", "", mockContainer1)

	// Second pane - inserted at beginning (afterIndex=-1)
	mockTitleBar2, mockContainer2 := setupInsertPaneMocks(t, mockFactory, mockBox, nil)

	// Visibility updates
	mockTitleBar1.EXPECT().SetVisible(true).Once() // First pane becomes inactive, show its title bar
	mockContainer1.EXPECT().SetVisible(false).Once()
	mockTitleBar1.EXPECT().RemoveCssClass("active").Once()

	mockTitleBar2.EXPECT().SetVisible(false).Once() // Second pane is active, hide its title bar
	mockContainer2.EXPECT().SetVisible(true).Once()
	mockTitleBar2.EXPECT().AddCssClass("active").Once()

	// Act
	index := sv.InsertPaneAfter(ctx, -1, "pane-0", "Page 0", "", mockContainer2)

	// Assert
	assert.Equal(t, 0, index)
	assert.Equal(t, 2, sv.Count())
	assert.Equal(t, 0, sv.ActiveIndex())
}

func TestInsertPaneAfter_InMiddle(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	// Setup 2 initial panes
	containers := make([]*mocks.MockWidget, 2)
	titleBars := make([]*mocks.MockBoxWidget, 2)

	for i := 0; i < 2; i++ {
		titleBars[i], _, _, containers[i] = setupPaneMocks(t, mockFactory, mockBox)
		titleBars[i].EXPECT().SetVisible(mock.Anything).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page 1", "", containers[0])
	sv.AddPane(ctx, "pane-2", "Page 2", "", containers[1])

	// Set active to first pane
	require.NoError(t, sv.SetActive(ctx, 0))

	// Insert new pane after index 0 (should become index 1)
	mockTitleBar3, mockContainer3 := setupInsertPaneMocks(t, mockFactory, mockBox, containers[0])
	mockTitleBar3.EXPECT().SetVisible(mock.Anything).Maybe()
	mockContainer3.EXPECT().SetVisible(mock.Anything).Maybe()
	mockTitleBar3.EXPECT().AddCssClass("active").Maybe()
	mockTitleBar3.EXPECT().RemoveCssClass("active").Maybe()

	// Act
	index := sv.InsertPaneAfter(ctx, 0, "pane-1.5", "Page 1.5", "", mockContainer3)

	// Assert
	assert.Equal(t, 1, index)
	assert.Equal(t, 3, sv.Count())
	assert.Equal(t, 1, sv.ActiveIndex()) // New pane becomes active
}

func TestInsertPaneAfter_AtEnd(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	// First pane
	mockTitleBar1, mockFavicon1, mockLabel1, mockContainer1 := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon1
	_ = mockLabel1
	mockTitleBar1.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer1.EXPECT().SetVisible(true).Once()
	mockTitleBar1.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page 1", "", mockContainer1)

	// Insert after last pane (afterIndex=0, becomes index 1)
	mockTitleBar2, mockContainer2 := setupInsertPaneMocks(t, mockFactory, mockBox, mockContainer1)

	mockTitleBar1.EXPECT().SetVisible(true).Once() // First pane becomes inactive, show its title bar
	mockContainer1.EXPECT().SetVisible(false).Once()
	mockTitleBar1.EXPECT().RemoveCssClass("active").Once()

	mockTitleBar2.EXPECT().SetVisible(false).Once() // Second pane is active, hide its title bar
	mockContainer2.EXPECT().SetVisible(true).Once()
	mockTitleBar2.EXPECT().AddCssClass("active").Once()

	// Act
	index := sv.InsertPaneAfter(ctx, 0, "pane-2", "Page 2", "", mockContainer2)

	// Assert
	assert.Equal(t, 1, index)
	assert.Equal(t, 2, sv.Count())
	assert.Equal(t, 1, sv.ActiveIndex())
}

func TestInsertPaneAfter_MaintainsOrder(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	// Setup 3 initial panes: A, B, C
	containers := make([]*mocks.MockWidget, 3)
	titleBars := make([]*mocks.MockBoxWidget, 3)

	for i := 0; i < 3; i++ {
		titleBar, mockFavicon, mockLabel, container := setupPaneMocks(t, mockFactory, mockBox)
		_ = mockFavicon
		_ = mockLabel

		titleBars[i] = titleBar
		containers[i] = container

		titleBars[i].EXPECT().SetVisible(mock.Anything).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-a", "A", "", containers[0]) // index 0
	sv.AddPane(ctx, "pane-b", "B", "", containers[1]) // index 1
	sv.AddPane(ctx, "pane-c", "C", "", containers[2]) // index 2

	// Navigate to pane B (index 1)
	require.NoError(t, sv.SetActive(ctx, 1))

	// Insert D after B (should be at index 2, C moves to index 3)
	mockTitleBar4, mockContainer4 := setupInsertPaneMocks(t, mockFactory, mockBox, containers[1])
	mockTitleBar4.EXPECT().SetVisible(mock.Anything).Maybe()
	mockContainer4.EXPECT().SetVisible(mock.Anything).Maybe()
	mockTitleBar4.EXPECT().AddCssClass("active").Maybe()
	mockTitleBar4.EXPECT().RemoveCssClass("active").Maybe()

	// Act
	index := sv.InsertPaneAfter(ctx, 1, "pane-d", "D", "", mockContainer4)

	// Assert: Order should be A(0), B(1), D(2), C(3)
	assert.Equal(t, 2, index)
	assert.Equal(t, 4, sv.Count())
	assert.Equal(t, 2, sv.ActiveIndex()) // D is now active

	// Verify we can still get containers at expected indices
	containerA, _ := sv.GetContainer(0)
	containerB, _ := sv.GetContainer(1)
	containerD, _ := sv.GetContainer(2)
	containerC, _ := sv.GetContainer(3)

	assert.Equal(t, containers[0], containerA)
	assert.Equal(t, containers[1], containerB)
	assert.Equal(t, mockContainer4, containerD)
	assert.Equal(t, containers[2], containerC)
}

func TestInsertPaneAfter_InvalidIndexClamped(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory, mockBox := setupMockFactory(t)

	// First pane
	mockTitleBar1, mockFavicon1, mockLabel1, mockContainer1 := setupPaneMocks(t, mockFactory, mockBox)
	_ = mockFavicon1
	_ = mockLabel1
	mockTitleBar1.EXPECT().SetVisible(false).Once() // Active pane hides its title bar
	mockContainer1.EXPECT().SetVisible(true).Once()
	mockTitleBar1.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane(ctx, "pane-1", "Page 1", "", mockContainer1)

	// Try to insert at invalid index (100) - should clamp to end
	mockTitleBar2, mockContainer2 := setupInsertPaneMocks(t, mockFactory, mockBox, mockContainer1)

	mockTitleBar1.EXPECT().SetVisible(true).Once() // First pane becomes inactive, show its title bar
	mockContainer1.EXPECT().SetVisible(false).Once()
	mockTitleBar1.EXPECT().RemoveCssClass("active").Once()

	mockTitleBar2.EXPECT().SetVisible(false).Once() // Second pane is active, hide its title bar
	mockContainer2.EXPECT().SetVisible(true).Once()
	mockTitleBar2.EXPECT().AddCssClass("active").Once()

	// Act - afterIndex=100 should be clamped to 0 (last valid index)
	index := sv.InsertPaneAfter(ctx, 100, "pane-2", "Page 2", "", mockContainer2)

	// Assert
	assert.Equal(t, 1, index)
	assert.Equal(t, 2, sv.Count())
}

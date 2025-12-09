package layout_test

import (
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
func setupPaneMocks(t *testing.T, mockFactory *mocks.MockWidgetFactory, mockBox *mocks.MockBoxWidget) (
	*mocks.MockBoxWidget, *mocks.MockImageWidget, *mocks.MockLabelWidget, *mocks.MockButtonWidget, *mocks.MockWidget,
) {
	mockTitleBar := mocks.NewMockBoxWidget(t)
	mockFavicon := mocks.NewMockImageWidget(t)
	mockLabel := mocks.NewMockLabelWidget(t)
	mockButton := mocks.NewMockButtonWidget(t)
	mockContainer := mocks.NewMockWidget(t)

	// Title bar creation
	mockFactory.EXPECT().NewBox(layout.OrientationHorizontal, 4).Return(mockTitleBar).Once()
	mockTitleBar.EXPECT().AddCssClass("stacked-pane-titlebar").Once()

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

	// Button wrapping title bar
	mockFactory.EXPECT().NewButton().Return(mockButton).Once()
	mockButton.EXPECT().SetChild(mockTitleBar).Once()
	mockButton.EXPECT().AddCssClass("stacked-pane-title-button").Once()
	mockButton.EXPECT().SetFocusOnClick(false).Once()
	mockButton.EXPECT().ConnectClicked(mock.Anything).Return(uint32(1)).Once()

	// Adding to main box
	mockBox.EXPECT().Append(mockButton).Once()
	mockBox.EXPECT().Append(mockContainer).Once()

	return mockTitleBar, mockFavicon, mockLabel, mockButton, mockContainer
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
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, _, _, _, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	// Visibility updates for active pane
	mockTitleBar.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)

	// Act
	index := sv.AddPane("Test Page", "web-browser-symbolic", mockContainer)

	// Assert
	assert.Equal(t, 0, index)
	assert.Equal(t, 1, sv.Count())
	assert.Equal(t, 0, sv.ActiveIndex())
}

func TestAddPane_MultiplePanes_LastBecomesActive(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)

	// First pane
	mockTitleBar1, _, _, _, mockContainer1 := setupPaneMocks(t, mockFactory, mockBox)
	mockTitleBar1.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer1.EXPECT().SetVisible(true).Once()
	mockTitleBar1.EXPECT().AddCssClass("active").Once()

	// Second pane - first pane becomes inactive
	mockTitleBar2, _, _, _, mockContainer2 := setupPaneMocks(t, mockFactory, mockBox)

	// When second pane is added, update visibility for both
	mockTitleBar1.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer1.EXPECT().SetVisible(false).Once()
	mockTitleBar1.EXPECT().RemoveCssClass("active").Once()

	mockTitleBar2.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer2.EXPECT().SetVisible(true).Once()
	mockTitleBar2.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)

	// Act
	sv.AddPane("Page 1", "", mockContainer1)
	index := sv.AddPane("Page 2", "", mockContainer2)

	// Assert
	assert.Equal(t, 1, index)
	assert.Equal(t, 2, sv.Count())
	assert.Equal(t, 1, sv.ActiveIndex())
}

func TestRemovePane_MiddlePane(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)

	// Setup 3 panes (simplified - just track the key behaviors)
	containers := make([]*mocks.MockWidget, 3)
	titleBars := make([]*mocks.MockBoxWidget, 3)
	buttons := make([]*mocks.MockButtonWidget, 3)

	for i := 0; i < 3; i++ {
		var btn *mocks.MockButtonWidget
		titleBars[i], _, _, btn, containers[i] = setupPaneMocks(t, mockFactory, mockBox)
		buttons[i] = btn
	}

	// Allow any visibility calls during setup and removal
	for i := 0; i < 3; i++ {
		titleBars[i].EXPECT().GetParent().Return(buttons[i]).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
		// Button visibility changes when panes are activated/deactivated
		buttons[i].EXPECT().SetVisible(mock.Anything).Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Page 1", "", containers[0])
	sv.AddPane("Page 2", "", containers[1])
	sv.AddPane("Page 3", "", containers[2])

	// Remove middle pane (index 1)
	// The parent of titleBar is the button widget
	mockBox.EXPECT().Remove(buttons[1]).Once()
	mockBox.EXPECT().Remove(containers[1]).Once()

	// Act
	err := sv.RemovePane(1)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, sv.Count())
}

func TestRemovePane_LastPane_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, _, _, _, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	mockTitleBar.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Only Page", "", mockContainer)

	// Act
	err := sv.RemovePane(0)

	// Assert
	assert.ErrorIs(t, err, layout.ErrCannotRemoveLastPane)
	assert.Equal(t, 1, sv.Count())
}

func TestRemovePane_EmptyStack_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.RemovePane(0)

	// Assert
	assert.ErrorIs(t, err, layout.ErrStackEmpty)
}

func TestRemovePane_IndexOutOfBounds(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, _, _, _, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	mockTitleBar.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Page", "", mockContainer)

	// Act
	err := sv.RemovePane(5)

	// Assert
	assert.ErrorIs(t, err, layout.ErrIndexOutOfBounds)
}

func TestSetActive_ValidIndex(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)

	containers := make([]*mocks.MockWidget, 2)
	titleBars := make([]*mocks.MockBoxWidget, 2)

	for i := 0; i < 2; i++ {
		titleBars[i], _, _, _, containers[i] = setupPaneMocks(t, mockFactory, mockBox)
		titleBars[i].EXPECT().GetParent().Return(nil).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Page 1", "", containers[0])
	sv.AddPane("Page 2", "", containers[1])

	// Act - switch back to first pane
	err := sv.SetActive(0)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 0, sv.ActiveIndex())
}

func TestSetActive_OutOfBounds(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, _, _, _, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	mockTitleBar.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Page", "", mockContainer)

	// Act
	err := sv.SetActive(5)

	// Assert
	assert.ErrorIs(t, err, layout.ErrIndexOutOfBounds)
}

func TestSetActive_EmptyStack(t *testing.T) {
	// Arrange
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.SetActive(0)

	// Assert
	assert.ErrorIs(t, err, layout.ErrStackEmpty)
}

func TestUpdateTitle(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, _, mockLabel, _, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	mockTitleBar.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Old Title", "", mockContainer)

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
	assert.ErrorIs(t, err, layout.ErrIndexOutOfBounds)
}

func TestUpdateFavicon(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, mockFavicon, _, _, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	mockTitleBar.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Page", "", mockContainer)

	mockFavicon.EXPECT().SetFromIconName("new-icon").Once()

	// Act
	err := sv.UpdateFavicon(0, "new-icon")

	// Assert
	require.NoError(t, err)
}

func TestGetContainer(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)
	mockTitleBar, _, _, _, mockContainer := setupPaneMocks(t, mockFactory, mockBox)

	mockTitleBar.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Page", "", mockContainer)

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
	assert.ErrorIs(t, err, layout.ErrIndexOutOfBounds)
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
	mockFactory, mockBox := setupMockFactory(t)

	containers := make([]*mocks.MockWidget, 2)
	titleBars := make([]*mocks.MockBoxWidget, 2)

	for i := 0; i < 2; i++ {
		titleBars[i], _, _, _, containers[i] = setupPaneMocks(t, mockFactory, mockBox)
		titleBars[i].EXPECT().GetParent().Return(nil).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Page 1", "", containers[0])
	sv.AddPane("Page 2", "", containers[1])

	// Currently at index 1 (last added)
	assert.Equal(t, 1, sv.ActiveIndex())

	// Act - navigate next should wrap to 0
	err := sv.NavigateNext()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 0, sv.ActiveIndex())
}

func TestNavigatePrevious_WrapsAround(t *testing.T) {
	// Arrange
	mockFactory, mockBox := setupMockFactory(t)

	containers := make([]*mocks.MockWidget, 2)
	titleBars := make([]*mocks.MockBoxWidget, 2)

	for i := 0; i < 2; i++ {
		titleBars[i], _, _, _, containers[i] = setupPaneMocks(t, mockFactory, mockBox)
		titleBars[i].EXPECT().GetParent().Return(nil).Maybe()
		containers[i].EXPECT().SetVisible(mock.Anything).Maybe()
		titleBars[i].EXPECT().AddCssClass("active").Maybe()
		titleBars[i].EXPECT().RemoveCssClass("active").Maybe()
	}

	sv := layout.NewStackedView(mockFactory)
	sv.AddPane("Page 1", "", containers[0])
	sv.AddPane("Page 2", "", containers[1])

	// Set to first pane
	sv.SetActive(0)

	// Act - navigate previous should wrap to 1
	err := sv.NavigatePrevious()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, sv.ActiveIndex())
}

func TestNavigateNext_EmptyStack(t *testing.T) {
	// Arrange
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.NavigateNext()

	// Assert
	assert.ErrorIs(t, err, layout.ErrStackEmpty)
}

func TestNavigatePrevious_EmptyStack(t *testing.T) {
	// Arrange
	mockFactory, _ := setupMockFactory(t)
	sv := layout.NewStackedView(mockFactory)

	// Act
	err := sv.NavigatePrevious()

	// Assert
	assert.ErrorIs(t, err, layout.ErrStackEmpty)
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

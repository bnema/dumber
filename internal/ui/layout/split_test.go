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

// setupSplitViewMocks configures the common mock expectations for NewSplitView.
// Since the mock paned returns 0 for GetAllocatedWidth/Height, ApplyRatio will
// return false and trigger the ConnectMap and AddTickCallback registrations.
func setupSplitViewMocks(mockPaned *mocks.MockPanedWidget, orientation layout.Orientation) {
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	// Note: SetShrinkStartChild/SetShrinkEndChild are NOT called - allowing shrink
	// enables GTK to respect the 50/50 ratio even if children have larger natural sizes

	// ApplyRatio is called immediately, which calls GetAllocatedWidth/Height
	if orientation == layout.OrientationHorizontal {
		mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	} else {
		mockPaned.EXPECT().GetAllocatedHeight().Return(0).Once()
	}

	// Since allocation is 0, ApplyRatio returns false and callbacks are registered
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()
}

func TestNewSplitView_Horizontal(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockStartChild := mocks.NewMockWidget(t)
	mockEndChild := mocks.NewMockWidget(t)

	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)
	mockPaned.EXPECT().SetStartChild(mockStartChild).Once()
	mockPaned.EXPECT().SetEndChild(mockEndChild).Once()

	// Act
	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, mockStartChild, mockEndChild, 0.5)

	// Assert
	require.NotNil(t, sv)
	assert.Equal(t, layout.OrientationHorizontal, sv.Orientation())
	assert.Equal(t, 0.5, sv.GetRatio())
	assert.Equal(t, mockStartChild, sv.StartChild())
	assert.Equal(t, mockEndChild, sv.EndChild())
}

func TestNewSplitView_Vertical(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockStartChild := mocks.NewMockWidget(t)
	mockEndChild := mocks.NewMockWidget(t)

	mockFactory.EXPECT().NewPaned(layout.OrientationVertical).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationVertical)
	mockPaned.EXPECT().SetStartChild(mockStartChild).Once()
	mockPaned.EXPECT().SetEndChild(mockEndChild).Once()

	// Act
	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationVertical, mockStartChild, mockEndChild, 0.5)

	// Assert
	require.NotNil(t, sv)
	assert.Equal(t, layout.OrientationVertical, sv.Orientation())
}

func TestNewSplitView_NilChildren(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)

	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)
	// SetStartChild and SetEndChild should NOT be called when children are nil

	// Act
	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, 0.5)

	// Assert
	require.NotNil(t, sv)
	assert.Nil(t, sv.StartChild())
	assert.Nil(t, sv.EndChild())
}

func TestSetRatio_ValidRange(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"zero", 0.0, 0.0},
		{"one", 1.0, 1.0},
		{"middle", 0.5, 0.5},
		{"quarter", 0.25, 0.25},
		{"three_quarters", 0.75, 0.75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// Arrange
			mockFactory := mocks.NewMockWidgetFactory(t)
			mockPaned := mocks.NewMockPanedWidget(t)

			mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
			setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)

			sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, 0.5)

			// Act
			sv.SetRatio(tt.input)

			// Assert
			assert.Equal(t, tt.expected, sv.GetRatio())
		})
	}
}

func TestSetRatio_OutOfRange_Clamped(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"negative", -0.5, 0.0},
		{"very_negative", -100.0, 0.0},
		{"greater_than_one", 1.5, 1.0},
		{"much_greater", 100.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// Arrange
			mockFactory := mocks.NewMockWidgetFactory(t)
			mockPaned := mocks.NewMockPanedWidget(t)

			mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
			setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)

			sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, 0.5)

			// Act
			sv.SetRatio(tt.input)

			// Assert
			assert.Equal(t, tt.expected, sv.GetRatio())
		})
	}
}

func TestSwapStart_ReplacesChild(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockOldChild := mocks.NewMockWidget(t)
	mockNewChild := mocks.NewMockWidget(t)

	mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)
	mockPaned.EXPECT().SetStartChild(mockOldChild).Once()

	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, mockOldChild, nil, 0.5)

	// Expect swap operations
	mockPaned.EXPECT().SetStartChild(nil).Once()          // Remove old
	mockPaned.EXPECT().SetStartChild(mockNewChild).Once() // Add new

	// Act
	sv.SwapStart(mockNewChild)

	// Assert
	assert.Equal(t, mockNewChild, sv.StartChild())
}

func TestSwapEnd_ReplacesChild(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockOldChild := mocks.NewMockWidget(t)
	mockNewChild := mocks.NewMockWidget(t)

	mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)
	mockPaned.EXPECT().SetEndChild(mockOldChild).Once()

	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, mockOldChild, 0.5)

	// Expect swap operations
	mockPaned.EXPECT().SetEndChild(nil).Once()          // Remove old
	mockPaned.EXPECT().SetEndChild(mockNewChild).Once() // Add new

	// Act
	sv.SwapEnd(mockNewChild)

	// Assert
	assert.Equal(t, mockNewChild, sv.EndChild())
}

func TestSwapStart_FromNil(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockNewChild := mocks.NewMockWidget(t)

	mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)

	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, 0.5)

	// Expect only adding new child (no removal since old was nil)
	mockPaned.EXPECT().SetStartChild(mockNewChild).Once()

	// Act
	sv.SwapStart(mockNewChild)

	// Assert
	assert.Equal(t, mockNewChild, sv.StartChild())
}

func TestSwapEnd_ToNil(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockOldChild := mocks.NewMockWidget(t)

	mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)
	mockPaned.EXPECT().SetEndChild(mockOldChild).Once()

	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, mockOldChild, 0.5)

	// Expect removal (SetEndChild(nil) called for clearing old)
	mockPaned.EXPECT().SetEndChild(nil).Once()

	// Act
	sv.SwapEnd(nil)

	// Assert
	assert.Nil(t, sv.EndChild())
}

func TestWidget_ReturnsPanedWidget(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)

	mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)

	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, 0.5)

	// Act
	widget := sv.Widget()

	// Assert
	assert.Equal(t, mockPaned, widget)
}

func TestSetPosition_DelegatesToPaned(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)

	mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)

	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, 0.5)

	mockPaned.EXPECT().SetPosition(400).Once()

	// Act
	sv.SetPosition(400)

	// Assert - verified by mock expectations
}

func TestGetPosition_DelegatesToPaned(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)

	mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)

	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, 0.5)

	mockPaned.EXPECT().GetPosition().Return(400).Once()

	// Act
	pos := sv.GetPosition()

	// Assert
	assert.Equal(t, 400, pos)
}

func TestSetWideHandle_DelegatesToPaned(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)

	mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
	setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)

	sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, 0.5)

	mockPaned.EXPECT().SetWideHandle(true).Once()

	// Act
	sv.SetWideHandle(true)

	// Assert - verified by mock expectations
}

func TestNewSplitView_InitialRatio_Clamped(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"negative_clamped_to_zero", -0.5, 0.0},
		{"greater_than_one_clamped", 1.5, 1.0},
		{"valid_preserved", 0.3, 0.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// Arrange
			mockFactory := mocks.NewMockWidgetFactory(t)
			mockPaned := mocks.NewMockPanedWidget(t)

			mockFactory.EXPECT().NewPaned(mock.Anything).Return(mockPaned).Once()
			setupSplitViewMocks(mockPaned, layout.OrientationHorizontal)

			// Act
			sv := layout.NewSplitView(ctx, mockFactory, layout.OrientationHorizontal, nil, nil, tt.input)

			// Assert
			assert.Equal(t, tt.expected, sv.GetRatio())
		})
	}
}

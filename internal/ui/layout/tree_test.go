package layout_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

func setupStackedLeafMocks(t *testing.T, mockFactory *mocks.MockWidgetFactory) (*mocks.MockBoxWidget, *mocks.MockWidget) {
	mockStackBox := mocks.NewMockBoxWidget(t)
	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockStackBox).Once()
	mockStackBox.EXPECT().SetHexpand(true).Once()
	mockStackBox.EXPECT().SetVexpand(true).Once()

	mockTitleBar, mockFavicon, mockOverlay, mockSpinner, mockContainer := setupPaneMocks(t, mockFactory, mockStackBox)
	_ = mockFavicon
	_ = mockOverlay
	_ = mockSpinner
	mockTitleBar.EXPECT().GetParent().Return(nil).Maybe()
	mockContainer.EXPECT().SetVisible(true).Once()
	mockTitleBar.EXPECT().AddCssClass("active").Once()

	return mockStackBox, mockContainer
}

func TestNewTreeRenderer_CreatesRenderer(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)

	// Act
	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Assert
	require.NotNil(t, renderer)
	assert.Equal(t, 0, renderer.NodeCount())
}

func TestBuild_NilRoot_ReturnsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Act
	widget, err := renderer.Build(ctx, nil)

	// Assert
	assert.Nil(t, widget)
	assert.ErrorIs(t, err, layout.ErrNilRoot)
}

func TestBuild_SingleLeafPane(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockStackBox, mockContainer := setupStackedLeafMocks(t, mockFactory)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{
		ID:   "node-1",
		Pane: pane,
	}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockContainer).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Act
	widget, err := renderer.Build(ctx, node)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, mockStackBox, widget)
	assert.Equal(t, 1, renderer.NodeCount())

	// Verify lookup works
	assert.Equal(t, mockStackBox, renderer.Lookup("node-1"))
}

func TestBuild_HorizontalSplit(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockLeftStackBox, mockLeftContainer := setupStackedLeafMocks(t, mockFactory)
	mockRightStackBox, mockRightContainer := setupStackedLeafMocks(t, mockFactory)

	leftPane := entity.NewPane(entity.PaneID("pane-left"))
	rightPane := entity.NewPane(entity.PaneID("pane-right"))

	leftNode := &entity.PaneNode{ID: "node-left", Pane: leftPane}
	rightNode := &entity.PaneNode{ID: "node-right", Pane: rightPane}

	splitNode := &entity.PaneNode{
		ID:         "node-split",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{leftNode, rightNode},
	}

	mockPaneViewFactory.EXPECT().CreatePaneView(leftNode).Return(mockLeftContainer).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(rightNode).Return(mockRightContainer).Once()

	// SplitView creation expectations
	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetVisible(true).Once()
	mockLeftStackBox.EXPECT().SetVisible(true).Once()
	mockRightStackBox.EXPECT().SetVisible(true).Once()
	mockPaned.EXPECT().SetStartChild(mockLeftStackBox).Once()
	mockPaned.EXPECT().SetEndChild(mockRightStackBox).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Act
	widget, err := renderer.Build(ctx, splitNode)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, widget)
	assert.Equal(t, 3, renderer.NodeCount()) // split + 2 leaves
}

func TestBuild_VerticalSplit(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockTopStackBox, mockTopContainer := setupStackedLeafMocks(t, mockFactory)
	mockBottomStackBox, mockBottomContainer := setupStackedLeafMocks(t, mockFactory)

	topPane := entity.NewPane(entity.PaneID("pane-top"))
	bottomPane := entity.NewPane(entity.PaneID("pane-bottom"))

	topNode := &entity.PaneNode{ID: "node-top", Pane: topPane}
	bottomNode := &entity.PaneNode{ID: "node-bottom", Pane: bottomPane}

	splitNode := &entity.PaneNode{
		ID:         "node-split",
		SplitDir:   entity.SplitVertical,
		SplitRatio: 0.3,
		Children:   []*entity.PaneNode{topNode, bottomNode},
	}

	mockPaneViewFactory.EXPECT().CreatePaneView(topNode).Return(mockTopContainer).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(bottomNode).Return(mockBottomContainer).Once()

	// SplitView creation expectations - should be vertical
	mockFactory.EXPECT().NewPaned(layout.OrientationVertical).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetVisible(true).Once()
	mockTopStackBox.EXPECT().SetVisible(true).Once()
	mockBottomStackBox.EXPECT().SetVisible(true).Once()
	mockPaned.EXPECT().SetStartChild(mockTopStackBox).Once()
	mockPaned.EXPECT().SetEndChild(mockBottomStackBox).Once()
	mockPaned.EXPECT().GetAllocatedHeight().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Act
	widget, err := renderer.Build(ctx, splitNode)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, widget)
}

func TestBuild_NestedSplits(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockOuterPaned := mocks.NewMockPanedWidget(t)
	mockInnerPaned := mocks.NewMockPanedWidget(t)
	mockStackBox1, mockContainer1 := setupStackedLeafMocks(t, mockFactory)
	mockStackBox2, mockContainer2 := setupStackedLeafMocks(t, mockFactory)
	mockStackBox3, mockContainer3 := setupStackedLeafMocks(t, mockFactory)

	// Create tree structure:
	//       outer (horizontal)
	//      /                \
	//   inner (vertical)    pane3
	//   /           \
	// pane1       pane2

	pane1 := entity.NewPane(entity.PaneID("pane-1"))
	pane2 := entity.NewPane(entity.PaneID("pane-2"))
	pane3 := entity.NewPane(entity.PaneID("pane-3"))

	node1 := &entity.PaneNode{ID: "node-1", Pane: pane1}
	node2 := &entity.PaneNode{ID: "node-2", Pane: pane2}
	node3 := &entity.PaneNode{ID: "node-3", Pane: pane3}

	innerSplit := &entity.PaneNode{
		ID:         "inner-split",
		SplitDir:   entity.SplitVertical,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{node1, node2},
	}

	outerSplit := &entity.PaneNode{
		ID:         "outer-split",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.6,
		Children:   []*entity.PaneNode{innerSplit, node3},
	}

	// Setup expectations for leaf nodes
	mockPaneViewFactory.EXPECT().CreatePaneView(node1).Return(mockContainer1).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node2).Return(mockContainer2).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node3).Return(mockContainer3).Once()

	// Inner split (vertical) is created first
	mockFactory.EXPECT().NewPaned(layout.OrientationVertical).Return(mockInnerPaned).Once()
	mockInnerPaned.EXPECT().SetResizeStartChild(true).Once()
	mockInnerPaned.EXPECT().SetResizeEndChild(true).Once()
	mockInnerPaned.EXPECT().SetVisible(true).Once()
	mockStackBox1.EXPECT().SetVisible(true).Once()
	mockStackBox2.EXPECT().SetVisible(true).Once()
	mockInnerPaned.EXPECT().SetStartChild(mockStackBox1).Once()
	mockInnerPaned.EXPECT().SetEndChild(mockStackBox2).Once()
	mockInnerPaned.EXPECT().GetAllocatedHeight().Return(0).Once()
	mockInnerPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockInnerPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	// Outer split (horizontal) gets inner paned and pane3
	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockOuterPaned).Once()
	mockOuterPaned.EXPECT().SetResizeStartChild(true).Once()
	mockOuterPaned.EXPECT().SetResizeEndChild(true).Once()
	mockOuterPaned.EXPECT().SetVisible(true).Once()
	mockInnerPaned.EXPECT().SetVisible(true).Once()
	mockStackBox3.EXPECT().SetVisible(true).Once()
	mockOuterPaned.EXPECT().SetStartChild(mockInnerPaned).Once()
	mockOuterPaned.EXPECT().SetEndChild(mockStackBox3).Once()
	mockOuterPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockOuterPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockOuterPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Act
	widget, err := renderer.Build(ctx, outerSplit)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, widget)
	assert.Equal(t, 5, renderer.NodeCount()) // 2 splits + 3 leaves
}

// Note: TestBuild_StackedPanes is omitted because StackedView has complex internal
// widget creation that makes mocking unwieldy. StackedView is thoroughly tested
// in stacked_test.go. TreeRenderer's stacked rendering is verified implicitly
// through integration tests.

func TestLookup_ExistingNode(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockStackBox, mockContainer := setupStackedLeafMocks(t, mockFactory)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "test-node-id", Pane: pane}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockContainer).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)
	_, _ = renderer.Build(ctx, node)

	// Act
	result := renderer.Lookup("test-node-id")

	// Assert
	assert.Equal(t, mockStackBox, result)
}

func TestLookup_NonExistentNode(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	_, mockContainer := setupStackedLeafMocks(t, mockFactory)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockContainer).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)
	_, _ = renderer.Build(ctx, node)

	// Act
	result := renderer.Lookup("non-existent-id")

	// Assert
	assert.Nil(t, result)
}

func TestLookupNode_Existing(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockStackBox, mockContainer := setupStackedLeafMocks(t, mockFactory)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockContainer).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)
	_, _ = renderer.Build(ctx, node)

	// Act
	widget, found := renderer.LookupNode("node-1")

	// Assert
	assert.True(t, found)
	assert.Equal(t, mockStackBox, widget)
}

func TestLookupNode_NonExistent(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Act
	widget, found := renderer.LookupNode("non-existent")

	// Assert
	assert.False(t, found)
	assert.Nil(t, widget)
}

func TestClear_RemovesAllMappings(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	_, mockContainer := setupStackedLeafMocks(t, mockFactory)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockContainer).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)
	_, _ = renderer.Build(ctx, node)

	require.Equal(t, 1, renderer.NodeCount())

	// Act
	renderer.Clear()

	// Assert
	assert.Equal(t, 0, renderer.NodeCount())
	assert.Nil(t, renderer.Lookup("node-1"))
}

func TestGetNodeIDs_ReturnsAllIDs(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	ctx := context.Background()
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockLeftStackBox, mockLeftContainer := setupStackedLeafMocks(t, mockFactory)
	mockRightStackBox, mockRightContainer := setupStackedLeafMocks(t, mockFactory)

	pane1 := entity.NewPane(entity.PaneID("pane-1"))
	pane2 := entity.NewPane(entity.PaneID("pane-2"))

	node1 := &entity.PaneNode{ID: "left-node", Pane: pane1}
	node2 := &entity.PaneNode{ID: "right-node", Pane: pane2}

	splitNode := &entity.PaneNode{
		ID:         "split-node",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{node1, node2},
	}

	mockPaneViewFactory.EXPECT().CreatePaneView(node1).Return(mockLeftContainer).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node2).Return(mockRightContainer).Once()

	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetVisible(true).Once()
	mockLeftStackBox.EXPECT().SetVisible(true).Once()
	mockRightStackBox.EXPECT().SetVisible(true).Once()
	mockPaned.EXPECT().SetStartChild(mockLeftStackBox).Once()
	mockPaned.EXPECT().SetEndChild(mockRightStackBox).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)
	_, _ = renderer.Build(ctx, splitNode)

	// Act
	ids := renderer.GetNodeIDs()

	// Assert
	assert.Len(t, ids, 3)
	assert.Contains(t, ids, "split-node")
	assert.Contains(t, ids, "left-node")
	assert.Contains(t, ids, "right-node")
}

func TestBuild_ClearsPreviousMappings(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockOldStackBox, mockOldContainer := setupStackedLeafMocks(t, mockFactory)
	mockNewStackBox, mockNewContainer := setupStackedLeafMocks(t, mockFactory)

	pane1 := entity.NewPane(entity.PaneID("pane-1"))
	node1 := &entity.PaneNode{ID: "old-node", Pane: pane1}

	pane2 := entity.NewPane(entity.PaneID("pane-2"))
	node2 := &entity.PaneNode{ID: "new-node", Pane: pane2}

	mockPaneViewFactory.EXPECT().CreatePaneView(node1).Return(mockOldContainer).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node2).Return(mockNewContainer).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// First build
	_, _ = renderer.Build(ctx, node1)
	require.Equal(t, 1, renderer.NodeCount())
	require.Equal(t, mockOldStackBox, renderer.Lookup("old-node"))

	// Act - second build should clear old mappings
	_, _ = renderer.Build(ctx, node2)

	// Assert
	assert.Equal(t, 1, renderer.NodeCount())
	assert.Nil(t, renderer.Lookup("old-node"))
	assert.Equal(t, mockNewStackBox, renderer.Lookup("new-node"))
}

func TestBuild_NilPaneViewFactory_ReturnsNilForLeaves(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	renderer := layout.NewTreeRenderer(ctx, mockFactory, nil)

	// Act
	widget, err := renderer.Build(ctx, node)

	// Assert
	require.NoError(t, err)
	assert.Nil(t, widget) // nil because paneViewFactory is nil
}

func TestBuild_EmptyNodeID_NotStored(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	_, mockContainer := setupStackedLeafMocks(t, mockFactory)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "", Pane: pane} // Empty ID

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockContainer).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Act
	widget, err := renderer.Build(ctx, node)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, widget)
	assert.Equal(t, 0, renderer.NodeCount()) // Empty ID nodes aren't stored
}

func TestUpdateSplitRatio_ExistingNode(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	ctx := context.Background()
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockLeftStackBox, mockLeftContainer := setupStackedLeafMocks(t, mockFactory)
	mockRightStackBox, mockRightContainer := setupStackedLeafMocks(t, mockFactory)

	pane1 := entity.NewPane(entity.PaneID("pane-1"))
	pane2 := entity.NewPane(entity.PaneID("pane-2"))

	node1 := &entity.PaneNode{ID: "node-1", Pane: pane1}
	node2 := &entity.PaneNode{ID: "node-2", Pane: pane2}

	splitNode := &entity.PaneNode{
		ID:         "split-node",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{node1, node2},
	}

	mockPaneViewFactory.EXPECT().CreatePaneView(node1).Return(mockLeftContainer).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node2).Return(mockRightContainer).Once()

	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetVisible(true).Once()
	mockLeftStackBox.EXPECT().SetVisible(true).Once()
	mockRightStackBox.EXPECT().SetVisible(true).Once()
	mockPaned.EXPECT().SetStartChild(mockLeftStackBox).Once()
	mockPaned.EXPECT().SetEndChild(mockRightStackBox).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Twice()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)
	_, _ = renderer.Build(ctx, splitNode)

	// Act
	err := renderer.UpdateSplitRatio("split-node", 0.7)

	// Assert
	assert.NoError(t, err)
}

func TestUpdateSplitRatio_NonExistentNode(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	ctx := context.Background()
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)

	renderer := layout.NewTreeRenderer(ctx, mockFactory, mockPaneViewFactory)

	// Act
	err := renderer.UpdateSplitRatio("non-existent", 0.7)

	// Assert
	assert.ErrorIs(t, err, layout.ErrNodeNotFound)
}

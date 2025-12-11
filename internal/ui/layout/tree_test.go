package layout_test

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

func TestNewTreeRenderer_CreatesRenderer(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)

	// Act
	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Assert
	require.NotNil(t, renderer)
	assert.Equal(t, 0, renderer.NodeCount())
}

func TestBuild_NilRoot_ReturnsError(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Act
	widget, err := renderer.Build(nil)

	// Assert
	assert.Nil(t, widget)
	assert.ErrorIs(t, err, layout.ErrNilRoot)
}

func TestBuild_SingleLeafPane(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockWidget := mocks.NewMockWidget(t)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{
		ID:   "node-1",
		Pane: pane,
	}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockWidget).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Act
	widget, err := renderer.Build(node)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, mockWidget, widget)
	assert.Equal(t, 1, renderer.NodeCount())

	// Verify lookup works
	assert.Equal(t, mockWidget, renderer.Lookup("node-1"))
}

func TestBuild_HorizontalSplit(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockLeftWidget := mocks.NewMockWidget(t)
	mockRightWidget := mocks.NewMockWidget(t)

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

	mockPaneViewFactory.EXPECT().CreatePaneView(leftNode).Return(mockLeftWidget).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(rightNode).Return(mockRightWidget).Once()

	// SplitView creation expectations
	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetShrinkStartChild(false).Once()
	mockPaned.EXPECT().SetShrinkEndChild(false).Once()
	mockPaned.EXPECT().SetStartChild(mockLeftWidget).Once()
	mockPaned.EXPECT().SetEndChild(mockRightWidget).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Act
	widget, err := renderer.Build(splitNode)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, widget)
	assert.Equal(t, 3, renderer.NodeCount()) // split + 2 leaves
}

func TestBuild_VerticalSplit(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockTopWidget := mocks.NewMockWidget(t)
	mockBottomWidget := mocks.NewMockWidget(t)

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

	mockPaneViewFactory.EXPECT().CreatePaneView(topNode).Return(mockTopWidget).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(bottomNode).Return(mockBottomWidget).Once()

	// SplitView creation expectations - should be vertical
	mockFactory.EXPECT().NewPaned(layout.OrientationVertical).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetShrinkStartChild(false).Once()
	mockPaned.EXPECT().SetShrinkEndChild(false).Once()
	mockPaned.EXPECT().SetStartChild(mockTopWidget).Once()
	mockPaned.EXPECT().SetEndChild(mockBottomWidget).Once()
	mockPaned.EXPECT().GetAllocatedHeight().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Act
	widget, err := renderer.Build(splitNode)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, widget)
}

func TestBuild_NestedSplits(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockOuterPaned := mocks.NewMockPanedWidget(t)
	mockInnerPaned := mocks.NewMockPanedWidget(t)
	mockWidget1 := mocks.NewMockWidget(t)
	mockWidget2 := mocks.NewMockWidget(t)
	mockWidget3 := mocks.NewMockWidget(t)

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
	mockPaneViewFactory.EXPECT().CreatePaneView(node1).Return(mockWidget1).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node2).Return(mockWidget2).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node3).Return(mockWidget3).Once()

	// Inner split (vertical) is created first
	mockFactory.EXPECT().NewPaned(layout.OrientationVertical).Return(mockInnerPaned).Once()
	mockInnerPaned.EXPECT().SetResizeStartChild(true).Once()
	mockInnerPaned.EXPECT().SetResizeEndChild(true).Once()
	mockInnerPaned.EXPECT().SetShrinkStartChild(false).Once()
	mockInnerPaned.EXPECT().SetShrinkEndChild(false).Once()
	mockInnerPaned.EXPECT().SetStartChild(mockWidget1).Once()
	mockInnerPaned.EXPECT().SetEndChild(mockWidget2).Once()
	mockInnerPaned.EXPECT().GetAllocatedHeight().Return(0).Once()
	mockInnerPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockInnerPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	// Outer split (horizontal) gets inner paned and pane3
	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockOuterPaned).Once()
	mockOuterPaned.EXPECT().SetResizeStartChild(true).Once()
	mockOuterPaned.EXPECT().SetResizeEndChild(true).Once()
	mockOuterPaned.EXPECT().SetShrinkStartChild(false).Once()
	mockOuterPaned.EXPECT().SetShrinkEndChild(false).Once()
	mockOuterPaned.EXPECT().SetStartChild(mockInnerPaned).Once()
	mockOuterPaned.EXPECT().SetEndChild(mockWidget3).Once()
	mockOuterPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockOuterPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockOuterPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Act
	widget, err := renderer.Build(outerSplit)

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
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockWidget := mocks.NewMockWidget(t)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "test-node-id", Pane: pane}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockWidget).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())
	_, _ = renderer.Build(node)

	// Act
	result := renderer.Lookup("test-node-id")

	// Assert
	assert.Equal(t, mockWidget, result)
}

func TestLookup_NonExistentNode(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockWidget := mocks.NewMockWidget(t)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockWidget).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())
	_, _ = renderer.Build(node)

	// Act
	result := renderer.Lookup("non-existent-id")

	// Assert
	assert.Nil(t, result)
}

func TestLookupNode_Existing(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockWidget := mocks.NewMockWidget(t)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockWidget).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())
	_, _ = renderer.Build(node)

	// Act
	widget, found := renderer.LookupNode("node-1")

	// Assert
	assert.True(t, found)
	assert.Equal(t, mockWidget, widget)
}

func TestLookupNode_NonExistent(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Act
	widget, found := renderer.LookupNode("non-existent")

	// Assert
	assert.False(t, found)
	assert.Nil(t, widget)
}

func TestClear_RemovesAllMappings(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockWidget := mocks.NewMockWidget(t)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockWidget).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())
	_, _ = renderer.Build(node)

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
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockWidget1 := mocks.NewMockWidget(t)
	mockWidget2 := mocks.NewMockWidget(t)

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

	mockPaneViewFactory.EXPECT().CreatePaneView(node1).Return(mockWidget1).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node2).Return(mockWidget2).Once()

	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetShrinkStartChild(false).Once()
	mockPaned.EXPECT().SetShrinkEndChild(false).Once()
	mockPaned.EXPECT().SetStartChild(mockWidget1).Once()
	mockPaned.EXPECT().SetEndChild(mockWidget2).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())
	_, _ = renderer.Build(splitNode)

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
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockWidget1 := mocks.NewMockWidget(t)
	mockWidget2 := mocks.NewMockWidget(t)

	pane1 := entity.NewPane(entity.PaneID("pane-1"))
	node1 := &entity.PaneNode{ID: "old-node", Pane: pane1}

	pane2 := entity.NewPane(entity.PaneID("pane-2"))
	node2 := &entity.PaneNode{ID: "new-node", Pane: pane2}

	mockPaneViewFactory.EXPECT().CreatePaneView(node1).Return(mockWidget1).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node2).Return(mockWidget2).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// First build
	_, _ = renderer.Build(node1)
	require.Equal(t, 1, renderer.NodeCount())
	require.NotNil(t, renderer.Lookup("old-node"))

	// Act - second build should clear old mappings
	_, _ = renderer.Build(node2)

	// Assert
	assert.Equal(t, 1, renderer.NodeCount())
	assert.Nil(t, renderer.Lookup("old-node"))
	assert.NotNil(t, renderer.Lookup("new-node"))
}

func TestBuild_NilPaneViewFactory_ReturnsNilForLeaves(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "node-1", Pane: pane}

	renderer := layout.NewTreeRenderer(mockFactory, nil, zerolog.Nop())

	// Act
	widget, err := renderer.Build(node)

	// Assert
	require.NoError(t, err)
	assert.Nil(t, widget) // nil because paneViewFactory is nil
}

func TestBuild_EmptyNodeID_NotStored(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockWidget := mocks.NewMockWidget(t)

	pane := entity.NewPane(entity.PaneID("pane-1"))
	node := &entity.PaneNode{ID: "", Pane: pane} // Empty ID

	mockPaneViewFactory.EXPECT().CreatePaneView(node).Return(mockWidget).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Act
	widget, err := renderer.Build(node)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, widget)
	assert.Equal(t, 0, renderer.NodeCount()) // Empty ID nodes aren't stored
}

func TestUpdateSplitRatio_ExistingNode(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)
	mockPaned := mocks.NewMockPanedWidget(t)
	mockWidget1 := mocks.NewMockWidget(t)
	mockWidget2 := mocks.NewMockWidget(t)

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

	mockPaneViewFactory.EXPECT().CreatePaneView(node1).Return(mockWidget1).Once()
	mockPaneViewFactory.EXPECT().CreatePaneView(node2).Return(mockWidget2).Once()

	mockFactory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(mockPaned).Once()
	mockPaned.EXPECT().SetResizeStartChild(true).Once()
	mockPaned.EXPECT().SetResizeEndChild(true).Once()
	mockPaned.EXPECT().SetShrinkStartChild(false).Once()
	mockPaned.EXPECT().SetShrinkEndChild(false).Once()
	mockPaned.EXPECT().SetStartChild(mockWidget1).Once()
	mockPaned.EXPECT().SetEndChild(mockWidget2).Once()
	mockPaned.EXPECT().GetAllocatedWidth().Return(0).Once()
	mockPaned.EXPECT().ConnectMap(mock.Anything).Return(uint32(0)).Once()
	mockPaned.EXPECT().AddTickCallback(mock.Anything).Return(uint(0)).Once()

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())
	_, _ = renderer.Build(splitNode)

	// Act
	err := renderer.UpdateSplitRatio("split-node", 0.7)

	// Assert
	assert.NoError(t, err)
}

func TestUpdateSplitRatio_NonExistentNode(t *testing.T) {
	// Arrange
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockPaneViewFactory := mocks.NewMockPaneViewFactory(t)

	renderer := layout.NewTreeRenderer(mockFactory, mockPaneViewFactory, zerolog.Nop())

	// Act
	err := renderer.UpdateSplitRatio("non-existent", 0.7)

	// Assert
	assert.ErrorIs(t, err, layout.ErrNodeNotFound)
}

package layout_test

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTreeRendererClearReleasesOwnedSplitTickExactlyOnce(t *testing.T) {
	ctx := context.Background()
	factory := mocks.NewMockWidgetFactory(t)
	paneFactory := mocks.NewMockPaneViewFactory(t)
	paned := mocks.NewMockPanedWidget(t)
	leftStack, leftContainer := setupStackedLeafMocks(t, factory)
	rightStack, rightContainer := setupStackedLeafMocks(t, factory)

	left := &entity.PaneNode{ID: "left", Pane: entity.NewPane("left")}
	right := &entity.PaneNode{ID: "right", Pane: entity.NewPane("right")}
	root := &entity.PaneNode{ID: "split", SplitDir: entity.SplitHorizontal, SplitRatio: 0.5, Children: []*entity.PaneNode{left, right}}

	paneFactory.EXPECT().CreatePaneView(left).Return(leftContainer).Once()
	paneFactory.EXPECT().CreatePaneView(right).Return(rightContainer).Once()
	factory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(paned).Once()
	paned.EXPECT().SetResizeStartChild(true).Once()
	paned.EXPECT().SetResizeEndChild(true).Once()
	paned.EXPECT().SetVisible(true).Once()
	leftStack.EXPECT().SetVisible(true).Once()
	rightStack.EXPECT().SetVisible(true).Once()
	paned.EXPECT().SetStartChild(leftStack).Once()
	paned.EXPECT().SetEndChild(rightStack).Once()
	paned.EXPECT().ConnectNotifyPosition(mock.Anything).Return(uint(0)).Once()
	paned.EXPECT().GetAllocatedWidth().Return(0).Once()
	paned.EXPECT().ConnectMap(mock.Anything).Return(uint(0)).Once()
	paned.EXPECT().AddTickCallback(mock.Anything).Return(uint(42)).Once()
	paned.EXPECT().RemoveTickCallback(uint(42)).Once()

	renderer := layout.NewTreeRenderer(ctx, factory, paneFactory)
	_, err := renderer.Build(ctx, root)
	require.NoError(t, err)

	renderer.Clear()
	renderer.Clear()
}

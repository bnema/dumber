package layout_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

func TestSplitViewCleanupReleasesTickCallbackExactlyOnce(t *testing.T) {
	factory := mocks.NewMockWidgetFactory(t)
	paned := mocks.NewMockPanedWidget(t)
	factory.EXPECT().NewPaned(layout.OrientationHorizontal).Return(paned).Once()
	paned.EXPECT().SetResizeStartChild(true).Once()
	paned.EXPECT().SetResizeEndChild(true).Once()
	paned.EXPECT().SetVisible(true).Once()
	paned.EXPECT().GetAllocatedWidth().Return(0).Once()
	paned.EXPECT().ConnectNotifyPosition(mock.Anything).Return(uint(0)).Once()
	paned.EXPECT().ConnectMap(mock.Anything).Return(uint(0)).Once()
	paned.EXPECT().AddTickCallback(mock.Anything).Return(uint(42)).Once()
	paned.EXPECT().RemoveTickCallback(uint(42)).Once()

	sv := layout.NewSplitView(context.Background(), factory, layout.OrientationHorizontal, nil, nil, 0.5)
	sv.Cleanup()
	sv.Cleanup()
}

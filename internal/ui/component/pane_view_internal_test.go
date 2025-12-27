package component

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

func TestPaneView_HideLoadingSkeleton_HidesWhenPresent(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockContainer := mocks.NewMockBoxWidget(t)
	mockContent := mocks.NewMockBoxWidget(t)
	mockSpinner := mocks.NewMockSpinnerWidget(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(mockContainer).Once()
	mockContainer.EXPECT().SetHexpand(true).Once()
	mockContainer.EXPECT().SetVexpand(true).Once()
	mockContainer.EXPECT().SetHalign(mock.Anything).Once()
	mockContainer.EXPECT().SetValign(mock.Anything).Once()
	mockContainer.EXPECT().SetCanFocus(false).Once()
	mockContainer.EXPECT().SetCanTarget(false).Once()
	mockContainer.EXPECT().AddCssClass("loading-skeleton").Once()

	mockFactory.EXPECT().NewBox(layout.OrientationVertical, 6).Return(mockContent).Once()
	mockContent.EXPECT().SetHalign(mock.Anything).Once()
	mockContent.EXPECT().SetValign(mock.Anything).Once()
	mockContent.EXPECT().SetCanFocus(false).Once()
	mockContent.EXPECT().SetCanTarget(false).Once()
	mockContent.EXPECT().AddCssClass("loading-skeleton-content").Once()

	mockFactory.EXPECT().NewSpinner().Return(mockSpinner).Once()
	mockSpinner.EXPECT().SetHalign(mock.Anything).Once()
	mockSpinner.EXPECT().SetValign(mock.Anything).Once()
	mockSpinner.EXPECT().SetCanFocus(false).Once()
	mockSpinner.EXPECT().SetCanTarget(false).Once()
	mockSpinner.EXPECT().SetSizeRequest(32, 32).Once()
	mockSpinner.EXPECT().AddCssClass("loading-skeleton-spinner").Once()

	mockFactory.EXPECT().NewLabel("Loading...").Return(mockLabel).Once()
	mockLabel.EXPECT().SetHalign(mock.Anything).Once()
	mockLabel.EXPECT().SetValign(mock.Anything).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("loading-skeleton-text").Once()

	mockContent.EXPECT().Append(mockSpinner).Once()
	mockContent.EXPECT().Append(mockLabel).Once()
	mockContainer.EXPECT().Append(mockContent).Once()

	mockContainer.EXPECT().SetVisible(true).Once()
	mockSpinner.EXPECT().Start().Once()
	ls := NewLoadingSkeleton(mockFactory)

	pv := &PaneView{loading: ls}

	mockContainer.EXPECT().SetVisible(false).Once()
	mockSpinner.EXPECT().Stop().Once()
	pv.HideLoadingSkeleton()
}

func TestPaneView_HideLoadingSkeleton_NoOpWhenNil(t *testing.T) {
	pv := &PaneView{}
	pv.HideLoadingSkeleton()
}

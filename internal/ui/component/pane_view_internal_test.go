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
	mockLogo := mocks.NewMockImageWidget(t)
	mockVersion := mocks.NewMockLabelWidget(t)

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

	mockFactory.EXPECT().NewImage().Return(mockLogo).Once()
	mockLogo.EXPECT().SetHalign(mock.Anything).Once()
	mockLogo.EXPECT().SetValign(mock.Anything).Once()
	mockLogo.EXPECT().SetCanFocus(false).Once()
	mockLogo.EXPECT().SetCanTarget(false).Once()
	mockLogo.EXPECT().SetSizeRequest(mock.Anything, mock.Anything).Once()
	mockLogo.EXPECT().SetPixelSize(mock.Anything).Once()
	mockLogo.EXPECT().AddCssClass("loading-skeleton-logo").Once()
	mockLogo.EXPECT().SetFromPaintable(mock.Anything).Maybe()

	mockFactory.EXPECT().NewSpinner().Return(mockSpinner).Once()
	mockSpinner.EXPECT().SetHalign(mock.Anything).Once()
	mockSpinner.EXPECT().SetValign(mock.Anything).Once()
	mockSpinner.EXPECT().SetCanFocus(false).Once()
	mockSpinner.EXPECT().SetCanTarget(false).Once()
	mockSpinner.EXPECT().SetSizeRequest(mock.Anything, mock.Anything).Once()
	mockSpinner.EXPECT().AddCssClass("loading-skeleton-spinner").Once()

	mockFactory.EXPECT().NewLabel(mock.Anything).Return(mockVersion).Once()
	mockVersion.EXPECT().SetHalign(mock.Anything).Once()
	mockVersion.EXPECT().SetValign(mock.Anything).Once()
	mockVersion.EXPECT().SetCanFocus(false).Once()
	mockVersion.EXPECT().SetCanTarget(false).Once()
	mockVersion.EXPECT().SetMaxWidthChars(mock.Anything).Once()
	mockVersion.EXPECT().SetEllipsize(mock.Anything).Once()
	mockVersion.EXPECT().AddCssClass("loading-skeleton-version").Once()

	mockContent.EXPECT().Append(mockLogo).Once()
	mockContent.EXPECT().Append(mockSpinner).Once()
	mockContent.EXPECT().Append(mockVersion).Once()
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

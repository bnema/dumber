package component_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

func TestNewLoadingSkeleton_ShowsAndStartsSpinner(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockContainer := mocks.NewMockBoxWidget(t)
	mockContent := mocks.NewMockBoxWidget(t)
	mockSpinner := mocks.NewMockSpinnerWidget(t)
	mockLogo := mocks.NewMockImageWidget(t)

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

	mockFactory.EXPECT().NewImage().Return(mockLogo).Once()
	mockLogo.EXPECT().SetHalign(mock.Anything).Once()
	mockLogo.EXPECT().SetValign(mock.Anything).Once()
	mockLogo.EXPECT().SetCanFocus(false).Once()
	mockLogo.EXPECT().SetCanTarget(false).Once()
	mockLogo.EXPECT().SetSizeRequest(512, 512).Once()
	mockLogo.EXPECT().SetPixelSize(512).Once()
	mockLogo.EXPECT().AddCssClass("loading-skeleton-logo").Once()
	mockLogo.EXPECT().SetFromPaintable(mock.Anything).Maybe()

	mockContent.EXPECT().Append(mockLogo).Once()
	mockContent.EXPECT().Append(mockSpinner).Once()
	mockContainer.EXPECT().Append(mockContent).Once()

	// NewLoadingSkeleton ends by calling SetVisible(true).
	mockContainer.EXPECT().SetVisible(true).Once()
	mockSpinner.EXPECT().Start().Once()

	ls := component.NewLoadingSkeleton(mockFactory)
	require.NotNil(t, ls)
}

func TestLoadingSkeleton_SetVisible_StopsSpinnerWhenHidden(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockContainer := mocks.NewMockBoxWidget(t)
	mockContent := mocks.NewMockBoxWidget(t)
	mockSpinner := mocks.NewMockSpinnerWidget(t)
	mockLogo := mocks.NewMockImageWidget(t)

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

	mockFactory.EXPECT().NewImage().Return(mockLogo).Once()
	mockLogo.EXPECT().SetHalign(mock.Anything).Once()
	mockLogo.EXPECT().SetValign(mock.Anything).Once()
	mockLogo.EXPECT().SetCanFocus(false).Once()
	mockLogo.EXPECT().SetCanTarget(false).Once()
	mockLogo.EXPECT().SetSizeRequest(512, 512).Once()
	mockLogo.EXPECT().SetPixelSize(512).Once()
	mockLogo.EXPECT().AddCssClass("loading-skeleton-logo").Once()
	mockLogo.EXPECT().SetFromPaintable(mock.Anything).Maybe()

	mockContent.EXPECT().Append(mockLogo).Once()
	mockContent.EXPECT().Append(mockSpinner).Once()
	mockContainer.EXPECT().Append(mockContent).Once()

	mockContainer.EXPECT().SetVisible(true).Once()
	mockSpinner.EXPECT().Start().Once()

	ls := component.NewLoadingSkeleton(mockFactory)
	require.NotNil(t, ls)

	mockContainer.EXPECT().SetVisible(false).Once()
	mockSpinner.EXPECT().Stop().Once()
	ls.SetVisible(false)
}

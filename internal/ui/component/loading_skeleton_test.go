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

	// NewLoadingSkeleton ends by calling SetVisible(true).
	mockContainer.EXPECT().SetVisible(true).Once()
	mockSpinner.EXPECT().Start().Once()

	ls := component.NewLoadingSkeleton(mockFactory)
	require.NotNil(t, ls)
}

type mockedLoadingSkeleton struct {
	skeleton  *component.LoadingSkeleton
	factory   *mocks.MockWidgetFactory
	container *mocks.MockBoxWidget
	content   *mocks.MockBoxWidget
	spinner   *mocks.MockSpinnerWidget
}

// newMockedLoadingSkeleton builds a visible skeleton on generated widget mocks.
func newMockedLoadingSkeleton(t *testing.T) mockedLoadingSkeleton {
	t.Helper()
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

	ls := component.NewLoadingSkeleton(mockFactory)
	require.NotNil(t, ls)
	return mockedLoadingSkeleton{
		skeleton:  ls,
		factory:   mockFactory,
		container: mockContainer,
		content:   mockContent,
		spinner:   mockSpinner,
	}
}

func TestLoadingSkeleton_SetVisible_StopsSpinnerWhenHidden(t *testing.T) {
	m := newMockedLoadingSkeleton(t)

	m.container.EXPECT().SetVisible(false).Once()
	m.spinner.EXPECT().Stop().Once()
	m.skeleton.SetVisible(false)
}

func TestLoadingSkeleton_ShowNotice_ReplacesSpinnerWithPersistentMessage(t *testing.T) {
	m := newMockedLoadingSkeleton(t)
	notice := mocks.NewMockLabelWidget(t)

	// The skeleton was already hidden by a first paint.
	m.container.EXPECT().SetVisible(false).Once()
	m.spinner.EXPECT().Stop().Once()
	m.skeleton.SetVisible(false)

	m.factory.EXPECT().NewLabel("engine updated").Return(notice).Once()
	notice.EXPECT().SetHalign(mock.Anything).Once()
	notice.EXPECT().SetValign(mock.Anything).Once()
	notice.EXPECT().SetCanFocus(false).Once()
	notice.EXPECT().SetCanTarget(false).Once()
	notice.EXPECT().AddCssClass("loading-skeleton-notice").Once()
	m.content.EXPECT().Append(notice).Once()
	m.spinner.EXPECT().Stop().Twice()
	m.spinner.EXPECT().SetVisible(false).Twice()
	m.container.EXPECT().SetVisible(true).Twice()
	m.skeleton.ShowNotice("engine updated")

	// Repeated renderer deaths reuse the same label.
	notice.EXPECT().SetText("engine updated again").Once()
	m.skeleton.ShowNotice("engine updated again")

	// A later paint or load event must not hide the notice: the mocks fail on
	// any unexpected SetVisible or Stop call.
	m.skeleton.SetVisible(false)
}

func TestLoadingSkeleton_ShowNotice_NilSafe(t *testing.T) {
	var ls *component.LoadingSkeleton
	ls.ShowNotice("engine updated")
}

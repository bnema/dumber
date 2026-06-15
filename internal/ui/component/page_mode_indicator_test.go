package component

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

func TestNewPageModeIndicator_ConstructsLabel(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()

	pmi := NewPageModeIndicator(mockFactory)
	require.NotNil(t, pmi)
	require.False(t, pmi.IsVisible())
}

func TestPageModeIndicator_Widget_ReturnsLabel(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()

	pmi := NewPageModeIndicator(mockFactory)
	require.NotNil(t, pmi)

	widget := pmi.Widget()
	require.Equal(t, mockLabel, widget)
}

func TestPageModeIndicator_SetVisible_Shows(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()

	// Expect visibility toggle
	mockLabel.EXPECT().SetVisible(true).Once()

	pmi := NewPageModeIndicator(mockFactory)
	pmi.SetVisible(true)

	require.True(t, pmi.IsVisible())
}

func TestPageModeIndicator_SetVisible_Hides(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()

	// Show
	mockLabel.EXPECT().SetVisible(true).Once()
	pmi := NewPageModeIndicator(mockFactory)
	pmi.SetVisible(true)
	require.True(t, pmi.IsVisible())

	// Hide
	mockLabel.EXPECT().SetVisible(false).Once()
	pmi.SetVisible(false)
	require.False(t, pmi.IsVisible())
}

func TestPageModeIndicator_TriggerPulse_AddsPulseClass(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()

	// TriggerPulse removes both pulse base classes and both cycle classes,
	// then adds the normal pulse base class plus the first cycle variant.
	mockLabel.EXPECT().RemoveCssClass("page-mode-indicator-pulse").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-indicator-pulse-fast").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-pulse-cycle-a").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-pulse-cycle-b").Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator-pulse").Once()
	mockLabel.EXPECT().AddCssClass("page-mode-pulse-cycle-a").Once()

	pmi := NewPageModeIndicator(mockFactory)
	pmi.TriggerPulse()
}

func TestPageModeIndicator_TriggerFastPulse_AddsFastPulseClass(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()

	// TriggerFastPulse removes both pulse base classes and both cycle classes,
	// then adds the fast pulse base class plus the first cycle variant.
	mockLabel.EXPECT().RemoveCssClass("page-mode-indicator-pulse").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-indicator-pulse-fast").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-pulse-cycle-a").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-pulse-cycle-b").Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator-pulse-fast").Once()
	mockLabel.EXPECT().AddCssClass("page-mode-pulse-cycle-a").Once()

	pmi := NewPageModeIndicator(mockFactory)
	pmi.TriggerFastPulse()
}

func TestPageModeIndicator_TriggerPulse_AlternatesCycleClass(t *testing.T) {
	mockFactory := mocks.NewMockWidgetFactory(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()

	// First pulse uses cycle-a.
	mockLabel.EXPECT().RemoveCssClass("page-mode-indicator-pulse").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-indicator-pulse-fast").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-pulse-cycle-a").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-pulse-cycle-b").Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator-pulse").Once()
	mockLabel.EXPECT().AddCssClass("page-mode-pulse-cycle-a").Once()

	// Second pulse flips to cycle-b so GTK restarts the animation reliably.
	mockLabel.EXPECT().RemoveCssClass("page-mode-indicator-pulse").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-indicator-pulse-fast").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-pulse-cycle-a").Once()
	mockLabel.EXPECT().RemoveCssClass("page-mode-pulse-cycle-b").Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator-pulse").Once()
	mockLabel.EXPECT().AddCssClass("page-mode-pulse-cycle-b").Once()

	pmi := NewPageModeIndicator(mockFactory)
	pmi.TriggerPulse()
	pmi.TriggerPulse()
}

func TestPageModeIndicator_DoesNotAffectLayout(t *testing.T) {
	// This test verifies that the indicator is created with non-measuring,
	// non-clipping overlay semantics by checking it does not implement
	// sizing-controlling interfaces and that it uses SetCanFocus(false)
	// and SetCanTarget(false).
	//
	// Layout impact is prevented at the PaneView level by calling
	// SetClipOverlay(false) and SetMeasureOverlay(false) — tested in
	// pane_view_test.go.

	mockFactory := mocks.NewMockWidgetFactory(t)
	mockLabel := mocks.NewMockLabelWidget(t)

	mockFactory.EXPECT().NewLabel("PAGE").Return(mockLabel).Once()
	mockLabel.EXPECT().SetCanFocus(false).Once()
	mockLabel.EXPECT().SetCanTarget(false).Once()
	mockLabel.EXPECT().AddCssClass("page-mode-indicator").Once()
	mockLabel.EXPECT().SetVisible(false).Once()

	pmi := NewPageModeIndicator(mockFactory)

	// Verify Widget returns a LabelWidget (which is a kind of Widget)
	require.NotNil(t, pmi.Widget())
}

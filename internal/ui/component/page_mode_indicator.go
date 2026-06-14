package component

import (
	"github.com/bnema/dumber/internal/ui/layout"
)

const (
	// pageModeIndicatorClass is the base CSS class for the Page mode indicator label.
	pageModeIndicatorClass = "page-mode-indicator"

	// pageModeIndicatorPulseClass is added for a normal (brief) scroll pulse.
	pageModeIndicatorPulseClass = "page-mode-indicator-pulse"

	// pageModeIndicatorFastPulseClass is added for a fast (stronger/longer) scroll pulse.
	pageModeIndicatorFastPulseClass = "page-mode-indicator-pulse-fast"

	// Exported aliases for testing and external inspection.
	PageModeIndicatorClass          = pageModeIndicatorClass
	PageModeActiveClass             = "page-mode-active"
	PageModeIndicatorPulseClass     = pageModeIndicatorPulseClass
	PageModeIndicatorFastPulseClass = pageModeIndicatorFastPulseClass
	// Pane overlay pulse classes (separate from indicator badge classes).
	PageModePulseClass     = "page-mode-pulse"
	PageModeFastPulseClass = "page-mode-pulse-fast"
)

// PageModeIndicator is a small pane-attached label ("PAGE") that is visible
// while Page mode is active. It is added as a non-measuring overlay so it
// does not affect pane layout.
type PageModeIndicator struct {
	label   layout.LabelWidget
	visible bool
}

// NewPageModeIndicator creates a new Page mode indicator label.
// The label starts hidden and should be added as an overlay with
// SetClipOverlay(false) and SetMeasureOverlay(false) to avoid layout impact.
func NewPageModeIndicator(factory layout.WidgetFactory) *PageModeIndicator {
	label := factory.NewLabel("PAGE")
	label.SetCanFocus(false)
	label.SetCanTarget(false)
	label.AddCssClass(pageModeIndicatorClass)
	label.SetVisible(false)

	return &PageModeIndicator{
		label:   label,
		visible: false,
	}
}

// Widget returns the underlying label widget for embedding.
func (pmi *PageModeIndicator) Widget() layout.Widget {
	return pmi.label
}

// SetVisible shows or hides the indicator.
func (pmi *PageModeIndicator) SetVisible(v bool) {
	pmi.visible = v
	pmi.label.SetVisible(v)
}

// IsVisible returns whether the indicator is currently visible.
func (pmi *PageModeIndicator) IsVisible() bool {
	return pmi.visible
}

// TriggerPulse adds the normal scroll pulse CSS class.
// This produces a brief, subtle visual pulse on the indicator.
// Both pulse classes are removed first so that repeated calls safely
// re-trigger the CSS animation.
func (pmi *PageModeIndicator) TriggerPulse() {
	pmi.label.RemoveCssClass(pageModeIndicatorPulseClass)
	pmi.label.RemoveCssClass(pageModeIndicatorFastPulseClass)
	pmi.label.AddCssClass(pageModeIndicatorPulseClass)
}

// TriggerFastPulse adds the fast scroll pulse CSS class.
// This produces a stronger/longer pulse on the indicator.
// Both pulse classes are removed first so that repeated calls safely
// re-trigger the CSS animation.
func (pmi *PageModeIndicator) TriggerFastPulse() {
	pmi.label.RemoveCssClass(pageModeIndicatorPulseClass)
	pmi.label.RemoveCssClass(pageModeIndicatorFastPulseClass)
	pmi.label.AddCssClass(pageModeIndicatorFastPulseClass)
}

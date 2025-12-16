package focus

import (
	"context"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
)

// CSS classes for mode borders.
const (
	paneModeClass = "pane-mode-active"
	tabModeClass  = "tab-mode-active"
)

// BorderManager manages mode indicator borders on the workspace container.
// It creates a transparent overlay box that displays colored borders when
// the user enters tab mode or pane mode.
type BorderManager struct {
	borderOverlay layout.BoxWidget
	currentMode   input.Mode
}

// NewBorderManager creates a border manager.
// It creates a border overlay widget that should be added to the workspace container.
func NewBorderManager(factory layout.WidgetFactory) *BorderManager {
	// Create overlay box for mode borders
	borderBox := factory.NewBox(layout.OrientationVertical, 0)
	borderBox.SetCanFocus(false)
	borderBox.SetCanTarget(false) // Don't intercept pointer events
	borderBox.SetHexpand(true)
	borderBox.SetVexpand(true)
	borderBox.SetVisible(false) // Hidden in normal mode

	return &BorderManager{
		borderOverlay: borderBox,
		currentMode:   input.ModeNormal,
	}
}

// Widget returns the border overlay widget for adding to workspace overlay.
func (bm *BorderManager) Widget() layout.Widget {
	return bm.borderOverlay
}

// OnModeChange updates border visibility based on mode.
func (bm *BorderManager) OnModeChange(ctx context.Context, from, to input.Mode) {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("from", from.String()).
		Str("to", to.String()).
		Msg("border manager: mode changed")

	// Remove old mode class
	switch from {
	case input.ModePane:
		bm.borderOverlay.RemoveCssClass(paneModeClass)
	case input.ModeTab:
		bm.borderOverlay.RemoveCssClass(tabModeClass)
	}

	// Add new mode class and set visibility
	switch to {
	case input.ModePane:
		bm.borderOverlay.AddCssClass(paneModeClass)
		bm.borderOverlay.SetVisible(true)
	case input.ModeTab:
		bm.borderOverlay.AddCssClass(tabModeClass)
		bm.borderOverlay.SetVisible(true)
	case input.ModeNormal:
		bm.borderOverlay.SetVisible(false)
	}

	bm.currentMode = to
}

// CurrentMode returns the current input mode.
func (bm *BorderManager) CurrentMode() input.Mode {
	return bm.currentMode
}

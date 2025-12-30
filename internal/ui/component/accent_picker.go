package component

import (
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Accent picker layout constants.
const (
	accentPickerSpacing    = 8 // Spacing between accent items in pixels.
	maxNumberedAccentItems = 9 // Maximum items that can be selected with number keys (1-9).
)

// AccentPicker displays an overlay with accent character options.
// The user can select an accent using:
// - Left/Right arrow keys to navigate
// - Number keys 1-9 to select directly
// - Enter to confirm selection
// - Escape to cancel
type AccentPicker struct {
	factory layout.WidgetFactory

	// Widgets
	container    layout.BoxWidget     // Outer container for positioning
	accentLabels []layout.LabelWidget // Individual accent character labels

	// State
	accents         []rune
	selectedIndex   int
	selectedCb      func(rune)
	cancelCb        func()
	visible         bool
	keyController   *gtk.EventControllerKey
	retainedClicked []interface{} // Keep click callbacks alive

	mu sync.Mutex
}

// Compile-time interface check.
var _ port.AccentPickerUI = (*AccentPicker)(nil)

// NewAccentPicker creates a new accent picker component.
func NewAccentPicker(factory layout.WidgetFactory) *AccentPicker {
	// Create container box
	container := factory.NewBox(layout.OrientationHorizontal, accentPickerSpacing)
	container.AddCssClass("accent-picker")

	// Position at bottom-center
	container.SetHalign(gtk.AlignCenterValue)
	container.SetValign(gtk.AlignEndValue)

	// Don't expand
	container.SetHexpand(false)
	container.SetVexpand(false)

	// Make focusable so it can receive keyboard events
	container.SetFocusable(true)
	container.SetCanFocus(true)

	// Hidden by default
	container.SetVisible(false)

	ap := &AccentPicker{
		factory:       factory,
		container:     container,
		selectedIndex: 0,
	}

	// Set up key event handling
	ap.setupKeyboardHandling()

	// Set up hover-to-focus: regain focus when mouse enters
	ap.setupHoverFocus()

	return ap
}

// Widget returns the accent picker's container widget.
func (ap *AccentPicker) Widget() layout.Widget {
	return ap.container
}

// Show displays the accent picker with the given accent options.
func (ap *AccentPicker) Show(accents []rune, selectedCb func(rune), cancelCb func()) {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	ap.accents = accents
	ap.selectedCb = selectedCb
	ap.cancelCb = cancelCb
	ap.selectedIndex = 0
	ap.visible = true

	// Clear existing labels
	ap.clearLabels()

	// Create new labels for each accent
	ap.accentLabels = make([]layout.LabelWidget, len(accents))
	ap.retainedClicked = make([]interface{}, len(accents))

	for i, accent := range accents {
		label := ap.factory.NewLabel(string(accent))
		label.AddCssClass("accent-picker-item")

		// Add number hint for first maxNumberedAccentItems items
		if i < maxNumberedAccentItems {
			label.AddCssClass("accent-picker-numbered")
		}

		// Set up click handling
		clickCtrl := gtk.NewGestureClick()
		idx := i // Capture for closure
		clickCb := func(_ gtk.GestureClick, _ int, _ float64, _ float64) {
			ap.selectAccent(idx)
		}
		clickCtrl.ConnectPressed(&clickCb)
		ap.retainedClicked[i] = clickCb
		label.AddController(&clickCtrl.EventController)

		ap.accentLabels[i] = label
		ap.container.Append(label)
	}

	// Highlight first item
	ap.updateSelection()

	ap.container.SetVisible(true)
	ap.container.GrabFocus()
}

// Hide hides the accent picker.
func (ap *AccentPicker) Hide() {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	ap.visible = false
	ap.container.SetVisible(false)
	ap.clearLabels()
}

// IsVisible returns true if the accent picker is currently visible.
func (ap *AccentPicker) IsVisible() bool {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	return ap.visible
}

// clearLabels removes all accent labels from the container.
func (ap *AccentPicker) clearLabels() {
	for _, label := range ap.accentLabels {
		label.Unparent()
	}
	ap.accentLabels = nil
	ap.retainedClicked = nil
}

// updateSelection updates the visual selection highlight.
func (ap *AccentPicker) updateSelection() {
	for i, label := range ap.accentLabels {
		if i == ap.selectedIndex {
			label.AddCssClass("accent-picker-selected")
		} else {
			label.RemoveCssClass("accent-picker-selected")
		}
	}
}

// selectAccent selects the accent at the given index.
func (ap *AccentPicker) selectAccent(index int) {
	ap.mu.Lock()
	if index < 0 || index >= len(ap.accents) || ap.selectedCb == nil {
		ap.mu.Unlock()
		return
	}
	accent := ap.accents[index]
	// Invoke callback while holding lock to avoid race with Hide() clearing callbacks
	ap.selectedCb(accent)
	ap.mu.Unlock()
}

// cancel cancels the accent picker.
func (ap *AccentPicker) cancel() {
	ap.mu.Lock()
	if ap.cancelCb == nil {
		ap.mu.Unlock()
		return
	}
	// Invoke callback while holding lock to avoid race with Hide() clearing callbacks
	ap.cancelCb()
	ap.mu.Unlock()
}

// setupKeyboardHandling sets up key event handling for navigation.
func (ap *AccentPicker) setupKeyboardHandling() {
	keyCtrl := gtk.NewEventControllerKey()

	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, _ gdk.ModifierType) bool {
		return ap.handleKeyPress(keyval)
	}

	keyCtrl.ConnectKeyPressed(&keyPressedCb)
	ap.keyController = keyCtrl
	ap.container.AddController(&keyCtrl.EventController)
}

// setupHoverFocus sets up mouse hover to regain focus.
// This ensures the picker regains focus when the mouse enters,
// so keyboard navigation (including Escape) continues to work.
func (ap *AccentPicker) setupHoverFocus() {
	motionCtrl := gtk.NewEventControllerMotion()

	enterCb := func(_ gtk.EventControllerMotion, _ float64, _ float64) {
		ap.mu.Lock()
		visible := ap.visible
		ap.mu.Unlock()

		if visible {
			ap.container.GrabFocus()
		}
	}

	motionCtrl.ConnectEnter(&enterCb)
	ap.container.AddController(&motionCtrl.EventController)
}

// handleKeyPress processes a key press event.
// Returns true if the key was handled.
func (ap *AccentPicker) handleKeyPress(keyval uint) bool {
	ap.mu.Lock()
	if !ap.visible {
		ap.mu.Unlock()
		return false
	}

	// Handle navigation keys
	if handled, shouldReturn := ap.handleNavigationKey(keyval); shouldReturn {
		return handled
	}

	// Handle number keys for direct selection
	if idx, ok := ap.numberKeyToIndex(keyval); ok {
		ap.mu.Unlock()
		ap.selectAccent(idx)
		return true
	}

	ap.mu.Unlock()
	return false
}

// handleNavigationKey handles arrow keys, Enter, and Escape.
// Returns (handled, shouldReturn) - if shouldReturn is true, caller should return handled.
func (ap *AccentPicker) handleNavigationKey(keyval uint) (bool, bool) {
	switch keyval {
	case uint(gdk.KEY_Left):
		if ap.selectedIndex > 0 {
			ap.selectedIndex--
			ap.updateSelection()
		}
		ap.mu.Unlock()
		return true, true

	case uint(gdk.KEY_Right):
		if ap.selectedIndex < len(ap.accents)-1 {
			ap.selectedIndex++
			ap.updateSelection()
		}
		ap.mu.Unlock()
		return true, true

	case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
		idx := ap.selectedIndex
		ap.mu.Unlock()
		ap.selectAccent(idx)
		return true, true

	case uint(gdk.KEY_Escape):
		ap.mu.Unlock()
		ap.cancel()
		return true, true

	default:
		return false, false
	}
}

// numberKeyToIndex converts a number key (1-9) to an accent index.
// Returns the index and true if the key is a valid number key.
func (ap *AccentPicker) numberKeyToIndex(keyval uint) (int, bool) {
	keyMap := map[uint]int{
		uint(gdk.KEY_1):    0,
		uint(gdk.KEY_KP_1): 0,
		uint(gdk.KEY_2):    1,
		uint(gdk.KEY_KP_2): 1,
		uint(gdk.KEY_3):    2,
		uint(gdk.KEY_KP_3): 2,
		uint(gdk.KEY_4):    3,
		uint(gdk.KEY_KP_4): 3,
		uint(gdk.KEY_5):    4,
		uint(gdk.KEY_KP_5): 4,
		uint(gdk.KEY_6):    5,
		uint(gdk.KEY_KP_6): 5,
		uint(gdk.KEY_7):    6,
		uint(gdk.KEY_KP_7): 6,
		uint(gdk.KEY_8):    7,
		uint(gdk.KEY_KP_8): 7,
		uint(gdk.KEY_9):    8,
		uint(gdk.KEY_KP_9): 8,
	}

	if idx, ok := keyMap[keyval]; ok {
		return idx, true
	}
	return 0, false
}

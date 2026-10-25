// Package component provides reusable GTK UI components.
package component

import (
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/pango"
)

// TabButton represents a single tab button in the tab bar.
type TabButton struct {
	entry           *gtk.Entry
	renameCallbacks []any
	cancelRename    func()
	button          *gtk.Button
	label           *gtk.Label
	tabID           entity.TabID
	isActive        bool

	// Callback for click events
	onClick func(tabID entity.TabID)
}

// NewTabButton creates a new tab button for the given tab.
func NewTabButton(tab *entity.Tab) *TabButton {
	tb := &TabButton{
		tabID: tab.ID,
	}

	// Create the button
	tb.button = gtk.NewButton()
	if tb.button == nil {
		return nil
	}

	// Prevent focus stealing from WebView
	tb.button.SetFocusOnClick(false)
	tb.button.SetCanFocus(false)

	// Add CSS class
	tb.button.AddCssClass("tab-button")

	// Create the label
	title := tab.Title()
	tb.label = gtk.NewLabel(&title)
	if tb.label == nil {
		tb.button.Unref()
		return nil
	}

	// Configure label for text overflow
	const maxTabTitleChars = 20
	tb.label.SetEllipsize(pango.EllipsizeMiddleValue)
	tb.label.SetMaxWidthChars(maxTabTitleChars)
	tb.label.AddCssClass("tab-title")

	// Set label as button child
	tb.button.SetChild(&tb.label.Widget)

	return tb
}

// Widget returns the underlying GTK widget for embedding.
func (tb *TabButton) Widget() *gtk.Widget {
	return &tb.button.Widget
}

// Button returns the underlying GTK button.
func (tb *TabButton) Button() *gtk.Button {
	return tb.button
}

// TabID returns the ID of the tab this button represents.
func (tb *TabButton) TabID() entity.TabID {
	return tb.tabID
}

// SetTitle updates the button's label text.
func (tb *TabButton) SetTitle(title string) {
	if tb.label != nil {
		tb.label.SetText(title)
	}
}

// SetActive updates the active state styling.
func (tb *TabButton) SetActive(active bool) {
	if tb.isActive == active {
		return
	}
	tb.isActive = active
	if tb.button == nil {
		return
	}

	if active {
		tb.button.AddCssClass("tab-button-active")
	} else {
		tb.button.RemoveCssClass("tab-button-active")
	}
}

// IsActive returns whether this tab is currently active.
func (tb *TabButton) IsActive() bool {
	return tb.isActive
}

// SetOnClick sets the callback for click events.
func (tb *TabButton) SetOnClick(fn func(tabID entity.TabID)) {
	tb.onClick = fn

	if fn != nil {
		tabID := tb.tabID // Capture for closure
		clickCb := func(_ gtk.Button) {
			if tb.onClick != nil {
				tb.onClick(tabID)
			}
		}
		tb.button.ConnectClicked(&clickCb)
	}
}

// Destroy cleans up the button resources.
func (tb *TabButton) Destroy() {
	if tb.cancelRename != nil {
		tb.cancelRename()
	}
	if tb.label != nil {
		tb.label.Unref()
		tb.label = nil
	}
	if tb.button != nil {
		tb.button.Unref()
		tb.button = nil
	}
}

// beginRename keeps the original label alive while it is temporarily unparented.
func (tb *TabButton) beginRename(onCommit func(string), onCancel, onFinish func()) {
	entry := gtk.NewEntry()
	if entry == nil {
		onFinish()
		return
	}
	entry.SetText(tb.label.GetText())
	const renameWidthChars = 20
	entry.SetWidthChars(renameWidthChars)
	entry.AddCssClass("tab-rename-entry")
	tb.entry = entry
	tb.label.Ref()
	tb.button.SetCanFocus(true)
	tb.button.SetChild(&entry.Widget)
	finished := false
	finish := func(commit bool, restoreFocus bool) {
		if finished {
			return
		}
		finished = true
		name := strings.TrimSpace(entry.GetText())
		tb.entry = nil
		tb.cancelRename = nil
		tb.button.SetChild(&tb.label.Widget)
		tb.label.Unref()
		// Release the reference taken by gtk.NewEntry; this destroys the entry
		// and its controllers, which drop their signal closures.
		entry.Unref()
		tb.renameCallbacks = nil
		tb.button.SetCanFocus(false)
		onFinish()
		if commit && name != "" {
			if onCommit != nil {
				onCommit(name)
			}
		} else if restoreFocus && onCancel != nil {
			onCancel()
		}
	}
	tb.cancelRename = func() { finish(false, false) }
	tb.connectRenameControllers(entry, finish)
	entry.GrabFocus()
	entry.SelectRegion(0, -1)
}

// connectRenameControllers maps Enter to commit, Escape to cancel, and focus
// loss to a cancel that leaves focus where the user moved it.
func (tb *TabButton) connectRenameControllers(entry *gtk.Entry, finish func(commit, restoreFocus bool)) {
	key := gtk.NewEventControllerKey()
	key.SetPropagationPhase(gtk.PhaseCaptureValue)
	pressed := func(_ gtk.EventControllerKey, keyval uint, _ uint, _ gdk.ModifierType) bool {
		switch keyval {
		case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
			finish(true, true)
			return true
		case uint(gdk.KEY_Escape):
			finish(false, true)
			return true
		}
		return false
	}
	key.ConnectKeyPressed(&pressed)
	entry.AddController(&key.EventController)
	focus := gtk.NewEventControllerFocus()
	leave := func(_ gtk.EventControllerFocus) { finish(false, false) }
	focus.ConnectLeave(&leave)
	entry.AddController(&focus.EventController)
	tb.renameCallbacks = []any{pressed, leave}
}

// Package component provides reusable GTK UI components.
package component

import (
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/jwijenbergh/puregotk/v4/pango"
)

// TabButton represents a single tab button in the tab bar.
type TabButton struct {
	button   *gtk.Button
	label    *gtk.Label
	tabID    entity.TabID
	isActive bool

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
	if tb.label != nil {
		tb.label.Unref()
		tb.label = nil
	}
	if tb.button != nil {
		tb.button.Unref()
		tb.button = nil
	}
}

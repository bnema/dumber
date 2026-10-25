package component

import (
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gtk"
)

// TabBar represents the horizontal tab bar widget.
type TabBar struct {
	box     *gtk.Box
	buttons map[entity.TabID]*TabButton

	explicitlyVisible bool
	autoHidden        bool
	// renaming temporarily forces the bar visible while an inline editor is open.
	renaming bool

	// Active tab tracking
	activeTabID entity.TabID

	// Callbacks
	onSwitch func(tabID entity.TabID)
	onClose  func(tabID entity.TabID)
	onCreate func()

	mu sync.RWMutex
}

// NewTabBar creates a new tab bar widget.
func NewTabBar() *TabBar {
	tb := &TabBar{
		buttons:           make(map[entity.TabID]*TabButton),
		explicitlyVisible: true,
	}

	// Create horizontal box container
	tb.box = gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	if tb.box == nil {
		return nil
	}

	// Configure the box
	tb.box.SetHexpand(true)
	tb.box.SetVexpand(false)
	tb.box.SetVisible(true)
	tb.box.AddCssClass("tab-bar")

	return tb
}

// Widget returns the underlying GTK widget for embedding.
func (tb *TabBar) Widget() *gtk.Widget {
	return &tb.box.Widget
}

// Box returns the underlying GTK box.
func (tb *TabBar) Box() *gtk.Box {
	return tb.box
}

// AddTab adds a new tab button to the bar.
func (tb *TabBar) AddTab(tab *entity.Tab) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Check if already exists
	if _, exists := tb.buttons[tab.ID]; exists {
		return
	}

	// Create the button
	button := NewTabButton(tab)
	if button == nil {
		return
	}

	// Set click handler
	button.SetOnClick(func(tabID entity.TabID) {
		if tb.onSwitch != nil {
			tb.onSwitch(tabID)
		}
	})

	// Add to container
	tb.box.Append(button.Widget())
	tb.buttons[tab.ID] = button
}

// RemoveTab removes a tab button from the bar.
func (tb *TabBar) RemoveTab(tabID entity.TabID) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	button, exists := tb.buttons[tabID]
	if !exists {
		return
	}

	// Remove from container
	tb.box.Remove(button.Widget())
	button.Destroy()
	delete(tb.buttons, tabID)
}

// SetActive updates which tab is shown as active.
func (tb *TabBar) SetActive(tabID entity.TabID) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Deactivate previous
	if tb.activeTabID != "" && tb.activeTabID != tabID {
		if prevButton, exists := tb.buttons[tb.activeTabID]; exists {
			prevButton.SetActive(false)
		}
	}

	// Activate new
	if button, exists := tb.buttons[tabID]; exists {
		button.SetActive(true)
	}

	tb.activeTabID = tabID
}

// UpdateTitle updates the title of a specific tab button.
func (tb *TabBar) UpdateTitle(tabID entity.TabID, title string) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	if button, exists := tb.buttons[tabID]; exists {
		button.SetTitle(title)
	}
}

// SetOnSwitch sets the callback for tab switch events.
func (tb *TabBar) SetOnSwitch(fn func(tabID entity.TabID)) {
	tb.onSwitch = fn
}

// SetOnClose sets the callback for tab close events.
func (tb *TabBar) SetOnClose(fn func(tabID entity.TabID)) {
	tb.onClose = fn
}

// SetOnCreate sets the callback for new tab creation.
func (tb *TabBar) SetOnCreate(fn func()) {
	tb.onCreate = fn
}

// Count returns the number of tabs in the bar.
func (tb *TabBar) Count() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.buttons)
}

// ActiveTabID returns the currently active tab ID.
func (tb *TabBar) ActiveTabID() entity.TabID {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.activeTabID
}

// SetVisible explicitly shows or hides the tab bar.
func (tb *TabBar) SetVisible(visible bool) {
	if tb == nil || tb.box == nil {
		return
	}
	tb.explicitlyVisible = visible
	tb.applyVisibilityState()
}

// SetAutoHidden hides the tab bar visually while keeping the overlay widget
// visible to GTK when the bar has not been explicitly hidden. It is used for
// hide_tab_bar_when_single_tab; WebView viewport height stays stable because the
// tab bar is mounted as a non-measured overlay above the content.
func (tb *TabBar) SetAutoHidden(hidden bool) {
	if tb == nil || tb.box == nil {
		return
	}

	tb.autoHidden = hidden
	tb.applyVisibilityState()
}

func (tb *TabBar) applyVisibilityState() {
	if tb == nil || tb.box == nil {
		return
	}

	interactive := tb.renaming || (tb.explicitlyVisible && !tb.autoHidden)
	tb.box.SetVisible(tb.renaming || tb.explicitlyVisible)
	if interactive {
		tb.box.SetOpacity(1.0)
	} else {
		tb.box.SetOpacity(0.0)
	}
	tb.box.SetCanTarget(interactive)
	tb.box.SetFocusable(interactive)
}

// Destroy cleans up all tab bar resources.
func (tb *TabBar) Destroy() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for _, button := range tb.buttons {
		button.Destroy()
	}
	tb.buttons = nil

	if tb.box != nil {
		tb.box.Unref()
		tb.box = nil
	}
}

// IsRenaming reports whether an inline title editor owns keyboard input.
func (tb *TabBar) IsRenaming() bool {
	for _, button := range tb.buttons {
		if button.entry != nil {
			return true
		}
	}
	return false
}

// BeginRename swaps a tab label for an editor. The component never mutates tab state.
func (tb *TabBar) BeginRename(tabID entity.TabID, onCommit func(string), onCancel func()) {
	if tb.IsRenaming() {
		return
	}
	button := tb.buttons[tabID]
	if button == nil {
		return
	}
	tb.setRenaming(true)
	button.beginRename(onCommit, onCancel, func() {
		tb.setRenaming(false)
	})
}

// setRenaming toggles the rename visibility override and reapplies the
// latest desired visibility, so tab-count changes during editing are kept.
func (tb *TabBar) setRenaming(renaming bool) {
	if tb == nil || tb.box == nil {
		return
	}
	tb.renaming = renaming
	tb.applyVisibilityState()
}

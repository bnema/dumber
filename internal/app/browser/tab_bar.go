package browser

import (
	"fmt"
	"sync/atomic"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// createTabBar creates the horizontal tab bar container.
// Returns the tab bar widget.
func (tm *TabManager) createTabBar() gtk.Widgetter {
	logging.Debug("[tabs] Creating tab bar container")

	tabBar := gtk.NewBox(gtk.OrientationHorizontal, 0)
	if tabBar == nil {
		logging.Error("[tabs] Failed to create tab bar box")
		return nil
	}

	tabBar.AddCSSClass("tab-bar")
	tabBar.SetHExpand(true)
	tabBar.SetVExpand(false)

	tm.tabBar = tabBar
	logging.Debug("[tabs] Tab bar container created")
	return tabBar
}

// createTabButton creates a simple GTK button widget for a tab with just a text label.
func (tm *TabManager) createTabButton(tab *Tab) gtk.Widgetter {
	logging.Debug(fmt.Sprintf("[tabs] Creating button for tab %s", tab.id))

	// Create button
	button := gtk.NewButton()
	if button == nil {
		logging.Error("[tabs] Failed to create tab button")
		return nil
	}

	// CRITICAL: Disable focus-on-click to prevent GTK focus system from interfering
	// Tab buttons should be clickable without grabbing focus from the WebView
	button.SetFocusOnClick(false)
	button.SetCanFocus(false)

	// Apply CSS classes
	button.AddCSSClass("tab-button")

	// Get display title (custom name or default name)
	displayTitle := tab.title
	if tab.customTitle != "" {
		displayTitle = tab.customTitle
	}

	// Create title label
	titleLabel := gtk.NewLabel(displayTitle)
	if titleLabel != nil {
		titleLabel.AddCSSClass("tab-title")
		titleLabel.SetEllipsize(2) // Pango ellipsize mode: middle
		titleLabel.SetMaxWidthChars(20)
		webkit.WidgetSetVisible(titleLabel, true)
	}

	// Set button child
	button.SetChild(titleLabel)

	// Store button reference for later updates
	tab.titleButton = button

	// Note: Click handler will be attached in createTabInternal after tab is added to slice

	// Set active class if this is the active tab
	if tab.isActive {
		webkit.WidgetAddCSSClass(button, "tab-button-active")
	}

	webkit.WidgetSetVisible(button, true)
	logging.Debug(fmt.Sprintf("[tabs] Button created for tab %s", tab.id))
	return button
}

// attachTabClickHandler attaches a click handler to switch to a tab when clicked.
// Uses the same pattern as StackedPaneManager: maps unique button ID -> tab,
// then looks up current index at click time to handle tab reordering correctly.
func (tm *TabManager) attachTabClickHandler(button gtk.Widgetter, tab *Tab) {
	if button == nil || tab == nil {
		logging.Error("[tabs] Cannot attach click handler: nil button or tab")
		return
	}

	gtkButton, ok := button.(*gtk.Button)
	if !ok {
		logging.Error(fmt.Sprintf("[tabs] Cannot attach click handler: widget is not *gtk.Button for tab %s", tab.id))
		return
	}

	// Generate unique button ID and store mapping (pattern from stacked panes)
	buttonID := atomic.AddUint64(&tm.nextButtonID, 1)
	tm.buttonToTab[buttonID] = tab

	gtkButton.ConnectClicked(func() {
		tm.handleTabButtonClick(buttonID)
	})

	logging.Debug(fmt.Sprintf("[tabs] Click handler attached to tab %s (buttonID=%d)", tab.id, buttonID))
}

// handleTabButtonClick handles clicks on tab buttons to switch tabs.
// Looks up the current index dynamically to handle tab reordering (pattern from stacked panes).
func (tm *TabManager) handleTabButtonClick(buttonID uint64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Look up tab from button ID
	tab, exists := tm.buttonToTab[buttonID]
	if !exists || tab == nil {
		logging.Warn(fmt.Sprintf("[tabs] Tab button click for unknown ID: %d", buttonID))
		return
	}

	// Find current index of this tab
	targetIndex := -1
	for i, t := range tm.tabs {
		if t == tab {
			targetIndex = i
			break
		}
	}

	if targetIndex < 0 {
		logging.Error(fmt.Sprintf("[tabs] Tab %s not found in tabs slice", tab.id))
		return
	}

	// Only switch if not already active
	if targetIndex == tm.activeIndex {
		logging.Debug(fmt.Sprintf("[tabs] Tab %s already active (index %d)", tab.id, targetIndex))
		return
	}

	logging.Info(fmt.Sprintf("[tabs] Tab button clicked: switching from tab %d to %d (%s)", tm.activeIndex, targetIndex, tab.id))

	// Switch to this tab
	if err := tm.switchToTabInternal(targetIndex); err != nil {
		logging.Error(fmt.Sprintf("[tabs] Failed to switch to tab %d: %v", targetIndex, err))
	}
}

// updateTabButton updates the tab button's title label.
func (tm *TabManager) updateTabButton(tab *Tab) {
	if tab.titleButton == nil {
		logging.Warn(fmt.Sprintf("[tabs] Cannot update button for tab %s: button is nil", tab.id))
		return
	}

	webkit.RunOnMainThread(func() {
		button, ok := tab.titleButton.(*gtk.Button)
		if !ok {
			logging.Error(fmt.Sprintf("[tabs] Tab button is not a *gtk.Button: %s", tab.id))
			return
		}

		// Get button child (the label)
		child := button.Child()
		if child == nil {
			logging.Warn(fmt.Sprintf("[tabs] Tab button has no child: %s", tab.id))
			return
		}

		label, ok := child.(*gtk.Label)
		if !ok {
			logging.Error(fmt.Sprintf("[tabs] Tab button child is not a *gtk.Label: %s", tab.id))
			return
		}

		// Update label text
		displayTitle := tab.title
		if tab.customTitle != "" {
			displayTitle = tab.customTitle
		}
		label.SetLabel(displayTitle)
		logging.Debug(fmt.Sprintf("[tabs] Updated tab button label: %s -> %s", tab.id, displayTitle))
	})
}

// addTabToBar adds a tab button to the tab bar.
func (tm *TabManager) addTabToBar(tab *Tab) {
	if tm.tabBar == nil {
		logging.Error("[tabs] Cannot add tab to bar: tab bar is nil")
		return
	}

	if tab.titleButton == nil {
		logging.Error(fmt.Sprintf("[tabs] Cannot add tab %s to bar: button is nil", tab.id))
		return
	}

	webkit.RunOnMainThread(func() {
		if tabBarBox, ok := tm.tabBar.(*gtk.Box); ok {
			tabBarBox.Append(tab.titleButton)
			logging.Debug(fmt.Sprintf("[tabs] Added tab %s to tab bar", tab.id))
		} else {
			logging.Error("[tabs] Tab bar is not a *gtk.Box")
		}
	})
}

// removeTabFromBar removes a tab button from the tab bar.
func (tm *TabManager) removeTabFromBar(tab *Tab) {
	if tm.tabBar == nil || tab.titleButton == nil {
		return
	}

	webkit.RunOnMainThread(func() {
		if tabBarBox, ok := tm.tabBar.(*gtk.Box); ok {
			tabBarBox.Remove(tab.titleButton)
			logging.Debug(fmt.Sprintf("[tabs] Removed tab %s from tab bar", tab.id))
		}
	})
}

// setTabActiveStyle applies or removes the active CSS class on a tab button.
func (tm *TabManager) setTabActiveStyle(tab *Tab, active bool) {
	if tab.titleButton == nil {
		logging.Warn(fmt.Sprintf("[tabs] Cannot set active style: button is nil for tab %s", tab.id))
		return
	}

	webkit.RunOnMainThread(func() {
		if active {
			webkit.WidgetAddCSSClass(tab.titleButton, "tab-button-active")
			logging.Debug(fmt.Sprintf("[tabs] Added active class to tab %s", tab.id))
		} else {
			webkit.WidgetRemoveCSSClass(tab.titleButton, "tab-button-active")
			logging.Debug(fmt.Sprintf("[tabs] Removed active class from tab %s", tab.id))
		}
	})
}

// startInlineRename replaces the tab button's label with an entry widget for inline editing.
func (tm *TabManager) startInlineRename(tab *Tab) {
	if tab.titleButton == nil {
		logging.Error(fmt.Sprintf("[tabs] Cannot start rename: button is nil for tab %s", tab.id))
		return
	}

	// Check if rename already in progress
	tm.mu.Lock()
	if tm.renameInProgress {
		logging.Debug("[tabs] Rename already in progress, ignoring")
		tm.mu.Unlock()
		return
	}
	tm.renameInProgress = true
	tm.renamingTab = tab

	// Stop tab mode timer during rename
	if tm.tabModeTimer != nil {
		tm.tabModeTimer.Stop()
		tm.tabModeTimer = nil
	}
	tm.mu.Unlock()

	webkit.RunOnMainThread(func() {
		button, ok := tab.titleButton.(*gtk.Button)
		if !ok {
			logging.Error(fmt.Sprintf("[tabs] Tab button is not a *gtk.Button: %s", tab.id))
			tm.cancelRename()
			return
		}

		// Get current text
		currentText := tab.title
		if tab.customTitle != "" {
			currentText = tab.customTitle
		}

		// Store original label for restoration
		originalLabel := button.Child()

		// Create entry widget
		entry := gtk.NewEntry()
		if entry == nil {
			logging.Error("[tabs] Failed to create entry widget for rename")
			tm.cancelRename()
			return
		}

		// Store current focus behavior so we can restore it later
		prevFocusOnClick := button.FocusOnClick()
		prevCanFocus := button.CanFocus()
		tm.mu.Lock()
		tm.renamePrevFocusOnClick = prevFocusOnClick
		tm.renamePrevCanFocus = prevCanFocus
		tm.mu.Unlock()

		// Allow the entry to receive focus while rename is active
		button.SetFocusOnClick(true)
		button.SetCanFocus(true)

		// Configure entry BEFORE adding to widget tree
		entry.SetText(currentText)
		entry.AddCSSClass("tab-rename-entry")
		entry.SetCanFocus(true)  // Explicitly allow focus
		entry.SetFocusable(true) // GTK4 focusability

		// Handle Enter key - save
		entry.ConnectActivate(func() {
			newName := entry.Text()
			logging.Debug(fmt.Sprintf("[tabs] Rename activate (Enter): '%s' -> '%s'", currentText, newName))
			tm.finishInlineRename(tab, newName, originalLabel)
		})

		// Handle Escape key - cancel
		keyController := gtk.NewEventControllerKey()
		keyController.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
			const gdkKeyEscape = 0xff1b
			if keyval == gdkKeyEscape {
				logging.Debug("[tabs] Rename cancelled (Escape)")
				tm.finishInlineRename(tab, "", originalLabel) // Empty = cancel
				return true
			}
			return false
		})
		entry.AddController(keyController)

		// Handle focus-out - cancel (safety)
		focusController := gtk.NewEventControllerFocus()
		focusController.ConnectLeave(func() {
			logging.Debug("[tabs] Rename cancelled (focus-out)")
			tm.finishInlineRename(tab, "", originalLabel)
		})
		entry.AddController(focusController)

		// Replace label with entry
		button.SetChild(entry)

		// Make entry visible first
		webkit.WidgetSetVisible(entry, true)

		// Grab focus after widget is visible and realized
		// Use idle callback to ensure widget is fully realized
		entry.GrabFocus()

		// Select all text after focus
		entry.SelectRegion(0, -1)

		logging.Debug(fmt.Sprintf("[tabs] Started inline rename for tab %s", tab.id))
	})
}

// cancelRename clears the rename state without changing anything
func (tm *TabManager) cancelRename() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.renameInProgress = false
	tm.renamingTab = nil
	tm.renamePrevFocusOnClick = false
	tm.renamePrevCanFocus = false
	logging.Debug("[tabs] Rename state cleared")
}

// finishInlineRename restores the label and saves the new name if provided.
func (tm *TabManager) finishInlineRename(tab *Tab, newName string, originalLabel gtk.Widgetter) {
	// Guard against multiple calls
	tm.mu.Lock()
	if !tm.renameInProgress {
		logging.Debug("[tabs] finishInlineRename called but no rename in progress, ignoring")
		tm.mu.Unlock()
		return
	}
	if tm.renamingTab != tab {
		logging.Debug(fmt.Sprintf("[tabs] finishInlineRename called for wrong tab (expected %s, got %s)",
			tm.renamingTab.id, tab.id))
		tm.mu.Unlock()
		return
	}

	// Clear rename state immediately to prevent re-entry
	tm.renameInProgress = false
	tm.renamingTab = nil
	tm.mu.Unlock()

	webkit.RunOnMainThread(func() {
		button, ok := tab.titleButton.(*gtk.Button)
		if !ok {
			logging.Error(fmt.Sprintf("[tabs] Tab button is not a *gtk.Button during finish: %s", tab.id))
			return
		}

		// Restore original label
		button.SetChild(originalLabel)

		// Restore original focus behavior now that rename is finished
		tm.mu.Lock()
		prevFocusOnClick := tm.renamePrevFocusOnClick
		prevCanFocus := tm.renamePrevCanFocus
		tm.renamePrevFocusOnClick = false
		tm.renamePrevCanFocus = false
		tm.mu.Unlock()
		button.SetFocusOnClick(prevFocusOnClick)
		button.SetCanFocus(prevCanFocus)

		// Save new name if provided and not just whitespace
		newName = trimWhitespace(newName)
		if newName != "" {
			tab.customTitle = newName
			logging.Info(fmt.Sprintf("[tabs] Tab %s renamed to '%s'", tab.id, newName))
		} else {
			logging.Debug(fmt.Sprintf("[tabs] Tab %s rename cancelled", tab.id))
		}

		// Update label text
		tm.updateTabButton(tab)

		// Exit tab mode
		tm.ExitTabMode("rename-complete")
	})
}

// trimWhitespace removes leading and trailing whitespace from a string.
func trimWhitespace(s string) string {
	start := 0
	end := len(s)

	// Trim leading whitespace
	for start < end && isWhitespace(s[start]) {
		start++
	}

	// Trim trailing whitespace
	for end > start && isWhitespace(s[end-1]) {
		end--
	}

	return s[start:end]
}

// isWhitespace checks if a byte is a whitespace character.
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

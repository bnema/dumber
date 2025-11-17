package browser

import (
	"fmt"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
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

	// Attach click handler
	tm.attachTabClickHandler(button, tab)

	// Set active class if this is the active tab
	if tab.isActive {
		webkit.WidgetAddCSSClass(button, "tab-button-active")
	}

	webkit.WidgetSetVisible(button, true)
	logging.Debug(fmt.Sprintf("[tabs] Button created for tab %s", tab.id))
	return button
}

// attachTabClickHandler attaches a click handler to switch to a tab when clicked.
func (tm *TabManager) attachTabClickHandler(button gtk.Widgetter, tab *Tab) {
	// Find tab index
	tabIndex := -1
	for i, t := range tm.tabs {
		if t == tab {
			tabIndex = i
			break
		}
	}

	if tabIndex < 0 {
		logging.Error(fmt.Sprintf("[tabs] Cannot attach click handler: tab %s not found", tab.id))
		return
	}

	// Create click gesture controller
	clickController := gtk.NewGestureClick()
	clickController.SetButton(1) // Left mouse button

	// Connect click signal
	clickController.ConnectPressed(func(_ int, _, _ float64) {
		logging.Debug(fmt.Sprintf("[tabs] Tab button clicked: %s (index %d)", tab.id, tabIndex))

		// Switch to this tab
		webkit.RunOnMainThread(func() {
			if err := tm.SwitchToTab(tabIndex); err != nil {
				logging.Error(fmt.Sprintf("[tabs] Failed to switch to tab %d: %v", tabIndex, err))
			}
		})
	})

	// Add gesture controller to button
	if gtkWidget, ok := button.(interface{ AddController(gtk.EventControllerer) }); ok {
		gtkWidget.AddController(clickController)
		logging.Debug(fmt.Sprintf("[tabs] Click handler attached to tab %s", tab.id))
	} else {
		logging.Error(fmt.Sprintf("[tabs] Failed to attach click handler to tab %s", tab.id))
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
		return
	}

	webkit.RunOnMainThread(func() {
		if active {
			webkit.WidgetAddCSSClass(tab.titleButton, "tab-button-active")
		} else {
			webkit.WidgetRemoveCSSClass(tab.titleButton, "tab-button-active")
		}
	})
}

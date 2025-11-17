package browser

import (
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
)

// EnterTabMode activates the modal tab management mode.
// This is triggered by Alt+T and provides visual feedback and temporary key bindings.
func (tm *TabManager) EnterTabMode() {
	logging.Debug("[tab-mode] EnterTabMode called - attempting to acquire lock")
	tm.mu.Lock()
	defer tm.mu.Unlock()
	logging.Debug("[tab-mode] Lock acquired")

	// Already in tab mode?
	if tm.tabModeActive {
		logging.Debug("[tab-mode] Tab mode already active")
		return
	}

	logging.Info("[tab-mode] Entering tab mode")

	logging.Debug("[tab-mode] Setting tabModeActive flag")
	tm.tabModeActive = true

	// Apply visual border indicator
	logging.Debug("[tab-mode] About to call applyTabModeBorder()")
	tm.applyTabModeBorder()
	logging.Debug("[tab-mode] Returned from applyTabModeBorder()")

	// Notify JavaScript for UI feedback (if needed)
	logging.Debug("[tab-mode] About to call dispatchTabModeEventInternal()")
	tm.dispatchTabModeEventInternal("entered", "")
	logging.Debug("[tab-mode] Returned from dispatchTabModeEventInternal()")

	// Start timeout timer to auto-exit tab mode
	logging.Debug("[tab-mode] Getting config")
	cfg := tm.getConfig()
	timeoutDuration := time.Duration(cfg.Workspace.TabMode.TimeoutMilliseconds) * time.Millisecond
	logging.Debug(fmt.Sprintf("[tab-mode] Config retrieved, timeout: %v", timeoutDuration))

	// Cancel existing timer if any
	if tm.tabModeTimer != nil {
		logging.Debug("[tab-mode] Stopping existing timer")
		tm.tabModeTimer.Stop()
	}

	logging.Debug("[tab-mode] Creating new timeout timer")
	tm.tabModeTimer = time.AfterFunc(timeoutDuration, func() {
		logging.Debug("[tab-mode] Timeout reached, exiting tab mode")
		tm.ExitTabMode("timeout")
	})

	logging.Debug(fmt.Sprintf("[tab-mode] Tab mode entered successfully, will auto-exit in %v", timeoutDuration))
}

// ExitTabMode deactivates tab mode and removes visual indicators.
func (tm *TabManager) ExitTabMode(reason string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !tm.tabModeActive {
		return
	}

	logging.Info(fmt.Sprintf("[tab-mode] Exiting tab mode: %s", reason))

	tm.tabModeActive = false

	// Stop timeout timer
	if tm.tabModeTimer != nil {
		tm.tabModeTimer.Stop()
		tm.tabModeTimer = nil
	}

	// Remove visual border indicator
	tm.removeTabModeBorder()

	// Notify JavaScript (use internal version to avoid lock reentry)
	tm.dispatchTabModeEventInternal("exited", reason)
}

// HandleTabAction processes a tab mode action (new-tab, close-tab, etc.).
func (tm *TabManager) HandleTabAction(action string) {
	tm.mu.Lock()

	if !tm.tabModeActive {
		tm.mu.Unlock()
		logging.Debug(fmt.Sprintf("[tab-mode] Ignoring action '%s': tab mode not active", action))
		return
	}

	tm.mu.Unlock()

	logging.Info(fmt.Sprintf("[tab-mode] Handling action '%s'", action))

	switch action {
	case "new-tab":
		tm.handleNewTab()
	case "close-tab":
		tm.handleCloseTab()
	case "next-tab":
		tm.handleNextTab()
	case "previous-tab":
		tm.handlePreviousTab()
	case "rename-tab":
		tm.handleRenameTab()
	case "confirm":
		// Just exit tab mode
		tm.ExitTabMode("confirm")
	case "cancel":
		// Exit tab mode without action
		tm.ExitTabMode("cancel")
	default:
		logging.Warn(fmt.Sprintf("[tab-mode] Unknown action: %s", action))
	}

	// Exit tab mode after most actions (except rename which shows a dialog)
	if action != "cancel" && action != "confirm" && action != "rename-tab" {
		tm.ExitTabMode(action)
	}
}

// handleNewTab creates a new tab.
func (tm *TabManager) handleNewTab() {
	logging.Debug("[tab-mode] Creating new tab")

	if err := tm.CreateTab(""); err != nil {
		logging.Error(fmt.Sprintf("[tab-mode] Failed to create new tab: %v", err))
	}
}

// handleCloseTab closes the currently active tab.
func (tm *TabManager) handleCloseTab() {
	logging.Debug("[tab-mode] Closing active tab")

	tm.mu.RLock()
	activeIndex := tm.activeIndex
	tabCount := len(tm.tabs)
	tm.mu.RUnlock()

	if tabCount <= 1 {
		logging.Warn("[tab-mode] Cannot close last tab")
		return
	}

	if err := tm.CloseTab(activeIndex); err != nil {
		logging.Error(fmt.Sprintf("[tab-mode] Failed to close tab: %v", err))
	}
}

// handleNextTab switches to the next tab.
func (tm *TabManager) handleNextTab() {
	logging.Debug("[tab-mode] Switching to next tab")

	if err := tm.NextTab(); err != nil {
		logging.Error(fmt.Sprintf("[tab-mode] Failed to switch to next tab: %v", err))
	}
}

// handlePreviousTab switches to the previous tab.
func (tm *TabManager) handlePreviousTab() {
	logging.Debug("[tab-mode] Switching to previous tab")

	if err := tm.PreviousTab(); err != nil {
		logging.Error(fmt.Sprintf("[tab-mode] Failed to switch to previous tab: %v", err))
	}
}

// handleRenameTab shows a dialog to rename the active tab.
func (tm *TabManager) handleRenameTab() {
	logging.Debug("[tab-mode] Opening rename dialog")

	// Exit tab mode first so the dialog can be interacted with
	tm.ExitTabMode("rename-dialog")

	tm.mu.RLock()
	activeIndex := tm.activeIndex
	var currentTitle string
	if activeIndex >= 0 && activeIndex < len(tm.tabs) {
		tab := tm.tabs[activeIndex]
		if tab.customTitle != "" {
			currentTitle = tab.customTitle
		} else {
			currentTitle = tab.title
		}
	}
	tm.mu.RUnlock()

	if activeIndex < 0 {
		logging.Error("[tab-mode] No active tab to rename")
		return
	}

	// Show rename dialog (will be implemented in Step 10)
	// For now, just log
	logging.Info(fmt.Sprintf("[tab-mode] Would show rename dialog for tab %d (current: %s)", activeIndex, currentTitle))

	// TODO: Implement GTK dialog in Step 10
	// tm.showRenameDialog(activeIndex, currentTitle)
}

// applyTabModeBorder applies visual indicator when tab mode is active.
// This adds an orange border/highlight to the tab bar.
func (tm *TabManager) applyTabModeBorder() {
	logging.Debug("[tab-mode] applyTabModeBorder: entry")

	if tm.window == nil {
		logging.Debug("[tab-mode] applyTabModeBorder: window is nil, returning")
		return
	}

	logging.Debug("[tab-mode] applyTabModeBorder: window exists, converting to gtk.Window")
	gtkWindow := tm.window.AsWindow()
	logging.Debug("[tab-mode] applyTabModeBorder: got gtk.Window, about to add CSS class")

	// Add CSS class to window for tab mode visual indicator
	// Note: Call directly without RunOnMainThread wrapper (matches pane mode pattern)
	webkit.WidgetAddCSSClass(gtkWindow, "tab-mode-active")
	logging.Debug("[tab-mode] applyTabModeBorder: CSS class added successfully")
}

// removeTabModeBorder removes the visual indicator when exiting tab mode.
func (tm *TabManager) removeTabModeBorder() {
	if tm.window == nil {
		return
	}

	// Remove CSS class from window
	// Note: Call directly without RunOnMainThread wrapper (matches pane mode pattern)
	webkit.WidgetRemoveCSSClass(tm.window.AsWindow(), "tab-mode-active")
	logging.Debug("[tab-mode] Removed tab mode visual indicator")
}

// dispatchTabModeEventInternal sends an event without acquiring locks.
// Must be called with lock already held.
func (tm *TabManager) dispatchTabModeEventInternal(event string, detail string) {
	logging.Debug(fmt.Sprintf("[tab-mode] dispatchTabModeEventInternal: entry (event=%s, detail=%s)", event, detail))

	// Get active tab
	if tm.activeIndex < 0 || tm.activeIndex >= len(tm.tabs) {
		logging.Debug(fmt.Sprintf("[tab-mode] dispatchTabModeEventInternal: invalid activeIndex=%d, returning", tm.activeIndex))
		return
	}

	activeTab := tm.tabs[tm.activeIndex]
	if activeTab == nil {
		logging.Debug("[tab-mode] dispatchTabModeEventInternal: activeTab is nil, returning")
		return
	}

	// Log tab mode events for debugging (no JS dispatch needed for simple text tab labels)
	logging.Debug(fmt.Sprintf("[tab-mode] Tab mode event '%s' with detail '%s' for tab %s", event, detail, activeTab.id))
	logging.Debug("[tab-mode] dispatchTabModeEventInternal: completed successfully")
}

// dispatchTabModeEvent sends a custom event to the active tab's webview (with locking).
// This can be used to provide UI feedback in the web content.
func (tm *TabManager) dispatchTabModeEvent(event string, detail string) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	tm.dispatchTabModeEventInternal(event, detail)
}

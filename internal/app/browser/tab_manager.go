package browser

import (
	"fmt"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// TabManager manages the global tab system for the browser.
// Each tab contains an independent workspace with its own pane tree.
type TabManager struct {
	tabs        []*Tab
	activeIndex int
	window      *webkit.Window
	app         *BrowserApp

	// GTK widgets
	rootContainer gtk.Widgetter // Main vertical box containing tab bar + content
	tabBar        gtk.Widgetter // Horizontal tab bar container
	ContentArea   gtk.Widgetter // Container for active workspace (exported for border styling)
	tabModeTarget gtk.Widgetter // Widget currently holding tab mode border margins
	progressBar   gtk.Widgetter // Thin load progress indicator

	// Modal state
	tabModeActive bool
	tabModeTimer  *time.Timer
	lastTabCreate time.Time // Debounce guard for rapid tab creation shortcuts

	// Rename state
	renameInProgress bool
	renamingTab      *Tab
	// Stores original focus behaviour while inline rename is active
	renamePrevFocusOnClick bool
	renamePrevCanFocus     bool

	// Tab button click handling (pattern from StackedPaneManager)
	buttonToTab  map[uint64]*Tab // Maps button ID to tab for click handling
	nextButtonID uint64          // Atomic counter for generating unique button IDs

	// Synchronization
	mu sync.RWMutex
}

// Tab represents a single browser tab containing a complete workspace.
type Tab struct {
	id          string
	title       string
	customTitle string // User-provided custom name (persists across page loads)
	workspace   *WorkspaceManager
	titleButton gtk.Widgetter
	isActive    bool
	progress    float64
	isLoading   bool
	webViews    map[*webkit.WebView]struct{}
}

// NewTabManager creates a new tab manager instance.
func NewTabManager(app *BrowserApp, window *webkit.Window) *TabManager {
	tm := &TabManager{
		tabs:        make([]*Tab, 0),
		activeIndex: -1,
		window:      window,
		app:         app,
		buttonToTab: make(map[uint64]*Tab),
	}

	logging.Info("[tabs] Tab manager initialized")
	return tm
}

// Initialize sets up the tab bar and creates the initial tab.
func (tm *TabManager) Initialize(initialURL string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	logging.Info("[tabs] Initializing tab system")

	// Create root container layout
	if err := tm.createRootContainer(); err != nil {
		return fmt.Errorf("failed to create root container: %w", err)
	}

	// Create initial tab
	if err := tm.createTabInternal(initialURL); err != nil {
		return fmt.Errorf("failed to create initial tab: %w", err)
	}

	logging.Info("[tabs] Tab system initialized with 1 tab")
	return nil
}

// createRootContainer creates the main vertical container with tab bar and content area.
func (tm *TabManager) createRootContainer() error {
	logging.Info("[tabs] Creating root container with tab bar")

	// Get tab bar position from config
	cfg := tm.getConfig()
	position := cfg.Workspace.TabBarPosition
	if position != "top" && position != "bottom" {
		position = "bottom" // Fallback to default
	}

	logging.Debug(fmt.Sprintf("[tabs] Tab bar position: %s", position))

	// Create main vertical box (tab bar + content area)
	rootBox := gtk.NewBox(gtk.OrientationVertical, 0)
	if rootBox == nil {
		return fmt.Errorf("failed to create root container box")
	}

	rootBox.SetHExpand(true)
	rootBox.SetVExpand(true)

	// Create tab bar
	tabBar := tm.createTabBar()
	if tabBar == nil {
		return fmt.Errorf("failed to create tab bar")
	}

	// Create content area for active workspace
	contentArea := gtk.NewBox(gtk.OrientationVertical, 0)
	if contentArea == nil {
		return fmt.Errorf("failed to create content area")
	}
	contentArea.SetHExpand(true)
	contentArea.SetVExpand(true)
	contentArea.AddCSSClass("tab-content-area")

	// Create progress bar (displayed at the bottom regardless of tab bar position)
	progressBar := tm.createProgressBar()

	// Add widgets in order based on position
	if position == "top" {
		rootBox.Append(tabBar)
		rootBox.Append(contentArea)
	} else {
		rootBox.Append(contentArea)
		rootBox.Append(tabBar)
	}

	// Wrap everything in an overlay so the progress bar can float without resizing layout
	rootOverlay := gtk.NewOverlay()
	if rootOverlay == nil {
		return fmt.Errorf("failed to create root overlay")
	}
	rootOverlay.SetHExpand(true)
	rootOverlay.SetVExpand(true)
	rootOverlay.SetChild(rootBox)
	if progressBar != nil {
		if bar, ok := progressBar.(*gtk.ProgressBar); ok && bar != nil {
			bar.SetHAlign(gtk.AlignFill)
			bar.SetVAlign(gtk.AlignEnd)
		}
		rootOverlay.AddOverlay(progressBar)
	}

	// Store references
	tm.rootContainer = rootOverlay
	tm.tabBar = tabBar
	tm.ContentArea = contentArea
	tm.progressBar = progressBar

	logging.Info(fmt.Sprintf("[tabs] Root container created with tab bar at %s", position))
	return nil
}

// CreateTab creates a new tab and switches to it.
// URL can be empty string to use default homepage.
func (tm *TabManager) CreateTab(url string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	return tm.createTabInternal(url)
}

// createTabInternal creates a new tab (must be called with lock held).
func (tm *TabManager) createTabInternal(url string) error {
	logging.Info(fmt.Sprintf("[tabs] Creating new tab with URL: %s", url))

	const tabCreateDebounce = 200 * time.Millisecond
	if !tm.lastTabCreate.IsZero() && time.Since(tm.lastTabCreate) < tabCreateDebounce {
		logging.Warn("[tabs] Ignoring rapid tab creation request (debounced)")
		return nil
	}

	// Use default URL if empty
	if url == "" {
		url = "about:blank"
	}

	// Generate unique tab ID and default name
	tabNumber := len(tm.tabs) + 1
	tabID := fmt.Sprintf("tab-%d-%d", tabNumber, time.Now().Unix())
	defaultTitle := fmt.Sprintf("Tab %d", tabNumber)

	// Create tab structure
	tab := &Tab{
		id:       tabID,
		title:    defaultTitle,
		isActive: false,
		webViews: make(map[*webkit.WebView]struct{}),
	}

	// Create WebView for this tab
	cfg, err := tm.app.buildWebkitConfig()
	if err != nil {
		return fmt.Errorf("failed to build webkit config: %w", err)
	}
	cfg.CreateWindow = false // Tab webviews are embedded, don't create their own window

	view, err := webkit.NewWebView(cfg)
	if err != nil {
		return fmt.Errorf("failed to create webview: %w", err)
	}

	// Create pane for this WebView
	pane, err := tm.app.createPaneForView(view)
	if err != nil {
		return fmt.Errorf("failed to create pane: %w", err)
	}

	// Create workspace for this tab
	workspace := NewWorkspaceManager(tm.app, pane)
	tab.workspace = workspace
	tm.registerWebViewLocked(tab, view)

	// Verify workspace root exists
	rootWidget := workspace.GetRootWidget()
	if rootWidget == nil {
		return fmt.Errorf("workspace root widget is nil")
	}
	logging.Info(fmt.Sprintf("[tabs] New tab created with root container: %p (workspace %p)", rootWidget, workspace))

	// Load the URL in the workspace's pane
	if url != "" {
		if err := view.LoadURL(url); err != nil {
			logging.Warn(fmt.Sprintf("[tabs] Failed to load URL %s: %v", url, err))
		}
	}

	// Create tab button in tab bar
	button := tm.createTabButton(tab)
	if button == nil {
		return fmt.Errorf("failed to create tab button")
	}
	tab.titleButton = button

	// Add button to tab bar
	tm.addTabToBar(tab)

	// Add to tabs slice
	tm.tabs = append(tm.tabs, tab)
	tm.lastTabCreate = time.Now()

	// Attach click handler with unique ID (pattern from stacked panes)
	tm.attachTabClickHandler(tab.titleButton, tab)

	// Switch to new tab
	newIndex := len(tm.tabs) - 1
	if err := tm.switchToTabInternal(newIndex); err != nil {
		return fmt.Errorf("failed to switch to new tab: %w", err)
	}

	// Update tab bar visibility (hide if only 1 tab)
	tm.updateTabBarVisibility()

	logging.Info(fmt.Sprintf("[tabs] Created tab %s (total: %d)", tabID, len(tm.tabs)))
	return nil
}

// SwitchToTab switches to the tab at the given index.
func (tm *TabManager) SwitchToTab(index int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	return tm.switchToTabInternal(index)
}

// switchToTabInternal switches to a tab (must be called with lock held).
func (tm *TabManager) switchToTabInternal(index int) error {
	// Validate index
	if index < 0 || index >= len(tm.tabs) {
		logging.Warn(fmt.Sprintf("[tabs] Invalid tab index: %d (valid: 0-%d)", index, len(tm.tabs)-1))
		return fmt.Errorf("tab index %d out of range", index)
	}

	// Check if already active
	if index == tm.activeIndex {
		logging.Debug(fmt.Sprintf("[tabs] Tab %d already active", index))
		return nil
	}

	logging.Info(fmt.Sprintf("[tabs] Switching from tab %d to %d", tm.activeIndex, index))

	// Hide current tab if any
	if tm.activeIndex >= 0 && tm.activeIndex < len(tm.tabs) {
		oldTab := tm.tabs[tm.activeIndex]
		oldTab.isActive = false

		// Clear GTK focus from old tab's workspace to prevent it from catching keyboard events
		if oldTab.workspace != nil {
			oldTab.workspace.ClearFocus()
		}

		// Hide old workspace container and remove from content area
		// CRITICAL: Get the CURRENT root widget from workspace, not the cached tab.container,
		// because splits/stacks change the workspace root
		if oldTab.workspace != nil {
			currentRoot := oldTab.workspace.GetRootWidget()
			if currentRoot != nil {
				webkit.RunOnMainThread(func() {
					if contentBox, ok := tm.ContentArea.(*gtk.Box); ok {
						contentBox.Remove(currentRoot)
						logging.Info(fmt.Sprintf("[tabs] Removed tab %d container %p from ContentArea (current root)", tm.activeIndex, currentRoot))
					}
				})
			}
		}

		// Remove active CSS class from old button
		tm.setTabActiveStyle(oldTab, false)
	}

	// Show new tab
	newTab := tm.tabs[index]
	newTab.isActive = true
	tm.activeIndex = index

	// Add new workspace container to content area
	// CRITICAL: Get the CURRENT root widget from workspace, not the cached tab.container
	if newTab.workspace != nil {
		currentRoot := newTab.workspace.GetRootWidget()
		if currentRoot != nil {
			webkit.RunOnMainThread(func() {
				if contentBox, ok := tm.ContentArea.(*gtk.Box); ok {
					contentBox.Append(currentRoot)
					webkit.WidgetSetVisible(currentRoot, true)
					logging.Info(fmt.Sprintf("[tabs] Added tab %d container %p to ContentArea (current root)", index, currentRoot))
				}
			})
		}
	}

	// Add active CSS class to new button
	tm.setTabActiveStyle(newTab, true)

	// Update app-level workspace/panes references for compatibility
	if newTab.workspace != nil {
		tm.app.workspace = newTab.workspace
		tm.app.panes = newTab.workspace.GetAllPanes()
		tm.app.activePane = newTab.workspace.GetActivePane()

		// Focus the new workspace's active pane
		newTab.workspace.RestoreFocus()
	}

	// Sync progress bar with the newly active tab
	tm.updateProgressBarLocked()

	logging.Info(fmt.Sprintf("[tabs] Switched to tab %d (%s)", index, newTab.id))
	return nil
}

// CloseTab closes the tab at the given index.
func (tm *TabManager) CloseTab(index int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	return tm.closeTabInternal(index)
}

// closeTabInternal closes a tab (must be called with lock held).
func (tm *TabManager) closeTabInternal(index int) error {
	// Validate index
	if index < 0 || index >= len(tm.tabs) {
		logging.Warn(fmt.Sprintf("[tabs] Cannot close: invalid tab index %d", index))
		return fmt.Errorf("tab index %d out of range", index)
	}

	// Prevent closing last tab
	if len(tm.tabs) == 1 {
		logging.Warn("[tabs] Cannot close last tab")
		return fmt.Errorf("cannot close the last tab")
	}

	tab := tm.tabs[index]
	logging.Info(fmt.Sprintf("[tabs] Closing tab %d (%s)", index, tab.id))

	// Cleanup workspace (WorkspaceManager cleanup is handled automatically when tabs are removed)

	// Clean up button mapping (find and remove this tab's button ID)
	for buttonID, mappedTab := range tm.buttonToTab {
		if mappedTab == tab {
			delete(tm.buttonToTab, buttonID)
			logging.Debug(fmt.Sprintf("[tabs] Cleaned up button mapping for closed tab %s (buttonID=%d)", tab.id, buttonID))
			break
		}
	}

	// Remove tab button from tab bar
	tm.removeTabFromBar(tab)

	// Remove workspace container from content area if it's currently visible
	// CRITICAL: Get the CURRENT root widget from workspace, not the cached tab.container
	if tab.isActive && tab.workspace != nil {
		currentRoot := tab.workspace.GetRootWidget()
		if currentRoot != nil {
			webkit.RunOnMainThread(func() {
				if contentBox, ok := tm.ContentArea.(*gtk.Box); ok {
					contentBox.Remove(currentRoot)
					logging.Info(fmt.Sprintf("[tabs] Removed closing tab %d container %p from ContentArea (current root)", index, currentRoot))
				}
			})
		}
	}

	// Remove from slice
	tm.tabs = append(tm.tabs[:index], tm.tabs[index+1:]...)

	// Determine which tab to switch to
	newIndex := index
	if index >= len(tm.tabs) {
		newIndex = len(tm.tabs) - 1
	}

	// Update active index and switch
	if index == tm.activeIndex {
		tm.activeIndex = -1 // Temporarily invalid
		if err := tm.switchToTabInternal(newIndex); err != nil {
			return fmt.Errorf("failed to switch after close: %w", err)
		}
	} else if index < tm.activeIndex {
		// Adjust active index if a tab before it was closed
		tm.activeIndex--
	}

	// Update tab bar visibility (hide if only 1 tab)
	tm.updateTabBarVisibility()

	// Update progress bar (active tab may have changed after close)
	tm.updateProgressBarLocked()

	logging.Info(fmt.Sprintf("[tabs] Tab closed (remaining: %d)", len(tm.tabs)))
	return nil
}

// RenameTab sets a custom title for the tab at the given index.
func (tm *TabManager) RenameTab(index int, customTitle string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if index < 0 || index >= len(tm.tabs) {
		return fmt.Errorf("tab index %d out of range", index)
	}

	tab := tm.tabs[index]
	tab.customTitle = customTitle

	logging.Info(fmt.Sprintf("[tabs] Renamed tab %d to '%s'", index, customTitle))

	// Update tab button label
	tm.updateTabButton(tab)

	return nil
}

// Note: Tab titles are static (Tab 1, Tab 2, etc.) and don't change based on page titles.
// This is different from stacked panes which show page titles.
// Users can only change tab names via explicit rename action (Alt+T → r)

// GetActiveTab returns the currently active tab.
func (tm *TabManager) GetActiveTab() *Tab {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.activeIndex >= 0 && tm.activeIndex < len(tm.tabs) {
		return tm.tabs[tm.activeIndex]
	}
	return nil
}

// GetTabCount returns the number of open tabs.
func (tm *TabManager) GetTabCount() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return len(tm.tabs)
}

// IsTabModeActive returns whether tab mode is currently active.
func (tm *TabManager) IsTabModeActive() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.tabModeActive
}

// IsRenameInProgress returns whether a tab rename is currently in progress.
func (tm *TabManager) IsRenameInProgress() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.renameInProgress
}

// NextTab switches to the next tab (wraps around).
func (tm *TabManager) NextTab() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.tabs) <= 1 {
		return nil // No other tab to switch to
	}

	nextIndex := (tm.activeIndex + 1) % len(tm.tabs)
	return tm.switchToTabInternal(nextIndex)
}

// PreviousTab switches to the previous tab (wraps around).
func (tm *TabManager) PreviousTab() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.tabs) <= 1 {
		return nil // No other tab to switch to
	}

	prevIndex := tm.activeIndex - 1
	if prevIndex < 0 {
		prevIndex = len(tm.tabs) - 1
	}
	return tm.switchToTabInternal(prevIndex)
}

// GetRootContainer returns the main container widget that should be set as window child.
func (tm *TabManager) GetRootContainer() gtk.Widgetter {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.rootContainer
}

// GetWindow returns the GTK window backing the tab manager.
func (tm *TabManager) GetWindow() *webkit.Window {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.window
}

// Cleanup performs cleanup of all tabs and resources.
func (tm *TabManager) Cleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	logging.Info("[tabs] Cleaning up tab manager")

	// Stop tab mode timer if active
	if tm.tabModeTimer != nil {
		tm.tabModeTimer.Stop()
	}

	// Cleanup all tabs (workspaces cleanup automatically when dereferenced)

	logging.Info("[tabs] Tab manager cleanup complete")
}

// updateTabBarVisibility shows or hides the tab bar based on tab count.
// Tab bar is hidden when there's only 1 tab (no need to show it).
func (tm *TabManager) updateTabBarVisibility() {
	if tm.tabBar == nil {
		return
	}

	tabCount := len(tm.tabs)
	shouldShow := tabCount > 1

	webkit.WidgetSetVisible(tm.tabBar, shouldShow)

	if shouldShow {
		logging.Debug(fmt.Sprintf("[tabs] Tab bar visible (%d tabs)", tabCount))
	} else {
		logging.Debug("[tabs] Tab bar hidden (only 1 tab)")
	}
}

// GetConfig is a helper to get the current configuration.
func (tm *TabManager) getConfig() *config.Config {
	return config.Get()
}

// updateProgressForWebView updates progress state for the tab that owns the given WebView.
func (tm *TabManager) updateProgressForWebView(view *webkit.WebView, progress float64, loading bool) {
	if view == nil || tm == nil {
		return
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	tab := tm.findTabForViewLocked(view)
	if tab == nil {
		return
	}

	// Clamp progress
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	// Only mark loading when progress is incomplete and the event indicates an active load.
	tab.isLoading = loading && progress < 1.0
	tab.progress = progress

	tm.updateProgressBarLocked()
}

// findTabForViewLocked finds the tab owning the given WebView. Caller must hold tm.mu.
func (tm *TabManager) findTabForViewLocked(view *webkit.WebView) *Tab {
	for _, tab := range tm.tabs {
		if _, ok := tab.webViews[view]; ok {
			return tab
		}
	}
	return nil
}

// registerWebViewLocked associates a WebView with a tab. Caller must hold tm.mu.
func (tm *TabManager) registerWebViewLocked(tab *Tab, view *webkit.WebView) {
	if tab == nil || view == nil {
		return
	}
	if tab.webViews == nil {
		tab.webViews = make(map[*webkit.WebView]struct{})
	}
	tab.webViews[view] = struct{}{}
}

// registerWebViewForWorkspace registers a webview to the tab that owns the workspace.
func (tm *TabManager) registerWebViewForWorkspace(view *webkit.WebView, workspace *WorkspaceManager) {
	if tm == nil || view == nil || workspace == nil {
		return
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, tab := range tm.tabs {
		if tab.workspace == workspace {
			tm.registerWebViewLocked(tab, view)
			return
		}
	}
}

// updateProgressBarLocked syncs the shared progress bar to the active tab. Caller must hold tm.mu.
func (tm *TabManager) updateProgressBarLocked() {
	if tm.progressBar == nil {
		return
	}

	bar, ok := tm.progressBar.(*gtk.ProgressBar)
	if !ok || bar == nil {
		return
	}

	// No active tab -> hide bar
	if tm.activeIndex < 0 || tm.activeIndex >= len(tm.tabs) {
		webkit.RunOnMainThread(func() {
			bar.SetFraction(0.0)
			webkit.WidgetSetVisible(bar, false)
		})
		return
	}

	activeTab := tm.tabs[tm.activeIndex]
	visible := activeTab.isLoading
	fraction := activeTab.progress
	if !visible {
		fraction = 0.0
	}

	webkit.RunOnMainThread(func() {
		bar.SetFraction(fraction)
		webkit.WidgetSetVisible(bar, visible)
	})
}

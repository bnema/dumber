package coordinator

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/window"
)

// TabScopeFunc filters whether a tab belongs to the current window scope.
// Returns true if the tab should be considered for window-local operations.
type TabScopeFunc func(tab *entity.Tab, mainWindow *window.MainWindow) bool

// TabCoordinator manages tab lifecycle operations.
type TabCoordinator struct {
	tabsUC                  *usecase.ManageTabsUseCase
	tabs                    *entity.TabList
	mainWindow              *window.MainWindow
	hideTabBarWhenSingleTab bool

	// Window-scoped tab filtering (per-window tab ownership)
	tabScope TabScopeFunc

	// Callbacks to avoid circular dependencies
	onTabCreated         func(ctx context.Context, tab *entity.Tab)
	onTabSwitched        func(ctx context.Context, tab *entity.Tab)
	onQuit               func()
	onCurrentWindowEmpty func(ctx context.Context, mainWindow *window.MainWindow)
	onAttachPopupToTab   func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv port.WebView) // For popup tabs
	onStateChanged       func()                                                                            // For session snapshots

	// previousActiveTabID provides the per-window previous active tab ID for Alt+Tab style switching.
	// When set and tabScope is active, SwitchToLastActive prefers this over the global PreviousActiveTabID.
	previousActiveTabID func(mainWindow *window.MainWindow) entity.TabID
}

// TabCoordinatorConfig holds configuration for TabCoordinator.
type TabCoordinatorConfig struct {
	TabsUC                  *usecase.ManageTabsUseCase
	Tabs                    *entity.TabList
	MainWindow              *window.MainWindow
	HideTabBarWhenSingleTab bool
}

// NewTabCoordinator creates a new TabCoordinator.
func NewTabCoordinator(ctx context.Context, cfg TabCoordinatorConfig) *TabCoordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating tab coordinator")

	return &TabCoordinator{
		tabsUC:                  cfg.TabsUC,
		tabs:                    cfg.Tabs,
		mainWindow:              cfg.MainWindow,
		hideTabBarWhenSingleTab: cfg.HideTabBarWhenSingleTab,
	}
}

// SetOnTabCreated sets the callback for when a tab is created.
func (c *TabCoordinator) SetOnTabCreated(fn func(ctx context.Context, tab *entity.Tab)) {
	c.onTabCreated = fn
}

// SetOnTabSwitched sets the callback for when a tab switch occurs.
// This is used to swap workspace views in the content area.
func (c *TabCoordinator) SetOnTabSwitched(fn func(ctx context.Context, tab *entity.Tab)) {
	c.onTabSwitched = fn
}

// SetOnQuit sets the callback for when the last tab is closed.
func (c *TabCoordinator) SetOnQuit(fn func()) {
	c.onQuit = fn
}

// SetOnStateChanged sets the callback for when tab state changes (for session snapshots).
func (c *TabCoordinator) SetOnStateChanged(fn func()) {
	c.onStateChanged = fn
}

// SetPreviousActiveTabIDProvider sets the callback for retrieving the per-window
// previous active tab ID. When set and tabScope is active, SwitchToLastActive
// prefers this provider over the global PreviousActiveTabID.
func (c *TabCoordinator) SetPreviousActiveTabIDProvider(fn func(mainWindow *window.MainWindow) entity.TabID) {
	c.previousActiveTabID = fn
}

// SetTabScope sets the function used to filter tabs to the current window.
// When set, tab operations (switch by index, next/prev, close) only consider
// tabs for which the scope function returns true.
func (c *TabCoordinator) SetTabScope(fn TabScopeFunc) {
	c.tabScope = fn
}

// SetOnCurrentWindowEmpty sets the callback for when the last tab in the current
// window is closed. This allows the app to close the window gracefully.
func (c *TabCoordinator) SetOnCurrentWindowEmpty(fn func(ctx context.Context, mainWindow *window.MainWindow)) {
	c.onCurrentWindowEmpty = fn
}

// scopedTabs returns tabs filtered by the tabScope function.
// When no scope function is set, returns all tabs (backward compatible).
func (c *TabCoordinator) scopedTabs() []*entity.Tab {
	if c.tabs == nil {
		return nil
	}

	if c.tabScope == nil {
		return c.tabs.Tabs
	}

	scoped := make([]*entity.Tab, 0, len(c.tabs.Tabs))
	for _, tab := range c.tabs.Tabs {
		if tab != nil && c.tabScope(tab, c.mainWindow) {
			scoped = append(scoped, tab)
		}
	}
	return scoped
}

// indexOfTab returns the 0-based index of the given tab ID in a slice of tabs,
// or -1 if not found.
func indexOfTab(tabs []*entity.Tab, id entity.TabID) int {
	for i, tab := range tabs {
		if tab != nil && tab.ID == id {
			return i
		}
	}
	return -1
}

// SetMainWindow updates the window targeted by tab UI operations.
func (c *TabCoordinator) SetMainWindow(mainWindow *window.MainWindow) {
	c.mainWindow = mainWindow
}

// notifyStateChanged triggers the state changed callback if set.
func (c *TabCoordinator) notifyStateChanged() {
	if c.onStateChanged != nil {
		c.onStateChanged()
	}
}

// Create creates a new tab with the given initial URL.
func (c *TabCoordinator) Create(ctx context.Context, initialURL string) (*entity.Tab, error) {
	return c.create(ctx, initialURL, true)
}

func (c *TabCoordinator) create(ctx context.Context, initialURL string, activate bool) (*entity.Tab, error) {
	log := logging.FromContext(ctx)

	output, err := c.tabsUC.Create(ctx, usecase.CreateTabInput{
		TabList:    c.tabs,
		Name:       "",
		InitialURL: initialURL,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create tab")
		return nil, err
	}

	if activate {
		// Set new tab as active (updates domain state and tracks previous)
		c.tabs.SetActive(output.Tab.ID)
	}

	// Notify app before adding the tab to the visible tab bar.
	// The app assigns window ownership and the window-scoped default name here,
	// and TabBar.AddTab reads tab.Title() immediately when creating the button.
	if c.onTabCreated != nil {
		c.onTabCreated(ctx, output.Tab)
	}

	// Update tab bar
	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().AddTab(output.Tab)
		if activate {
			c.mainWindow.TabBar().SetActive(output.Tab.ID)
		}
	}

	// Update tab bar visibility
	c.UpdateBarVisibility(ctx)

	// Switch to the new tab's workspace view when activation is requested.
	if activate && c.onTabSwitched != nil {
		c.onTabSwitched(ctx, output.Tab)
	}

	// Notify state change for session snapshots
	c.notifyStateChanged()

	log.Debug().Str("tab_id", string(output.Tab.ID)).Msg("tab created")
	return output.Tab, nil
}

// Close closes the active tab.
// When scoped, prefers switching to a sibling tab in the same window.
// If this was the last tab in the window, fires onCurrentWindowEmpty.
// If this was the last tab globally, fires onQuit.
func (c *TabCoordinator) Close(ctx context.Context) error {
	log := logging.FromContext(ctx)

	activeID := c.tabs.ActiveTabID
	if activeID == "" {
		log.Debug().Msg("no active tab to close")
		return nil
	}

	// Compute the next scoped tab to activate BEFORE closing
	scoped := c.scopedTabs()
	activeIdx := indexOfTab(scoped, activeID)
	if activeIdx < 0 && len(scoped) > 0 {
		log.Debug().Str("active_tab_id", string(activeID)).Int("scoped_count", len(scoped)).Msg("active tab not in scoped list")
	}

	var nextScopedID entity.TabID
	if activeIdx >= 0 && len(scoped) > 1 {
		if activeIdx < len(scoped)-1 {
			nextScopedID = scoped[activeIdx+1].ID
		} else {
			nextScopedID = scoped[activeIdx-1].ID
		}
	}

	wasLast, err := c.tabsUC.Close(ctx, c.tabs, activeID)
	if err != nil {
		log.Error().Err(err).Msg("failed to close tab")
		return err
	}

	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().RemoveTab(activeID)
	}

	// Update tab bar visibility
	c.UpdateBarVisibility(ctx)

	// Quit if no tabs left globally
	if wasLast {
		if c.onQuit != nil {
			c.onQuit()
		}
		log.Debug().Str("tab_id", string(activeID)).Bool("was_last", wasLast).Msg("tab closed, last globally")
		return nil
	}

	// If there's a sibling tab in the same window, switch to it
	if nextScopedID != "" {
		if err := c.tabsUC.Switch(ctx, c.tabs, nextScopedID); err != nil {
			log.Error().Err(err).Str("tab_id", string(nextScopedID)).Msg("failed to switch to scoped sibling")
			return err
		}

		if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
			c.mainWindow.TabBar().SetActive(nextScopedID)
		}

		if c.onTabSwitched != nil {
			if tab := c.tabs.Find(nextScopedID); tab != nil {
				c.onTabSwitched(ctx, tab)
			}
		}

		// Notify state change for session snapshots
		c.notifyStateChanged()

		log.Debug().Str("tab_id", string(activeID)).Str("next", string(nextScopedID)).Msg("tab closed, switched to scoped sibling")
		return nil
	}

	// Last tab in this window but not globally last — close the window
	c.notifyStateChanged()
	if c.onCurrentWindowEmpty != nil {
		c.onCurrentWindowEmpty(ctx, c.mainWindow)
	}

	log.Debug().Str("tab_id", string(activeID)).Msg("tab closed, last in window")
	return nil
}

// Switch switches to a specific tab by ID.
func (c *TabCoordinator) Switch(ctx context.Context, tabID entity.TabID) error {
	log := logging.FromContext(ctx)

	// Skip if already active
	if tabID == c.tabs.ActiveTabID {
		return nil
	}

	// Reject switching to a tab outside the current window scope
	if tab := c.tabs.Find(tabID); tab != nil && c.tabScope != nil && !c.tabScope(tab, c.mainWindow) {
		log.Debug().
			Str("tab_id", string(tabID)).
			Msg("ignoring switch to tab outside current window scope")
		return nil
	}

	log.Debug().
		Str("from", string(c.tabs.ActiveTabID)).
		Str("to", string(tabID)).
		Msg("switching tab")

	// Update domain state
	if err := c.tabsUC.Switch(ctx, c.tabs, tabID); err != nil {
		log.Error().Err(err).Str("tab_id", string(tabID)).Msg("failed to switch tab")
		return err
	}

	// Update tab bar UI
	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().SetActive(tabID)
	}

	// Invoke callback for workspace view switching
	if c.onTabSwitched != nil {
		if tab := c.tabs.Find(tabID); tab != nil {
			c.onTabSwitched(ctx, tab)
		}
	}

	return nil
}

// switchRelative switches to the tab delta positions away within the current window scope.
// delta of +1 switches next, -1 switches previous, wrapping within the scoped list.
func (c *TabCoordinator) switchRelative(ctx context.Context, delta int) error {
	scoped := c.scopedTabs()
	if len(scoped) <= 1 {
		return nil
	}

	current := indexOfTab(scoped, c.tabs.ActiveTabID)
	if current < 0 {
		current = 0
	} else {
		current = (current + delta + len(scoped)) % len(scoped)
	}

	return c.Switch(ctx, scoped[current].ID)
}

// SwitchNext switches to the next tab within the current window scope.
func (c *TabCoordinator) SwitchNext(ctx context.Context) error {
	return c.switchRelative(ctx, 1)
}

// SwitchPrev switches to the previous tab within the current window scope.
func (c *TabCoordinator) SwitchPrev(ctx context.Context) error {
	return c.switchRelative(ctx, -1)
}

// SwitchByIndex switches to a tab by 0-based index within the current window scope.
func (c *TabCoordinator) SwitchByIndex(ctx context.Context, index int) error {
	log := logging.FromContext(ctx)

	scoped := c.scopedTabs()
	if index < 0 || index >= len(scoped) {
		log.Debug().Int("index", index).Int("scoped_count", len(scoped)).Msg("invalid scoped tab index")
		return nil
	}

	return c.Switch(ctx, scoped[index].ID)
}

// EnsureTabByIndex creates tabs until the requested index exists within the current
// window scope, then switches to it.
func (c *TabCoordinator) EnsureTabByIndex(ctx context.Context, index int, initialURL string) error {
	log := logging.FromContext(ctx)

	if index < 0 {
		log.Debug().Int("index", index).Msg("invalid tab index")
		return nil
	}

	if c.tabs == nil {
		return fmt.Errorf("tab list is required")
	}

	// Create tabs within the current window scope until we have enough
	scoped := c.scopedTabs()
	if len(scoped) <= index && initialURL == "" {
		log.Debug().Int("index", index).Int("scoped_count", len(scoped)).Msg("cannot auto-create missing tabs without initial URL")
		return fmt.Errorf("initial URL is required to auto-create missing tabs")
	}

	for len(c.scopedTabs()) <= index {
		if _, err := c.create(ctx, initialURL, false); err != nil {
			log.Error().Err(err).Int("index", index).Msg("failed to create tab while ensuring tab index")
			return err
		}
	}

	return c.SwitchByIndex(ctx, index)
}

// SwitchToLastActive switches to the previously active tab (Alt+Tab style).
// When window-scoped (tabScope is set and previousActiveTabID provider is available),
// it uses the per-window previous active tab ID. Falls back to the global
// PreviousActiveTabID otherwise.
func (c *TabCoordinator) SwitchToLastActive(ctx context.Context) error {
	log := logging.FromContext(ctx)

	var prevID entity.TabID
	if c.tabScope != nil && c.previousActiveTabID != nil {
		// Use per-window previous active tab ID when scoped
		prevID = c.previousActiveTabID(c.mainWindow)
	} else {
		// Fall back to global PreviousActiveTabID when not scoped or no provider
		prevID = c.tabs.PreviousActiveTabID
	}

	if prevID == "" || prevID == c.tabs.ActiveTabID {
		return nil
	}

	tab := c.tabs.Find(prevID)
	if tab == nil {
		// Only clear the global previous active tab ID if that's what we used
		if c.tabScope == nil || c.previousActiveTabID == nil {
			c.tabs.PreviousActiveTabID = ""
		}
		return nil
	}

	// With a scope function, only allow switching to the previous tab if it's in scope.
	if c.tabScope != nil && !c.tabScope(tab, c.mainWindow) {
		log.Debug().Str("tab_id", string(prevID)).Msg("previous active tab outside current window scope")
		return nil
	}

	return c.Switch(ctx, prevID)
}

// UpdateBarVisibility shows or hides the tab bar based on tab count.
func (c *TabCoordinator) UpdateBarVisibility(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Check if feature is enabled
	hideEnabled := c.hideTabBarWhenSingleTab

	if !hideEnabled {
		log.Debug().Msg("tab bar auto-hide disabled by config")
		return
	}

	if c.mainWindow == nil || c.mainWindow.TabBar() == nil {
		log.Debug().Msg("mainWindow or tabBar is nil, skipping visibility update")
		return
	}

	tabCount := c.mainWindow.TabBar().Count()
	shouldShow := tabCount > 1

	log.Debug().Int("tab_count", tabCount).Bool("should_show", shouldShow).Msg("setting tab bar visibility")
	c.mainWindow.TabBar().SetVisible(shouldShow)
}

// GetTabBar returns the tab bar component.
func (c *TabCoordinator) GetTabBar() *component.TabBar {
	if c.mainWindow != nil {
		return c.mainWindow.TabBar()
	}
	return nil
}

// SetOnAttachPopupToTab sets the callback for attaching popup WebViews to tabs.
func (c *TabCoordinator) SetOnAttachPopupToTab(fn func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv port.WebView)) {
	c.onAttachPopupToTab = fn
}

// CreateWithPane creates a new tab with a pre-created pane and WebView.
// This is used for tabbed popup behavior where the popup pane already exists.
func (c *TabCoordinator) CreateWithPane(
	ctx context.Context,
	pane *entity.Pane,
	wv port.WebView,
	initialURL string,
) (*entity.Tab, error) {
	log := logging.FromContext(ctx)

	output, err := c.tabsUC.CreateWithPane(ctx, usecase.CreateTabWithPaneInput{
		TabList:    c.tabs,
		Name:       pane.Title,
		Pane:       pane,
		InitialURL: initialURL,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create tab with pane")
		return nil, err
	}

	// Set new tab as active
	c.tabs.SetActive(output.Tab.ID)

	// Update tab bar
	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().AddTab(output.Tab)
		c.mainWindow.TabBar().SetActive(output.Tab.ID)
	}

	// Update tab bar visibility
	c.UpdateBarVisibility(ctx)

	// Notify app to create workspace view
	if c.onTabCreated != nil {
		c.onTabCreated(ctx, output.Tab)
	}

	// Attach the popup WebView to the new tab's workspace
	if c.onAttachPopupToTab != nil {
		c.onAttachPopupToTab(ctx, output.Tab.ID, pane, wv)
	}

	// Switch to the new tab's workspace view
	if c.onTabSwitched != nil {
		c.onTabSwitched(ctx, output.Tab)
	}

	// Notify state change for session snapshots
	c.notifyStateChanged()

	log.Debug().
		Str("tab_id", string(output.Tab.ID)).
		Str("pane_id", string(pane.ID)).
		Msg("tab created with popup pane")
	return output.Tab, nil
}

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

// TabTarget identifies which TabList and MainWindow a tab operation applies to.
// Each browser window owns a dedicated TabTarget; operations are scoped to that target.
type TabTarget struct {
	Tabs       *entity.TabList
	MainWindow *window.MainWindow
}

// TabCoordinator manages tab lifecycle operations within a target window.
// Public methods accept an explicit TabTarget so callers control which window's
// tabs are mutated. The currentTarget field provides a fallback for callers that
// operate on the "currently active" browser window (e.g., the keyboard dispatcher).
type TabCoordinator struct {
	tabsUC                  *usecase.ManageTabsUseCase
	currentTarget           TabTarget
	mainWindow              *window.MainWindow // retained for GetTabBar and SetMainWindow
	hideTabBarWhenSingleTab bool

	// Callbacks to avoid circular dependencies
	onTabCreated         func(ctx context.Context, target TabTarget, tab *entity.Tab)
	onTabSwitched        func(ctx context.Context, target TabTarget, tab *entity.Tab)
	onTabClosed          func(ctx context.Context, target TabTarget, tab *entity.Tab)
	onQuit               func()
	onCurrentWindowEmpty func(ctx context.Context, target TabTarget)
	onAttachPopupToTab   func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv port.WebView) // For popup tabs
	onStateChanged       func()                                                                            // For session snapshots
}

// TabCoordinatorConfig holds configuration for TabCoordinator.
type TabCoordinatorConfig struct {
	TabsUC                  *usecase.ManageTabsUseCase
	Tabs                    *entity.TabList // Initial tab list for the default target
	MainWindow              *window.MainWindow
	HideTabBarWhenSingleTab bool
}

// NewTabCoordinator creates a new TabCoordinator.
func NewTabCoordinator(ctx context.Context, cfg TabCoordinatorConfig) *TabCoordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating tab coordinator")

	return &TabCoordinator{
		tabsUC: cfg.TabsUC,
		currentTarget: TabTarget{
			Tabs:       cfg.Tabs,
			MainWindow: cfg.MainWindow,
		},
		mainWindow:              cfg.MainWindow,
		hideTabBarWhenSingleTab: cfg.HideTabBarWhenSingleTab,
	}
}

// SetCurrentTarget updates the fallback target used by methods that don't receive
// an explicit TabTarget. This should be called whenever the active browser window changes.
func (c *TabCoordinator) SetCurrentTarget(target TabTarget) {
	c.currentTarget = target
	if target.MainWindow != nil {
		c.mainWindow = target.MainWindow
	}
}

// CurrentTarget returns the currently active fallback target.
func (c *TabCoordinator) CurrentTarget() TabTarget {
	return c.currentTarget
}

// SetOnTabCreated sets the callback for when a tab is created.
func (c *TabCoordinator) SetOnTabCreated(fn func(ctx context.Context, target TabTarget, tab *entity.Tab)) {
	c.onTabCreated = fn
}

// SetOnTabSwitched sets the callback for when a tab switch occurs.
// This is used to swap workspace views in the content area.
func (c *TabCoordinator) SetOnTabSwitched(fn func(ctx context.Context, target TabTarget, tab *entity.Tab)) {
	c.onTabSwitched = fn
}

// SetOnTabClosed sets the callback for when a tab is removed from its target.
func (c *TabCoordinator) SetOnTabClosed(fn func(ctx context.Context, target TabTarget, tab *entity.Tab)) {
	c.onTabClosed = fn
}

// SetOnQuit sets the callback for when the last tab is closed.
func (c *TabCoordinator) SetOnQuit(fn func()) {
	c.onQuit = fn
}

// SetOnStateChanged sets the callback for when tab state changes (for session snapshots).
func (c *TabCoordinator) SetOnStateChanged(fn func()) {
	c.onStateChanged = fn
}

// SetOnCurrentWindowEmpty sets the callback for when the last tab in the current
// window is closed. This allows the app to close the window gracefully.
func (c *TabCoordinator) SetOnCurrentWindowEmpty(fn func(ctx context.Context, target TabTarget)) {
	c.onCurrentWindowEmpty = fn
}

// SetMainWindow updates only the main window reference for coordinator setup.
//
// Deprecated: Use SetCurrentTarget for runtime targeting. This method preserves
// currentTarget.Tabs and is safe only during app-level setup.
func (c *TabCoordinator) SetMainWindow(mainWindow *window.MainWindow) {
	oldMainWindow := c.mainWindow
	c.mainWindow = mainWindow
	if c.currentTarget.MainWindow != mainWindow {
		c.currentTarget = TabTarget{
			Tabs:       c.currentTarget.Tabs,
			MainWindow: mainWindow,
		}
	}
	if c.currentTarget.Tabs != nil && c.currentTarget.Tabs.Count() > 0 && oldMainWindow != mainWindow {
		logging.FromContext(context.Background()).Debug().
			Int("tab_count", c.currentTarget.Tabs.Count()).
			Str("old_main_window", fmt.Sprintf("%p", oldMainWindow)).
			Str("new_main_window", fmt.Sprintf("%p", mainWindow)).
			Msg("SetMainWindow: changing mainWindow while tabs exist")
	}
}

// SetOnAttachPopupToTab sets the callback for attaching popup WebViews to tabs.
func (c *TabCoordinator) SetOnAttachPopupToTab(fn func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv port.WebView)) {
	c.onAttachPopupToTab = fn
}

// notifyStateChanged triggers the state changed callback if set.
func (c *TabCoordinator) notifyStateChanged() {
	if c.onStateChanged != nil {
		c.onStateChanged()
	}
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

// Create creates a new tab in the given target with the given initial URL.
func (c *TabCoordinator) Create(ctx context.Context, target TabTarget, initialURL string) (*entity.Tab, error) {
	return c.create(ctx, target, initialURL, true)
}

func (c *TabCoordinator) create(ctx context.Context, target TabTarget, initialURL string, activate bool) (*entity.Tab, error) {
	log := logging.FromContext(ctx)

	if target.Tabs == nil {
		return nil, fmt.Errorf("target.Tabs is nil")
	}

	output, err := c.tabsUC.Create(ctx, usecase.CreateTabInput{
		TabList:    target.Tabs,
		Name:       "",
		InitialURL: initialURL,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create tab")
		return nil, err
	}

	if activate {
		// Set new tab as active (updates domain state and tracks previous)
		target.Tabs.SetActive(output.Tab.ID)
	}

	// Notify app before adding the tab to the visible tab bar.
	// The app assigns window ownership and the window-scoped default name here,
	// and TabBar.AddTab reads tab.Title() immediately when creating the button.
	if c.onTabCreated != nil {
		c.onTabCreated(ctx, target, output.Tab)
	}

	// Update tab bar
	if target.MainWindow != nil && target.MainWindow.TabBar() != nil {
		target.MainWindow.TabBar().AddTab(output.Tab)
		if activate {
			target.MainWindow.TabBar().SetActive(output.Tab.ID)
		}
	}

	// Update tab bar visibility
	c.UpdateBarVisibility(ctx, target)

	// Switch to the new tab's workspace view when activation is requested.
	if activate && c.onTabSwitched != nil {
		c.onTabSwitched(ctx, target, output.Tab)
	}

	// Notify state change for session snapshots
	c.notifyStateChanged()

	log.Debug().Str("tab_id", string(output.Tab.ID)).Msg("tab created")
	return output.Tab, nil
}

// Close closes the active tab in the given target.
// When the target's TabList becomes empty, fires onCurrentWindowEmpty.
// The caller (App) is responsible for deciding whether all windows are empty
// and the process should quit. onQuit is NOT fired by Close.
func (c *TabCoordinator) Close(ctx context.Context, target TabTarget) error {
	log := logging.FromContext(ctx)

	if target.Tabs == nil {
		return fmt.Errorf("target.Tabs is nil")
	}

	activeID := target.Tabs.ActiveTabID
	if activeID == "" {
		log.Debug().Msg("no active tab to close")
		return nil
	}

	tabs := target.Tabs.Tabs

	// Compute the next tab to activate BEFORE closing
	activeIdx := indexOfTab(tabs, activeID)

	var nextID entity.TabID
	if activeIdx >= 0 && len(tabs) > 1 {
		if activeIdx < len(tabs)-1 {
			nextID = tabs[activeIdx+1].ID
		} else {
			nextID = tabs[activeIdx-1].ID
		}
	}

	closedTab := target.Tabs.Find(activeID)
	wasLast, err := c.tabsUC.Close(ctx, target.Tabs, activeID)
	if err != nil {
		log.Error().Err(err).Msg("failed to close tab")
		return err
	}

	if c.onTabClosed != nil && closedTab != nil {
		c.onTabClosed(ctx, target, closedTab)
	}

	if target.MainWindow != nil && target.MainWindow.TabBar() != nil {
		target.MainWindow.TabBar().RemoveTab(activeID)
	}

	// Update tab bar visibility
	c.UpdateBarVisibility(ctx, target)

	// Target TabList became empty — signal window-empty.
	// App decides whether to quit (all windows empty) or only close this window.
	if wasLast {
		c.notifyStateChanged()
		if c.onCurrentWindowEmpty != nil {
			c.onCurrentWindowEmpty(ctx, target)
		}
		log.Debug().Str("tab_id", string(activeID)).Msg("tab closed, target window empty")
		return nil
	}

	// If there's a sibling tab in the same window, switch to it
	if nextID != "" {
		if err := c.tabsUC.Switch(ctx, target.Tabs, nextID); err != nil {
			log.Error().Err(err).Str("tab_id", string(nextID)).Msg("failed to switch to sibling")
			return err
		}

		if target.MainWindow != nil && target.MainWindow.TabBar() != nil {
			target.MainWindow.TabBar().SetActive(nextID)
		}

		if c.onTabSwitched != nil {
			if tab := target.Tabs.Find(nextID); tab != nil {
				c.onTabSwitched(ctx, target, tab)
			}
		}

		// Notify state change for session snapshots
		c.notifyStateChanged()

		log.Debug().Str("tab_id", string(activeID)).Str("next", string(nextID)).Msg("tab closed, switched to sibling")
		return nil
	}

	return c.recoverCloseWithoutSibling(ctx, target, activeID)
}

func (c *TabCoordinator) recoverCloseWithoutSibling(ctx context.Context, target TabTarget, closedID entity.TabID) error {
	log := logging.FromContext(ctx)
	// Defensive corrupted-state guard: only treat the target as empty if it is
	// actually empty after close; otherwise recover by activating a surviving tab.
	c.notifyStateChanged()
	if len(target.Tabs.Tabs) == 0 {
		if c.onCurrentWindowEmpty != nil {
			c.onCurrentWindowEmpty(ctx, target)
		}
		log.Debug().Str("tab_id", string(closedID)).Msg("tab closed, target window empty")
		return nil
	}
	for _, tab := range target.Tabs.Tabs {
		if tab != nil {
			return c.Switch(ctx, target, tab.ID)
		}
	}
	return fmt.Errorf("tab closed but no surviving tab could be activated")
}

// Switch switches to a specific tab by ID in the given target.
func (c *TabCoordinator) Switch(ctx context.Context, target TabTarget, tabID entity.TabID) error {
	log := logging.FromContext(ctx)

	if target.Tabs == nil {
		return fmt.Errorf("target.Tabs is nil")
	}

	// Skip if already active
	if tabID == target.Tabs.ActiveTabID {
		return nil
	}

	// Verify the tab exists in this target
	tab := target.Tabs.Find(tabID)
	if tab == nil {
		log.Debug().Str("tab_id", string(tabID)).Msg("tab not found in target")
		return nil
	}

	log.Debug().
		Str("from", string(target.Tabs.ActiveTabID)).
		Str("to", string(tabID)).
		Msg("switching tab")

	// Update domain state
	if err := c.tabsUC.Switch(ctx, target.Tabs, tabID); err != nil {
		log.Error().Err(err).Str("tab_id", string(tabID)).Msg("failed to switch tab")
		return err
	}

	// Update tab bar UI
	if target.MainWindow != nil && target.MainWindow.TabBar() != nil {
		target.MainWindow.TabBar().SetActive(tabID)
	}

	// Invoke callback for workspace view switching
	if c.onTabSwitched != nil {
		c.onTabSwitched(ctx, target, tab)
	}

	c.notifyStateChanged()

	return nil
}

// switchRelative switches to the tab delta positions away within the target.
// delta of +1 switches next, -1 switches previous, wrapping within the list.
func (c *TabCoordinator) switchRelative(ctx context.Context, target TabTarget, delta int) error {
	if target.Tabs == nil {
		return fmt.Errorf("target.Tabs is nil")
	}
	tabs := target.Tabs.Tabs
	if len(tabs) <= 1 {
		return nil
	}

	current := indexOfTab(tabs, target.Tabs.ActiveTabID)
	if current < 0 {
		current = 0
	} else {
		current = (current + delta + len(tabs)) % len(tabs)
	}

	return c.Switch(ctx, target, tabs[current].ID)
}

// SwitchNext switches to the next tab within the given target.
func (c *TabCoordinator) SwitchNext(ctx context.Context, target TabTarget) error {
	return c.switchRelative(ctx, target, 1)
}

// SwitchPrev switches to the previous tab within the given target.
func (c *TabCoordinator) SwitchPrev(ctx context.Context, target TabTarget) error {
	return c.switchRelative(ctx, target, -1)
}

// SwitchByIndexCreating switches to a tab by 0-based index within the given target,
// creating new tabs as needed to reach the requested index.
// If the requested index is beyond the current tab count and initialURL is empty,
// it returns an error without mutating tabs. If index < 0, it logs debug and returns nil.
func (c *TabCoordinator) SwitchByIndexCreating(ctx context.Context, target TabTarget, index int, initialURL string) error {
	log := logging.FromContext(ctx)

	if target.Tabs == nil {
		return fmt.Errorf("target.Tabs is nil")
	}
	if index < 0 {
		log.Debug().Int("index", index).Msg("invalid tab index, ignoring")
		return nil
	}
	if target.Tabs.Count() <= index && initialURL == "" {
		return fmt.Errorf("initial URL is required to create missing tabs")
	}
	// Intermediate tabs are created inactive (create(..., false)) and the final
	// missing tab is created active (create(..., true)) before SwitchByIndex.
	for target.Tabs.Count() < index {
		if _, err := c.create(ctx, target, initialURL, false); err != nil {
			return err
		}
	}
	if target.Tabs.Count() <= index {
		if _, err := c.create(ctx, target, initialURL, true); err != nil {
			return err
		}
	}
	return c.SwitchByIndex(ctx, target, index)
}

// SwitchByIndex switches to a tab by 0-based index within the given target.
func (c *TabCoordinator) SwitchByIndex(ctx context.Context, target TabTarget, index int) error {
	log := logging.FromContext(ctx)

	if target.Tabs == nil {
		return fmt.Errorf("target.Tabs is nil")
	}
	tabs := target.Tabs.Tabs
	if index < 0 || index >= len(tabs) {
		log.Debug().Int("index", index).Int("count", len(tabs)).Msg("invalid tab index")
		return nil
	}

	return c.Switch(ctx, target, tabs[index].ID)
}

// SwitchToLastActive switches to the previously active tab (Alt+Tab style)
// within the given target.
func (c *TabCoordinator) SwitchToLastActive(ctx context.Context, target TabTarget) error {
	if target.Tabs == nil {
		return nil
	}

	prevID := target.Tabs.PreviousActiveTabID
	if prevID == "" || prevID == target.Tabs.ActiveTabID {
		return nil
	}

	tab := target.Tabs.Find(prevID)
	if tab == nil {
		target.Tabs.PreviousActiveTabID = ""
		return nil
	}

	return c.Switch(ctx, target, prevID)
}

// UpdateBarVisibility shows or hides the tab bar based on tab count in the target.
func (c *TabCoordinator) UpdateBarVisibility(ctx context.Context, target TabTarget) {
	log := logging.FromContext(ctx)

	if target.MainWindow == nil || target.MainWindow.TabBar() == nil {
		log.Debug().Msg("mainWindow or tabBar is nil, skipping visibility update")
		return
	}

	// Check if feature is enabled
	if !c.hideTabBarWhenSingleTab {
		log.Debug().Msg("tab bar auto-hide disabled by config")
		target.MainWindow.TabBar().SetAutoHidden(false)
		target.MainWindow.SetTabBarContentInsetVisible(true)
		return
	}

	tabCount := target.MainWindow.TabBar().Count()
	shouldShow := tabCount > 1

	log.Debug().Int("tab_count", tabCount).Bool("should_show", shouldShow).Msg("setting tab bar visibility")
	target.MainWindow.TabBar().SetAutoHidden(!shouldShow)
	target.MainWindow.SetTabBarContentInsetVisible(shouldShow)
}

// GetTabBar returns the tab bar component from the current main window.
func (c *TabCoordinator) GetTabBar() *component.TabBar {
	if c.mainWindow != nil {
		return c.mainWindow.TabBar()
	}
	return nil
}

// CreateWithPane creates a new tab with a pre-created pane and WebView in the given target.
// This is used for tabbed popup behavior where the popup pane already exists.
func (c *TabCoordinator) CreateWithPane(
	ctx context.Context,
	target TabTarget,
	pane *entity.Pane,
	wv port.WebView,
	initialURL string,
) (*entity.Tab, error) {
	log := logging.FromContext(ctx)

	if target.Tabs == nil {
		return nil, fmt.Errorf("target.Tabs is nil")
	}

	output, err := c.tabsUC.CreateWithPane(ctx, usecase.CreateTabWithPaneInput{
		TabList:    target.Tabs,
		Name:       pane.Title,
		Pane:       pane,
		InitialURL: initialURL,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create tab with pane")
		return nil, err
	}

	// Set new tab as active
	target.Tabs.SetActive(output.Tab.ID)

	// Notify app before adding tab to the visible tab bar.
	if c.onTabCreated != nil {
		c.onTabCreated(ctx, target, output.Tab)
	}

	// Update tab bar
	if target.MainWindow != nil && target.MainWindow.TabBar() != nil {
		target.MainWindow.TabBar().AddTab(output.Tab)
		target.MainWindow.TabBar().SetActive(output.Tab.ID)
	}

	// Update tab bar visibility
	c.UpdateBarVisibility(ctx, target)

	// Attach the popup WebView to the new tab's workspace
	if c.onAttachPopupToTab != nil {
		c.onAttachPopupToTab(ctx, output.Tab.ID, pane, wv)
	}

	// Switch to the new tab's workspace view
	if c.onTabSwitched != nil {
		c.onTabSwitched(ctx, target, output.Tab)
	}

	// Notify state change for session snapshots
	c.notifyStateChanged()

	log.Debug().
		Str("tab_id", string(output.Tab.ID)).
		Str("pane_id", string(pane.ID)).
		Msg("tab created with popup pane")
	return output.Tab, nil
}

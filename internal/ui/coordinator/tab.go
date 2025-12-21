package coordinator

import (
	"context"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/window"
)

// TabCoordinator manages tab lifecycle operations.
type TabCoordinator struct {
	tabsUC     *usecase.ManageTabsUseCase
	tabs       *entity.TabList
	mainWindow *window.MainWindow
	config     *config.Config

	// Callbacks to avoid circular dependencies
	onTabCreated       func(ctx context.Context, tab *entity.Tab)
	onTabSwitched      func(ctx context.Context, tab *entity.Tab)
	onQuit             func()
	onAttachPopupToTab func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv *webkit.WebView) // For popup tabs
}

// TabCoordinatorConfig holds configuration for TabCoordinator.
type TabCoordinatorConfig struct {
	TabsUC     *usecase.ManageTabsUseCase
	Tabs       *entity.TabList
	MainWindow *window.MainWindow
	Config     *config.Config
}

// NewTabCoordinator creates a new TabCoordinator.
func NewTabCoordinator(ctx context.Context, cfg TabCoordinatorConfig) *TabCoordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating tab coordinator")

	return &TabCoordinator{
		tabsUC:     cfg.TabsUC,
		tabs:       cfg.Tabs,
		mainWindow: cfg.MainWindow,
		config:     cfg.Config,
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

// Create creates a new tab with the given initial URL.
func (c *TabCoordinator) Create(ctx context.Context, initialURL string) (*entity.Tab, error) {
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

	// Set new tab as active (updates domain state and tracks previous)
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

	// Switch to the new tab's workspace view
	if c.onTabSwitched != nil {
		c.onTabSwitched(ctx, output.Tab)
	}

	log.Debug().Str("tab_id", string(output.Tab.ID)).Msg("tab created")
	return output.Tab, nil
}

// Close closes the active tab.
func (c *TabCoordinator) Close(ctx context.Context) error {
	log := logging.FromContext(ctx)

	activeID := c.tabs.ActiveTabID
	if activeID == "" {
		log.Debug().Msg("no active tab to close")
		return nil
	}

	wasLast, err := c.tabsUC.Close(ctx, c.tabs, activeID)
	if err != nil {
		log.Error().Err(err).Msg("failed to close tab")
		return err
	}

	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().RemoveTab(activeID)

		// Switch to new active tab if any
		if c.tabs.ActiveTabID != "" {
			c.mainWindow.TabBar().SetActive(c.tabs.ActiveTabID)
		}
	}

	// Update tab bar visibility
	c.UpdateBarVisibility(ctx)

	// Quit if no tabs left
	if wasLast && c.onQuit != nil {
		c.onQuit()
	}

	// Switch workspace view to new active tab (if not last)
	if !wasLast && c.onTabSwitched != nil {
		if tab := c.tabs.Find(c.tabs.ActiveTabID); tab != nil {
			c.onTabSwitched(ctx, tab)
		}
	}

	log.Debug().Str("tab_id", string(activeID)).Bool("was_last", wasLast).Msg("tab closed")
	return nil
}

// Switch switches to a specific tab by ID.
func (c *TabCoordinator) Switch(ctx context.Context, tabID entity.TabID) error {
	log := logging.FromContext(ctx)

	// Skip if already active
	if tabID == c.tabs.ActiveTabID {
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

// SwitchNext switches to the next tab.
func (c *TabCoordinator) SwitchNext(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if err := c.tabsUC.SwitchNext(ctx, c.tabs); err != nil {
		log.Error().Err(err).Msg("failed to switch to next tab")
		return err
	}

	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().SetActive(c.tabs.ActiveTabID)
	}

	// Invoke callback for workspace view switching
	if c.onTabSwitched != nil {
		if tab := c.tabs.Find(c.tabs.ActiveTabID); tab != nil {
			c.onTabSwitched(ctx, tab)
		}
	}

	log.Debug().Str("tab_id", string(c.tabs.ActiveTabID)).Msg("switched to next tab")
	return nil
}

// SwitchPrev switches to the previous tab.
func (c *TabCoordinator) SwitchPrev(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if err := c.tabsUC.SwitchPrevious(ctx, c.tabs); err != nil {
		log.Error().Err(err).Msg("failed to switch to previous tab")
		return err
	}

	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().SetActive(c.tabs.ActiveTabID)
	}

	// Invoke callback for workspace view switching
	if c.onTabSwitched != nil {
		if tab := c.tabs.Find(c.tabs.ActiveTabID); tab != nil {
			c.onTabSwitched(ctx, tab)
		}
	}

	log.Debug().Str("tab_id", string(c.tabs.ActiveTabID)).Msg("switched to previous tab")
	return nil
}

// SwitchByIndex switches to a tab by 0-based index.
func (c *TabCoordinator) SwitchByIndex(ctx context.Context, index int) error {
	log := logging.FromContext(ctx)

	if err := c.tabsUC.SwitchByIndex(ctx, c.tabs, index); err != nil {
		log.Error().Err(err).Int("index", index).Msg("failed to switch to tab by index")
		return err
	}

	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().SetActive(c.tabs.ActiveTabID)
	}

	// Invoke callback for workspace view switching
	if c.onTabSwitched != nil {
		if tab := c.tabs.Find(c.tabs.ActiveTabID); tab != nil {
			c.onTabSwitched(ctx, tab)
		}
	}

	log.Debug().Int("index", index).Str("tab_id", string(c.tabs.ActiveTabID)).Msg("switched to tab by index")
	return nil
}

// SwitchToLastActive switches to the previously active tab (Alt+Tab style).
func (c *TabCoordinator) SwitchToLastActive(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if err := c.tabsUC.SwitchToLastActive(ctx, c.tabs); err != nil {
		log.Error().Err(err).Msg("failed to switch to last active tab")
		return err
	}

	if c.mainWindow != nil && c.mainWindow.TabBar() != nil {
		c.mainWindow.TabBar().SetActive(c.tabs.ActiveTabID)
	}

	// Invoke callback for workspace view switching
	if c.onTabSwitched != nil {
		if tab := c.tabs.Find(c.tabs.ActiveTabID); tab != nil {
			c.onTabSwitched(ctx, tab)
		}
	}

	log.Debug().Str("tab_id", string(c.tabs.ActiveTabID)).Msg("switched to last active tab")
	return nil
}

// UpdateBarVisibility shows or hides the tab bar based on tab count.
func (c *TabCoordinator) UpdateBarVisibility(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Check if feature is enabled
	hideEnabled := true
	if c.config != nil {
		hideEnabled = c.config.Workspace.HideTabBarWhenSingleTab
	}

	if !hideEnabled {
		log.Debug().Msg("tab bar auto-hide disabled by config")
		return
	}

	if c.mainWindow == nil || c.mainWindow.TabBar() == nil {
		log.Debug().Msg("mainWindow or tabBar is nil, skipping visibility update")
		return
	}

	tabCount := c.tabs.Count()
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
func (c *TabCoordinator) SetOnAttachPopupToTab(fn func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv *webkit.WebView)) {
	c.onAttachPopupToTab = fn
}

// CreateWithPane creates a new tab with a pre-created pane and WebView.
// This is used for tabbed popup behavior where the popup pane already exists.
func (c *TabCoordinator) CreateWithPane(
	ctx context.Context,
	pane *entity.Pane,
	wv *webkit.WebView,
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

	log.Debug().
		Str("tab_id", string(output.Tab.ID)).
		Str("pane_id", string(pane.ID)).
		Msg("tab created with popup pane")
	return output.Tab, nil
}

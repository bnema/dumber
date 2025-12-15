package coordinator

import (
	"context"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
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
	onTabCreated func(ctx context.Context, tab *entity.Tab)
	onQuit       func()
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

	log.Debug().Str("tab_id", string(activeID)).Bool("was_last", wasLast).Msg("tab closed")
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

	// Switch workspace view
	if c.onTabCreated != nil {
		// TODO: Add onTabSwitched callback for workspace view switching
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

	log.Debug().Str("tab_id", string(c.tabs.ActiveTabID)).Msg("switched to previous tab")
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

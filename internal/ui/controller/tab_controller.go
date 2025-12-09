// Package controller provides controllers that bridge domain state and UI widgets.
package controller

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/rs/zerolog"
)

// TabController synchronizes TabList domain state with the TabBar UI widget.
type TabController struct {
	tabs   *entity.TabList
	tabBar *component.TabBar
	tabsUC *usecase.ManageTabsUseCase

	// Callback when tab switch requires content swap
	onTabSwitch func(tabID entity.TabID, tab *entity.Tab)

	ctx    context.Context
	logger *zerolog.Logger
	mu     sync.RWMutex
}

// NewTabController creates a new controller linking TabList to TabBar.
func NewTabController(
	ctx context.Context,
	tabs *entity.TabList,
	tabBar *component.TabBar,
	tabsUC *usecase.ManageTabsUseCase,
) *TabController {
	logger := logging.FromContext(ctx)

	tc := &TabController{
		tabs:   tabs,
		tabBar: tabBar,
		tabsUC: tabsUC,
		ctx:    ctx,
		logger: logger,
	}

	// Wire tab bar callbacks to controller methods
	tabBar.SetOnSwitch(tc.handleTabSwitch)
	tabBar.SetOnClose(tc.handleTabClose)
	tabBar.SetOnCreate(tc.handleTabCreate)

	return tc
}

// SetOnTabSwitch sets the callback invoked when a tab switch occurs.
// The callback receives the tab ID and the Tab entity for content swapping.
func (tc *TabController) SetOnTabSwitch(fn func(tabID entity.TabID, tab *entity.Tab)) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.onTabSwitch = fn
}

// handleTabSwitch handles tab button click events from the tab bar.
func (tc *TabController) handleTabSwitch(tabID entity.TabID) {
	tc.mu.RLock()
	currentActive := tc.tabs.ActiveTabID
	tc.mu.RUnlock()

	if tabID == currentActive {
		return // Already active
	}

	tc.logger.Debug().
		Str("from", string(currentActive)).
		Str("to", string(tabID)).
		Msg("switching tab")

	// Use the use case to update domain state
	if err := tc.tabsUC.Switch(tc.ctx, tc.tabs, tabID); err != nil {
		tc.logger.Error().Err(err).Str("tab_id", string(tabID)).Msg("failed to switch tab")
		return
	}

	// Update UI
	tc.tabBar.SetActive(tabID)

	// Notify for content swap
	tc.mu.RLock()
	callback := tc.onTabSwitch
	tab := tc.tabs.Find(tabID)
	tc.mu.RUnlock()

	if callback != nil && tab != nil {
		callback(tabID, tab)
	}
}

// handleTabClose handles tab close button click events.
func (tc *TabController) handleTabClose(tabID entity.TabID) {
	tc.logger.Debug().Str("tab_id", string(tabID)).Msg("closing tab")

	wasLast, err := tc.tabsUC.Close(tc.ctx, tc.tabs, tabID)
	if err != nil {
		tc.logger.Error().Err(err).Str("tab_id", string(tabID)).Msg("failed to close tab")
		return
	}

	// Remove from UI
	tc.tabBar.RemoveTab(tabID)

	if wasLast {
		tc.logger.Info().Msg("last tab closed, application should exit")
		// The application should handle this by monitoring tab count
		return
	}

	// Switch to new active tab
	tc.mu.RLock()
	newActiveID := tc.tabs.ActiveTabID
	newActiveTab := tc.tabs.Find(newActiveID)
	callback := tc.onTabSwitch
	tc.mu.RUnlock()

	tc.tabBar.SetActive(newActiveID)

	if callback != nil && newActiveTab != nil {
		callback(newActiveID, newActiveTab)
	}
}

// handleTabCreate handles new tab creation requests.
func (tc *TabController) handleTabCreate() {
	tc.CreateTab("", "about:blank")
}

// CreateTab creates a new tab with the given name and URL.
func (tc *TabController) CreateTab(name, url string) *entity.Tab {
	tc.logger.Debug().
		Str("name", name).
		Str("url", url).
		Msg("creating new tab")

	output, err := tc.tabsUC.Create(tc.ctx, usecase.CreateTabInput{
		TabList:    tc.tabs,
		Name:       name,
		InitialURL: url,
	})
	if err != nil {
		tc.logger.Error().Err(err).Msg("failed to create tab")
		return nil
	}

	// Add to UI
	tc.tabBar.AddTab(output.Tab)
	tc.tabBar.SetActive(output.Tab.ID)

	// Notify for content setup
	tc.mu.RLock()
	callback := tc.onTabSwitch
	tc.mu.RUnlock()

	if callback != nil {
		callback(output.Tab.ID, output.Tab)
	}

	return output.Tab
}

// SwitchToTab switches to the specified tab by ID.
func (tc *TabController) SwitchToTab(tabID entity.TabID) {
	tc.handleTabSwitch(tabID)
}

// SwitchToNextTab switches to the next tab.
func (tc *TabController) SwitchToNextTab() {
	if err := tc.tabsUC.SwitchNext(tc.ctx, tc.tabs); err != nil {
		tc.logger.Error().Err(err).Msg("failed to switch to next tab")
		return
	}

	tc.mu.RLock()
	activeID := tc.tabs.ActiveTabID
	activeTab := tc.tabs.Find(activeID)
	callback := tc.onTabSwitch
	tc.mu.RUnlock()

	tc.tabBar.SetActive(activeID)

	if callback != nil && activeTab != nil {
		callback(activeID, activeTab)
	}
}

// SwitchToPreviousTab switches to the previous tab.
func (tc *TabController) SwitchToPreviousTab() {
	if err := tc.tabsUC.SwitchPrevious(tc.ctx, tc.tabs); err != nil {
		tc.logger.Error().Err(err).Msg("failed to switch to previous tab")
		return
	}

	tc.mu.RLock()
	activeID := tc.tabs.ActiveTabID
	activeTab := tc.tabs.Find(activeID)
	callback := tc.onTabSwitch
	tc.mu.RUnlock()

	tc.tabBar.SetActive(activeID)

	if callback != nil && activeTab != nil {
		callback(activeID, activeTab)
	}
}

// SwitchToTabByIndex switches to tab at the given 0-based index.
func (tc *TabController) SwitchToTabByIndex(index int) {
	if err := tc.tabsUC.SwitchByIndex(tc.ctx, tc.tabs, index); err != nil {
		tc.logger.Error().Err(err).Int("index", index).Msg("failed to switch to tab by index")
		return
	}

	tc.mu.RLock()
	activeID := tc.tabs.ActiveTabID
	activeTab := tc.tabs.Find(activeID)
	callback := tc.onTabSwitch
	tc.mu.RUnlock()

	tc.tabBar.SetActive(activeID)

	if callback != nil && activeTab != nil {
		callback(activeID, activeTab)
	}
}

// UpdateTabTitle updates the title of a tab.
func (tc *TabController) UpdateTabTitle(tabID entity.TabID, title string) {
	tc.tabBar.UpdateTitle(tabID, title)
}

// TabCount returns the current number of tabs.
func (tc *TabController) TabCount() int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.tabs.Count()
}

// ActiveTab returns the currently active tab.
func (tc *TabController) ActiveTab() *entity.Tab {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.tabs.ActiveTab()
}

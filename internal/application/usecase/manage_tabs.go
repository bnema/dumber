package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
)

// IDGenerator is a function type for generating unique IDs.
type IDGenerator func() string

// ManageTabsUseCase handles tab lifecycle operations.
type ManageTabsUseCase struct {
	idGenerator IDGenerator
}

// NewManageTabsUseCase creates a new tab management use case.
func NewManageTabsUseCase(idGenerator IDGenerator) *ManageTabsUseCase {
	return &ManageTabsUseCase{
		idGenerator: idGenerator,
	}
}

// CreateTabInput contains parameters for creating a new tab.
type CreateTabInput struct {
	TabList    *entity.TabList
	Name       string // Optional custom name
	InitialURL string // URL to load (default: about:blank)
	IsPinned   bool
}

// CreateTabOutput contains the result of tab creation.
type CreateTabOutput struct {
	Tab *entity.Tab
}

// Create creates a new tab with a workspace and initial pane.
func (uc *ManageTabsUseCase) Create(ctx context.Context, input CreateTabInput) (*CreateTabOutput, error) {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("name", input.Name).
		Str("initial_url", input.InitialURL).
		Bool("is_pinned", input.IsPinned).
		Msg("creating new tab")

	if input.TabList == nil {
		return nil, fmt.Errorf("tab list is required")
	}

	// Generate IDs
	tabID := entity.TabID(uc.idGenerator())
	workspaceID := entity.WorkspaceID(uc.idGenerator())
	paneID := entity.PaneID(uc.idGenerator())

	// Create initial pane with normalized URL
	pane := entity.NewPane(paneID)
	if input.InitialURL != "" {
		pane.URI = url.Normalize(input.InitialURL)
	}

	// Create tab with workspace
	tab := entity.NewTab(tabID, workspaceID, pane)
	tab.Name = input.Name
	tab.IsPinned = input.IsPinned

	// Add to list (handles position and active tab)
	input.TabList.Add(tab)

	log.Info().
		Str("tab_id", string(tabID)).
		Str("workspace_id", string(workspaceID)).
		Str("pane_id", string(paneID)).
		Int("position", tab.Position).
		Msg("tab created")

	return &CreateTabOutput{Tab: tab}, nil
}

// Close removes a tab from the list.
// Returns true if this was the last tab (caller should handle app exit).
func (uc *ManageTabsUseCase) Close(ctx context.Context, tabs *entity.TabList, tabID entity.TabID) (wasLast bool, err error) {
	log := logging.FromContext(ctx)
	ctx = logging.WithTabID(ctx, string(tabID))
	log = logging.FromContext(ctx)

	log.Debug().Msg("closing tab")

	if tabs == nil {
		return false, fmt.Errorf("tab list is required")
	}

	tab := tabs.Find(tabID)
	if tab == nil {
		log.Debug().Msg("tab not found")
		return false, nil
	}

	// Check if this is the last tab
	if tabs.Count() == 1 {
		log.Info().Msg("closing last tab")
		tabs.Remove(tabID)
		return true, nil
	}

	// Remove tab (TabList handles active tab switching)
	if !tabs.Remove(tabID) {
		return false, fmt.Errorf("failed to remove tab")
	}

	log.Info().
		Str("new_active", string(tabs.ActiveTabID)).
		Int("remaining", tabs.Count()).
		Msg("tab closed")

	return false, nil
}

// Switch changes the active tab.
func (uc *ManageTabsUseCase) Switch(ctx context.Context, tabs *entity.TabList, tabID entity.TabID) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("tab_id", string(tabID)).Msg("switching to tab")

	if tabs == nil {
		return fmt.Errorf("tab list is required")
	}

	tab := tabs.Find(tabID)
	if tab == nil {
		return fmt.Errorf("tab not found: %s", tabID)
	}

	oldActive := tabs.ActiveTabID
	tabs.ActiveTabID = tabID

	log.Info().
		Str("from", string(oldActive)).
		Str("to", string(tabID)).
		Msg("tab switched")

	return nil
}

// Move repositions a tab within the tab bar.
func (uc *ManageTabsUseCase) Move(ctx context.Context, tabs *entity.TabList, tabID entity.TabID, newPosition int) error {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("tab_id", string(tabID)).
		Int("new_position", newPosition).
		Msg("moving tab")

	if tabs == nil {
		return fmt.Errorf("tab list is required")
	}

	if !tabs.Move(tabID, newPosition) {
		return fmt.Errorf("failed to move tab to position %d", newPosition)
	}

	log.Info().
		Str("tab_id", string(tabID)).
		Int("position", newPosition).
		Msg("tab moved")

	return nil
}

// Rename changes a tab's custom name.
func (uc *ManageTabsUseCase) Rename(ctx context.Context, tabs *entity.TabList, tabID entity.TabID, name string) error {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("tab_id", string(tabID)).
		Str("name", name).
		Msg("renaming tab")

	if tabs == nil {
		return fmt.Errorf("tab list is required")
	}

	tab := tabs.Find(tabID)
	if tab == nil {
		return fmt.Errorf("tab not found: %s", tabID)
	}

	tab.Name = name

	log.Info().
		Str("tab_id", string(tabID)).
		Str("name", name).
		Msg("tab renamed")

	return nil
}

// GetNext returns the next tab ID in the given direction.
// direction: 1 for next, -1 for previous.
func (uc *ManageTabsUseCase) GetNext(tabs *entity.TabList, direction int) entity.TabID {
	if tabs == nil || tabs.Count() == 0 {
		return ""
	}

	// Find current active tab position
	activeTab := tabs.ActiveTab()
	if activeTab == nil {
		// Return first tab if no active
		if len(tabs.Tabs) > 0 {
			return tabs.Tabs[0].ID
		}
		return ""
	}

	currentPos := activeTab.Position
	newPos := currentPos + direction

	// Wrap around
	if newPos < 0 {
		newPos = tabs.Count() - 1
	} else if newPos >= tabs.Count() {
		newPos = 0
	}

	if newPos >= 0 && newPos < len(tabs.Tabs) {
		return tabs.Tabs[newPos].ID
	}

	return tabs.ActiveTabID
}

// SwitchNext switches to the next tab (wraps around).
func (uc *ManageTabsUseCase) SwitchNext(ctx context.Context, tabs *entity.TabList) error {
	nextID := uc.GetNext(tabs, 1)
	if nextID == "" || nextID == tabs.ActiveTabID {
		return nil
	}
	return uc.Switch(ctx, tabs, nextID)
}

// SwitchPrevious switches to the previous tab (wraps around).
func (uc *ManageTabsUseCase) SwitchPrevious(ctx context.Context, tabs *entity.TabList) error {
	prevID := uc.GetNext(tabs, -1)
	if prevID == "" || prevID == tabs.ActiveTabID {
		return nil
	}
	return uc.Switch(ctx, tabs, prevID)
}

// SwitchByIndex switches to tab at the given index (0-based).
func (uc *ManageTabsUseCase) SwitchByIndex(ctx context.Context, tabs *entity.TabList, index int) error {
	log := logging.FromContext(ctx)

	if tabs == nil || index < 0 || index >= tabs.Count() {
		log.Debug().Int("index", index).Msg("invalid tab index")
		return nil
	}

	return uc.Switch(ctx, tabs, tabs.Tabs[index].ID)
}

// Pin toggles the pinned state of a tab.
func (uc *ManageTabsUseCase) Pin(ctx context.Context, tabs *entity.TabList, tabID entity.TabID, pinned bool) error {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("tab_id", string(tabID)).
		Bool("pinned", pinned).
		Msg("toggling tab pin state")

	if tabs == nil {
		return fmt.Errorf("tab list is required")
	}

	tab := tabs.Find(tabID)
	if tab == nil {
		return fmt.Errorf("tab not found: %s", tabID)
	}

	tab.IsPinned = pinned

	log.Info().
		Str("tab_id", string(tabID)).
		Bool("pinned", pinned).
		Msg("tab pin state changed")

	return nil
}

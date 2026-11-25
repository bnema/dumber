package systemviews

import (
	"context"
	"strconv"
)

// paletteActionOpen asks the App for the commands of the current view.
const paletteActionOpen = "palette.open"

const (
	paletteGroupGoTo     = "Go to"
	paletteGroupHistory  = "History"
	paletteGroupSettings = "Settings"
	paletteGroupSection  = "Jump to"
	paletteGroupFavorite = "Favorites"
	paletteGroupTags     = "Show tag"
	paletteGroupHome     = "Home"
)

// DOM-handled favorites commands.
const (
	// uiActionFocus focuses the element matching Data["selector"].
	uiActionFocus = "ui.focus"
	// uiActionOpenPopover opens the popover with id Data["id"].
	uiActionOpenPopover = "ui.openPopover"
)

func (a *App) handlePaletteOpen(_ context.Context) error {
	palette, ok := a.deps.DOM.(DOMPalette)
	if !ok {
		return nil
	}
	a.lockState()
	items := a.paletteItemsLocked()
	a.unlockState()
	palette.ShowPalette(PaletteRequest{Placeholder: "Type a command…", Items: items})
	return nil
}

// paletteItemsLocked lists the commands available on the current view.
func (a *App) paletteItemsLocked() []PaletteItem {
	route := a.currentRoute
	items := make([]PaletteItem, 0, 12)
	switch route {
	case RouteHistory:
		items = append(items, a.historyPaletteItemsLocked()...)
	case RouteConfig:
		items = append(items, configPaletteItems()...)
	case RouteHome:
		items = append(items, homePaletteItems()...)
	case RouteFavorites:
		if a.deps.Favorites != nil {
			items = append(items, favoritesPaletteItems(a.favoritesData)...)
		}
	}
	for _, target := range []struct {
		route    Route
		label    string
		keywords string
	}{
		{RouteHome, "Home", "start dashboard"},
		{RouteHistory, "History", "visits timeline"},
		{RouteFavorites, "Favorites", "bookmarks tags"},
		{RouteConfig, "Settings", "config preferences appearance keybindings"},
	} {
		if target.route == route {
			continue
		}
		items = append(items, PaletteItem{
			ID:       "goto." + string(target.route),
			Label:    target.label,
			Group:    paletteGroupGoTo,
			Hint:     "dumb://" + string(target.route),
			Keywords: target.keywords,
			Action:   DOMAction{Action: uiActionNavigate, Data: map[string]string{"route": string(target.route)}},
		})
	}
	return items
}

func (a *App) historyPaletteItemsLocked() []PaletteItem {
	items := []PaletteItem{{
		ID:     "history.search",
		Label:  "Search history",
		Group:  paletteGroupHistory,
		Hint:   "/",
		Action: DOMAction{Action: uiActionFocusSearch},
	}}
	if a.historyQuery != "" || a.historyDomainFilter != "" {
		items = append(items, PaletteItem{
			ID:     "history.clear",
			Label:  "Clear filters",
			Group:  paletteGroupHistory,
			Action: DOMAction{Action: historyActionClear},
		})
	}
	for _, cleanup := range historyCleanupItems() {
		items = append(items, PaletteItem{
			ID:       "history.deleteRange." + cleanup.RangeID,
			Label:    "Delete history: " + historyCleanupPaletteLabel(cleanup),
			Group:    paletteGroupHistory,
			Keywords: "clear remove forget " + cleanup.RangeID,
			Danger:   true,
			Confirm:  cleanup.Confirm,
			Action:   DOMAction{Action: historyActionDeleteRange, Data: map[string]string{"range": cleanup.RangeID}},
		})
	}
	return items
}

// historyCleanupPaletteLabel turns the compact footer labels into phrases
// that read well after "Delete history:".
func historyCleanupPaletteLabel(item historyCleanupItem) string {
	switch item.RangeID {
	case "week":
		return "last week"
	case "month":
		return "last month"
	case "all":
		return "everything"
	case "hour":
		return "last hour"
	case "day":
		return "today"
	default:
		return item.Label
	}
}

// configPaletteItems lists the settings commands: save, discard, the resets
// and jumps to each section.
func configPaletteItems() []PaletteItem {
	items := []PaletteItem{
		{
			ID: "config.save", Label: "Save changes", Group: paletteGroupSettings, Hint: "Ctrl S",
			Keywords: "apply write", Action: DOMAction{Action: uiActionConfigSave},
		},
		{
			ID: "config.discard", Label: "Discard changes", Group: paletteGroupSettings,
			Keywords: "revert undo cancel", Action: DOMAction{Action: uiActionConfigDiscard},
		},
		{
			ID: "config.resetAppearance", Label: "Reset appearance to defaults", Group: paletteGroupSettings,
			Keywords: "theme colors fonts", Danger: true,
			Confirm: "Reset appearance settings to defaults? Unsaved changes on this page are lost.",
			Action:  DOMAction{Action: configActionResetAppearance},
		},
		{
			ID: "config.resetKeybindings", Label: "Reset all keybindings", Group: paletteGroupSettings,
			Keywords: "shortcuts keys defaults", Danger: true,
			Confirm: "Reset all keybindings to defaults? Unsaved changes on this page are lost.",
			Action:  DOMAction{Action: configActionResetAllKeybindings},
		},
	}
	for _, section := range configSections {
		items = append(items, PaletteItem{
			ID:     "config.section." + section.ID,
			Label:  section.Label,
			Group:  paletteGroupSection,
			Action: DOMAction{Action: uiActionConfigSection, Data: map[string]string{"section": section.ID}},
		})
	}
	return items
}

// favoritesPaletteItems lists the favorites commands and one filter per tag.
func favoritesPaletteItems(data favoritesRenderData) []PaletteItem {
	items := []PaletteItem{
		{
			ID: "favorites.add", Label: "Add favorite", Group: paletteGroupFavorite, Keywords: "new bookmark url",
			Action: DOMAction{Action: uiActionFocus, Data: map[string]string{"selector": "[data-sv-favorite-add-url]"}},
		},
		{
			ID: "favorites.filter", Label: "Filter favorites", Group: paletteGroupFavorite, Hint: "/", Keywords: "search find",
			Action: DOMAction{Action: uiActionFocus, Data: map[string]string{"selector": "[data-sv-favorite-filter]"}},
		},
		{
			ID: "favorites.newTag", Label: "New tag", Group: paletteGroupFavorite, Keywords: "create label",
			Action: DOMAction{Action: uiActionOpenPopover, Data: map[string]string{"id": "sv-tag-new"}},
		},
		{
			ID: "favorites.all", Label: "All favorites", Group: paletteGroupTags, Hint: countLabel(data.TotalCount),
			Action: DOMAction{Action: favoriteActionClearFilters},
		},
		{
			ID: "favorites.untagged", Label: "Untagged", Group: paletteGroupTags, Hint: countLabel(data.UntaggedCount),
			Action: DOMAction{Action: favoriteActionFilterUntagged},
		},
	}
	for _, tag := range data.Tags {
		id := strconv.FormatInt(int64(tag.ID), 10)
		items = append(items, PaletteItem{
			ID: "favorites.tag." + id, Label: tag.Name, Group: paletteGroupTags, Hint: countLabel(data.TagCounts[tag.ID]),
			Keywords: "tag filter",
			Action:   DOMAction{Action: favoriteActionFilterTag, Data: map[string]string{"tagId": id}},
		})
	}
	return items
}

func homePaletteItems() []PaletteItem {
	return []PaletteItem{
		{
			ID:       "home.search",
			Label:    "Search",
			Group:    paletteGroupHome,
			Keywords: "find address url",
			Action:   DOMAction{Action: uiActionFocus, Data: map[string]string{"selector": "[data-sv-home-search]"}},
		},
		{
			ID:       "home.openConfig",
			Label:    "Open home.toml",
			Group:    paletteGroupHome,
			Keywords: "edit layout config",
			Action:   DOMAction{Action: homeActionOpenConfig},
		},
	}
}

package usecase

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
)

// BuildContextMenuUseCase normalizes context menu items from menu state.
type BuildContextMenuUseCase struct{}

var _ port.ContextMenuBuilder = (*BuildContextMenuUseCase)(nil)

// NewBuildContextMenuUseCase creates a new BuildContextMenuUseCase.
func NewBuildContextMenuUseCase() *BuildContextMenuUseCase {
	return &BuildContextMenuUseCase{}
}

// Build returns normalized menu items for the supplied context.
func (*BuildContextMenuUseCase) Build(_ context.Context, menuContext port.MenuContext) []port.MenuItem {
	if menuContext.IsEditable {
		return nil
	}

	items := make([]port.MenuItem, 0, 9)

	if menuContext.CanGoBack {
		items = append(items, port.MenuItem{Action: port.MenuActionBack, Label: "Back"})
	}
	if menuContext.CanGoForward {
		items = append(items, port.MenuItem{Action: port.MenuActionForward, Label: "Forward"})
	}

	items = append(items, port.MenuItem{Action: port.MenuActionReload, Label: "Reload"})

	if menuContext.LinkURI != "" {
		items = append(items,
			port.MenuItem{Action: port.MenuActionOpenLinkNewTab, Label: "Open Link in New Tab"},
			port.MenuItem{Action: port.MenuActionCopyLink, Label: "Copy Link"},
		)
	}

	if menuContext.ImageURI != "" {
		items = append(items,
			port.MenuItem{Action: port.MenuActionCopyImage, Label: "Copy Image"},
			port.MenuItem{Action: port.MenuActionSaveImage, Label: "Save Image"},
		)
	}

	if menuContext.HasSelection {
		items = append(items, port.MenuItem{Action: port.MenuActionCopySelection, Label: "Copy Selection"})
	}

	items = append(items, port.MenuItem{Action: port.MenuActionInspectElement, Label: "Inspect Element"})

	return items
}

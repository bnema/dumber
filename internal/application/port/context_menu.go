package port

import "context"

// MenuAction identifies a normalized context menu action.
type MenuAction string

const (
	MenuActionBack           MenuAction = "back"
	MenuActionForward        MenuAction = "forward"
	MenuActionReload         MenuAction = "reload"
	MenuActionOpenLinkNewTab MenuAction = "open_link_new_tab"
	MenuActionCopyLink       MenuAction = "copy_link"
	MenuActionCopyImage      MenuAction = "copy_image"
	MenuActionSaveImage      MenuAction = "save_image"
	MenuActionInspectElement MenuAction = "inspect_element"
	MenuActionCopySelection  MenuAction = "copy_selection"
)

// MenuContext captures the state needed to build and execute a context menu.
type MenuContext struct {
	PageURI       string
	LinkURI       string
	ImageURI      string
	SelectionText string
	HasSelection  bool
	IsEditable    bool
	CanGoBack     bool
	CanGoForward  bool
	X             int
	Y             int
}

// MenuItem is a normalized context menu entry.
type MenuItem struct {
	Action MenuAction
	Label  string
}

// ContextMenuBuilder builds normalized menu items from menu state.
type ContextMenuBuilder interface {
	Build(ctx context.Context, menuContext MenuContext) []MenuItem
}

// ContextMenuActionExecutor executes normalized context menu actions.
type ContextMenuActionExecutor interface {
	ExecuteMenuAction(ctx context.Context, action MenuAction, menuContext MenuContext) error
}

// ContextMenuActionExecutorFactory creates context menu action executors.
type ContextMenuActionExecutorFactory interface {
	NewContextMenuActionExecutor(
		clipboard Clipboard,
		resolver ImageDataResolver,
		saver ResolvedImageSaver,
		delegator MenuActionDelegator,
	) ContextMenuActionExecutor
}

// MenuActionDelegator handles actions that are not shared application concerns.
type MenuActionDelegator interface {
	DelegateMenuAction(ctx context.Context, action MenuAction, menuContext MenuContext) error
}

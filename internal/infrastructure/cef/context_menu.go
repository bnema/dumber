package cef

import (
	"context"
	purecef "github.com/bnema/purego-cef/cef"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/contextmenu"
)

// ===========================================================================
// ContextMenuHandler (implemented on handlerSet)
// ===========================================================================

func (h *handlerSet) OnBeforeContextMenu(_ purecef.Browser, _ purecef.Frame, _ purecef.ContextMenuParams, _ purecef.MenuModel) {
}

func (h *handlerSet) RunContextMenu(
	_ purecef.Browser, _ purecef.Frame,
	params purecef.ContextMenuParams, model purecef.MenuModel,
	callback purecef.RunContextMenuCallback,
) int32 {
	if h == nil || callback == nil || model == nil {
		if callback != nil {
			callback.Cancel()
		}
		return 1
	}

	menuContext := buildMenuContext(h.wv, params)
	items := usecase.NewBuildContextMenuUseCase().Build(context.Background(), menuContext)
	if len(items) == 0 {
		callback.Cancel()
		return 1
	}

	// Snapshot the command IDs from CEF — model/params pointers are only valid during this call.
	count := model.GetCount()
	if count == 0 {
		callback.Cancel()
		return 1
	}
	commandIDsByLabel := make(map[string]int32, count)
	for i := range count {
		t := model.GetTypeAt(i)
		if t == purecef.MenuItemTypeMenuitemtypeSeparator {
			continue
		}
		if t == purecef.MenuItemTypeMenuitemtypeCommand {
			label := strings.ToLower(strings.TrimSpace(model.GetLabelAt(i)))
			if label != "" {
				commandIDsByLabel[label] = model.GetCommandIDAt(i)
			}
		}
	}
	commandIDByAction := make(map[port.MenuAction]int32, len(items))
	for _, item := range items {
		if cmdID, ok := lookupContextMenuCommandID(item.Action, item.Label, commandIDsByLabel); ok {
			commandIDByAction[item.Action] = cmdID
		}
	}
	if h.wv == nil || h.wv.pipeline == nil || h.wv.pipeline.glArea == nil {
		callback.Cancel()
		return 1
	}

	x := params.GetXcoord()
	y := params.GetYcoord()
	glArea := h.wv.pipeline.glArea

	contextmenu.NewRenderer(h.wv.runOnGTK).Show(
		items,
		&glArea.Widget,
		x,
		y,
		func(item port.MenuItem) { dispatchContextMenuSelection(callback, commandIDByAction, item) },
		func() {
			callback.Cancel()
		},
	)
	return 1
}

func dispatchContextMenuSelection(
	callback purecef.RunContextMenuCallback,
	commandIDByAction map[port.MenuAction]int32,
	item port.MenuItem,
) {
	if callback == nil {
		return
	}
	if cmdID, ok := commandIDByAction[item.Action]; ok {
		callback.Cont(cmdID, 0)
		return
	}
	callback.Cancel()
}

func lookupContextMenuCommandID(action port.MenuAction, label string, commandIDsByLabel map[string]int32) (int32, bool) {
	for _, candidate := range contextMenuActionLabels(action, label) {
		if cmdID, ok := commandIDsByLabel[strings.ToLower(candidate)]; ok {
			return cmdID, true
		}
	}
	return 0, false
}

func contextMenuActionLabels(action port.MenuAction, label string) []string {
	labels := []string{label}
	switch action {
	case port.MenuActionBack:
		labels = append(labels, "Back")
	case port.MenuActionForward:
		labels = append(labels, "Forward")
	case port.MenuActionReload:
		labels = append(labels, "Reload")
	case port.MenuActionOpenLinkNewTab:
		labels = append(labels, "Open Link in New Tab", "Open link in new tab")
	case port.MenuActionCopyLink:
		labels = append(labels, "Copy Link", "Copy link address")
	case port.MenuActionCopyImage:
		labels = append(labels, "Copy Image", "Copy image")
	case port.MenuActionSaveImage:
		labels = append(labels, "Save Image", "Save image as...")
	case port.MenuActionInspectElement:
		labels = append(labels, "Inspect Element", "Inspect")
	case port.MenuActionCopySelection:
		labels = append(labels, "Copy Selection", "Copy")
	}
	return labels
}

func buildMenuContext(wv *WebView, params purecef.ContextMenuParams) port.MenuContext {
	menuContext := port.MenuContext{}
	if wv != nil {
		menuContext.CanGoBack = wv.CanGoBack()
		menuContext.CanGoForward = wv.CanGoForward()
		menuContext.PageURI = wv.URI()
	}
	if params == nil {
		return menuContext
	}

	if pageURI := params.GetPageURL(); pageURI != "" {
		menuContext.PageURI = pageURI
	}
	menuContext.LinkURI = params.GetLinkURL()
	if params.HasImageContents() {
		menuContext.ImageURI = params.GetSourceURL()
	}
	menuContext.HasSelection = strings.TrimSpace(params.GetSelectionText()) != ""
	menuContext.IsEditable = params.IsEditable()
	menuContext.X = int(params.GetXcoord())
	menuContext.Y = int(params.GetYcoord())
	return menuContext
}

func (h *handlerSet) OnContextMenuCommand(
	_ purecef.Browser, _ purecef.Frame, _ purecef.ContextMenuParams,
	_ int32, _ purecef.EventFlags,
) int32 {
	return 0
}

func (h *handlerSet) OnContextMenuDismissed(_ purecef.Browser, _ purecef.Frame) {}

func (h *handlerSet) RunQuickMenu(
	_ purecef.Browser, _ purecef.Frame, _ *purecef.Point, _ *purecef.Size,
	_ purecef.QuickMenuEditStateFlags, _ purecef.RunQuickMenuCallback,
) int32 {
	return 0
}

func (h *handlerSet) OnQuickMenuCommand(_ purecef.Browser, _ purecef.Frame, _ int32, _ purecef.EventFlags) int32 {
	return 0
}

func (h *handlerSet) OnQuickMenuDismissed(_ purecef.Browser, _ purecef.Frame) {}

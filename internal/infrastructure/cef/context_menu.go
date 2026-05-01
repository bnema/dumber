package cef

import (
	"context"
	"fmt"
	"strings"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gtk"
)

// ContextMenuRenderer renders normalized context menu items for the CEF adapter.
// It is implemented by the GTK menu adapter and injected by bootstrap so this
// adapter does not depend on a sibling infrastructure adapter.
type ContextMenuRenderer interface {
	Show(items []port.MenuItem, anchor *gtk.Widget, x, y int32, onSelect func(port.MenuItem), onClose func())
}

// ===========================================================================
// ContextMenuHandler (implemented on handlerSet)
// ===========================================================================

func (h *handlerSet) OnBeforeContextMenu(_ purecef.Browser, _ purecef.Frame, _ purecef.ContextMenuParams, _ purecef.MenuModel) {
}

//nolint:gocyclo // CEF callback coordinates menu extraction, rendering, and native fallback in one synchronous call.
func (h *handlerSet) RunContextMenu(
	_ purecef.Browser, _ purecef.Frame,
	params purecef.ContextMenuParams, model purecef.MenuModel,
	callback purecef.RunContextMenuCallback,
) int32 {
	if h == nil || callback == nil || model == nil {
		if callback != nil {
			callback.Cancel()
		}
		// Returning 1 tells CEF we handled the menu, so cancel suppresses the default menu.
		return 1
	}
	if h.wv == nil || h.wv.engine == nil || h.wv.engine.ctxMenuBuilder == nil {
		// Returning 0 lets CEF show its native menu when no custom builder is wired.
		return 0
	}

	menuContext := buildMenuContext(h.wv, params)
	items := h.wv.engine.ctxMenuBuilder.Build(context.Background(), menuContext)
	if len(items) == 0 {
		return 0
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
	if h.wv == nil || h.wv.viewBridge == nil {
		return 0
	}

	x, y := contextMenuAnchorPosition(params, h.wv.viewBridgeScale())
	executor := h.contextMenuExecutor()
	wv := h.wv
	wv.runOnGTK(func() {
		if wv.viewBridge == nil || wv.viewBridge.Widget() == nil {
			callback.Cancel()
			return
		}
		widget := wv.viewBridge.Widget()
		logContextMenuPopupRequest(h, widget, params, x, y)
		renderer := wv.engine.contextMenuRenderer()
		if renderer == nil {
			callback.Cancel()
			return
		}
		renderer.Show(
			items,
			widget,
			x,
			y,
			func(item port.MenuItem) {
				var copiedCallback func(string)
				if wv.engine != nil {
					copiedCallback = wv.engine.notifyClipboardCopied
				}
				dispatchContextMenuSelection(wv.ctx, executor, callback, copiedCallback, commandIDByAction, item, menuContext)
			},
			func() {
				callback.Cancel()
			},
		)
	})
	return 1
}

func (h *handlerSet) contextMenuExecutor() port.ContextMenuActionExecutor {
	if h == nil || h.wv == nil || h.wv.engine == nil || h.wv.engine.ctxMenuExecutorFactory == nil {
		return nil
	}
	return h.wv.engine.ctxMenuExecutorFactory.NewContextMenuActionExecutor(
		h.wv.engine.clipboard,
		h.wv.engine.resolver,
		nil,
		&cefMenuDelegator{wv: h.wv},
	)
}

func contextMenuAnchorPosition(params purecef.ContextMenuParams, scale int32) (int32, int32) {
	if params == nil {
		return 0, 0
	}
	if scale <= 0 {
		scale = 1
	}
	return params.GetXcoord() / scale, params.GetYcoord() / scale
}

func logContextMenuPopupRequest(
	h *handlerSet,
	widget *gtk.Widget,
	params purecef.ContextMenuParams,
	x, y int32,
) {
	if h == nil || h.wv == nil || h.wv.ctx == nil || widget == nil {
		return
	}
	rawX, rawY := contextMenuRawPosition(params)
	parent := widget.GetParent()
	logging.FromContext(h.wv.ctx).Debug().
		Int32("raw_x", rawX).
		Int32("raw_y", rawY).
		Int32("popup_x", x).
		Int32("popup_y", y).
		Int32("scale", h.wv.viewBridgeScale()).
		Int("anchor_width", widget.GetAllocatedWidth()).
		Int("anchor_height", widget.GetAllocatedHeight()).
		Int("parent_width", cefWidgetAllocatedWidth(parent)).
		Int("parent_height", cefWidgetAllocatedHeight(parent)).
		Msg("cef: context menu popup request")
}

func contextMenuRawPosition(params purecef.ContextMenuParams) (int32, int32) {
	if params == nil {
		return 0, 0
	}
	return params.GetXcoord(), params.GetYcoord()
}

func cefWidgetAllocatedWidth(widget *gtk.Widget) int {
	if widget == nil {
		return 0
	}
	return widget.GetAllocatedWidth()
}

func cefWidgetAllocatedHeight(widget *gtk.Widget) int {
	if widget == nil {
		return 0
	}
	return widget.GetAllocatedHeight()
}

func dispatchContextMenuSelection(
	ctx context.Context,
	executor port.ContextMenuActionExecutor,
	callback purecef.RunContextMenuCallback,
	copiedCallback func(text string),
	commandIDByAction map[port.MenuAction]int32,
	item port.MenuItem,
	menuContext port.MenuContext,
) {
	log := logging.FromContext(ctx)
	if callback == nil {
		return
	}
	log.Debug().Str("action", string(item.Action)).Str("label", item.Label).Msg("cef: context menu item selected")
	if item.Action == port.MenuActionCopySelection && menuContext.SelectionText == "" {
		if cmdID, ok := commandIDByAction[item.Action]; ok {
			log.Debug().Str("action", string(item.Action)).Int32("command_id", cmdID).Msg("cef: continuing native context menu command")
			callback.Cont(cmdID, 0)
			return
		}
		log.Warn().Str("action", string(item.Action)).Msg("cef: no matching native context menu command")
		callback.Cancel()
		return
	}
	if shouldExecuteDirectCEFAction(item.Action) && executor != nil {
		if err := executor.ExecuteMenuAction(ctx, item.Action, menuContext); err != nil {
			log.Warn().Err(err).Str("action", string(item.Action)).Msg("cef: context menu action failed")
			callback.Cancel()
			return
		}
		if copiedCallback != nil {
			if copiedText := contextMenuCopiedText(item.Action, menuContext); copiedText != "" {
				copiedCallback(copiedText)
			}
		}
		log.Debug().Str("action", string(item.Action)).Msg("cef: context menu action executed directly")
		callback.Cancel()
		return
	}
	if cmdID, ok := commandIDByAction[item.Action]; ok {
		log.Debug().Str("action", string(item.Action)).Int32("command_id", cmdID).Msg("cef: continuing native context menu command")
		callback.Cont(cmdID, 0)
		return
	}
	log.Warn().Str("action", string(item.Action)).Msg("cef: no matching native context menu command")
	callback.Cancel()
}

func contextMenuCopiedText(action port.MenuAction, menuContext port.MenuContext) string {
	switch action {
	case port.MenuActionCopySelection:
		return menuContext.SelectionText
	case port.MenuActionCopyLink:
		return menuContext.LinkURI
	default:
		return ""
	}
}

func shouldExecuteDirectCEFAction(action port.MenuAction) bool {
	switch action {
	case port.MenuActionBack,
		port.MenuActionForward,
		port.MenuActionReload,
		port.MenuActionOpenLinkNewTab,
		port.MenuActionCopyLink,
		port.MenuActionCopyImage,
		port.MenuActionInspectElement,
		port.MenuActionCopySelection:
		return true
	default:
		return false
	}
}

type cefMenuDelegator struct {
	wv *WebView
}

func (d *cefMenuDelegator) DelegateMenuAction(ctx context.Context, action port.MenuAction, menuContext port.MenuContext) error {
	if d == nil || d.wv == nil {
		return fmt.Errorf("cef menu delegator: webview not available")
	}
	switch action {
	case port.MenuActionBack:
		return d.wv.GoBack(ctx)
	case port.MenuActionForward:
		return d.wv.GoForward(ctx)
	case port.MenuActionReload:
		return d.wv.Reload(ctx)
	case port.MenuActionOpenLinkNewTab:
		if menuContext.LinkURI == "" {
			return fmt.Errorf("open link in new tab: link URI not available")
		}
		d.wv.mu.RLock()
		cb := d.wv.callbacks
		d.wv.mu.RUnlock()
		if cb == nil || cb.OnLinkMiddleClick == nil {
			return fmt.Errorf("open link in new tab: middle-click handler not available")
		}
		cb.OnLinkMiddleClick(menuContext.LinkURI)
		return nil
	case port.MenuActionInspectElement:
		d.wv.OpenDevTools()
		return nil
	case port.MenuActionCopySelection:
		d.wv.RunJavaScript(ctx, "document.execCommand('copy');")
		return nil
	default:
		return fmt.Errorf("cef menu delegator: unsupported action %s", action)
	}
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
	menuContext.SelectionText = params.GetSelectionText()
	menuContext.HasSelection = strings.TrimSpace(menuContext.SelectionText) != ""
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

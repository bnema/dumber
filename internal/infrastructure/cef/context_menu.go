package cef

import (
	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
)

// Compile-time interface check.
var _ purecef.ContextMenuHandler = (*contextMenuHandler)(nil)

// contextMenuHandler builds a native GTK4 popover from CEF's menu model.
// In OSR mode CEF cannot render its own context menu.
type contextMenuHandler struct {
	wv *WebView
}

// menuEntry holds a snapshot of one CEF menu item.
type menuEntry struct {
	label     string
	commandID int32
	isSep     bool
}

func (h *contextMenuHandler) OnBeforeContextMenu(_ purecef.Browser, _ purecef.Frame, _ purecef.ContextMenuParams, _ purecef.MenuModel) {
}

func (h *contextMenuHandler) RunContextMenu(
	_ purecef.Browser, _ purecef.Frame,
	params purecef.ContextMenuParams, model purecef.MenuModel,
	callback purecef.RunContextMenuCallback,
) int32 {
	count := model.GetCount()
	if count == 0 {
		callback.Cancel()
		return 1
	}

	// Snapshot menu items — model/params pointers are only valid during this call.
	items := make([]menuEntry, 0, count)
	for i := range count {
		t := model.GetTypeAt(i)
		if t == purecef.MenuItemTypeMenuitemtypeSeparator {
			items = append(items, menuEntry{isSep: true})
			continue
		}
		if t == purecef.MenuItemTypeMenuitemtypeCommand {
			items = append(items, menuEntry{
				label:     model.GetLabelAt(i),
				commandID: model.GetCommandIDAt(i),
			})
		}
	}
	if len(items) == 0 {
		callback.Cancel()
		return 1
	}

	x := params.GetXcoord()
	y := params.GetYcoord()

	h.wv.runOnGTK(func() {
		h.showMenu(items, x, y, callback)
	})
	return 1
}

func (h *contextMenuHandler) showMenu(items []menuEntry, x, y int32, callback purecef.RunContextMenuCallback) {
	glArea := h.wv.pipeline.glArea
	if glArea == nil {
		callback.Cancel()
		return
	}

	// Build a vertical box of buttons inside a popover.
	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	box.AddCssClass("context-menu")

	selected := false
	var popover *gtk.Popover

	for _, item := range items {
		if item.isSep {
			sep := gtk.NewSeparator(gtk.OrientationHorizontalValue)
			box.Append(&sep.Widget)
			continue
		}

		btn := gtk.NewButton()
		btn.SetLabel(item.label)
		btn.AddCssClass("flat")

		cmdID := item.commandID
		clickCb := func(_ gtk.Button) {
			selected = true
			callback.Cont(cmdID, 0)
			if popover != nil {
				popover.Popdown()
			}
		}
		btn.ConnectClicked(&clickCb)
		box.Append(&btn.Widget)
	}

	popover = gtk.NewPopover()
	popover.SetChild(&box.Widget)
	popover.SetParent(&glArea.Widget)
	popover.SetHasArrow(false)
	popover.SetAutohide(true)

	rect := &gdk.Rectangle{X: int(x), Y: int(y), Width: 1, Height: 1}
	popover.SetPointingTo(rect)

	closedCb := func(_ gtk.Popover) {
		if !selected {
			callback.Cancel()
		}
		popover.Unparent()
	}
	popover.ConnectClosed(&closedCb)

	popover.Popup()
}

func (h *contextMenuHandler) OnContextMenuCommand(
	_ purecef.Browser, _ purecef.Frame, _ purecef.ContextMenuParams,
	_ int32, _ purecef.EventFlags,
) int32 {
	return 0
}

func (h *contextMenuHandler) OnContextMenuDismissed(_ purecef.Browser, _ purecef.Frame) {}

func (h *contextMenuHandler) RunQuickMenu(
	_ purecef.Browser, _ purecef.Frame, _ *purecef.Point, _ *purecef.Size,
	_ purecef.QuickMenuEditStateFlags, _ purecef.RunQuickMenuCallback,
) int32 {
	return 0
}

func (h *contextMenuHandler) OnQuickMenuCommand(_ purecef.Browser, _ purecef.Frame, _ int32, _ purecef.EventFlags) int32 {
	return 0
}

func (h *contextMenuHandler) OnQuickMenuDismissed(_ purecef.Browser, _ purecef.Frame) {}

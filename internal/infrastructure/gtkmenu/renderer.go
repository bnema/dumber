package gtkmenu

import (
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
)

type renderItem struct {
	item      port.MenuItem
	separator bool
}

// Renderer shows GTK context menus on the provided anchor widget.
type Renderer struct {
	dispatch func(func())
}

// NewRenderer creates a new Renderer.
func NewRenderer(dispatch func(func())) *Renderer {
	return &Renderer{dispatch: dispatch}
}

func BuildButtons(items []port.MenuItem) []renderItem {
	renderItems := make([]renderItem, 0, len(items))
	for _, item := range items {
		if isSeparator(item) {
			renderItems = append(renderItems, renderItem{separator: true})
			continue
		}
		renderItems = append(renderItems, renderItem{item: item})
	}
	return renderItems
}

func isSeparator(item port.MenuItem) bool {
	return item.Action == "" && item.Label == ""
}

// Show renders a popover menu on the GTK thread.
func (r *Renderer) Show(
	items []port.MenuItem,
	anchor *gtk.Widget,
	x, y int32,
	onSelect func(port.MenuItem),
	onClose func(),
) {
	if r == nil {
		return
	}
	run := func() {
		showMenu(anchor, BuildButtons(items), x, y, onSelect, onClose)
	}
	if r.dispatch == nil {
		run()
		return
	}
	r.dispatch(run)
}

func showMenu(
	anchor *gtk.Widget,
	items []renderItem,
	x, y int32,
	onSelect func(port.MenuItem),
	onClose func(),
) {
	if anchor == nil {
		if onClose != nil {
			onClose()
		}
		return
	}
	if !hasRenderableItems(items) {
		if onClose != nil {
			onClose()
		}
		return
	}

	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	box.AddCssClass("context-menu")

	selected := false
	var popover *gtk.Popover
	parent := ChoosePopoverParent(anchor, anchor.GetParent())
	popoverHost, overlay := createPopoverHost(parent, x, y)

	for _, item := range items {
		if item.separator {
			sep := gtk.NewSeparator(gtk.OrientationHorizontalValue)
			box.Append(&sep.Widget)
			continue
		}

		btn := gtk.NewButton()
		btn.SetLabel(item.item.Label)
		btn.AddCssClass("flat")

		menuItem := item.item
		clickCb := func(_ gtk.Button) {
			selected = true
			if onSelect != nil {
				onSelect(menuItem)
			}
			if popover != nil {
				popover.Popdown()
			}
		}
		btn.ConnectClicked(&clickCb)
		box.Append(&btn.Widget)
	}

	popover = gtk.NewPopover()
	popover.SetChild(&box.Widget)
	attachPopover(popover, popoverHost, parent)
	popover.SetHasArrow(false)
	popover.SetAutohide(true)

	if popoverHost == nil {
		rect := PopoverPointingRect(x, y, func(srcX, srcY float64) (float64, float64, bool) {
			if anchor == nil || parent == nil || anchor == parent {
				return srcX, srcY, false
			}
			var destX, destY float64
			if !anchor.TranslateCoordinates(parent, srcX, srcY, &destX, &destY) {
				return srcX, srcY, false
			}
			return destX, destY, true
		})
		popover.SetPointingTo(rect)
	}

	closedCb := func(_ gtk.Popover) {
		if !selected && onClose != nil {
			onClose()
		}
		cleanupPopover(popover, popoverHost, overlay)
	}
	popover.ConnectClosed(&closedCb)

	popupPopover(popover, popoverHost)
}

// ChoosePopoverParent picks the best parent for a GTK popover.
func ChoosePopoverParent(anchor, parent *gtk.Widget) *gtk.Widget {
	if parent != nil {
		return parent
	}
	return anchor
}

// PopoverPointingRect builds the popover pointing rectangle.
func PopoverPointingRect(
	x, y int32,
	translate func(srcX, srcY float64) (float64, float64, bool),
) *gdk.Rectangle {
	destX, destY := float64(x), float64(y)
	if translate != nil {
		if tx, ty, ok := translate(destX, destY); ok {
			destX, destY = tx, ty
		}
	}
	return &gdk.Rectangle{X: int(destX), Y: int(destY), Width: 1, Height: 1}
}

func createPopoverHost(parent *gtk.Widget, x, y int32) (*gtk.MenuButton, *gtk.Overlay) {
	overlay := overlayFromWidget(parent)
	if overlay == nil {
		return nil, nil
	}
	host := gtk.NewMenuButton()
	host.SetCanFocus(false)
	host.SetHalign(gtk.AlignStartValue)
	host.SetValign(gtk.AlignStartValue)
	host.SetMarginStart(int(x))
	host.SetMarginTop(int(y))
	host.SetOpacity(0)
	host.SetSizeRequest(1, 1)
	host.SetVisible(true)
	overlay.AddOverlay(&host.Widget)
	overlay.SetClipOverlay(&host.Widget, false)
	overlay.SetMeasureOverlay(&host.Widget, false)
	return host, overlay
}

func overlayFromWidget(widget *gtk.Widget) *gtk.Overlay {
	if widget == nil || widget.GoPointer() == 0 {
		return nil
	}
	return gtk.OverlayNewFromInternalPtr(widget.GoPointer())
}

func attachPopover(popover *gtk.Popover, host *gtk.MenuButton, parent *gtk.Widget) {
	if host != nil {
		host.SetPopover(popover)
		return
	}
	popover.SetParent(parent)
}

func cleanupPopover(popover *gtk.Popover, host *gtk.MenuButton, overlay *gtk.Overlay) {
	if host != nil && overlay != nil {
		host.SetPopover(nil)
		overlay.RemoveOverlay(&host.Widget)
		return
	}
	popover.Unparent()
}

func popupPopover(popover *gtk.Popover, host *gtk.MenuButton) {
	if host != nil {
		host.Popup()
		return
	}
	popover.Popup()
}

func hasRenderableItems(items []renderItem) bool {
	for _, item := range items {
		if !item.separator {
			return true
		}
	}
	return false
}

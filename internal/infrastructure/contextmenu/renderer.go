package contextmenu

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

func buildButtons(items []port.MenuItem) []renderItem {
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
		showMenu(anchor, buildButtons(items), x, y, onSelect, onClose)
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

	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	box.AddCssClass("context-menu")

	selected := false
	var popover *gtk.Popover

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

	if len(items) == 0 {
		if onClose != nil {
			onClose()
		}
		return
	}

	popover = gtk.NewPopover()
	popover.SetChild(&box.Widget)
	popover.SetParent(anchor)
	popover.SetHasArrow(false)
	popover.SetAutohide(true)

	rect := &gdk.Rectangle{X: int(x), Y: int(y), Width: 1, Height: 1}
	popover.SetPointingTo(rect)

	closedCb := func(_ gtk.Popover) {
		if !selected && onClose != nil {
			onClose()
		}
		popover.Unparent()
	}
	popover.ConnectClosed(&closedCb)

	popover.Popup()
}

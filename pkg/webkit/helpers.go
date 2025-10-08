package webkit

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// IsNativeAvailable returns true if the native GTK/WebKit libraries are available
// In gotk4, this is always true if the import succeeded
func IsNativeAvailable() bool {
	return true
}

// IdleAdd adds a function to be called in an idle callback
// Returns an idle source ID that can be used with IdleRemove
func IdleAdd(fn func() bool) uint {
	return uint(glib.IdleAdd(fn))
}

// IdleRemove removes an idle callback by its source ID
func IdleRemove(sourceID uint) {
	glib.SourceRemove(glib.SourceHandle(sourceID))
}

// WidgetIsValid checks if a widget pointer is valid (non-nil)
func WidgetIsValid(w gtk.Widgetter) bool {
	return w != nil
}

// WidgetSetMargin sets all margins for a widget
func WidgetSetMargin(w gtk.Widgetter, margin int) {
	if widget := getWidget(w); widget != nil {
		widget.SetMarginStart(margin)
		widget.SetMarginEnd(margin)
		widget.SetMarginTop(margin)
		widget.SetMarginBottom(margin)
	}
}

// WidgetAddCSSClass adds a CSS class to a widget
func WidgetAddCSSClass(w gtk.Widgetter, className string) {
	if widget := getWidget(w); widget != nil {
		widget.AddCSSClass(className)
	}
}

// WidgetRemoveCSSClass removes a CSS class from a widget
func WidgetRemoveCSSClass(w gtk.Widgetter, className string) {
	if widget := getWidget(w); widget != nil {
		widget.RemoveCSSClass(className)
	}
}

// WidgetHasCSSClass checks if a widget has a CSS class
func WidgetHasCSSClass(w gtk.Widgetter, className string) bool {
	if widget := getWidget(w); widget != nil {
		return widget.HasCSSClass(className)
	}
	return false
}

// WidgetSetHExpand sets whether the widget should expand horizontally
func WidgetSetHExpand(w gtk.Widgetter, expand bool) {
	if widget := getWidget(w); widget != nil {
		widget.SetHExpand(expand)
	}
}

// WidgetSetVExpand sets whether the widget should expand vertically
func WidgetSetVExpand(w gtk.Widgetter, expand bool) {
	if widget := getWidget(w); widget != nil {
		widget.SetVExpand(expand)
	}
}

// WidgetRealizeInContainer realizes a widget within a container
func WidgetRealizeInContainer(w gtk.Widgetter, container gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.Realize()
	}
}

// WidgetGetParent returns the parent widget
func WidgetGetParent(w gtk.Widgetter) gtk.Widgetter {
	if widget := getWidget(w); widget != nil {
		return widget.Parent()
	}
	return nil
}

// WidgetUnparent removes a widget from its parent
func WidgetUnparent(w gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.Unparent()
	}
}

// WidgetQueueAllocate queues an allocate operation
func WidgetQueueAllocate(w gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.QueueAllocate()
	}
}

// WidgetQueueDraw queues a redraw operation
func WidgetQueueDraw(w gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.QueueDraw()
	}
}

// WidgetGetAllocation returns the widget's allocated area
func WidgetGetAllocation(w gtk.Widgetter) (x, y, width, height int) {
	if widget := getWidget(w); widget != nil {
		alloc := widget.Allocation()
		return alloc.X(), alloc.Y(), alloc.Width(), alloc.Height()
	}
	return 0, 0, 0, 0
}

// WidgetGetBounds returns the widget's bounds relative to its parent
func WidgetGetBounds(w gtk.Widgetter) (x, y, width, height float64) {
	if widget := getWidget(w); widget != nil {
		parent := widget.Parent()
		if parent != nil {
			rect, ok := widget.ComputeBounds(parent)
			if ok && rect != nil {
				// Get bounds directly from graphene.Rect
				return float64(rect.X()), float64(rect.Y()),
					float64(rect.Width()), float64(rect.Height())
			}
		}
	}
	return 0, 0, 0, 0
}

// WidgetSetFocusChild sets the focus child of a widget
func WidgetSetFocusChild(w gtk.Widgetter, child gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.SetFocusChild(child)
	}
}

// WidgetAddFocusController adds a focus event controller to a widget
func WidgetAddFocusController(w gtk.Widgetter, onEnter, onLeave func()) *gtk.EventControllerFocus {
	if widget := getWidget(w); widget != nil {
		controller := gtk.NewEventControllerFocus()
		if onEnter != nil {
			controller.ConnectEnter(onEnter)
		}
		if onLeave != nil {
			controller.ConnectLeave(onLeave)
		}
		widget.AddController(controller)
		return controller
	}
	return nil
}

// WidgetRemoveFocusController removes a focus event controller from a widget
func WidgetRemoveFocusController(w gtk.Widgetter, controller *gtk.EventControllerFocus) {
	if widget := getWidget(w); widget != nil && controller != nil {
		widget.RemoveController(controller)
	}
}

// PrefersDarkTheme checks if the system prefers dark theme
func PrefersDarkTheme() bool {
	settings := gtk.SettingsGetDefault()
	if settings != nil {
		// In GTK4, check the color scheme preference
		// The property is "gtk-application-prefer-dark-theme"
		obj := settings.Object
		if obj != nil {
			prop := obj.ObjectProperty("gtk-application-prefer-dark-theme")
			if v, ok := prop.(bool); ok && v {
				return true
			}
		}
	}
	return false
}

// AddCSSProvider adds a CSS provider for styling
func AddCSSProvider(css string) error {
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(css)

	gtk.StyleContextAddProviderForDisplay(
		gdk.DisplayGetDefault(),
		provider,
		gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
	)

	return nil
}

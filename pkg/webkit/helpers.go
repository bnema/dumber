package webkit

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
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

// WidgetSetMarginTop sets the top margin for a widget
func WidgetSetMarginTop(w gtk.Widgetter, margin int) {
	if widget := getWidget(w); widget != nil {
		widget.SetMarginTop(margin)
	}
}

// WidgetSetMarginBottom sets the bottom margin for a widget
func WidgetSetMarginBottom(w gtk.Widgetter, margin int) {
	if widget := getWidget(w); widget != nil {
		widget.SetMarginBottom(margin)
	}
}

// WidgetSetMarginStart sets the start margin for a widget
func WidgetSetMarginStart(w gtk.Widgetter, margin int) {
	if widget := getWidget(w); widget != nil {
		widget.SetMarginStart(margin)
	}
}

// WidgetSetMarginEnd sets the end margin for a widget
func WidgetSetMarginEnd(w gtk.Widgetter, margin int) {
	if widget := getWidget(w); widget != nil {
		widget.SetMarginEnd(margin)
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

// WidgetRealize realizes a widget
func WidgetRealize(w gtk.Widgetter) {
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

// WidgetGetWindowBounds returns the widget's bounds relative to the toplevel window
// This provides window-absolute coordinates needed for navigation geometry calculations
func WidgetGetWindowBounds(w gtk.Widgetter) (x, y, width, height int) {
	if widget := getWidget(w); widget != nil {
		// Get the toplevel window (GtkWindow)
		root := widget.Root()
		if root != nil {
			rect, ok := widget.ComputeBounds(root)
			if ok && rect != nil {
				return int(rect.X()), int(rect.Y()),
					int(rect.Width()), int(rect.Height())
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

// WidgetAddHoverHandler registers a hover callback for the given widget and
// returns a controller that can be used to detach it later.
func WidgetAddHoverHandler(w gtk.Widgetter, fn func()) *gtk.EventControllerMotion {
	if widget := getWidget(w); widget != nil && fn != nil {
		motion := gtk.NewEventControllerMotion()
		motion.SetPropagationPhase(gtk.PhaseCapture)

		// Connect to enter signal - triggered when mouse enters the widget
		motion.ConnectEnter(func(x, y float64) {
			fn()
		})

		widget.AddController(motion)
		return motion
	}
	return nil
}

// WidgetRemoveHoverHandler removes a previously registered hover controller
func WidgetRemoveHoverHandler(w gtk.Widgetter, controller *gtk.EventControllerMotion) {
	if widget := getWidget(w); widget != nil && controller != nil {
		widget.RemoveController(controller)
	}
}

// WidgetAddController adds an event controller to a widget
func WidgetAddController(w gtk.Widgetter, controller gtk.EventControllerer) {
	if widget := getWidget(w); widget != nil && controller != nil {
		widget.AddController(controller)
	}
}

// WidgetAllocation returns the widget's allocation (alias for WidgetGetAllocation)
func WidgetAllocation(w gtk.Widgetter) (x, y, width, height int) {
	return WidgetGetAllocation(w)
}

// PrefersDarkTheme checks if the system prefers dark theme
func PrefersDarkTheme() bool {
	// First try to read GNOME's color-scheme setting (GTK4+)
	// Check if the schema exists before attempting to create GSettings object
	// to avoid panics on non-GNOME systems (e.g., Sway, Hyprland, etc.)
	var colorScheme string
	schemaSource := gio.SettingsSchemaSourceGetDefault()
	if schemaSource != nil {
		schema := schemaSource.Lookup("org.gnome.desktop.interface", true)
		if schema != nil {
			if gnomeSettings := gio.NewSettings("org.gnome.desktop.interface"); gnomeSettings != nil {
				colorScheme = gnomeSettings.String("color-scheme")
			}
		}
	}

	switch colorScheme {
	case "prefer-dark":
		return true
	case "prefer-light":
		return false
	}

	// Fallback to GTK's application-level preference
	settings := gtk.SettingsGetDefault()
	if settings != nil {
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

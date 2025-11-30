package webkit

import (
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/logging"
	gdk "github.com/diamondburned/gotk4/pkg/gdk/v4"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	gtk "github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// WindowShortcuts manages keyboard shortcuts for a window
type WindowShortcuts struct {
	window     *Window
	shortcuts  map[string]*gtk.Shortcut
	controller *gtk.ShortcutController
}

// NewWindowShortcuts creates a new WindowShortcuts manager
func NewWindowShortcuts(win *Window) *WindowShortcuts {
	controller := gtk.NewShortcutController()

	// Set scope to GLOBAL so shortcuts work at the window level
	// and aren't captured by child widgets (like the WebView)
	controller.SetScope(gtk.ShortcutScopeGlobal)

	if win != nil && win.win != nil {
		win.win.AddController(controller)
	}

	return &WindowShortcuts{
		window:     win,
		shortcuts:  make(map[string]*gtk.Shortcut),
		controller: controller,
	}
}

// InitializeGlobalShortcuts initializes global keyboard shortcuts for a window
func (w *Window) InitializeGlobalShortcuts(shortcuts map[string]func()) {
	if w == nil || w.win == nil {
		return
	}

	// Create event controller for keyboard events
	controller := gtk.NewEventControllerKey()

	controller.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		// Handle keyboard shortcuts
		// TODO: Implement proper shortcut matching
		return false
	})

	w.win.AddController(controller)
}

// RegisterShortcut registers a keyboard shortcut
func (ws *WindowShortcuts) RegisterShortcut(accelerator string, handler func()) {
	if ws == nil || ws.controller == nil {
		return
	}

	// Convert to GTK format (e.g., "ctrl+plus" -> "<Control>plus")
	gtkFormat := convertToGtkFormat(accelerator)
	if gtkFormat == "" {
		logging.Warn(fmt.Sprintf("[shortcuts] Invalid key format: %s", accelerator))
		return
	}

	// Create a callback action that will trigger our handler
	// ShortcutFunc signature: func(widget Widgetter, args *glib.Variant) (ok bool)
	action := gtk.NewCallbackAction(func(widget gtk.Widgetter, args *glib.Variant) bool {
		if handler != nil {
			logging.Debug(fmt.Sprintf("[shortcuts] Shortcut triggered: %s", accelerator))
			handler()
		}
		// Return false to allow event propagation to WebView
		// This lets JavaScript KeyboardService also handle events if needed
		return false
	})

	// Parse the accelerator string into a GDK key trigger
	// GTK4 uses gtk.ShortcutTrigger for key combinations
	trigger := gtk.NewShortcutTriggerParseString(gtkFormat)
	if trigger == nil {
		logging.Error(fmt.Sprintf("[shortcuts] Failed to parse trigger: %s", gtkFormat))
		return
	}

	// Create the shortcut with trigger and action
	shortcut := gtk.NewShortcut(trigger, action)

	// Add shortcut to the shared controller
	ws.controller.AddShortcut(shortcut)

	// Store the shortcut for potential cleanup
	ws.shortcuts[accelerator] = shortcut

	logging.Debug(fmt.Sprintf("[shortcuts] Registered: %s -> %s", accelerator, gtkFormat))
}

// convertToGtkFormat converts common key formats to GTK shortcut format
// Examples: "ctrl+plus" -> "<Control>plus", "ctrl+l" -> "<Control>l"
func convertToGtkFormat(key string) string {
	key = strings.ToLower(key)

	// Handle cmdorctrl -> Control
	key = strings.ReplaceAll(key, "cmdorctrl+", "ctrl+")
	key = strings.ReplaceAll(key, "cmd+", "ctrl+")

	// Split by + to get components
	components := strings.Split(key, "+")
	if len(components) == 0 {
		return ""
	}

	var parts []string
	for i, part := range components {
		if i == len(components)-1 {
			// Last part is the actual key
			switch part {
			case "f12":
				parts = append(parts, "F12")
			case "f5":
				parts = append(parts, "F5")
			case "plus":
				parts = append(parts, "plus")
			case "equal":
				parts = append(parts, "equal")
			case "minus":
				parts = append(parts, "minus")
			case "0":
				parts = append(parts, "0")
			case "left", "arrowleft":
				parts = append(parts, "Left")
			case "right", "arrowright":
				parts = append(parts, "Right")
			case "up", "arrowup":
				parts = append(parts, "Up")
			case "down", "arrowdown":
				parts = append(parts, "Down")
			case "tab":
				parts = append(parts, "Tab")
			case "escape":
				parts = append(parts, "Escape")
			case "return", "enter":
				parts = append(parts, "Return")
			default:
				// Single letter keys like l, f, c, w, etc.
				parts = append(parts, part)
			}
		} else {
			// Modifier keys
			switch part {
			case "ctrl":
				parts = append(parts, "<Control>")
			case "shift":
				parts = append(parts, "<Shift>")
			case "alt":
				parts = append(parts, "<Alt>")
			case "super", "meta":
				parts = append(parts, "<Super>")
			}
		}
	}

	return strings.Join(parts, "")
}

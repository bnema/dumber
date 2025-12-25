// Package window provides GTK window implementations.
package window

import (
	"context"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/rs/zerolog"
)

const (
	defaultWidth  = 1280
	defaultHeight = 800
	windowTitle   = "Dumber"
)

// MainWindow represents the main browser window.
type MainWindow struct {
	window         *gtk.ApplicationWindow
	rootBox        *gtk.Box // Vertical: tab bar + content
	tabBar         *component.TabBar
	contentOverlay *gtk.Overlay // Overlay for content + omnibox
	contentArea    *gtk.Box     // Container for workspace content
	currentContent *gtk.Widget  // Track current content for removal on tab switch

	cfg    *config.Config
	logger zerolog.Logger
}

// New creates a new main browser window.
func New(ctx context.Context, app *gtk.Application, cfg *config.Config) (*MainWindow, error) {
	log := logging.FromContext(ctx)

	mw := &MainWindow{
		cfg:    cfg,
		logger: log.With().Str("component", "main-window").Logger(),
	}

	// Create the application window
	mw.window = gtk.NewApplicationWindow(app)
	if mw.window == nil {
		return nil, ErrWindowCreationFailed
	}

	// Configure window
	title := windowTitle
	mw.window.SetTitle(&title)
	mw.window.SetDefaultSize(defaultWidth, defaultHeight)

	// Create root container (vertical box)
	mw.rootBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.rootBox == nil {
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("rootBox")
	}
	mw.rootBox.SetHexpand(true)
	mw.rootBox.SetVexpand(true)
	mw.rootBox.SetVisible(true)

	// Create tab bar
	mw.tabBar = component.NewTabBar()
	if mw.tabBar == nil {
		mw.rootBox.Unref()
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("tabBar")
	}

	// Create content overlay (for omnibox and other overlays)
	mw.contentOverlay = gtk.NewOverlay()
	if mw.contentOverlay == nil {
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("contentOverlay")
	}
	mw.contentOverlay.SetHexpand(true)
	mw.contentOverlay.SetVexpand(true)
	mw.contentOverlay.SetVisible(true)

	// Create content area
	mw.contentArea = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.contentArea == nil {
		mw.contentOverlay.Unref()
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("contentArea")
	}
	mw.contentArea.SetHexpand(true)
	mw.contentArea.SetVexpand(true)
	mw.contentArea.SetVisible(true)
	mw.contentArea.AddCssClass("content-area")

	// Set content area as the main child of the overlay
	mw.contentOverlay.SetChild(&mw.contentArea.Widget)

	// Assemble layout based on tab bar position
	mw.assembleLayout()

	// Set root box as window content
	mw.window.SetChild(&mw.rootBox.Widget)

	return mw, nil
}

// assembleLayout arranges widgets based on configuration.
func (mw *MainWindow) assembleLayout() {
	tabBarPos := "top" // default
	if mw.cfg != nil {
		tabBarPos = mw.cfg.Workspace.TabBarPosition
	}

	if tabBarPos == "bottom" {
		// Content first, then tab bar
		mw.rootBox.Append(&mw.contentOverlay.Widget)
		mw.rootBox.Append(mw.tabBar.Widget())
	} else {
		// Tab bar first, then content (default: top)
		mw.rootBox.Append(mw.tabBar.Widget())
		mw.rootBox.Append(&mw.contentOverlay.Widget)
	}

	mw.logger.Debug().
		Str("tab_bar_position", tabBarPos).
		Msg("window layout assembled")
}

// Show makes the window visible.
func (mw *MainWindow) Show() {
	mw.window.Present()
}

// Close closes the window.
func (mw *MainWindow) Close() {
	mw.window.Close()
}

// TabBar returns the tab bar component.
func (mw *MainWindow) TabBar() *component.TabBar {
	return mw.tabBar
}

// ContentArea returns the content area box.
func (mw *MainWindow) ContentArea() *gtk.Box {
	return mw.contentArea
}

// SetContent sets the content of the content area.
// This removes any existing content first to properly swap workspace views on tab switch.
func (mw *MainWindow) SetContent(widget *gtk.Widget) {
	// Remove existing content first
	if mw.currentContent != nil {
		mw.contentArea.Remove(mw.currentContent)
		mw.currentContent = nil
	}

	// Add new content
	if widget != nil {
		widget.SetVisible(true)
		mw.contentArea.Append(widget)
		mw.currentContent = widget
	}
}

// Window returns the underlying GTK window.
func (mw *MainWindow) Window() *gtk.ApplicationWindow {
	return mw.window
}

// SetTitle updates the window title with proper formatting.
// The title is capped at 255 characters for display.
func (mw *MainWindow) SetTitle(title string) {
	if mw.window == nil {
		return
	}
	// Truncate title to 255 characters max
	const maxTitleLen = 255
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen-3] + "..."
	}
	mw.window.SetTitle(&title)
}

// AddOverlay adds a widget to the content overlay.
// The widget will be displayed on top of the workspace content.
func (mw *MainWindow) AddOverlay(widget *gtk.Widget) {
	if mw.contentOverlay != nil && widget != nil {
		mw.contentOverlay.AddOverlay(widget)
	}
}

// Destroy cleans up window resources.
func (mw *MainWindow) Destroy() {
	if mw.tabBar != nil {
		mw.tabBar.Destroy()
		mw.tabBar = nil
	}
	if mw.contentArea != nil {
		mw.contentArea.Unref()
		mw.contentArea = nil
	}
	if mw.rootBox != nil {
		mw.rootBox.Unref()
		mw.rootBox = nil
	}
	if mw.window != nil {
		mw.window.Destroy()
		mw.window = nil
	}
}

// WindowError represents a window-related error.
type WindowError struct {
	Message string
}

func (e WindowError) Error() string {
	return e.Message
}

// Error constants.
var (
	ErrWindowCreationFailed = WindowError{Message: "failed to create application window"}
)

// ErrWidgetCreationFailed creates an error for widget creation failure.
func ErrWidgetCreationFailed(name string) error {
	return WindowError{Message: "failed to create widget: " + name}
}

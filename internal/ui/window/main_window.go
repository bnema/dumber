// Package window provides GTK window implementations.
package window

import (
	"github.com/bnema/dumber/internal/infrastructure/config"
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
	window      *gtk.ApplicationWindow
	rootBox     *gtk.Box // Vertical: tab bar + content
	tabBar      *component.TabBar
	contentArea *gtk.Box // Container for workspace content

	cfg    *config.Config
	logger *zerolog.Logger
}

// New creates a new main browser window.
func New(app *gtk.Application, cfg *config.Config, logger *zerolog.Logger) (*MainWindow, error) {
	mw := &MainWindow{
		cfg:    cfg,
		logger: logger,
	}

	// Create the application window
	mw.window = gtk.NewApplicationWindow(app)
	if mw.window == nil {
		return nil, ErrWindowCreationFailed
	}

	// Configure window
	mw.window.SetTitle(windowTitle)
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

	// Create content area
	mw.contentArea = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.contentArea == nil {
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("contentArea")
	}
	mw.contentArea.SetHexpand(true)
	mw.contentArea.SetVexpand(true)
	mw.contentArea.SetVisible(true)
	mw.contentArea.AddCssClass("content-area")

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
		mw.rootBox.Append(&mw.contentArea.Widget)
		mw.rootBox.Append(mw.tabBar.Widget())
	} else {
		// Tab bar first, then content (default: top)
		mw.rootBox.Append(mw.tabBar.Widget())
		mw.rootBox.Append(&mw.contentArea.Widget)
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
// This removes any existing children first.
func (mw *MainWindow) SetContent(widget *gtk.Widget) {
	// Remove existing children
	// Note: GTK4 Box doesn't have a simple "clear" method,
	// so we'd need to track children. For now, just append.
	// TODO: Track children for proper content swapping
	if widget != nil {
		widget.SetVisible(true)
		mw.contentArea.Append(widget)
	}
}

// Window returns the underlying GTK window.
func (mw *MainWindow) Window() *gtk.ApplicationWindow {
	return mw.window
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

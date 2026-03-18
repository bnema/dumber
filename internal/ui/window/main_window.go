// Package window provides GTK window implementations.
package window

import (
	"context"

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

	tabBarPosition string // "top" or "bottom"
	logger         zerolog.Logger
}

// New creates a new main browser window.
// tabBarPosition controls whether the tab bar is at the "top" or "bottom" (default: "top").
func New(ctx context.Context, app *gtk.Application, tabBarPosition string) (*MainWindow, error) {
	log := logging.FromContext(ctx)

	mw := &MainWindow{
		tabBarPosition: tabBarPosition,
		logger:         log.With().Str("component", "main-window").Logger(),
	}

	mw.window = gtk.NewApplicationWindow(app)
	if mw.window == nil {
		return nil, ErrWindowCreationFailed
	}

	title := windowTitle
	mw.window.SetTitle(&title)
	mw.window.SetDefaultSize(defaultWidth, defaultHeight)

	mw.rootBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.rootBox == nil {
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("rootBox")
	}
	mw.rootBox.SetHexpand(true)
	mw.rootBox.SetVexpand(true)
	mw.rootBox.SetVisible(true)

	mw.tabBar = component.NewTabBar()
	if mw.tabBar == nil {
		mw.rootBox.Unref()
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("tabBar")
	}

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

	mw.contentOverlay.SetChild(&mw.contentArea.Widget)

	mw.assembleLayout()

	mw.window.SetChild(&mw.rootBox.Widget)

	return mw, nil
}

func (mw *MainWindow) assembleLayout() {
	tabBarPos := "top" // default
	if mw.tabBarPosition != "" {
		tabBarPos = mw.tabBarPosition
	}

	if tabBarPos == "bottom" {
		mw.rootBox.Append(&mw.contentOverlay.Widget)
		mw.rootBox.Append(mw.tabBar.Widget())
	} else {
		mw.rootBox.Append(mw.tabBar.Widget())
		mw.rootBox.Append(&mw.contentOverlay.Widget)
	}

	mw.logger.Debug().
		Str("tab_bar_position", tabBarPos).
		Msg("window layout assembled")
}

// Show presents the window.
func (mw *MainWindow) Show() {
	mw.window.Present()
}

// Close closes the window.
func (mw *MainWindow) Close() {
	mw.window.Close()
}

// TabBar returns the window's tab bar component.
func (mw *MainWindow) TabBar() *component.TabBar {
	return mw.tabBar
}

// ContentArea returns the main content container widget.
func (mw *MainWindow) ContentArea() *gtk.Box {
	return mw.contentArea
}

// SetContent replaces the current content widget with the given widget.
func (mw *MainWindow) SetContent(widget *gtk.Widget) {
	if mw.currentContent != nil {
		mw.contentArea.Remove(mw.currentContent)
		mw.currentContent = nil
	}

	if widget != nil {
		widget.SetVisible(true)
		mw.contentArea.Append(widget)
		mw.currentContent = widget
	}
}

// Window returns the underlying GTK application window.
func (mw *MainWindow) Window() *gtk.ApplicationWindow {
	return mw.window
}

// SetTitle sets the window title.
func (mw *MainWindow) SetTitle(title string) {
	if mw.window == nil {
		return
	}
	const maxTitleLen = 255
	runes := []rune(title)
	if len(runes) > maxTitleLen {
		title = string(runes[:maxTitleLen-3]) + "..."
	}
	mw.window.SetTitle(&title)
}

// ContentOverlay returns the overlay container for the content area.
func (mw *MainWindow) ContentOverlay() *gtk.Overlay {
	return mw.contentOverlay
}

// AddOverlay adds a widget as an overlay above the content area.
func (mw *MainWindow) AddOverlay(widget *gtk.Widget) {
	if mw.contentOverlay != nil && widget != nil {
		mw.contentOverlay.AddOverlay(widget)
	}
}

// Destroy releases all GTK resources held by the window.
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

// WindowError is a window-related error.
type WindowError struct {
	Message string
}

// Error implements the error interface.
func (e WindowError) Error() string {
	return e.Message
}

var (
	// ErrWindowCreationFailed is returned when the GTK application window cannot be created.
	ErrWindowCreationFailed = WindowError{Message: "failed to create application window"}
)

// ErrWidgetCreationFailed returns an error for a failed widget creation.
func ErrWidgetCreationFailed(name string) error {
	return WindowError{Message: "failed to create widget: " + name}
}

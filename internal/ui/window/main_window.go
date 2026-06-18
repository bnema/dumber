// Package window provides GTK window implementations.
package window

import (
	"context"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/rs/zerolog"
)

const (
	defaultWidth  = 1280
	defaultHeight = 800
	windowTitle   = "Dumber"
)

// MainWindow represents the main browser window.
//
// Layout hierarchy:
//
//	window
//	  rootBox (vertical)
//	    contentOverlay
//	      contentAreaHBox (horizontal)
//	        mainContentBox (vertical, expand) ← workspace tab content
//	        sidebarBox (vertical, fixed width) ← optional sidebar
//	      tabBar (overlay, not measured)
type MainWindow struct {
	window         *gtk.ApplicationWindow
	rootBox        *gtk.Box // Vertical: tab bar + content
	tabBar         *component.TabBar
	contentOverlay *gtk.Overlay // Overlay for content + omnibox
	contentAreaBox *gtk.Box     // Horizontal: main content + sidebar
	mainContentBox *gtk.Box     // Vertical container for workspace content
	sidebarBox     *gtk.Box     // Vertical container for sidebar (hidden by default)
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

	if err := mw.initLayoutWidgets(); err != nil {
		return nil, err
	}

	mw.contentAreaBox.Append(&mw.mainContentBox.Widget)
	mw.contentAreaBox.Append(&mw.sidebarBox.Widget)
	mw.contentOverlay.SetChild(&mw.contentAreaBox.Widget)

	mw.assembleLayout()

	mw.window.SetChild(&mw.rootBox.Widget)

	return mw, nil
}

func (mw *MainWindow) initLayoutWidgets() error {
	mw.rootBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.rootBox == nil {
		mw.window.Unref()
		return ErrWidgetCreationFailed("rootBox")
	}
	mw.rootBox.SetHexpand(true)
	mw.rootBox.SetVexpand(true)
	mw.rootBox.SetVisible(true)

	mw.tabBar = component.NewTabBar()
	if mw.tabBar == nil {
		mw.rootBox.Unref()
		mw.window.Unref()
		return ErrWidgetCreationFailed("tabBar")
	}

	if err := mw.initContentWidgets(); err != nil {
		return err
	}
	return nil
}

func (mw *MainWindow) initContentWidgets() error {
	mw.contentOverlay = gtk.NewOverlay()
	if mw.contentOverlay == nil {
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return ErrWidgetCreationFailed("contentOverlay")
	}
	mw.contentOverlay.SetHexpand(true)
	mw.contentOverlay.SetVexpand(true)
	mw.contentOverlay.SetVisible(true)

	// Horizontal box to hold both main content and sidebar side by side.
	mw.contentAreaBox = gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	if mw.contentAreaBox == nil {
		mw.contentOverlay.Unref()
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return ErrWidgetCreationFailed("contentAreaBox")
	}
	mw.contentAreaBox.SetHexpand(true)
	mw.contentAreaBox.SetVexpand(true)
	mw.contentAreaBox.SetVisible(true)

	if err := mw.initContentBoxes(); err != nil {
		return err
	}
	return nil
}

func (mw *MainWindow) initContentBoxes() error {
	// Main content box (vertical) for workspace content.
	mw.mainContentBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.mainContentBox == nil {
		mw.contentAreaBox.Unref()
		mw.contentOverlay.Unref()
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return ErrWidgetCreationFailed("mainContentBox")
	}
	mw.mainContentBox.SetHexpand(true)
	mw.mainContentBox.SetVexpand(true)
	mw.mainContentBox.SetVisible(true)

	// Sidebar box (hidden by default).
	mw.sidebarBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.sidebarBox == nil {
		mw.mainContentBox.Unref()
		mw.contentAreaBox.Unref()
		mw.contentOverlay.Unref()
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return ErrWidgetCreationFailed("sidebarBox")
	}
	mw.sidebarBox.SetHexpand(false)
	mw.sidebarBox.SetVexpand(true)
	mw.sidebarBox.SetVisible(false)

	return nil
}

func (mw *MainWindow) assembleLayout() {
	tabBarPos := "top" // default
	if mw.tabBarPosition != "" {
		tabBarPos = mw.tabBarPosition
	}

	mw.rootBox.Append(&mw.contentOverlay.Widget)

	tabBarWidget := mw.tabBar.Widget()
	tabBarWidget.SetHalign(gtk.AlignFillValue)
	tabBarWidget.SetValign(gtk.AlignStartValue)
	if tabBarPos == "bottom" {
		tabBarWidget.SetValign(gtk.AlignEndValue)
		// Inset is NOT applied unconditionally. See SetTabBarContentInsetVisible
		// which is called when the bottom tab bar is actually visible.
	}
	mw.contentOverlay.AddOverlay(tabBarWidget)
	mw.contentOverlay.SetClipOverlay(tabBarWidget, false)
	// Keep the tab bar out of measurement so showing/hiding it never changes the
	// WebView content allocation; the content area reserves a stable inset while
	// the overlay remains non-measured to avoid WebView allocation changes.
	mw.contentOverlay.SetMeasureOverlay(tabBarWidget, false)

	mw.logger.Debug().
		Str("tab_bar_position", tabBarPos).
		Msg("window layout assembled")
}

// Show presents the window.
func (mw *MainWindow) Show() {
	if mw == nil || mw.window == nil {
		return
	}
	mw.window.Present()
}

// Close closes the window.
func (mw *MainWindow) Close() {
	if mw == nil || mw.window == nil {
		return
	}
	mw.window.Close()
}

// TabBar returns the window's tab bar component.
func (mw *MainWindow) TabBar() *component.TabBar {
	return mw.tabBar
}

// ContentArea returns the main content container widget (the vertical
// box where workspace content is placed).
func (mw *MainWindow) ContentArea() *gtk.Box {
	return mw.mainContentBox
}

// SetContent replaces the current content widget in the main content area
// (the vertical box that holds workspace tab content).
func (mw *MainWindow) SetContent(widget *gtk.Widget) {
	if mw == nil || mw.mainContentBox == nil {
		return
	}
	if mw.currentContent != nil {
		mw.mainContentBox.Remove(mw.currentContent)
		mw.currentContent = nil
	}

	if widget != nil {
		widget.SetVisible(true)
		mw.mainContentBox.Append(widget)
		mw.currentContent = widget
	}
}

// Window returns the underlying GTK application window.
func (mw *MainWindow) Window() *gtk.ApplicationWindow {
	return mw.window
}

// ConnectActiveNotify wires activation state changes for the top-level window.
func (mw *MainWindow) ConnectActiveNotify(callback func(active bool)) uint {
	if mw == nil || mw.window == nil || callback == nil {
		return 0
	}
	cb := func(_ gobject.Object, _ *gobject.ParamSpec) {
		callback(mw.window.IsActive())
	}
	return mw.window.ConnectNotifyWithDetail("is-active", &cb)
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

// SetTabBarContentInsetVisible adds or removes the tab bar inset CSS class
// on the main content area. Avoids duplicate add/remove if already in the desired state.
func (mw *MainWindow) SetTabBarContentInsetVisible(visible bool) {
	if mw.mainContentBox == nil {
		return
	}
	class := mw.tabBarContentInsetClass()
	if visible {
		if !mw.mainContentBox.HasCssClass(class) {
			mw.mainContentBox.AddCssClass(class)
		}
	} else {
		if mw.mainContentBox.HasCssClass(class) {
			mw.mainContentBox.RemoveCssClass(class)
		}
	}
}

// HasTabBarContentInset returns whether the tab bar inset CSS class is currently
// applied to the main content area.
func (mw *MainWindow) HasTabBarContentInset() bool {
	if mw.mainContentBox == nil {
		return false
	}
	return mw.mainContentBox.HasCssClass(mw.tabBarContentInsetClass())
}

func (mw *MainWindow) tabBarContentInsetClass() string {
	if mw.tabBarPosition == "bottom" {
		return "content-area-tabbar-inset-bottom"
	}
	return "content-area-tabbar-inset-top"
}

// AddOverlay adds a widget as an overlay above the content area.
func (mw *MainWindow) AddOverlay(widget *gtk.Widget) {
	if mw.contentOverlay != nil && widget != nil {
		mw.contentOverlay.AddOverlay(widget)
	}
}

// AddNonMeasuringOverlay adds a widget as a visual overlay that does not affect
// content measurement or get clipped to the main child allocation. Use this for
// transient indicators that should float above the WebView.
func (mw *MainWindow) AddNonMeasuringOverlay(widget *gtk.Widget) {
	if mw.contentOverlay == nil || widget == nil {
		return
	}
	mw.contentOverlay.AddOverlay(widget)
	mw.contentOverlay.SetClipOverlay(widget, false)
	mw.contentOverlay.SetMeasureOverlay(widget, false)
}

// Destroy releases all GTK resources held by the window.
func (mw *MainWindow) Destroy() {
	if mw.tabBar != nil {
		mw.tabBar.Destroy()
		mw.tabBar = nil
	}
	if mw.sidebarBox != nil {
		mw.sidebarBox.Unref()
		mw.sidebarBox = nil
	}
	if mw.mainContentBox != nil {
		mw.mainContentBox.Unref()
		mw.mainContentBox = nil
	}
	if mw.contentAreaBox != nil {
		mw.contentAreaBox.Unref()
		mw.contentAreaBox = nil
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

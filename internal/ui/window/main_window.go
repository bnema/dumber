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

	tabBarPosition      string             // "top" or "bottom"
	lastSidebarWidthCfg SidebarWidthConfig // last config passed to SetSidebarWidth (zero-value = unset; test seam)
	logger              zerolog.Logger
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

	// Horizontal box to hold both main content and sidebar side by side.
	mw.contentAreaBox = gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	if mw.contentAreaBox == nil {
		mw.contentOverlay.Unref()
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("contentAreaBox")
	}
	mw.contentAreaBox.SetHexpand(true)
	mw.contentAreaBox.SetVexpand(true)
	mw.contentAreaBox.SetVisible(true)

	// Main content box (vertical) for workspace content.
	mw.mainContentBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.mainContentBox == nil {
		mw.contentAreaBox.Unref()
		mw.contentOverlay.Unref()
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("mainContentBox")
	}
	mw.mainContentBox.SetHexpand(true)
	mw.mainContentBox.SetVexpand(true)
	mw.mainContentBox.SetVisible(true)
	mw.mainContentBox.AddCssClass("content-area")

	// Sidebar box (hidden by default).
	mw.sidebarBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if mw.sidebarBox == nil {
		mw.mainContentBox.Unref()
		mw.contentAreaBox.Unref()
		mw.contentOverlay.Unref()
		mw.tabBar.Destroy()
		mw.rootBox.Unref()
		mw.window.Unref()
		return nil, ErrWidgetCreationFailed("sidebarBox")
	}
	mw.sidebarBox.SetHexpand(false)
	mw.sidebarBox.SetVexpand(true)
	mw.sidebarBox.SetVisible(false)

	mw.contentAreaBox.Append(&mw.mainContentBox.Widget)
	mw.contentAreaBox.Append(&mw.sidebarBox.Widget)
	mw.contentOverlay.SetChild(&mw.contentAreaBox.Widget)

	mw.assembleLayout()

	mw.window.SetChild(&mw.rootBox.Widget)

	return mw, nil
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

// SidebarWidthConfig defines the initial/recommended width range for the sidebar.
type SidebarWidthConfig struct {
	// WidthPx is the preferred sidebar width.
	WidthPx int
	// MinPx is the minimum clamped width (default 280).
	MinPx int
	// MaxPx is the maximum clamping bound (default 380).
	MaxPx int
}

// SidebarDefaultWidth returns a sensible default width configuration:
// preferred 320px, clamped to [280, 380].
func SidebarDefaultWidth() SidebarWidthConfig {
	return SidebarWidthConfig{
		WidthPx: 320,
		MinPx:   280,
		MaxPx:   380,
	}
}

// SidebarBox returns the sidebar container widget for embedding sidebar
// components. The sidebar box is hidden by default.
func (mw *MainWindow) SidebarBox() *gtk.Box {
	return mw.sidebarBox
}

// SetSidebarWidth sets the sidebar box width to widthPx, clamped to the
// config's [MinPx, MaxPx] bounds. Using the zero-value SidebarWidthConfig{}
// sets sensible defaults (320px clamped to [280, 380]).
func (mw *MainWindow) SetSidebarWidth(cfg SidebarWidthConfig) {
	mw.lastSidebarWidthCfg = cfg // record for testability
	if mw.sidebarBox == nil {
		return
	}
	if cfg.MinPx == 0 {
		cfg.MinPx = 280
	}
	if cfg.MaxPx == 0 {
		cfg.MaxPx = 380
	}
	if cfg.WidthPx == 0 {
		cfg.WidthPx = 320
	}
	clamped := cfg.WidthPx
	if clamped < cfg.MinPx {
		clamped = cfg.MinPx
	}
	if clamped > cfg.MaxPx {
		clamped = cfg.MaxPx
	}
	mw.sidebarBox.SetSizeRequest(clamped, -1)
	mw.logger.Debug().Int("sidebar_width", clamped).Msg("sidebar width set")
}

// SetSidebarVisible shows or hides the sidebar pane.
func (mw *MainWindow) SetSidebarVisible(visible bool) {
	if mw.sidebarBox == nil {
		return
	}
	mw.sidebarBox.SetVisible(visible)
	mw.logger.Debug().Bool("sidebar_visible", visible).Msg("sidebar visibility changed")
}

// IsSidebarVisible returns whether the sidebar pane is currently visible.
func (mw *MainWindow) IsSidebarVisible() bool {
	if mw.sidebarBox == nil {
		return false
	}
	return mw.sidebarBox.GetVisible()
}

// LastSidebarWidthCfg returns the last SidebarWidthConfig passed to
// SetSidebarWidth. Returns the zero value if never called (test seam).
func (mw *MainWindow) LastSidebarWidthCfg() SidebarWidthConfig {
	return mw.lastSidebarWidthCfg
}

// SetSidebarWidget replaces the current sidebar content widget.
func (mw *MainWindow) SetSidebarWidget(widget *gtk.Widget) {
	if mw.sidebarBox == nil {
		return
	}
	// Remove existing children
	for {
		child := mw.sidebarBox.GetFirstChild()
		if child == nil {
			break
		}
		mw.sidebarBox.Remove(child)
	}
	if widget != nil {
		widget.SetVisible(true)
		mw.sidebarBox.Append(widget)
	}
}

// SetContent replaces the current content widget in the main content area
// (the vertical box that holds workspace tab content).
func (mw *MainWindow) SetContent(widget *gtk.Widget) {
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

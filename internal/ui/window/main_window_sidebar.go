package window

import (
	"github.com/bnema/puregotk/v4/gtk"
)

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

// SetSidebarWidth sets the sidebar box width to widthPx, clamped to the
// config's [MinPx, MaxPx] bounds. Using the zero-value SidebarWidthConfig{}
// sets sensible defaults (320px clamped to [280, 380]).
func (mw *MainWindow) SetSidebarWidth(cfg SidebarWidthConfig) {
	if mw.sidebarBox == nil {
		return
	}
	defaults := SidebarDefaultWidth()
	if cfg.MinPx == 0 {
		cfg.MinPx = defaults.MinPx
	}
	if cfg.MaxPx == 0 {
		cfg.MaxPx = defaults.MaxPx
	}
	if cfg.WidthPx == 0 {
		cfg.WidthPx = defaults.WidthPx
	}
	if cfg.MinPx > cfg.MaxPx {
		mw.logger.Warn().Int("min_px", cfg.MinPx).Int("max_px", cfg.MaxPx).Msg("invalid sidebar width bounds; swapping")
		cfg.MinPx, cfg.MaxPx = cfg.MaxPx, cfg.MinPx
	}
	clamped := min(max(cfg.WidthPx, cfg.MinPx), cfg.MaxPx)
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

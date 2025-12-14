package theme

import (
	"context"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Manager handles theme state and CSS application.
type Manager struct {
	scheme       string  // "light", "dark", "system"
	prefersDark  bool    // Resolved dark mode preference
	lightPalette Palette // Light theme colors
	darkPalette  Palette // Dark theme colors
	cssProvider  *gtk.CssProvider
}

// NewManager creates a new theme manager from configuration.
func NewManager(ctx context.Context, cfg *config.Config) *Manager {
	log := logging.FromContext(ctx)

	// Determine color scheme preference
	scheme := "system"
	if cfg != nil && cfg.Appearance.ColorScheme != "" {
		scheme = cfg.Appearance.ColorScheme
	}

	// Resolve whether we should use dark mode
	prefersDark := ResolveColorScheme(scheme)

	// Build palettes from config or defaults
	var lightPalette, darkPalette Palette
	if cfg != nil {
		lightPalette = PaletteFromConfig(&cfg.Appearance.LightPalette, false)
		darkPalette = PaletteFromConfig(&cfg.Appearance.DarkPalette, true)
	} else {
		lightPalette = DefaultLightPalette()
		darkPalette = DefaultDarkPalette()
	}

	log.Debug().
		Str("scheme", scheme).
		Bool("prefers_dark", prefersDark).
		Msg("theme manager initialized")

	return &Manager{
		scheme:       scheme,
		prefersDark:  prefersDark,
		lightPalette: lightPalette,
		darkPalette:  darkPalette,
	}
}

// PrefersDark returns true if dark mode is active.
func (m *Manager) PrefersDark() bool {
	return m.prefersDark
}

// GetCurrentPalette returns the active palette based on current scheme.
func (m *Manager) GetCurrentPalette() Palette {
	if m.prefersDark {
		return m.darkPalette
	}
	return m.lightPalette
}

// GetLightPalette returns the light theme palette.
func (m *Manager) GetLightPalette() Palette {
	return m.lightPalette
}

// GetDarkPalette returns the dark theme palette.
func (m *Manager) GetDarkPalette() Palette {
	return m.darkPalette
}

// ApplyToDisplay loads the theme CSS into the display.
func (m *Manager) ApplyToDisplay(ctx context.Context, display *gdk.Display) {
	log := logging.FromContext(ctx)

	if display == nil {
		log.Warn().Msg("cannot apply theme: display is nil")
		return
	}

	// Generate CSS with current palette
	palette := m.GetCurrentPalette()
	css := GenerateCSS(palette)

	// Create CSS provider if needed
	if m.cssProvider == nil {
		m.cssProvider = gtk.NewCssProvider()
	}

	if m.cssProvider == nil {
		log.Error().Msg("failed to create CSS provider")
		return
	}

	// Load CSS
	m.cssProvider.LoadFromString(css)
	gtk.StyleContextAddProviderForDisplay(
		display,
		m.cssProvider,
		uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION),
	)

	log.Debug().
		Bool("dark_mode", m.prefersDark).
		Msg("theme CSS applied to display")
}

// SetColorScheme changes the active color scheme at runtime.
func (m *Manager) SetColorScheme(ctx context.Context, scheme string, display *gdk.Display) {
	log := logging.FromContext(ctx)

	m.scheme = scheme
	m.prefersDark = ResolveColorScheme(scheme)

	log.Info().
		Str("scheme", scheme).
		Bool("prefers_dark", m.prefersDark).
		Msg("color scheme changed")

	// Re-apply CSS if display is available
	if display != nil {
		m.ApplyToDisplay(ctx, display)
	}
}

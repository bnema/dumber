package theme

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Manager handles theme state and CSS application.
type Manager struct {
	scheme        string  // "light", "dark", "system"
	prefersDark   bool    // Resolved dark mode preference
	lightPalette  Palette // Light theme colors
	darkPalette   Palette // Dark theme colors
	uiScale       float64 // UI scaling factor (1.0 = 100%)
	fonts         FontConfig
	modeColors    ModeColors // Modal mode indicator colors
	cssProvider   *gtk.CssProvider
	colorResolver port.ColorSchemeResolver // Resolver for dynamic detection
}

// ManagerOption is a functional option for configuring Manager.
type ManagerOption func(*Manager)

// WithResolver configures the Manager to use a ColorSchemeResolver.
func WithResolver(resolver port.ColorSchemeResolver) ManagerOption {
	return func(m *Manager) {
		m.colorResolver = resolver
	}
}

// NewManager creates a new theme manager from configuration.
// The ColorSchemeResolver is required for proper color scheme detection.
func NewManager(ctx context.Context, cfg *config.Config, resolver port.ColorSchemeResolver) *Manager {
	log := logging.FromContext(ctx)

	// Determine color scheme preference
	scheme := "system"
	if cfg != nil && cfg.Appearance.ColorScheme != "" {
		scheme = cfg.Appearance.ColorScheme
	}

	// Resolve whether we should use dark mode via resolver
	pref := resolver.Resolve()
	prefersDark := pref.PrefersDark

	// Build palettes from config or defaults
	var lightPalette, darkPalette Palette
	if cfg != nil {
		lightPalette = PaletteFromConfig(&cfg.Appearance.LightPalette, false)
		darkPalette = PaletteFromConfig(&cfg.Appearance.DarkPalette, true)
	} else {
		lightPalette = DefaultLightPalette()
		darkPalette = DefaultDarkPalette()
	}

	// Get UI scale factor (default to 1.0 if not set)
	uiScale := 1.0
	if cfg != nil && cfg.DefaultUIScale > 0 {
		uiScale = cfg.DefaultUIScale
	}

	fonts := DefaultFontConfig()
	if cfg != nil {
		fonts.SansFont = Coalesce(cfg.Appearance.SansFont, fonts.SansFont)
		fonts.MonospaceFont = Coalesce(cfg.Appearance.MonospaceFont, fonts.MonospaceFont)
	}

	// Build mode colors from config
	modeColors := modeColorsFromConfig(cfg)

	log.Debug().
		Str("scheme", scheme).
		Bool("prefers_dark", prefersDark).
		Float64("ui_scale", uiScale).
		Str("sans_font", fonts.SansFont).
		Str("monospace_font", fonts.MonospaceFont).
		Msg("theme manager initialized")

	return &Manager{
		scheme:        scheme,
		prefersDark:   prefersDark,
		lightPalette:  lightPalette,
		darkPalette:   darkPalette,
		uiScale:       uiScale,
		fonts:         fonts,
		modeColors:    modeColors,
		colorResolver: resolver,
	}
}

// modeColorsFromConfig extracts mode colors from config, using defaults for missing values.
func modeColorsFromConfig(cfg *config.Config) ModeColors {
	defaults := DefaultModeColors()
	if cfg == nil {
		return defaults
	}
	styling := &cfg.Workspace.Styling
	return ModeColors{
		PaneMode:    Coalesce(styling.PaneModeColor, defaults.PaneMode),
		TabMode:     Coalesce(styling.TabModeColor, defaults.TabMode),
		SessionMode: Coalesce(styling.SessionModeColor, defaults.SessionMode),
		ResizeMode:  Coalesce(styling.ResizeModeColor, defaults.ResizeMode),
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

// GetBackgroundRGBA returns the current theme's background color as RGBA float32 values.
// Used to set WebView background color to eliminate white flash during loading.
func (m *Manager) GetBackgroundRGBA() (r, g, b, a float32) {
	palette := m.GetCurrentPalette()
	return HexToRGBA(palette.Background)
}

// GetLightPalette returns the light theme palette.
func (m *Manager) GetLightPalette() Palette {
	return m.lightPalette
}

// GetDarkPalette returns the dark theme palette.
func (m *Manager) GetDarkPalette() Palette {
	return m.darkPalette
}

// GetModeColors returns the modal mode indicator colors.
func (m *Manager) GetModeColors() ModeColors {
	return m.modeColors
}

// GetWebUIThemeCSS returns CSS text that defines both light and dark variables.
// WebUI uses `:root` for light and `.dark` overrides for dark.
func (m *Manager) GetWebUIThemeCSS() string {
	lightVars := m.lightPalette.ToWebCSSVars()
	darkVars := m.darkPalette.ToWebCSSVars()
	return fmt.Sprintf(":root{\n%s}\n\n.dark{\n%s}\n", lightVars, darkVars)
}

// ApplyToDisplay loads the theme CSS into the display.
func (m *Manager) ApplyToDisplay(ctx context.Context, display *gdk.Display) {
	log := logging.FromContext(ctx)

	if display == nil {
		log.Warn().Msg("cannot apply theme: display is nil")
		return
	}

	// Generate CSS with current palette, UI scale, fonts, and mode colors
	palette := m.GetCurrentPalette()
	css := GenerateCSSFull(palette, m.uiScale, m.fonts, m.modeColors)

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
	pref := m.colorResolver.Refresh()
	m.prefersDark = pref.PrefersDark

	log.Info().
		Str("scheme", scheme).
		Bool("prefers_dark", m.prefersDark).
		Str("source", pref.Source).
		Msg("color scheme changed")

	// Re-apply CSS if display is available
	if display != nil {
		m.ApplyToDisplay(ctx, display)
	}
}

// UpdateFromConfig updates the theme manager state from a new config.
func (m *Manager) UpdateFromConfig(ctx context.Context, cfg *config.Config, display *gdk.Display) {
	log := logging.FromContext(ctx)

	if cfg == nil {
		return
	}

	// Update scheme and resolve prefersDark via resolver
	scheme := "system"
	if cfg.Appearance.ColorScheme != "" {
		scheme = cfg.Appearance.ColorScheme
	}
	m.scheme = scheme
	pref := m.colorResolver.Refresh()
	m.prefersDark = pref.PrefersDark

	// Update palettes
	m.lightPalette = PaletteFromConfig(&cfg.Appearance.LightPalette, false)
	m.darkPalette = PaletteFromConfig(&cfg.Appearance.DarkPalette, true)

	// Update UI scale
	if cfg.DefaultUIScale > 0 {
		m.uiScale = cfg.DefaultUIScale
	}

	defaults := DefaultFontConfig()
	m.fonts = FontConfig{
		SansFont:      Coalesce(cfg.Appearance.SansFont, defaults.SansFont),
		MonospaceFont: Coalesce(cfg.Appearance.MonospaceFont, defaults.MonospaceFont),
	}

	// Update mode colors
	m.modeColors = modeColorsFromConfig(cfg)

	log.Info().
		Str("scheme", m.scheme).
		Bool("prefers_dark", m.prefersDark).
		Float64("ui_scale", m.uiScale).
		Str("sans_font", m.fonts.SansFont).
		Str("monospace_font", m.fonts.MonospaceFont).
		Msg("theme manager updated from config")

	// Re-apply CSS if display is available
	if display != nil {
		m.ApplyToDisplay(ctx, display)
	}
}

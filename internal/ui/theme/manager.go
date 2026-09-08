package theme

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
)

// Manager handles theme state and CSS application.
// Theme precedence and fallback are resolved before data reaches this adapter.
type Manager struct {
	prefersDark          bool    // Resolved dark mode preference
	lightPalette         Palette // Light theme colors
	darkPalette          Palette // Dark theme colors
	uiScale              float64 // UI scaling factor (1.0 = 100%)
	fonts                FontConfig
	gtkFont              string
	modeColors           ModeColors // Modal mode indicator colors
	transitionDurationMs int
	cssProvider          *gtk.CssProvider
	// appliedFonts records the GTK font name last set per display, keyed by
	// native display address. GtkSettings are per display, so the font must
	// be applied on every display even when the theme is unchanged. Keys
	// are plain addresses: no display reference is retained, so destroyed
	// displays cannot leak through the manager.
	appliedFonts map[uintptr]string
	// appliedCSS is the effective CSS identity last loaded into cssProvider.
	// appliedDisplayPtr is the native display address that provider was last
	// added for. Both are plain values: no display reference is retained, so
	// destroyed displays cannot leak through the manager. Native display
	// addresses are unique among live displays; address reuse after a display
	// is destroyed is pathological in single-display processes.
	appliedCSS        string
	appliedDisplayPtr uintptr
}

// NewManager creates a new theme manager from an already-resolved theme.
func NewManager(ctx context.Context, resolved entity.ResolvedTheme) *Manager {
	log := logging.FromContext(ctx)

	m := &Manager{transitionDurationMs: defaultTransitionDurationMs}
	m.applyResolvedTheme(resolved)

	log.Debug().
		Bool("prefers_dark", m.prefersDark).
		Float64("ui_scale", m.uiScale).
		Str("sans_font", m.fonts.SansFont).
		Str("monospace_font", m.fonts.MonospaceFont).
		Str("gtk_font", m.gtkFont).
		Msg("theme manager initialized")

	return m
}

func (m *Manager) applyResolvedTheme(resolved entity.ResolvedTheme) {
	m.prefersDark = resolved.PrefersDark
	m.lightPalette = PaletteFromEntity(resolved.LightPalette, false)
	m.darkPalette = PaletteFromEntity(resolved.DarkPalette, true)
	m.uiScale = resolved.UIScale
	if m.uiScale <= 0 {
		m.uiScale = 1.0
	}
	m.fonts = FontConfigFromEntity(resolved.Fonts)
	if m.fonts.GtkFont == "" {
		m.fonts.GtkFont = DefaultGTKFont()
	}
	m.gtkFont = m.fonts.GtkFont
	m.modeColors = ModeColorsFromEntity(resolved.ModeColors)
}

// SetTransitionDuration sets the CSS transition duration used for theme-driven
// animations such as Vim mode pulse timing.
func (m *Manager) SetTransitionDuration(ms int) {
	if ms < 0 {
		ms = defaultTransitionDurationMs
	}
	m.transitionDurationMs = ms
}

// PrefersDark returns true if dark mode is active.
func (m *Manager) PrefersDark() bool {
	return m.prefersDark
}

// GetCurrentPalette returns the active palette based on current preference.
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

const gtkDefaultFontPointSize = 11

func DefaultGTKFont() string { return "Adwaita Sans" }

func formatGTKFontName(family string, uiScale float64) string {
	family = strings.TrimSpace(family)
	if family == "" {
		family = DefaultGTKFont()
	}
	if uiScale <= 0 {
		uiScale = 1.0
	}
	size := int(math.Round(gtkDefaultFontPointSize * uiScale))
	if size < 1 {
		size = gtkDefaultFontPointSize
	}
	return fmt.Sprintf("%s %d", family, size)
}

func shouldApplyGTKFontName(current, next string) bool {
	return next != "" && next != current
}

// lastAppliedFontFor returns the GTK font name recorded for a display, or
// "" when the manager has not applied a font there yet.
func (m *Manager) lastAppliedFontFor(ptr uintptr) string {
	if m == nil || m.appliedFonts == nil {
		return ""
	}
	return m.appliedFonts[ptr]
}

// recordAppliedFontFor stores the GTK font name set on a display. The map
// is created on first use so zero-value Managers stay usable.
func (m *Manager) recordAppliedFontFor(ptr uintptr, font string) {
	if m == nil {
		return
	}
	if m.appliedFonts == nil {
		m.appliedFonts = make(map[uintptr]string)
	}
	m.appliedFonts[ptr] = font
}

// shouldSkipThemeReapply reports whether a theme application can be skipped
// entirely: the provider exists, the effective CSS and font are unchanged,
// and the target is the display the provider was last added for.
func shouldSkipThemeReapply(hasProvider bool, appliedPtr, ptr uintptr, appliedCSS, css, appliedFont, font string) bool {
	return hasProvider && ptr == appliedPtr && css == appliedCSS && font != "" && font == appliedFont
}

// ApplyToDisplay loads the theme CSS into the display, skipping the reload
// and re-add when the same provider already carries the effective CSS on the
// same display. A different display always gets the provider added; the CSS
// is only re-parsed when its identity changed.
func (m *Manager) ApplyToDisplay(ctx context.Context, display *gdk.Display) {
	log := logging.FromContext(ctx)

	if display == nil {
		log.Warn().Msg("cannot apply theme: display is nil")
		return
	}

	// Generate CSS with current palette, UI scale, fonts, mode colors, and the
	// configured transition duration.
	palette := m.GetCurrentPalette()
	css := GenerateCSSFullWithTiming(palette, m.uiScale, m.fonts, m.modeColors, m.transitionDurationMs)
	fontName := formatGTKFontName(m.gtkFont, m.uiScale)
	displayFont := m.lastAppliedFontFor(display.Ptr)

	if shouldSkipThemeReapply(m.cssProvider != nil, m.appliedDisplayPtr, display.Ptr, m.appliedCSS, css, displayFont, fontName) {
		log.Debug().
			Bool("dark_mode", m.prefersDark).
			Msg("theme CSS unchanged for display, skipping reapply")
		return
	}

	settings := gtk.SettingsGetForDisplay(display)
	if settings == nil {
		log.Warn().Msg("cannot apply theme font scaling: settings unavailable")
	} else if shouldApplyGTKFontName(displayFont, fontName) {
		settings.SetPropertyGtkFontName(fontName)
		m.recordAppliedFontFor(display.Ptr, fontName)
		settings.Unref()
	} else {
		settings.Unref()
	}

	// Create CSS provider if needed
	providerFresh := false
	if m.cssProvider == nil {
		m.cssProvider = gtk.NewCssProvider()
		providerFresh = true
	}

	if m.cssProvider == nil {
		log.Error().Msg("failed to create CSS provider")
		return
	}

	// Load CSS only when its identity changed; the shared provider already
	// carries the previous CSS for a display change with an unchanged theme.
	// A freshly created provider always loads, even if the generated CSS is
	// empty, so the skip identity can never mask a missing load.
	if providerFresh || css != m.appliedCSS {
		m.cssProvider.LoadFromString(css)
	}
	gtk.StyleContextAddProviderForDisplay(
		display,
		m.cssProvider,
		uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION),
	)
	m.appliedCSS = css
	m.appliedDisplayPtr = display.Ptr

	log.Debug().
		Bool("dark_mode", m.prefersDark).
		Msg("theme CSS applied to display")
}

// UpdateFromResolved updates the theme manager state from a resolved theme.
func (m *Manager) UpdateFromResolved(ctx context.Context, resolved entity.ResolvedTheme, display *gdk.Display) {
	log := logging.FromContext(ctx)

	m.applyResolvedTheme(resolved)

	log.Info().
		Bool("prefers_dark", m.prefersDark).
		Float64("ui_scale", m.uiScale).
		Str("sans_font", m.fonts.SansFont).
		Str("monospace_font", m.fonts.MonospaceFont).
		Str("gtk_font", m.gtkFont).
		Msg("theme manager updated from resolved theme")

	// Re-apply CSS if display is available
	if display != nil {
		m.ApplyToDisplay(ctx, display)
	}
}

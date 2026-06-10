package usecase

import (
	"context"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

// ResolveThemeInput contains the inputs needed to resolve the active theme.
// It accepts config-level overrides and delegates to external sources via port.
type ResolveThemeInput struct {
	// ColorScheme is the user-configured color scheme preference.
	// Values: "default", "prefer-dark", "prefer-light", "dark", "light".
	// Phase 2 expects callers to resolve this into PrefersDark before invoking the usecase.
	ColorScheme string

	// PrefersDark is the detected system color scheme preference.
	PrefersDark bool

	// ColorSchemeSource identifies which detector provided the color scheme preference.
	// Examples: "config", "adwaita", "gsettings", "env", "fallback".
	ColorSchemeSource string

	// LightPalette from user config (may be empty/nil).
	LightPalette *entity.ColorPalette

	// DarkPalette from user config (may be empty/nil).
	DarkPalette *entity.ColorPalette

	// Fonts from user config (may be nil for defaults).
	Fonts *entity.ThemeFonts

	// UIScale from GTK/display settings.
	UIScale float64

	// ModeColors from user config (may be nil for defaults).
	ModeColors *entity.ThemeModeColors
}

// ResolveThemeOutput is the result of theme resolution.
type ResolveThemeOutput struct {
	// Theme is the resolved theme.
	Theme entity.ResolvedTheme
}

// ResolveThemeUseCase resolves the active theme by merging config defaults,
// user configuration, and an optional external theme source.
//
// Resolution precedence (locked contract):
//   - External disabled -> config/default palettes
//   - Valid external -> external palettes override palette colors only
//   - Malformed/missing external at startup -> config/default palettes + warning
//   - Malformed after valid external -> keep last-good external + warning/source metadata
//   - Disabling external after valid -> clear last-good and use config/defaults
//
// CSS generation is NOT part of this usecase.
// This usecase must NOT import GTK, WebKit, CEF, lipgloss, fsnotify, Viper, or infrastructure config.
type ResolveThemeUseCase struct {
	external port.ExternalThemeSource
	mu       sync.Mutex

	// lastGoodExternal holds the most recent fully-resolved valid external palette.
	// Used for the "malformed after valid" -> keep last-good contract.
	lastGoodExternal *entity.ExternalTheme

	// lastExternalIdentity tracks the configured external source identity that produced lastGoodExternal.
	// If the enabled provider/format/path changes, last-good is invalidated before resolving.
	lastExternalIdentity string
}

// NewResolveThemeUseCase creates a new ResolveThemeUseCase.
// external may be nil if no external theme source is available.
func NewResolveThemeUseCase(external port.ExternalThemeSource) *ResolveThemeUseCase {
	return &ResolveThemeUseCase{external: external}
}

// ResolveThemeInputFromConfig maps domain config shapes plus a detected color-scheme
// preference into the application usecase input. It performs no precedence/fallback
// resolution; Execute/Refresh owns that business logic.
func ResolveThemeInputFromConfig(
	appearance *entity.AppearanceConfig,
	uiScale float64,
	styling *entity.WorkspaceStylingConfig,
	preference port.ColorSchemePreference,
) ResolveThemeInput {
	input := ResolveThemeInput{
		PrefersDark:       preference.PrefersDark,
		ColorSchemeSource: preference.Source,
		UIScale:           uiScale,
	}

	if appearance != nil {
		lightPalette := appearance.LightPalette
		darkPalette := appearance.DarkPalette
		fonts := entity.ThemeFonts{
			SansFont:      appearance.SansFont,
			SerifFont:     appearance.SerifFont,
			MonospaceFont: appearance.MonospaceFont,
			GtkFont:       appearance.GtkFont,
			DefaultSize:   appearance.DefaultFontSize,
		}
		input.ColorScheme = appearance.ColorScheme
		input.LightPalette = &lightPalette
		input.DarkPalette = &darkPalette
		input.Fonts = &fonts
	}

	if styling != nil {
		modeColors := entity.ThemeModeColors{
			PaneMode:    styling.PaneModeColor,
			TabMode:     styling.TabModeColor,
			SessionMode: styling.SessionModeColor,
			ResizeMode:  styling.ResizeModeColor,
		}
		input.ModeColors = &modeColors
	}

	return input
}

// ColorSchemePreferenceFromConfig converts explicit config color-scheme values into
// a resolver preference, falling back when config asks for the default/system value.
func ColorSchemePreferenceFromConfig(colorScheme string, fallback port.ColorSchemePreference) port.ColorSchemePreference {
	switch strings.ToLower(strings.TrimSpace(colorScheme)) {
	case "prefer-dark", "dark":
		return port.ColorSchemePreference{PrefersDark: true, Source: "config"}
	case "prefer-light", "light":
		return port.ColorSchemePreference{PrefersDark: false, Source: "config"}
	default:
		return fallback
	}
}

// Refresh resolves the active theme for config reloads and future external-theme reloads.
func (uc *ResolveThemeUseCase) Refresh(ctx context.Context, input ResolveThemeInput) (ResolveThemeOutput, error) {
	return uc.Execute(ctx, input)
}

// Execute resolves the active theme.
func (uc *ResolveThemeUseCase) Execute(ctx context.Context, input ResolveThemeInput) (ResolveThemeOutput, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	base := uc.resolveBase(input)
	lightPalette := base.lightPalette
	darkPalette := base.darkPalette
	warnings := base.warnings
	themeSource := base.themeSource

	if uc.external != nil && uc.external.IsEnabled() {
		uc.syncExternalIdentity(externalThemeIdentity(uc.external))
		ext, err := uc.external.Get(ctx)
		if err != nil {
			lightPalette, darkPalette, themeSource, warnings = uc.handleExternalError(err, lightPalette, darkPalette, themeSource, warnings)
		} else if ext != nil {
			lightPalette, darkPalette, themeSource, warnings = uc.applyExternalTheme(ext, lightPalette, darkPalette, themeSource, warnings)
		} else {
			lightPalette, darkPalette, themeSource, warnings = uc.handleMissingExternalTheme(lightPalette, darkPalette, themeSource, warnings)
		}
	} else {
		uc.clearLastGoodExternal()
	}

	return ResolveThemeOutput{
		Theme: buildResolvedTheme(input, lightPalette, darkPalette, base.fonts, base.modeColors, base.uiScale, themeSource, warnings),
	}, nil
}

type resolvedThemeBase struct {
	lightPalette entity.ColorPalette
	darkPalette  entity.ColorPalette
	fonts        entity.ThemeFonts
	modeColors   entity.ThemeModeColors
	uiScale      float64
	themeSource  entity.ThemeSourceMetadata
	warnings     []entity.ThemeWarning
}

func (*ResolveThemeUseCase) resolveBase(input ResolveThemeInput) resolvedThemeBase {
	lightDefault := entity.DefaultLightPalette()
	darkDefault := entity.DefaultDarkPalette()
	lightPalette := entity.MergePalette(input.LightPalette, &lightDefault)
	darkPalette := entity.MergePalette(input.DarkPalette, &darkDefault)

	fonts := entity.MergeFonts(input.Fonts, entity.DefaultThemeFonts())
	modeColors := entity.MergeModeColors(input.ModeColors, entity.DefaultThemeModeColors())
	uiScale := input.UIScale
	if uiScale <= 0 {
		uiScale = 1.0
	}

	themeSourceKind := entity.ThemeSourceConfig
	if input.LightPalette == nil && input.DarkPalette == nil {
		themeSourceKind = entity.ThemeSourceDefault
	}

	return resolvedThemeBase{
		lightPalette: lightPalette,
		darkPalette:  darkPalette,
		fonts:        fonts,
		modeColors:   modeColors,
		uiScale:      uiScale,
		themeSource: entity.ThemeSourceMetadata{
			Kind: themeSourceKind,
		},
	}
}

func (uc *ResolveThemeUseCase) clearLastGoodExternal() {
	uc.lastGoodExternal = nil
	uc.lastExternalIdentity = ""
}

func (uc *ResolveThemeUseCase) syncExternalIdentity(identity string) {
	if identity == "" {
		return
	}
	if uc.lastExternalIdentity != "" && uc.lastExternalIdentity != identity {
		uc.lastGoodExternal = nil
	}
	uc.lastExternalIdentity = identity
}

func externalThemeIdentity(source port.ExternalThemeSource) string {
	identityProvider, ok := source.(port.ExternalThemeIdentityProvider)
	if !ok {
		return ""
	}
	return identityProvider.ExternalThemeIdentity()
}

func (uc *ResolveThemeUseCase) handleMissingExternalTheme(
	lightPalette, darkPalette entity.ColorPalette,
	themeSource entity.ThemeSourceMetadata,
	warnings []entity.ThemeWarning,
) (entity.ColorPalette, entity.ColorPalette, entity.ThemeSourceMetadata, []entity.ThemeWarning) {
	if uc.lastGoodExternal != nil {
		warnings = append(warnings, entity.ThemeWarning{
			Field:   "external",
			Message: "external theme is missing, using last-good",
		})
		return uc.lastGoodPalettes(themeSource, warnings)
	}

	warnings = append(warnings, entity.ThemeWarning{
		Field:   "external",
		Message: "external theme is missing, using config/defaults",
	})
	return lightPalette, darkPalette, themeSource, warnings
}

func (uc *ResolveThemeUseCase) handleExternalError(
	err error,
	lightPalette, darkPalette entity.ColorPalette,
	themeSource entity.ThemeSourceMetadata,
	warnings []entity.ThemeWarning,
) (entity.ColorPalette, entity.ColorPalette, entity.ThemeSourceMetadata, []entity.ThemeWarning) {
	if uc.lastGoodExternal != nil {
		warnings = append(warnings, entity.ThemeWarning{
			Field:   "external",
			Message: "failed to fetch external theme: " + err.Error() + ", using last-good",
		})
		return uc.lastGoodPalettes(themeSource, warnings)
	}

	warnings = append(warnings, entity.ThemeWarning{
		Field:   "external",
		Message: "failed to fetch external theme: " + err.Error(),
	})
	return lightPalette, darkPalette, themeSource, warnings
}

func (uc *ResolveThemeUseCase) applyExternalTheme(
	ext *entity.ExternalTheme,
	lightPalette, darkPalette entity.ColorPalette,
	themeSource entity.ThemeSourceMetadata,
	warnings []entity.ThemeWarning,
) (entity.ColorPalette, entity.ColorPalette, entity.ThemeSourceMetadata, []entity.ThemeWarning) {
	validationWarnings := externalThemeWarnings(ext)
	if len(validationWarnings) > 0 {
		if uc.lastGoodExternal != nil {
			warnings = append(warnings, entity.ThemeWarning{
				Field:   "external",
				Message: "malformed external theme update, keeping last-good",
			})
			warnings = append(warnings, validationWarnings...)
			return uc.lastGoodPalettes(themeSource, warnings)
		}

		warnings = append(warnings, entity.ThemeWarning{
			Field:   "external",
			Message: "malformed external theme, using config/defaults",
		})
		warnings = append(warnings, validationWarnings...)
		return lightPalette, darkPalette, themeSource, warnings
	}

	// External palettes are partial overrides. Empty fields fall back through a
	// repaired config/default palette so one invalid config value cannot discard
	// otherwise valid external overrides or poison the last-good cache.
	lightPalette, warnings = paletteWithValidFallbackFields(lightPalette, entity.DefaultLightPalette(), "light_palette", warnings)
	darkPalette, warnings = paletteWithValidFallbackFields(darkPalette, entity.DefaultDarkPalette(), "dark_palette", warnings)
	resolvedLight := entity.MergePalette(ext.LightPalette, &lightPalette)
	resolvedDark := entity.MergePalette(ext.DarkPalette, &darkPalette)

	provider := ext.Provider
	lightCopy := resolvedLight
	darkCopy := resolvedDark
	uc.lastGoodExternal = &entity.ExternalTheme{
		Name:         ext.Name,
		Provider:     provider,
		LightPalette: &lightCopy,
		DarkPalette:  &darkCopy,
	}

	return resolvedLight, resolvedDark, entity.ThemeSourceMetadata{
		Kind:     entity.ThemeSourceExternal,
		Provider: provider,
		LastGood: false,
	}, warnings
}

func externalThemeWarnings(ext *entity.ExternalTheme) []entity.ThemeWarning {
	if ext == nil {
		return []entity.ThemeWarning{{Field: "external", Message: "theme is nil"}}
	}
	var warnings []entity.ThemeWarning
	warnings = append(warnings, entity.ValidatePaletteOverrideHex(ext.LightPalette, "external.light_palette")...)
	warnings = append(warnings, entity.ValidatePaletteOverrideHex(ext.DarkPalette, "external.dark_palette")...)
	return warnings
}

func paletteWithValidFallbackFields(
	palette entity.ColorPalette,
	fallback entity.ColorPalette,
	fieldPrefix string,
	warnings []entity.ThemeWarning,
) (entity.ColorPalette, []entity.ThemeWarning) {
	if entity.IsPaletteValid(&palette) {
		return palette, warnings
	}
	warnings = append(warnings, entity.ThemeWarning{
		Field:   fieldPrefix,
		Message: "resolved config palette contained invalid colors, using built-in defaults for invalid fields",
	})
	if !entity.IsValidHex(palette.Background) {
		palette.Background = fallback.Background
	}
	if !entity.IsValidHex(palette.Surface) {
		palette.Surface = fallback.Surface
	}
	if !entity.IsValidHex(palette.SurfaceVariant) {
		palette.SurfaceVariant = fallback.SurfaceVariant
	}
	if !entity.IsValidHex(palette.Text) {
		palette.Text = fallback.Text
	}
	if !entity.IsValidHex(palette.Muted) {
		palette.Muted = fallback.Muted
	}
	if !entity.IsValidHex(palette.Accent) {
		palette.Accent = fallback.Accent
	}
	if !entity.IsValidHex(palette.Border) {
		palette.Border = fallback.Border
	}
	return palette, warnings
}

func (uc *ResolveThemeUseCase) lastGoodPalettes(
	fallback entity.ThemeSourceMetadata,
	warnings []entity.ThemeWarning,
) (entity.ColorPalette, entity.ColorPalette, entity.ThemeSourceMetadata, []entity.ThemeWarning) {
	if uc.lastGoodExternal == nil || uc.lastGoodExternal.LightPalette == nil || uc.lastGoodExternal.DarkPalette == nil {
		return entity.DefaultLightPalette(), entity.DefaultDarkPalette(), fallback, warnings
	}

	return *uc.lastGoodExternal.LightPalette, *uc.lastGoodExternal.DarkPalette, entity.ThemeSourceMetadata{
		Kind:     entity.ThemeSourceExternal,
		Provider: uc.lastGoodExternal.Provider,
		LastGood: true,
	}, warnings
}

func buildResolvedTheme(
	input ResolveThemeInput,
	lightPalette, darkPalette entity.ColorPalette,
	fonts entity.ThemeFonts,
	modeColors entity.ThemeModeColors,
	uiScale float64,
	themeSource entity.ThemeSourceMetadata,
	warnings []entity.ThemeWarning,
) entity.ResolvedTheme {
	if !entity.IsPaletteValid(&lightPalette) {
		warnings = append(warnings, entity.ThemeWarning{
			Field:   "light_palette",
			Message: "resolved light palette was invalid, using built-in default",
		})
		lightPalette = entity.DefaultLightPalette()
	}
	if !entity.IsPaletteValid(&darkPalette) {
		warnings = append(warnings, entity.ThemeWarning{
			Field:   "dark_palette",
			Message: "resolved dark palette was invalid, using built-in default",
		})
		darkPalette = entity.DefaultDarkPalette()
	}
	if !entity.IsModeColorsValid(&modeColors) {
		warnings = append(warnings, entity.ThemeWarning{
			Field:   "mode_colors",
			Message: "resolved mode colors were invalid, using built-in defaults",
		})
		modeColors = entity.DefaultThemeModeColors()
	}

	activePalette := lightPalette
	if input.PrefersDark {
		activePalette = darkPalette
	}

	return entity.ResolvedTheme{
		LightPalette:      lightPalette,
		DarkPalette:       darkPalette,
		ActivePalette:     activePalette,
		PrefersDark:       input.PrefersDark,
		ColorSchemeSource: input.ColorSchemeSource,
		ThemeSource:       themeSource,
		Fonts:             fonts,
		UIScale:           uiScale,
		ModeColors:        modeColors,
		Warnings:          warnings,
	}
}

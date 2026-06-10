package entity

import "regexp"

// ThemeSourceKind describes where the resolved palette colors originated.
type ThemeSourceKind string

const (
	// ThemeSourceDefault indicates palette colors come from built-in defaults.
	ThemeSourceDefault ThemeSourceKind = "default"
	// ThemeSourceConfig indicates palette colors come from user configuration.
	ThemeSourceConfig ThemeSourceKind = "config"
	// ThemeSourceExternal indicates palette colors come from an external theme source.
	ThemeSourceExternal ThemeSourceKind = "external"
)

// ThemeSourceMetadata describes the palette source used for a resolved theme.
type ThemeSourceMetadata struct {
	Kind     ThemeSourceKind
	Provider string
	LastGood bool
}

// ThemeWarning represents a non-fatal issue encountered during theme resolution.
type ThemeWarning struct {
	Field   string
	Message string
}

// ThemeFonts holds the resolved font families and default size.
type ThemeFonts struct {
	SansFont      string
	SerifFont     string
	MonospaceFont string
	GtkFont       string
	DefaultSize   int
}

// ThemeModeColors holds mode indicator colors.
// All values must be CSS-safe hex colors (e.g., "#4A90E2").
type ThemeModeColors struct {
	PaneMode    string
	TabMode     string
	SessionMode string
	ResizeMode  string
}

// ResolvedThemePalette is a CSS-safe hex color palette.
// Alias for ColorPalette with the contract that all color strings
// are valid CSS hex colors (validated during resolution).
type ResolvedThemePalette = ColorPalette

// ExternalTheme represents palette data from an external source (e.g., Noctalia).
// External themes override palette colors only; fonts, UI scale, and mode colors stay in Dumber config.
// It must not depend on GTK, WebKit, CEF, lipgloss, fsnotify, Viper, or infrastructure config.
type ExternalTheme struct {
	Name     string
	Provider string

	// LightPalette and DarkPalette may contain partial overrides.
	// Nil palette pointers are malformed; empty fields mean "use the resolved config/default field".
	LightPalette *ColorPalette
	DarkPalette  *ColorPalette
}

// ResolvedTheme is the final resolved theme output.
// It contains palettes, fonts, colors, scale, and metadata.
// All palette colors are guaranteed to be CSS-safe hex colors.
// CSS generation is NOT part of this core type — it is a separate adapter concern.
type ResolvedTheme struct {
	LightPalette      ResolvedThemePalette
	DarkPalette       ResolvedThemePalette
	ActivePalette     ResolvedThemePalette
	PrefersDark       bool
	ColorSchemeSource string
	ThemeSource       ThemeSourceMetadata
	Fonts             ThemeFonts
	UIScale           float64
	ModeColors        ThemeModeColors
	Warnings          []ThemeWarning
}

// hexColorRegex matches valid CSS-safe #RRGGBB hex colors (strict).
var hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// IsValidHex returns true if s is a valid CSS-safe hex color (#RRGGBB format only).
func IsValidHex(s string) bool {
	return hexColorRegex.MatchString(s)
}

// DefaultThemeFonts returns Dumber's appearance font defaults.
func DefaultThemeFonts() ThemeFonts {
	return ThemeFonts{
		SansFont:      "Fira Sans",
		SerifFont:     "Fira Sans",
		MonospaceFont: "Fira Code",
		GtkFont:       "Adwaita Sans",
		DefaultSize:   16,
	}
}

// DefaultThemeModeColors returns the default mode indicator colors.
// These match the existing default mode colors from config/infrastructure.
func DefaultThemeModeColors() ThemeModeColors {
	return ThemeModeColors{
		PaneMode:    "#4A90E2",
		TabMode:     "#FFA500",
		SessionMode: "#9B59B6",
		ResizeMode:  "#00D4AA",
	}
}

// DefaultDarkPalette returns the built-in dark theme palette.
// These values mirror the config default dark palette.
func DefaultDarkPalette() ColorPalette {
	return ColorPalette{
		Background:     "#0a0a0b",
		Surface:        "#18181b",
		SurfaceVariant: "#27272a",
		Text:           "#fafafa",
		Muted:          "#a1a1aa",
		Accent:         "#4ade80",
		Border:         "#3f3f46",
	}
}

// DefaultLightPalette returns the built-in light theme palette.
// These values mirror the config default light palette.
func DefaultLightPalette() ColorPalette {
	return ColorPalette{
		Background:     "#fafafa",
		Surface:        "#f4f4f5",
		SurfaceVariant: "#e4e4e7",
		Text:           "#18181b",
		Muted:          "#71717a",
		Accent:         "#22c55e",
		Border:         "#d4d4d8",
	}
}

// ValidatePaletteHex checks every field in the palette is a valid #RRGGBB hex color.
// Returns a slice of warnings for each invalid field. Empty slice means valid.
func ValidatePaletteHex(p *ColorPalette, prefix string) []ThemeWarning {
	if p == nil {
		return []ThemeWarning{{Field: prefix, Message: "palette is nil"}}
	}
	var warnings []ThemeWarning
	checkRequiredHex(&warnings, prefix, "background", p.Background)
	checkRequiredHex(&warnings, prefix, "surface", p.Surface)
	checkRequiredHex(&warnings, prefix, "surface_variant", p.SurfaceVariant)
	checkRequiredHex(&warnings, prefix, "text", p.Text)
	checkRequiredHex(&warnings, prefix, "muted", p.Muted)
	checkRequiredHex(&warnings, prefix, "accent", p.Accent)
	checkRequiredHex(&warnings, prefix, "border", p.Border)
	return warnings
}

// ValidatePaletteOverrideHex checks non-empty override fields are valid #RRGGBB hex colors.
// Empty fields are accepted and mean "inherit from the fallback palette".
func ValidatePaletteOverrideHex(p *ColorPalette, prefix string) []ThemeWarning {
	if p == nil {
		return []ThemeWarning{{Field: prefix, Message: "palette is nil"}}
	}
	var warnings []ThemeWarning
	checkOptionalHex(&warnings, prefix, "background", p.Background)
	checkOptionalHex(&warnings, prefix, "surface", p.Surface)
	checkOptionalHex(&warnings, prefix, "surface_variant", p.SurfaceVariant)
	checkOptionalHex(&warnings, prefix, "text", p.Text)
	checkOptionalHex(&warnings, prefix, "muted", p.Muted)
	checkOptionalHex(&warnings, prefix, "accent", p.Accent)
	checkOptionalHex(&warnings, prefix, "border", p.Border)
	return warnings
}

func checkRequiredHex(warnings *[]ThemeWarning, prefix, field, value string) {
	if !IsValidHex(value) {
		*warnings = append(*warnings, ThemeWarning{
			Field:   prefix + "." + field,
			Message: "not a valid CSS-safe hex color (#RRGGBB)",
		})
	}
}

func checkOptionalHex(warnings *[]ThemeWarning, prefix, field, value string) {
	if value != "" && !IsValidHex(value) {
		*warnings = append(*warnings, ThemeWarning{
			Field:   prefix + "." + field,
			Message: "not a valid CSS-safe hex color (#RRGGBB)",
		})
	}
}

// IsPaletteValid returns true if every field in the palette is a valid #RRGGBB hex color.
func IsPaletteValid(p *ColorPalette) bool {
	return len(ValidatePaletteHex(p, "palette")) == 0
}

// IsPaletteOverrideValid returns true if every non-empty field in the palette is a valid #RRGGBB hex color.
func IsPaletteOverrideValid(p *ColorPalette) bool {
	return len(ValidatePaletteOverrideHex(p, "palette")) == 0
}

// MergePalette returns a new ColorPalette where empty fields in src are filled from defaults.
// Both src and defaults may be nil. Returns a copy, never mutates inputs.
func MergePalette(src, defaults *ColorPalette) ColorPalette {
	var d ColorPalette
	if defaults != nil {
		d = *defaults
	}
	if src == nil {
		return d
	}
	result := *src
	if result.Background == "" {
		result.Background = d.Background
	}
	if result.Surface == "" {
		result.Surface = d.Surface
	}
	if result.SurfaceVariant == "" {
		result.SurfaceVariant = d.SurfaceVariant
	}
	if result.Text == "" {
		result.Text = d.Text
	}
	if result.Muted == "" {
		result.Muted = d.Muted
	}
	if result.Accent == "" {
		result.Accent = d.Accent
	}
	if result.Border == "" {
		result.Border = d.Border
	}
	return result
}

// MergeFonts returns a ThemeFonts where zero/empty fields in src are filled from defaults.
func MergeFonts(src *ThemeFonts, defaults ThemeFonts) ThemeFonts {
	if src == nil {
		return defaults
	}
	result := *src
	if result.SansFont == "" {
		result.SansFont = defaults.SansFont
	}
	if result.SerifFont == "" {
		result.SerifFont = defaults.SerifFont
	}
	if result.MonospaceFont == "" {
		result.MonospaceFont = defaults.MonospaceFont
	}
	if result.GtkFont == "" {
		result.GtkFont = defaults.GtkFont
	}
	if result.DefaultSize <= 0 {
		result.DefaultSize = defaults.DefaultSize
	}
	return result
}

// ValidateModeColorsHex checks every mode color field is a valid #RRGGBB hex color.
func ValidateModeColorsHex(colors *ThemeModeColors, prefix string) []ThemeWarning {
	if colors == nil {
		return []ThemeWarning{{Field: prefix, Message: "mode colors are nil"}}
	}
	var warnings []ThemeWarning
	checkRequiredHex(&warnings, prefix, "pane_mode", colors.PaneMode)
	checkRequiredHex(&warnings, prefix, "tab_mode", colors.TabMode)
	checkRequiredHex(&warnings, prefix, "session_mode", colors.SessionMode)
	checkRequiredHex(&warnings, prefix, "resize_mode", colors.ResizeMode)
	return warnings
}

// IsModeColorsValid returns true when every mode color is a valid #RRGGBB hex color.
func IsModeColorsValid(colors *ThemeModeColors) bool {
	return len(ValidateModeColorsHex(colors, "mode_colors")) == 0
}

// MergeModeColors returns a ThemeModeColors where empty fields in src are filled from defaults.
func MergeModeColors(src *ThemeModeColors, defaults ThemeModeColors) ThemeModeColors {
	if src == nil {
		return defaults
	}
	result := *src
	if result.PaneMode == "" {
		result.PaneMode = defaults.PaneMode
	}
	if result.TabMode == "" {
		result.TabMode = defaults.TabMode
	}
	if result.SessionMode == "" {
		result.SessionMode = defaults.SessionMode
	}
	if result.ResizeMode == "" {
		result.ResizeMode = defaults.ResizeMode
	}
	return result
}

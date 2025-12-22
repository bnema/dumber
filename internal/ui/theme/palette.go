// Package theme provides GTK CSS styling for UI components.
package theme

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bnema/dumber/internal/infrastructure/config"
)

// Palette holds semantic color tokens for theming.
type Palette struct {
	Background     string // Main background color
	Surface        string // Elevated surfaces (cards, popups)
	SurfaceVariant string // Secondary surfaces
	Text           string // Primary text color
	Muted          string // Secondary/disabled text
	Accent         string // Primary accent color (actions, highlights)
	Border         string // Border and divider lines
	// Semantic status colors (not user-editable, derived defaults)
	Success     string // Success/positive feedback
	Warning     string // Warning/caution feedback
	Destructive string // Error/destructive actions
}

// DefaultDarkPalette returns the default dark theme palette.
func DefaultDarkPalette() Palette {
	return Palette{
		Background:     "#0a0a0b",
		Surface:        "#1a1a1b",
		SurfaceVariant: "#2d2d2d",
		Text:           "#ffffff",
		Muted:          "#909090",
		Accent:         "#4ade80",
		Border:         "#333333",
		Success:        "#4ade80",
		Warning:        "#fbbf24",
		Destructive:    "#ef4444",
	}
}

// DefaultLightPalette returns the default light theme palette.
func DefaultLightPalette() Palette {
	return Palette{
		Background:     "#fafafa",
		Surface:        "#ffffff",
		SurfaceVariant: "#f0f0f0",
		Text:           "#1a1a1a",
		Muted:          "#666666",
		Accent:         "#22c55e",
		Border:         "#dddddd",
		Success:        "#22c55e",
		Warning:        "#f59e0b",
		Destructive:    "#dc2626",
	}
}

// PaletteFromConfig creates a Palette from config values, filling missing values with defaults.
func PaletteFromConfig(cfg *config.ColorPalette, isDark bool) Palette {
	var defaults Palette
	if isDark {
		defaults = DefaultDarkPalette()
	} else {
		defaults = DefaultLightPalette()
	}

	if cfg == nil {
		return defaults
	}

	return Palette{
		Background:     Coalesce(cfg.Background, defaults.Background),
		Surface:        Coalesce(cfg.Surface, defaults.Surface),
		SurfaceVariant: Coalesce(cfg.SurfaceVariant, defaults.SurfaceVariant),
		Text:           Coalesce(cfg.Text, defaults.Text),
		Muted:          Coalesce(cfg.Muted, defaults.Muted),
		Accent:         Coalesce(cfg.Accent, defaults.Accent),
		Border:         Coalesce(cfg.Border, defaults.Border),
		// Semantic colors always use defaults (not user-editable)
		Success:     defaults.Success,
		Warning:     defaults.Warning,
		Destructive: defaults.Destructive,
	}
}

// Coalesce returns the first non-empty string.
func Coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// hexColorRegex matches valid hex colors (#RGB, #RRGGBB, #RRGGBBAA).
var hexColorRegex = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})$`)

// ValidateHexColor checks if a string is a valid hex color.
func ValidateHexColor(color string) error {
	if color == "" {
		return nil // Empty is valid (will use default)
	}
	if !hexColorRegex.MatchString(color) {
		return fmt.Errorf("invalid hex color: %s", color)
	}
	return nil
}

// Validate checks all palette colors are valid hex values.
func (p Palette) Validate() error {
	colors := map[string]string{
		"background":      p.Background,
		"surface":         p.Surface,
		"surface_variant": p.SurfaceVariant,
		"text":            p.Text,
		"muted":           p.Muted,
		"accent":          p.Accent,
		"border":          p.Border,
	}

	for name, color := range colors {
		if err := ValidateHexColor(color); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// ToCSSVars generates CSS custom property declarations for GTK.
func (p Palette) ToCSSVars() string {
	var sb strings.Builder
	sb.WriteString("  --bg: " + p.Background + ";\n")
	sb.WriteString("  --surface: " + p.Surface + ";\n")
	sb.WriteString("  --surface-variant: " + p.SurfaceVariant + ";\n")
	sb.WriteString("  --text: " + p.Text + ";\n")
	sb.WriteString("  --muted: " + p.Muted + ";\n")
	sb.WriteString("  --accent: " + p.Accent + ";\n")
	sb.WriteString("  --border: " + p.Border + ";\n")
	sb.WriteString("  --success: " + p.Success + ";\n")
	sb.WriteString("  --warning: " + p.Warning + ";\n")
	sb.WriteString("  --destructive: " + p.Destructive + ";\n")
	return sb.String()
}

// ToWebCSSVars generates CSS custom properties for web UI (Tailwind compatibility).
// Maps GTK palette names to Tailwind variable names used in the WebUI homepage.
func (p Palette) ToWebCSSVars() string {
	var sb strings.Builder
	// Tailwind-compatible variables
	sb.WriteString("  --background: " + p.Background + " !important;\n")
	sb.WriteString("  --foreground: " + p.Text + " !important;\n")
	sb.WriteString("  --card: " + p.Surface + " !important;\n")
	sb.WriteString("  --card-foreground: " + p.Text + " !important;\n")
	sb.WriteString("  --primary: " + p.Accent + " !important;\n")
	sb.WriteString("  --primary-foreground: " + p.Background + " !important;\n")
	sb.WriteString("  --muted: " + p.SurfaceVariant + " !important;\n")
	sb.WriteString("  --muted-foreground: " + p.Muted + " !important;\n")
	sb.WriteString("  --border: " + p.Border + " !important;\n")
	// Ring follows primary (accent)
	sb.WriteString("  --ring: " + p.Accent + " !important;\n")
	// Semantic colors from palette defaults
	sb.WriteString("  --success: " + p.Success + " !important;\n")
	sb.WriteString("  --success-foreground: " + p.Background + " !important;\n")
	sb.WriteString("  --warning: " + p.Warning + " !important;\n")
	sb.WriteString("  --warning-foreground: " + p.Text + " !important;\n")
	sb.WriteString("  --destructive: " + p.Destructive + " !important;\n")
	sb.WriteString("  --destructive-foreground: " + p.Background + " !important;\n")
	// Keep GTK names for compatibility
	sb.WriteString("  --bg: " + p.Background + " !important;\n")
	sb.WriteString("  --surface: " + p.Surface + " !important;\n")
	sb.WriteString("  --surface-variant: " + p.SurfaceVariant + " !important;\n")
	sb.WriteString("  --text: " + p.Text + " !important;\n")
	sb.WriteString("  --accent: " + p.Accent + " !important;\n")
	return sb.String()
}

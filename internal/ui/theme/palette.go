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

// ModeColors holds colors for modal mode indicators (borders and toasters).
type ModeColors struct {
	PaneMode    string // Color for pane mode (default: #4A90E2 blue)
	TabMode     string // Color for tab mode (default: #FFA500 orange)
	SessionMode string // Color for session mode (default: #9B59B6 purple)
	ResizeMode  string // Color for resize mode (default: #00D4AA teal)
}

// DefaultModeColors returns the default mode indicator colors.
func DefaultModeColors() ModeColors {
	return ModeColors{
		PaneMode:    "#4A90E2",
		TabMode:     "#FFA500",
		SessionMode: "#9B59B6",
		ResizeMode:  "#00D4AA",
	}
}

// ToCSSVars generates CSS custom property declarations for mode colors.
func (m ModeColors) ToCSSVars() string {
	var sb strings.Builder
	sb.WriteString("  --pane-mode-color: " + m.PaneMode + ";\n")
	sb.WriteString("  --tab-mode-color: " + m.TabMode + ";\n")
	sb.WriteString("  --session-mode-color: " + m.SessionMode + ";\n")
	sb.WriteString("  --resize-mode-color: " + m.ResizeMode + ";\n")
	return sb.String()
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

// Hex format length constants.
const (
	hexLenShort    = 3   // #RGB
	hexLenStandard = 6   // #RRGGBB
	hexLenWithAlha = 8   // #RRGGBBAA
	maxColorValue  = 255 // Maximum value for 8-bit color channel
)

// HexToRGBA converts a hex color string to RGBA float32 values (0.0-1.0).
// Supports #RGB, #RRGGBB, and #RRGGBBAA formats.
// Returns opaque black (0,0,0,1) if parsing fails.
func HexToRGBA(hex string) (r, g, b, a float32) {
	// Default to opaque black
	if hex == "" || hex[0] != '#' {
		return 0, 0, 0, 1
	}
	hex = hex[1:]

	var ri, gi, bi, ai uint64
	switch len(hex) {
	case hexLenShort: // #RGB
		ri, _ = parseHex(hex[0:1])
		gi, _ = parseHex(hex[1:2])
		bi, _ = parseHex(hex[2:3])
		ri *= 17 // 0xF -> 0xFF
		gi *= 17
		bi *= 17
		ai = maxColorValue
	case hexLenStandard: // #RRGGBB
		ri, _ = parseHex(hex[0:2])
		gi, _ = parseHex(hex[2:4])
		bi, _ = parseHex(hex[4:6])
		ai = maxColorValue
	case hexLenWithAlha: // #RRGGBBAA
		ri, _ = parseHex(hex[0:2])
		gi, _ = parseHex(hex[2:4])
		bi, _ = parseHex(hex[4:6])
		ai, _ = parseHex(hex[6:8])
	default:
		return 0, 0, 0, 1
	}

	return float32(ri) / maxColorValue, float32(gi) / maxColorValue, float32(bi) / maxColorValue, float32(ai) / maxColorValue
}

// parseHex parses a hex string to uint64.
func parseHex(s string) (uint64, error) {
	var result uint64
	for _, c := range s {
		result *= 16
		switch {
		case c >= '0' && c <= '9':
			result += uint64(c - '0')
		case c >= 'a' && c <= 'f':
			result += uint64(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			result += uint64(c - 'A' + 10)
		default:
			return 0, fmt.Errorf("invalid hex char: %c", c)
		}
	}
	return result, nil
}

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

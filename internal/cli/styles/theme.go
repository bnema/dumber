// Package styles provides reusable lipgloss-based TUI components.
package styles

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/infrastructure/config"
)

// Theme holds lipgloss colors and styles derived from config.
type Theme struct {
	// Base colors (from config.ColorPalette)
	Background     lipgloss.Color
	Surface        lipgloss.Color
	SurfaceVariant lipgloss.Color
	Text           lipgloss.Color
	Muted          lipgloss.Color
	Accent         lipgloss.Color
	Border         lipgloss.Color

	// Additional semantic colors
	Error   lipgloss.Color
	Warning lipgloss.Color
	Success lipgloss.Color

	// Pre-built styles
	Title        lipgloss.Style
	Subtitle     lipgloss.Style
	Normal       lipgloss.Style
	Subtle       lipgloss.Style
	Highlight    lipgloss.Style
	ErrorStyle   lipgloss.Style
	WarningStyle lipgloss.Style
	SuccessStyle lipgloss.Style

	// Component styles
	ActiveTab   lipgloss.Style
	InactiveTab lipgloss.Style
	TabBar      lipgloss.Style

	ListItem         lipgloss.Style
	ListItemSelected lipgloss.Style
	ListItemTitle    lipgloss.Style
	ListItemDesc     lipgloss.Style

	Badge      lipgloss.Style
	BadgeMuted lipgloss.Style

	Input        lipgloss.Style
	InputFocused lipgloss.Style

	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style

	Box       lipgloss.Style
	BoxHeader lipgloss.Style
}

// DefaultDarkPalette returns hardcoded dark theme colors.
func DefaultDarkPalette() config.ColorPalette {
	return config.ColorPalette{
		Background:     "#0a0a0b",
		Surface:        "#1a1a1b",
		SurfaceVariant: "#2d2d2d",
		Text:           "#ffffff",
		Muted:          "#909090",
		Accent:         "#4ade80",
		Border:         "#333333",
	}
}

// NewTheme creates a Theme from config, using dark palette only.
func NewTheme(cfg *config.Config) *Theme {
	var p config.ColorPalette
	if cfg != nil && cfg.Appearance.DarkPalette.Background != "" {
		p = cfg.Appearance.DarkPalette
	} else {
		p = DefaultDarkPalette()
	}

	return NewThemeFromPalette(p)
}

// NewThemeFromPalette creates a Theme from a ColorPalette.
func NewThemeFromPalette(p config.ColorPalette) *Theme {
	t := &Theme{
		Background:     lipgloss.Color(p.Background),
		Surface:        lipgloss.Color(p.Surface),
		SurfaceVariant: lipgloss.Color(p.SurfaceVariant),
		Text:           lipgloss.Color(p.Text),
		Muted:          lipgloss.Color(p.Muted),
		Accent:         lipgloss.Color(p.Accent),
		Border:         lipgloss.Color(p.Border),

		// Semantic colors (not in config, use sensible defaults)
		Error:   lipgloss.Color("#ef4444"),
		Warning: lipgloss.Color("#f59e0b"),
		Success: lipgloss.Color(p.Accent), // Use accent as success
	}

	// Build derived styles
	t.buildStyles()
	return t
}

// buildStyles creates all derived lipgloss styles.
func (t *Theme) buildStyles() {
	// Text styles
	t.Title = lipgloss.NewStyle().
		Foreground(t.Text).
		Bold(true)

	t.Subtitle = lipgloss.NewStyle().
		Foreground(t.Muted).
		Bold(true)

	t.Normal = lipgloss.NewStyle().
		Foreground(t.Text)

	t.Subtle = lipgloss.NewStyle().
		Foreground(t.Muted)

	t.Highlight = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)

	t.ErrorStyle = lipgloss.NewStyle().
		Foreground(t.Error)

	t.WarningStyle = lipgloss.NewStyle().
		Foreground(t.Warning)

	t.SuccessStyle = lipgloss.NewStyle().
		Foreground(t.Success)

	// Tab styles
	t.ActiveTab = lipgloss.NewStyle().
		Foreground(t.Background).
		Background(t.Accent).
		Padding(0, 2).
		Bold(true)

	t.InactiveTab = lipgloss.NewStyle().
		Foreground(t.Muted).
		Background(t.Surface).
		Padding(0, 2)

	t.TabBar = lipgloss.NewStyle().
		Background(t.Surface).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(t.Border)

	// List item styles
	t.ListItem = lipgloss.NewStyle().
		Foreground(t.Text).
		PaddingLeft(2)

	t.ListItemSelected = lipgloss.NewStyle().
		Foreground(t.Accent).
		Background(t.SurfaceVariant).
		PaddingLeft(2).
		Bold(true)

	t.ListItemTitle = lipgloss.NewStyle().
		Foreground(t.Text)

	t.ListItemDesc = lipgloss.NewStyle().
		Foreground(t.Muted)

	// Badge styles
	t.Badge = lipgloss.NewStyle().
		Foreground(t.Background).
		Background(t.Accent).
		Padding(0, 1)

	t.BadgeMuted = lipgloss.NewStyle().
		Foreground(t.Text).
		Background(t.SurfaceVariant).
		Padding(0, 1)

	// Input styles
	t.Input = lipgloss.NewStyle().
		Foreground(t.Text).
		Background(t.Surface).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)

	t.InputFocused = lipgloss.NewStyle().
		Foreground(t.Text).
		Background(t.Surface).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Padding(0, 1)

	// Help styles
	t.HelpKey = lipgloss.NewStyle().
		Foreground(t.Accent)

	t.HelpDesc = lipgloss.NewStyle().
		Foreground(t.Muted)

	// Box/container styles
	t.Box = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(1, 2)

	t.BoxHeader = lipgloss.NewStyle().
		Foreground(t.Text).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(t.Border).
		MarginBottom(1)
}

// Renderer returns a lipgloss renderer for the current output.
func (t *Theme) Renderer() *lipgloss.Renderer {
	if t == nil {
		return lipgloss.DefaultRenderer()
	}
	return lipgloss.DefaultRenderer()
}

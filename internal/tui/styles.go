package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Grayscale palette (ANSI 232-255)
const (
	ColorBlack      = lipgloss.Color("232") // Nearly black
	ColorDarkGray   = lipgloss.Color("236")
	ColorGray       = lipgloss.Color("240")
	ColorMediumGray = lipgloss.Color("244")
	ColorLightGray  = lipgloss.Color("248")
	ColorVeryLight  = lipgloss.Color("252")
	ColorWhite      = lipgloss.Color("255") // Nearly white
)

// Base styles for reuse across TUIs.
var (
	BaseStyle     = lipgloss.NewStyle().Foreground(ColorVeryLight)
	FaintStyle    = BaseStyle.Copy().Foreground(ColorLightGray)
	MutedStyle    = BaseStyle.Copy().Foreground(ColorGray)
	HeaderStyle   = BaseStyle.Copy().Bold(true).Foreground(ColorWhite)
	SubtleBorder  = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(ColorDarkGray)
	ActiveStyle   = lipgloss.NewStyle().Foreground(ColorWhite).Background(ColorDarkGray)
	InactiveStyle = BaseStyle.Copy().Foreground(ColorMediumGray)
	SelectedStyle = lipgloss.NewStyle().Foreground(ColorWhite).Background(ColorDarkGray).Bold(true)
	ErrorStyle    = lipgloss.NewStyle().Foreground(ColorWhite).Background(ColorBlack)
	SuccessStyle  = lipgloss.NewStyle().Foreground(ColorBlack).Background(ColorVeryLight)
)

// Badge renders a small label with the given colors.
func Badge(text string, fg, bg lipgloss.Color) string {
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Padding(0, 1).
		MarginRight(1).
		Bold(true).
		Render(text)
}

// TruncateMiddle shortens text by keeping start/end and inserting an ellipsis.
func TruncateMiddle(text string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(text) <= width {
		return text
	}

	if width <= 1 {
		return text[:width]
	}

	prefix := (width - 1) / 2
	suffix := width - prefix - 1

	runes := []rune(text)
	if len(runes) <= width {
		return text
	}

	start := string(runes[:prefix])
	end := string(runes[len(runes)-suffix:])
	return start + "…" + end
}

// Truncate shortens text from the end with an ellipsis if it exceeds width.
func Truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(text) <= width {
		return text
	}

	if width == 1 {
		return "…"
	}

	// Convert to runes for Unicode-safe truncation
	runes := []rune(text)
	if len(runes) <= width-1 {
		return text
	}

	// Keep first (width-1) runes and add ellipsis
	return string(runes[:width-1]) + "…"
}

// Columnize renders values padded to their respective widths.
// Text exceeding the allocated width is truncated with an ellipsis.
func Columnize(values []string, widths []int) string {
	var b strings.Builder
	for i, val := range values {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		rendered := val
		if w > 0 {
			// Truncate FIRST if text exceeds width (lipgloss.Width measures rendered width)
			if lipgloss.Width(val) > w {
				val = Truncate(val, w)
			}
			// Then apply Width() for padding/alignment
			rendered = lipgloss.NewStyle().Width(w).Render(val)
		}
		b.WriteString(rendered)
		if i < len(values)-1 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

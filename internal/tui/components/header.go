package components

import (
	"github.com/bnema/dumber/internal/tui"
	"github.com/charmbracelet/lipgloss"
)

// Header renders a title with optional metadata.
type Header struct {
	Title string
	Meta  string
}

// View returns the styled header string.
func (h Header) View() string {
	title := tui.HeaderStyle.Render(h.Title)
	if h.Meta == "" {
		return title
	}

	meta := lipgloss.NewStyle().
		Foreground(tui.ColorLightGray).
		PaddingLeft(1).
		Render(h.Meta)

	return lipgloss.JoinHorizontal(lipgloss.Top, title, meta)
}

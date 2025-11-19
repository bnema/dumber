package components

import (
	"github.com/bnema/dumber/internal/tui"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// Help renders a compact help footer using bubbles/help.
type Help struct {
	model help.Model
	keys  []key.Binding
}

// NewHelp creates a new help component with the provided bindings.
func NewHelp(keys []key.Binding) Help {
	h := help.New()
	h.ShortSeparator = " • "
	h.Styles.ShortDesc = tui.MutedStyle
	h.Styles.ShortKey = tui.FaintStyle.Copy().Bold(true)

	return Help{
		model: h,
		keys:  keys,
	}
}

// View renders the footer.
func (h Help) View() string {
	if len(h.keys) == 0 {
		return ""
	}
	return lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(tui.ColorDarkGray).
		PaddingTop(1).
		Render(h.model.ShortHelpView(h.keys))
}

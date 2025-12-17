package styles

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// NewStyledInput creates a themed text input.
func NewStyledInput(theme *Theme, placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	ti.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Accent)
	ti.PromptStyle = lipgloss.NewStyle().Foreground(theme.Accent)
	ti.Prompt = "/ "
	return ti
}

// NewSearchInput creates a search-specific input.
func NewSearchInput(theme *Theme) textinput.Model {
	ti := NewStyledInput(theme, "Search...")
	ti.Prompt = "🔍 "
	ti.CharLimit = 256
	return ti
}

// NewURLInput creates a URL input field.
func NewURLInput(theme *Theme) textinput.Model {
	ti := NewStyledInput(theme, "Enter URL or search query...")
	ti.Prompt = "→ "
	ti.CharLimit = 2048
	return ti
}

// InputBox wraps a text input in a styled box.
func (t *Theme) InputBox(input string, focused bool) string {
	style := t.Input
	if focused {
		style = t.InputFocused
	}
	return style.Render(input)
}

package styles

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmModel is a yes/no confirmation dialog.
type ConfirmModel struct {
	Message   string
	Yes       bool // Current selection
	Confirmed bool // User pressed enter
	Canceled  bool // User pressed escape
	theme     *Theme
}

// ConfirmKeyMap defines keybindings for the confirm dialog.
type ConfirmKeyMap struct {
	Yes     key.Binding
	No      key.Binding
	Left    key.Binding
	Right   key.Binding
	Confirm key.Binding
	Cancel  key.Binding
}

// DefaultConfirmKeyMap returns the default keybindings.
func DefaultConfirmKeyMap() ConfirmKeyMap {
	return ConfirmKeyMap{
		Yes:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		No:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "no")),
		Left:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "no")),
		Right:   key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "yes")),
		Confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Cancel:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

// NewConfirm creates a new confirmation dialog.
func NewConfirm(theme *Theme, message string) ConfirmModel {
	return ConfirmModel{
		Message: message,
		Yes:     false, // Default to "No"
		theme:   theme,
	}
}

// Init implements tea.Model.
func (m ConfirmModel) Init() tea.Cmd {
	if m.theme == nil {
		return nil
	}
	return nil
}

// Update implements tea.Model.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	keys := DefaultConfirmKeyMap()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Yes):
			m.Yes = true
		case key.Matches(msg, keys.No):
			m.Yes = false
		case key.Matches(msg, keys.Left):
			m.Yes = false
		case key.Matches(msg, keys.Right):
			m.Yes = true
		case key.Matches(msg, keys.Confirm):
			m.Confirmed = true
		case key.Matches(msg, keys.Cancel):
			m.Canceled = true
		}
	}

	return m, nil
}

// View implements tea.Model.
func (m ConfirmModel) View() string {
	t := m.theme

	// Build buttons
	yesStyle := t.InactiveTab
	noStyle := t.InactiveTab

	if m.Yes {
		yesStyle = t.ActiveTab
	} else {
		noStyle = t.ActiveTab
	}

	yes := yesStyle.Render(" Yes ")
	no := noStyle.Render(" No ")

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, no, "  ", yes)

	// Build dialog box
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		t.Title.Render(m.Message),
		"",
		buttons,
		"",
		t.Subtle.Render("y/n or ←/→ to select • enter to confirm • esc to cancel"),
	)

	return t.Box.Render(content)
}

// Done returns true if the dialog is complete.
func (m ConfirmModel) Done() bool {
	return m.Confirmed || m.Canceled
}

// Result returns true if user confirmed "Yes".
func (m ConfirmModel) Result() bool {
	return m.Confirmed && m.Yes
}

// ConfirmResult wraps the confirm dialog result for parent models.
type ConfirmResult struct {
	Confirmed bool
	Yes       bool
}

// ConfirmResultMsg is sent when the confirm dialog completes.
type ConfirmResultMsg ConfirmResult

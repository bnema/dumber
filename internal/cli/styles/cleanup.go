package styles

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CleanupRange represents a time range for history cleanup.
type CleanupRange int

const (
	CleanupRangeLastHour CleanupRange = iota
	CleanupRangeLast24Hours
	CleanupRangeLast7Days
	CleanupRangeLast30Days
	CleanupRangeAll
)

// Cursor symbols for list items (using Unicode).
const (
	cursorEmpty    = "  "
	cursorSelected = "\u25b8 " // ▸ Black right-pointing small triangle
)

// CleanupRangeInfo holds display info for a cleanup range.
type CleanupRangeInfo struct {
	Range       CleanupRange
	Label       string
	Description string
}

// AllCleanupRanges returns all available cleanup ranges.
func AllCleanupRanges() []CleanupRangeInfo {
	return []CleanupRangeInfo{
		{CleanupRangeLastHour, "Last hour", "Delete entries from the last hour"},
		{CleanupRangeLast24Hours, "Last 24 hours", "Delete entries from the last day"},
		{CleanupRangeLast7Days, "Last 7 days", "Delete entries from the last week"},
		{CleanupRangeLast30Days, "Last 30 days", "Delete entries from the last month"},
		{CleanupRangeAll, "All time", "Delete all history entries"},
	}
}

// CutoffTime returns the cutoff time for this range.
func (r CleanupRange) CutoffTime() time.Time {
	now := time.Now()
	switch r {
	case CleanupRangeLastHour:
		return now.Add(-1 * time.Hour)
	case CleanupRangeLast24Hours:
		return now.Add(-24 * time.Hour)
	case CleanupRangeLast7Days:
		return now.Add(-7 * 24 * time.Hour)
	case CleanupRangeLast30Days:
		return now.Add(-30 * 24 * time.Hour)
	case CleanupRangeAll:
		return time.Time{} // Zero time = delete all
	default:
		return time.Time{}
	}
}

// CleanupModel is a cleanup range selection modal.
type CleanupModel struct {
	Ranges    []CleanupRangeInfo
	Selected  int
	Confirmed bool
	Cancelled bool
	theme     *Theme
}

// CleanupKeyMap defines keybindings for cleanup modal.
type CleanupKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Confirm key.Binding
	Cancel  key.Binding
}

// DefaultCleanupKeyMap returns default keybindings.
func DefaultCleanupKeyMap() CleanupKeyMap {
	return CleanupKeyMap{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Cancel:  key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "cancel")),
	}
}

// NewCleanup creates a new cleanup modal.
func NewCleanup(theme *Theme) CleanupModel {
	return CleanupModel{
		Ranges:   AllCleanupRanges(),
		Selected: 0,
		theme:    theme,
	}
}

// Init implements tea.Model.
func (m CleanupModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m CleanupModel) Update(msg tea.Msg) (CleanupModel, tea.Cmd) {
	keys := DefaultCleanupKeyMap()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			if m.Selected > 0 {
				m.Selected--
			}
		case key.Matches(msg, keys.Down):
			if m.Selected < len(m.Ranges)-1 {
				m.Selected++
			}
		case key.Matches(msg, keys.Confirm):
			m.Confirmed = true
		case key.Matches(msg, keys.Cancel):
			m.Cancelled = true
		}
	}

	return m, nil
}

// View implements tea.Model.
func (m CleanupModel) View() string {
	t := m.theme

	// Build header
	header := t.Title.
		Foreground(t.Error).
		Render("Clear History")

	// Build range list
	var rows []string
	for i, r := range m.Ranges {
		cursor := cursorEmpty
		style := t.ListItem
		if i == m.Selected {
			cursor = cursorSelected
			style = t.ListItemSelected
		}

		label := style.Render(r.Label)
		desc := t.Subtle.Render(r.Description)

		row := lipgloss.JoinHorizontal(
			lipgloss.Left,
			t.Highlight.Render(cursor),
			lipgloss.JoinVertical(lipgloss.Left, label, desc),
		)
		rows = append(rows, row)
	}

	list := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Warning for "All time"
	var warning string
	if m.Ranges[m.Selected].Range == CleanupRangeAll {
		warning = t.ErrorStyle.Render("⚠ This will delete ALL history entries!")
	}

	// Help text
	help := t.Subtle.Render("↑/↓ to select • enter to confirm • esc to cancel")

	// Combine
	var content string
	if warning != "" {
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			"",
			list,
			"",
			warning,
			"",
			help,
		)
	} else {
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			"",
			list,
			"",
			help,
		)
	}

	return t.Box.Render(content)
}

// Done returns true if the modal is complete.
func (m CleanupModel) Done() bool {
	return m.Confirmed || m.Cancelled
}

// SelectedRange returns the selected cleanup range.
func (m CleanupModel) SelectedRange() CleanupRange {
	if m.Selected >= 0 && m.Selected < len(m.Ranges) {
		return m.Ranges[m.Selected].Range
	}
	return CleanupRangeAll
}

// CleanupResultMsg is sent when cleanup modal completes.
type CleanupResultMsg struct {
	Confirmed bool
	Range     CleanupRange
}

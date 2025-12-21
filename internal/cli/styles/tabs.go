package styles

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// TabsModel represents a horizontal tab bar.
type TabsModel struct {
	Tabs   []string
	Active int
	theme  *Theme
}

// NewTabs creates a new tab bar with the given labels.
func NewTabs(theme *Theme, tabs ...string) TabsModel {
	return TabsModel{
		Tabs:   tabs,
		Active: 0,
		theme:  theme,
	}
}

// SetActive sets the active tab index.
func (m *TabsModel) SetActive(index int) {
	if index >= 0 && index < len(m.Tabs) {
		m.Active = index
	}
}

// Next moves to the next tab.
func (m *TabsModel) Next() {
	m.Active = (m.Active + 1) % len(m.Tabs)
}

// Prev moves to the previous tab.
func (m *TabsModel) Prev() {
	m.Active = (m.Active - 1 + len(m.Tabs)) % len(m.Tabs)
}

// View renders the tab bar.
func (m TabsModel) View() string {
	var tabs []string

	for i, tab := range m.Tabs {
		style := m.theme.InactiveTab
		if i == m.Active {
			style = m.theme.ActiveTab
		}
		tabs = append(tabs, style.Render(tab))
	}

	// Join tabs with separator
	gap := lipgloss.NewStyle().
		Foreground(m.theme.Border).
		Render(" │ ")

	row := ""
	for i, tab := range tabs {
		if i > 0 {
			row += gap
		}
		row += tab
	}

	const tabBarWidth = 80
	return m.theme.TabBar.Width(tabBarWidth).Render(row)
}

// ViewCompact renders a compact tab bar without separators.
func (m TabsModel) ViewCompact() string {
	var tabs []string

	for i, tab := range m.Tabs {
		style := m.theme.InactiveTab
		if i == m.Active {
			style = m.theme.ActiveTab
		}
		tabs = append(tabs, style.Render(tab))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

// TimelineTabs creates tabs for history timeline view.
func TimelineTabs(theme *Theme) TabsModel {
	return NewTabs(theme, "Today", "Yesterday", "This Week", "Older")
}

// ViewWithCounts renders tabs with item counts.
func (m TabsModel) ViewWithCounts(counts []int) string {
	var tabs []string

	for i, tab := range m.Tabs {
		count := 0
		if i < len(counts) {
			count = counts[i]
		}

		label := tab
		if count > 0 {
			label = lipgloss.JoinHorizontal(
				lipgloss.Center,
				tab,
				" ",
				m.theme.BadgeMuted.Render(formatCount(count)),
			)
		}

		style := m.theme.InactiveTab
		if i == m.Active {
			style = m.theme.ActiveTab
		}
		tabs = append(tabs, style.Render(label))
	}

	gap := lipgloss.NewStyle().
		Foreground(m.theme.Border).
		Render(" ")

	return lipgloss.JoinHorizontal(lipgloss.Top, join(tabs, gap)...)
}

// join inserts a separator between items.
func join(items []string, sep string) []string {
	if len(items) == 0 {
		return items
	}
	result := make([]string, 0, len(items)*2-1)
	for i, item := range items {
		if i > 0 {
			result = append(result, sep)
		}
		result = append(result, item)
	}
	return result
}

// formatCount formats a count for display.
func formatCount(n int) string {
	if n >= 1000 {
		return "999+"
	}
	return fmt.Sprintf("%d", n)
}

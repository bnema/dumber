package styles

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HistoryItem represents a history entry for the list.
type HistoryItem struct {
	ID          int64
	URL         string
	Title       string
	Domain      string
	VisitCount  int
	LastVisited time.Time
}

// FilterValue implements list.Item.
func (i HistoryItem) FilterValue() string {
	return i.Title + " " + i.URL
}

// Title implements list.DefaultItem.
func (i HistoryItem) TitleValue() string {
	if i.Title != "" {
		return i.Title
	}
	return i.URL
}

// Description implements list.DefaultItem.
func (i HistoryItem) DescriptionValue() string {
	return i.URL
}

// HistoryDelegate renders history items with theme styling.
type HistoryDelegate struct {
	Theme      *Theme
	ShowDomain bool
}

// NewHistoryDelegate creates a themed history list delegate.
func NewHistoryDelegate(theme *Theme) HistoryDelegate {
	return HistoryDelegate{
		Theme:      theme,
		ShowDomain: true,
	}
}

// Height returns the height of each item.
func (d HistoryDelegate) Height() int {
	if d.Theme == nil {
		return 2
	}
	return 2
}

// Spacing returns the spacing between items.
func (d HistoryDelegate) Spacing() int {
	if d.Theme == nil {
		return 0
	}
	return 0
}

// Update handles item-level events.
func (d HistoryDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	if d.Theme == nil {
		return nil
	}
	return nil
}

// Render renders a single list item.
func (d HistoryDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	hi, ok := item.(HistoryItem)
	if !ok {
		return
	}

	t := d.Theme
	isSelected := index == m.Index()
	const (
		maxTitleLength = 60
		maxURLLength   = 50
		ellipsisLength = 3
	)

	// Title line
	title := hi.TitleValue()
	if len(title) > maxTitleLength {
		title = title[:maxTitleLength-ellipsisLength] + "..."
	}

	// URL/description line
	url := hi.URL
	if len(url) > maxURLLength {
		url = url[:maxURLLength-ellipsisLength] + "..."
	}

	// Metadata badges
	visitBadge := t.MutedBadge(fmt.Sprintf("%d", hi.VisitCount))
	timeBadge := t.MutedBadge(RelativeTime(hi.LastVisited))

	// Build cursor
	cursor := cursorEmpty
	if isSelected {
		cursor = cursorSelected
	}

	// Apply styles
	cursorStyle := t.Highlight
	titleStyle := t.ListItemTitle
	urlStyle := t.ListItemDesc

	if isSelected {
		titleStyle = titleStyle.Foreground(t.Accent).Bold(true)
		urlStyle = urlStyle.Foreground(t.Text)
	}

	// First line: cursor + title
	line1 := lipgloss.JoinHorizontal(
		lipgloss.Left,
		cursorStyle.Render(cursor),
		titleStyle.Render(title),
	)

	// Second line: url + badges
	meta := lipgloss.JoinHorizontal(
		lipgloss.Left,
		visitBadge,
		" ",
		timeBadge,
	)

	line2 := lipgloss.JoinHorizontal(
		lipgloss.Left,
		strings.Repeat(" ", 3), // Indent under cursor
		urlStyle.Render(url),
		" ",
		meta,
	)

	_, _ = fmt.Fprintf(w, "%s\n%s", line1, line2)
}

// NewHistoryList creates a themed list for history items.
func NewHistoryList(theme *Theme, items []HistoryItem, width, height int) list.Model {
	// Convert to list.Item slice
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}

	delegate := NewHistoryDelegate(theme)

	l := list.New(listItems, delegate, width, height)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.SetShowPagination(true)

	// Apply theme colors to pagination
	l.Styles.PaginationStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	l.Styles.ActivePaginationDot = lipgloss.NewStyle().Foreground(theme.Accent)
	l.Styles.InactivePaginationDot = lipgloss.NewStyle().Foreground(theme.Muted)

	return l
}

// SimpleItem is a basic list item with title and description.
type SimpleItem struct {
	TitleText string
	DescText  string
}

// FilterValue implements list.Item.
func (i SimpleItem) FilterValue() string {
	return i.TitleText + " " + i.DescText
}

// SimpleDelegate renders simple items.
type SimpleDelegate struct {
	Theme *Theme
}

// Height returns the height of each item.
func (d SimpleDelegate) Height() int {
	if d.Theme == nil {
		return 1
	}
	return 1
}

// Spacing returns the spacing between items.
func (d SimpleDelegate) Spacing() int {
	if d.Theme == nil {
		return 0
	}
	return 0
}

// Update handles item-level events.
func (d SimpleDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	if d.Theme == nil {
		return nil
	}
	return nil
}

// Render renders a single list item.
func (d SimpleDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(SimpleItem)
	if !ok {
		return
	}

	t := d.Theme
	isSelected := index == m.Index()

	cursor := cursorEmpty
	style := t.ListItem
	if isSelected {
		cursor = cursorSelected
		style = t.ListItemSelected
	}

	line := lipgloss.JoinHorizontal(
		lipgloss.Left,
		t.Highlight.Render(cursor),
		style.Render(si.TitleText),
	)

	if si.DescText != "" {
		line = lipgloss.JoinHorizontal(
			lipgloss.Left,
			line,
			" ",
			t.Subtle.Render(si.DescText),
		)
	}

	_, _ = fmt.Fprint(w, line)
}

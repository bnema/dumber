package styles

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// NewStyledTable creates a themed table model.
func NewStyledTable(theme *Theme, columns []table.Column, rows []table.Row, width, height int) table.Model {
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
		table.WithWidth(width),
	)

	// Apply theme styles
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.Border).
		BorderBottom(true).
		Foreground(theme.Accent).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(theme.Text).
		Background(theme.SurfaceVariant).
		Bold(true)
	s.Cell = s.Cell.
		Foreground(theme.Text)

	t.SetStyles(s)
	return t
}

// StatsTableColumns returns columns for domain stats table.
func StatsTableColumns() []table.Column {
	return []table.Column{
		{Title: "Domain", Width: 30},
		{Title: "Visits", Width: 10},
		{Title: "Entries", Width: 10},
		{Title: "Last Visit", Width: 15},
	}
}

// HistoryTableColumns returns columns for history list table.
func HistoryTableColumns() []table.Column {
	return []table.Column{
		{Title: "Title", Width: 40},
		{Title: "URL", Width: 30},
		{Title: "Visits", Width: 8},
		{Title: "Last Visit", Width: 12},
	}
}

// DomainStatsRow converts domain stats to table row.
type DomainStatsRow struct {
	Domain    string
	Visits    int
	Entries   int
	LastVisit string
}

// ToRow converts to table.Row.
func (d DomainStatsRow) ToRow() table.Row {
	return table.Row{d.Domain, formatInt(d.Visits), formatInt(d.Entries), d.LastVisit}
}

// formatInt formats an integer for display.
func formatInt(n int) string {
	switch {
	case n >= 1000000:
		return formatFloat(float64(n)/1000000) + "M"
	case n >= 1000:
		return formatFloat(float64(n)/1000) + "K"
	default:
		return intToString(n)
	}
}

// formatFloat formats a float with one decimal.
func formatFloat(f float64) string {
	i := int(f * 10)
	whole := i / 10
	dec := i % 10
	if dec == 0 {
		return intToString(whole)
	}
	return intToString(whole) + "." + intToString(dec)
}

// intToString converts int to string without fmt.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + intToString(-n)
	}

	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}

	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}

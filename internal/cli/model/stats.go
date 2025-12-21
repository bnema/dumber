package model

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
)

// StatsModel displays history analytics.
type StatsModel struct {
	analytics *entity.HistoryAnalytics
	table     table.Model
	loading   bool
	err       error
	width     int
	height    int

	historyUC *usecase.SearchHistoryUseCase
	theme     *styles.Theme
}

// NewStatsModel creates a new stats display model.
func NewStatsModel(theme *styles.Theme, historyUC *usecase.SearchHistoryUseCase) StatsModel {
	return StatsModel{
		historyUC: historyUC,
		theme:     theme,
		loading:   true,
		width:     80,
		height:    24,
	}
}

// analyticsLoadedMsg is sent when analytics are loaded.
type analyticsLoadedMsg struct {
	analytics *entity.HistoryAnalytics
	err       error
}

// Init implements tea.Model.
func (m StatsModel) Init() tea.Cmd {
	return m.loadAnalytics
}

// loadAnalytics loads history analytics.
func (m StatsModel) loadAnalytics() tea.Msg {
	ctx := context.Background()
	analytics, err := m.historyUC.GetAnalytics(ctx)
	return analyticsLoadedMsg{analytics: analytics, err: err}
}

// Update implements tea.Model.
func (m StatsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateTable()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			m.table, cmd = m.table.Update(msg)
			return m, cmd
		}

	case analyticsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.analytics = msg.analytics
			m.updateTable()
		}
	}

	return m, nil
}

// updateTable builds the domain stats table.
func (m *StatsModel) updateTable() {
	if m.analytics == nil || len(m.analytics.TopDomains) == 0 {
		return
	}

	columns := styles.StatsTableColumns()

	rows := make([]table.Row, len(m.analytics.TopDomains))
	for i, d := range m.analytics.TopDomains {
		rows[i] = table.Row{
			d.Domain,
			formatNumber(d.TotalVisits),
			formatNumber(d.PageCount),
			styles.RelativeTime(d.LastVisit),
		}
	}

	tableHeight := len(rows)
	if tableHeight > m.height-10 {
		tableHeight = m.height - 10
	}
	if tableHeight < 3 {
		tableHeight = 3
	}

	m.table = styles.NewStyledTable(m.theme, columns, rows, m.width-4, tableHeight)
}

// View implements tea.Model.
func (m StatsModel) View() string {
	t := m.theme

	if m.loading {
		spinner := styles.NewLoading(t, "Loading analytics...")
		return t.Box.Render(spinner.View())
	}

	if m.err != nil {
		return t.Box.Render(t.ErrorStyle.Render("Error: " + m.err.Error()))
	}

	if m.analytics == nil {
		return t.Box.Render(t.Subtle.Render("No data available"))
	}

	// Build header with summary stats
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		t.Title.Render("History Analytics"),
		"",
		lipgloss.JoinHorizontal(
			lipgloss.Top,
			t.Badge.Render(fmt.Sprintf("%d entries", m.analytics.TotalEntries)),
			" ",
			t.Badge.Render(fmt.Sprintf("%d visits", m.analytics.TotalVisits)),
			" ",
			t.BadgeMuted.Render(fmt.Sprintf("%d days", m.analytics.UniqueDays)),
		),
	)

	// Domain table
	tableHeader := t.Subtitle.Render("Top Domains")
	tableView := m.table.View()

	// Daily activity (simple bar chart)
	var activityView string
	if len(m.analytics.DailyVisits) > 0 {
		activityView = m.renderDailyActivity()
	}

	// Help
	helpView := t.Subtle.Render("q to quit")

	// Combine
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		tableHeader,
		tableView,
	)

	if activityView != "" {
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			t.Subtitle.Render("Daily Activity (Last 30 Days)"),
			activityView,
		)
	}

	content = lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		"",
		helpView,
	)

	return content
}

// renderDailyActivity renders a simple sparkline-style bar chart.
func (m StatsModel) renderDailyActivity() string {
	if m.analytics == nil || len(m.analytics.DailyVisits) == 0 {
		return ""
	}

	// Find max for scaling
	var maxVisits int64
	for _, d := range m.analytics.DailyVisits {
		if d.Visits > maxVisits {
			maxVisits = d.Visits
		}
	}

	if maxVisits == 0 {
		return m.theme.Subtle.Render("No activity")
	}

	// Build sparkline using block characters
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	const (
		sparklineMaxDays  = 30
		sparklineMaxIndex = 7
	)
	var result string

	// Show last 30 days or available data
	days := m.analytics.DailyVisits
	if len(days) > sparklineMaxDays {
		days = days[len(days)-sparklineMaxDays:]
	}

	for _, d := range days {
		// Scale to block index (0-7)
		idx := int(float64(d.Visits) / float64(maxVisits) * sparklineMaxIndex)
		if idx > sparklineMaxIndex {
			idx = sparklineMaxIndex
		}
		result += string(blocks[idx])
	}

	return lipgloss.NewStyle().Foreground(m.theme.Accent).Render(result)
}

// formatNumber formats a number for display.
func formatNumber(n int64) string {
	const (
		formatMillion = 1_000_000
		formatThousand = 1_000
	)
	switch {
	case n >= formatMillion:
		return fmt.Sprintf("%.1fM", float64(n)/formatMillion)
	case n >= formatThousand:
		return fmt.Sprintf("%.1fK", float64(n)/formatThousand)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// Ensure interface compliance.
var _ tea.Model = (*StatsModel)(nil)

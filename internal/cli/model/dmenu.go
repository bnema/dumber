package model

import (
	"context"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
)

// DmenuModel is the Bubble Tea model for interactive dmenu.
type DmenuModel struct {
	// UI components
	list   list.Model
	search textinput.Model
	help   help.Model
	keys   styles.DmenuKeyMap

	// State
	allItems    []styles.HistoryItem
	selected    string // Selected URL
	searchQuery string
	width       int
	height      int
	err         error

	// Dependencies
	historyUC *usecase.SearchHistoryUseCase
	theme     *styles.Theme
}

// NewDmenuModel creates a new dmenu model.
func NewDmenuModel(theme *styles.Theme, historyUC *usecase.SearchHistoryUseCase) DmenuModel {
	// Create search input
	search := styles.NewSearchInput(theme)
	search.Focus()

	// Create help
	helpModel := styles.NewStyledHelp(theme)

	// Create keybindings
	keys := styles.DefaultDmenuKeyMap()

	return DmenuModel{
		search:    search,
		help:      helpModel,
		keys:      keys,
		historyUC: historyUC,
		theme:     theme,
		width:     80,
		height:    24,
	}
}

// dmenuLoadedMsg is sent when entries are loaded.
type dmenuLoadedMsg struct {
	items []styles.HistoryItem
	err   error
}

// Init implements tea.Model.
func (m DmenuModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.loadEntries,
	)
}

// loadEntries loads history entries.
func (m DmenuModel) loadEntries() tea.Msg {
	ctx := context.Background()

	entries, err := m.historyUC.GetRecent(ctx, 500, 0)
	if err != nil {
		return dmenuLoadedMsg{err: err}
	}

	items := make([]styles.HistoryItem, len(entries))
	for i, e := range entries {
		items[i] = styles.HistoryItem{
			ID:          e.ID,
			URL:         e.URL,
			Title:       e.Title,
			Domain:      getDomain(e.URL),
			VisitCount:  int(e.VisitCount),
			LastVisited: e.LastVisited,
		}
	}

	return dmenuLoadedMsg{items: items}
}

// Update implements tea.Model.
func (m DmenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.updateList()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Cancel):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Open):
			// Select current item and quit
			if item := m.list.SelectedItem(); item != nil {
				if hi, ok := item.(styles.HistoryItem); ok {
					m.selected = hi.URL
				}
			}
			return m, tea.Quit

		case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.Down):
			// Pass to list
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			cmds = append(cmds, cmd)

		default:
			// Update search input
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			cmds = append(cmds, cmd)

			// Filter list based on search
			if m.search.Value() != m.searchQuery {
				m.searchQuery = m.search.Value()
				m.updateList()
			}
		}

	case dmenuLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.allItems = msg.items
			m.updateList()
		}
	}

	return m, tea.Batch(cmds...)
}

// updateList updates the list with filtered items.
func (m *DmenuModel) updateList() {
	items := m.allItems

	// Apply search filter
	if m.searchQuery != "" {
		query := m.searchQuery
		filtered := make([]styles.HistoryItem, 0)
		for _, item := range items {
			// Simple case-insensitive contains matching
			if containsIgnoreCase(item.Title, query) || containsIgnoreCase(item.URL, query) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	// Create list
	listHeight := m.height - 6 // Account for search, help
	if listHeight < 5 {
		listHeight = 5
	}

	m.list = styles.NewHistoryList(m.theme, items, m.width, listHeight)
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)

	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

// toLower converts string to lowercase without importing strings.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// View implements tea.Model.
func (m DmenuModel) View() string {
	t := m.theme

	// Search bar
	searchBar := t.InputFocused.Render(m.search.View())

	// List
	listView := m.list.View()

	if m.err != nil {
		listView = t.ErrorStyle.Render("Error: " + m.err.Error())
	}

	// Help
	helpView := m.help.View(m.keys)

	// Combine
	return lipgloss.JoinVertical(
		lipgloss.Left,
		searchBar,
		"",
		listView,
		"",
		helpView,
	)
}

// SelectedURL returns the URL selected by the user.
func (m DmenuModel) SelectedURL() string {
	return m.selected
}

// Ensure interface compliance.
var _ tea.Model = (*DmenuModel)(nil)

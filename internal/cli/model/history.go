// Package model provides Bubble Tea models for CLI commands.
package model

import (
	"context"
	"net/url"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// HistoryModel is the Bubble Tea model for interactive history browser.
type HistoryModel struct {
	// UI components
	tabs    styles.TabsModel
	list    list.Model
	search  textinput.Model
	help    help.Model
	keys    styles.HistoryKeyMap
	cleanup *styles.CleanupModel
	confirm *styles.ConfirmModel

	// State
	entries      [][]*entity.HistoryEntry // Entries grouped by tab (Today/Yesterday/Week/Older)
	topDomains   []*entity.DomainStat
	domainFilter string
	searchMode   bool
	showHelp     bool
	width        int
	height       int
	err          error

	// Dependencies
	ctx       context.Context
	historyUC *usecase.SearchHistoryUseCase
	theme     *styles.Theme
}

// NewHistoryModel creates a new history browser model.
func NewHistoryModel(ctx context.Context, theme *styles.Theme, historyUC *usecase.SearchHistoryUseCase) HistoryModel {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating history model")

	// Create tabs for timeline grouping
	tabs := styles.TimelineTabs(theme)

	// Create search input
	search := styles.NewSearchInput(theme)

	// Create help
	helpModel := styles.NewStyledHelp(theme)

	// Create keybindings
	keys := styles.DefaultHistoryKeyMap()

	return HistoryModel{
		tabs:      tabs,
		search:    search,
		help:      helpModel,
		keys:      keys,
		entries:   make([][]*entity.HistoryEntry, 4), // Today, Yesterday, This Week, Older
		ctx:       ctx,
		historyUC: historyUC,
		theme:     theme,
		width:     80,
		height:    24,
	}
}

// Init implements tea.Model.
func (m HistoryModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadHistory,
		m.loadDomainStats,
	)
}

// historyLoadedMsg is sent when history entries are loaded.
type historyLoadedMsg struct {
	entries [][]*entity.HistoryEntry
	err     error
}

// domainStatsMsg is sent when domain stats are loaded.
type domainStatsMsg struct {
	domains []*entity.DomainStat
	err     error
}

// historyDeletedMsg is sent when an entry is deleted.
type historyDeletedMsg struct {
	err error
}

// historyCleanedMsg is sent when history is cleaned up.
type historyCleanedMsg struct {
	err error
}

// loadHistory loads history entries grouped by time category.
func (m HistoryModel) loadHistory() tea.Msg {
	log := logging.FromContext(m.ctx)
	log.Debug().Msg("loading history entries")

	// Get recent entries (all)
	entries, err := m.historyUC.GetRecent(m.ctx, 1000, 0)
	if err != nil {
		log.Error().Err(err).Msg("failed to load history")
		return historyLoadedMsg{err: err}
	}

	log.Debug().Int("total_count", len(entries)).Msg("loaded history entries from database")

	// Group by time category
	grouped := make([][]*entity.HistoryEntry, 4)
	for i := range grouped {
		grouped[i] = make([]*entity.HistoryEntry, 0)
	}

	for _, entry := range entries {
		cat := styles.GetTimeCategory(entry.LastVisited)
		switch cat {
		case styles.TimeCategoryToday:
			grouped[0] = append(grouped[0], entry)
		case styles.TimeCategoryYesterday:
			grouped[1] = append(grouped[1], entry)
		case styles.TimeCategoryThisWeek:
			grouped[2] = append(grouped[2], entry)
		case styles.TimeCategoryOlder:
			grouped[3] = append(grouped[3], entry)
		}
	}

	log.Debug().
		Int("today", len(grouped[0])).
		Int("yesterday", len(grouped[1])).
		Int("this_week", len(grouped[2])).
		Int("older", len(grouped[3])).
		Msg("grouped history entries by time category")

	return historyLoadedMsg{entries: grouped}
}

// loadDomainStats loads top domain statistics.
func (m HistoryModel) loadDomainStats() tea.Msg {
	log := logging.FromContext(m.ctx)
	domains, err := m.historyUC.GetDomainStats(m.ctx, 10)
	if err != nil {
		log.Error().Err(err).Msg("failed to load domain stats")
		return domainStatsMsg{err: err}
	}
	log.Debug().Int("domain_count", len(domains)).Msg("loaded domain statistics")
	return domainStatsMsg{domains: domains}
}

// Update implements tea.Model.
func (m HistoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle cleanup modal
	if m.cleanup != nil {
		cleanup, cmd := m.cleanup.Update(msg)
		m.cleanup = &cleanup
		if m.cleanup.Done() {
			if m.cleanup.Confirmed {
				// Perform cleanup
				cmds = append(cmds, m.performCleanup(m.cleanup.SelectedRange()))
			}
			m.cleanup = nil
		}
		return m, cmd
	}

	// Handle confirm dialog
	if m.confirm != nil {
		confirm, cmd := m.confirm.Update(msg)
		m.confirm = &confirm
		if m.confirm.Done() {
			if m.confirm.Result() {
				// Perform the confirmed action (delete current entry)
				cmds = append(cmds, m.deleteCurrentEntry())
			}
			m.confirm = nil
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.updateList()

	case tea.KeyMsg:
		// Search mode handling
		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.search.Blur()
				return m, nil
			case "enter":
				m.searchMode = false
				m.search.Blur()
				// Perform search
				return m, m.performSearch(m.search.Value())
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				return m, cmd
			}
		}

		// Normal mode handling
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp

		case key.Matches(msg, m.keys.Search):
			m.searchMode = true
			m.search.Focus()
			return m, textinput.Blink

		case key.Matches(msg, m.keys.Tab1):
			m.tabs.SetActive(0)
			m.updateList()

		case key.Matches(msg, m.keys.Tab2):
			m.tabs.SetActive(1)
			m.updateList()

		case key.Matches(msg, m.keys.Tab3):
			m.tabs.SetActive(2)
			m.updateList()

		case key.Matches(msg, m.keys.Tab4):
			m.tabs.SetActive(3)
			m.updateList()

		case key.Matches(msg, m.keys.NextTab):
			// Cycle to next tab (wraps around)
			next := (m.tabs.Active + 1) % 4
			m.tabs.SetActive(next)
			m.updateList()

		case key.Matches(msg, m.keys.PrevTab):
			// Cycle to previous tab (wraps around)
			prev := (m.tabs.Active + 3) % 4 // +3 is same as -1 mod 4
			m.tabs.SetActive(prev)
			m.updateList()

		case key.Matches(msg, m.keys.Open):
			// Open selected URL in browser
			if item := m.list.SelectedItem(); item != nil {
				if hi, ok := item.(styles.HistoryItem); ok {
					cmds = append(cmds, m.openURL(hi.URL))
				}
			}

		case key.Matches(msg, m.keys.Delete):
			// Show delete confirmation
			if item := m.list.SelectedItem(); item != nil {
				confirm := styles.NewConfirm(m.theme, "Delete this entry?")
				m.confirm = &confirm
			}

		case key.Matches(msg, m.keys.DeleteDomain):
			// Delete all from domain
			if item := m.list.SelectedItem(); item != nil {
				if hi, ok := item.(styles.HistoryItem); ok {
					confirm := styles.NewConfirm(m.theme, "Delete all from "+hi.Domain+"?")
					m.confirm = &confirm
				}
			}

		case key.Matches(msg, m.keys.Cleanup):
			// Show cleanup modal
			cleanup := styles.NewCleanup(m.theme)
			m.cleanup = &cleanup

		case key.Matches(msg, m.keys.Filter):
			// Cycle through domain filters
			m.cycleDomainFilter()
			m.updateList()

		default:
			// Pass to list
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			cmds = append(cmds, cmd)
		}

	case historyLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.entries = msg.entries
			m.updateList()
		}

	case domainStatsMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.topDomains = msg.domains
		}

	case historyDeletedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			// Reload history
			cmds = append(cmds, m.loadHistory)
		}

	case historyCleanedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			// Reload history
			cmds = append(cmds, m.loadHistory)
		}
	}

	return m, tea.Batch(cmds...)
}

// updateList updates the list with entries for the current tab.
func (m *HistoryModel) updateList() {
	activeTab := m.tabs.Active
	if activeTab < 0 || activeTab >= len(m.entries) {
		return
	}

	entries := m.entries[activeTab]

	// Apply domain filter if set
	if m.domainFilter != "" {
		filtered := make([]*entity.HistoryEntry, 0)
		for _, e := range entries {
			if getDomain(e.URL) == m.domainFilter {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Convert to list items
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

	// Create new list
	listHeight := m.height - 8 // Account for tabs, search, help
	if listHeight < 5 {
		listHeight = 5
	}

	m.list = styles.NewHistoryList(m.theme, items, m.width, listHeight)
}

// cycleDomainFilter cycles through domain filters.
func (m *HistoryModel) cycleDomainFilter() {
	if len(m.topDomains) == 0 {
		m.domainFilter = ""
		return
	}

	// Find current filter in list
	currentIdx := -1
	for i, d := range m.topDomains {
		if d.Domain == m.domainFilter {
			currentIdx = i
			break
		}
	}

	// Move to next, or clear filter
	if currentIdx == len(m.topDomains)-1 {
		m.domainFilter = "" // Clear filter
	} else {
		m.domainFilter = m.topDomains[currentIdx+1].Domain
	}
}

// performSearch executes a fuzzy search.
func (m HistoryModel) performSearch(query string) tea.Cmd {
	return func() tea.Msg {
		log := logging.FromContext(m.ctx)
		log.Debug().Str("query", query).Msg("performing search")

		result, err := m.historyUC.Search(m.ctx, usecase.SearchInput{
			Query: query,
			Limit: 100,
		})
		if err != nil {
			log.Error().Err(err).Msg("search failed")
			return historyLoadedMsg{err: err}
		}

		log.Debug().Int("matches", len(result.Matches)).Msg("search completed")

		// Convert matches to single list (all in "Today" tab for search results)
		entries := make([]*entity.HistoryEntry, len(result.Matches))
		for i, match := range result.Matches {
			entries[i] = match.Entry
		}

		grouped := make([][]*entity.HistoryEntry, 4)
		grouped[0] = entries // Put all results in first tab
		for i := 1; i < 4; i++ {
			grouped[i] = make([]*entity.HistoryEntry, 0)
		}

		return historyLoadedMsg{entries: grouped}
	}
}

// deleteCurrentEntry deletes the currently selected entry.
func (m HistoryModel) deleteCurrentEntry() tea.Cmd {
	return func() tea.Msg {
		if item := m.list.SelectedItem(); item != nil {
			if hi, ok := item.(styles.HistoryItem); ok {
				log := logging.FromContext(m.ctx)
				log.Debug().Int64("id", hi.ID).Str("url", hi.URL).Msg("deleting history entry")
				err := m.historyUC.Delete(m.ctx, hi.ID)
				if err != nil {
					log.Error().Err(err).Msg("failed to delete entry")
				}
				return historyDeletedMsg{err: err}
			}
		}
		return historyDeletedMsg{}
	}
}

// performCleanup cleans up history based on selected range.
func (m HistoryModel) performCleanup(cleanupRange styles.CleanupRange) tea.Cmd {
	return func() tea.Msg {
		log := logging.FromContext(m.ctx)
		cutoff := cleanupRange.CutoffTime()

		var err error
		if cutoff.IsZero() {
			log.Info().Msg("clearing all history")
			err = m.historyUC.ClearAll(m.ctx)
		} else {
			log.Info().Time("cutoff", cutoff).Msg("clearing history older than cutoff")
			err = m.historyUC.ClearOlderThan(m.ctx, cutoff)
		}

		if err != nil {
			log.Error().Err(err).Msg("failed to cleanup history")
		}
		return historyCleanedMsg{err: err}
	}
}

// openURL opens a URL in the default browser.
func (m HistoryModel) openURL(urlStr string) tea.Cmd {
	return func() tea.Msg {
		// Use xdg-open on Linux
		_ = exec.Command("xdg-open", urlStr).Start()
		return nil
	}
}

// View implements tea.Model.
func (m HistoryModel) View() string {
	// Handle modals
	if m.cleanup != nil {
		return m.cleanup.View()
	}
	if m.confirm != nil {
		return m.confirm.View()
	}

	t := m.theme

	// Build tab counts
	counts := make([]int, 4)
	for i, entries := range m.entries {
		counts[i] = len(entries)
	}

	// Header with tabs
	header := m.tabs.ViewWithCounts(counts)

	// Search bar
	var searchBar string
	if m.searchMode {
		searchBar = t.InputFocused.Render(m.search.View())
	} else {
		if m.domainFilter != "" {
			searchBar = t.Subtle.Render("Filter: ") + t.Badge.Render(m.domainFilter) + t.Subtle.Render(" (f to cycle)")
		} else {
			searchBar = t.Subtle.Render("Press / to search, f to filter by domain, Tab/Shift+Tab to switch tabs")
		}
	}

	// List
	listView := m.list.View()

	// Error display
	if m.err != nil {
		listView = t.ErrorStyle.Render("Error: " + m.err.Error())
	}

	// Help
	var helpView string
	if m.showHelp {
		helpView = m.help.View(m.keys)
	} else {
		helpView = t.Subtle.Render("? for help • q to quit")
	}

	// Combine all parts
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		searchBar,
		"",
		listView,
		"",
		helpView,
	)
}

// getDomain extracts domain from URL.
func getDomain(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	return u.Host
}

// HistoryListModel is a simpler non-interactive model for JSON output.
type HistoryListModel struct {
	entries []*entity.HistoryEntry
	max     int
	err     error

	ctx       context.Context
	historyUC *usecase.SearchHistoryUseCase
}

// NewHistoryListModel creates a model for list output.
func NewHistoryListModel(ctx context.Context, historyUC *usecase.SearchHistoryUseCase, maxEntries int) HistoryListModel {
	return HistoryListModel{
		ctx:       ctx,
		historyUC: historyUC,
		max:       maxEntries,
	}
}

// Init implements tea.Model.
func (m HistoryListModel) Init() tea.Cmd {
	return func() tea.Msg {
		log := logging.FromContext(m.ctx)
		log.Debug().Int("max", m.max).Msg("loading history for JSON output")

		entries, err := m.historyUC.GetRecent(m.ctx, m.max, 0)
		if err != nil {
			log.Error().Err(err).Msg("failed to load history")
			return historyLoadedMsg{err: err}
		}

		log.Debug().Int("count", len(entries)).Msg("loaded history entries")

		grouped := make([][]*entity.HistoryEntry, 1)
		grouped[0] = entries
		return historyLoadedMsg{entries: grouped}
	}
}

// Update implements tea.Model.
func (m HistoryListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case historyLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else if len(msg.entries) > 0 {
			m.entries = msg.entries[0]
		}
		return m, tea.Quit
	}
	return m, nil
}

// View implements tea.Model.
func (m HistoryListModel) View() string {
	return "" // Output handled externally
}

// Entries returns the loaded entries.
func (m HistoryListModel) Entries() []*entity.HistoryEntry {
	return m.entries
}

// Error returns any error that occurred.
func (m HistoryListModel) Error() error {
	return m.err
}

// Ensure interface compliance at compile time.
var _ tea.Model = (*HistoryModel)(nil)
var _ tea.Model = (*HistoryListModel)(nil)

// Unexported helper to avoid exposing time package in imports when not needed.
var _ = time.Now

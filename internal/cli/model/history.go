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
	if m.cleanup != nil {
		return m.handleCleanupModal(msg)
	}
	if m.confirm != nil {
		return m.handleConfirmModal(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case historyLoadedMsg:
		return m.handleHistoryLoaded(msg)

	case domainStatsMsg:
		return m.handleDomainStats(msg)

	case historyDeletedMsg:
		return m.handleHistoryDeleted(msg)

	case historyCleanedMsg:
		return m.handleHistoryCleaned(msg)
	}

	return m, nil
}

func (m HistoryModel) handleCleanupModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	cleanup, cmd := m.cleanup.Update(msg)
	m.cleanup = &cleanup
	if m.cleanup.Done() {
		if m.cleanup.Confirmed {
			cmd = m.performCleanup(m.cleanup.SelectedRange())
		}
		m.cleanup = nil
	}
	return m, cmd
}

func (m HistoryModel) handleConfirmModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	confirm, cmd := m.confirm.Update(msg)
	m.confirm = &confirm
	if m.confirm.Done() {
		if m.confirm.Result() {
			cmd = m.deleteCurrentEntry()
		}
		m.confirm = nil
	}
	return m, cmd
}

func (m HistoryModel) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.help.Width = msg.Width
	m.updateList()
	return m, nil
}

func (m HistoryModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searchMode {
		return m.handleSearchKey(msg)
	}
	return m.handleNormalKey(msg)
}

func (m HistoryModel) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.search.Blur()
		return m, nil
	case "enter":
		m.searchMode = false
		m.search.Blur()
		return m, m.performSearch(m.search.Value())
	default:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
}

func (m HistoryModel) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if updated, cmd, handled := m.handleGlobalKeys(msg); handled {
		return updated, cmd
	}
	if updated, handled := m.handleTabSwitchKeys(msg); handled {
		return updated, nil
	}
	if updated, cmd, handled := m.handleEntryActionKeys(msg); handled {
		return updated, cmd
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m HistoryModel) handleGlobalKeys(msg tea.KeyMsg) (HistoryModel, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit, true
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		return m, nil, true
	case key.Matches(msg, m.keys.Search):
		m.searchMode = true
		m.search.Focus()
		return m, textinput.Blink, true
	default:
		return m, nil, false
	}
}

func (m HistoryModel) handleTabSwitchKeys(msg tea.KeyMsg) (HistoryModel, bool) {
	switch {
	case key.Matches(msg, m.keys.Tab1):
		m.tabs.SetActive(0)
	case key.Matches(msg, m.keys.Tab2):
		m.tabs.SetActive(1)
	case key.Matches(msg, m.keys.Tab3):
		m.tabs.SetActive(2)
	case key.Matches(msg, m.keys.Tab4):
		m.tabs.SetActive(3)
	case key.Matches(msg, m.keys.NextTab):
		next := (m.tabs.Active + 1) % 4
		m.tabs.SetActive(next)
	case key.Matches(msg, m.keys.PrevTab):
		prev := (m.tabs.Active + 3) % 4 // +3 is same as -1 mod 4
		m.tabs.SetActive(prev)
	default:
		return m, false
	}

	m.updateList()
	return m, true
}

func (m HistoryModel) handleEntryActionKeys(msg tea.KeyMsg) (HistoryModel, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Open):
		if item := m.list.SelectedItem(); item != nil {
			if hi, ok := item.(styles.HistoryItem); ok {
				return m, m.openURL(hi.URL), true
			}
		}
		return m, nil, true
	case key.Matches(msg, m.keys.Delete):
		if m.list.SelectedItem() != nil {
			confirm := styles.NewConfirm(m.theme, "Delete this entry?")
			m.confirm = &confirm
		}
		return m, nil, true
	case key.Matches(msg, m.keys.DeleteDomain):
		if item := m.list.SelectedItem(); item != nil {
			if hi, ok := item.(styles.HistoryItem); ok {
				confirm := styles.NewConfirm(m.theme, "Delete all from "+hi.Domain+"?")
				m.confirm = &confirm
			}
		}
		return m, nil, true
	case key.Matches(msg, m.keys.Cleanup):
		cleanup := styles.NewCleanup(m.theme)
		m.cleanup = &cleanup
		return m, nil, true
	case key.Matches(msg, m.keys.Filter):
		m.cycleDomainFilter()
		m.updateList()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m HistoryModel) handleHistoryLoaded(msg historyLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	m.entries = msg.entries
	m.updateList()
	return m, nil
}

func (m HistoryModel) handleDomainStats(msg domainStatsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	m.topDomains = msg.domains
	return m, nil
}

func (m HistoryModel) handleHistoryDeleted(msg historyDeletedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	return m, m.loadHistory
}

func (m HistoryModel) handleHistoryCleaned(msg historyCleanedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	return m, m.loadHistory
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
		log := logging.FromContext(m.ctx)
		log.Debug().Str("url", urlStr).Msg("opening URL in browser")

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
	if m.err != nil {
		return ""
	}
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

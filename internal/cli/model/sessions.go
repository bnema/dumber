// Package model provides Bubble Tea models for CLI commands.
package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
	"github.com/bnema/dumber/internal/logging"
)

// SessionsModel is the Bubble Tea model for interactive session browser.
type SessionsModel struct {
	// UI components
	help    help.Model
	keys    sessionsKeyMap
	confirm *styles.ConfirmModel

	// State
	sessions       []entity.SessionInfo
	selectedIdx    int
	expandedIdx    int // -1 means none expanded
	width          int
	height         int
	err            error
	statusMessage  string
	currentSession entity.SessionID

	// Config
	maxListedSessions int

	// Dependencies
	ctx             context.Context
	listSessionsUC  *usecase.ListSessionsUseCase
	restoreUC       *usecase.RestoreSessionUseCase
	deleteSessionUC *usecase.DeleteSessionUseCase
	theme           *styles.Theme
}

// sessionsKeyMap defines keybindings for the sessions browser.
type sessionsKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Expand  key.Binding
	Restore key.Binding
	Delete  key.Binding
	Refresh key.Binding
	Help    key.Binding
	Quit    key.Binding
}

// ShortHelp returns keybindings for the short help view.
func (k sessionsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Expand, k.Restore, k.Delete, k.Quit}
}

// FullHelp returns keybindings for the full help view.
func (k sessionsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Expand},
		{k.Restore, k.Delete, k.Refresh},
		{k.Help, k.Quit},
	}
}

func defaultSessionsKeyMap() sessionsKeyMap {
	return sessionsKeyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("↓/j", "down"),
		),
		Expand: key.NewBinding(
			key.WithKeys("enter", "tab"),
			key.WithHelp("enter", "expand/collapse"),
		),
		Restore: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "restore"),
		),
		Delete: key.NewBinding(
			key.WithKeys("x", "d"),
			key.WithHelp("x", "delete"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// SessionsModelConfig holds configuration for the sessions model.
type SessionsModelConfig struct {
	ListSessionsUC    *usecase.ListSessionsUseCase
	RestoreUC         *usecase.RestoreSessionUseCase
	DeleteSessionUC   *usecase.DeleteSessionUseCase
	CurrentSession    entity.SessionID
	MaxExitedSessions int
}

// NewSessionsModel creates a new sessions browser model.
func NewSessionsModel(ctx context.Context, theme *styles.Theme, cfg SessionsModelConfig) SessionsModel {
	maxListed := cfg.MaxExitedSessions
	if maxListed <= 0 {
		maxListed = 50 // fallback default
	}

	return SessionsModel{
		help:              help.New(),
		keys:              defaultSessionsKeyMap(),
		selectedIdx:       0,
		expandedIdx:       -1,
		width:             80,
		height:            24,
		maxListedSessions: maxListed,
		ctx:               ctx,
		listSessionsUC:    cfg.ListSessionsUC,
		restoreUC:         cfg.RestoreUC,
		deleteSessionUC:   cfg.DeleteSessionUC,
		currentSession:    cfg.CurrentSession,
		theme:             theme,
	}
}

// Init implements tea.Model.
func (m SessionsModel) Init() tea.Cmd {
	return m.loadSessions
}

// sessionsLoadedMsg is sent when sessions are loaded.
type sessionsLoadedMsg struct {
	sessions []entity.SessionInfo
	err      error
}

// sessionDeletedMsg is sent when a session is deleted.
type sessionDeletedMsg struct {
	sessionID entity.SessionID
	err       error
}

// sessionRestoredMsg is sent when a session restoration is triggered.
type sessionRestoredMsg struct {
	sessionID entity.SessionID
	err       error
}

func (m SessionsModel) loadSessions() tea.Msg {
	log := logging.FromContext(m.ctx)
	log.Debug().Msg("loading sessions")

	if m.listSessionsUC == nil {
		return sessionsLoadedMsg{err: fmt.Errorf("session management not available")}
	}

	output, err := m.listSessionsUC.Execute(m.ctx, m.currentSession, m.maxListedSessions)
	if err != nil {
		log.Error().Err(err).Msg("failed to load sessions")
		return sessionsLoadedMsg{err: err}
	}

	log.Debug().Int("count", len(output.Sessions)).Msg("loaded sessions")
	return sessionsLoadedMsg{sessions: output.Sessions}
}

// Update implements tea.Model.
func (m SessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle confirm modal
	if m.confirm != nil {
		return m.handleConfirmModal(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case sessionsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.sessions = msg.sessions
			m.err = nil
		}
		return m, nil

	case sessionDeletedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.statusMessage = fmt.Sprintf("Session %s deleted", msg.sessionID)
		}
		return m, m.loadSessions

	case sessionRestoredMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.statusMessage = fmt.Sprintf("Restoring session %s...", msg.sessionID)
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

func (m SessionsModel) handleConfirmModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	confirm, cmd := m.confirm.Update(msg)
	m.confirm = &confirm
	if m.confirm.Done() {
		if m.confirm.Result() {
			// User confirmed deletion
			if m.selectedIdx >= 0 && m.selectedIdx < len(m.sessions) {
				session := m.sessions[m.selectedIdx]
				cmd = m.deleteSession(session.Session.ID)
			}
		}
		m.confirm = nil
		return m, cmd
	}
	return m, cmd
}

func (m SessionsModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Up):
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.selectedIdx < len(m.sessions)-1 {
			m.selectedIdx++
		}
		return m, nil

	case key.Matches(msg, m.keys.Expand):
		if m.expandedIdx == m.selectedIdx {
			m.expandedIdx = -1 // collapse
		} else {
			m.expandedIdx = m.selectedIdx // expand
		}
		return m, nil

	case key.Matches(msg, m.keys.Restore):
		if m.selectedIdx >= 0 && m.selectedIdx < len(m.sessions) {
			session := m.sessions[m.selectedIdx]
			if session.IsCurrent {
				m.statusMessage = "Cannot restore current session"
				return m, nil
			}
			return m, m.restoreSession(session.Session.ID)
		}
		return m, nil

	case key.Matches(msg, m.keys.Delete):
		if m.selectedIdx >= 0 && m.selectedIdx < len(m.sessions) {
			session := m.sessions[m.selectedIdx]
			if session.IsCurrent || session.IsActive {
				m.statusMessage = "Cannot delete active session"
				return m, nil
			}
			confirm := styles.NewConfirm(m.theme, fmt.Sprintf("Delete session %s?", session.Session.ID))
			m.confirm = &confirm
		}
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		return m, m.loadSessions

	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	}

	return m, nil
}

func (m SessionsModel) deleteSession(sessionID entity.SessionID) tea.Cmd {
	return func() tea.Msg {
		if m.deleteSessionUC == nil {
			return sessionDeletedMsg{sessionID: sessionID, err: fmt.Errorf("delete session use case not available")}
		}

		err := m.deleteSessionUC.Execute(m.ctx, usecase.DeleteSessionInput{
			SessionID:        sessionID,
			CurrentSessionID: m.currentSession,
		})
		return sessionDeletedMsg{sessionID: sessionID, err: err}
	}
}

func (m SessionsModel) restoreSession(sessionID entity.SessionID) tea.Cmd {
	return func() tea.Msg {
		log := logging.FromContext(m.ctx)
		log.Info().Str("session_id", string(sessionID)).Msg("restoring session")

		if m.restoreUC == nil {
			return sessionRestoredMsg{sessionID: sessionID, err: fmt.Errorf("restore use case not available")}
		}

		// Validate the session has restorable state
		_, err := m.restoreUC.Execute(m.ctx, usecase.RestoreInput{SessionID: sessionID})
		if err != nil {
			return sessionRestoredMsg{sessionID: sessionID, err: err}
		}

		// Spawn a new dumber instance
		spawner := desktop.NewSessionSpawner(m.ctx)
		if err := spawner.SpawnWithSession(sessionID); err != nil {
			return sessionRestoredMsg{sessionID: sessionID, err: err}
		}

		return sessionRestoredMsg{sessionID: sessionID}
	}
}

// View implements tea.Model.
func (m SessionsModel) View() string {
	// Handle confirm modal
	if m.confirm != nil {
		return m.confirm.View()
	}

	t := m.theme
	var prefix strings.Builder

	// Header
	prefix.WriteString(m.renderHeader())
	prefix.WriteString("\n")

	// Error display
	if m.err != nil {
		prefix.WriteString("\n")
		prefix.WriteString(t.ErrorStyle.Render(fmt.Sprintf("%s Error: %v", styles.IconX, m.err)))
		prefix.WriteString("\n")
	}

	// Status message
	if m.statusMessage != "" {
		prefix.WriteString("\n")
		prefix.WriteString(t.Subtle.Render(m.statusMessage))
		prefix.WriteString("\n")
	}

	// Help
	helpView := m.help.View(m.keys)

	// Compute available height for list so the view never exceeds terminal height.
	// This prevents Bubble Tea from truncating the bottom of the UI when there
	// are many sessions.
	prefixH := lipgloss.Height(prefix.String())
	helpH := lipgloss.Height(helpView)
	listH := m.height - prefixH - 1 - helpH // one spacer line between list and help
	if listH < 1 {
		listH = 1
	}

	listView := m.renderSessionsList(listH)

	// Compose final output with a single spacer line above help.
	return strings.TrimRight(prefix.String(), "\n") + "\n" + listView + "\n" + helpView
}

func (m SessionsModel) renderHeader() string {
	t := m.theme

	iconStyle := lipgloss.NewStyle().Foreground(t.Accent)
	titleStyle := t.Title.MarginLeft(1)

	icon := iconStyle.Render(styles.IconSession)
	title := titleStyle.Render("Sessions")

	// Count stats
	var activeCount, exitedCount int
	for _, s := range m.sessions {
		if s.IsCurrent || s.IsActive {
			activeCount++
		} else {
			exitedCount++
		}
	}

	stats := t.Subtle.Render(fmt.Sprintf("  %s %d active  %s %d exited",
		styles.IconPlay, activeCount,
		styles.IconStop, exitedCount,
	))

	return icon + title + stats
}

func (m SessionsModel) renderSessionsList(maxHeight int) string {
	t := m.theme

	if maxHeight <= 0 {
		return ""
	}
	if len(m.sessions) == 0 {
		return t.Subtle.Render("  No saved sessions found.")
	}

	// Determine starting index so the selected row stays visible even when
	// rows above it are expanded and consume multiple lines.
	start := m.selectedIdx
	if start < 0 {
		start = 0
	}
	if start >= len(m.sessions) {
		start = len(m.sessions) - 1
	}
	used := m.rowHeight(start)
	for start > 0 {
		prevHeight := m.rowHeight(start - 1)
		if used+prevHeight > maxHeight {
			break
		}
		start--
		used += prevHeight
	}

	var b strings.Builder
	remaining := maxHeight

	for i := start; i < len(m.sessions) && remaining > 0; i++ {
		info := m.sessions[i]
		isSelected := i == m.selectedIdx
		isExpanded := i == m.expandedIdx

		b.WriteString(m.renderSessionRow(info, isSelected, isExpanded))
		remaining--
		if remaining <= 0 {
			break
		}

		// Render expanded details, but never exceed available height.
		if isExpanded && remaining > 0 {
			details := m.renderSessionDetails(info)
			details = truncateLines(details, remaining)
			if details != "" {
				b.WriteString("\n")
				b.WriteString(details)
				remaining -= lipgloss.Height(details)
			}
		}

		if remaining > 0 {
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m SessionsModel) rowHeight(idx int) int {
	if idx < 0 || idx >= len(m.sessions) {
		return 0
	}

	height := 1 // always render one row line
	if idx != m.expandedIdx {
		return height
	}

	info := m.sessions[idx]
	details := m.renderSessionDetails(info)
	if details == "" {
		return height
	}

	return height + lipgloss.Height(details)
}

func (m SessionsModel) renderSessionRow(info entity.SessionInfo, isSelected, isExpanded bool) string {
	t := m.theme

	// Cursor
	cursor := "  "
	if isSelected {
		cursor = t.Highlight.Render(styles.IconCursor + " ")
	}

	// Status icon
	var statusIcon string
	var statusStyle lipgloss.Style
	switch {
	case info.IsCurrent:
		statusIcon = styles.IconPlay
		statusStyle = lipgloss.NewStyle().Foreground(t.Accent)
	case info.IsActive:
		statusIcon = styles.IconPlay
		statusStyle = lipgloss.NewStyle().Foreground(t.Warning)
	default:
		statusIcon = styles.IconStop
		statusStyle = t.Subtle
	}

	// Session ID
	idStyle := t.Normal
	if isSelected {
		idStyle = t.Highlight
	}

	// Label for current/active
	label := ""
	if info.IsCurrent {
		label = t.Badge.Render("current")
	} else if info.IsActive {
		label = t.BadgeMuted.Render("active")
	}

	// Expand indicator
	expandIcon := styles.IconExpand
	if isExpanded {
		expandIcon = styles.IconCollapse
	}
	expandStyle := t.Subtle

	// Tab/pane counts
	counts := t.Subtle.Render(fmt.Sprintf("%s %d  %s %d",
		styles.IconTab, info.TabCount,
		styles.IconPane, info.PaneCount,
	))

	// Time
	relTime := usecase.GetRelativeTime(info.UpdatedAt)
	timeStr := t.Subtle.Render(fmt.Sprintf("%s %s", styles.IconClock, relTime))

	// Build row
	row := fmt.Sprintf("%s%s %s %s  %s  %s  %s",
		cursor,
		statusStyle.Render(statusIcon),
		idStyle.Render(string(info.Session.ID)),
		label,
		expandStyle.Render(expandIcon),
		counts,
		timeStr,
	)

	return row
}

func (m SessionsModel) renderSessionDetails(info entity.SessionInfo) string {
	t := m.theme
	var b strings.Builder

	if info.State == nil {
		b.WriteString(t.Subtle.Render("      No state data available"))
		return b.String()
	}

	// Tree prefix styles
	treeStyle := lipgloss.NewStyle().Foreground(t.Border)
	leafStyle := lipgloss.NewStyle().Foreground(t.Muted)

	// Render tabs as tree
	for tabIdx, tab := range info.State.Tabs {
		isLastTab := tabIdx == len(info.State.Tabs)-1

		// Tab branch
		branch := "├── "
		if isLastTab {
			branch = "└── "
		}

		tabIcon := styles.IconTab
		if tab.IsPinned {
			tabIcon = styles.IconCheckboxChecked
		}

		tabName := tab.Name
		if tabName == "" {
			tabName = "Tab"
		}

		b.WriteString(fmt.Sprintf("      %s%s %s\n",
			treeStyle.Render(branch),
			leafStyle.Render(tabIcon),
			t.Normal.Render(tabName),
		))

		// Render panes under this tab
		m.renderPaneTree(&b, &tab.Workspace, isLastTab, t, treeStyle, leafStyle)
	}

	return b.String()
}

func truncateLines(s string, maxLines int) string {
	if maxLines <= 0 || s == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:maxLines], "\n")
}

func (m SessionsModel) renderPaneTree(
	b *strings.Builder,
	ws *entity.WorkspaceSnapshot,
	isLastTab bool,
	t *styles.Theme,
	treeStyle, leafStyle lipgloss.Style,
) {
	if ws == nil || ws.Root == nil {
		return
	}

	// Determine prefix for children
	childPrefix := "      │   "
	if isLastTab {
		childPrefix = "          "
	}

	m.renderPaneNode(b, ws.Root, childPrefix, true, t, treeStyle, leafStyle)
}

func (m SessionsModel) renderPaneNode(
	b *strings.Builder,
	node *entity.PaneNodeSnapshot,
	prefix string,
	isLast bool,
	t *styles.Theme,
	treeStyle, leafStyle lipgloss.Style,
) {
	if node == nil {
		return
	}

	branch := "├── "
	if isLast {
		branch = "└── "
	}

	if node.Pane != nil {
		// Leaf node (actual pane)
		pane := node.Pane
		title := pane.Title
		if title == "" {
			title = pane.URI
		}
		// Truncate long titles
		const maxTitleLen = 50
		if len(title) > maxTitleLen {
			title = title[:maxTitleLen-3] + "..."
		}

		fmt.Fprintf(b, "%s%s%s %s\n",
			prefix,
			treeStyle.Render(branch),
			leafStyle.Render(styles.IconPane),
			t.Subtle.Render(title),
		)
	} else if len(node.Children) > 0 {
		// Container node (split or stacked)
		splitIcon := styles.IconSessionStack
		splitType := "split"
		if node.IsStacked {
			splitType = "stacked"
		} else if node.SplitRatio > 0 {
			splitType = fmt.Sprintf("split %.0f%%", node.SplitRatio*100)
		}

		fmt.Fprintf(b, "%s%s%s %s\n",
			prefix,
			treeStyle.Render(branch),
			leafStyle.Render(splitIcon),
			t.Subtle.Render(splitType),
		)

		// Child prefix
		childPrefix := prefix + "│   "
		if isLast {
			childPrefix = prefix + "    "
		}

		// Render all children
		for i, child := range node.Children {
			isLastChild := i == len(node.Children)-1
			m.renderPaneNode(b, child, childPrefix, isLastChild, t, treeStyle, leafStyle)
		}
	}
}

// Ensure interface compliance at compile time.
var _ tea.Model = (*SessionsModel)(nil)

package styles

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/domain/entity"
)

// PurgeItem wraps entity.PurgeTarget with selection state for the UI.
type PurgeItem struct {
	entity.PurgeTarget
	Selected bool
}

// PurgeModel is the multi-select purge modal with sessions support.
type PurgeModel struct {
	Items        []PurgeItem
	Sessions     []entity.SessionPurgeItem
	SessionsSize int64 // Total size of all session snapshots
	Cursor       int
	Confirmed    bool
	Canceled     bool
	theme        *Theme

	// Track cursor position: false = items section, true = sessions section
	inSessionsSection bool
}

// PurgeKeyMap defines keybindings for purge modal.
type PurgeKeyMap struct {
	Up             key.Binding
	Down           key.Binding
	Toggle         key.Binding
	ToggleAll      key.Binding
	ToggleSessions key.Binding
	Confirm        key.Binding
	Cancel         key.Binding
}

// DefaultPurgeKeyMap returns default keybindings.
func DefaultPurgeKeyMap() PurgeKeyMap {
	return PurgeKeyMap{
		Up:             key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:           key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Toggle:         key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		ToggleAll:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle all")),
		ToggleSessions: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "toggle sessions")),
		Confirm:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Cancel:         key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "cancel")),
	}
}

// NewPurge creates a new purge modal with the given targets.
func NewPurge(theme *Theme, targets []entity.PurgeTarget) PurgeModel {
	items := make([]PurgeItem, 0, len(targets))
	for _, t := range targets {
		items = append(items, PurgeItem{PurgeTarget: t, Selected: t.Exists})
	}

	m := PurgeModel{Items: items, Cursor: 0, theme: theme}
	m.Cursor = m.firstSelectableIndex()
	return m
}

// NewPurgeWithSessions creates a new purge modal with targets and sessions.
func NewPurgeWithSessions(theme *Theme, targets []entity.PurgeTarget, sessions []entity.SessionPurgeItem, sessionsSize int64) PurgeModel {
	m := NewPurge(theme, targets)
	m.Sessions = sessions
	m.SessionsSize = sessionsSize
	return m
}

// Init implements tea.Model.
func (m PurgeModel) Init() tea.Cmd {
	_ = m
	return nil
}

// Update implements tea.Model.
func (m PurgeModel) Update(msg tea.Msg) (PurgeModel, tea.Cmd) {
	keys := DefaultPurgeKeyMap()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, keys.Toggle):
			m.toggleCurrent()
		case key.Matches(msg, keys.ToggleAll):
			m.toggleAll()
		case key.Matches(msg, keys.ToggleSessions):
			m.ToggleAllSessions()
		case key.Matches(msg, keys.Confirm):
			m.Confirmed = true
		case key.Matches(msg, keys.Cancel):
			m.Canceled = true
		}
	}

	return m, nil
}

// isDataSelected returns true if the Data target is selected.
func (m PurgeModel) isDataSelected() bool {
	for _, it := range m.Items {
		if it.Type == entity.PurgeTargetData && it.Selected {
			return true
		}
	}
	return false
}

// SessionsEnabled returns true if sessions can be individually selected.
// Sessions are disabled when Data is selected (database will be removed).
func (m PurgeModel) SessionsEnabled() bool {
	return !m.isDataSelected()
}

// totalSelectableCount returns the total number of selectable items across both sections.
func (m PurgeModel) totalSelectableCount() int {
	count := 0
	for _, it := range m.Items {
		if it.Exists {
			count++
		}
	}
	if m.SessionsEnabled() {
		count += len(m.Sessions)
	}
	return count
}

func (m *PurgeModel) moveCursor(delta int) {
	total := m.totalSelectableCount()
	if total == 0 {
		return
	}

	// Build a flat list of selectable indices
	// Items first, then sessions (if enabled)
	type position struct {
		inSessions bool
		index      int
	}
	var positions []position

	for i, it := range m.Items {
		if it.Exists {
			positions = append(positions, position{inSessions: false, index: i})
		}
	}
	if m.SessionsEnabled() {
		for i := range m.Sessions {
			positions = append(positions, position{inSessions: true, index: i})
		}
	}

	if len(positions) == 0 {
		return
	}

	// Find current position in the flat list
	currentFlat := 0
	for i, p := range positions {
		if p.inSessions == m.inSessionsSection && p.index == m.Cursor {
			currentFlat = i
			break
		}
	}

	// Move in the flat list
	newFlat := (currentFlat + delta + len(positions)) % len(positions)
	m.inSessionsSection = positions[newFlat].inSessions
	m.Cursor = positions[newFlat].index
}

func (m PurgeModel) firstSelectableIndex() int {
	for i, it := range m.Items {
		if it.Exists {
			return i
		}
	}
	return 0
}

func (m *PurgeModel) toggleCurrent() {
	if m.inSessionsSection {
		if !m.SessionsEnabled() {
			return
		}
		if m.Cursor >= 0 && m.Cursor < len(m.Sessions) {
			m.Sessions[m.Cursor].Selected = !m.Sessions[m.Cursor].Selected
		}
		return
	}

	if m.Cursor < 0 || m.Cursor >= len(m.Items) {
		return
	}
	if !m.Items[m.Cursor].Exists {
		return
	}
	m.Items[m.Cursor].Selected = !m.Items[m.Cursor].Selected
}

func (m *PurgeModel) toggleAll() {
	// Toggle all items
	anyUnselected := false
	for _, it := range m.Items {
		if it.Exists && !it.Selected {
			anyUnselected = true
			break
		}
	}
	// Also check sessions if enabled
	if m.SessionsEnabled() {
		for _, s := range m.Sessions {
			if !s.Selected {
				anyUnselected = true
				break
			}
		}
	}

	for i := range m.Items {
		if !m.Items[i].Exists {
			continue
		}
		m.Items[i].Selected = anyUnselected
	}
	if m.SessionsEnabled() {
		for i := range m.Sessions {
			m.Sessions[i].Selected = anyUnselected
		}
	}
}

// ToggleAllSessions toggles the selection state of all sessions.
func (m *PurgeModel) ToggleAllSessions() {
	if !m.SessionsEnabled() || len(m.Sessions) == 0 {
		return
	}

	anyUnselected := false
	for _, s := range m.Sessions {
		if !s.Selected {
			anyUnselected = true
			break
		}
	}

	for i := range m.Sessions {
		m.Sessions[i].Selected = anyUnselected
	}
}

// View implements tea.Model.
func (m PurgeModel) View() string {
	t := m.theme

	header := t.Title.Render(fmt.Sprintf("%s Purge", IconTrash))
	subtitle := t.Subtle.Render("Select items to remove")

	// Application Data section
	dataSectionHeader := t.Subtle.Render("── Application Data ──")
	var itemRows []string
	for i, it := range m.Items {
		itemRows = append(itemRows, m.renderItemRow(i, it))
	}
	itemsList := lipgloss.JoinVertical(lipgloss.Left, itemRows...)

	// Sessions section
	var sessionsList string
	if len(m.Sessions) > 0 {
		sessionsEnabled := m.SessionsEnabled()
		var sessionHeader string
		if sessionsEnabled {
			sessionHeader = t.Subtle.Render(fmt.Sprintf("── Inactive Sessions (%d total, %s) ──",
				len(m.Sessions), formatSize(m.SessionsSize)))
		} else {
			sessionHeader = t.Subtle.Render("── Inactive Sessions (database will be removed) ──")
		}

		var sessionRows []string
		sessionRows = append(sessionRows, sessionHeader)
		for i, s := range m.Sessions {
			sessionRows = append(sessionRows, m.renderSessionRow(i, s, sessionsEnabled))
		}
		sessionsList = lipgloss.JoinVertical(lipgloss.Left, sessionRows...)
	}

	selectedCount := m.SelectedCount()
	size := m.SelectedSize()
	var summary string
	if selectedCount > 0 {
		summary = lipgloss.JoinHorizontal(
			lipgloss.Left,
			t.WarningStyle.Render(IconWarning),
			" ",
			t.Subtle.Render(fmt.Sprintf("%d selected", selectedCount)),
			" ",
			t.Subtle.Render(fmt.Sprintf("(%s)", formatSize(size))),
		)
	} else {
		summary = t.Subtle.Render("0 selected")
	}

	helpText := "↑/↓ j/k move • space toggle • a all"
	if len(m.Sessions) > 0 && m.SessionsEnabled() {
		helpText += " • s sessions"
	}
	helpText += " • enter • esc"
	help := t.Subtle.Render(helpText)

	contentParts := []string{
		header,
		subtitle,
		"",
		dataSectionHeader,
		itemsList,
	}
	if sessionsList != "" {
		contentParts = append(contentParts, "", sessionsList)
	}
	contentParts = append(contentParts, "", summary, "", help)

	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	return t.Box.Render(content)
}

func (m PurgeModel) renderItemRow(i int, it PurgeItem) string {
	t := m.theme

	cursor := "  "
	if !m.inSessionsSection && i == m.Cursor {
		cursor = IconCursor + " "
	}

	checkbox := IconCheckboxEmpty
	if it.Selected {
		checkbox = IconCheckboxChecked
	}

	label := purgeLabel(it.Type)

	pathStyle := t.Subtle
	labelStyle := t.Normal
	sizeStyle := t.Subtle
	checkboxStyle := lipgloss.NewStyle().Foreground(t.Accent)
	cursorStyle := lipgloss.NewStyle().Foreground(t.Accent)

	missing := ""
	if !it.Exists {
		checkbox = IconCheckboxEmpty
		checkboxStyle = t.Subtle
		cursorStyle = t.Subtle
		labelStyle = t.Subtle
		pathStyle = t.Subtle
		sizeStyle = t.Subtle
		missing = t.Subtle.Render("(not found)")
	}

	size := ""
	if it.Exists {
		size = sizeStyle.Render(formatSize(it.Size))
	}

	const labelPadWidth = 12
	left := lipgloss.JoinHorizontal(
		lipgloss.Left,
		cursorStyle.Render(cursor),
		checkboxStyle.Render(checkbox),
		" ",
		labelStyle.Render(padRight(label, labelPadWidth)),
		pathStyle.Render(it.Path),
	)

	if it.Exists {
		return lipgloss.JoinHorizontal(lipgloss.Left, left, " ", size)
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, left, " ", missing)
}

func (m PurgeModel) renderSessionRow(i int, s entity.SessionPurgeItem, enabled bool) string {
	t := m.theme

	cursor := "  "
	if m.inSessionsSection && i == m.Cursor && enabled {
		cursor = IconCursor + " "
	}

	checkbox := IconCheckboxEmpty
	checkboxStyle := t.Subtle
	cursorStyle := t.Subtle
	labelStyle := t.Subtle
	detailStyle := t.Subtle

	if enabled {
		if s.Selected {
			checkbox = IconCheckboxChecked
		}
		checkboxStyle = lipgloss.NewStyle().Foreground(t.Accent)
		cursorStyle = lipgloss.NewStyle().Foreground(t.Accent)
		labelStyle = t.Normal
		detailStyle = t.Subtle
	}

	// Format: ...abc1   2 tabs  3 panes   2d ago
	shortID := s.Info.Session.ShortID()
	tabLabel := "tab"
	if s.Info.TabCount != 1 {
		tabLabel = "tabs"
	}
	paneLabel := "pane"
	if s.Info.PaneCount != 1 {
		paneLabel = "panes"
	}

	const idPadWidth = 8
	const tabPadWidth = 8
	const panePadWidth = 9

	idPart := labelStyle.Render(padRight("..."+shortID, idPadWidth))
	tabPart := detailStyle.Render(padRight(fmt.Sprintf("%d %s", s.Info.TabCount, tabLabel), tabPadWidth))
	panePart := detailStyle.Render(padRight(fmt.Sprintf("%d %s", s.Info.PaneCount, paneLabel), panePadWidth))
	timePart := detailStyle.Render(getRelativeTime(s.Info.UpdatedAt))

	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		cursorStyle.Render(cursor),
		checkboxStyle.Render(checkbox),
		" ",
		idPart,
		tabPart,
		panePart,
		timePart,
	)
}

func purgeLabel(t entity.PurgeTargetType) string {
	switch t {
	case entity.PurgeTargetConfig:
		return fmt.Sprintf("%s Config", IconConfig)
	case entity.PurgeTargetData:
		return fmt.Sprintf("%s Data", IconDatabase)
	case entity.PurgeTargetState:
		return fmt.Sprintf("%s State", IconLogs)
	case entity.PurgeTargetCache:
		return fmt.Sprintf("%s Cache", IconCache)
	case entity.PurgeTargetFilterJSON:
		return fmt.Sprintf("%s Filter JSON", IconFilter)
	case entity.PurgeTargetFilterStore:
		return fmt.Sprintf("%s Filter Store", IconFilter)
	case entity.PurgeTargetFilterCache:
		return fmt.Sprintf("%s Filter Cache", IconFilter)
	case entity.PurgeTargetDesktopFile:
		return fmt.Sprintf("%s Desktop file", IconDesktop)
	case entity.PurgeTargetIcon:
		return fmt.Sprintf("%s Icon", IconImage)
	default:
		return "Item"
	}
}

func padRight(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(r))
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func getRelativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	const (
		hoursPerDay = 24
		daysPerWeek = 7
	)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case diff < hoursPerDay*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	case diff < daysPerWeek*hoursPerDay*time.Hour:
		days := int(diff.Hours() / hoursPerDay)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	default:
		weeks := int(diff.Hours() / hoursPerDay / daysPerWeek)
		if weeks == 1 {
			return "1w ago"
		}
		return fmt.Sprintf("%dw ago", weeks)
	}
}

// Done returns true if the modal is complete.
func (m PurgeModel) Done() bool {
	return m.Confirmed || m.Canceled
}

// SelectedTypes returns the selected target types.
func (m PurgeModel) SelectedTypes() []entity.PurgeTargetType {
	selected := make(map[entity.PurgeTargetType]struct{})
	for _, it := range m.Items {
		if it.Exists && it.Selected {
			selected[it.Type] = struct{}{}
		}
	}

	out := make([]entity.PurgeTargetType, 0, len(selected))
	for tt := range selected {
		out = append(out, tt)
	}
	return out
}

// SelectedCount returns the number of selected items (targets + sessions).
func (m PurgeModel) SelectedCount() int {
	count := 0
	for _, it := range m.Items {
		if it.Exists && it.Selected {
			count++
		}
	}
	// Only count sessions if they're enabled (Data not selected)
	if m.SessionsEnabled() {
		for _, s := range m.Sessions {
			if s.Selected {
				count++
			}
		}
	}
	return count
}

// SelectedSize returns the total size of selected items.
// When Data is selected, includes SessionsSize since DB will be removed.
func (m PurgeModel) SelectedSize() int64 {
	var total int64
	for _, it := range m.Items {
		if it.Exists && it.Selected {
			total += it.Size
		}
	}
	// If Data is selected, sessions are implicitly included
	if m.isDataSelected() {
		total += m.SessionsSize
	} else {
		// Otherwise, count only selected sessions
		// Note: individual session sizes aren't tracked, only total
		// When sessions are selected, we include the total size proportionally
		selectedCount := 0
		for _, s := range m.Sessions {
			if s.Selected {
				selectedCount++
			}
		}
		if len(m.Sessions) > 0 && selectedCount > 0 {
			total += m.SessionsSize * int64(selectedCount) / int64(len(m.Sessions))
		}
	}
	return total
}

// SelectedSessionIDs returns the IDs of selected sessions.
// Returns nil if Data is selected (sessions are deleted with the database).
func (m PurgeModel) SelectedSessionIDs() []entity.SessionID {
	if m.isDataSelected() {
		return nil
	}

	var ids []entity.SessionID
	for _, s := range m.Sessions {
		if s.Selected {
			ids = append(ids, s.Info.Session.ID)
		}
	}
	return ids
}

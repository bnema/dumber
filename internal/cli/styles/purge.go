package styles

import (
	"fmt"
	"strings"

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

// PurgeModel is the multi-select purge modal with simplified sessions support.
// Sessions are handled as a single "purge all" option instead of individual selection.
type PurgeModel struct {
	Items        []PurgeItem
	Sessions     []entity.SessionPurgeItem
	SessionsSize int64 // Total size of all session snapshots
	Cursor       int   // Flat cursor index across all rows
	Confirmed    bool
	Canceled     bool
	theme        *Theme

	// allSessionsSelected indicates if all sessions should be purged
	allSessionsSelected bool
}

// PurgeKeyMap defines keybindings for purge modal.
type PurgeKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Toggle    key.Binding
	ToggleAll key.Binding
	Confirm   key.Binding
	Cancel    key.Binding
}

// DefaultPurgeKeyMap returns default keybindings.
func DefaultPurgeKeyMap() PurgeKeyMap {
	return PurgeKeyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Toggle:    key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		ToggleAll: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle all")),
		Confirm:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Cancel:    key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "cancel")),
	}
}

// rowType indicates what kind of row we're dealing with
type rowType int

const (
	rowTypeItem rowType = iota
	rowTypeSessions
)

// row represents a single row in the flat list
type row struct {
	Type      rowType
	ItemIndex int  // index into Items array (only for rowTypeItem)
	Exists    bool // whether this row is selectable
}

// buildRows creates a flat list of rows in display order
func (m PurgeModel) buildRows() []row {
	var rows []row
	for i, it := range m.Items {
		rows = append(rows, row{Type: rowTypeItem, ItemIndex: i, Exists: it.Exists})
		// Insert sessions row right after Data
		if it.Type == entity.PurgeTargetData && m.hasSessionsItem() {
			rows = append(rows, row{Type: rowTypeSessions, Exists: m.SessionsEnabled()})
		}
	}
	return rows
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

// SessionsEnabled returns true if sessions can be selected.
// Sessions are disabled when Data is selected (database will be removed).
func (m PurgeModel) SessionsEnabled() bool {
	return !m.isDataSelected()
}

// hasSessionsItem returns true if there are sessions available to purge.
func (m PurgeModel) hasSessionsItem() bool {
	return len(m.Sessions) > 0
}

func (m *PurgeModel) moveCursor(delta int) {
	rows := m.buildRows()
	if len(rows) == 0 {
		return
	}

	// Build list of selectable row indices
	var selectableIndices []int
	for i, r := range rows {
		if r.Exists {
			selectableIndices = append(selectableIndices, i)
		}
	}

	if len(selectableIndices) == 0 {
		return
	}

	// Find current position in selectable list
	currentPos := 0
	for i, idx := range selectableIndices {
		if idx == m.Cursor {
			currentPos = i
			break
		}
	}

	// Move
	newPos := (currentPos + delta + len(selectableIndices)) % len(selectableIndices)
	m.Cursor = selectableIndices[newPos]
}

func (m PurgeModel) firstSelectableIndex() int {
	rows := m.buildRows()
	for i, r := range rows {
		if r.Exists {
			return i
		}
	}
	return 0
}

func (m *PurgeModel) toggleCurrent() {
	rows := m.buildRows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		return
	}

	r := rows[m.Cursor]
	if !r.Exists {
		return
	}

	switch r.Type {
	case rowTypeItem:
		m.Items[r.ItemIndex].Selected = !m.Items[r.ItemIndex].Selected
	case rowTypeSessions:
		m.allSessionsSelected = !m.allSessionsSelected
	}
}

func (m *PurgeModel) toggleAll() {
	// Check if any selectable item is unselected
	anyUnselected := false
	for _, it := range m.Items {
		if it.Exists && !it.Selected {
			anyUnselected = true
			break
		}
	}
	if m.hasSessionsItem() && m.SessionsEnabled() && !m.allSessionsSelected {
		anyUnselected = true
	}

	// Set all to the opposite state
	for i := range m.Items {
		if m.Items[i].Exists {
			m.Items[i].Selected = anyUnselected
		}
	}
	if m.hasSessionsItem() && m.SessionsEnabled() {
		m.allSessionsSelected = anyUnselected
	}
}

// ToggleAllSessions toggles the selection state of all sessions.
func (m *PurgeModel) ToggleAllSessions() {
	if !m.hasSessionsItem() || !m.SessionsEnabled() {
		return
	}
	m.allSessionsSelected = !m.allSessionsSelected
}

// View implements tea.Model.
func (m PurgeModel) View() string {
	t := m.theme

	header := t.Title.Render(fmt.Sprintf("%s Purge", IconTrash))
	subtitle := t.Subtle.Render("Select items to remove")

	// Build rows and render
	rows := m.buildRows()
	var itemRows []string
	for i, r := range rows {
		isCursor := i == m.Cursor
		switch r.Type {
		case rowTypeItem:
			itemRows = append(itemRows, m.renderItemRow(r.ItemIndex, m.Items[r.ItemIndex], isCursor))
		case rowTypeSessions:
			itemRows = append(itemRows, m.renderSessionsItem(isCursor))
		}
	}

	itemsList := lipgloss.JoinVertical(lipgloss.Left, itemRows...)

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

	helpText := "↑/↓ j/k move • space toggle • a all • enter • esc"
	help := t.Subtle.Render(helpText)

	contentParts := []string{
		header,
		subtitle,
		"",
		itemsList,
		"",
		summary,
		"",
		help,
	}

	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	return t.Box.Render(content)
}

func (m PurgeModel) renderItemRow(_ int, it PurgeItem, isCursor bool) string {
	t := m.theme

	cursor := "  "
	if isCursor && it.Exists {
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

// renderSessionsItem renders the single "Sessions" item.
func (m PurgeModel) renderSessionsItem(isCursor bool) string {
	t := m.theme
	enabled := m.SessionsEnabled()

	cursor := "  "
	if isCursor && enabled {
		cursor = IconCursor + " "
	}

	checkbox := IconCheckboxEmpty
	checkboxStyle := t.Subtle
	cursorStyle := t.Subtle
	labelStyle := t.Subtle
	detailStyle := t.Subtle

	if enabled {
		if m.allSessionsSelected {
			checkbox = IconCheckboxChecked
		}
		checkboxStyle = lipgloss.NewStyle().Foreground(t.Accent)
		cursorStyle = lipgloss.NewStyle().Foreground(t.Accent)
		labelStyle = t.Normal
	}

	// Format: Sessions (5 inactive, 12.3 MB) or (included with Data)
	label := fmt.Sprintf("%s Sessions", IconLogs)
	var detail string
	if enabled {
		detail = fmt.Sprintf("(%d inactive, %s)", len(m.Sessions), formatSize(m.SessionsSize))
	} else {
		detail = fmt.Sprintf("(%d inactive, included with Data)", len(m.Sessions))
	}

	const labelPadWidth = 12
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		cursorStyle.Render(cursor),
		checkboxStyle.Render(checkbox),
		" ",
		labelStyle.Render(padRight(label, labelPadWidth)),
		detailStyle.Render(detail),
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

// SelectedCount returns the number of selected items (targets + sessions as 1 if selected).
func (m PurgeModel) SelectedCount() int {
	count := 0
	for _, it := range m.Items {
		if it.Exists && it.Selected {
			count++
		}
	}
	// Count sessions as 1 item if selected (and enabled)
	if m.hasSessionsItem() && m.SessionsEnabled() && m.allSessionsSelected {
		count++
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
	} else if m.allSessionsSelected {
		// Sessions are explicitly selected
		total += m.SessionsSize
	}
	return total
}

// SelectedSessionIDs returns the IDs of all sessions if sessions are selected.
// Returns nil if Data is selected (sessions are deleted with the database)
// or if sessions are not selected.
func (m PurgeModel) SelectedSessionIDs() []entity.SessionID {
	if m.isDataSelected() {
		return nil
	}
	if !m.allSessionsSelected {
		return nil
	}

	ids := make([]entity.SessionID, 0, len(m.Sessions))
	for _, s := range m.Sessions {
		ids = append(ids, s.Info.Session.ID)
	}
	return ids
}

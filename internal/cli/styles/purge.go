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

// PurgeModel is the multi-select purge modal.
type PurgeModel struct {
	Items     []PurgeItem
	Cursor    int
	Confirmed bool
	Canceled  bool
	theme     *Theme
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

func (m *PurgeModel) moveCursor(delta int) {
	if len(m.Items) == 0 {
		return
	}

	idx := m.Cursor
	for i := 0; i < len(m.Items); i++ {
		idx = (idx + delta + len(m.Items)) % len(m.Items)
		if m.Items[idx].Exists {
			m.Cursor = idx
			return
		}
	}
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
	if m.Cursor < 0 || m.Cursor >= len(m.Items) {
		return
	}
	if !m.Items[m.Cursor].Exists {
		return
	}
	m.Items[m.Cursor].Selected = !m.Items[m.Cursor].Selected
}

func (m *PurgeModel) toggleAll() {
	anyUnselected := false
	for _, it := range m.Items {
		if it.Exists && !it.Selected {
			anyUnselected = true
			break
		}
	}

	for i := range m.Items {
		if !m.Items[i].Exists {
			continue
		}
		m.Items[i].Selected = anyUnselected
	}
}

// View implements tea.Model.
func (m PurgeModel) View() string {
	t := m.theme

	header := t.Title.Render(fmt.Sprintf("%s Purge", IconTrash))
	subtitle := t.Subtle.Render("Select items to remove")

	var rows []string
	for i, it := range m.Items {
		rows = append(rows, m.renderRow(i, it))
	}

	list := lipgloss.JoinVertical(lipgloss.Left, rows...)

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

	help := t.Subtle.Render("↑/↓ or j/k to move • space to toggle • a toggle all • enter confirm • esc cancel")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		subtitle,
		"",
		list,
		"",
		summary,
		"",
		help,
	)

	return t.Box.Render(content)
}

func (m PurgeModel) renderRow(i int, it PurgeItem) string {
	t := m.theme

	cursor := "  "
	if i == m.Cursor {
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

// SelectedCount returns the number of selected, existing items.
func (m PurgeModel) SelectedCount() int {
	count := 0
	for _, it := range m.Items {
		if it.Exists && it.Selected {
			count++
		}
	}
	return count
}

// SelectedSize returns the total size of selected, existing items.
func (m PurgeModel) SelectedSize() int64 {
	var total int64
	for _, it := range m.Items {
		if it.Exists && it.Selected {
			total += it.Size
		}
	}
	return total
}

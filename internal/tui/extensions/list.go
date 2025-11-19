package extensions

import (
	"fmt"
	"io"
	"sort"

	tuistyles "github.com/bnema/dumber/internal/tui"
	tuicomp "github.com/bnema/dumber/internal/tui/components"
	"github.com/bnema/dumber/internal/webext"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the interactive TUI for listing and toggling extensions.
type Model struct {
	list    list.Model
	manager *webext.Manager
	keys    keyMap
	header  tuicomp.Header
	help    tuicomp.Help
	status  string
	width   int
	height  int
	err     error
}

type keyMap struct {
	Toggle  key.Binding
	Refresh key.Binding
	Quit    key.Binding
	Filter  key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Toggle: key.NewBinding(
			key.WithKeys(" ", "enter"),
			key.WithHelp("space", "toggle enable"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

type toggleMsg struct {
	id      string
	enabled bool
	err     error
}

type refreshMsg []item

// NewModel returns a fully configured extensions list model.
func NewModel(manager *webext.Manager) *Model {
	items := ItemsFromManager(manager)
	keys := newKeyMap()

	l := tuicomp.NewList(items, 0, 0)
	l.SetDelegate(extensionDelegate{})
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Toggle, keys.Refresh, keys.Filter, keys.Quit}
	}

	return &Model{
		list:    l,
		manager: manager,
		keys:    keys,
		header:  tuicomp.Header{Title: fmt.Sprintf("Extensions (%d)", len(items))},
		help:    tuicomp.NewHelp([]key.Binding{keys.Toggle, keys.Refresh, keys.Filter, keys.Quit}),
		status:  "Loaded extensions",
		width:   80,
		height:  24,
	}
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeList()

	case toggleMsg:
		if msg.err != nil {
			m.status = tuistyles.ErrorStyle.Render(msg.err.Error())
			m.err = msg.err
			return m, nil
		}
		m.updateItemState(msg.id, msg.enabled)
		state := "disabled"
		if msg.enabled {
			state = "enabled"
		}
		m.status = fmt.Sprintf("%s %s", msg.id, state)

	case refreshMsg:
		items := make([]list.Item, 0, len(msg))
		for _, it := range msg {
			items = append(items, it)
		}
		m.list.SetItems(items)
		m.header.Title = fmt.Sprintf("Extensions (%d)", len(items))
		m.status = fmt.Sprintf("Refreshed (%d)", len(items))

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Toggle):
			return m, m.toggleSelected()
		case key.Matches(msg, m.keys.Refresh):
			return m, m.refreshCmd()
		case key.Matches(msg, m.keys.Filter):
			// Let list handle filter activation
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *Model) View() string {
	containerWidth := m.width
	header := lipgloss.NewStyle().Width(containerWidth).Render(m.header.View())
	// Don't set width on border - let it auto-size to content (list width is already m.width - 2)
	// Total rendered width will be (m.width - 2) + 2 borders = m.width
	body := tuistyles.SubtleBorder.Render(m.list.View())

	var status string
	if m.status != "" {
		status = lipgloss.NewStyle().
			Width(containerWidth).
			Render(tuistyles.MutedStyle.Render(m.status))
	}

	help := ""
	if hv := m.help.View(); hv != "" {
		help = lipgloss.NewStyle().Width(containerWidth).Render(hv)
	}

	parts := []string{header, body}
	if status != "" {
		parts = append(parts, status)
	}
	if help != "" {
		parts = append(parts, help)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) toggleSelected() tea.Cmd {
	if m.manager == nil {
		return nil
	}
	item, ok := m.list.SelectedItem().(item)
	if !ok {
		return nil
	}

	target := !item.enabled
	return func() tea.Msg {
		var err error
		if target {
			err = m.manager.Enable(item.id)
		} else {
			err = m.manager.Disable(item.id)
		}
		return toggleMsg{
			id:      item.id,
			enabled: target,
			err:     err,
		}
	}
}

func (m *Model) refreshCmd() tea.Cmd {
	if m.manager == nil {
		return nil
	}
	return func() tea.Msg {
		return refreshMsg(toItems(m.manager.ListExtensions(), m.manager))
	}
}

func (m *Model) updateItemState(id string, enabled bool) {
	items := m.list.Items()
	for i, li := range items {
		ext, ok := li.(item)
		if !ok {
			continue
		}
		if ext.id == id {
			ext.enabled = enabled
			items[i] = ext
			m.list.SetItems(items)
			break
		}
	}
}

// resizeList sets list size based on the current window, accounting for header/status/help and border.
func (m *Model) resizeList() {
	listWidth := m.width - 2 // border adds 1 per side
	if listWidth < 20 {
		listWidth = 20
	}

	// Rough height allocation subtracting header/help/status (single lines).
	listHeight := m.height - 4 // header + border top/bottom + status/help approx
	if m.status != "" {
		listHeight -= 1
	}
	if m.help.View() != "" {
		listHeight -= lipgloss.Height(m.help.View())
	}
	if listHeight < 5 {
		listHeight = 5
	}
	m.list.SetSize(listWidth, listHeight)
}

// item represents a single extension row for the TUI list.
type item struct {
	id            string
	name          string
	version       string
	latestVersion string
	path          string
	enabled       bool
	bundled       bool
}

func (e item) Title() string       { return e.name }
func (e item) Description() string { return e.path }
func (e item) FilterValue() string { return e.name + " " + e.id }

// ItemsFromManager converts manager extensions into list items sorted by bundled/name.
func ItemsFromManager(manager *webext.Manager) []list.Item {
	items := toListItems(manager)
	out := make([]list.Item, len(items))
	for i := range items {
		out[i] = items[i]
	}
	return out
}

// toListItems keeps the concrete type for internal rendering while ensuring stable ordering.
func toListItems(manager *webext.Manager) []item {
	if manager == nil {
		return nil
	}
	return toItems(manager.ListExtensions(), manager)
}

func toItems(exts []*webext.Extension, manager *webext.Manager) []item {
	sort.Slice(exts, func(i, j int) bool {
		if exts[i].Bundled != exts[j].Bundled {
			return exts[i].Bundled
		}
		ni := exts[i].ID
		nj := exts[j].ID
		if exts[i].Manifest != nil && exts[i].Manifest.Name != "" {
			ni = exts[i].Manifest.Name
		}
		if exts[j].Manifest != nil && exts[j].Manifest.Name != "" {
			nj = exts[j].Manifest.Name
		}
		return ni < nj
	})

	items := make([]item, 0, len(exts))
	for _, ext := range exts {
		name := ext.ID
		if ext.Manifest != nil && ext.Manifest.Name != "" {
			name = ext.Manifest.Name
		}
		version := ""
		if ext.Manifest != nil {
			version = ext.Manifest.Version
		}

		items = append(items, item{
			id:      ext.ID,
			name:    name,
			version: version,
			path:    ext.Path,
			enabled: manager.IsEnabled(ext.ID),
			bundled: ext.Bundled,
		})
	}
	return items
}

// Delegate to render extension rows with column layout.
type extensionDelegate struct{}

func (extensionDelegate) Height() int  { return 1 }
func (extensionDelegate) Spacing() int { return 0 }
func (extensionDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (extensionDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(item)
	if !ok {
		return
	}
	selected := index == m.Index()
	fmt.Fprint(w, renderRow(item, m.Width(), selected))
}

func renderRow(item item, width int, selected bool) string {
	idWidth, nameWidth, versionWidth, statusWidth, typeWidth, pathWidth := computeWidths(width)

	// Prepare plain text values (no styling yet)
	id := tuistyles.Truncate(item.id, idWidth)
	name := tuistyles.Truncate(nameWithIndicator(item), nameWidth)

	version := item.version
	if item.latestVersion != "" && item.latestVersion != item.version {
		version = fmt.Sprintf("%s ⬆ %s", item.version, item.latestVersion)
	}
	version = tuistyles.Truncate(version, versionWidth)

	path := tuistyles.TruncateMiddle(item.path, pathWidth)

	// Create badges - these are already styled but we need to ensure they fit
	status := tuistyles.Badge("Disabled", tuistyles.ColorBlack, tuistyles.ColorLightGray)
	if item.enabled {
		status = tuistyles.Badge("Enabled", tuistyles.ColorBlack, tuistyles.ColorVeryLight)
	}
	// Truncate badge if it exceeds allocated width
	if lipgloss.Width(status) > statusWidth {
		status = tuistyles.Truncate(status, statusWidth)
	}

	extType := tuistyles.Badge("User", tuistyles.ColorWhite, tuistyles.ColorDarkGray)
	if item.bundled {
		extType = tuistyles.Badge("Bundled", tuistyles.ColorWhite, tuistyles.ColorGray)
	}
	// Truncate badge if it exceeds allocated width
	if lipgloss.Width(extType) > typeWidth {
		extType = tuistyles.Truncate(extType, typeWidth)
	}

	// Build columns with pre-truncated values, applying only color/style (no Width here)
	columns := []string{
		lipgloss.NewStyle().Foreground(tuistyles.ColorLightGray).Render(id),
		lipgloss.NewStyle().Bold(true).Render(name),
		lipgloss.NewStyle().Foreground(tuistyles.ColorVeryLight).Render(version),
		status, // Badge is already fully styled
		extType, // Badge is already fully styled
		lipgloss.NewStyle().Foreground(tuistyles.ColorGray).Render(path),
	}

	style := tuistyles.BaseStyle
	if selected {
		style = tuistyles.SelectedStyle
	}

	// Columnize will apply Width() for padding and truncate if still needed
	return style.Render(tuistyles.Columnize(columns, []int{
		idWidth,
		nameWidth,
		versionWidth,
		statusWidth,
		typeWidth,
		pathWidth,
	}))
}

func nameWithIndicator(item item) string {
	if item.bundled {
		return "[B] " + item.name
	}
	return item.name
}

// computeWidths dynamically fits columns into the available width without wrapping.
// Spacing accounts for the single spaces inserted between columns.
func computeWidths(total int) (idW, nameW, versionW, statusW, typeW, pathW int) {
	const (
		spaceCount = 5
		pathMin    = 8
		idMin      = 12
		nameMin    = 12
		verMin     = 9
	)

	// Preferred widths.
	idW = 14
	nameW = 18
	versionW = 12
	statusW = 10
	typeW = 8

	fixed := idW + nameW + versionW + statusW + typeW
	available := total - spaceCount
	if available < fixed+pathMin {
		deficit := fixed + pathMin - available
		// Shrink name, then id, then version until the deficit is covered.
		shrink := func(v *int, min int) {
			if deficit == 0 {
				return
			}
			room := *v - min
			if room <= 0 {
				return
			}
			delta := deficit
			if delta > room {
				delta = room
			}
			*v -= delta
			deficit -= delta
		}
		shrink(&nameW, nameMin)
		shrink(&idW, idMin)
		shrink(&versionW, verMin)
	}

	pathW = available - (idW + nameW + versionW + statusW + typeW)
	if pathW < pathMin {
		pathW = pathMin
	}

	return
}

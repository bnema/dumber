package styles

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// KeyMap defines keybindings that can be rendered as help.
type KeyMap interface {
	ShortHelp() []key.Binding
	FullHelp() [][]key.Binding
}

// HistoryKeyMap defines keybindings for history browser.
type HistoryKeyMap struct {
	Up           key.Binding
	Down         key.Binding
	Tab1         key.Binding
	Tab2         key.Binding
	Tab3         key.Binding
	Tab4         key.Binding
	NextTab      key.Binding
	PrevTab      key.Binding
	Search       key.Binding
	Open         key.Binding
	Delete       key.Binding
	DeleteDomain key.Binding
	Cleanup      key.Binding
	Filter       key.Binding
	Help         key.Binding
	Quit         key.Binding
}

// ShortHelp returns keybindings to show in compact help.
func (k HistoryKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Search, k.Help, k.Quit}
}

// FullHelp returns keybindings for expanded help.
func (k HistoryKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Open},
		{k.NextTab, k.PrevTab, k.Tab1, k.Tab2, k.Tab3, k.Tab4},
		{k.Search, k.Filter},
		{k.Delete, k.DeleteDomain, k.Cleanup},
		{k.Help, k.Quit},
	}
}

// DefaultHistoryKeyMap returns the default history keybindings.
func DefaultHistoryKeyMap() HistoryKeyMap {
	return HistoryKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Tab1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "today"),
		),
		Tab2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "yesterday"),
		),
		Tab3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "this week"),
		),
		Tab4: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "older"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("S-tab", "prev tab"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Open: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		DeleteDomain: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "delete domain"),
		),
		Cleanup: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "cleanup"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "filter domain"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// NewStyledHelp creates a themed help model.
func NewStyledHelp(theme *Theme) help.Model {
	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(theme.Accent)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(theme.Muted)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(theme.Border)
	h.Styles.FullKey = lipgloss.NewStyle().Foreground(theme.Accent)
	h.Styles.FullDesc = lipgloss.NewStyle().Foreground(theme.Text)
	h.Styles.FullSeparator = lipgloss.NewStyle().Foreground(theme.Border)
	return h
}

// DmenuKeyMap defines keybindings for dmenu browser.
type DmenuKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Open   key.Binding
	Cancel key.Binding
}

// ShortHelp returns keybindings to show in compact help.
func (k DmenuKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Cancel}
}

// FullHelp returns keybindings for expanded help.
func (k DmenuKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Open, k.Cancel},
	}
}

// DefaultDmenuKeyMap returns the default dmenu keybindings.
func DefaultDmenuKeyMap() DmenuKeyMap {
	return DmenuKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Open: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "ctrl+c"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

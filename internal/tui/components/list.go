package components

import (
	"github.com/bnema/dumber/internal/tui"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// NewList creates a bubbles/list instance styled with the shared grayscale palette.
func NewList(items []list.Item, width, height int) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.Styles.NormalTitle = tui.BaseStyle
	delegate.Styles.DimmedTitle = tui.MutedStyle
	delegate.Styles.SelectedTitle = tui.SelectedStyle
	delegate.Styles.SelectedDesc = tui.SelectedStyle.Copy().Faint(true)
	delegate.SetSpacing(0)

	model := list.New(items, delegate, width, height)
	model.SetFilteringEnabled(true)
	model.SetShowHelp(false)
	model.SetShowPagination(false)
	model.SetShowStatusBar(false)
	model.SetShowTitle(false)
	model.DisableQuitKeybindings()
	model.Styles.NoItems = tui.MutedStyle
	model.Styles.PaginationStyle = tui.MutedStyle
	model.Styles.FilterPrompt = tui.MutedStyle.Copy().Bold(true)
	model.Styles.FilterCursor = tui.BaseStyle.Copy().Bold(true)
	model.Styles.DefaultFilterCharacterMatch = tui.HeaderStyle
	model.Styles.StatusBarActiveFilter = tui.MutedStyle
	model.Styles.StatusBarFilterCount = tui.MutedStyle
	model.Styles.StatusBar = lipgloss.NewStyle().Foreground(tui.ColorLightGray)

	return model
}

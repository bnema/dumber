package model

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
)

// CleanupModel wraps styles.CleanupModel for standalone use.
type CleanupModel struct {
	cleanup  styles.CleanupModel
	cleaned  bool
	cleaning bool
	err      error

	historyUC *usecase.SearchHistoryUseCase
	theme     *styles.Theme
}

// NewCleanupModel creates a new cleanup command model.
func NewCleanupModel(theme *styles.Theme, historyUC *usecase.SearchHistoryUseCase) CleanupModel {
	return CleanupModel{
		cleanup:   styles.NewCleanup(theme),
		historyUC: historyUC,
		theme:     theme,
	}
}

// cleanupCompleteMsg is sent when cleanup is done.
type cleanupCompleteMsg struct {
	err error
}

// Init implements tea.Model.
func (m CleanupModel) Init() tea.Cmd {
	if m.theme == nil {
		return nil
	}
	return nil
}

// Update implements tea.Model.
func (m CleanupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cleanupCompleteMsg:
		m.cleaning = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.cleaned = true
		}
		return m, nil

	case tea.KeyMsg:
		// If cleaned, any key exits
		if m.cleaned || m.err != nil {
			return m, tea.Quit
		}

		// If cleaning, ignore input
		if m.cleaning {
			return m, nil
		}
	}

	// Update inner cleanup model
	cleanup, cmd := m.cleanup.Update(msg)
	m.cleanup = cleanup

	if m.cleanup.Done() {
		if m.cleanup.Canceled {
			return m, tea.Quit
		}
		if m.cleanup.Confirmed {
			m.cleaning = true
			return m, m.performCleanup(m.cleanup.SelectedRange())
		}
	}

	return m, cmd
}

// performCleanup executes the cleanup operation.
func (m CleanupModel) performCleanup(cleanupRange styles.CleanupRange) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		cutoff := cleanupRange.CutoffTime()

		var err error
		if cutoff.IsZero() {
			err = m.historyUC.ClearAll(ctx)
		} else {
			err = m.historyUC.ClearOlderThan(ctx, cutoff)
		}

		return cleanupCompleteMsg{err: err}
	}
}

// View implements tea.Model.
func (m CleanupModel) View() string {
	t := m.theme

	if m.cleaning {
		spinner := styles.NewLoading(t, "Cleaning up...")
		return t.Box.Render(spinner.View())
	}

	if m.err != nil {
		content := lipgloss.JoinVertical(
			lipgloss.Left,
			t.ErrorStyle.Render("Error: "+m.err.Error()),
			"",
			t.Subtle.Render("Press any key to exit"),
		)
		return t.Box.Render(content)
	}

	if m.cleaned {
		content := lipgloss.JoinVertical(
			lipgloss.Left,
			t.SuccessStyle.Render("History cleaned successfully!"),
			"",
			t.Subtle.Render("Press any key to exit"),
		)
		return t.Box.Render(content)
	}

	return m.cleanup.View()
}

// Ensure interface compliance.
var _ tea.Model = (*CleanupModel)(nil)

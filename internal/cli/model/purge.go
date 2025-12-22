package model

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
)

// PurgeModel wraps styles.PurgeModel for standalone CLI use.
type PurgeModel struct {
	selector styles.PurgeModel
	purgeUC  *usecase.PurgeDataUseCase

	loading bool
	purging bool
	done    bool

	results *usecase.PurgeOutput
	info    string
	err     error

	theme *styles.Theme
}

// NewPurgeModel creates a new purge command model.
func NewPurgeModel(theme *styles.Theme, purgeUC *usecase.PurgeDataUseCase) PurgeModel {
	return PurgeModel{theme: theme, purgeUC: purgeUC, loading: true}
}

type purgeTargetsLoadedMsg struct {
	targets []entity.PurgeTarget
	err     error
}

type purgeCompleteMsg struct {
	out *usecase.PurgeOutput
	err error
}

// Init implements tea.Model.
func (m PurgeModel) Init() tea.Cmd {
	return m.loadTargets()
}

func (m PurgeModel) loadTargets() tea.Cmd {
	return func() tea.Msg {
		targets, err := m.purgeUC.GetPurgeTargets(context.Background())
		return purgeTargetsLoadedMsg{targets: targets, err: err}
	}
}

// Update implements tea.Model.
func (m PurgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case purgeTargetsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, nil
		}

		m.selector = styles.NewPurge(m.theme, msg.targets)
		return m, nil

	case purgeCompleteMsg:
		m.purging = false
		m.done = true
		m.results = msg.out
		if msg.err != nil {
			m.err = msg.err
		}
		return m, nil

	case tea.KeyMsg:
		if m.done {
			return m, tea.Quit
		}
		if m.loading || m.purging {
			return m, nil
		}
	}

	if m.loading || m.purging {
		return m, nil
	}

	selector, cmd := m.selector.Update(msg)
	m.selector = selector

	if m.selector.Done() {
		if m.selector.Canceled {
			return m, tea.Quit
		}
		if m.selector.Confirmed {
			targetTypes := m.selector.SelectedTypes()
			if len(targetTypes) == 0 {
				m.done = true
				m.info = "Nothing selected"
				return m, nil
			}
			m.purging = true
			return m, m.performPurge(targetTypes)
		}
	}

	return m, cmd
}

func (m PurgeModel) performPurge(targetTypes []entity.PurgeTargetType) tea.Cmd {
	return func() tea.Msg {
		out, err := m.purgeUC.Execute(context.Background(), usecase.PurgeInput{TargetTypes: targetTypes})
		return purgeCompleteMsg{out: out, err: err}
	}
}

// View implements tea.Model.
func (m PurgeModel) View() string {
	t := m.theme

	if m.loading {
		spinner := styles.NewLoading(t, "Scanning purge targets...")
		return t.Box.Render(spinner.View())
	}

	if m.purging {
		spinner := styles.NewLoading(t, "Purging...")
		return t.Box.Render(spinner.View())
	}

	if m.done {
		if m.info != "" {
			content := lipgloss.JoinVertical(
				lipgloss.Left,
				t.Subtle.Render(m.info),
				"",
				t.Subtle.Render("Press any key to exit"),
			)
			return t.Box.Render(content)
		}

		if m.err != nil && m.results == nil {
			content := lipgloss.JoinVertical(
				lipgloss.Left,
				t.ErrorStyle.Render("Error: "+m.err.Error()),
				"",
				t.Subtle.Render("Press any key to exit"),
			)
			return t.Box.Render(content)
		}

		contentLines := []string{t.Title.Render("Purge complete")}
		if m.results != nil {
			for _, r := range m.results.Results {
				if r.Success {
					contentLines = append(contentLines, fmt.Sprintf("%s %s", t.SuccessStyle.Render(styles.IconCheck), r.Target.Path))
				} else {
					contentLines = append(contentLines, fmt.Sprintf("%s %s: %v", t.ErrorStyle.Render(styles.IconX), r.Target.Path, r.Error))
				}
			}
			contentLines = append(
				contentLines,
				"",
				t.Subtle.Render(
					fmt.Sprintf("%d succeeded, %d failed", m.results.SuccessCount, m.results.FailureCount),
				),
			)
		}
		contentLines = append(contentLines, "", t.Subtle.Render("Press any key to exit"))
		return t.Box.Render(lipgloss.JoinVertical(lipgloss.Left, contentLines...))
	}

	return m.selector.View()
}

var _ tea.Model = (*PurgeModel)(nil)

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
	selector        styles.PurgeModel
	purgeUC         *usecase.PurgeDataUseCase
	listSessionsUC  *usecase.ListSessionsUseCase
	deleteSessionUC *usecase.DeleteSessionUseCase

	loading         bool
	loadingSessions bool
	purging         bool
	done            bool

	// Track loading state for parallel loading
	targetsLoaded  bool
	sessionsLoaded bool
	targets        []entity.PurgeTarget
	sessions       []entity.SessionPurgeItem
	sessionsSize   int64

	results        *usecase.PurgeOutput
	sessionResults []sessionPurgeResult
	info           string
	err            error

	theme *styles.Theme
	ctx   context.Context
}

// sessionPurgeResult tracks the result of deleting a session.
type sessionPurgeResult struct {
	SessionID entity.SessionID
	Success   bool
	Error     error
}

// PurgeModelConfig contains optional dependencies for session purging.
type PurgeModelConfig struct {
	ListSessionsUC  *usecase.ListSessionsUseCase
	DeleteSessionUC *usecase.DeleteSessionUseCase
}

// NewPurgeModel creates a new purge command model.
func NewPurgeModel(ctx context.Context, theme *styles.Theme, purgeUC *usecase.PurgeDataUseCase, cfg PurgeModelConfig) PurgeModel {
	m := PurgeModel{
		theme:           theme,
		purgeUC:         purgeUC,
		listSessionsUC:  cfg.ListSessionsUC,
		deleteSessionUC: cfg.DeleteSessionUC,
		loading:         true,
		loadingSessions: cfg.ListSessionsUC != nil,
		ctx:             ctx,
	}
	return m
}

type purgeTargetsLoadedMsg struct {
	targets []entity.PurgeTarget
	err     error
}

type purgeSessionsLoadedMsg struct {
	sessions []entity.SessionPurgeItem
	size     int64
	err      error
}

type purgeCompleteMsg struct {
	targetResults  []entity.PurgeResult
	sessionResults []sessionPurgeResult
	err            error
}

// Init implements tea.Model.
func (m PurgeModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadTargets()}
	if m.listSessionsUC != nil {
		cmds = append(cmds, m.loadSessions())
	}
	return tea.Batch(cmds...)
}

func (m PurgeModel) loadTargets() tea.Cmd {
	return func() tea.Msg {
		targets, err := m.purgeUC.GetPurgeTargets(context.Background())
		return purgeTargetsLoadedMsg{targets: targets, err: err}
	}
}

func (m PurgeModel) loadSessions() tea.Cmd {
	return func() tea.Msg {
		output, err := m.listSessionsUC.GetPurgeableSessions(context.Background())
		if err != nil {
			return purgeSessionsLoadedMsg{err: err}
		}
		return purgeSessionsLoadedMsg{
			sessions: output.Sessions,
			size:     output.TotalSize,
		}
	}
}

// Update implements tea.Model.
func (m PurgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case purgeTargetsLoadedMsg:
		return m.handleTargetsLoaded(msg), nil
	case purgeSessionsLoadedMsg:
		return m.handleSessionsLoaded(msg), nil
	case purgeCompleteMsg:
		return m.handlePurgeComplete(msg), nil
	case tea.KeyMsg:
		if m.done {
			return m, tea.Quit
		}
		if m.isBlocked() {
			return m, nil
		}
		// Fall through to updateSelector
	}

	if m.isBlocked() {
		return m, nil
	}

	return m.updateSelector(msg)
}

func (m PurgeModel) handleTargetsLoaded(msg purgeTargetsLoadedMsg) PurgeModel {
	if msg.err != nil {
		m.err = msg.err
		m.done = true
		m.loading = false
		return m
	}
	m.targetsLoaded = true
	m.targets = msg.targets
	return m.checkLoadingComplete()
}

func (m PurgeModel) handleSessionsLoaded(msg purgeSessionsLoadedMsg) PurgeModel {
	if msg.err != nil {
		// Session loading failure is non-fatal, continue without sessions
		m.sessionsLoaded = true
		m.loadingSessions = false
		return m.checkLoadingComplete()
	}
	m.sessionsLoaded = true
	m.sessions = msg.sessions
	m.sessionsSize = msg.size
	return m.checkLoadingComplete()
}

func (m PurgeModel) handlePurgeComplete(msg purgeCompleteMsg) PurgeModel {
	m.purging = false
	m.done = true
	if msg.targetResults != nil {
		m.results = &usecase.PurgeOutput{Results: msg.targetResults}
		for _, r := range msg.targetResults {
			if r.Success {
				m.results.SuccessCount++
			} else {
				m.results.FailureCount++
			}
		}
	}
	m.sessionResults = msg.sessionResults
	if msg.err != nil {
		m.err = msg.err
	}
	return m
}

func (m PurgeModel) isBlocked() bool {
	return m.loading || m.loadingSessions || m.purging
}

func (m PurgeModel) updateSelector(msg tea.Msg) (tea.Model, tea.Cmd) {
	selector, cmd := m.selector.Update(msg)
	m.selector = selector

	if !m.selector.Done() {
		return m, cmd
	}

	if m.selector.Canceled {
		return m, tea.Quit
	}

	if m.selector.Confirmed {
		targetTypes := m.selector.SelectedTypes()
		sessionIDs := m.selector.SelectedSessionIDs()
		if len(targetTypes) == 0 && len(sessionIDs) == 0 {
			m.done = true
			m.info = "Nothing selected"
			return m, nil
		}
		m.purging = true
		return m, m.performPurge(targetTypes, sessionIDs)
	}

	return m, cmd
}

func (m PurgeModel) checkLoadingComplete() PurgeModel {
	// If sessions aren't being loaded, only wait for targets
	needSessions := m.listSessionsUC != nil
	if m.targetsLoaded && (!needSessions || m.sessionsLoaded) {
		m.loading = false
		m.loadingSessions = false
		m.selector = styles.NewPurgeWithSessions(m.theme, m.targets, m.sessions, m.sessionsSize)
	}
	return m
}

func (m PurgeModel) performPurge(targetTypes []entity.PurgeTargetType, sessionIDs []entity.SessionID) tea.Cmd {
	return func() tea.Msg {
		var targetResults []entity.PurgeResult
		var sessionResults []sessionPurgeResult
		var lastErr error

		// Delete sessions first (if any selected and Data not selected)
		if m.deleteSessionUC != nil {
			for _, sid := range sessionIDs {
				err := m.deleteSessionUC.Execute(m.ctx, usecase.DeleteSessionInput{
					SessionID:        sid,
					CurrentSessionID: "", // CLI has no current session
				})
				sessionResults = append(sessionResults, sessionPurgeResult{
					SessionID: sid,
					Success:   err == nil,
					Error:     err,
				})
				if err != nil {
					lastErr = err
				}
			}
		}

		// Then delete target directories
		if len(targetTypes) > 0 {
			out, err := m.purgeUC.Execute(m.ctx, usecase.PurgeInput{TargetTypes: targetTypes})
			if out != nil {
				targetResults = out.Results
			}
			if err != nil {
				lastErr = err
			}
		}

		return purgeCompleteMsg{
			targetResults:  targetResults,
			sessionResults: sessionResults,
			err:            lastErr,
		}
	}
}

// View implements tea.Model.
func (m PurgeModel) View() string {
	t := m.theme

	if m.loading || m.loadingSessions {
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

		if m.err != nil && m.results == nil && len(m.sessionResults) == 0 {
			content := lipgloss.JoinVertical(
				lipgloss.Left,
				t.ErrorStyle.Render("Error: "+m.err.Error()),
				"",
				t.Subtle.Render("Press any key to exit"),
			)
			return t.Box.Render(content)
		}

		return m.renderResults()
	}

	return m.selector.View()
}

func (m PurgeModel) renderResults() string {
	t := m.theme
	contentLines := []string{t.Title.Render("Purge complete")}

	successCount := 0
	failureCount := 0

	// Target results
	if m.results != nil {
		for _, r := range m.results.Results {
			if r.Success {
				contentLines = append(contentLines, fmt.Sprintf("%s %s", t.SuccessStyle.Render(styles.IconCheck), r.Target.Path))
				successCount++
			} else {
				contentLines = append(contentLines, fmt.Sprintf("%s %s: %v", t.ErrorStyle.Render(styles.IconX), r.Target.Path, r.Error))
				failureCount++
			}
		}
	}

	// Session results
	for _, r := range m.sessionResults {
		if r.Success {
			contentLines = append(contentLines, fmt.Sprintf("%s Session %s", t.SuccessStyle.Render(styles.IconCheck), r.SessionID))
			successCount++
		} else {
			contentLines = append(contentLines, fmt.Sprintf("%s Session %s: %v", t.ErrorStyle.Render(styles.IconX), r.SessionID, r.Error))
			failureCount++
		}
	}

	contentLines = append(
		contentLines,
		"",
		t.Subtle.Render(fmt.Sprintf("%d succeeded, %d failed", successCount, failureCount)),
		"",
		t.Subtle.Render("Press any key to exit"),
	)

	return t.Box.Render(lipgloss.JoinVertical(lipgloss.Left, contentLines...))
}

var _ tea.Model = (*PurgeModel)(nil)

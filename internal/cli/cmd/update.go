package cmd

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/updater"
)

var updateForce bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install updates",
	Long: `Check for available updates and download/install them.

Use --force to reinstall even if already up-to-date (skips version check).`,
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVarP(&updateForce, "force", "f", false, "force reinstall (skips version check)")
}

// updateState represents the current state of the update process.
type updateState int

const (
	stateChecking updateState = iota
	stateDownloading
	stateDone
)

// updateModel is the bubbletea model for the update command.
type updateModel struct {
	spinner  spinner.Model
	renderer *styles.UpdateRenderer
	state    updateState
	force    bool

	// Results from check.
	updateAvailable bool
	canAutoUpdate   bool
	currentVersion  string
	latestVersion   string
	releaseURL      string
	downloadURL     string

	// Final result.
	result       string
	err          error
	quitting     bool
	updateStaged bool // tracks if an update was actually downloaded and staged
}

// checkResultMsg is sent when the update check completes.
type checkResultMsg struct {
	output *usecase.CheckUpdateOutput
	err    error
}

// downloadResultMsg is sent when the download completes.
type downloadResultMsg struct {
	output *usecase.ApplyUpdateOutput
	err    error
}

func newUpdateModel(renderer *styles.UpdateRenderer, accentColor lipgloss.Color, force bool) updateModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accentColor)

	return updateModel{
		spinner:  s,
		renderer: renderer,
		state:    stateChecking,
		force:    force,
	}
}

func (m updateModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.checkForUpdates())
}

func (m updateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case checkResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateDone
			return m, tea.Quit
		}

		m.updateAvailable = msg.output.UpdateAvailable
		m.canAutoUpdate = msg.output.CanAutoUpdate
		m.currentVersion = msg.output.CurrentVersion
		m.latestVersion = msg.output.LatestVersion
		m.releaseURL = msg.output.ReleaseURL
		m.downloadURL = msg.output.DownloadURL

		// Handle dev builds when force is used.
		if m.force && (m.currentVersion == "" || m.currentVersion == "dev") {
			m.result = m.renderer.RenderDevBuild()
			m.state = stateDone
			return m, tea.Quit
		}

		// Decide next action.
		switch {
		case !m.updateAvailable && !m.force:
			// Already up to date.
			m.result = m.renderer.RenderUpToDate(m.currentVersion)
			m.state = stateDone
			return m, tea.Quit

		case m.updateAvailable && !m.canAutoUpdate:
			// Update available but can't auto-update.
			m.result = m.renderer.RenderCannotAutoUpdate(m.currentVersion, m.latestVersion, m.releaseURL)
			m.state = stateDone
			return m, tea.Quit

		default:
			// Proceed with download (either update available or force mode).
			m.state = stateDownloading
			return m, m.downloadUpdate()
		}

	case downloadResultMsg:
		m.state = stateDone
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}

		if msg.output.Status == entity.UpdateStatusFailed {
			m.result = m.renderer.RenderError(fmt.Errorf("%s", msg.output.Message))
		} else {
			m.result = m.renderer.RenderStaged(m.latestVersion)
			m.updateStaged = true
		}
		return m, tea.Quit
	}

	return m, nil
}

func (m updateModel) View() string {
	if m.quitting {
		return ""
	}

	if m.err != nil {
		return m.renderer.RenderError(m.err)
	}

	if m.state == stateDone {
		return m.result
	}

	switch m.state {
	case stateChecking:
		return m.renderer.RenderChecking(m.spinner.View())
	case stateDownloading:
		version := m.latestVersion
		if m.force && !m.updateAvailable {
			version = m.currentVersion
		}
		return m.renderer.RenderDownloading(m.spinner.View(), version)
	default:
		return ""
	}
}

func (updateModel) checkForUpdates() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		checker := updater.NewGitHubChecker()
		applier, err := updater.NewApplierFromXDG()
		if err != nil {
			return checkResultMsg{err: fmt.Errorf("failed to create applier: %w", err)}
		}

		app := GetApp()
		if app == nil {
			return checkResultMsg{err: fmt.Errorf("app not initialized")}
		}

		checkUC := usecase.NewCheckUpdateUseCase(checker, applier, app.BuildInfo)
		result, err := checkUC.Execute(ctx, usecase.CheckUpdateInput{})

		return checkResultMsg{output: result, err: err}
	}
}

func (m updateModel) downloadUpdate() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		downloader := updater.NewGitHubDownloader()
		applier, err := updater.NewApplierFromXDG()
		if err != nil {
			return downloadResultMsg{err: fmt.Errorf("failed to create applier: %w", err)}
		}

		dirs, err := config.GetXDGDirs()
		if err != nil {
			return downloadResultMsg{err: fmt.Errorf("failed to get cache dir: %w", err)}
		}

		applyUC := usecase.NewApplyUpdateUseCase(downloader, applier, dirs.CacheHome)
		result, err := applyUC.Execute(ctx, usecase.ApplyUpdateInput{
			DownloadURL: m.downloadURL,
		})

		return downloadResultMsg{output: result, err: err}
	}
}

func runUpdate(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	renderer := styles.NewUpdateRenderer(app.Theme)

	// Create and run the bubbletea program.
	m := newUpdateModel(renderer, app.Theme.Accent, updateForce)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	// Check if the update was staged and we need to finalize on exit.
	if model, ok := finalModel.(updateModel); ok {
		if model.updateStaged {
			// If update was successfully staged, finalize it.
			if err := finalizeUpdate(); err != nil {
				fmt.Println(renderer.RenderError(err))
			}
		}
	}

	return nil
}

// finalizeUpdate applies any staged update.
func finalizeUpdate() error {
	ctx := context.Background()

	downloader := updater.NewGitHubDownloader()
	applier, err := updater.NewApplierFromXDG()
	if err != nil {
		return fmt.Errorf("failed to create applier: %w", err)
	}

	dirs, err := config.GetXDGDirs()
	if err != nil {
		return fmt.Errorf("failed to get cache dir: %w", err)
	}

	applyUC := usecase.NewApplyUpdateUseCase(downloader, applier, dirs.CacheHome)
	return applyUC.FinalizeOnExit(ctx)
}

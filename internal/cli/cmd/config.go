package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

var configYes bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `View configuration status and migrate to add new default settings.`,
}

var configStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show config file status and migration availability",
	Long:  `Display the config file path and check if any new settings are available.`,
	RunE:  runConfigStatus,
}

var configMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Add missing default settings to config file",
	Long: `Compares your config file with available defaults and adds any missing settings.

Existing settings are never modified - only missing keys are added with default values.`,
	RunE: runConfigMigrate,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configStatusCmd)
	configCmd.AddCommand(configMigrateCmd)
	configMigrateCmd.Flags().BoolVarP(&configYes, "yes", "y", false, "skip confirmation prompt")
}

// runConfigStatus shows config file path and migration status.
func runConfigStatus(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	renderer := styles.NewConfigRenderer(app.Theme)
	migrator := config.NewMigrator()
	uc := usecase.NewMigrateConfigUseCase(migrator)

	// Get config file path
	configFile, err := config.GetConfigFile()
	if err != nil {
		fmt.Println(renderer.RenderError(err))
		return nil
	}

	// Check if config file exists
	if _, statErr := os.Stat(configFile); os.IsNotExist(statErr) {
		fmt.Println(renderer.RenderNoConfigFile(configFile))
		return nil
	}

	// Check for migration
	ctx := context.Background()
	result, err := uc.Check(ctx, usecase.CheckConfigMigrationInput{})
	if err != nil {
		fmt.Println(renderer.RenderError(err))
		return nil
	}

	if !result.NeedsMigration {
		fmt.Println(renderer.RenderUpToDate(configFile))
		return nil
	}

	// Show status with missing count
	fmt.Println(renderer.RenderConfigInfo(configFile, len(result.MissingKeys)))
	fmt.Println(renderer.RenderMigrateHint())

	return nil
}

// runConfigMigrate runs the migration with optional confirmation.
func runConfigMigrate(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	renderer := styles.NewConfigRenderer(app.Theme)
	migrator := config.NewMigrator()
	uc := usecase.NewMigrateConfigUseCase(migrator)

	// Get config file path
	configFile, err := config.GetConfigFile()
	if err != nil {
		fmt.Println(renderer.RenderError(err))
		return nil
	}

	// Check if config file exists
	if _, statErr := os.Stat(configFile); os.IsNotExist(statErr) {
		fmt.Println(renderer.RenderNoConfigFile(configFile))
		return nil
	}

	// Check for migration
	ctx := context.Background()
	result, err := uc.Check(ctx, usecase.CheckConfigMigrationInput{})
	if err != nil {
		fmt.Println(renderer.RenderError(err))
		return nil
	}

	if !result.NeedsMigration {
		fmt.Println(renderer.RenderUpToDate(configFile))
		return nil
	}

	// Show config info and missing keys
	fmt.Println(renderer.RenderConfigInfo(configFile, len(result.MissingKeys)))
	fmt.Println(renderer.RenderMissingKeys(result.MissingKeys))

	// If --yes flag, proceed without confirmation
	if configYes {
		return executeMigration(ctx, uc, renderer)
	}

	// Show confirmation dialog
	return runMigrateWithConfirmation(ctx, uc, renderer, app.Theme, result.MissingKeys)
}

// executeMigration performs the actual migration.
func executeMigration(ctx context.Context, uc *usecase.MigrateConfigUseCase, renderer *styles.ConfigRenderer) error {
	result, err := uc.Execute(ctx, usecase.MigrateConfigInput{})
	if err != nil {
		fmt.Println(renderer.RenderError(err))
		return nil
	}

	if len(result.AddedKeys) > 0 {
		fmt.Println(renderer.RenderMigrationSuccess(len(result.AddedKeys), result.ConfigFile))
	}

	return nil
}

// migrateState represents the current state of the migrate confirmation.
type migrateState int

const (
	migrateStateConfirm migrateState = iota
	migrateStateDone
)

// migrateModel is the bubbletea model for the migrate confirmation.
type migrateModel struct {
	spinner     spinner.Model
	renderer    *styles.ConfigRenderer
	confirm     styles.ConfirmModel
	state       migrateState
	uc          *usecase.MigrateConfigUseCase
	missingKeys []port.KeyInfo

	result   string
	err      error
	quitting bool
}

// migrateResultMsg is sent when the migration completes.
type migrateResultMsg struct {
	output *usecase.MigrateConfigOutput
	err    error
}

func newMigrateModel(
	renderer *styles.ConfigRenderer,
	theme *styles.Theme,
	uc *usecase.MigrateConfigUseCase,
	missingKeys []port.KeyInfo,
) migrateModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.Accent)

	confirm := styles.NewConfirm(theme, "Add these settings with default values?")

	return migrateModel{
		spinner:     s,
		renderer:    renderer,
		confirm:     confirm,
		state:       migrateStateConfirm,
		uc:          uc,
		missingKeys: missingKeys,
	}
}

func (m migrateModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m migrateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case migrateResultMsg:
		m.state = migrateStateDone
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}

		if len(msg.output.AddedKeys) > 0 {
			m.result = m.renderer.RenderMigrationSuccess(len(msg.output.AddedKeys), msg.output.ConfigFile)
		}
		return m, tea.Quit
	}

	// Handle confirm dialog
	if m.state == migrateStateConfirm {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)

		if m.confirm.Done() {
			if m.confirm.Result() {
				// User confirmed, run migration
				return m, m.runMigration()
			}
			// User canceled
			m.quitting = true
			return m, tea.Quit
		}

		return m, cmd
	}

	return m, nil
}

func (m migrateModel) View() string {
	if m.quitting {
		return ""
	}

	if m.err != nil {
		return m.renderer.RenderError(m.err)
	}

	if m.state == migrateStateDone {
		return m.result
	}

	// Show confirmation dialog
	return m.confirm.View()
}

func (m migrateModel) runMigration() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		result, err := m.uc.Execute(ctx, usecase.MigrateConfigInput{})
		return migrateResultMsg{output: result, err: err}
	}
}

// runMigrateWithConfirmation runs the migrate with an interactive confirmation dialog.
func runMigrateWithConfirmation(
	ctx context.Context,
	uc *usecase.MigrateConfigUseCase,
	renderer *styles.ConfigRenderer,
	theme *styles.Theme,
	missingKeys []port.KeyInfo,
) error {
	// Suppress unused ctx warning - kept for consistency with other run functions
	_ = ctx

	m := newMigrateModel(renderer, theme, uc, missingKeys)
	p := tea.NewProgram(m)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/cli/model"
)

var (
	historyJSON bool
	historyMax  int
)

const defaultHistoryMax = 50

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Browse and manage history",
	Long:  `Interactive history browser with timeline tabs, fuzzy search, and cleanup.`,
	RunE:  runHistory,
}

func init() {
	rootCmd.AddCommand(historyCmd)

	historyCmd.Flags().BoolVar(&historyJSON, "json", false, "output as JSON")
	historyCmd.Flags().IntVar(&historyMax, "max", defaultHistoryMax, "maximum entries to show (for --json)")
}

func runHistory(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	// JSON output mode (non-interactive)
	if historyJSON {
		return runHistoryJSON()
	}

	// Interactive TUI mode
	return runHistoryTUI()
}

// runHistoryTUI runs the interactive history browser.
func runHistoryTUI() error {
	app := GetApp()

	m := model.NewHistoryModel(app.Ctx(), app.Theme, app.SearchHistoryUC)

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// runHistoryJSON outputs history as JSON.
func runHistoryJSON() error {
	app := GetApp()

	m := model.NewHistoryListModel(app.Ctx(), app.SearchHistoryUC, historyMax)

	// Run briefly to load data
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("run model: %w", err)
	}

	// Extract results
	listModel, ok := finalModel.(model.HistoryListModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}

	if listModel.Error() != nil {
		return listModel.Error()
	}

	// Output as JSON
	entries := listModel.Entries()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// statsCmd shows history statistics.
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show history statistics",
	Long:  `Display analytics about your browsing history.`,
	RunE:  runStats,
}

func init() {
	historyCmd.AddCommand(statsCmd)
}

func runStats(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	// Run stats model
	m := model.NewStatsModel(app.Theme, app.SearchHistoryUC)

	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

// clearCmd clears history.
var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear history",
	Long:  `Interactively select time range to clear history.`,
	RunE:  runClear,
}

func init() {
	historyCmd.AddCommand(clearCmd)
}

func runClear(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	// Run cleanup modal directly
	m := model.NewCleanupModel(app.Theme, app.SearchHistoryUC)

	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

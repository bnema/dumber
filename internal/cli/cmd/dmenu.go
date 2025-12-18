package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/cli/model"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

var (
	dmenuInteractive bool
	dmenuSelect      bool
	dmenuMax         int
)

var dmenuCmd = &cobra.Command{
	Use:   "dmenu",
	Short: "Launcher integration for rofi/fuzzel",
	Long: `Output history for use with rofi, fuzzel, or other launchers.

Default mode outputs entries for piping:
  dumber dmenu | rofi -dmenu -p "Browse: " | dumber dmenu --select

Interactive mode provides a built-in TUI fuzzy finder:
  dumber dmenu --interactive`,
	RunE: runDmenu,
}

func init() {
	rootCmd.AddCommand(dmenuCmd)

	dmenuCmd.Flags().BoolVarP(&dmenuInteractive, "interactive", "i", false, "use interactive TUI mode")
	dmenuCmd.Flags().BoolVar(&dmenuSelect, "select", false, "process selection from stdin")
	dmenuCmd.Flags().IntVar(&dmenuMax, "max", 0, "maximum entries to output (default from config)")
}

func runDmenu(cmd *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	// Selection mode: read URL from stdin and open it
	if dmenuSelect {
		return runDmenuSelect()
	}

	// Interactive TUI mode
	if dmenuInteractive {
		return runDmenuInteractive()
	}

	// Pipe mode: output entries for rofi/fuzzel
	return runDmenuPipe()
}

// getDmenuConfig returns the dmenu configuration, with CLI flag overrides.
func getDmenuConfig() config.DmenuConfig {
	app := GetApp()
	cfg := app.Config.Dmenu

	// Override max if explicitly set via CLI flag
	if dmenuMax > 0 {
		cfg.MaxHistoryItems = dmenuMax
	}

	return cfg
}

// runDmenuPipe outputs history entries for launcher consumption.
func runDmenuPipe() error {
	app := GetApp()
	ctx := context.Background()
	cfg := getDmenuConfig()

	entries, err := app.SearchHistoryUC.GetRecent(ctx, cfg.MaxHistoryItems, 0)
	if err != nil {
		return fmt.Errorf("get history: %w", err)
	}

	// Output in rofi-compatible format
	for _, entry := range entries {
		title := entry.Title
		if title == "" {
			title = entry.URL
		}

		// Truncate long titles
		if len(title) > 80 {
			title = title[:77] + "..."
		}

		// Build output line based on config
		var parts []string
		parts = append(parts, cfg.HistoryPrefix+" "+title)

		if cfg.ShowVisitCount && entry.VisitCount > 0 {
			parts[0] = fmt.Sprintf("%s (%d)", parts[0], entry.VisitCount)
		}

		if cfg.ShowLastVisited && !entry.LastVisited.IsZero() {
			dateStr := entry.LastVisited.Format(cfg.DateFormat)
			parts[0] = fmt.Sprintf("%s [%s]", parts[0], dateStr)
		}

		// Output with URL as the actual value (for selection)
		// Using tab separator so rofi can use -d '\t'
		fmt.Printf("%s\t%s\n", parts[0], entry.URL)
	}

	return nil
}

// runDmenuSelect processes a selection from stdin.
func runDmenuSelect() error {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read selection: %w", err)
	}

	line = strings.TrimSpace(line)
	if line == "" {
		return nil // No selection
	}

	// If the line contains a tab, the URL is after it
	// Otherwise, try to extract URL or use the whole line
	url := line
	if idx := strings.Index(line, "\t"); idx != -1 {
		url = strings.TrimSpace(line[idx+1:])
	}

	// Try to open the URL
	if url != "" {
		return openInBrowser(url)
	}

	return nil
}

// runDmenuInteractive runs the interactive TUI dmenu.
func runDmenuInteractive() error {
	app := GetApp()

	m := model.NewDmenuModel(app.Theme, app.SearchHistoryUC)

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	// If user selected an entry, open it
	dm, ok := finalModel.(model.DmenuModel)
	if ok && dm.SelectedURL() != "" {
		return openInBrowser(dm.SelectedURL())
	}

	return nil
}

// openInBrowser opens a URL in the default browser.
func openInBrowser(url string) error {
	// Try xdg-open first (Linux)
	cmd := newCommand("xdg-open", url)
	return cmd.Start()
}

// newCommand creates a command (abstracted for testing).
var newCommand = func(name string, args ...string) interface{ Start() error } {
	return &commandWrapper{name: name, args: args}
}

type commandWrapper struct {
	name string
	args []string
}

func (c *commandWrapper) Start() error {
	cmd := exec.Command(c.name, c.args...)
	return cmd.Start()
}

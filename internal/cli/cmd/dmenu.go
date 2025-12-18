package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/cli/model"
	domainurl "github.com/bnema/dumber/internal/domain/url"
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

// getFaviconPath returns the path to a cached favicon for the given URL.
// Returns empty string if the favicon doesn't exist in cache.
func getFaviconPath(rawURL string) string {
	cacheDir, err := config.GetFaviconCacheDir()
	if err != nil {
		return ""
	}
	domain := domainurl.ExtractDomain(rawURL)
	if domain == "" {
		return ""
	}
	filename := domainurl.SanitizeDomainForFilename(domain)
	path := filepath.Join(cacheDir, filename)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
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

	// Output in rofi/fuzzel compatible format
	for _, entry := range entries {
		title := entry.Title
		if title == "" {
			title = entry.URL
		}

		// Truncate long titles
		if len(title) > 80 {
			title = title[:77] + "..."
		}

		// Check if we have a favicon
		faviconPath := getFaviconPath(entry.URL)

		// Build display text
		var displayText string
		if faviconPath != "" {
			displayText = title
		} else {
			displayText = cfg.HistoryPrefix + " " + title
		}

		// Format: "DisplayText\0icon\x1f/path\x1finfo\x1fURL\n"
		// Using info field to store URL for selection
		if faviconPath != "" {
			fmt.Printf("%s\x00icon\x1f%s\x1finfo\x1f%s\n", displayText, faviconPath, entry.URL)
		} else {
			fmt.Printf("%s\x00info\x1f%s\n", displayText, entry.URL)
		}
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

	url := parseSelection(line)
	if url != "" {
		return openInBrowser(url)
	}

	return nil
}

// parseSelection extracts the URL from a dmenu selection.
func parseSelection(selection string) string {
	selection = strings.TrimSpace(selection)

	// Extract info field if present (format: "text\0...info\x1fURL")
	if infoIndex := strings.Index(selection, "info\x1f"); infoIndex > 0 {
		info := selection[infoIndex+5:] // Skip "info\x1f"
		// Info might have more fields after, but URL should be first
		if endIndex := strings.Index(info, "\x1f"); endIndex > 0 {
			return strings.TrimSpace(info[:endIndex])
		}
		return strings.TrimSpace(info)
	}

	// Strip metadata if present (format: "text\0...")
	if nullIndex := strings.Index(selection, "\x00"); nullIndex > 0 {
		selection = selection[:nullIndex]
	}

	// Legacy: strip emoji prefix if present
	selection = strings.TrimPrefix(selection, "🕒 ")
	selection = strings.TrimPrefix(selection, "🔍 ")
	selection = strings.TrimPrefix(selection, "🌐 ")

	// If it looks like a URL, return it
	if strings.Contains(selection, "://") || strings.Contains(selection, ".") {
		return selection
	}

	return selection
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

// openInBrowser opens a URL in dumber browser.
func openInBrowser(url string) error {
	// Use the current executable to open in dumber
	executable, err := os.Executable()
	if err != nil {
		// Fallback to searching PATH
		executable = "dumber"
	}
	cmd := newCommand(executable, "browse", url)
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

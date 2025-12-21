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

func runDmenu(_ *cobra.Command, _ []string) error {
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

// getFaviconPath returns the path to a cached PNG favicon for the given URL.
// Returns empty string if the PNG favicon doesn't exist in cache.
// PNG format is required by rofi/fuzzel launchers.
func getFaviconPath(rawURL string) string {
	cacheDir, err := config.GetFaviconCacheDir()
	if err != nil {
		return ""
	}
	domain := domainurl.ExtractDomain(rawURL)
	if domain == "" {
		return ""
	}
	// Use PNG format for fuzzel/rofi compatibility
	filename := domainurl.SanitizeDomainForPNG(domain)
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
	// Display URL (stripped of scheme) so selection directly returns usable URL
	for _, entry := range entries {
		// Strip scheme from URL for cleaner display
		display := entry.URL
		display = strings.TrimPrefix(display, "https://")
		display = strings.TrimPrefix(display, "http://")

		// Truncate long URLs
		const maxDisplayLen = 100
		if len(display) > maxDisplayLen {
			display = display[:maxDisplayLen-3] + "..."
		}

		// Check if we have a favicon
		faviconPath := getFaviconPath(entry.URL)

		// Format: "URL\0icon\x1f/path\n" (simple, selection returns URL directly)
		if faviconPath != "" {
			fmt.Printf("%s\x00icon\x1f%s\n", display, faviconPath)
		} else {
			fmt.Println(display)
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
// Selection is the URL (stripped of scheme) as displayed by runDmenuPipe,
// or a bang shortcut like "!g query" for search.
func parseSelection(selection string) string {
	selection = strings.TrimSpace(selection)

	// Strip metadata if present (format: "url\0icon\x1f...")
	if nullIndex := strings.Index(selection, "\x00"); nullIndex > 0 {
		selection = selection[:nullIndex]
	}

	selection = strings.TrimSpace(selection)
	if selection == "" {
		return ""
	}

	// Use BuildSearchURL to handle bang shortcuts, URLs, and search queries
	app := GetApp()
	if app != nil {
		return domainurl.BuildSearchURL(selection, app.Config.ShortcutURLs(), app.Config.DefaultSearchEngine)
	}

	// Fallback: add https:// scheme if no app context
	if !strings.Contains(selection, "://") {
		selection = "https://" + selection
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

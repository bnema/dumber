package cli

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/bnema/dumber/internal/config"

	"github.com/spf13/cobra"
)

// NewBrowseCmd creates the browse command
func NewBrowseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browse <url|query>",
		Short: "Browse a URL or search query",
		Long: `Browse a URL directly or use search shortcuts like:
  g:golang      -> Google search for "golang"
  gh:cobra      -> GitHub search for "cobra"
  yt:tutorials  -> YouTube search for "tutorials"
  w:go language -> Wikipedia search for "go language"

Direct URLs are also supported:
  https://example.com
  example.com (automatically adds https://)`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cli, err := NewCLI()
			if err != nil {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			}
			defer func() {
				if closeErr := cli.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", closeErr)
				}
			}()

			return browse(cli, args[0])
		},
	}

	return cmd
}

// browse handles the core browsing logic
func browse(cli *CLI, input string) error {
	ctx := context.Background()

	// Parse the input to determine URL or shortcut
	finalURL, err := parseInput(ctx, cli, input)
	if err != nil {
		return fmt.Errorf("failed to parse input: %w", err)
	}

	// Record in history
	if err := recordVisit(ctx, cli, finalURL, ""); err != nil {
		// Don't fail the browse operation if history recording fails
		fmt.Fprintf(os.Stderr, "Warning: failed to record history: %v\n", err)
	}

	// Open URL using configuration
	return openURLWithConfig(finalURL, cli.Config)
}

// parseInput processes user input and returns the final URL to browse
func parseInput(ctx context.Context, cli *CLI, input string) (string, error) {
	// Check if it's a shortcut (format: "prefix:query")
	if strings.Contains(input, ":") && !strings.Contains(input, "://") {
		parts := strings.SplitN(input, ":", 2) //nolint:mnd // split on first colon only
		if len(parts) == 2 {                   //nolint:mnd // expect prefix and query parts
			shortcut := strings.TrimSpace(parts[0])
			query := strings.TrimSpace(parts[1])

			if query == "" {
				return "", fmt.Errorf("empty query for shortcut '%s'", shortcut)
			}

			// First check configuration-based shortcuts
			if shortcutCfg, exists := cli.Config.SearchShortcuts[shortcut]; exists {
				return fmt.Sprintf(shortcutCfg.URL, url.QueryEscape(query)), nil
			}

			// Fallback to database shortcuts for backward compatibility
			shortcuts, err := cli.Queries.GetShortcuts(ctx)
			if err != nil {
				return "", fmt.Errorf("failed to get shortcuts: %w", err)
			}

			for _, s := range shortcuts {
				if s.Shortcut == shortcut {
					return fmt.Sprintf(s.UrlTemplate, url.QueryEscape(query)), nil
				}
			}

			return "", fmt.Errorf("unknown shortcut '%s'", shortcut)
		}
	}

	// Check if it's already a valid URL
	if isValidURL(input) {
		return input, nil
	}

	// Try to make it a URL by adding https://
	if !strings.Contains(input, "://") {
		candidate := "https://" + input
		if isValidURL(candidate) {
			return candidate, nil
		}
	}

	// If all else fails, search Google for it
	return fmt.Sprintf("https://www.google.com/search?q=%s", url.QueryEscape(input)), nil
}

// isValidURL checks if a string is a valid URL
func isValidURL(str string) bool {
	u, err := url.Parse(str)
	if err != nil {
		return false
	}

	// Must have a scheme and host
	if u.Scheme == "" || u.Host == "" {
		return false
	}

	// Must be http or https
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	// Basic domain validation
	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?` +
		`(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	return domainRegex.MatchString(u.Host)
}

// recordVisit adds or updates a URL visit in the history
func recordVisit(ctx context.Context, cli *CLI, url, title string) error {
	var sqlTitle sql.NullString
	if title != "" {
		sqlTitle = sql.NullString{String: title, Valid: true}
	}

	return cli.Queries.AddOrUpdateHistory(ctx, url, sqlTitle)
}

// openURL opens a URL using the configured browser
func openURL(url string) error {
	return openURLWithConfig(url, nil)
}

// openURLWithConfig opens a URL using our built-in browser
func openURLWithConfig(url string, cfg *config.Config) error {
	// Get the path to our own executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Launch our own browser in browse mode with the URL directly
	cmd := exec.Command(executable, "browse", url)

	// Start the browser in detached mode
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open URL in built-in browser: %w", err)
	}

	// Don't wait for the browser process to exit
	go func() {
		_ = cmd.Wait()
	}()

	fmt.Printf("Opening: %s (using built-in browser)\n", url)
	return nil
}

// commandExists checks if a command is available in the system PATH
func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

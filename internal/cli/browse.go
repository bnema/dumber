package cli

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/bnema/dumber/internal/cache"
	"github.com/bnema/dumber/internal/config"

	"github.com/spf13/cobra"
)

// NewBrowseCmd creates the browse command
func NewBrowseCmd() *cobra.Command {
	var renderingMode string

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
			// Propagate rendering mode to GUI process via env var
			if renderingMode != "" {
				rm := strings.ToLower(renderingMode)
				switch rm {
				case "auto", "gpu", "cpu":
					_ = os.Setenv("DUMBER_RENDERING_MODE", rm)
				default:
					return fmt.Errorf("invalid --rendering-mode: %s (expected auto|gpu|cpu)", renderingMode)
				}
			}
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

	// Flags
	cmd.Flags().StringVar(&renderingMode, "rendering-mode", "", "Rendering mode: auto|gpu|cpu")

	return cmd
}

// browse handles the core browsing logic
func browse(cli *CLI, input string) error {
	ctx := context.Background()

	// Parse the input using parser service
	result, err := cli.ParserService.ParseInput(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to parse input: %w", err)
	}

	finalURL := result.URL

	// Record in history
	if err := recordVisit(ctx, cli, finalURL, ""); err != nil {
		// Don't fail the browse operation if history recording fails
		fmt.Fprintf(os.Stderr, "Warning: failed to record history: %v\n", err)
	}

	// Update favicon asynchronously (non-blocking)
	go updateFavicon(ctx, cli, finalURL)

	// Open URL using configuration
	return openURLWithConfig(finalURL, cli.Config)
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
func openURL(url string) error { //nolint:unused // Public API function
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

	// The subprocess will read the same config file and apply the proper precedence:
	// 1. Config file defaults
	// 2. Environment variables (override config)
	// 3. Command line flags (override both)
	//
	// We don't convert config to env vars here because that would break precedence.
	// The cfg parameter ensures we use the same config source in both processes.

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

// updateFavicon is now deprecated in favor of WebKit's native favicon API
// When using the built-in browser, WebKit automatically handles favicon detection
// This function is kept for CLI compatibility but may use fallback behavior
func updateFavicon(ctx context.Context, cli *CLI, pageURL string) {
	// For CLI usage (non-GUI), we still use the fallback method
	// The GUI browser handles favicons through WebKit signals
	parsedURL, err := url.Parse(pageURL)
	if err != nil {
		return // Silently fail for invalid URLs
	}

	// Skip favicon update for localhost, file://, or special schemes
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return
	}
	if strings.Contains(parsedURL.Host, "localhost") || strings.Contains(parsedURL.Host, "127.0.0.1") {
		return
	}

	// Standard favicon location (fallback)
	faviconURL := fmt.Sprintf("%s://%s/favicon.ico", parsedURL.Scheme, parsedURL.Host)

	// Update in database using the new sqlc-generated method
	faviconNullString := sql.NullString{String: faviconURL, Valid: true}
	if err := cli.Queries.UpdateHistoryFavicon(ctx, faviconNullString, pageURL); err != nil {
		// Silently fail - favicon is not critical
		return
	}

	// Also cache the favicon for dmenu use
	if faviconCache, err := cache.NewFaviconCache(); err == nil {
		faviconCache.CacheAsync(faviconURL)
	}
}

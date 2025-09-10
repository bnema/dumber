package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/config"
	"github.com/spf13/cobra"
)

// PurgeFlags holds all the purge command flags
type PurgeFlags struct {
	Database     bool
	HistoryCache bool
	BrowserCache bool
	BrowserData  bool
	State        bool
	Config       bool
	All          bool
	Force        bool
}

// NewPurgeCmd creates the purge command
func NewPurgeCmd() *cobra.Command {
	var flags PurgeFlags

	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge dumber data and cache files",
		Long: `Purge various dumber data and cache files. By default, purges everything.

Available purge targets:
  --database, -d       Purge the SQLite database (history, shortcuts, zoom levels)
  --history-cache, -H  Purge dmenu fuzzy search cache for history
  --browser-cache, -c  Purge WebKit browser cache (cached images, files, etc.)
  --browser-data, -b   Purge WebKit browser data (cookies, localStorage, sessionStorage)
  --state, -s          Purge all state data (includes database and caches)
  --config             Purge configuration files
  --all, -a            Purge everything (default if no specific flags are provided)

Use --force to skip the confirmation prompt.

Examples:
  dumber purge                     # Purge everything (with confirmation)
  dumber purge --force             # Purge everything (no confirmation)
  dumber purge -d -H -c            # Purge database and both caches
  dumber purge --browser-data -f   # Force purge browser data only`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Initialize CLI to get config paths
			cli, err := NewCLI()
			if err != nil {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			}
			defer func() {
				if closeErr := cli.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", closeErr)
				}
			}()

			return executePurge(flags)
		},
	}

	// Add flags
	cmd.Flags().BoolVarP(&flags.Database, "database", "d", false, "Purge the SQLite database")
	cmd.Flags().BoolVarP(&flags.HistoryCache, "history-cache", "H", false, "Purge dmenu fuzzy search cache")
	cmd.Flags().BoolVarP(&flags.BrowserCache, "browser-cache", "c", false, "Purge WebKit browser cache")
	cmd.Flags().BoolVarP(&flags.BrowserData, "browser-data", "b", false, "Purge WebKit browser data (cookies, localStorage)")
	cmd.Flags().BoolVarP(&flags.State, "state", "s", false, "Purge all state data")
	cmd.Flags().BoolVar(&flags.Config, "config", false, "Purge configuration files")
	cmd.Flags().BoolVarP(&flags.All, "all", "a", false, "Purge everything")
	cmd.Flags().BoolVarP(&flags.Force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

// executePurge handles the purge logic
func executePurge(flags PurgeFlags) error {
	// Determine what to purge
	purgeItems := determinePurgeItems(flags)
	if len(purgeItems) == 0 {
		fmt.Println("Nothing to purge.")
		return nil
	}

	// Get file paths for each item
	paths, err := getPurgePaths(purgeItems)
	if err != nil {
		return fmt.Errorf("failed to get purge paths: %w", err)
	}

	// Show confirmation unless --force is used
	if !flags.Force {
		if !confirmPurge(purgeItems, paths) {
			fmt.Println("Purge cancelled.")
			return nil
		}
	}

	// Execute the purge
	return performPurge(purgeItems, paths)
}

// determinePurgeItems determines what should be purged based on flags
func determinePurgeItems(flags PurgeFlags) []string {
	var items []string

	// If no specific flags are set, or --all is set, purge everything
	if flags.All || (!flags.Database && !flags.HistoryCache && !flags.BrowserCache && !flags.BrowserData && !flags.State && !flags.Config) {
		return []string{"database", "history-cache", "browser-cache", "browser-data", "config"}
	}

	// Add items based on flags
	if flags.Database {
		items = append(items, "database")
	}
	if flags.HistoryCache {
		items = append(items, "history-cache")
	}
	if flags.BrowserCache {
		items = append(items, "browser-cache")
	}
	if flags.BrowserData {
		items = append(items, "browser-data")
	}
	if flags.State {
		// State includes database and both caches
		items = append(items, "database", "history-cache", "browser-cache")
	}
	if flags.Config {
		items = append(items, "config")
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// getPurgePaths gets the file/directory paths for each purge item
func getPurgePaths(items []string) (map[string][]string, error) {
	paths := make(map[string][]string)

	for _, item := range items {
		switch item {
		case "database":
			dbPath, err := config.GetDatabaseFile()
			if err != nil {
				return nil, fmt.Errorf("failed to get database path: %w", err)
			}
			paths[item] = []string{dbPath}

		case "history-cache":
			stateDir, err := config.GetStateDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get state directory: %w", err)
			}
			dmenuCache := filepath.Join(stateDir, "dmenu_fuzzy_cache.bin")
			paths[item] = []string{dmenuCache}

		case "browser-cache":
			stateDir, err := config.GetStateDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get state directory: %w", err)
			}
			webkitCache := filepath.Join(stateDir, "webkit-cache")
			paths[item] = []string{webkitCache}

		case "browser-data":
			dataDir, err := config.GetDataDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get data directory: %w", err)
			}
			webkitData := filepath.Join(dataDir, "webkit")
			paths[item] = []string{webkitData}

		case "config":
			configFile, err := config.GetConfigFile()
			if err != nil {
				return nil, fmt.Errorf("failed to get config file path: %w", err)
			}
			paths[item] = []string{configFile}
		}
	}

	return paths, nil
}

// confirmPurge shows a confirmation prompt and returns true if user confirms
func confirmPurge(items []string, paths map[string][]string) bool {
	fmt.Printf("This will delete the following:\n\n")

	for _, item := range items {
		fmt.Printf("• %s:\n", strings.Title(item))
		for _, path := range paths[item] {
			// Check if path exists
			if _, err := os.Stat(path); err == nil {
				fmt.Printf("  - %s\n", path)
			} else {
				fmt.Printf("  - %s (not found)\n", path)
			}
		}
	}

	fmt.Printf("\nAre you sure you want to continue? (y/N): ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		response := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return response == "y" || response == "yes"
	}

	return false
}

// performPurge executes the actual deletion
func performPurge(items []string, paths map[string][]string) error {
	var errors []string

	for _, item := range items {
		fmt.Printf("Purging %s...\n", item)

		for _, path := range paths[item] {
			if err := deletePath(path); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", path, err))
				fmt.Printf("  ✗ Failed to delete %s: %v\n", path, err)
			} else {
				fmt.Printf("  ✓ Deleted %s\n", path)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("some items could not be deleted:\n%s", strings.Join(errors, "\n"))
	}

	fmt.Println("\nPurge completed successfully!")
	return nil
}

// deletePath deletes a file or directory
func deletePath(path string) error {
	// Check if path exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Path doesn't exist, consider it successful
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check path: %w", err)
	}

	// Delete based on type
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}
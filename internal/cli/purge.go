package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/spf13/cobra"
)

// toTitle capitalizes the first letter of the string
func toTitle(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// PurgeFlags holds all the purge command flags
type PurgeFlags struct {
	Database     bool
	HistoryCache bool
	BrowserCache bool
	BrowserData  bool
	FilterCache  bool
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
  --database, -d       Purge SQLite databases (dumber.sqlite and cookies.db)
                       Includes: history, zoom levels, certificates, cookies, favorites
  --history-cache, -H  Purge browsing history from database and dmenu fuzzy search cache
                       (binary file). Keeps cookies, zoom levels, and other database data.
  --browser-cache, -b  Purge WebKit browser cache (cached images, files, etc.)
  --browser-data, -B   Purge WebKit browser data (localStorage, sessionStorage, IndexedDB)
                       Note: Cookies are in cookies.db and deleted via --database
  --filter-cache, -F   Purge compiled ad blocking filters cache
  --state, -s          Purge all state data (includes databases and caches)
  --config, -c         Purge configuration files
  --all, -a            Purge everything (default if no specific flags are provided)

Use --force to skip the confirmation prompt.

Database Storage:
  Cookies are stored in cookies.db (separate from dumber.sqlite due to WebKit requirements).
  Use --database to delete both dumber.sqlite and cookies.db.
  The --browser-data flag only affects localStorage and other WebKit-managed data.

Examples:
  dumber purge                     # Purge everything (with confirmation)
  dumber purge --force             # Purge everything (no confirmation)
  dumber purge -d -H -b -F         # Purge database and all caches
  dumber purge --filter-cache -f   # Force purge filter cache only
  dumber purge -d                  # Purge database (includes cookies)
  dumber purge -H                  # Purge only browsing history (keep cookies)
  dumber purge -c                  # Purge config file only`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return executePurge(flags)
		},
	}

	// Add flags
	cmd.Flags().BoolVarP(&flags.Database, "database", "d", false, "Purge SQLite databases (dumber.sqlite and cookies.db)")
	cmd.Flags().BoolVarP(&flags.HistoryCache, "history-cache", "H", false, "Purge browsing history and dmenu fuzzy cache")
	cmd.Flags().BoolVarP(&flags.BrowserCache, "browser-cache", "b", false, "Purge WebKit browser cache")
	cmd.Flags().BoolVarP(&flags.BrowserData, "browser-data", "B", false, "Purge WebKit browser data (localStorage, sessionStorage)")
	cmd.Flags().BoolVarP(&flags.FilterCache, "filter-cache", "F", false, "Purge compiled ad blocking filters cache")
	cmd.Flags().BoolVarP(&flags.State, "state", "s", false, "Purge all state data (databases and caches)")
	cmd.Flags().BoolVarP(&flags.Config, "config", "c", false, "Purge configuration files")
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
	if flags.All || (!flags.Database && !flags.HistoryCache && !flags.BrowserCache && !flags.BrowserData && !flags.FilterCache && !flags.State && !flags.Config) {
		return []string{"database", "history-cache", "browser-cache", "browser-data", "filter-cache", "config"}
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
	if flags.FilterCache {
		items = append(items, "filter-cache")
	}
	if flags.State {
		// State includes database and all caches
		items = append(items, "database", "history-cache", "browser-cache", "filter-cache")
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
			// Get main database path
			dbPath, err := config.GetDatabaseFile()
			if err != nil {
				return nil, fmt.Errorf("failed to get database path: %w", err)
			}

			// Get data dir for cookies.db
			dataDir, err := config.GetDataDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get data directory: %w", err)
			}
			cookiesPath := filepath.Join(dataDir, "cookies.db")

			paths[item] = []string{dbPath, cookiesPath}

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

		case "filter-cache":
			filterCacheDir, err := config.GetFilterCacheDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get filter cache directory: %w", err)
			}
			paths[item] = []string{filterCacheDir}

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
		fmt.Printf("• %s:\n", toTitle(item))

		// Special message for history-cache
		if item == "history-cache" {
			fmt.Printf("  - All browsing history from database\n")
		}

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

		// Special handling for history-cache: delete from database first
		if item == "history-cache" {
			if err := purgeHistoryFromDB(); err != nil {
				errors = append(errors, fmt.Sprintf("history table: %v", err))
				fmt.Printf("  ✗ Failed to purge history from database: %v\n", err)
			} else {
				fmt.Printf("  ✓ Purged history from database\n")
			}
		}

		// Then proceed with file/directory deletions
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

// purgeHistoryFromDB deletes all history entries from the database
func purgeHistoryFromDB() error {
	dbPath, err := config.GetDatabaseFile()
	if err != nil {
		return fmt.Errorf("failed to get database path: %w", err)
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Database doesn't exist, nothing to purge
		return nil
	}

	// Initialize database connection with proper configuration
	database, err := db.InitDB(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Create queries instance
	queries := db.New(database)

	// Execute delete all history
	ctx := context.Background()
	if err := queries.DeleteAllHistory(ctx); err != nil {
		return fmt.Errorf("failed to delete history: %w", err)
	}

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

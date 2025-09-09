// Package cli provides the command-line interface for dumber.
package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"

	_ "github.com/ncruces/go-sqlite3/driver" // SQLite driver for database/sql
	_ "github.com/ncruces/go-sqlite3/embed"  // Embed SQLite for cross-platform compatibility
	"github.com/spf13/cobra"
)

// CLI holds the database connection, queries, and configuration for the CLI commands
type CLI struct {
	DB      *sql.DB
	Queries *db.Queries
	Config  *config.Config
}

// NewCLI creates a new CLI instance with database connection and configuration
func NewCLI() (*CLI, error) {
	// Get configuration
	cfg := config.Get()

	// Use configured database path
	dbPath := cfg.Database.Path
	if dbPath == "" {
		// Fallback to default path if not configured
		var err error
		dbPath, err = config.GetDatabaseFile()
		if err != nil {
			return nil, fmt.Errorf("failed to get database path: %w", err)
		}
	}

	// Ensure database directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize database schema with configuration-based shortcuts
	if err := initializeDatabase(database, cfg); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return &CLI{
		DB:      database,
		Queries: db.New(database),
		Config:  cfg,
	}, nil
}

// Close closes the database connection
func (c *CLI) Close() error {
	if c.DB != nil {
		return c.DB.Close()
	}
	return nil
}

// NewRootCmd creates the root command for dumber
func NewRootCmd(version, commit, buildDate string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "dumber [url]",
		Short: "A dumb browser for Wayland window managers",
		Long:  `A fast, simple browser with rofi/dmenu integration for sway and hyprland window managers.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize CLI
			cli, err := NewCLI()
			if err != nil {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			}
			defer func() {
				if closeErr := cli.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", closeErr)
				}
			}()

			// If URL provided, browse it directly
			if len(args) > 0 {
				return browse(cli, args[0])
			}

			// Otherwise show help
			return cmd.Help()
		},
	}

	// Add dmenu flag for launcher integration
	rootCmd.Flags().Bool("dmenu", false, "Run in dmenu mode for launcher integration")

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("dumber %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("built: %s\n", buildDate)
		},
	}

	// Init command (enhanced)
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize dumber database and configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cli, err := NewCLI()
			if err != nil {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			}
			defer func() {
				if closeErr := cli.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", closeErr)
				}
			}()

			fmt.Printf("dumber %s - Initialization complete!\n", version)
			fmt.Println("Database initialized at:", cli.Config.Database.Path)

			// Show XDG directories
			xdgDirs, err := config.GetXDGDirs()
			if err == nil {
				fmt.Println("Configuration directories:")
				fmt.Printf("- Config: %s\n", xdgDirs.ConfigHome)
				fmt.Printf("- Data: %s\n", xdgDirs.DataHome)
				fmt.Printf("- State: %s\n", xdgDirs.StateHome)
			}

			// Show configured shortcuts
			fmt.Println("Configured shortcuts:")
			for shortcut, cfg := range cli.Config.SearchShortcuts {
				fmt.Printf("- %s:%s -> %s\n", shortcut, "query", cfg.Description)
			}
			return nil
		},
	}

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(NewBrowseCmd())
	rootCmd.AddCommand(NewDmenuCmd())
	rootCmd.AddCommand(NewHistoryCmd())

	return rootCmd
}

// getDatabasePath returns the path to the SQLite database file (legacy function for compatibility)
func getDatabasePath() (string, error) {
	return config.GetDatabaseFile()
}

// getDatabasePathOrDefault returns the database path or a default message (legacy function for compatibility)
func getDatabasePathOrDefault() string {
	path, err := config.GetDatabaseFile()
	if err != nil {
		return "~/.local/state/dumber/history.db"
	}
	return path
}

// initializeDatabase creates the database schema if it doesn't exist
func initializeDatabase(db *sql.DB, cfg *config.Config) error {
	schema := `
	-- History tracking for visited URLs
	CREATE TABLE IF NOT EXISTS history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL UNIQUE,
		title TEXT,
		visit_count INTEGER DEFAULT 1,
		last_visited DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_history_url ON history(url);
	CREATE INDEX IF NOT EXISTS idx_history_last_visited ON history(last_visited);

	-- URL shortcuts configuration
	CREATE TABLE IF NOT EXISTS shortcuts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		shortcut TEXT NOT NULL UNIQUE,
		url_template TEXT NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	// Execute schema creation
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Insert configured shortcuts
	for shortcut, shortcutCfg := range cfg.SearchShortcuts {
		_, err := db.Exec(
			"INSERT OR IGNORE INTO shortcuts (shortcut, url_template, description) VALUES (?, ?, ?)",
			shortcut, shortcutCfg.URL, shortcutCfg.Description,
		)
		if err != nil {
			return fmt.Errorf("failed to insert shortcut %s: %w", shortcut, err)
		}
	}

	return nil
}

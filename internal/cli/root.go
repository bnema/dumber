// Package cli provides the command-line interface for dumber.
package cli

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/cache"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/migrations"
	"github.com/bnema/dumber/services"

	_ "github.com/ncruces/go-sqlite3/driver" // SQLite driver for database/sql
	_ "github.com/ncruces/go-sqlite3/embed"  // Embed SQLite for cross-platform compatibility
	"github.com/spf13/cobra"
)

// File permission constants
const (
	dirPerm = 0755 // Standard directory permissions (rwxr-xr-x)
)

// CLI holds the database connection, queries, and configuration for the CLI commands
type CLI struct {
	DB            *sql.DB
	Queries       *db.Queries
	Config        *config.Config
	ParserService *services.ParserService
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
	if err := os.MkdirAll(filepath.Dir(dbPath), dirPerm); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := database.Ping(); err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize database schema with configuration-based shortcuts
	if err := initializeDatabase(database, cfg); err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	queries := db.New(database)
	parserService := services.NewParserService(cfg, queries)

	// Clean old cached favicons on startup (async, non-blocking)
	go func() {
		if faviconCache, err := cache.NewFaviconCache(); err == nil {
			faviconCache.CleanOld()
		}
	}()

	return &CLI{
		DB:            database,
		Queries:       queries,
		Config:        cfg,
		ParserService: parserService,
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
	rootCmd.AddCommand(NewPurgeCmd())
	rootCmd.AddCommand(NewLogsCmd())

	return rootCmd
}


// initializeDatabase runs embedded migrations and ensures database is up to date
func initializeDatabase(db *sql.DB, cfg *config.Config) error {
	// Run embedded migrations - this will create all tables and apply any new migrations
	if err := migrations.RunEmbeddedMigrations(db); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Insert configured shortcuts (these are additive, won't override existing)
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

// Package cli provides CLI commands using Bubble Tea TUI.
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/build"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/logging"
)

// App holds CLI dependencies.
type App struct {
	Config    *config.Config
	Theme     *styles.Theme
	BuildInfo build.Info
	db        *sql.DB
	History   repository.HistoryRepository

	// Use cases
	SearchHistoryUC *usecase.SearchHistoryUseCase

	// Context with logger
	ctx        context.Context
	logCleanup func()
}

// NewApp creates a new CLI application with all dependencies.
func NewApp() (*App, error) {
	// Load config
	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Create theme from config
	theme := styles.NewTheme(cfg)

	// Initialize logger with file output for debugging
	// Logs can be viewed with: tail -f ~/.local/state/dumber/logs/session_*.log
	logCfg := logging.DefaultConfig()
	if level := os.Getenv("DUMBER_LOG_LEVEL"); level != "" {
		logCfg.Level = logging.ParseLevel(level)
	}

	// Create session log file
	home, _ := os.UserHomeDir()
	sessionID := logging.GenerateSessionID()
	logDir := filepath.Join(home, ".local", "state", "dumber", "logs")

	logger, logCleanup, err := logging.NewWithFile(logCfg, logging.FileConfig{
		Enabled:       true,
		LogDir:        logDir,
		SessionID:     sessionID,
		WriteToStderr: false, // CLI: log to file only, keep terminal clean
	})
	if err != nil {
		// Fallback to no-file logging
		logger = logging.NewFromEnv()
		logCleanup = func() {}
	}

	ctx := logging.WithContext(context.Background(), logger)
	logger.Info().Str("command", "cli").Msg("CLI session started")

	// Determine database path - use share directory per XDG spec (user data)
	dbFile := cfg.Database.Path
	if dbFile == "" {
		dbFile = filepath.Join(home, ".local", "share", "dumber", "dumber.db")
	}
	dbPath := filepath.Dir(dbFile)

	// Ensure data directory exists
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// Open database connection
	db, err := sqlite.NewConnection(ctx, dbFile)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	logger.Debug().Str("db_path", dbFile).Msg("database connected")

	// Create repositories
	historyRepo := sqlite.NewHistoryRepository(db)

	// Create use cases
	searchHistoryUC := usecase.NewSearchHistoryUseCase(historyRepo)

	return &App{
		Config:          cfg,
		Theme:           theme,
		db:              db,
		History:         historyRepo,
		SearchHistoryUC: searchHistoryUC,
		ctx:             ctx,
		logCleanup:      logCleanup,
	}, nil
}

// Close releases all resources.
func (a *App) Close() error {
	if a.logCleanup != nil {
		a.logCleanup()
	}
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// Ctx returns the application context with logger.
func (a *App) Ctx() context.Context {
	return a.ctx
}

// loadConfig loads configuration from standard locations.
func loadConfig() (*config.Config, error) {
	mgr, err := config.NewManager()
	if err != nil {
		// Return default config if manager fails
		return config.DefaultConfig(), nil
	}

	if err := mgr.Load(); err != nil {
		// Return default config if loading fails
		return config.DefaultConfig(), nil
	}

	return mgr.Get(), nil
}

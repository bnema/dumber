// Package cli provides CLI commands using Bubble Tea TUI.
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/build"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/favicon"
	infralogging "github.com/bnema/dumber/internal/infrastructure/logging"
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
	SessionUC       *usecase.ManageSessionUseCase

	// Services
	FaviconService *favicon.Service

	// Context with logger
	ctx        context.Context
	logCleanup func()
}

// NewApp creates a new CLI application with all dependencies.
func NewApp() (*App, error) {
	const dataDirPerm = 0o755
	var err error
	// Load config
	cfg := loadConfig()

	// Create theme from config
	theme := styles.NewTheme(cfg)

	// Start with a quiet logger. If a browser session is active, we'll attach to it.
	logLevel := cfg.Logging.Level
	if envLevel := os.Getenv("DUMBER_LOG_LEVEL"); envLevel != "" {
		logLevel = envLevel
	}

	logger, logCleanup, _ := logging.NewWithFile(
		logging.Config{Level: logging.ParseLevel(logLevel), Format: cfg.Logging.Format, TimeFormat: "15:04:05"},
		logging.FileConfig{Enabled: false, WriteToStderr: false},
	)
	ctx := logging.WithContext(context.Background(), logger)

	// Determine database path - use share directory per XDG spec (user data)
	home, _ := os.UserHomeDir()
	dbFile := cfg.Database.Path
	if dbFile == "" {
		dbFile = filepath.Join(home, ".local", "share", "dumber", "dumber.db")
	}
	dbPath := filepath.Dir(dbFile)

	// Ensure data directory exists
	if err = os.MkdirAll(dbPath, dataDirPerm); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// Open database connection
	db, err := sqlite.NewConnection(ctx, dbFile)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Attach CLI logs to the active browser session (if any).
	// This avoids creating extra tiny session log files for each CLI invocation.
	sessionRepo := sqlite.NewSessionRepository(db)
	sessionLoggerAdapter := infralogging.NewSessionLoggerAdapter()
	sessionUC := usecase.NewManageSessionUseCase(sessionRepo, sessionLoggerAdapter)
	if active, activeErr := sessionUC.GetActiveSession(ctx); activeErr == nil && active != nil {
		attachedLogger, attachedCleanup, _ := sessionLoggerAdapter.CreateLogger(ctx, active.ID, port.SessionLogConfig{
			Level:         logLevel,
			Format:        cfg.Logging.Format,
			TimeFormat:    "15:04:05",
			LogDir:        cfg.Logging.LogDir,
			WriteToStderr: false,
			EnableFileLog: cfg.Logging.EnableFileLog,
		})
		logger = attachedLogger
		logCleanup = attachedCleanup
		ctx = logging.WithContext(context.Background(), logger)
		logger.Info().Str("session_id", string(active.ID)).Msg("attached to active session")
	}

	logger.Debug().Str("db_path", dbFile).Msg("database connected")

	// Create repositories
	historyRepo := sqlite.NewHistoryRepository(db)

	// Create use cases
	searchHistoryUC := usecase.NewSearchHistoryUseCase(historyRepo)

	// Create favicon service for CLI (path resolution for dmenu/fuzzel)
	faviconCacheDir, _ := config.GetFaviconCacheDir()
	faviconService := favicon.NewService(faviconCacheDir)

	return &App{
		Config:          cfg,
		Theme:           theme,
		db:              db,
		History:         historyRepo,
		SearchHistoryUC: searchHistoryUC,
		SessionUC:       sessionUC,
		FaviconService:  faviconService,
		ctx:             ctx,
		logCleanup:      logCleanup,
	}, nil
}

// Close releases all resources.
func (a *App) Close() error {
	if a.logCleanup != nil {
		a.logCleanup()
	}
	if a.FaviconService != nil {
		a.FaviconService.Close()
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
func loadConfig() *config.Config {
	mgr, err := config.NewManager()
	if err != nil {
		// Return default config if manager fails
		return config.DefaultConfig()
	}

	if err := mgr.Load(); err != nil {
		// Return default config if loading fails
		return config.DefaultConfig()
	}

	return mgr.Get()
}

// Package cli provides CLI commands using Bubble Tea TUI.
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/bootstrap"
	"bnema.dev/dumber/internal/cli/styles"
	"bnema.dev/dumber/internal/domain/build"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/domain/repository"
	"bnema.dev/dumber/internal/infrastructure/config"
	"bnema.dev/dumber/internal/infrastructure/externaltheme/noctalia"
	"bnema.dev/dumber/internal/infrastructure/favicon"
	"bnema.dev/dumber/internal/infrastructure/filesystem"
	infralogging "bnema.dev/dumber/internal/infrastructure/logging"
	"bnema.dev/dumber/internal/infrastructure/persistence/sqlite"
	"bnema.dev/dumber/internal/logging"
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
	ListSessionsUC  *usecase.ListSessionsUseCase
	RestoreUC       *usecase.RestoreSessionUseCase
	DeleteSessionUC *usecase.DeleteSessionUseCase

	// Services
	FaviconService          *favicon.Service
	SessionRestoreLauncher  port.SessionRestoreLauncher
	LocalPaths              port.LocalPathResolver
	NavigationURLNormalizer *usecase.NavigationURLNormalizer

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

	launcher, err := bootstrap.NewSessionRestoreLauncher(cfg, "", "")
	if err != nil {
		return nil, err
	}

	// Create theme through the shared resolver. CLI keeps the historical dark default
	// unless config explicitly requests a light scheme.
	theme := styles.NewThemeFromResolved(resolveCLITheme(cfg))

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
	sessionStateRepo := sqlite.NewSessionStateRepository(db)

	// Create use cases
	searchHistoryUC := usecase.NewSearchHistoryUseCase(historyRepo)
	listSessionsUC := usecase.NewListSessionsUseCase(sessionRepo, sessionStateRepo)
	restoreUC := usecase.NewRestoreSessionUseCase(sessionStateRepo, sessionRepo)
	deleteSessionUC := usecase.NewDeleteSessionUseCase(sessionStateRepo, sessionRepo)

	// Create favicon service for CLI (path resolution for dmenu/fuzzel)
	faviconCacheDir, _ := config.GetFaviconCacheDir()
	faviconService := favicon.NewService(faviconCacheDir)
	localPaths := filesystem.New()

	return &App{
		Config:                  cfg,
		Theme:                   theme,
		db:                      db,
		History:                 historyRepo,
		SearchHistoryUC:         searchHistoryUC,
		SessionUC:               sessionUC,
		ListSessionsUC:          listSessionsUC,
		RestoreUC:               restoreUC,
		DeleteSessionUC:         deleteSessionUC,
		FaviconService:          faviconService,
		SessionRestoreLauncher:  launcher,
		LocalPaths:              localPaths,
		NavigationURLNormalizer: usecase.NewNavigationURLNormalizer(localPaths),
		ctx:                     ctx,
		logCleanup:              logCleanup,
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

func resolveCLITheme(cfg *config.Config) entity.ResolvedTheme {
	colorScheme := ""
	if cfg != nil {
		colorScheme = cfg.Appearance.ColorScheme
	}
	preference := usecase.ColorSchemePreferenceFromConfig(colorScheme, port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "cli",
	})

	var appearance *entity.AppearanceConfig
	var styling *entity.WorkspaceStylingConfig
	var uiScale float64
	if cfg != nil {
		appearance = &cfg.Appearance
		styling = &cfg.Workspace.Styling
		uiScale = cfg.DefaultUIScale
	}

	externalThemeSource := noctalia.NewFileSource(false, "")
	if cfg != nil {
		externalThemeSource.Configure(cfg.Appearance.ExternalTheme)
	}
	uc := usecase.NewResolveThemeUseCase(externalThemeSource)
	out, err := uc.Execute(context.Background(), usecase.ResolveThemeInputFromConfig(
		appearance,
		uiScale,
		styling,
		preference,
	))
	if err != nil {
		return entity.ResolvedTheme{ActivePalette: entity.DefaultDarkPalette()}
	}
	return out.Theme
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

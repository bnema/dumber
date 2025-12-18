package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/cmd"
	"github.com/bnema/dumber/internal/domain/build"
	"github.com/bnema/dumber/internal/infrastructure/clipboard"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/media"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui"
	"github.com/bnema/dumber/internal/ui/theme"
)

// Build-time variables (set via ldflags)
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// initialURL holds the URL to open on startup (from browse command).
var initialURL string

func main() {
	// Run GUI mode for browse command
	if len(os.Args) > 1 && os.Args[1] == "browse" {
		// Extract URL if provided
		if len(os.Args) > 2 {
			initialURL = os.Args[2]
		}
		// Strip "browse" and URL from args so GTK doesn't see them
		os.Args = os.Args[:1]
		runGUI()
		return
	}

	// Pass build info to CLI
	cmd.SetBuildInfo(build.Info{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
		GoVersion: runtime.Version(),
	})

	// Default: run CLI (shows help if no subcommand)
	cmd.Execute()
}

func runGUI() {
	// GTK requires all GTK calls to be made from the main thread
	runtime.LockOSThread()

	// Initialize configuration
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize configuration: %v\n", err)
		os.Exit(1)
	}

	cfg := config.Get()

	// Generate session ID for this browser run
	sessionID := logging.GenerateSessionID()

	// Initialize logger with session file output
	logger, logCleanup, err := logging.NewWithFile(
		logging.Config{
			Level:      logging.ParseLevel(cfg.Logging.Level),
			Format:     cfg.Logging.Format,
			TimeFormat: "15:04:05",
		},
		logging.FileConfig{
			Enabled:       cfg.Logging.EnableFileLog,
			LogDir:        cfg.Logging.LogDir,
			SessionID:     sessionID,
			WriteToStderr: true, // GUI: keep stderr output for debugging
		},
	)
	if err != nil {
		// Fall back to stderr-only logger
		logger = logging.NewFromConfigValues(cfg.Logging.Level, cfg.Logging.Format)
		fmt.Fprintf(os.Stderr, "Warning: failed to create session log file: %v\n", err)
	} else {
		defer logCleanup()
	}

	logger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("build_date", buildDate).
		Msg("starting dumber")

	// Create root context with logger
	ctx := logging.WithContext(context.Background(), logger)

	// Check media playback requirements (GStreamer/VA-API)
	mediaDiagAdapter := media.New()
	checkMediaUC := usecase.NewCheckMediaUseCase(mediaDiagAdapter)
	if _, err := checkMediaUC.Execute(ctx, usecase.CheckMediaInput{
		ShowDiagnostics: cfg.Media.ShowDiagnosticsOnStartup,
	}); err != nil {
		logger.Fatal().Err(err).Msg("media requirements check failed")
	}

	// Resolve data/cache directories for WebKit
	dataDir, err := config.GetDataDir()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to resolve data directory")
	}
	stateDir, err := config.GetStateDir()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to resolve state directory")
	}
	cacheDir := filepath.Join(stateDir, "webkit-cache")
	if mkErr := os.MkdirAll(cacheDir, 0o755); mkErr != nil {
		logger.Fatal().Err(mkErr).Str("path", cacheDir).Msg("failed to create cache directory")
	}

	// Initialize SQLite database (in data dir per XDG spec - user data)
	dbPath := filepath.Join(dataDir, "dumber.db")
	db, err := sqlite.NewConnection(ctx, dbPath)
	if err != nil {
		logger.Fatal().Err(err).Str("path", dbPath).Msg("failed to initialize database")
	}
	defer sqlite.Close(db)

	// Create repositories
	historyRepo := sqlite.NewHistoryRepository(db)
	favoriteRepo := sqlite.NewFavoriteRepository(db)
	folderRepo := sqlite.NewFolderRepository(db)
	tagRepo := sqlite.NewTagRepository(db)
	zoomRepo := sqlite.NewZoomRepository(db)

	// Theme management
	themeManager := theme.NewManager(ctx, cfg)

	// WebKit plumbing
	wkCtx, err := webkit.NewWebKitContext(ctx, dataDir, cacheDir)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize WebKit context")
	}

	// Register dumb:// scheme handler for serving embedded webui
	schemeHandler := webkit.NewDumbSchemeHandler(ctx)
	schemeHandler.SetAssets(assets.WebUIAssets)
	schemeHandler.RegisterWithContext(wkCtx)

	settings := webkit.NewSettingsManager(ctx, cfg)
	injector := webkit.NewContentInjector(themeManager.PrefersDark())

	// Prepare theme CSS for WebUI pages (dumb://*)
	prepareThemeUC := usecase.NewPrepareWebUIThemeUseCase(injector)
	themeCSSVars := themeManager.GetCurrentPalette().ToWebCSSVars()
	if err := prepareThemeUC.Execute(ctx, usecase.PrepareWebUIThemeInput{CSSVars: themeCSSVars}); err != nil {
		logger.Warn().Err(err).Msg("failed to prepare WebUI theme CSS")
	}

	messageRouter := webkit.NewMessageRouter(ctx)
	poolCfg := webkit.DefaultPoolConfig()
	pool := webkit.NewWebViewPool(ctx, wkCtx, settings, poolCfg, injector, messageRouter)

	// Create use cases
	idCounter := uint64(0)
	idGenerator := func() string {
		idCounter++
		return fmt.Sprintf("%c%d", 'a'+rune(idCounter%26), idCounter/26)
	}
	tabsUC := usecase.NewManageTabsUseCase(idGenerator)
	panesUC := usecase.NewManagePanesUseCase(idGenerator)
	historyUC := usecase.NewSearchHistoryUseCase(historyRepo)
	favoritesUC := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)
	zoomUC := usecase.NewManageZoomUseCase(zoomRepo, cfg.DefaultWebpageZoom)
	navigateUC := usecase.NewNavigateUseCase(historyRepo, zoomRepo, cfg.DefaultWebpageZoom)
	clipboardAdapter := clipboard.New()
	copyURLUC := usecase.NewCopyURLUseCase(clipboardAdapter)

	// Build dependencies
	deps := &ui.Dependencies{
		Ctx:           ctx,
		Config:        cfg,
		InitialURL:    initialURL,
		Theme:         themeManager,
		WebContext:    wkCtx,
		Pool:          pool,
		Settings:      settings,
		Injector:      injector,
		MessageRouter: messageRouter,
		// Repositories
		HistoryRepo:  historyRepo,
		FavoriteRepo: favoriteRepo,
		ZoomRepo:     zoomRepo,
		// Use Cases
		TabsUC:      tabsUC,
		PanesUC:     panesUC,
		HistoryUC:   historyUC,
		FavoritesUC: favoritesUC,
		ZoomUC:      zoomUC,
		NavigateUC:  navigateUC,
		CopyURLUC:   copyURLUC,
		// Infrastructure Adapters
		Clipboard: clipboardAdapter,
	}

	// Run the application
	exitCode := ui.RunWithArgs(ctx, deps)
	os.Exit(exitCode)
}

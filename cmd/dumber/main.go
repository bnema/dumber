package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/bootstrap"
	"github.com/bnema/dumber/internal/cli/cmd"
	"github.com/bnema/dumber/internal/domain/build"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/clipboard"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/deps"
	"github.com/bnema/dumber/internal/infrastructure/favicon"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/infrastructure/media"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/rs/zerolog"
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
		os.Exit(runGUI())
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

func runGUI() int {
	// GTK requires all GTK calls to be made from the main thread
	runtime.LockOSThread()

	cfg := initConfig()

	// Apply optional /opt-style runtime prefix overrides (if configured)
	deps.ApplyPrefixEnv(cfg.Runtime.Prefix)

	bootstrapLogger := logging.NewFromConfigValues(cfg.Logging.Level, cfg.Logging.Format)
	bootstrapLogger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("build_date", buildDate).
		Msg("starting dumber")

	ctx := logging.WithContext(context.Background(), bootstrapLogger)

	dataDir, cacheDir := resolveWebKitDirs(bootstrapLogger)

	db, dbCleanup := openDatabase(ctx, bootstrapLogger, dataDir)
	if dbCleanup != nil {
		defer dbCleanup()
	}

	browserSession, sessionCtx, err := bootstrap.StartBrowserSession(ctx, cfg, db)
	if err != nil {
		bootstrapLogger.Fatal().Err(err).Msg("failed to start session")
	}
	if browserSession.LogCleanup != nil {
		defer browserSession.LogCleanup()
	}
	defer func() {
		_ = browserSession.End(sessionCtx)
	}()

	logger := browserSession.Logger
	ctx = sessionCtx

	checkRuntimeRequirements(ctx, cfg, logger)
	checkMediaRequirements(ctx, cfg, logger)

	// Create repositories
	historyRepo := sqlite.NewHistoryRepository(db)
	favoriteRepo := sqlite.NewFavoriteRepository(db)
	folderRepo := sqlite.NewFolderRepository(db)
	tagRepo := sqlite.NewTagRepository(db)
	zoomRepo := sqlite.NewZoomRepository(db)

	// Theme management
	themeManager := theme.NewManager(ctx, cfg)

	stack := bootstrap.BuildWebKitStack(ctx, cfg, dataDir, cacheDir, themeManager, logger)

	// Create use cases
	const idAlphabetSize = 26
	idCounter := uint64(0)
	idGenerator := func() string {
		idCounter++
		return fmt.Sprintf("%c%d", 'a'+rune(idCounter%idAlphabetSize), idCounter/idAlphabetSize)
	}
	tabsUC := usecase.NewManageTabsUseCase(idGenerator)
	panesUC := usecase.NewManagePanesUseCase(idGenerator)
	historyUC := usecase.NewSearchHistoryUseCase(historyRepo)
	favoritesUC := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)
	zoomUC := usecase.NewManageZoomUseCase(zoomRepo, cfg.DefaultWebpageZoom)
	navigateUC := usecase.NewNavigateUseCase(historyRepo, zoomRepo, cfg.DefaultWebpageZoom)
	clipboardAdapter := clipboard.New()
	copyURLUC := usecase.NewCopyURLUseCase(clipboardAdapter)

	// Create favicon service
	faviconCacheDir, _ := config.GetFaviconCacheDir()
	faviconService := favicon.NewService(faviconCacheDir)

	uiDeps := buildUIDependencies(
		ctx,
		cfg,
		themeManager,
		stack.Context,
		stack.Pool,
		stack.Settings,
		stack.Injector,
		stack.MessageRouter,
		stack.FilterManager,
		historyRepo,
		favoriteRepo,
		zoomRepo,
		tabsUC,
		panesUC,
		historyUC,
		favoritesUC,
		zoomUC,
		navigateUC,
		copyURLUC,
		clipboardAdapter,
		faviconService,
	)

	app, err := ui.New(uiDeps)
	if err != nil {
		logger.Error().Err(err).Msg("failed to create application")
		return 1
	}

	// Ensure sessions are closed on Ctrl+C / SIGTERM.
	// Without this, the process may exit immediately and skip defers,
	// leaving the DB session stuck in "active" state.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		logger.Info().Msg("received interrupt, quitting")
		app.Quit()
	}()

	// Run the application
	exitCode := app.Run(ctx, os.Args)
	return exitCode
}

func initConfig() *config.Config {
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize configuration: %v\n", err)
		os.Exit(1)
	}
	return config.Get()
}

func checkRuntimeRequirements(ctx context.Context, cfg *config.Config, logger zerolog.Logger) {
	probe := deps.NewPkgConfigProbe()
	checkRuntimeUC := usecase.NewCheckRuntimeDependenciesUseCase(probe)
	runtimeOut, err := checkRuntimeUC.Execute(ctx, usecase.CheckRuntimeDependenciesInput{
		Prefix: cfg.Runtime.Prefix,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("runtime requirements check failed")
	}
	if runtimeOut.OK {
		return
	}
	for _, c := range runtimeOut.Checks {
		if c.Installed {
			logger.Error().
				Str("dependency", c.PkgConfigName).
				Str("have", c.Version).
				Str("need", c.RequiredVersion).
				Bool("ok", c.MeetsRequirement).
				Msg("runtime dependency")
		} else {
			logger.Error().
				Str("dependency", c.PkgConfigName).
				Str("need", c.RequiredVersion).
				Msg("runtime dependency missing")
		}
	}
	logger.Fatal().
		Str("hint", "Run: dumber doctor (and set runtime.prefix for /opt installs)").
		Msg("runtime requirements not met")
}

func checkMediaRequirements(ctx context.Context, cfg *config.Config, logger zerolog.Logger) {
	mediaDiagAdapter := media.New()
	checkMediaUC := usecase.NewCheckMediaUseCase(mediaDiagAdapter)
	if _, mediaErr := checkMediaUC.Execute(ctx, usecase.CheckMediaInput{
		ShowDiagnostics: cfg.Media.ShowDiagnosticsOnStartup,
	}); mediaErr != nil {
		logger.Fatal().Err(mediaErr).Msg("media requirements check failed")
	}
}

func resolveWebKitDirs(logger zerolog.Logger) (string, string) {
	const cacheDirPerm = 0o755
	dataDir, err := config.GetDataDir()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to resolve data directory")
	}
	stateDir, err := config.GetStateDir()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to resolve state directory")
	}
	cacheDir := filepath.Join(stateDir, "webkit-cache")
	if mkErr := os.MkdirAll(cacheDir, cacheDirPerm); mkErr != nil {
		logger.Fatal().Err(mkErr).Str("path", cacheDir).Msg("failed to create cache directory")
	}
	return dataDir, cacheDir
}

func openDatabase(ctx context.Context, logger zerolog.Logger, dataDir string) (*sql.DB, func()) {
	dbPath := filepath.Join(dataDir, "dumber.db")
	db, err := sqlite.NewConnection(ctx, dbPath)
	if err != nil {
		logger.Fatal().Err(err).Str("path", dbPath).Msg("failed to initialize database")
	}
	cleanup := func() {
		if err := sqlite.Close(db); err != nil {
			logger.Error().Err(err).Msg("failed to close database")
		}
	}
	return db, cleanup
}

func buildUIDependencies(
	ctx context.Context,
	cfg *config.Config,
	themeManager *theme.Manager,
	wkCtx *webkit.WebKitContext,
	pool *webkit.WebViewPool,
	settings *webkit.SettingsManager,
	injector *webkit.ContentInjector,
	messageRouter *webkit.MessageRouter,
	filterManager *filtering.Manager,
	historyRepo repository.HistoryRepository,
	favoriteRepo repository.FavoriteRepository,
	zoomRepo repository.ZoomRepository,
	tabsUC *usecase.ManageTabsUseCase,
	panesUC *usecase.ManagePanesUseCase,
	historyUC *usecase.SearchHistoryUseCase,
	favoritesUC *usecase.ManageFavoritesUseCase,
	zoomUC *usecase.ManageZoomUseCase,
	navigateUC *usecase.NavigateUseCase,
	copyURLUC *usecase.CopyURLUseCase,
	clipboardAdapter port.Clipboard,
	faviconService *favicon.Service,
) *ui.Dependencies {
	return &ui.Dependencies{
		Ctx:            ctx,
		Config:         cfg,
		InitialURL:     initialURL,
		Theme:          themeManager,
		WebContext:     wkCtx,
		Pool:           pool,
		Settings:       settings,
		Injector:       injector,
		MessageRouter:  messageRouter,
		HistoryRepo:    historyRepo,
		FavoriteRepo:   favoriteRepo,
		ZoomRepo:       zoomRepo,
		TabsUC:         tabsUC,
		PanesUC:        panesUC,
		HistoryUC:      historyUC,
		FavoritesUC:    favoritesUC,
		ZoomUC:         zoomUC,
		NavigateUC:     navigateUC,
		CopyURLUC:      copyURLUC,
		Clipboard:      clipboardAdapter,
		FaviconService: faviconService,
		FilterManager:  filterManager,
	}
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
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

func main() {
	// GTK requires all GTK calls to be made from the main thread
	runtime.LockOSThread()

	// Initialize configuration
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize configuration: %v\n", err)
		os.Exit(1)
	}

	cfg := config.Get()

	// Initialize logger from config
	logger := logging.NewFromConfigValues(cfg.Logging.Level, cfg.Logging.Format)
	logger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("build_date", buildDate).
		Msg("starting dumber")

	// Create root context with logger
	ctx := logging.WithContext(context.Background(), logger)

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

	// Initialize SQLite database
	dbPath := filepath.Join(stateDir, "dumber.db")
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
	settings := webkit.NewSettingsManager(ctx, cfg)
	injector := webkit.NewContentInjector(themeManager.PrefersDark())
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

	// Build dependencies
	deps := &ui.Dependencies{
		Ctx:           ctx,
		Config:        cfg,
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
	}

	// Run the application
	exitCode := ui.RunWithArgs(ctx, deps)
	os.Exit(exitCode)
}

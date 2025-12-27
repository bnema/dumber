package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/bootstrap"
	"github.com/bnema/dumber/internal/cli/cmd"
	"github.com/bnema/dumber/internal/domain/build"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/cache"
	"github.com/bnema/dumber/internal/infrastructure/clipboard"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/deps"
	"github.com/bnema/dumber/internal/infrastructure/favicon"
	"github.com/bnema/dumber/internal/infrastructure/idle"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/infrastructure/updater"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui"
	"github.com/bnema/dumber/internal/ui/theme"
)

// Build-time variables (set via ldflags).
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// initialURL holds the URL to open on startup (from browse command).
var initialURL string

// restoreSessionID holds the session ID to restore on startup.
var restoreSessionID string

func main() {
	// Run GUI mode for browse command
	if len(os.Args) > 1 && os.Args[1] == "browse" {
		if len(os.Args) > 2 {
			initialURL = os.Args[2]
		}
		restoreSessionID = os.Getenv("DUMBER_RESTORE_SESSION")
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
	runtime.LockOSThread()
	timer := bootstrap.NewStartupTimer()

	cfg := initConfig()
	timer.Mark("config")

	deps.ApplyPrefixEnv(cfg.Runtime.Prefix)
	bootstrapLogger := logging.NewFromConfigValues(cfg.Logging.Level, cfg.Logging.Format)
	bootstrapLogger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("build_date", buildDate).
		Msg("starting dumber")
	ctx := logging.WithContext(context.Background(), bootstrapLogger)
	timer.Mark("logger")

	// Parallel phase: directories, runtime/media checks, theme, WASM precompile
	initResult, err := bootstrap.RunParallelInit(bootstrap.ParallelInitInput{
		Ctx:    ctx,
		Config: cfg,
	})
	if err != nil {
		handleParallelInitError(ctx, err)
		return 1
	}
	timer.MarkDuration("parallel_phase", initResult.Duration)

	// Parallel phase 2: Database + WebKit stack initialize concurrently
	dbWebKit, err := bootstrap.RunParallelDBWebKit(bootstrap.ParallelDBWebKitInput{
		Ctx:          ctx,
		Config:       cfg,
		DataDir:      initResult.DataDir,
		CacheDir:     initResult.CacheDir,
		ThemeManager: initResult.ThemeManager,
	})
	if err != nil {
		bootstrapLogger.Fatal().Err(err).Msg("failed to initialize database/webkit")
	}
	if dbWebKit.DBCleanup != nil {
		defer dbWebKit.DBCleanup()
	}
	db := dbWebKit.DB
	stack := dbWebKit.Stack
	timer.Mark("db_webkit_parallel")

	browserSession, sessionCtx, err := bootstrap.StartBrowserSession(ctx, cfg, db)
	if err != nil {
		bootstrapLogger.Fatal().Err(err).Msg("failed to start session")
	}
	if browserSession.LogCleanup != nil {
		defer browserSession.LogCleanup()
	}
	defer func() { _ = browserSession.End(sessionCtx) }()
	timer.Mark("session")

	ctx = sessionCtx
	log := logging.FromContext(ctx)

	// Repositories and use cases
	repos := createRepositories(db)
	useCases := createUseCases(repos, cfg)
	handleAutoRestore(ctx, cfg, useCases, browserSession.Session.ID)

	idleInhibitor := idle.NewPortalInhibitor(ctx)
	defer func() {
		if idleInhibitor != nil {
			_ = idleInhibitor.Close()
		}
	}()
	timer.Mark("use_cases")

	// Build UI
	uiDeps := buildUIDependencies(ctx, cfg, initResult.ThemeManager, &stack, repos, useCases, idleInhibitor, browserSession.Session.ID)
	app, err := ui.New(uiDeps)
	if err != nil {
		log.Error().Err(err).Msg("failed to create application")
		return 1
	}
	timer.Mark("ui_deps")
	timer.Log(ctx)

	// Signal handling
	setupSignalHandler(ctx, app)

	return app.Run(ctx, os.Args)
}

func initConfig() *config.Config {
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize configuration: %v\n", err)
		os.Exit(1)
	}
	return config.Get()
}

func handleParallelInitError(ctx context.Context, err error) {
	log := logging.FromContext(ctx)
	if runtimeErr, ok := err.(*bootstrap.RuntimeRequirementsError); ok {
		runtimeErr.LogDetails(ctx)
		log.Fatal().Err(runtimeErr).
			Str("hint", "Run: dumber doctor (and set runtime.prefix for /opt installs)").
			Msg("runtime requirements not met")
	}
	log.Fatal().Err(err).Msg("initialization failed")
}

func handleAutoRestore(
	ctx context.Context,
	cfg *config.Config,
	uc *useCases,
	currentSessionID entity.SessionID,
) {
	if !cfg.Session.AutoRestore || restoreSessionID != "" {
		return
	}
	log := logging.FromContext(ctx)
	out, err := uc.lastRestorable.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: currentSessionID,
	})
	if err != nil {
		log.Warn().Err(err).Msg("auto-restore: failed to find restorable session")
		return
	}
	if out.SessionID != "" {
		restoreSessionID = string(out.SessionID)
		log.Info().
			Str("session_id", restoreSessionID).
			Int("tabs", len(out.State.Tabs)).
			Msg("auto-restore: found last session")
	}
}

func setupSignalHandler(ctx context.Context, app *ui.App) {
	log := logging.FromContext(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		signal.Stop(sigCh)
		log.Info().Str("signal", sig.String()).Msg("received interrupt, quitting")
		app.Quit()
	}()
}

// repositories groups infrastructure layer repository implementations.
type repositories struct {
	history      repository.HistoryRepository
	favorite     repository.FavoriteRepository
	folder       repository.FolderRepository
	tag          repository.TagRepository
	zoom         repository.ZoomRepository
	session      repository.SessionRepository
	sessionState repository.SessionStateRepository
}

func createRepositories(db *sql.DB) *repositories {
	return &repositories{
		history:      sqlite.NewHistoryRepository(db),
		favorite:     sqlite.NewFavoriteRepository(db),
		folder:       sqlite.NewFolderRepository(db),
		tag:          sqlite.NewTagRepository(db),
		zoom:         sqlite.NewZoomRepository(db),
		session:      sqlite.NewSessionRepository(db),
		sessionState: sqlite.NewSessionStateRepository(db),
	}
}

// useCases groups application layer use case implementations.
type useCases struct {
	tabs           *usecase.ManageTabsUseCase
	panes          *usecase.ManagePanesUseCase
	history        *usecase.SearchHistoryUseCase
	favorites      *usecase.ManageFavoritesUseCase
	zoom           *usecase.ManageZoomUseCase
	navigate       *usecase.NavigateUseCase
	copyURL        *usecase.CopyURLUseCase
	snapshot       *usecase.SnapshotSessionUseCase
	lastRestorable *usecase.GetLastRestorableSessionUseCase
	checkUpdate    *usecase.CheckUpdateUseCase
	applyUpdate    *usecase.ApplyUpdateUseCase
	clipboard      port.Clipboard
	favicon        *favicon.Service
}

func createUseCases(repos *repositories, cfg *config.Config) *useCases {
	const idAlphabetSize = 26
	idCounter := uint64(0)
	idGenerator := func() string {
		idCounter++
		return fmt.Sprintf("%c%d", 'a'+rune(idCounter%idAlphabetSize), idCounter/idAlphabetSize)
	}

	clipboardAdapter := clipboard.New()
	faviconCacheDir, _ := config.GetFaviconCacheDir()
	xdgDirs, _ := config.GetXDGDirs()
	stateDir, _ := config.GetStateDir()
	defaultZoom := cfg.DefaultWebpageZoom
	zoomCache := cache.NewLRU[string, *entity.ZoomLevel](cfg.Performance.ZoomCacheSize)

	buildInfo := build.Info{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
		GoVersion: runtime.Version(),
	}
	updateChecker := updater.NewGitHubChecker()
	updateDownloader := updater.NewGitHubDownloader()
	updateApplier := updater.NewApplier(stateDir)

	return &useCases{
		tabs:           usecase.NewManageTabsUseCase(idGenerator),
		panes:          usecase.NewManagePanesUseCase(idGenerator),
		history:        usecase.NewSearchHistoryUseCase(repos.history),
		favorites:      usecase.NewManageFavoritesUseCase(repos.favorite, repos.folder, repos.tag),
		zoom:           usecase.NewManageZoomUseCase(repos.zoom, defaultZoom, zoomCache),
		navigate:       usecase.NewNavigateUseCase(repos.history, repos.zoom, defaultZoom),
		copyURL:        usecase.NewCopyURLUseCase(clipboardAdapter),
		snapshot:       usecase.NewSnapshotSessionUseCase(repos.sessionState),
		lastRestorable: usecase.NewGetLastRestorableSessionUseCase(repos.session, repos.sessionState, stateDir),
		checkUpdate:    usecase.NewCheckUpdateUseCase(updateChecker, updateApplier, buildInfo),
		applyUpdate:    usecase.NewApplyUpdateUseCase(updateDownloader, updateApplier, xdgDirs.CacheHome),
		clipboard:      clipboardAdapter,
		favicon:        favicon.NewService(faviconCacheDir),
	}
}

func buildUIDependencies(
	ctx context.Context,
	cfg *config.Config,
	themeManager *theme.Manager,
	stack *bootstrap.WebKitStack,
	repos *repositories,
	uc *useCases,
	idleInhibitor port.IdleInhibitor,
	currentSessionID entity.SessionID,
) *ui.Dependencies {
	return &ui.Dependencies{
		Ctx:              ctx,
		Config:           cfg,
		InitialURL:       initialURL,
		RestoreSessionID: restoreSessionID,
		Theme:            themeManager,
		WebContext:       stack.Context,
		Pool:             stack.Pool,
		Settings:         stack.Settings,
		Injector:         stack.Injector,
		MessageRouter:    stack.MessageRouter,
		FilterManager:    stack.FilterManager,
		HistoryRepo:      repos.history,
		FavoriteRepo:     repos.favorite,
		ZoomRepo:         repos.zoom,
		TabsUC:           uc.tabs,
		PanesUC:          uc.panes,
		HistoryUC:        uc.history,
		FavoritesUC:      uc.favorites,
		ZoomUC:           uc.zoom,
		NavigateUC:       uc.navigate,
		CopyURLUC:        uc.copyURL,
		Clipboard:        uc.clipboard,
		FaviconService:   uc.favicon,
		IdleInhibitor:    idleInhibitor,
		SessionRepo:      repos.session,
		SessionStateRepo: repos.sessionState,
		CurrentSessionID: currentSessionID,
		SnapshotUC:       uc.snapshot,
		CheckUpdateUC:    uc.checkUpdate,
		ApplyUpdateUC:    uc.applyUpdate,
	}
}

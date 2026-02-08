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
	"github.com/bnema/dumber/internal/infrastructure/colorscheme"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/deps"
	"github.com/bnema/dumber/internal/infrastructure/favicon"
	"github.com/bnema/dumber/internal/infrastructure/idle"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/infrastructure/updater"
	"github.com/bnema/dumber/internal/infrastructure/xdg"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/rs/zerolog"
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
	enableCrashForensics()

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
	component.SetSkeletonVersion(version)
	timer := bootstrap.NewStartupTimer()

	cfg := initConfig()
	timer.Mark("config")

	ctx := initStartupContextWithTrace(cfg)
	timer.Mark("logger")
	bootstrapLog := logging.FromContext(ctx)

	initResult, err := runParallelInitPhase(ctx, cfg)
	if err != nil {
		return 1
	}
	timer.MarkDuration("parallel_phase", initResult.Duration)

	needsEagerDB := restoreSessionID != "" || cfg.Session.AutoRestore

	stack, repos, dbCleanup, err := initStackAndRepos(ctx, cfg, initResult, needsEagerDB)
	if err != nil {
		bootstrapLog.Fatal().Err(err).Msg("failed to initialize database/webkit")
	}
	if dbCleanup != nil {
		defer dbCleanup()
	}
	timer.Mark("db_webkit_parallel")

	ctx, browserSession, sessionCleanup := initBrowserSession(ctx, cfg, repos, bootstrapLog)
	defer sessionCleanup()
	timer.Mark("session")

	log := logging.FromContext(ctx)
	logging.Trace().UpdateLogger(log)
	logCoreDumpLimits(ctx)

	if stack.MessageRouter != nil {
		stack.MessageRouter.SetBaseContext(ctx)
	}

	useCases := createUseCases(repos, cfg)
	if needsEagerDB {
		handleAutoRestore(ctx, cfg, useCases, browserSession.Session.ID)
	}

	idleInhibitor := idle.NewPortalInhibitor(ctx)
	defer closeIdleInhibitor(idleInhibitor)
	timer.Mark("use_cases")

	app, err := buildAndConfigureApp(ctx, cfg, initResult, &stack, repos, useCases, idleInhibitor, browserSession)
	if err != nil {
		log.Error().Err(err).Msg("failed to create application")
		return 1
	}
	timer.Mark("ui_deps")
	timer.Log(ctx)

	setupSignalHandler(ctx, app)

	return app.Run(ctx, os.Args)
}

func initStartupContextWithTrace(cfg *config.Config) context.Context {
	logging.InitStartupTrace(cfg.Logging.Level)
	logging.Trace().Mark("config_loaded")

	ctx := initStartupContext(cfg)
	bootstrapLog := logging.FromContext(ctx)

	logging.Trace().SetLogger(bootstrapLog)
	logging.Trace().Mark("logger_init")

	return ctx
}

func runParallelInitPhase(ctx context.Context, cfg *config.Config) (*bootstrap.ParallelInitResult, error) {
	logging.Trace().Mark("parallel_start")
	initResult, err := bootstrap.RunParallelInit(bootstrap.ParallelInitInput{
		Ctx:    ctx,
		Config: cfg,
	})
	if err != nil {
		handleParallelInitError(ctx, err)
		return nil, err
	}
	logging.Trace().Mark("parallel_done")
	return initResult, nil
}

func initBrowserSession(
	ctx context.Context,
	cfg *config.Config,
	repos *repositories,
	bootstrapLog *zerolog.Logger,
) (context.Context, *bootstrap.BrowserSession, func()) {
	deferPersist := restoreSessionID == "" && !cfg.Session.AutoRestore
	browserSession, sessionCtx, err := bootstrap.StartBrowserSession(ctx, cfg, repos.session, deferPersist)
	if err != nil {
		bootstrapLog.Fatal().Err(err).Msg("failed to start session")
	}
	cleanup := func() {
		if browserSession.LogCleanup != nil {
			browserSession.LogCleanup()
		}
		_ = browserSession.End(sessionCtx)
	}
	return sessionCtx, browserSession, cleanup
}

func closeIdleInhibitor(inhibitor port.IdleInhibitor) {
	if inhibitor != nil {
		_ = inhibitor.Close()
	}
}

func buildAndConfigureApp(
	ctx context.Context,
	cfg *config.Config,
	initResult *bootstrap.ParallelInitResult,
	stack *bootstrap.WebKitStack,
	repos *repositories,
	useCases *useCases,
	idleInhibitor port.IdleInhibitor,
	browserSession *bootstrap.BrowserSession,
) (*ui.App, error) {
	uiDeps := buildUIDependencies(
		ctx, cfg, initResult.ThemeManager,
		initResult.ColorResolver, initResult.AdwaitaDetector,
		stack, repos, useCases, idleInhibitor, browserSession.Session.ID, browserSession.UnexpectedCloseReports(),
	)
	configureDeferredInit(uiDeps, cfg, browserSession)
	return ui.New(uiDeps)
}

func initStartupContext(cfg *config.Config) context.Context {
	deps.ApplyPrefixEnv(cfg.Runtime.Prefix)
	bootstrapLogger := logging.NewFromConfigValuesWithTimeFormat(
		cfg.Logging.Level,
		cfg.Logging.Format,
		logging.ConsoleTimeFormat,
	)
	bootstrapLogger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("build_date", buildDate).
		Msg("starting dumber")
	ctx := logging.WithContext(context.Background(), bootstrapLogger)
	return ctx
}

func initStackAndRepos(
	ctx context.Context,
	cfg *config.Config,
	initResult *bootstrap.ParallelInitResult,
	needsEagerDB bool,
) (bootstrap.WebKitStack, *repositories, func(), error) {
	if needsEagerDB {
		// Parallel phase 2: Database + WebKit stack initialize concurrently
		dbWebKit, err := bootstrap.RunParallelDBWebKit(bootstrap.ParallelDBWebKitInput{
			Ctx:           ctx,
			Config:        cfg,
			DataDir:       initResult.DataDir,
			CacheDir:      initResult.CacheDir,
			ThemeManager:  initResult.ThemeManager,
			ColorResolver: initResult.ColorResolver,
		})
		if err != nil {
			return bootstrap.WebKitStack{}, nil, nil, err
		}
		return dbWebKit.Stack, createRepositories(dbWebKit.DB), dbWebKit.DBCleanup, nil
	}

	log := logging.FromContext(ctx)
	stack := bootstrap.BuildWebKitStack(bootstrap.WebKitStackInput{
		Ctx:           ctx,
		Config:        cfg,
		DataDir:       initResult.DataDir,
		CacheDir:      initResult.CacheDir,
		ThemeManager:  initResult.ThemeManager,
		ColorResolver: initResult.ColorResolver,
		Logger:        *log,
	})

	lazyDB, err := bootstrap.CreateLazyDatabase()
	if err != nil {
		return stack, nil, nil, err
	}
	dbCleanup := func() { _ = lazyDB.Close() }
	return stack, createLazyRepositories(lazyDB), dbCleanup, nil
}

func configureDeferredInit(
	uiDeps *ui.Dependencies,
	cfg *config.Config,
	session *bootstrap.BrowserSession,
) {
	if uiDeps == nil {
		return
	}

	// If session was already persisted (not deferred), mark snapshot service ready immediately
	// via callback that will be set by the app after initialization
	sessionAlreadyPersisted := session == nil || session.Persist == nil

	uiDeps.OnFirstWebViewShown = func(cbCtx context.Context) {
		logger := logging.FromContext(cbCtx)
		bgCtx := logging.WithContext(context.Background(), *logger)
		go func() {
			result := bootstrap.RunDeferredInit(bootstrap.DeferredInitInput{
				Ctx:    bgCtx,
				Config: cfg,
			})
			logDeferredInitResults(bgCtx, result)
		}()

		if sessionAlreadyPersisted {
			// Session was persisted eagerly, notify immediately
			if uiDeps.OnSessionPersisted != nil {
				uiDeps.OnSessionPersisted()
			}
		} else if session.Persist != nil {
			// Session persist is deferred, notify after it completes
			go func() {
				if persistErr := session.Persist(bgCtx); persistErr != nil {
					logger.Error().Err(persistErr).Msg("deferred session persistence failed")
				} else {
					if uiDeps.OnCrashReportsDetected != nil {
						if reports := session.UnexpectedCloseReports(); len(reports) > 0 {
							uiDeps.OnCrashReportsDetected(reports)
						}
					}
					if uiDeps.OnSessionPersisted != nil {
						uiDeps.OnSessionPersisted()
					}
				}
			}()
		}
	}
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

func logDeferredInitResults(ctx context.Context, result bootstrap.DeferredInitResult) {
	log := logging.FromContext(ctx)
	if result.SQLiteErr != nil {
		log.Warn().Err(result.SQLiteErr).Msg("deferred sqlite wasm init failed")
	}
	if result.RuntimeErr != nil {
		if runtimeErr, ok := result.RuntimeErr.(*bootstrap.RuntimeRequirementsError); ok {
			runtimeErr.LogDetails(ctx)
			log.Warn().Err(runtimeErr).
				Str("hint", "Run: dumber doctor (and set runtime.prefix for /opt installs)").
				Msg("runtime requirements not met")
		} else {
			log.Warn().Err(result.RuntimeErr).Msg("runtime requirements check failed")
		}
	}
	if result.MediaErr != nil {
		log.Warn().Err(result.MediaErr).Msg("media check failed")
	}
	log.Debug().Dur("duration", result.Duration).Msg("deferred init complete")
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
	permission   port.PermissionRepository
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
		permission:   sqlite.NewPermissionRepository(db),
		session:      sqlite.NewSessionRepository(db),
		sessionState: sqlite.NewSessionStateRepository(db),
	}
}

func createLazyRepositories(provider port.DatabaseProvider) *repositories {
	return &repositories{
		history:      sqlite.NewLazyHistoryRepository(provider),
		favorite:     sqlite.NewLazyFavoriteRepository(provider),
		folder:       sqlite.NewLazyFolderRepository(provider),
		tag:          sqlite.NewLazyTagRepository(provider),
		zoom:         sqlite.NewLazyZoomRepository(provider),
		permission:   sqlite.NewLazyPermissionRepository(provider),
		session:      sqlite.NewLazySessionRepository(provider),
		sessionState: sqlite.NewLazySessionStateRepository(provider),
	}
}

// useCases groups application layer use case implementations.
type useCases struct {
	tabs           *usecase.ManageTabsUseCase
	panes          *usecase.ManagePanesUseCase
	history        *usecase.SearchHistoryUseCase
	favorites      *usecase.ManageFavoritesUseCase
	zoom           *usecase.ManageZoomUseCase
	permission     *usecase.HandlePermissionUseCase
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

	// Permission use case will be initialized later with dialog presenter
	permissionUC := usecase.NewHandlePermissionUseCase(repos.permission, nil, logging.FromContext)

	return &useCases{
		tabs:           usecase.NewManageTabsUseCase(idGenerator),
		panes:          usecase.NewManagePanesUseCase(idGenerator),
		history:        usecase.NewSearchHistoryUseCase(repos.history),
		favorites:      usecase.NewManageFavoritesUseCase(repos.favorite, repos.folder, repos.tag),
		zoom:           usecase.NewManageZoomUseCase(repos.zoom, defaultZoom, zoomCache),
		permission:     permissionUC,
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
	colorResolver port.ColorSchemeResolver,
	adwaitaDetector *colorscheme.AdwaitaDetector,
	stack *bootstrap.WebKitStack,
	repos *repositories,
	uc *useCases,
	idleInhibitor port.IdleInhibitor,
	currentSessionID entity.SessionID,
	startupCrashReports []string,
) *ui.Dependencies {
	return &ui.Dependencies{
		Ctx:              ctx,
		Config:           cfg,
		InitialURL:       initialURL,
		RestoreSessionID: restoreSessionID,
		Theme:            themeManager,
		ColorResolver:    colorResolver,
		AdwaitaDetector:  adwaitaDetector,
		XDG:              xdg.New(),
		WebContext:       stack.Context,
		Pool:             stack.Pool,
		Settings:         stack.Settings,
		Injector:         stack.Injector,
		MessageRouter:    stack.MessageRouter,
		FilterManager:    stack.FilterManager,
		HistoryRepo:      repos.history,
		FavoriteRepo:     repos.favorite,
		ZoomRepo:         repos.zoom,
		PermissionRepo:   repos.permission,
		TabsUC:           uc.tabs,
		PanesUC:          uc.panes,
		HistoryUC:        uc.history,
		FavoritesUC:      uc.favorites,
		ZoomUC:           uc.zoom,
		PermissionUC:     uc.permission,
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
		Ctx:                 ctx,
		Config:              cfg,
		InitialURL:          initialURL,
		RestoreSessionID:    restoreSessionID,
		StartupCrashReports: startupCrashReports,
		Theme:               themeManager,
		ColorResolver:       colorResolver,
		AdwaitaDetector:     adwaitaDetector,
		XDG:                 xdg.New(),
		WebContext:          stack.Context,
		Pool:                stack.Pool,
		Settings:            stack.Settings,
		Injector:            stack.Injector,
		MessageRouter:       stack.MessageRouter,
		FilterManager:       stack.FilterManager,
		HistoryRepo:         repos.history,
		FavoriteRepo:        repos.favorite,
		ZoomRepo:            repos.zoom,
		TabsUC:              uc.tabs,
		PanesUC:             uc.panes,
		HistoryUC:           uc.history,
		FavoritesUC:         uc.favorites,
		ZoomUC:              uc.zoom,
		NavigateUC:          uc.navigate,
		CopyURLUC:           uc.copyURL,
		Clipboard:           uc.clipboard,
		FaviconService:      uc.favicon,
		IdleInhibitor:       idleInhibitor,
		SessionRepo:         repos.session,
		SessionStateRepo:    repos.sessionState,
		CurrentSessionID:    currentSessionID,
		SnapshotUC:          uc.snapshot,
		CheckUpdateUC:       uc.checkUpdate,
		ApplyUpdateUC:       uc.applyUpdate,
	}
}

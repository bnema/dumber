package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/infrastructure/snapshot"
	"github.com/bnema/dumber/internal/infrastructure/webkit"

	"github.com/bnema/dumber/internal/infrastructure/webkit/handlers"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/dispatcher"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	// AppID is the application identifier for GTK.
	AppID = "com.github.bnema.dumber"
)

// App wraps the GTK Application and manages the browser lifecycle.
type App struct {
	deps       *Dependencies
	gtkApp     *gtk.Application
	mainWindow *window.MainWindow

	// State
	tabs   *entity.TabList
	tabsUC *usecase.ManageTabsUseCase

	// Coordinators (new architecture)
	contentCoord  *coordinator.ContentCoordinator
	tabCoord      *coordinator.TabCoordinator
	wsCoord       *coordinator.WorkspaceCoordinator
	navCoord      *coordinator.NavigationCoordinator
	kbDispatcher  *dispatcher.KeyboardDispatcher
	configManager *config.Manager

	// Pane management (used by coordinators)
	panesUC        *usecase.ManagePanesUseCase
	workspaceViews map[entity.TabID]*component.WorkspaceView
	widgetFactory  layout.WidgetFactory
	stackedPaneMgr *component.StackedPaneManager

	// Input handling
	keyboardHandler       *input.KeyboardHandler
	globalShortcutHandler *input.GlobalShortcutHandler

	// Focus and border management
	focusMgr  *focus.Manager
	borderMgr *focus.BorderManager

	resizeModeBorderTarget layout.Widget

	// Omnibox configuration (omnibox is created per workspace view)
	omniboxCfg component.OmniboxConfig
	// Find bar configuration (find bar is created per workspace view)
	findBarCfg component.FindBarConfig

	// Web content (managed by ContentCoordinator)
	pool           *webkit.WebViewPool
	webViewFactory *webkit.WebViewFactory
	injector       *webkit.ContentInjector
	router         *webkit.MessageRouter
	settings       *webkit.SettingsManager
	faviconAdapter *adapter.FaviconAdapter

	// App-level toaster for system notifications (filter status, etc.)
	appToaster *component.Toaster

	// Session management
	sessionManager  *component.SessionManager
	snapshotService *snapshot.Service

	// Update management
	updateCoord *coordinator.UpdateCoordinator

	// ID generator for tabs/panes
	idCounter uint64
	idMu      sync.Mutex

	// lifecycle
	cancel context.CancelCauseFunc
}

// New creates a new App with the given dependencies.
func New(deps *Dependencies) (*App, error) {
	if err := deps.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancelCause(deps.Ctx)

	app := &App{
		deps:           deps,
		tabs:           entity.NewTabList(),
		tabsUC:         deps.TabsUC,
		panesUC:        deps.PanesUC,
		workspaceViews: make(map[entity.TabID]*component.WorkspaceView),
		pool:           deps.Pool,
		injector:       deps.Injector,
		router:         deps.MessageRouter,
		settings:       deps.Settings,
		configManager:  config.GetManager(),
		cancel:         cancel,
	}
	if app.router == nil {
		app.router = webkit.NewMessageRouter(ctx)
	}

	// Register message handlers
	if app.router != nil {
		if err := handlers.RegisterAll(ctx, app.router, handlers.Config{
			HistoryUC:   deps.HistoryUC,
			FavoritesUC: deps.FavoritesUC,
		}); err != nil {
			return nil, err
		}
	}

	return app, nil
}

// Run starts the GTK application and blocks until it exits.
// Returns the exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating GTK application")

	// TODO: Use AppID once puregotk GC bug is fixed (nullable-string-gc-memory-corruption)
	a.gtkApp = gtk.NewApplication(nil, gio.GApplicationFlagsNoneValue)
	if a.gtkApp == nil {
		log.Error().Msg("failed to create GTK application")
		return 1
	}
	defer a.gtkApp.Unref()

	// Connect activate signal
	activateCb := func(_ gio.Application) {
		a.onActivate(ctx)
	}
	a.gtkApp.ConnectActivate(&activateCb)

	// Connect shutdown signal
	shutdownCb := func(_ gio.Application) {
		a.onShutdown(ctx)
	}
	a.gtkApp.ConnectShutdown(&shutdownCb)

	log.Info().Msg("starting GTK main loop")
	code := a.gtkApp.Run(len(args), args)

	return code
}

// onActivate is called when the GTK application is activated.
func (a *App) onActivate(ctx context.Context) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("GTK application activated")

	a.applyGTKColorSchemePreference(ctx)
	a.prewarmWebViewPool(ctx)

	if err := a.createMainWindow(ctx); err != nil {
		log.Error().Err(err).Msg("failed to create main window")
		return
	}

	a.initLayoutInfrastructure()
	a.initAppToasterOverlay()
	a.initFocusAndBorderOverlay()

	a.initCoordinators(ctx)
	a.initKeyboardHandler(ctx)
	a.initOmniboxConfig(ctx)
	a.initFindBarConfig()
	a.initSessionManager(ctx)
	a.initSnapshotService(ctx)
	a.initUpdateCoordinator(ctx)
	a.createInitialTab(ctx)
	a.finalizeActivation(ctx)
}

func (a *App) applyGTKColorSchemePreference(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Propagate app color scheme preference to GTK (and WebKit)
	// so web content can correctly evaluate prefers-color-scheme.
	settings := gtk.SettingsGetDefault()
	if settings == nil {
		return
	}

	scheme := "default"
	if a.deps != nil && a.deps.Config != nil && a.deps.Config.Appearance.ColorScheme != "" {
		scheme = a.deps.Config.Appearance.ColorScheme
	}
	prefersDark := theme.ResolveColorScheme(scheme)
	settings.SetPropertyGtkApplicationPreferDarkTheme(prefersDark)
	log.Debug().
		Str("scheme", scheme).
		Bool("prefers_dark", prefersDark).
		Msg("set gtk-application-prefer-dark-theme")
}

func (a *App) prewarmWebViewPool(ctx context.Context) {
	// Set theme background color on pool to eliminate white flash.
	// Must be done before prewarming so WebViews get the correct color.
	if a.pool != nil && a.deps != nil && a.deps.Theme != nil {
		r, g, b, alpha := a.deps.Theme.GetBackgroundRGBA()
		a.pool.SetBackgroundColor(r, g, b, alpha)
	}

	// Prewarm WebView pool now that GTK is initialized.
	if a.pool == nil {
		return
	}
	a.pool.Prewarm(ctx, 0)
}

func (a *App) createMainWindow(ctx context.Context) error {
	mainWindow, err := window.New(ctx, a.gtkApp, a.deps.Config)
	if err != nil {
		return err
	}
	a.mainWindow = mainWindow

	// Apply GTK CSS styling from theme manager.
	if a.deps == nil || a.deps.Theme == nil {
		return nil
	}
	if display := a.mainWindow.Window().GetDisplay(); display != nil {
		a.deps.Theme.ApplyToDisplay(ctx, display)
	}
	return nil
}

func (a *App) initLayoutInfrastructure() {
	// Initialize widget factory for pane layout.
	a.widgetFactory = layout.NewGtkWidgetFactory()

	// Initialize stacked pane manager for incremental widget operations.
	a.stackedPaneMgr = component.NewStackedPaneManager(a.widgetFactory)
}

func (a *App) initAppToasterOverlay() {
	if a.mainWindow == nil {
		return
	}

	// Create app-level toaster for system notifications.
	a.appToaster = component.NewToaster(a.widgetFactory)
	toasterWidget := a.appToaster.Widget()
	if toasterWidget == nil {
		return
	}
	gtkWidget := toasterWidget.GtkWidget()
	if gtkWidget == nil {
		return
	}
	a.mainWindow.AddOverlay(gtkWidget)
}

func (a *App) initFocusAndBorderOverlay() {
	// Initialize focus manager for geometric navigation.
	a.focusMgr = focus.NewManager(a.panesUC)

	// Initialize border manager for mode indicators.
	a.borderMgr = focus.NewBorderManager(a.widgetFactory)

	// Attach border overlay to main window (visible for all tabs).
	if a.borderMgr == nil || a.mainWindow == nil {
		return
	}
	borderWidget := a.borderMgr.Widget()
	if borderWidget == nil {
		return
	}
	gtkWidget := borderWidget.GtkWidget()
	if gtkWidget == nil {
		return
	}
	a.mainWindow.AddOverlay(gtkWidget)
}

func (a *App) initKeyboardHandler(ctx context.Context) {
	if a.mainWindow == nil || a.deps == nil || a.deps.Config == nil {
		return
	}

	// Create keyboard handler and wire to dispatcher.
	a.keyboardHandler = input.NewKeyboardHandler(ctx, a.deps.Config)
	a.keyboardHandler.SetOnAction(func(ctx context.Context, action input.Action) error {
		return a.kbDispatcher.Dispatch(ctx, action)
	})
	a.keyboardHandler.SetOnModeChange(func(from, to input.Mode) {
		a.handleModeChange(ctx, from, to)
	})
	a.keyboardHandler.SetShouldBypassInput(func() bool {
		// Bypass keyboard handler when session manager is visible
		if a.sessionManager != nil && a.sessionManager.IsVisible() {
			return true
		}
		wsView := a.activeWorkspaceView()
		if wsView == nil {
			return false
		}
		return wsView.IsOmniboxVisible()
	})
	a.keyboardHandler.AttachTo(a.mainWindow.Window())

	// Create global shortcut handler for Alt+1-9 tab switching.
	// This uses GtkShortcutController with GTK_SHORTCUT_SCOPE_GLOBAL
	// to intercept shortcuts even when WebView has focus.
	a.globalShortcutHandler = input.NewGlobalShortcutHandler(
		ctx,
		a.mainWindow.Window(),
		func(ctx context.Context, action input.Action) error {
			return a.kbDispatcher.Dispatch(ctx, action)
		},
	)
}

func (a *App) initOmniboxConfig(ctx context.Context) {
	if a.deps == nil || a.deps.Config == nil {
		return
	}

	log := logging.FromContext(ctx)

	// Store omnibox config (omnibox is created per-pane via WorkspaceView).
	a.omniboxCfg = component.OmniboxConfig{
		HistoryUC:       a.deps.HistoryUC,
		FavoritesUC:     a.deps.FavoritesUC,
		FaviconAdapter:  a.faviconAdapter,
		CopyURLUC:       a.deps.CopyURLUC,
		Shortcuts:       a.deps.Config.SearchShortcuts,
		DefaultSearch:   a.deps.Config.DefaultSearchEngine,
		InitialBehavior: a.deps.Config.Omnibox.InitialBehavior,
		UIScale:         a.deps.Config.DefaultUIScale,
		OnNavigate: func(url string) {
			if err := a.navCoord.Navigate(ctx, url); err != nil {
				log.Error().Err(err).Str("url", url).Msg("navigation failed")
			}
		},
	}
	a.navCoord.SetOmniboxProvider(a)
	log.Debug().Msg("omnibox config stored, provider set")
}

func (a *App) initFindBarConfig() {
	// Store find bar config (find bar is created per-pane via WorkspaceView).
	a.findBarCfg = component.FindBarConfig{
		GetFindController: func(paneID entity.PaneID) port.FindController {
			if a.contentCoord == nil {
				return nil
			}
			wv := a.contentCoord.GetWebView(paneID)
			if wv == nil {
				return nil
			}
			return wv.GetFindController()
		},
	}
}

func (a *App) initSessionManager(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.deps == nil {
		log.Debug().Msg("deps not available, skipping session manager")
		return
	}

	// Session manager can work without a repo - it just shows an empty list
	var listSessionsUC *usecase.ListSessionsUseCase
	var deleteSessionUC *usecase.DeleteSessionUseCase
	if a.deps.SessionRepo != nil && a.deps.SessionStateRepo != nil {
		listSessionsUC = usecase.NewListSessionsUseCase(
			a.deps.SessionRepo,
			a.deps.SessionStateRepo,
			a.deps.Config.Logging.LogDir,
		)
		deleteSessionUC = usecase.NewDeleteSessionUseCase(
			a.deps.SessionStateRepo,
			a.deps.SessionRepo,
			a.deps.Config.Logging.LogDir,
		)
	}

	// Create session spawner for restoration
	spawner := desktop.NewSessionSpawner(ctx)

	// Create session manager component
	a.sessionManager = component.NewSessionManager(ctx, component.SessionManagerConfig{
		ListSessionsUC:  listSessionsUC,
		DeleteSessionUC: deleteSessionUC,
		CurrentSession:  a.deps.CurrentSessionID,
		UIScale:         a.deps.Config.DefaultUIScale,
		OnClose: func() {
			log.Debug().Msg("session manager closed")
		},
		OnOpen: func(sessionID entity.SessionID) {
			log.Info().Str("session_id", string(sessionID)).Msg("session restoration requested")
			if err := spawner.SpawnWithSession(sessionID); err != nil {
				log.Error().Err(err).Str("session_id", string(sessionID)).Msg("failed to spawn session")
			}
		},
		OnToast: func(ctx context.Context, message string, level component.ToastLevel) {
			if a.appToaster != nil {
				a.appToaster.Show(ctx, message, level)
			}
		},
	})

	if a.sessionManager == nil {
		log.Warn().Msg("failed to create session manager")
		return
	}

	// Add session manager to main window overlay
	if a.mainWindow != nil {
		widget := a.sessionManager.Widget()
		if widget != nil {
			a.mainWindow.AddOverlay(widget)
		}
	}

	// Wire session manager action to keyboard dispatcher
	a.kbDispatcher.SetOnSessionOpen(func(ctx context.Context) error {
		a.ToggleSessionManager(ctx)
		return nil
	})

	log.Debug().Msg("session manager initialized")
}

// ToggleSessionManager shows or hides the session manager.
func (a *App) ToggleSessionManager(ctx context.Context) {
	if a.sessionManager == nil {
		return
	}
	a.sessionManager.Toggle(ctx)
}

func (a *App) initSnapshotService(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.deps == nil || a.deps.SnapshotUC == nil {
		log.Debug().Msg("snapshot use case not available, skipping snapshot service")
		return
	}

	intervalMs := 5000 // default
	if a.deps.Config != nil && a.deps.Config.Session.SnapshotIntervalMs > 0 {
		intervalMs = a.deps.Config.Session.SnapshotIntervalMs
	}

	a.snapshotService = snapshot.NewService(a.deps.SnapshotUC, a, intervalMs)
	a.snapshotService.Start(ctx)

	log.Debug().Int("interval_ms", intervalMs).Msg("snapshot service started")
}

func (a *App) initUpdateCoordinator(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.deps == nil || a.deps.CheckUpdateUC == nil {
		log.Debug().Msg("update use cases not available, skipping update coordinator")
		return
	}

	a.updateCoord = coordinator.NewUpdateCoordinator(
		a.deps.CheckUpdateUC,
		a.deps.ApplyUpdateUC,
		a.appToaster,
		a.deps.Config,
	)

	// Start async update check
	a.updateCoord.CheckOnStartup(ctx)

	log.Debug().Msg("update coordinator initialized")
}

// GetTabList implements port.TabListProvider.
func (a *App) GetTabList() *entity.TabList {
	return a.tabs
}

// GetSessionID implements port.TabListProvider.
func (a *App) GetSessionID() entity.SessionID {
	if a.deps == nil {
		return ""
	}
	return a.deps.CurrentSessionID
}

// MarkDirty signals that session state has changed and should be saved.
func (a *App) MarkDirty() {
	if a.snapshotService != nil {
		a.snapshotService.MarkDirty()
	}
}

func (a *App) createInitialTab(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Check if we should restore a session
	if a.deps != nil && a.deps.RestoreSessionID != "" {
		if err := a.restoreSession(ctx, entity.SessionID(a.deps.RestoreSessionID)); err != nil {
			log.Error().Err(err).Str("session_id", a.deps.RestoreSessionID).Msg("failed to restore session, creating default tab")
			// Show error toast and fall through to create default tab
			if a.appToaster != nil {
				a.appToaster.Show(ctx, "Session restore failed", component.ToastWarning)
			}
		} else {
			log.Info().Str("session_id", a.deps.RestoreSessionID).Msg("session restored")
			// Show success toast for session restoration
			if a.appToaster != nil {
				a.appToaster.Show(ctx, "Session restored", component.ToastSuccess)
			}
			return
		}
	}

	// Create an initial tab using coordinator.
	initialURL := "dumb://home"
	if a.deps != nil && a.deps.InitialURL != "" {
		initialURL = a.deps.InitialURL
	}
	if _, err := a.tabCoord.Create(ctx, initialURL); err != nil {
		log.Error().Err(err).Msg("failed to create initial tab")
	}
}

func (a *App) restoreSession(ctx context.Context, sessionID entity.SessionID) error {
	log := logging.FromContext(ctx)

	if a.deps == nil || a.deps.SessionStateRepo == nil {
		return fmt.Errorf("session state repo not available")
	}

	// Load session state
	state, err := a.deps.SessionStateRepo.GetSnapshot(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session state: %w", err)
	}
	if state == nil {
		return fmt.Errorf("session state not found")
	}

	log.Info().Int("tabs", len(state.Tabs)).Int("panes", state.CountPanes()).Msg("restoring session state")

	// Build complete TabList from snapshot (state-first approach)
	// This reconstructs the full pane tree with new IDs
	restoredTabs := entity.TabListFromSnapshot(state, a.generateID)
	if restoredTabs == nil || len(restoredTabs.Tabs) == 0 {
		return fmt.Errorf("failed to build tab list from snapshot")
	}

	// Replace tabs in-place to preserve references held by TabCoordinator
	a.tabs.ReplaceFrom(restoredTabs)

	// Create UI for each restored tab and add to tab bar
	// Don't attach to content area yet - only the active tab should be attached
	tabBar := a.mainWindow.TabBar()
	for _, tab := range a.tabs.Tabs {
		a.createWorkspaceViewWithoutAttach(ctx, tab)
		if tabBar != nil {
			tabBar.AddTab(tab)
		}
		log.Debug().
			Str("tab_id", string(tab.ID)).
			Str("name", tab.Name).
			Int("panes", tab.Workspace.PaneCount()).
			Msg("restored tab with workspace")
	}

	// Switch to active tab - this attaches the active workspace view to content area
	activeTab := a.tabs.ActiveTab()
	if activeTab != nil {
		a.switchWorkspaceView(ctx, activeTab.ID)
		if tabBar != nil {
			tabBar.SetActive(activeTab.ID)
		}
	}

	return nil
}

func (a *App) finalizeActivation(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Show the window.
	if a.mainWindow != nil {
		a.mainWindow.Show()
	}
	log.Info().Msg("main window displayed")

	// Start watching config for appearance changes.
	a.initConfigWatcher(ctx)

	// Ensure the current appearance config is applied once at startup.
	// This avoids the WebUI showing default tokens until the first config change event.
	if a.deps != nil {
		a.applyAppearanceConfig(ctx, a.deps.Config)
	}

	// Start async filter loading after window is visible.
	a.initFilteringAsync(ctx)
}

// onShutdown is called when the GTK application is shutting down.
func (a *App) onShutdown(ctx context.Context) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("GTK application shutting down")

	// Save final session state before shutdown
	if a.snapshotService != nil {
		if err := a.snapshotService.Stop(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to save final session state")
		}
	}

	// Apply staged update if available (before cleanup)
	if a.updateCoord != nil {
		if err := a.updateCoord.FinalizeOnExit(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to apply staged update")
		}
	}

	// Cancel context to signal all goroutines
	a.cancel(errors.New("application shutdown"))

	// Cleanup resources
	if a.faviconAdapter != nil {
		a.faviconAdapter.Close()
	}
	if a.deps.Pool != nil {
		a.deps.Pool.Close(ctx)
	}
	// Close idle inhibitor to release D-Bus connection
	if a.deps.IdleInhibitor != nil {
		if err := a.deps.IdleInhibitor.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close idle inhibitor")
		}
	}

	log.Info().Msg("application shutdown complete")
}

// initCoordinators initializes all coordinators and wires their callbacks.
func (a *App) initCoordinators(ctx context.Context) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("initializing coordinators")

	// Helper to get active workspace and view
	getActiveWS := func() (*entity.Workspace, *component.WorkspaceView) {
		return a.activeWorkspace(), a.activeWorkspaceView()
	}

	// Create FaviconAdapter with service and WebKit FaviconDatabase
	var faviconDB *webkit.FaviconDatabase
	if a.deps.WebContext != nil {
		faviconDB = a.deps.WebContext.FaviconDatabase()
	}
	a.faviconAdapter = adapter.NewFaviconAdapter(a.deps.FaviconService, faviconDB)

	// 1. Content Coordinator (no dependencies on other coordinators)
	a.contentCoord = coordinator.NewContentCoordinator(
		ctx,
		a.pool,
		a.widgetFactory,
		a.faviconAdapter,
		getActiveWS,
		a.deps.ZoomUC,
	)

	// Set idle inhibitor for fullscreen video playback
	if a.deps.IdleInhibitor != nil {
		a.contentCoord.SetIdleInhibitor(a.deps.IdleInhibitor)
	}

	// 2. Tab Coordinator
	a.tabCoord = coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{
		TabsUC:     a.tabsUC,
		Tabs:       a.tabs,
		MainWindow: a.mainWindow,
		Config:     a.deps.Config,
	})
	a.tabCoord.SetOnTabCreated(func(ctx context.Context, tab *entity.Tab) {
		a.createWorkspaceView(ctx, tab)
	})
	a.tabCoord.SetOnTabSwitched(func(ctx context.Context, tab *entity.Tab) {
		a.switchWorkspaceView(ctx, tab.ID)
	})
	a.tabCoord.SetOnQuit(a.Quit)
	a.tabCoord.SetOnStateChanged(a.MarkDirty)
	// Wire popup tab WebView attachment
	a.tabCoord.SetOnAttachPopupToTab(func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv *webkit.WebView) {
		a.attachPopupToTab(ctx, tabID, pane, wv)
	})

	// Wire tab bar click handling to coordinator
	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().SetOnSwitch(func(tabID entity.TabID) {
			if err := a.tabCoord.Switch(ctx, tabID); err != nil {
				log.Error().Err(err).Str("tab_id", string(tabID)).Msg("tab switch failed")
			}
		})
	}

	// 3. Workspace Coordinator
	a.wsCoord = coordinator.NewWorkspaceCoordinator(ctx, coordinator.WorkspaceCoordinatorConfig{
		PanesUC:        a.panesUC,
		FocusMgr:       a.focusMgr,
		StackedPaneMgr: a.stackedPaneMgr,
		WidgetFactory:  a.widgetFactory,
		ContentCoord:   a.contentCoord,
		GetActiveWS:    getActiveWS,
		GenerateID:     a.generateID,
	})
	a.wsCoord.SetOnCloseLastPane(func(ctx context.Context) error {
		return a.tabCoord.Close(ctx)
	})
	a.wsCoord.SetOnStateChanged(a.MarkDirty)

	// Wire popup handling
	a.webViewFactory = webkit.NewWebViewFactory(
		a.deps.WebContext,
		a.settings,
		a.pool,
		a.injector,
		a.router,
	)
	// Set theme background color on factory to eliminate white flash
	if a.deps.Theme != nil {
		r, g, b, alpha := a.deps.Theme.GetBackgroundRGBA()
		a.webViewFactory.SetBackgroundColor(r, g, b, alpha)
	}
	a.contentCoord.SetPopupConfig(
		a.webViewFactory,
		&a.deps.Config.Workspace.Popups,
		a.generateID,
	)
	a.contentCoord.SetOnInsertPopup(func(ctx context.Context, input coordinator.InsertPopupInput) error {
		return a.wsCoord.InsertPopup(ctx, input)
	})
	a.contentCoord.SetOnClosePane(func(ctx context.Context, paneID entity.PaneID) error {
		return a.wsCoord.ClosePaneByID(ctx, paneID)
	})
	// Wire tabbed popup behavior to create new tabs
	a.wsCoord.SetOnCreatePopupTab(func(ctx context.Context, input coordinator.InsertPopupInput) error {
		// Create a new tab with the popup pane
		tab, err := a.tabCoord.CreateWithPane(ctx, input.PopupPane, input.WebView, input.TargetURI)
		if err != nil {
			return err
		}
		log.Debug().Str("tab_id", string(tab.ID)).Msg("created tab for popup")
		return nil
	})

	// 4. Navigation Coordinator
	a.navCoord = coordinator.NewNavigationCoordinator(
		ctx,
		a.deps.NavigateUC,
		a.contentCoord,
	)

	// Wire title updates to history persistence
	a.contentCoord.SetOnTitleUpdated(func(ctx context.Context, paneID entity.PaneID, url, title string) {
		a.navCoord.UpdateHistoryTitle(ctx, paneID, url, title)
	})

	// Wire history recording on LoadCommitted (URI is guaranteed correct at this point)
	a.contentCoord.SetOnHistoryRecord(func(ctx context.Context, paneID entity.PaneID, url string) {
		a.navCoord.RecordHistory(ctx, paneID, url)
	})

	// Wire window title updates when active pane's title changes
	a.contentCoord.SetOnWindowTitleChanged(func(title string) {
		a.updateWindowTitle(title)
	})

	// Wire pane URI updates for session snapshots (searches all tabs)
	a.contentCoord.SetOnPaneURIUpdated(func(paneID entity.PaneID, url string) {
		a.updatePaneURIInAllTabs(paneID, url)
		// Mark dirty so snapshot captures the new URI
		a.MarkDirty()
	})

	// 5. Keyboard Dispatcher
	a.kbDispatcher = dispatcher.NewKeyboardDispatcher(
		ctx,
		a.tabCoord,
		a.wsCoord,
		a.navCoord,
		a.deps.ZoomUC,
		a.deps.CopyURLUC,
	)
	a.kbDispatcher.SetOnQuit(a.Quit)
	a.kbDispatcher.SetOnFindOpen(func(ctx context.Context) error {
		a.ToggleFindBar(ctx)
		return nil
	})
	a.kbDispatcher.SetOnFindNext(func(ctx context.Context) error {
		a.FindNext(ctx)
		return nil
	})
	a.kbDispatcher.SetOnFindPrev(func(ctx context.Context) error {
		a.FindPrevious(ctx)
		return nil
	})
	a.kbDispatcher.SetOnFindClose(func(ctx context.Context) error {
		a.CloseFindBar(ctx)
		return nil
	})

	// Wire gesture handler to dispatcher (for mouse button 8/9 navigation)
	a.contentCoord.SetGestureActionHandler(func(ctx context.Context, action input.Action) error {
		return a.kbDispatcher.Dispatch(ctx, action)
	})

	log.Debug().Msg("coordinators initialized")
}

// generateID generates a unique ID for tabs and panes.
// Uses monotonically increasing counter to avoid ID collisions.
func (a *App) generateID() string {
	a.idMu.Lock()
	defer a.idMu.Unlock()
	a.idCounter++
	return fmt.Sprintf("p%d", a.idCounter)
}

// Tabs returns the tab list.
func (a *App) Tabs() *entity.TabList {
	return a.tabs
}

// MainWindow returns the main window.
func (a *App) MainWindow() *window.MainWindow {
	return a.mainWindow
}

// Quit requests the application to quit.
func (a *App) Quit() {
	if a.gtkApp != nil {
		a.gtkApp.Quit()
	}
}

// RunWithArgs is a convenience function that creates and runs an App.
func RunWithArgs(ctx context.Context, deps *Dependencies) int {
	app, err := New(deps)
	if err != nil {
		log := logging.FromContext(ctx)
		log.Error().Err(err).Msg("failed to create application")
		return 1
	}
	return app.Run(ctx, os.Args)
}

// updateWindowTitle updates the window title with the given page title.
// Format: "<Page Title> - Dumber" or just "Dumber" if title is empty.
func (a *App) updateWindowTitle(pageTitle string) {
	if a.mainWindow == nil {
		return
	}

	title := "Dumber"
	if pageTitle != "" {
		title = pageTitle + " - Dumber"
	}
	a.mainWindow.SetTitle(title)
}

// updateWindowTitleFromActivePane updates the window title based on the current active pane.
func (a *App) updateWindowTitleFromActivePane() {
	ws := a.activeWorkspace()
	if ws == nil || a.contentCoord == nil {
		a.updateWindowTitle("")
		return
	}
	title := a.contentCoord.GetTitle(ws.ActivePaneID)
	a.updateWindowTitle(title)
}

// handleModeChange is called when the input mode changes.
func (a *App) handleModeChange(ctx context.Context, from, to input.Mode) {
	log := logging.FromContext(ctx)
	log.Debug().Str("from", from.String()).Str("to", to.String()).Msg("input mode changed")

	if from == input.ModeResize && to != input.ModeResize {
		a.clearResizeModeBorder()
	}
	if to == input.ModeResize {
		a.applyResizeModeBorder(ctx, a.activeWorkspace())
	}

	// Update global border overlay visibility based on mode.
	// Note: resize mode border is handled per-pane (stack container), not via global overlay.
	if a.borderMgr != nil {
		a.borderMgr.OnModeChange(ctx, from, to)
	}
}

func (a *App) clearResizeModeBorder() {
	if a.resizeModeBorderTarget != nil {
		a.resizeModeBorderTarget.RemoveCssClass("resize-mode-active")
		a.resizeModeBorderTarget = nil
	}
}

func (a *App) applyResizeModeBorder(ctx context.Context, ws *entity.Workspace) {
	wsView := a.activeWorkspaceView()
	if wsView == nil || ws == nil {
		a.clearResizeModeBorder()
		return
	}

	paneID := ws.ActivePaneID
	if paneID == "" {
		a.clearResizeModeBorder()
		return
	}

	// Important: wrap the whole stack container when the active pane is in a stack.
	target := wsView.GetStackContainerWidget(paneID)
	if target == nil {
		a.clearResizeModeBorder()
		return
	}

	if !target.HasCssClass("resize-mode-active") {
		target.AddCssClass("resize-mode-active")
	}

	if a.resizeModeBorderTarget != target {
		if a.resizeModeBorderTarget != nil {
			a.resizeModeBorderTarget.RemoveCssClass("resize-mode-active")
		} else {
			logging.FromContext(ctx).Debug().Str("pane_id", string(paneID)).Msg("resize mode border attached")
		}
		a.resizeModeBorderTarget = target
	}
}

// createWorkspaceView creates a WorkspaceView for a tab and attaches it to the content area.
func (a *App) createWorkspaceView(ctx context.Context, tab *entity.Tab) {
	a.createWorkspaceViewWithoutAttach(ctx, tab)

	// Attach to content area
	wsView := a.workspaceViews[tab.ID]
	if wsView != nil && a.mainWindow != nil {
		widget := wsView.Widget()
		if widget != nil {
			gtkWidget := widget.GtkWidget()
			if gtkWidget != nil {
				a.mainWindow.SetContent(gtkWidget)
			}
		}
	}
}

// createWorkspaceViewWithoutAttach creates a WorkspaceView for a tab without attaching to content area.
// Used during session restoration where we create all views first, then attach only the active one.
func (a *App) createWorkspaceViewWithoutAttach(ctx context.Context, tab *entity.Tab) {
	log := logging.FromContext(ctx)

	if a.widgetFactory == nil {
		log.Error().Msg("widget factory not initialized")
		return
	}

	// Create workspace view
	wsView := component.NewWorkspaceView(ctx, a.widgetFactory)
	if wsView == nil {
		log.Error().Msg("failed to create workspace view")
		return
	}

	// Set the workspace
	if err := wsView.SetWorkspace(ctx, tab.Workspace); err != nil {
		log.Error().Err(err).Msg("failed to set workspace in view")
		return
	}

	// Ensure WebViews are attached to panes
	if a.contentCoord != nil {
		a.contentCoord.AttachToWorkspace(ctx, tab.Workspace, wsView)
	}

	// Note: Mode borders for tab/pane/session are attached to MainWindow.
	// Resize mode border is attached to the active pane's stack container.

	// Set omnibox config for this workspace view
	wsView.SetOmniboxConfig(a.omniboxCfg)
	// Set find bar config for this workspace view
	wsView.SetFindBarConfig(a.findBarCfg)

	wsView.SetOnPaneFocused(func(paneID entity.PaneID) {
		if a.keyboardHandler != nil && a.keyboardHandler.Mode() == input.ModeResize {
			ws := a.activeWorkspace()
			if ws != nil {
				ws.ActivePaneID = paneID
			}
			a.applyResizeModeBorder(ctx, ws)
		}
	})

	wsView.SetOnSplitRatioDragged(func(nodeID string, ratio float64) {
		if a.wsCoord != nil {
			_ = a.wsCoord.SetSplitRatio(ctx, nodeID, ratio)
		}
	})

	// Store in map
	a.workspaceViews[tab.ID] = wsView

	log.Debug().Str("tab_id", string(tab.ID)).Msg("workspace view created")
}

// activeWorkspace returns the workspace of the active tab.
func (a *App) activeWorkspace() *entity.Workspace {
	activeTab := a.tabs.ActiveTab()
	if activeTab == nil {
		return nil
	}
	return activeTab.Workspace
}

// updatePaneURIInAllTabs finds a pane by ID across all tabs and updates its URI.
// This is necessary because panes in inactive tabs also need URI updates for session snapshots.
func (a *App) updatePaneURIInAllTabs(paneID entity.PaneID, url string) {
	for _, tab := range a.tabs.Tabs {
		if tab.Workspace == nil {
			continue
		}
		paneNode := tab.Workspace.FindPane(paneID)
		if paneNode != nil && paneNode.Pane != nil {
			paneNode.Pane.URI = url
			return // Pane IDs are unique, no need to continue
		}
	}
}

// activeWorkspaceView returns the workspace view for the active tab.
func (a *App) activeWorkspaceView() *component.WorkspaceView {
	activeTab := a.tabs.ActiveTab()
	if activeTab == nil {
		return nil
	}
	return a.workspaceViews[activeTab.ID]
}

// attachPopupToTab attaches a popup WebView to a newly created tab.
// This is called when a popup uses tabbed behavior.
func (a *App) attachPopupToTab(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv *webkit.WebView) {
	log := logging.FromContext(ctx)

	wsView := a.workspaceViews[tabID]
	if wsView == nil {
		log.Warn().Str("tab_id", string(tabID)).Msg("workspace view not found for popup tab")
		return
	}

	// Register WebView with content coordinator
	if a.contentCoord != nil {
		// Track the WebView
		a.contentCoord.RegisterPopupWebView(pane.ID, wv)

		// Wrap and attach widget
		widget := a.contentCoord.WrapWidget(ctx, wv)
		if widget != nil {
			paneView := wsView.GetPaneView(pane.ID)
			if paneView != nil {
				paneView.SetWebViewWidget(widget)
			} else {
				log.Warn().Str("pane_id", string(pane.ID)).Msg("pane view not found for popup")
			}
		}

		// Setup popup handling for nested popups
		a.contentCoord.SetupPopupHandling(ctx, pane.ID, wv)
	}

	log.Debug().
		Str("tab_id", string(tabID)).
		Str("pane_id", string(pane.ID)).
		Msg("popup webview attached to tab")
}

// switchWorkspaceView swaps the displayed workspace view for a tab.
func (a *App) switchWorkspaceView(ctx context.Context, tabID entity.TabID) {
	log := logging.FromContext(ctx)

	wsView, exists := a.workspaceViews[tabID]
	if !exists {
		log.Warn().Str("tab_id", string(tabID)).Msg("no workspace view for tab")
		return
	}

	// Get the workspace view's root widget
	widget := wsView.Widget()
	if widget == nil {
		log.Warn().Str("tab_id", string(tabID)).Msg("workspace view has no widget")
		return
	}

	gtkWidget := widget.GtkWidget()
	if gtkWidget == nil {
		log.Warn().Str("tab_id", string(tabID)).Msg("workspace view widget has no GTK widget")
		return
	}

	// Swap content (MainWindow.SetContent now properly removes old content)
	if a.mainWindow != nil {
		a.mainWindow.SetContent(gtkWidget)
	}

	// Update window title with the new active pane's title
	a.updateWindowTitleFromActivePane()

	log.Debug().Str("tab_id", string(tabID)).Msg("workspace view switched")
}

// ToggleOmnibox implements OmniboxProvider.
// Toggles the omnibox visibility in the active workspace view.
func (a *App) ToggleOmnibox(ctx context.Context) {
	log := logging.FromContext(ctx)

	wsView := a.activeWorkspaceView()
	if wsView == nil {
		log.Warn().Msg("no active workspace view for omnibox toggle")
		return
	}

	if wsView.IsOmniboxVisible() {
		wsView.HideOmnibox()
	} else {
		wsView.ShowOmnibox(ctx, "")
	}
}

// ToggleFindBar shows or hides the find bar in the active workspace view.
func (a *App) ToggleFindBar(ctx context.Context) {
	log := logging.FromContext(ctx)

	wsView := a.activeWorkspaceView()
	if wsView == nil {
		log.Warn().Msg("no active workspace view for find bar toggle")
		return
	}

	if wsView.IsFindBarVisible() {
		wsView.HideFindBar()
	} else {
		wsView.ShowFindBar(ctx)
	}
}

// FindNext selects the next match in the active find bar.
func (a *App) FindNext(ctx context.Context) {
	wsView := a.activeWorkspaceView()
	if wsView == nil {
		return
	}

	if !wsView.IsFindBarVisible() {
		wsView.ShowFindBar(ctx)
	}
	wsView.FindNext()
}

// FindPrevious selects the previous match in the active find bar.
func (a *App) FindPrevious(ctx context.Context) {
	wsView := a.activeWorkspaceView()
	if wsView == nil {
		return
	}

	if !wsView.IsFindBarVisible() {
		wsView.ShowFindBar(ctx)
	}
	wsView.FindPrevious()
}

// CloseFindBar hides the find bar if visible.
func (a *App) CloseFindBar(ctx context.Context) {
	wsView := a.activeWorkspaceView()
	if wsView == nil {
		return
	}
	wsView.HideFindBar()
}

// UpdateOmniboxZoom implements OmniboxProvider.
// Updates the zoom indicator on the current omnibox if visible.
func (a *App) UpdateOmniboxZoom(factor float64) {
	wsView := a.activeWorkspaceView()
	if wsView == nil {
		return
	}

	omnibox := wsView.GetOmnibox()
	if omnibox != nil {
		omnibox.UpdateZoomIndicator(factor)
	}
}

// SetOmniboxOnNavigate implements OmniboxProvider.
// This is called to set the navigation callback on new omniboxes.
// Since omniboxes are created per-pane, we store the config with the callback.
func (a *App) SetOmniboxOnNavigate(fn func(url string)) {
	// The navigate callback is set when the omnibox is created via WorkspaceView.ShowOmnibox
	// The WorkspaceView uses the stored omniboxCfg and sets up navigation via the omnibox's SetOnNavigate
}

// initFilteringAsync starts background filter loading with toast feedback.
func (a *App) initConfigWatcher(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.configManager == nil {
		log.Debug().Msg("no config manager available, skipping watcher")
		return
	}

	// Start viper watcher
	if err := a.configManager.Watch(); err != nil {
		log.Warn().Err(err).Msg("failed to start config watcher")
		return
	}

	// Only appearance is hot-reloaded for now.
	a.configManager.OnConfigChange(func(newCfg *config.Config) {
		cfgCopy := newCfg
		cb := glib.SourceFunc(func(_ uintptr) bool {
			a.applyAppearanceConfig(ctx, cfgCopy)
			return false
		})
		glib.IdleAdd(&cb, 0)
	})

	log.Debug().Msg("config watcher initialized")
}

func (a *App) applyAppearanceConfig(ctx context.Context, cfg *config.Config) {
	log := logging.FromContext(ctx)
	if cfg == nil {
		return
	}

	// Update the shared config pointer in-place so existing references see changes.
	if a.deps != nil && a.deps.Config != nil {
		*a.deps.Config = *cfg
	}

	// Update WebKit settings manager
	if a.settings != nil {
		a.settings.UpdateFromConfig(ctx, cfg)
	}

	// Apply settings to existing webviews via coordinator
	if a.contentCoord != nil {
		a.contentCoord.ApplySettingsToAll(ctx, a.settings)
	}

	// Update GTK theme and injected WebUI theme vars
	if a.deps != nil && a.deps.Theme != nil {
		var display *gdk.Display
		if a.mainWindow != nil && a.mainWindow.Window() != nil {
			display = a.mainWindow.Window().GetDisplay()
		}
		a.deps.Theme.UpdateFromConfig(ctx, cfg, display)

		// Keep injector's dark-mode flag in sync for future navigations
		if a.injector != nil {
			a.injector.SetPrefersDark(a.deps.Theme.PrefersDark())
			findCSS := theme.GenerateFindHighlightCSS(a.deps.Theme.GetCurrentPalette())
			if err := a.injector.InjectFindHighlightCSS(ctx, findCSS); err != nil {
				log.Warn().Err(err).Msg("failed to update find highlight CSS")
			}
		}

		prepareThemeUC := usecase.NewPrepareWebUIThemeUseCase(a.injector)
		cssText := a.deps.Theme.GetWebUIThemeCSS()
		if err := prepareThemeUC.Execute(ctx, usecase.PrepareWebUIThemeInput{CSSVars: cssText}); err != nil {
			log.Warn().Err(err).Msg("failed to update WebUI theme CSS")
		}

		// Refresh injected scripts so future navigations use the latest theme.
		if a.contentCoord != nil && a.injector != nil {
			a.contentCoord.RefreshInjectedScriptsToAll(ctx, a.injector)
		}

		// Apply WebUI theme to already-loaded dumb:// pages
		if a.contentCoord != nil {
			a.contentCoord.ApplyWebUIThemeToAll(ctx, a.deps.Theme.PrefersDark(), cssText)
		}
	}

	log.Info().Msg("appearance config updated")
}

func (a *App) initFilteringAsync(ctx context.Context) {
	if a.deps.FilterManager == nil {
		return
	}

	a.deps.FilterManager.SetStatusCallback(func(status filtering.FilterStatus) {
		statusCopy := status // Capture for closure
		cb := glib.SourceFunc(func(_ uintptr) bool {
			a.showFilterStatus(ctx, statusCopy)
			return false // Don't repeat
		})
		glib.IdleAdd(&cb, 0)
	})
	a.deps.FilterManager.LoadAsync(ctx)
}

// showFilterStatus displays toast notification for filter status.
func (a *App) showFilterStatus(ctx context.Context, status filtering.FilterStatus) {
	log := logging.FromContext(ctx)

	switch status.State {
	case filtering.StateLoading:
		if a.appToaster != nil {
			a.appToaster.Show(ctx, status.Message, component.ToastInfo)
		}
	case filtering.StateActive:
		// Apply filters to existing webviews that were created before filters loaded
		if a.contentCoord != nil && a.deps.FilterManager != nil {
			a.contentCoord.ApplyFiltersToAll(ctx, a.deps.FilterManager)
			log.Debug().Msg("applied filters to all existing webviews")
		}
		if a.appToaster != nil {
			a.appToaster.Show(ctx, fmt.Sprintf("Ad blocker ready (%s)", status.Version), component.ToastInfo)
		}
	case filtering.StateError:
		if a.appToaster != nil {
			a.appToaster.Show(ctx, "Filter load failed: "+status.Message, component.ToastError)
		}
	}
}

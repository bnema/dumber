package ui

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/cache"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/dispatcher"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/window"
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
	contentCoord *coordinator.ContentCoordinator
	tabCoord     *coordinator.TabCoordinator
	wsCoord      *coordinator.WorkspaceCoordinator
	navCoord     *coordinator.NavigationCoordinator
	kbDispatcher *dispatcher.KeyboardDispatcher

	// Pane management (used by coordinators)
	panesUC        *usecase.ManagePanesUseCase
	workspaceViews map[entity.TabID]*component.WorkspaceView
	widgetFactory  layout.WidgetFactory
	stackedPaneMgr *component.StackedPaneManager

	// Input handling
	keyboardHandler *input.KeyboardHandler

	// Focus and border management
	focusMgr  *focus.Manager
	borderMgr *focus.BorderManager

	// Native omnibox
	omnibox *component.Omnibox

	// Web content (managed by ContentCoordinator)
	pool         *webkit.WebViewPool
	injector     *webkit.ContentInjector
	router       *webkit.MessageRouter
	settings     *webkit.SettingsManager
	faviconCache *cache.FaviconCache

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
		cancel:         cancel,
	}
	if app.router == nil {
		app.router = webkit.NewMessageRouter(ctx)
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

	// Prewarm WebView pool now that GTK is initialized
	if a.pool != nil {
		a.pool.Prewarm(0)
	}

	// Create the main window
	var err error
	a.mainWindow, err = window.New(ctx, a.gtkApp, a.deps.Config)
	if err != nil {
		log.Error().Err(err).Msg("failed to create main window")
		return
	}

	// Apply GTK CSS styling from theme manager
	if a.deps.Theme != nil {
		if display := a.mainWindow.Window().GetDisplay(); display != nil {
			a.deps.Theme.ApplyToDisplay(ctx, display)
		}
	}

	// Initialize widget factory for pane layout
	a.widgetFactory = layout.NewGtkWidgetFactory()

	// Initialize stacked pane manager for incremental widget operations
	a.stackedPaneMgr = component.NewStackedPaneManager(a.widgetFactory)

	// Initialize focus manager for geometric navigation
	a.focusMgr = focus.NewManager(a.panesUC)

	// Initialize border manager for mode indicators
	a.borderMgr = focus.NewBorderManager(a.widgetFactory)

	// Initialize coordinators
	a.initCoordinators(ctx)

	// Create keyboard handler and wire to dispatcher
	a.keyboardHandler = input.NewKeyboardHandler(
		ctx,
		&a.deps.Config.Workspace,
	)
	a.keyboardHandler.SetOnAction(func(ctx context.Context, action input.Action) error {
		return a.kbDispatcher.Dispatch(ctx, action)
	})
	a.keyboardHandler.SetOnModeChange(func(from, to input.Mode) {
		a.handleModeChange(ctx, from, to)
	})
	a.keyboardHandler.AttachTo(a.mainWindow.Window())

	// Create native omnibox
	a.omnibox = component.NewOmnibox(ctx, a.mainWindow.Window(), component.OmniboxConfig{
		HistoryUC:       a.deps.HistoryUC,
		FavoritesUC:     a.deps.FavoritesUC,
		FaviconCache:    a.faviconCache,
		Shortcuts:       a.deps.Config.SearchShortcuts,
		DefaultSearch:   a.deps.Config.DefaultSearchEngine,
		InitialBehavior: a.deps.Config.Omnibox.InitialBehavior,
		UIScale:         a.deps.Config.DefaultUIScale,
	})
	if a.omnibox != nil {
		a.omnibox.SetOnNavigate(func(url string) {
			a.navCoord.Navigate(ctx, url)
		})
		a.navCoord.SetOmnibox(a.omnibox)
		// Add omnibox widget to the main window overlay
		a.mainWindow.AddOverlay(a.omnibox.Widget())
		log.Debug().Msg("native omnibox created and added to overlay")
	} else {
		log.Warn().Msg("failed to create native omnibox")
	}

	// Create an initial tab using coordinator
	if _, err := a.tabCoord.Create(ctx, "https://duckduckgo.com"); err != nil {
		log.Error().Err(err).Msg("failed to create initial tab")
	}

	// Show the window
	a.mainWindow.Show()

	log.Info().Msg("main window displayed")
}

// onShutdown is called when the GTK application is shutting down.
func (a *App) onShutdown(ctx context.Context) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("GTK application shutting down")

	// Cancel context to signal all goroutines
	a.cancel(errors.New("application shutdown"))

	// Cleanup resources
	if a.deps.Pool != nil {
		a.deps.Pool.Close()
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

	// Create FaviconCache with FaviconDatabase from WebKitContext
	if a.deps.WebContext != nil {
		a.faviconCache = cache.NewFaviconCache(a.deps.WebContext.FaviconDatabase())
	} else {
		a.faviconCache = cache.NewFaviconCache(nil)
	}

	// 1. Content Coordinator (no dependencies on other coordinators)
	a.contentCoord = coordinator.NewContentCoordinator(
		ctx,
		a.pool,
		a.widgetFactory,
		a.faviconCache,
		getActiveWS,
	)

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
	a.tabCoord.SetOnQuit(a.Quit)

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

	// 5. Keyboard Dispatcher
	a.kbDispatcher = dispatcher.NewKeyboardDispatcher(
		ctx,
		a.tabCoord,
		a.wsCoord,
		a.navCoord,
		a.deps.ZoomUC,
	)
	a.kbDispatcher.SetOnQuit(a.Quit)

	log.Debug().Msg("coordinators initialized")
}

// generateID generates a unique ID for tabs and panes.
func (a *App) generateID() string {
	a.idMu.Lock()
	defer a.idMu.Unlock()
	a.idCounter++
	return string(rune('a'+a.idCounter%26)) + string(rune('0'+a.idCounter/26))
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

// handleModeChange is called when the input mode changes.
func (a *App) handleModeChange(ctx context.Context, from, to input.Mode) {
	log := logging.FromContext(ctx)
	log.Debug().Str("from", from.String()).Str("to", to.String()).Msg("input mode changed")

	// Update border overlay visibility based on mode
	if a.borderMgr != nil {
		a.borderMgr.OnModeChange(ctx, from, to)
	}
}

// createWorkspaceView creates a WorkspaceView for a tab and attaches it to the content area.
func (a *App) createWorkspaceView(ctx context.Context, tab *entity.Tab) {
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
	if err := wsView.SetWorkspace(tab.Workspace); err != nil {
		log.Error().Err(err).Msg("failed to set workspace in view")
		return
	}

	// Ensure WebViews are attached to panes
	if a.contentCoord != nil {
		a.contentCoord.AttachToWorkspace(ctx, tab.Workspace, wsView)
	}

	// Attach border manager overlay for mode indicators
	if a.borderMgr != nil {
		wsView.SetModeBorderOverlay(a.borderMgr.Widget())
	}

	// Store in map
	a.workspaceViews[tab.ID] = wsView

	// Attach to content area
	if a.mainWindow != nil {
		widget := wsView.Widget()
		if widget != nil {
			gtkWidget := widget.GtkWidget()
			if gtkWidget != nil {
				a.mainWindow.SetContent(gtkWidget)
			}
		}
	}

	log.Debug().Str("tab_id", string(tab.ID)).Msg("workspace view created and attached")
}

// activeWorkspace returns the workspace of the active tab.
func (a *App) activeWorkspace() *entity.Workspace {
	activeTab := a.tabs.ActiveTab()
	if activeTab == nil {
		return nil
	}
	return activeTab.Workspace
}

// activeWorkspaceView returns the workspace view for the active tab.
func (a *App) activeWorkspaceView() *component.WorkspaceView {
	activeTab := a.tabs.ActiveTab()
	if activeTab == nil {
		return nil
	}
	return a.workspaceViews[activeTab.ID]
}

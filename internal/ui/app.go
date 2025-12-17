package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/infrastructure/webkit/handlers"
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

	// Omnibox configuration (omnibox is created per workspace view)
	omniboxCfg component.OmniboxConfig

	// Web content (managed by ContentCoordinator)
	pool           *webkit.WebViewPool
	webViewFactory *webkit.WebViewFactory
	injector       *webkit.ContentInjector
	router         *webkit.MessageRouter
	settings       *webkit.SettingsManager
	faviconCache   *cache.FaviconCache

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

	// Register message handlers
	if app.router != nil && deps.HistoryUC != nil && deps.FavoritesUC != nil {
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

	// Prewarm WebView pool now that GTK is initialized
	if a.pool != nil {
		a.pool.Prewarm(ctx, 0)
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

	// Attach border overlay to main window (visible for all tabs)
	if a.borderMgr != nil && a.mainWindow != nil {
		if borderWidget := a.borderMgr.Widget(); borderWidget != nil {
			if gtkWidget := borderWidget.GtkWidget(); gtkWidget != nil {
				a.mainWindow.AddOverlay(gtkWidget)
			}
		}
	}

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

	// Store omnibox config (omnibox is created per-pane via WorkspaceView)
	a.omniboxCfg = component.OmniboxConfig{
		HistoryUC:       a.deps.HistoryUC,
		FavoritesUC:     a.deps.FavoritesUC,
		FaviconCache:    a.faviconCache,
		Clipboard:       a.deps.Clipboard,
		Shortcuts:       a.deps.Config.SearchShortcuts,
		DefaultSearch:   a.deps.Config.DefaultSearchEngine,
		InitialBehavior: a.deps.Config.Omnibox.InitialBehavior,
		UIScale:         a.deps.Config.DefaultUIScale,
		OnNavigate: func(url string) {
			a.navCoord.Navigate(ctx, url)
		},
	}
	a.navCoord.SetOmniboxProvider(a)
	log.Debug().Msg("omnibox config stored, provider set")

	// Create an initial tab using coordinator
	initialURL := "dumb://home"
	if a.deps.InitialURL != "" {
		initialURL = a.deps.InitialURL
	}
	if _, err := a.tabCoord.Create(ctx, initialURL); err != nil {
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
	if a.faviconCache != nil {
		a.faviconCache.Close()
	}
	if a.deps.Pool != nil {
		a.deps.Pool.Close(ctx)
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
		a.deps.ZoomUC,
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
	a.tabCoord.SetOnTabSwitched(func(ctx context.Context, tab *entity.Tab) {
		a.switchWorkspaceView(ctx, tab.ID)
	})
	a.tabCoord.SetOnQuit(a.Quit)
	// Wire popup tab WebView attachment
	a.tabCoord.SetOnAttachPopupToTab(func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv *webkit.WebView) {
		a.attachPopupToTab(ctx, tabID, pane, wv)
	})

	// Wire tab bar click handling to coordinator
	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().SetOnSwitch(func(tabID entity.TabID) {
			a.tabCoord.Switch(ctx, tabID)
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

	// Wire popup handling
	a.webViewFactory = webkit.NewWebViewFactory(
		a.deps.WebContext,
		a.settings,
		a.pool,
		a.injector,
		a.router,
	)
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

	// 5. Keyboard Dispatcher
	a.kbDispatcher = dispatcher.NewKeyboardDispatcher(
		ctx,
		a.tabCoord,
		a.wsCoord,
		a.navCoord,
		a.deps.ZoomUC,
	)
	a.kbDispatcher.SetOnQuit(a.Quit)

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
	if err := wsView.SetWorkspace(ctx, tab.Workspace); err != nil {
		log.Error().Err(err).Msg("failed to set workspace in view")
		return
	}

	// Ensure WebViews are attached to panes
	if a.contentCoord != nil {
		a.contentCoord.AttachToWorkspace(ctx, tab.Workspace, wsView)
	}

	// Note: Border overlay is attached to MainWindow, not per-workspace
	// This ensures it's visible regardless of which tab is active

	// Set omnibox config for this workspace view
	wsView.SetOmniboxConfig(a.omniboxCfg)

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

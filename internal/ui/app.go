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
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	uihandler "github.com/bnema/dumber/internal/ui/handler"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/theme"
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

	// Pane management
	panesUC        *usecase.ManagePanesUseCase
	workspaceViews map[entity.TabID]*component.WorkspaceView
	widgetFactory  layout.WidgetFactory

	// Input handling
	keyboardHandler *input.KeyboardHandler

	// Web content
	pool     *webkit.WebViewPool
	injector *webkit.ContentInjector
	router   *webkit.MessageRouter
	overlay  *component.OverlayController
	settings *webkit.SettingsManager
	webViews map[entity.PaneID]*webkit.WebView

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
		overlay:        deps.Overlay,
		settings:       deps.Settings,
		webViews:       make(map[entity.PaneID]*webkit.WebView),
		cancel:         cancel,
	}
	if app.overlay == nil {
		app.overlay = component.NewOverlayController(ctx)
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

	a.gtkApp = gtk.NewApplication(AppID, gio.GApplicationFlagsNoneValue)
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
	a.mainWindow, err = window.New(a.gtkApp, a.deps.Config, a.deps.Logger)
	if err != nil {
		log.Error().Err(err).Msg("failed to create main window")
		return
	}

	// Apply GTK CSS styling for tab bar
	if display := a.mainWindow.Window().GetDisplay(); display != nil {
		theme.ApplyCSS(display)
	}

	// Initialize widget factory for pane layout
	a.widgetFactory = layout.NewGtkWidgetFactory()

	// Create keyboard handler
	a.keyboardHandler = input.NewKeyboardHandler(
		ctx,
		&a.deps.Config.Workspace,
		a.deps.Logger,
	)
	a.keyboardHandler.SetOnAction(a.handleKeyboardAction)
	// Wrap handleModeChange to match the callback signature
	a.keyboardHandler.SetOnModeChange(func(from, to input.Mode) {
		a.handleModeChange(ctx, from, to)
	})
	a.keyboardHandler.AttachTo(a.mainWindow.Window())

	if err := a.registerMessageHandlers(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to register message handlers")
	}

	// Create an initial tab
	a.createInitialTab(ctx)

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

// createInitialTab creates the first tab when the application starts.
func (a *App) createInitialTab(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.tabsUC == nil {
		// Create use case if not injected
		a.tabsUC = usecase.NewManageTabsUseCase(a.generateID)
	}

	output, err := a.tabsUC.Create(ctx, usecase.CreateTabInput{
		TabList:    a.tabs,
		Name:       "",
		InitialURL: "https://duckduckgo.com",
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create initial tab")
		return
	}

	// Update tab bar
	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().AddTab(output.Tab)
		a.mainWindow.TabBar().SetActive(output.Tab.ID)
	}

	// Update tab bar visibility (hide if only 1 tab)
	a.updateTabBarVisibility(ctx)

	// Create workspace view for this tab
	a.createWorkspaceView(ctx, output.Tab)

	log.Debug().Str("tab_id", string(output.Tab.ID)).Msg("initial tab created")
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
		if deps.Logger != nil {
			deps.Logger.Error().Err(err).Msg("failed to create application")
		}
		return 1
	}
	return app.Run(ctx, os.Args)
}

// handleKeyboardAction dispatches keyboard actions to appropriate handlers.
func (a *App) handleKeyboardAction(ctx context.Context, action input.Action) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("action", string(action)).Msg("handling keyboard action")

	switch action {
	// Tab actions
	case input.ActionNewTab:
		return a.handleNewTab(ctx)
	case input.ActionCloseTab:
		return a.handleCloseTab(ctx)
	case input.ActionNextTab:
		return a.handleNextTab(ctx)
	case input.ActionPreviousTab:
		return a.handlePreviousTab(ctx)

	// Pane actions
	case input.ActionSplitRight:
		return a.handlePaneSplit(ctx, usecase.SplitRight)
	case input.ActionSplitLeft:
		return a.handlePaneSplit(ctx, usecase.SplitLeft)
	case input.ActionSplitUp:
		return a.handlePaneSplit(ctx, usecase.SplitUp)
	case input.ActionSplitDown:
		return a.handlePaneSplit(ctx, usecase.SplitDown)
	case input.ActionClosePane:
		return a.handleClosePane(ctx)
	case input.ActionFocusRight:
		return a.handlePaneFocus(ctx, usecase.NavRight)
	case input.ActionFocusLeft:
		return a.handlePaneFocus(ctx, usecase.NavLeft)
	case input.ActionFocusUp:
		return a.handlePaneFocus(ctx, usecase.NavUp)
	case input.ActionFocusDown:
		return a.handlePaneFocus(ctx, usecase.NavDown)

	// Navigation (stub implementations for now)
	case input.ActionGoBack:
		log.Debug().Msg("go back action (not yet implemented)")
	case input.ActionGoForward:
		log.Debug().Msg("go forward action (not yet implemented)")
	case input.ActionReload:
		log.Debug().Msg("reload action (not yet implemented)")
	case input.ActionHardReload:
		log.Debug().Msg("hard reload action (not yet implemented)")

	// Zoom (stub implementations for now)
	case input.ActionZoomIn:
		log.Debug().Msg("zoom in action (not yet implemented)")
	case input.ActionZoomOut:
		log.Debug().Msg("zoom out action (not yet implemented)")
	case input.ActionZoomReset:
		log.Debug().Msg("zoom reset action (not yet implemented)")

	// UI
	case input.ActionOpenOmnibox:
		return a.openOmnibox(ctx)
	case input.ActionOpenDevTools:
		return a.openDevTools(ctx)
	case input.ActionToggleFullscreen:
		log.Debug().Msg("toggle fullscreen action (not yet implemented)")

	// Application
	case input.ActionQuit:
		a.Quit()

	default:
		log.Warn().Str("action", string(action)).Msg("unhandled keyboard action")
	}

	return nil
}

// handleModeChange is called when the input mode changes.
func (a *App) handleModeChange(ctx context.Context, from, to input.Mode) {
	log := logging.FromContext(ctx)
	log.Debug().Str("from", from.String()).Str("to", to.String()).Msg("input mode changed")
}

// handleNewTab creates a new tab.
func (a *App) handleNewTab(ctx context.Context) error {
	log := logging.FromContext(ctx)

	output, err := a.tabsUC.Create(ctx, usecase.CreateTabInput{
		TabList:    a.tabs,
		Name:       "",
		InitialURL: "about:blank",
	})
	if err != nil {
		return err
	}

	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().AddTab(output.Tab)
		a.mainWindow.TabBar().SetActive(output.Tab.ID)
	}

	// Update tab bar visibility
	a.updateTabBarVisibility(ctx)

	log.Debug().Str("tab_id", string(output.Tab.ID)).Msg("new tab created")

	return nil
}

// handleCloseTab closes the active tab.
func (a *App) handleCloseTab(ctx context.Context) error {
	activeID := a.tabs.ActiveTabID
	if activeID == "" {
		return nil
	}

	wasLast, err := a.tabsUC.Close(ctx, a.tabs, activeID)
	if err != nil {
		return err
	}

	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().RemoveTab(activeID)

		// Switch to new active tab if any
		if a.tabs.ActiveTabID != "" {
			a.mainWindow.TabBar().SetActive(a.tabs.ActiveTabID)
		}
	}

	// Update tab bar visibility
	a.updateTabBarVisibility(ctx)

	// Quit if no tabs left
	if wasLast {
		a.Quit()
	}

	return nil
}

// updateTabBarVisibility shows or hides the tab bar based on tab count.
// Tab bar is hidden when there's only 1 tab (configurable).
func (a *App) updateTabBarVisibility(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Check if feature is enabled
	if a.deps.Config != nil && !a.deps.Config.Workspace.HideTabBarWhenSingleTab {
		return
	}

	if a.mainWindow == nil || a.mainWindow.TabBar() == nil {
		return
	}

	tabCount := a.tabs.Count()
	shouldShow := tabCount > 1

	a.mainWindow.TabBar().SetVisible(shouldShow)

	if shouldShow {
		log.Debug().Int("tab_count", tabCount).Msg("tab bar visible")
	} else {
		log.Debug().Msg("tab bar hidden (single tab)")
	}
}

// handleNextTab switches to the next tab.
func (a *App) handleNextTab(ctx context.Context) error {
	if err := a.tabsUC.SwitchNext(ctx, a.tabs); err != nil {
		return err
	}

	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().SetActive(a.tabs.ActiveTabID)
	}

	return nil
}

// handlePreviousTab switches to the previous tab.
func (a *App) handlePreviousTab(ctx context.Context) error {
	if err := a.tabsUC.SwitchPrevious(ctx, a.tabs); err != nil {
		return err
	}

	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().SetActive(a.tabs.ActiveTabID)
	}

	return nil
}

// createWorkspaceView creates a WorkspaceView for a tab and attaches it to the content area.
func (a *App) createWorkspaceView(ctx context.Context, tab *entity.Tab) {
	log := logging.FromContext(ctx)

	if a.widgetFactory == nil {
		log.Error().Msg("widget factory not initialized")
		return
	}

	// Create workspace view
	wsView := component.NewWorkspaceView(a.widgetFactory, *a.deps.Logger)
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
	a.attachWorkspaceWebViews(ctx, tab.Workspace, wsView)

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

// handlePaneSplit splits the active pane in the given direction.
func (a *App) handlePaneSplit(ctx context.Context, direction usecase.SplitDirection) error {
	log := logging.FromContext(ctx)

	if a.panesUC == nil {
		log.Warn().Msg("panes use case not available")
		return nil
	}

	ws := a.activeWorkspace()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	activePane := ws.ActivePane()
	if activePane == nil {
		log.Warn().Msg("no active pane to split")
		return nil
	}

	output, err := a.panesUC.Split(ctx, usecase.SplitPaneInput{
		Workspace:  ws,
		TargetPane: activePane,
		Direction:  direction,
	})
	if err != nil {
		log.Error().Err(err).Str("direction", string(direction)).Msg("failed to split pane")
		return err
	}

	// Set the new pane as active
	ws.ActivePaneID = output.NewPaneNode.Pane.ID

	// Rebuild the workspace view
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		if err := wsView.Rebuild(); err != nil {
			log.Error().Err(err).Msg("failed to rebuild workspace view")
		}
		a.attachWorkspaceWebViews(ctx, ws, wsView)
		wsView.FocusPane(ws.ActivePaneID)
	}

	log.Info().Str("direction", string(direction)).Str("new_pane_id", string(output.NewPaneNode.Pane.ID)).Msg("pane split completed")

	return nil
}

// handleClosePane closes the active pane.
func (a *App) handleClosePane(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if a.panesUC == nil {
		log.Warn().Msg("panes use case not available")
		return nil
	}

	ws := a.activeWorkspace()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	activePane := ws.ActivePane()
	if activePane == nil {
		log.Warn().Msg("no active pane to close")
		return nil
	}
	closingPaneID := activePane.Pane.ID

	// Don't close the last pane - close the tab instead
	if ws.PaneCount() <= 1 {
		return a.handleCloseTab(ctx)
	}

	_, err := a.panesUC.Close(ctx, ws, activePane)
	if err != nil {
		log.Error().Err(err).Msg("failed to close pane")
		return err
	}

	// Rebuild the workspace view
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		if err := wsView.Rebuild(); err != nil {
			log.Error().Err(err).Msg("failed to rebuild workspace view")
		}
		a.releasePaneWebView(ctx, closingPaneID)
		a.attachWorkspaceWebViews(ctx, ws, wsView)
		wsView.FocusPane(ws.ActivePaneID)
	}

	log.Info().Msg("pane closed")
	return nil
}

// handlePaneFocus navigates focus to an adjacent pane.
func (a *App) handlePaneFocus(ctx context.Context, direction usecase.NavigateDirection) error {
	log := logging.FromContext(ctx)

	if a.panesUC == nil {
		log.Warn().Msg("panes use case not available")
		return nil
	}

	ws := a.activeWorkspace()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	newPane, err := a.panesUC.NavigateFocus(ctx, ws, direction)
	if err != nil {
		log.Error().Err(err).Str("direction", string(direction)).Msg("failed to navigate focus")
		return err
	}

	if newPane == nil {
		log.Debug().Str("direction", string(direction)).Msg("no pane in that direction")
		return nil
	}

	// Update the workspace view's active pane
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		if err := wsView.SetActivePaneID(newPane.Pane.ID); err != nil {
			log.Warn().Err(err).Msg("failed to update active pane in view")
		} else {
			wsView.FocusPane(newPane.Pane.ID)
		}
	}

	log.Debug().Str("direction", string(direction)).Str("new_pane_id", newPane.ID).Msg("focus navigated")

	return nil
}

// registerMessageHandlers wires frontend message types to Go handlers.
func (a *App) registerMessageHandlers(ctx context.Context) error {
	if a.router == nil {
		return fmt.Errorf("message router is nil")
	}

	// Omnibox search
	if err := a.router.RegisterHandlerWithCallbacks("query", "__dumber_omnibox_suggestions", "", "", uihandler.NewQueryHandler(a.deps.HistoryUC, a.deps.Config)); err != nil {
		return err
	}
	if err := a.router.RegisterHandlerWithCallbacks("omnibox_initial_history", "__dumber_omnibox_suggestions", "", "", uihandler.NewInitialHistoryHandler(a.deps.HistoryUC, a.deps.Config)); err != nil {
		return err
	}
	if err := a.router.RegisterHandlerWithCallbacks("prefix_query", "__dumber_omnibox_inline_suggestion", "", "", uihandler.NewPrefixQueryHandler(a.deps.HistoryUC)); err != nil {
		return err
	}

	// Navigation
	if err := a.router.RegisterHandler("navigate", uihandler.NewNavigateHandler()); err != nil {
		return err
	}

	// Favorites
	if err := a.router.RegisterHandlerWithCallbacks("get_favorites", "__dumber_favorites", "", "", uihandler.NewFavoritesHandler(a.deps.FavoritesUC)); err != nil {
		return err
	}
	if err := a.router.RegisterHandlerWithCallbacks("toggle_favorite", "__dumber_favorites", "", "", uihandler.NewToggleFavoriteHandler(a.deps.FavoritesUC)); err != nil {
		return err
	}

	// Shortcuts + palettes
	if err := a.router.RegisterHandlerWithCallbacks("get_search_shortcuts", "__dumber_search_shortcuts", "__dumber_search_shortcuts_error", "", uihandler.NewShortcutsHandler(a.deps.Config)); err != nil {
		return err
	}
	if err := a.router.RegisterHandlerWithCallbacks("get_color_palettes", "__dumber_color_palettes", "__dumber_color_palettes_error", "", uihandler.NewPaletteHandler(a.deps.Config)); err != nil {
		return err
	}

	// Keyboard blocking
	if err := a.router.RegisterHandler("keyboard_blocking", uihandler.NewKeyboardBlockingHandler()); err != nil {
		return err
	}

	return nil
}

// attachWorkspaceWebViews ensures each pane has a WebView widget attached.
func (a *App) attachWorkspaceWebViews(ctx context.Context, ws *entity.Workspace, wsView *component.WorkspaceView) {
	log := logging.FromContext(ctx)

	if ws == nil || wsView == nil || a.widgetFactory == nil {
		return
	}

	for _, pane := range ws.AllPanes() {
		if pane == nil {
			continue
		}

		wv, err := a.ensureWebViewForPane(ctx, pane.ID)
		if err != nil {
			log.Warn().Err(err).Str("pane_id", string(pane.ID)).Msg("failed to ensure webview for pane")
			continue
		}

		// Load the pane's URI if set and different from current
		if pane.URI != "" && pane.URI != wv.URI() {
			if err := wv.LoadURI(ctx, pane.URI); err != nil {
				log.Warn().Err(err).Str("pane_id", string(pane.ID)).Str("uri", pane.URI).Msg("failed to load pane URI")
			}
		}

		widget := a.wrapWebViewWidget(ctx, wv)
		if widget == nil {
			continue
		}

		if err := wsView.SetWebViewWidget(pane.ID, widget); err != nil {
			log.Warn().Err(err).Str("pane_id", string(pane.ID)).Msg("failed to attach webview widget")
		}
	}
}

// ensureWebViewForPane acquires or reuses a WebView for the given pane.
func (a *App) ensureWebViewForPane(ctx context.Context, paneID entity.PaneID) (*webkit.WebView, error) {
	if wv, ok := a.webViews[paneID]; ok && wv != nil && !wv.IsDestroyed() {
		return wv, nil
	}
	if a.pool == nil {
		return nil, fmt.Errorf("webview pool not configured")
	}

	wv, err := a.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	a.webViews[paneID] = wv
	return wv, nil
}

// releasePaneWebView returns the WebView for a pane to the pool.
func (a *App) releasePaneWebView(ctx context.Context, paneID entity.PaneID) {
	wv, ok := a.webViews[paneID]
	if !ok || wv == nil {
		return
	}
	delete(a.webViews, paneID)

	if a.pool != nil {
		a.pool.Release(wv)
	} else {
		wv.Destroy()
	}
}

// wrapWebViewWidget converts a WebView to a layout.Widget for embedding.
func (a *App) wrapWebViewWidget(ctx context.Context, wv *webkit.WebView) layout.Widget {
	if wv == nil || a.widgetFactory == nil {
		return nil
	}
	gtkView := wv.Widget()
	if gtkView == nil {
		return nil
	}
	return a.widgetFactory.WrapWidget(&gtkView.Widget)
}

// activeWebView returns the WebView for the active pane.
func (a *App) activeWebView(ctx context.Context) *webkit.WebView {
	ws := a.activeWorkspace()
	if ws == nil {
		return nil
	}
	pane := ws.ActivePane()
	if pane == nil || pane.Pane == nil {
		return nil
	}
	return a.webViews[pane.Pane.ID]
}

// openOmnibox toggles the omnibox overlay for the active WebView.
func (a *App) openOmnibox(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if a.overlay == nil {
		log.Error().Msg("overlay controller not configured")
		return fmt.Errorf("overlay controller not configured")
	}
	wv := a.activeWebView(ctx)
	if wv == nil {
		log.Error().Msg("no active webview for omnibox")
		return fmt.Errorf("no active webview for omnibox")
	}
	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("opening omnibox")
	return a.overlay.Show(ctx, wv.ID(), "omnibox", "")
}

// openDevTools opens the WebKit inspector for the active WebView.
func (a *App) openDevTools(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := a.activeWebView(ctx)
	if wv == nil {
		log.Warn().Msg("no active webview for devtools")
		return fmt.Errorf("no active webview for devtools")
	}
	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("opening devtools")
	return wv.ShowDevTools()
}

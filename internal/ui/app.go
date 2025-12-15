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

	// Pane management
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

	// Web content
	pool       *webkit.WebViewPool
	injector   *webkit.ContentInjector
	router     *webkit.MessageRouter
	settings   *webkit.SettingsManager
	webViews   map[entity.PaneID]*webkit.WebView
	paneTitles map[entity.PaneID]string // Dynamic title tracking
	titleMu    sync.RWMutex

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
		webViews:       make(map[entity.PaneID]*webkit.WebView),
		paneTitles:     make(map[entity.PaneID]string),
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

	// Create keyboard handler
	a.keyboardHandler = input.NewKeyboardHandler(
		ctx,
		&a.deps.Config.Workspace,
	)
	a.keyboardHandler.SetOnAction(a.handleKeyboardAction)
	// Wrap handleModeChange to match the callback signature
	a.keyboardHandler.SetOnModeChange(func(from, to input.Mode) {
		a.handleModeChange(ctx, from, to)
	})
	a.keyboardHandler.AttachTo(a.mainWindow.Window())

	// Create native omnibox
	a.omnibox = component.NewOmnibox(ctx, a.mainWindow.Window(), component.OmniboxConfig{
		HistoryUC:       a.deps.HistoryUC,
		FavoritesUC:     a.deps.FavoritesUC,
		Shortcuts:       a.deps.Config.SearchShortcuts,
		DefaultSearch:   a.deps.Config.DefaultSearchEngine,
		InitialBehavior: a.deps.Config.Omnibox.InitialBehavior,
	})
	if a.omnibox != nil {
		a.omnibox.SetOnNavigate(func(url string) {
			a.navigateActivePane(ctx, url)
		})
		log.Debug().Msg("native omnibox created")
	} else {
		log.Warn().Msg("failed to create native omnibox")
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
		log := logging.FromContext(ctx)
		log.Error().Err(err).Msg("failed to create application")
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
	case input.ActionStackPane:
		return a.handleStackPane(ctx)
	case input.ActionFocusRight:
		return a.handlePaneFocus(ctx, usecase.NavRight)
	case input.ActionFocusLeft:
		return a.handlePaneFocus(ctx, usecase.NavLeft)
	case input.ActionFocusUp:
		return a.handlePaneFocus(ctx, usecase.NavUp)
	case input.ActionFocusDown:
		return a.handlePaneFocus(ctx, usecase.NavDown)

	// Stack navigation
	case input.ActionStackNavUp:
		return a.handleStackNavigate(ctx, "up")
	case input.ActionStackNavDown:
		return a.handleStackNavigate(ctx, "down")

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

	// Update border overlay visibility based on mode
	if a.borderMgr != nil {
		a.borderMgr.OnModeChange(ctx, from, to)
	}
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

	// Debug: trace entry
	hideEnabled := true
	if a.deps.Config != nil {
		hideEnabled = a.deps.Config.Workspace.HideTabBarWhenSingleTab
	}
	log.Debug().Bool("hide_enabled", hideEnabled).Msg("updateTabBarVisibility called")

	// Check if feature is enabled
	if a.deps.Config != nil && !a.deps.Config.Workspace.HideTabBarWhenSingleTab {
		log.Debug().Msg("tab bar auto-hide disabled by config")
		return
	}

	if a.mainWindow == nil {
		log.Debug().Msg("mainWindow is nil, skipping visibility update")
		return
	}
	if a.mainWindow.TabBar() == nil {
		log.Debug().Msg("tabBar is nil, skipping visibility update")
		return
	}

	tabCount := a.tabs.Count()
	shouldShow := tabCount > 1

	log.Debug().Int("tab_count", tabCount).Bool("should_show", shouldShow).Msg("setting tab bar visibility")
	a.mainWindow.TabBar().SetVisible(shouldShow)
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
	a.attachWorkspaceWebViews(ctx, tab.Workspace, wsView)

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

	ws := a.activeWorkspace()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	wsView := a.activeWorkspaceView()
	if wsView == nil {
		log.Warn().Msg("no active workspace view")
		return nil
	}

	// Use geometric navigation if focus manager is available
	var newPane *entity.PaneNode
	var err error

	if a.focusMgr != nil {
		newPane, err = a.focusMgr.NavigateGeometric(ctx, ws, wsView, direction)
	} else if a.panesUC != nil {
		// Fallback to structural navigation
		newPane, err = a.panesUC.NavigateFocus(ctx, ws, direction)
	} else {
		log.Warn().Msg("no navigation manager available")
		return nil
	}

	if err != nil {
		log.Error().Err(err).Str("direction", string(direction)).Msg("failed to navigate focus")
		return err
	}

	if newPane == nil {
		log.Debug().Str("direction", string(direction)).Msg("no pane in that direction")
		return nil
	}

	// Update the workspace view's active pane
	if err := wsView.SetActivePaneID(newPane.Pane.ID); err != nil {
		log.Warn().Err(err).Msg("failed to update active pane in view")
	} else {
		wsView.FocusPane(newPane.Pane.ID)
	}

	// Sync StackedView visibility if new pane is in a stack
	if newPane.Parent != nil && newPane.Parent.IsStacked {
		a.syncStackedViewActive(ctx, wsView, newPane)
	}

	log.Debug().Str("direction", string(direction)).Str("new_pane_id", newPane.ID).Msg("focus navigated")

	return nil
}

// syncStackedViewActive updates the StackedView's visibility to match the domain model.
// Call this after navigating focus to a pane inside a stack.
func (a *App) syncStackedViewActive(ctx context.Context, wsView *component.WorkspaceView, paneNode *entity.PaneNode) {
	log := logging.FromContext(ctx)

	if paneNode == nil || paneNode.Parent == nil || !paneNode.Parent.IsStacked {
		return
	}

	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}

	// Get the StackedView for this pane
	stackedView := tr.GetStackedViewForPane(string(paneNode.Pane.ID))
	if stackedView == nil {
		log.Warn().Str("pane_id", string(paneNode.Pane.ID)).Msg("stacked view not found for pane")
		return
	}

	// Use the parent's ActiveStackIndex which was set by the focus manager
	stackIndex := paneNode.Parent.ActiveStackIndex

	log.Debug().
		Str("pane_id", string(paneNode.Pane.ID)).
		Int("stack_index", stackIndex).
		Msg("syncing stacked view visibility")

	if err := stackedView.SetActive(ctx, stackIndex); err != nil {
		log.Warn().Err(err).Msg("failed to set stacked view active index")
	}
}

// handleStackPane adds a new pane stacked on top of the active pane.
// This modifies both the domain tree and the UI StackedView.
func (a *App) handleStackPane(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if a.stackedPaneMgr == nil {
		log.Warn().Msg("stacked pane manager not available")
		return nil
	}

	ws := a.activeWorkspace()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	activeNode := ws.ActivePane()
	if activeNode == nil || activeNode.Pane == nil {
		log.Warn().Msg("no active pane")
		return nil
	}

	wsView := a.activeWorkspaceView()
	if wsView == nil {
		log.Warn().Msg("no workspace view")
		return nil
	}

	activePaneID := activeNode.Pane.ID

	// Create a new pane entity
	newPaneID := entity.PaneID(a.generateID())
	newPane := entity.NewPane(newPaneID)
	newPane.URI = "about:blank"
	newPane.Title = "Untitled"

	// Get the original pane's current title
	originalTitle := a.getPaneTitle(activePaneID)
	if originalTitle == "" {
		originalTitle = activeNode.Pane.Title
	}
	if originalTitle == "" {
		originalTitle = "Untitled"
	}

	// Update domain model: convert leaf to stacked if needed, add new pane
	var stackNode *entity.PaneNode
	var needsFirstPaneTitleUpdate bool
	if activeNode.IsStacked {
		// Already stacked, just add to it
		stackNode = activeNode
	} else {
		// Convert leaf node to stacked container
		// Move current pane to a child node
		originalPane := activeNode.Pane
		originalPane.Title = originalTitle // Ensure title is set
		originalPaneChild := &entity.PaneNode{
			ID:     activeNode.ID + "_0",
			Pane:   originalPane,
			Parent: activeNode,
		}

		activeNode.Pane = nil // No longer a leaf
		activeNode.IsStacked = true
		activeNode.Children = []*entity.PaneNode{originalPaneChild}
		stackNode = activeNode
		needsFirstPaneTitleUpdate = true

		log.Debug().
			Str("node_id", activeNode.ID).
			Str("original_pane", string(originalPane.ID)).
			Str("original_title", originalTitle).
			Msg("converted leaf to stacked node")
	}

	// Create new child node for the new pane
	newChildNode := &entity.PaneNode{
		ID:     stackNode.ID + "_" + string(newPaneID),
		Pane:   newPane,
		Parent: stackNode,
	}
	stackNode.Children = append(stackNode.Children, newChildNode)
	stackNode.ActiveStackIndex = len(stackNode.Children) - 1

	log.Debug().
		Int("stack_size", len(stackNode.Children)).
		Int("active_index", stackNode.ActiveStackIndex).
		Msg("domain tree updated")

	// Create PaneView for the new pane
	newPaneView := component.NewPaneView(a.widgetFactory, newPaneID, nil)
	wsView.RegisterPaneView(newPaneID, newPaneView)

	// Add to the UI StackedView
	if err := a.stackedPaneMgr.AddPaneToStack(ctx, wsView, activePaneID, newPaneView, "Untitled"); err != nil {
		log.Error().Err(err).Msg("failed to add pane to stack")
		return err
	}

	// Update the first pane's title if we just converted from leaf to stacked
	if needsFirstPaneTitleUpdate {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(activePaneID))
			if stackedView != nil {
				if err := stackedView.UpdateTitle(0, originalTitle); err != nil {
					log.Warn().Err(err).Str("title", originalTitle).Msg("failed to update first pane title")
				} else {
					log.Debug().Str("title", originalTitle).Msg("updated first pane title in StackedView")
				}
			}
		}
	}

	// Get WebView and attach
	wv, err := a.ensureWebViewForPane(ctx, newPaneID)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get webview for new pane")
	} else {
		widget := a.wrapWebViewWidget(ctx, wv)
		if widget != nil {
			newPaneView.SetWebViewWidget(widget)
		}
		// Load blank page
		if err := wv.LoadURI(ctx, "about:blank"); err != nil {
			log.Warn().Err(err).Msg("failed to load blank page")
		}
	}

	// Update workspace active pane ID
	ws.ActivePaneID = newPaneID

	// Update workspace view
	if err := wsView.SetActivePaneID(newPaneID); err != nil {
		log.Warn().Err(err).Msg("failed to set active pane")
	}

	// Set up title bar click callback
	tr := wsView.TreeRenderer()
	if tr != nil {
		stackedView := tr.GetStackedViewForPane(string(activePaneID))
		if stackedView != nil {
			// Capture stackNode for the callback
			capturedStackNode := stackNode
			stackedView.SetOnActivate(func(index int) {
				a.handleTitleBarClick(ctx, capturedStackNode, stackedView, index)
			})
		}
	}

	log.Info().
		Str("original_pane", string(activePaneID)).
		Str("new_pane", string(newPaneID)).
		Int("stack_size", len(stackNode.Children)).
		Msg("stacked new pane")

	return nil
}

// handleStackNavigate navigates up or down within a stacked pane container.
// With the new architecture, every pane is in a StackedView - we just navigate within it.
func (a *App) handleStackNavigate(ctx context.Context, direction string) error {
	log := logging.FromContext(ctx)

	if a.stackedPaneMgr == nil {
		return nil
	}

	ws := a.activeWorkspace()
	if ws == nil {
		return nil
	}

	activePane := ws.ActivePane()
	if activePane == nil || activePane.Pane == nil {
		return nil
	}

	wsView := a.activeWorkspaceView()
	if wsView == nil {
		return nil
	}

	currentPaneID := activePane.Pane.ID

	// Check if we're actually in a multi-pane stack
	if !a.stackedPaneMgr.IsStacked(wsView, currentPaneID) {
		log.Debug().Msg("current pane is not in a multi-pane stack")
		return nil
	}

	// Navigate within the stack
	_, err := a.stackedPaneMgr.NavigateStack(ctx, wsView, currentPaneID, direction)
	if err != nil {
		log.Warn().Err(err).Str("direction", direction).Msg("failed to navigate stack")
		return err
	}

	// Note: With the current StackedView navigation, the new pane ID is not returned
	// because the mapping from visual index to pane ID requires additional tracking.
	// For now, the StackedView handles the visual navigation.
	// TODO: Add pane ID tracking to StackedView for proper domain model sync

	log.Debug().
		Str("direction", direction).
		Str("current_pane", string(currentPaneID)).
		Msg("navigated stack")

	return nil
}

// handleStackPaneActivated is called when a pane in a stack is activated via title bar click.
// This is a stub for future use when title bar click callbacks are wired up.
// The paneID parameter would come from the StackedView's OnActivate callback.
func (a *App) handleStackPaneActivated(ctx context.Context, paneID entity.PaneID) {
	log := logging.FromContext(ctx)

	if paneID == "" {
		return
	}

	// Update workspace active pane
	ws := a.activeWorkspace()
	if ws != nil {
		ws.ActivePaneID = paneID
	}

	// Update workspace view
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		if err := wsView.SetActivePaneID(paneID); err != nil {
			log.Warn().Err(err).Msg("failed to set active pane after stack activation")
		}
	}

	log.Debug().
		Str("pane_id", string(paneID)).
		Msg("stack pane activated")
}

// handleTitleBarClick handles clicks on title bars to switch the active pane in a stack.
func (a *App) handleTitleBarClick(ctx context.Context, stackNode *entity.PaneNode, sv *layout.StackedView, clickedIndex int) {
	log := logging.FromContext(ctx)

	if stackNode == nil || sv == nil {
		return
	}

	// Validate index
	if clickedIndex < 0 || clickedIndex >= len(stackNode.Children) {
		log.Warn().Int("index", clickedIndex).Int("children", len(stackNode.Children)).Msg("invalid stack index clicked")
		return
	}

	// Get the current active index
	currentIndex := sv.ActiveIndex()
	if clickedIndex == currentIndex {
		log.Debug().Int("index", clickedIndex).Msg("clicked pane is already active")
		return
	}

	// Update the outgoing pane's title bar with its current webpage title
	if currentIndex >= 0 && currentIndex < len(stackNode.Children) {
		outgoingChild := stackNode.Children[currentIndex]
		if outgoingChild.Pane != nil {
			outgoingPaneID := outgoingChild.Pane.ID
			outgoingTitle := a.getPaneTitle(outgoingPaneID)
			if outgoingTitle == "" {
				outgoingTitle = outgoingChild.Pane.Title
			}
			if outgoingTitle != "" {
				if err := sv.UpdateTitle(currentIndex, outgoingTitle); err != nil {
					log.Warn().Err(err).Msg("failed to update outgoing pane title")
				}
			}
		}
	}

	// Get the pane ID at the clicked index
	clickedChild := stackNode.Children[clickedIndex]
	if clickedChild.Pane == nil {
		log.Warn().Int("index", clickedIndex).Msg("clicked child has no pane")
		return
	}
	clickedPaneID := clickedChild.Pane.ID

	// Update StackedView active index
	if err := sv.SetActive(ctx, clickedIndex); err != nil {
		log.Warn().Err(err).Int("index", clickedIndex).Msg("failed to set active pane in stack")
		return
	}

	// Update domain model
	stackNode.ActiveStackIndex = clickedIndex

	// Update workspace active pane
	ws := a.activeWorkspace()
	if ws != nil {
		ws.ActivePaneID = clickedPaneID
	}

	// Update workspace view
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		if err := wsView.SetActivePaneID(clickedPaneID); err != nil {
			log.Warn().Err(err).Msg("failed to set active pane in workspace view")
		}
	}

	log.Info().
		Int("from_index", currentIndex).
		Int("to_index", clickedIndex).
		Str("pane_id", string(clickedPaneID)).
		Msg("switched active pane via title bar click")
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

	// Set up title change callback to track dynamic titles
	wv.OnTitleChanged = func(title string) {
		a.handlePaneTitleChanged(ctx, paneID, title)
	}

	return wv, nil
}

// handlePaneTitleChanged updates title tracking when a WebView's title changes.
func (a *App) handlePaneTitleChanged(ctx context.Context, paneID entity.PaneID, title string) {
	log := logging.FromContext(ctx)

	// Update title map
	a.titleMu.Lock()
	a.paneTitles[paneID] = title
	a.titleMu.Unlock()

	// Update domain model
	ws := a.activeWorkspace()
	if ws != nil {
		paneNode := ws.FindPane(paneID)
		if paneNode != nil && paneNode.Pane != nil {
			paneNode.Pane.Title = title
		}
	}

	// Update StackedView title bar if this pane is in a stack
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(paneID))
			if stackedView != nil {
				// Find the pane's index in the stack and update title
				a.updateStackedPaneTitle(ctx, stackedView, paneID, title)
			}
		}
	}

	log.Debug().
		Str("pane_id", string(paneID)).
		Str("title", title).
		Msg("pane title updated")
}

// updateStackedPaneTitle updates the title of a pane in a StackedView.
func (a *App) updateStackedPaneTitle(ctx context.Context, sv *layout.StackedView, paneID entity.PaneID, title string) {
	// We need to find which index this pane is at in the stack
	// For now, we track by iterating - could be optimized with reverse mapping
	wsView := a.activeWorkspaceView()
	if wsView == nil {
		return
	}

	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}

	// Find the stack node in the domain to get the index
	ws := a.activeWorkspace()
	if ws == nil {
		return
	}

	paneNode := ws.FindPane(paneID)
	if paneNode == nil {
		return
	}

	// If the pane is in a stacked parent, find its index
	if paneNode.Parent != nil && paneNode.Parent.IsStacked {
		for i, child := range paneNode.Parent.Children {
			if child.Pane != nil && child.Pane.ID == paneID {
				_ = sv.UpdateTitle(i, title)
				return
			}
		}
	}
}

// getPaneTitle returns the current title for a pane.
func (a *App) getPaneTitle(paneID entity.PaneID) string {
	a.titleMu.RLock()
	defer a.titleMu.RUnlock()
	return a.paneTitles[paneID]
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

// openOmnibox toggles the native GTK omnibox.
func (a *App) openOmnibox(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if a.omnibox == nil {
		log.Error().Msg("native omnibox not initialized")
		return fmt.Errorf("native omnibox not initialized")
	}

	log.Debug().Msg("toggling native omnibox")
	a.omnibox.Toggle(ctx)
	return nil
}

// navigateActivePane navigates the active pane to the given URL.
// Records history for omnibox search to find it immediately.
func (a *App) navigateActivePane(ctx context.Context, url string) {
	log := logging.FromContext(ctx)

	wv := a.activeWebView(ctx)
	if wv == nil {
		log.Warn().Str("url", url).Msg("no active webview for navigation")
		return
	}

	// Load the URL
	if err := wv.LoadURI(ctx, url); err != nil {
		log.Error().Err(err).Str("url", url).Msg("failed to navigate")
		return
	}

	// Record in history asynchronously so it appears in omnibox immediately
	if a.deps.HistoryRepo != nil {
		go a.recordHistory(ctx, url)
	}

	log.Debug().Str("url", url).Msg("navigated active pane")
}

// recordHistory saves or updates a history entry.
func (a *App) recordHistory(ctx context.Context, url string) {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", url).Msg("recording history entry")

	existing, err := a.deps.HistoryRepo.FindByURL(ctx, url)
	if err != nil {
		log.Warn().Err(err).Str("url", url).Msg("failed to check history")
		return
	}

	if existing != nil {
		// Increment visit count
		if err := a.deps.HistoryRepo.IncrementVisitCount(ctx, url); err != nil {
			log.Warn().Err(err).Str("url", url).Msg("failed to increment visit count")
		} else {
			log.Debug().Str("url", url).Int64("prev_count", existing.VisitCount).Msg("history visit count incremented")
		}
	} else {
		// Create new entry
		entry := entity.NewHistoryEntry(url, "")
		if err := a.deps.HistoryRepo.Save(ctx, entry); err != nil {
			log.Warn().Err(err).Str("url", url).Msg("failed to save history")
		} else {
			log.Debug().Str("url", url).Msg("new history entry saved")
		}
	}
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

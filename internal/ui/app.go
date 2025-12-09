package ui

import (
	"context"
	"os"
	"sync"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/rs/zerolog"
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

	// ID generator for tabs/panes
	idCounter uint64
	idMu      sync.Mutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	logger *zerolog.Logger
}

// New creates a new App with the given dependencies.
func New(deps *Dependencies) (*App, error) {
	if err := deps.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(deps.Ctx)
	logger := logging.FromContext(ctx)

	app := &App{
		deps:           deps,
		tabs:           entity.NewTabList(),
		tabsUC:         deps.TabsUC,
		panesUC:        deps.PanesUC,
		workspaceViews: make(map[entity.TabID]*component.WorkspaceView),
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
	}

	return app, nil
}

// Run starts the GTK application and blocks until it exits.
// Returns the exit code.
func (a *App) Run(args []string) int {
	a.logger.Debug().Msg("creating GTK application")

	a.gtkApp = gtk.NewApplication(AppID, gio.GApplicationFlagsNoneValue)
	if a.gtkApp == nil {
		a.logger.Error().Msg("failed to create GTK application")
		return 1
	}
	defer a.gtkApp.Unref()

	// Connect activate signal
	activateCb := func(_ gio.Application) {
		a.onActivate()
	}
	a.gtkApp.ConnectActivate(&activateCb)

	// Connect shutdown signal
	shutdownCb := func(_ gio.Application) {
		a.onShutdown()
	}
	a.gtkApp.ConnectShutdown(&shutdownCb)

	a.logger.Info().Msg("starting GTK main loop")
	code := a.gtkApp.Run(len(args), args)

	return code
}

// onActivate is called when the GTK application is activated.
func (a *App) onActivate() {
	a.logger.Debug().Msg("GTK application activated")

	// Create the main window
	var err error
	a.mainWindow, err = window.New(a.gtkApp, a.deps.Config, a.logger)
	if err != nil {
		a.logger.Error().Err(err).Msg("failed to create main window")
		return
	}

	// Initialize widget factory for pane layout
	a.widgetFactory = layout.NewGtkWidgetFactory()

	// Create keyboard handler
	a.keyboardHandler = input.NewKeyboardHandler(
		a.ctx,
		&a.deps.Config.Workspace,
		a.logger,
	)
	a.keyboardHandler.SetOnAction(a.handleKeyboardAction)
	a.keyboardHandler.SetOnModeChange(a.handleModeChange)
	a.keyboardHandler.AttachTo(a.mainWindow.Window())

	// Create an initial tab
	a.createInitialTab()

	// Show the window
	a.mainWindow.Show()

	a.logger.Info().Msg("main window displayed")
}

// onShutdown is called when the GTK application is shutting down.
func (a *App) onShutdown() {
	a.logger.Debug().Msg("GTK application shutting down")

	// Cancel context to signal all goroutines
	a.cancel()

	// Cleanup resources
	if a.deps.Pool != nil {
		a.deps.Pool.Close()
	}

	a.logger.Info().Msg("application shutdown complete")
}

// createInitialTab creates the first tab when the application starts.
func (a *App) createInitialTab() {
	if a.tabsUC == nil {
		// Create use case if not injected
		a.tabsUC = usecase.NewManageTabsUseCase(a.generateID)
	}

	output, err := a.tabsUC.Create(a.ctx, usecase.CreateTabInput{
		TabList:    a.tabs,
		Name:       "",
		InitialURL: "about:blank",
	})
	if err != nil {
		a.logger.Error().Err(err).Msg("failed to create initial tab")
		return
	}

	// Update tab bar
	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().AddTab(output.Tab)
		a.mainWindow.TabBar().SetActive(output.Tab.ID)
	}

	// Create workspace view for this tab
	a.createWorkspaceView(output.Tab)

	a.logger.Debug().
		Str("tab_id", string(output.Tab.ID)).
		Msg("initial tab created")
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
func RunWithArgs(deps *Dependencies) int {
	app, err := New(deps)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Error().Err(err).Msg("failed to create application")
		}
		return 1
	}
	return app.Run(os.Args)
}

// handleKeyboardAction dispatches keyboard actions to appropriate handlers.
func (a *App) handleKeyboardAction(ctx context.Context, action input.Action) error {
	a.logger.Debug().
		Str("action", string(action)).
		Msg("handling keyboard action")

	switch action {
	// Tab actions
	case input.ActionNewTab:
		return a.handleNewTab()
	case input.ActionCloseTab:
		return a.handleCloseTab()
	case input.ActionNextTab:
		return a.handleNextTab()
	case input.ActionPreviousTab:
		return a.handlePreviousTab()

	// Pane actions
	case input.ActionSplitRight:
		return a.handlePaneSplit(usecase.SplitRight)
	case input.ActionSplitLeft:
		return a.handlePaneSplit(usecase.SplitLeft)
	case input.ActionSplitUp:
		return a.handlePaneSplit(usecase.SplitUp)
	case input.ActionSplitDown:
		return a.handlePaneSplit(usecase.SplitDown)
	case input.ActionClosePane:
		return a.handleClosePane()
	case input.ActionFocusRight:
		return a.handlePaneFocus(usecase.NavRight)
	case input.ActionFocusLeft:
		return a.handlePaneFocus(usecase.NavLeft)
	case input.ActionFocusUp:
		return a.handlePaneFocus(usecase.NavUp)
	case input.ActionFocusDown:
		return a.handlePaneFocus(usecase.NavDown)

	// Navigation (stub implementations for now)
	case input.ActionGoBack:
		a.logger.Debug().Msg("go back action (not yet implemented)")
	case input.ActionGoForward:
		a.logger.Debug().Msg("go forward action (not yet implemented)")
	case input.ActionReload:
		a.logger.Debug().Msg("reload action (not yet implemented)")
	case input.ActionHardReload:
		a.logger.Debug().Msg("hard reload action (not yet implemented)")

	// Zoom (stub implementations for now)
	case input.ActionZoomIn:
		a.logger.Debug().Msg("zoom in action (not yet implemented)")
	case input.ActionZoomOut:
		a.logger.Debug().Msg("zoom out action (not yet implemented)")
	case input.ActionZoomReset:
		a.logger.Debug().Msg("zoom reset action (not yet implemented)")

	// UI
	case input.ActionOpenOmnibox:
		a.logger.Debug().Msg("open omnibox action (not yet implemented)")
	case input.ActionOpenDevTools:
		a.logger.Debug().Msg("open devtools action (not yet implemented)")
	case input.ActionToggleFullscreen:
		a.logger.Debug().Msg("toggle fullscreen action (not yet implemented)")

	// Application
	case input.ActionQuit:
		a.Quit()

	default:
		a.logger.Warn().Str("action", string(action)).Msg("unhandled keyboard action")
	}

	return nil
}

// handleModeChange is called when the input mode changes.
func (a *App) handleModeChange(from, to input.Mode) {
	a.logger.Debug().
		Str("from", from.String()).
		Str("to", to.String()).
		Msg("input mode changed")

	// TODO: Update UI to indicate mode (e.g., change border color)
}

// handleNewTab creates a new tab.
func (a *App) handleNewTab() error {
	output, err := a.tabsUC.Create(a.ctx, usecase.CreateTabInput{
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

	a.logger.Debug().
		Str("tab_id", string(output.Tab.ID)).
		Msg("new tab created")

	return nil
}

// handleCloseTab closes the active tab.
func (a *App) handleCloseTab() error {
	activeID := a.tabs.ActiveTabID
	if activeID == "" {
		return nil
	}

	wasLast, err := a.tabsUC.Close(a.ctx, a.tabs, activeID)
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

	// Quit if no tabs left
	if wasLast {
		a.Quit()
	}

	return nil
}

// handleNextTab switches to the next tab.
func (a *App) handleNextTab() error {
	if err := a.tabsUC.SwitchNext(a.ctx, a.tabs); err != nil {
		return err
	}

	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().SetActive(a.tabs.ActiveTabID)
	}

	return nil
}

// handlePreviousTab switches to the previous tab.
func (a *App) handlePreviousTab() error {
	if err := a.tabsUC.SwitchPrevious(a.ctx, a.tabs); err != nil {
		return err
	}

	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().SetActive(a.tabs.ActiveTabID)
	}

	return nil
}

// createWorkspaceView creates a WorkspaceView for a tab and attaches it to the content area.
func (a *App) createWorkspaceView(tab *entity.Tab) {
	if a.widgetFactory == nil {
		a.logger.Error().Msg("widget factory not initialized")
		return
	}

	// Create workspace view
	wsView := component.NewWorkspaceView(a.widgetFactory)
	if wsView == nil {
		a.logger.Error().Msg("failed to create workspace view")
		return
	}

	// Set the workspace
	if err := wsView.SetWorkspace(tab.Workspace); err != nil {
		a.logger.Error().Err(err).Msg("failed to set workspace in view")
		return
	}

	// Store in map
	a.workspaceViews[tab.ID] = wsView

	// Attach to content area
	if a.mainWindow != nil {
		widget := wsView.Widget()
		if widget != nil {
			if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
				a.mainWindow.SetContent(gtkWidget)
			}
		}
	}

	a.logger.Debug().
		Str("tab_id", string(tab.ID)).
		Msg("workspace view created and attached")
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
func (a *App) handlePaneSplit(direction usecase.SplitDirection) error {
	if a.panesUC == nil {
		a.logger.Warn().Msg("panes use case not available")
		return nil
	}

	ws := a.activeWorkspace()
	if ws == nil {
		a.logger.Warn().Msg("no active workspace")
		return nil
	}

	activePane := ws.ActivePane()
	if activePane == nil {
		a.logger.Warn().Msg("no active pane to split")
		return nil
	}

	output, err := a.panesUC.Split(a.ctx, usecase.SplitPaneInput{
		Workspace:  ws,
		TargetPane: activePane,
		Direction:  direction,
	})
	if err != nil {
		a.logger.Error().Err(err).Str("direction", string(direction)).Msg("failed to split pane")
		return err
	}

	// Set the new pane as active
	ws.ActivePaneID = output.NewPaneNode.Pane.ID

	// Rebuild the workspace view
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		if err := wsView.Rebuild(); err != nil {
			a.logger.Error().Err(err).Msg("failed to rebuild workspace view")
		}
	}

	a.logger.Info().
		Str("direction", string(direction)).
		Str("new_pane_id", string(output.NewPaneNode.Pane.ID)).
		Msg("pane split completed")

	return nil
}

// handleClosePane closes the active pane.
func (a *App) handleClosePane() error {
	if a.panesUC == nil {
		a.logger.Warn().Msg("panes use case not available")
		return nil
	}

	ws := a.activeWorkspace()
	if ws == nil {
		a.logger.Warn().Msg("no active workspace")
		return nil
	}

	activePane := ws.ActivePane()
	if activePane == nil {
		a.logger.Warn().Msg("no active pane to close")
		return nil
	}

	// Don't close the last pane - close the tab instead
	if ws.PaneCount() <= 1 {
		return a.handleCloseTab()
	}

	_, err := a.panesUC.Close(a.ctx, ws, activePane)
	if err != nil {
		a.logger.Error().Err(err).Msg("failed to close pane")
		return err
	}

	// Rebuild the workspace view
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		if err := wsView.Rebuild(); err != nil {
			a.logger.Error().Err(err).Msg("failed to rebuild workspace view")
		}
	}

	a.logger.Info().Msg("pane closed")
	return nil
}

// handlePaneFocus navigates focus to an adjacent pane.
func (a *App) handlePaneFocus(direction usecase.NavigateDirection) error {
	if a.panesUC == nil {
		a.logger.Warn().Msg("panes use case not available")
		return nil
	}

	ws := a.activeWorkspace()
	if ws == nil {
		a.logger.Warn().Msg("no active workspace")
		return nil
	}

	newPane, err := a.panesUC.NavigateFocus(a.ctx, ws, direction)
	if err != nil {
		a.logger.Error().Err(err).Str("direction", string(direction)).Msg("failed to navigate focus")
		return err
	}

	if newPane == nil {
		a.logger.Debug().Str("direction", string(direction)).Msg("no pane in that direction")
		return nil
	}

	// Update the workspace view's active pane
	wsView := a.activeWorkspaceView()
	if wsView != nil {
		if err := wsView.SetActivePaneID(newPane.Pane.ID); err != nil {
			a.logger.Warn().Err(err).Msg("failed to update active pane in view")
		}
	}

	a.logger.Debug().
		Str("direction", string(direction)).
		Str("new_pane_id", newPane.ID).
		Msg("focus navigated")

	return nil
}

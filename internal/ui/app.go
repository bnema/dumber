package ui

import (
	"context"
	"os"
	"sync"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
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
		deps:   deps,
		tabs:   entity.NewTabList(),
		tabsUC: deps.TabsUC,
		ctx:    ctx,
		cancel: cancel,
		logger: logger,
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

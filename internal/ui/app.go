package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	urlutil "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
	"github.com/bnema/dumber/internal/infrastructure/filesystem"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/infrastructure/snapshot"
	"github.com/bnema/dumber/internal/infrastructure/textinput"
	"github.com/bnema/dumber/internal/infrastructure/webkit"

	"github.com/bnema/dumber/internal/infrastructure/webkit/handlers"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/dialog"
	"github.com/bnema/dumber/internal/ui/dispatcher"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	// AppID is the application identifier for GTK.
	AppID = "com.github.bnema.dumber"
	// crashReportToastDurationMs keeps startup crash-report toast visible longer.
	crashReportToastDurationMs = 5000
	floatingPaneFallbackWidth  = 1200
	floatingPaneFallbackHeight = 800
	floatingPaneIDPrefix       = "floating-pane:"
	floatingSessionIDDefault   = "default"
	floatingPaneVisibleClass   = "floating-pane-visible"
)

func gtkApplicationFlags() gio.ApplicationFlags {
	return gio.GApplicationNonUniqueValue
}

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
	omniboxCfg        component.OmniboxConfig
	omniboxNavigateFn func(url string)
	// Find bar configuration (find bar is created per workspace view)
	findBarCfg component.FindBarConfig
	// Floating pane sessions keyed by tab and profile session ID.
	floatingSessions map[floatingSessionKey]*floatingWorkspaceSession

	// Web content (managed by ContentCoordinator)
	pool           *webkit.WebViewPool
	webViewFactory *webkit.WebViewFactory
	injector       *webkit.ContentInjector
	router         *webkit.MessageRouter
	settings       *webkit.SettingsManager
	faviconAdapter *adapter.FaviconAdapter

	// App-level toaster for system notifications (filter status, etc.)
	appToaster *component.Toaster
	// Mode indicator toaster for modal mode notifications (pane, tab, session, resize)
	modeToaster *component.Toaster
	// Top-right WebRTC permission activity indicator
	webrtcIndicator *component.WebRTCPermissionIndicator

	// Session management
	sessionManager  *component.SessionManager
	tabPicker       *component.TabPicker
	tabPickerWidget layout.Widget
	tabPickerPaneID entity.PaneID
	snapshotService *snapshot.Service

	// Update management
	updateCoord *coordinator.UpdateCoordinator

	// ID generator for tabs/panes
	idCounter             uint64
	idMu                  sync.Mutex
	firstWebViewShownOnce sync.Once

	movePaneToTabUC *usecase.MovePaneToTabUseCase

	// Accent picker for dead keys support
	accentPicker        *component.AccentPicker
	insertAccentUC      *usecase.InsertAccentUseCase
	accentFocusProvider *textinput.FocusProvider

	// Deferred initialization - runs after first load_started to avoid blocking initial navigation
	deferredInitOnce sync.Once
	deferredInitFn   func()

	// lifecycle
	cancel context.CancelCauseFunc
}

type floatingWorkspaceSession struct {
	paneID              entity.PaneID
	pane                *component.FloatingPane
	paneView            *component.PaneView
	webView             *webkit.WebView
	overlay             layout.OverlayWidget
	widget              layout.Widget
	focusWidget         layout.Widget
	omnibox             *component.Omnibox
	omniboxWidget       layout.Widget
	resizeWatcherActive bool
	resizeTickID        uint
	appliedWidth        int
	appliedHeight       int
}

type floatingSessionKey struct {
	tabID     entity.TabID
	sessionID string
}

// New creates a new App with the given dependencies.
func New(deps *Dependencies) (*App, error) {
	if err := deps.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancelCause(deps.Ctx)

	app := &App{
		deps:             deps,
		tabs:             entity.NewTabList(),
		tabsUC:           deps.TabsUC,
		panesUC:          deps.PanesUC,
		workspaceViews:   make(map[entity.TabID]*component.WorkspaceView),
		floatingSessions: make(map[floatingSessionKey]*floatingWorkspaceSession),
		pool:             deps.Pool,
		injector:         deps.Injector,
		router:           deps.MessageRouter,
		settings:         deps.Settings,
		configManager:    config.GetManager(),
		cancel:           cancel,
	}
	if app.router == nil {
		app.router = webkit.NewMessageRouter(ctx)
	}

	// Register message handlers
	if app.router != nil {
		if err := handlers.RegisterAll(ctx, app.router, handlers.Config{
			HistoryUC:    deps.HistoryUC,
			FavoritesUC:  deps.FavoritesUC,
			Clipboard:    deps.Clipboard,
			ConfigGetter: config.Get,
			OnClipboardCopied: func(textLen int) {
				// Show brief toast on auto-copy (similar to zellij footer notification)
				// Must schedule on GTK main thread since this is called from WebKit handler
				cb := glib.SourceFunc(func(_ uintptr) bool {
					if app.appToaster != nil {
						app.appToaster.Show(ctx, "Copied to clipboard", component.ToastInfo,
							component.WithDuration(component.ToastBriefDurationMs),
							component.WithPosition(component.ToastPositionBottomRight),
						)
					}
					return false
				})
				glib.IdleAdd(&cb, 0)
			},
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

	// Initialize libadwaita once (required before using StyleManager).
	// This also initializes GTK implicitly.
	adw.Init()
	logging.Trace().Mark("gtk_init")

	// Mark adwaita detector as available now that adw.Init() is complete.
	// This enables the highest-priority color scheme detector.
	if a.deps != nil && a.deps.AdwaitaDetector != nil {
		a.deps.AdwaitaDetector.MarkAvailable()
		log.Debug().Msg("adwaita detector marked available")

		// Refresh scripts in pooled WebViews that were prewarmed before adw.Init().
		// They have the wrong dark mode preference injected; re-inject with correct value.
		if a.pool != nil {
			a.pool.RefreshScripts(ctx)
		}
	}

	appID := AppID
	a.gtkApp = gtk.NewApplication(&appID, gtkApplicationFlags())
	if a.gtkApp == nil {
		log.Error().Msg("failed to create GTK application")
		return 1
	}
	defer a.gtkApp.Unref()
	logging.Trace().Mark("gtk_app_created")

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
	// Configure pool background color early (prevents white flash), but avoid
	// synchronous prewarming that delays the first navigation on cold start.
	a.setupPoolBackgroundColor(ctx)

	if err := a.createMainWindow(ctx); err != nil {
		log.Error().Err(err).Msg("failed to create main window")
		return
	}
	logging.Trace().Mark("window_created")

	a.initLayoutInfrastructure()
	a.initAppToasterOverlay()
	a.installCrashReportNotifier(ctx)
	a.initFocusAndBorderOverlay()
	a.initAccentPicker(ctx)
	a.initDownloadHandler(ctx)

	a.initCoordinators(ctx)
	a.wireWebRTCPermissionIndicator()
	logging.Trace().Mark("coordinators_init")
	a.initKeyboardHandler(ctx)
	a.initOmniboxConfig(ctx)
	a.initFindBarConfig()
	a.initSessionManager(ctx)
	a.initTabPicker(ctx)
	a.initSnapshotService(ctx)
	a.initUpdateCoordinator(ctx)
	a.createInitialTab(ctx)
	a.finalizeActivation(ctx)
}

func (a *App) applyGTKColorSchemePreference(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Get the config color scheme preference (default follows system theme)
	scheme := "default"
	if a.deps != nil && a.deps.Config != nil && a.deps.Config.Appearance.ColorScheme != "" {
		scheme = a.deps.Config.Appearance.ColorScheme
	}

	// Apply color scheme via libadwaita's StyleManager.
	// This properly communicates the preference to GTK and WebKit,
	// so web content can correctly evaluate prefers-color-scheme media queries.
	styleMgr := adw.StyleManagerGetDefault()
	if styleMgr == nil {
		log.Warn().Msg("failed to get adw.StyleManager")
		return
	}

	var adwScheme adw.ColorScheme
	switch scheme {
	case "dark", "prefer-dark":
		adwScheme = adw.ColorSchemeForceDarkValue
	case "light", "prefer-light":
		adwScheme = adw.ColorSchemeForceLightValue
	default:
		// "default" or empty - follow system preference
		adwScheme = adw.ColorSchemeDefaultValue
	}

	styleMgr.SetColorScheme(adwScheme)

	// Refresh the color resolver now that adwaita detector is available.
	// This ensures all components get the correct preference.
	var pref string
	var prefersDark bool
	if a.deps != nil && a.deps.ColorResolver != nil {
		result := a.deps.ColorResolver.Refresh()
		prefersDark = result.PrefersDark
		pref = result.Source
	} else {
		prefersDark = styleMgr.GetDark()
		pref = "adw.StyleManager"
	}

	log.Debug().
		Str("scheme", scheme).
		Bool("prefers_dark", prefersDark).
		Str("source", pref).
		Msg("applied color scheme via adw.StyleManager")
}

func (a *App) setupPoolBackgroundColor(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Set theme background color on pool to eliminate white flash.
	// Must be done before any WebView creation so WebViews get the correct color.
	if a.pool == nil {
		log.Debug().Msg("webview pool not available; skipping background color setup")
		return
	}
	if a.deps == nil || a.deps.Theme == nil {
		log.Debug().Msg("theme not available; skipping webview pool background color setup")
		return
	}

	r, g, b, alpha := a.deps.Theme.GetBackgroundRGBA()
	a.pool.SetBackgroundColor(r, g, b, alpha)
	log.Debug().Msg("configured webview pool background color")
}

func (a *App) prewarmWebViewPoolAsync(ctx context.Context) {
	// Prewarm WebView pool after startup so cold-start navigation is not blocked.
	if a.pool == nil {
		return
	}
	a.pool.PrewarmAsync(ctx, 0)
}

func (a *App) createMainWindow(ctx context.Context) error {
	mainWindow, err := window.New(ctx, a.gtkApp, a.deps.Config)
	if err != nil {
		return err
	}
	a.mainWindow = mainWindow

	// Create permission popup and dialog presenter
	if a.deps != nil && a.deps.PermissionUC != nil {
		uiScale := 1.0
		if a.deps.Config != nil {
			uiScale = a.deps.Config.DefaultUIScale
		}
		permPopup := component.NewPermissionPopup(nil, uiScale)
		if permPopup != nil {
			// Add popup to the main window's content overlay
			if w := permPopup.Widget(); w != nil {
				a.mainWindow.AddOverlay(w)
			}
			permDialog := dialog.NewPermissionDialog(permPopup)
			a.deps.PermissionUC.SetDialogPresenter(permDialog)
		}
	}

	// Create top-right WebRTC permission activity indicator.
	indicator := component.NewWebRTCPermissionIndicator()
	if indicator != nil {
		if w := indicator.Widget(); w != nil {
			a.mainWindow.AddOverlay(w)
			a.webrtcIndicator = indicator
		}
	}

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

	// Create mode indicator toaster for modal mode notifications.
	a.modeToaster = component.NewToaster(a.widgetFactory)
	modeToasterWidget := a.modeToaster.Widget()
	if modeToasterWidget == nil {
		return
	}
	modeGtkWidget := modeToasterWidget.GtkWidget()
	if modeGtkWidget == nil {
		return
	}
	a.mainWindow.AddOverlay(modeGtkWidget)
}

func (a *App) installCrashReportNotifier(ctx context.Context) {
	if a.deps == nil {
		return
	}

	a.deps.OnCrashReportsDetected = func(paths []string) {
		a.showCrashReportToast(ctx, paths)
	}
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

func (a *App) initAccentPicker(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.widgetFactory == nil || a.mainWindow == nil {
		log.Debug().Msg("widget factory or main window not available, skipping accent picker")
		return
	}

	// Create accent picker component
	a.accentPicker = component.NewAccentPicker(a.widgetFactory)
	if a.accentPicker == nil {
		log.Warn().Msg("failed to create accent picker")
		return
	}

	// Add to main window overlay
	pickerWidget := a.accentPicker.Widget()
	if pickerWidget != nil {
		gtkWidget := pickerWidget.GtkWidget()
		if gtkWidget != nil {
			a.mainWindow.AddOverlay(gtkWidget)
		}
	}

	// Create focus provider (tracks which input has focus)
	a.accentFocusProvider = textinput.NewFocusProvider()

	// Create the use case with glib.IdleAdd for thread-safe GTK calls
	a.insertAccentUC = usecase.NewInsertAccentUseCase(
		a.accentFocusProvider,
		a.accentPicker,
		func(fn func()) {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				fn()
				return false // Don't repeat
			})
			glib.IdleAdd(&cb, 0)
		},
	)

	log.Debug().Msg("accent picker initialized")
}

func (a *App) initDownloadHandler(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.deps == nil || a.deps.WebContext == nil {
		log.Debug().Msg("WebContext not available, skipping download handler")
		return
	}

	// Determine download path from config, fallback to XDG.
	downloadPath := ""
	if a.deps.Config != nil {
		downloadPath = a.deps.Config.Downloads.Path
	}
	if downloadPath == "" && a.deps.XDG != nil {
		var err error
		downloadPath, err = a.deps.XDG.DownloadDir()
		if err != nil {
			log.Warn().Err(err).Msg("failed to get XDG download dir")
		}
	}
	if downloadPath == "" {
		// Final fallback to ~/Downloads, with /tmp as last resort.
		home, err := os.UserHomeDir()
		if err != nil {
			log.Warn().Err(err).Msg("failed to get home dir, using /tmp for downloads")
			downloadPath = "/tmp"
		} else {
			downloadPath = filepath.Join(home, "Downloads")
		}
	}

	// Create download event adapter to show toasts.
	eventAdapter := &downloadEventAdapter{app: a}

	// Create use case for preparing download destinations with file deduplication.
	prepareDownloadUC := usecase.NewPrepareDownloadUseCase(filesystem.New())

	// Create and wire the download handler.
	handler := webkit.NewDownloadHandler(downloadPath, eventAdapter, prepareDownloadUC)
	a.deps.WebContext.SetDownloadHandler(ctx, handler)

	log.Info().Str("path", downloadPath).Msg("download handler initialized")
}

// downloadEventAdapter implements port.DownloadEventHandler and shows toasts.
type downloadEventAdapter struct {
	app *App
}

func (d *downloadEventAdapter) OnDownloadEvent(ctx context.Context, event port.DownloadEvent) {
	// Must schedule on GTK main thread.
	cb := glib.SourceFunc(func(_ uintptr) bool {
		if d.app.appToaster == nil {
			return false
		}

		switch event.Type {
		case port.DownloadEventStarted:
			d.app.appToaster.Show(ctx, "Download started: "+event.Filename, component.ToastInfo)
		case port.DownloadEventFinished:
			d.app.appToaster.Show(ctx, "Download complete: "+event.Filename, component.ToastSuccess)
		case port.DownloadEventFailed:
			d.app.appToaster.Show(ctx, "Download failed: "+event.Filename, component.ToastError)
		}
		return false
	})
	glib.IdleAdd(&cb, 0)
}

func (a *App) initKeyboardHandler(ctx context.Context) {
	if a.mainWindow == nil || a.deps == nil || a.deps.Config == nil {
		return
	}

	// Create keyboard handler and wire to dispatcher.
	a.keyboardHandler = input.NewKeyboardHandler(ctx, a.deps.Config)
	a.keyboardHandler.SetOnAction(func(ctx context.Context, action input.Action) error {
		if action == input.ActionClosePane {
			if a.closeAndReleaseActiveFloatingPane(ctx) {
				return nil
			}
		}
		return a.kbDispatcher.Dispatch(ctx, action)
	})
	a.keyboardHandler.SetOnModeChange(func(from, to input.Mode) {
		a.handleModeChange(ctx, from, to)
	})
	a.keyboardHandler.SetShouldBypassInput(func(modifiers input.Modifier) bool {
		// Bypass keyboard handler when modals are visible
		if a.sessionManager != nil && a.sessionManager.IsVisible() {
			return true
		}
		if a.tabPicker != nil && a.tabPicker.IsVisible() {
			return true
		}
		wsView := a.activeWorkspaceView()
		if wsView == nil {
			return false
		}
		if wsView.IsOmniboxVisible() {
			// Let Alt-modified keys through for pane navigation
			return modifiers&input.ModAlt == 0
		}
		return false
	})
	// Wire accent handler for dead keys support
	if a.insertAccentUC != nil {
		a.keyboardHandler.SetAccentHandler(a.insertAccentUC)
	}
	a.keyboardHandler.AttachTo(a.mainWindow.Window())

	// Create global shortcut handler for Alt+1-9 tab switching.
	// This uses GtkShortcutController with GTK_SHORTCUT_SCOPE_GLOBAL
	// to intercept shortcuts even when WebView has focus.
	a.globalShortcutHandler = input.NewGlobalShortcutHandler(
		ctx,
		a.mainWindow.Window(),
		a.deps.Config,
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

	// Convert config shortcuts to use case type
	shortcuts := make(map[string]usecase.SearchShortcut, len(a.deps.Config.SearchShortcuts))
	for key, shortcut := range a.deps.Config.SearchShortcuts {
		shortcuts[key] = usecase.SearchShortcut{
			URL:         shortcut.URL,
			Description: shortcut.Description,
		}
	}
	shortcutsUC := usecase.NewSearchShortcutsUseCase(shortcuts)

	// Create autocomplete use case for inline suggestions
	autocompleteUC := usecase.NewAutocompleteUseCase(a.deps.HistoryUC, a.deps.FavoritesUC, shortcutsUC)

	// Store omnibox config (omnibox is created per-pane via WorkspaceView).
	a.omniboxCfg = component.OmniboxConfig{
		HistoryUC:       a.deps.HistoryUC,
		FavoritesUC:     a.deps.FavoritesUC,
		FaviconAdapter:  a.faviconAdapter,
		CopyURLUC:       a.deps.CopyURLUC,
		ShortcutsUC:     shortcutsUC,
		AutocompleteUC:  autocompleteUC,
		DefaultSearch:   a.deps.Config.DefaultSearchEngine,
		InitialBehavior: a.deps.Config.Omnibox.InitialBehavior,
		UIScale:         a.deps.Config.DefaultUIScale,
		OnNavigate: func(url string) {
			if err := a.navigateFromOmnibox(ctx, url); err != nil {
				log.Error().Err(err).Str("url", url).Msg("navigation failed")
			}
		},
		OnFocusIn: func(entry *gtk.SearchEntry) {
			// Set omnibox entry as the focused input for accent picker
			if a.accentFocusProvider != nil && entry != nil {
				a.accentFocusProvider.SetFocusedInput(textinput.NewGTKEntryTarget(entry))
			}
		},
		OnFocusOut: func() {
			// When omnibox loses focus, set WebView as the focused input
			if a.accentFocusProvider != nil {
				a.accentFocusProvider.SetFocusedInput(a.getActiveWebViewTarget())
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
		OnFocusIn: func(entry *gtk.SearchEntry) {
			// Set find bar entry as the focused input for accent picker
			if a.accentFocusProvider != nil && entry != nil {
				a.accentFocusProvider.SetFocusedInput(textinput.NewGTKEntryTarget(entry))
			}
		},
		OnFocusOut: func() {
			// When find bar loses focus, set WebView as the focused input
			if a.accentFocusProvider != nil {
				a.accentFocusProvider.SetFocusedInput(a.getActiveWebViewTarget())
			}
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

func (a *App) attachTabPickerToActivePane() {
	if a.tabPicker == nil || a.widgetFactory == nil {
		return
	}
	wsView := a.activeWorkspaceView()
	if wsView == nil {
		return
	}

	activeTab := a.tabs.ActiveTab()
	if activeTab == nil || activeTab.Workspace == nil {
		return
	}
	activePaneID := activeTab.Workspace.ActivePaneID
	if activePaneID == "" {
		return
	}
	pv := wsView.GetPaneView(activePaneID)
	if pv == nil {
		return
	}

	if a.tabPickerWidget == nil {
		a.tabPickerWidget = a.tabPicker.WidgetAsLayout(a.widgetFactory)
		if a.tabPickerWidget == nil {
			return
		}
	}

	// If currently attached to a different pane overlay, detach.
	if a.tabPickerPaneID != "" && a.tabPickerPaneID != activePaneID {
		for _, view := range a.workspaceViews {
			if view == nil {
				continue
			}
			if oldPV := view.GetPaneView(a.tabPickerPaneID); oldPV != nil {
				if parent := a.tabPickerWidget.GetParent(); parent == oldPV.Overlay() {
					oldPV.RemoveOverlayWidget(a.tabPickerWidget)
				} else if parent != nil {
					a.tabPickerWidget.Unparent()
				}
				break
			}
		}
	}

	// Ensure the widget can be reparented.
	if parent := a.tabPickerWidget.GetParent(); parent != nil {
		a.tabPickerWidget.Unparent()
	}

	a.tabPicker.SetParentOverlay(pv.Overlay())
	pv.AddOverlayWidget(a.tabPickerWidget)
	a.tabPickerPaneID = activePaneID
}

func (a *App) HandleMovePaneToTab(ctx context.Context) error {
	if a.movePaneToTabUC == nil {
		return nil
	}
	if a.tabPicker == nil {
		return nil
	}

	sourceTab := a.tabs.ActiveTab()
	if sourceTab == nil || sourceTab.Workspace == nil {
		return nil
	}

	// If there is only 1 tab, auto-create a new tab and move there.
	if a.tabs.Count() <= 1 {
		return a.MoveActivePaneToTab(ctx, "")
	}

	items := make([]component.TabPickerItem, 0, a.tabs.Count())
	for _, tab := range a.tabs.Tabs {
		if tab == nil {
			continue
		}
		if tab.ID == sourceTab.ID {
			continue
		}
		items = append(items, component.TabPickerItem{
			TabID: tab.ID,
			Title: tab.Title(),
			IsNew: false,
			Index: tab.Position,
		})
	}
	items = append(items, component.TabPickerItem{IsNew: true, Index: -1})

	a.attachTabPickerToActivePane()
	a.tabPicker.Show(ctx, items)
	return nil
}

func (a *App) HandleMovePaneToNextTab(ctx context.Context) error {
	if a.movePaneToTabUC == nil {
		return nil
	}
	active := a.tabs.ActiveTab()
	if active == nil {
		return nil
	}

	nextPos := active.Position + 1
	if nextPos >= a.tabs.Count() {
		// Create a new tab on the right.
		return a.MoveActivePaneToTab(ctx, "")
	}
	next := a.tabs.TabAt(nextPos)
	if next == nil {
		return nil
	}
	return a.MoveActivePaneToTab(ctx, next.ID)
}

func (a *App) MoveActivePaneToTab(ctx context.Context, targetTabID entity.TabID) error {
	in, sourceTab := a.buildMovePaneToTabInput(targetTabID)
	if in == nil {
		return nil
	}

	out, err := a.movePaneToTabUC.Execute(*in)
	if err != nil {
		return err
	}
	if out == nil || out.TargetTab == nil {
		return nil
	}

	a.applyMovePaneToTabUI(ctx, out, sourceTab)
	a.switchToTargetTabIfConfigured(ctx, out)
	a.updateTabBarVisibility(ctx)
	a.MarkDirty()
	return nil
}

func (a *App) buildMovePaneToTabInput(targetTabID entity.TabID) (*usecase.MovePaneToTabInput, *entity.Tab) {
	if a.movePaneToTabUC == nil {
		return nil, nil
	}
	activeTab := a.tabs.ActiveTab()
	if activeTab == nil || activeTab.Workspace == nil {
		return nil, nil
	}
	sourcePaneID := activeTab.Workspace.ActivePaneID
	if sourcePaneID == "" {
		return nil, nil
	}

	in := &usecase.MovePaneToTabInput{
		TabList:      a.tabs,
		SourceTabID:  activeTab.ID,
		SourcePaneID: sourcePaneID,
		TargetTabID:  targetTabID,
	}
	return in, activeTab
}

func (a *App) applyMovePaneToTabUI(ctx context.Context, out *usecase.MovePaneToTabOutput, sourceTab *entity.Tab) {
	if out == nil || out.TargetTab == nil {
		return
	}
	sourceTabID := entity.TabID("")
	if sourceTab != nil {
		sourceTabID = sourceTab.ID
	}

	if out.SourceTabClosed {
		a.removeSourceTabUI(sourceTabID)
	}
	if out.NewTabCreated {
		a.ensureTargetTabUI(ctx, out.TargetTab)
	}

	if !out.SourceTabClosed {
		a.rebuildAndAttachWorkspace(ctx, sourceTabID, sourceTab)
	}
	a.rebuildAndAttachWorkspace(ctx, out.TargetTab.ID, out.TargetTab)
}

func (a *App) removeSourceTabUI(sourceTabID entity.TabID) {
	a.releaseFloatingSessionsForTab(context.Background(), sourceTabID)
	delete(a.workspaceViews, sourceTabID)
	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().RemoveTab(sourceTabID)
	}
}

func (a *App) ensureTargetTabUI(ctx context.Context, tab *entity.Tab) {
	if tab == nil {
		return
	}
	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().AddTab(tab)
	}
	a.createWorkspaceViewWithoutAttach(ctx, tab)
}

func (a *App) rebuildAndAttachWorkspace(ctx context.Context, tabID entity.TabID, tab *entity.Tab) {
	wsView := a.workspaceViews[tabID]
	if wsView == nil {
		return
	}
	_ = wsView.Rebuild(ctx)

	if tab == nil || tab.Workspace == nil || a.contentCoord == nil {
		return
	}
	a.contentCoord.AttachToWorkspace(ctx, tab.Workspace, wsView)
	if a.wsCoord != nil {
		a.wsCoord.SetupStackedPaneCallbacks(ctx, tab.Workspace, wsView)
	}
}

func (a *App) switchToTargetTabIfConfigured(ctx context.Context, out *usecase.MovePaneToTabOutput) {
	if out == nil || out.TargetTab == nil {
		return
	}

	switchToTarget := config.Get().Workspace.SwitchToTabOnMove
	if out.SourceTabClosed {
		switchToTarget = true
	}
	if !switchToTarget {
		return
	}

	a.tabs.SetActive(out.TargetTab.ID)
	if a.mainWindow != nil && a.mainWindow.TabBar() != nil {
		a.mainWindow.TabBar().SetActive(out.TargetTab.ID)
	}
	a.switchWorkspaceView(ctx, out.TargetTab.ID)
}

func (a *App) updateTabBarVisibility(ctx context.Context) {
	if a.tabCoord != nil {
		a.tabCoord.UpdateBarVisibility(ctx)
	}
}

func (a *App) initTabPicker(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.deps == nil || a.deps.Config == nil {
		log.Debug().Msg("deps/config not available, skipping tab picker")
		return
	}

	a.tabPicker = component.NewTabPicker(ctx, component.TabPickerConfig{
		UIScale: a.deps.Config.DefaultUIScale,
		OnClose: func() {
			log.Debug().Msg("tab picker closed")
		},
		OnSelect: func(item component.TabPickerItem) {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				var targetID entity.TabID
				if !item.IsNew {
					targetID = item.TabID
				}
				if err := a.MoveActivePaneToTab(ctx, targetID); err != nil {
					log.Warn().Err(err).Msg("move pane to tab failed")
				}
				return false
			})
			glib.IdleAdd(&cb, 0)
		},
	})

	if a.tabPicker == nil {
		log.Warn().Msg("failed to create tab picker")
		return
	}

	log.Debug().Msg("tab picker initialized")
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

	// Set up callback for main.go to notify when session is persisted
	a.deps.OnSessionPersisted = func() {
		a.snapshotService.SetReady()
	}

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

	// Show the window
	if a.mainWindow != nil {
		a.mainWindow.Show()
	}
	log.Info().Msg("main window displayed")

	if a.deps != nil && len(a.deps.StartupCrashReports) > 0 {
		a.showCrashReportToast(ctx, a.deps.StartupCrashReports)
	}

	// Defer non-critical initialization until after first navigation starts.
	// This keeps pool prewarm, config watcher, and filter loading from
	// competing with the initial page load.
	a.runAfterFirstLoadStarted(func() {
		a.prewarmWebViewPoolAsync(ctx)
		a.initConfigWatcher(ctx)
		a.initFilteringAsync(ctx)
		a.checkConfigMigration(ctx)
	})
}

func (a *App) showCrashReportToast(ctx context.Context, paths []string) {
	if len(paths) == 0 {
		return
	}
	log := logging.FromContext(ctx)
	log.Warn().Int("count", len(paths)).Msg("unexpected-close reports available")

	cb := glib.SourceFunc(func(_ uintptr) bool {
		if a.appToaster != nil {
			msg := fmt.Sprintf("Detected %d unexpected close report(s). Run: dumber crashes issue latest", len(paths))
			a.appToaster.Show(ctx, msg, component.ToastWarning,
				component.WithDuration(crashReportToastDurationMs),
				component.WithPosition(component.ToastPositionBottomRight),
			)
		}
		return false
	})
	glib.IdleAdd(&cb, 0)
}

// runAfterFirstLoadStarted schedules work to run after the first navigation starts.
// This keeps the GTK main loop free to process the initial load_uri() quickly,
// reducing the gap between webview_attached and load_started.
func (a *App) runAfterFirstLoadStarted(fn func()) {
	a.deferredInitFn = fn
}

// triggerDeferredInit is called from ContentCoordinator on first load_started.
// It runs deferred initialization at LOW priority so navigation continues unblocked.
func (a *App) triggerDeferredInit(ctx context.Context) {
	a.deferredInitOnce.Do(func() {
		if a.deferredInitFn == nil {
			return
		}
		log := logging.FromContext(ctx)
		log.Debug().Msg("triggering deferred initialization")

		fn := a.deferredInitFn
		cb := glib.SourceFunc(func(_ uintptr) bool {
			fn()
			return false
		})
		// Use LOW priority so navigation processing continues first
		glib.IdleAddFull(glib.PRIORITY_LOW, &cb, 0, nil)
	})
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
	tabIDSet := make(map[entity.TabID]struct{}, len(a.floatingSessions))
	for key := range a.floatingSessions {
		tabIDSet[key.tabID] = struct{}{}
	}
	tabIDs := make([]entity.TabID, 0, len(tabIDSet))
	for tabID := range tabIDSet {
		tabIDs = append(tabIDs, tabID)
	}
	for _, tabID := range tabIDs {
		a.releaseFloatingSessionsForTab(ctx, tabID)
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
		a.deps.PermissionUC,
	)

	// Set idle inhibitor for fullscreen video playback
	if a.deps.IdleInhibitor != nil {
		a.contentCoord.SetIdleInhibitor(a.deps.IdleInhibitor)
	}

	// Wire deferred init trigger - runs after first navigation starts
	a.contentCoord.SetOnFirstLoadStarted(func() {
		a.triggerDeferredInit(ctx)
	})

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

	// Set fullscreen callback to hide/show tab bar (after tabCoord is initialized)
	a.contentCoord.SetOnFullscreenChanged(func(entering bool) {
		if a.mainWindow == nil || a.mainWindow.TabBar() == nil {
			return
		}
		if entering {
			// Hide tab bar when entering fullscreen video
			a.mainWindow.TabBar().SetVisible(false)
		} else {
			// Restore tab bar visibility based on normal logic
			a.tabCoord.UpdateBarVisibility(ctx)
		}
	})

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

	// Move pane use case (cross-tab)
	a.movePaneToTabUC = usecase.NewMovePaneToTabUseCase(a.generateID)

	// 4. Navigation Coordinator
	a.navCoord = coordinator.NewNavigationCoordinator(
		ctx,
		a.deps.NavigateUC,
		a.contentCoord,
	)
	a.wsCoord.SetOnPaneClosed(func(paneID entity.PaneID) {
		a.navCoord.ClearPaneHistory(paneID)
	})

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
		a.updateFloatingSessionURI(paneID, url)
		// Mark dirty so snapshot captures the new URI
		a.MarkDirty()
	})

	// Hide loading skeleton once the WebView paints
	a.contentCoord.SetOnWebViewShown(func(paneID entity.PaneID) {
		// The WebView can be shown while its pane is in a background tab, so scan
		// all WorkspaceViews rather than only the active one.
		for _, wsView := range a.workspaceViews {
			if wsView == nil {
				continue
			}
			if pv := wsView.GetPaneView(paneID); pv != nil {
				pv.HideLoadingSkeleton()
			}
		}
		if a.deps != nil && a.deps.OnFirstWebViewShown != nil {
			a.firstWebViewShownOnce.Do(func() {
				a.deps.OnFirstWebViewShown(ctx)
			})
		}

		// Set the WebView as the focused input for accent picker (if this is the active pane)
		ws := a.activeWorkspace()
		if ws != nil && ws.ActivePaneID == paneID && a.accentFocusProvider != nil {
			a.accentFocusProvider.SetFocusedInput(a.getActiveWebViewTarget())
		}
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
	a.wireKeyboardActions()

	log.Debug().Msg("coordinators initialized")
}

func (a *App) wireKeyboardActions() {
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
	a.kbDispatcher.SetOnMovePaneToTab(func(ctx context.Context) error {
		return a.HandleMovePaneToTab(ctx)
	})
	a.kbDispatcher.SetOnMovePaneToNextTab(func(ctx context.Context) error {
		return a.HandleMovePaneToNextTab(ctx)
	})
	a.kbDispatcher.SetOnToggleFloatingPane(func(ctx context.Context) error {
		return a.ToggleFloatingPane(ctx)
	})
	a.kbDispatcher.SetOnOpenFloatingTarget(func(ctx context.Context, target input.FloatingProfileTarget) error {
		return a.OpenFloatingPaneProfileURL(ctx, target.SessionID, target.URL)
	})

	// Wire gesture handler to dispatcher (for mouse button 8/9 navigation)
	a.contentCoord.SetGestureActionHandler(func(ctx context.Context, action input.Action) error {
		return a.kbDispatcher.Dispatch(ctx, action)
	})
}

func (a *App) wireWebRTCPermissionIndicator() {
	if a.webrtcIndicator == nil || a.contentCoord == nil {
		return
	}
	ctx := context.Background()
	if a.deps != nil && a.deps.Ctx != nil {
		ctx = a.deps.Ctx
	}
	log := logging.FromContext(ctx)

	a.contentCoord.SetOnPermissionActivity(func(origin string, permTypes []entity.PermissionType, state coordinator.PermissionActivityState) {
		a.webrtcIndicator.SetOrigin(origin)

		switch state {
		case coordinator.PermissionActivityRequesting:
			a.webrtcIndicator.MarkRequesting(permTypes)
		case coordinator.PermissionActivityAllowed:
			a.webrtcIndicator.MarkAllowed(permTypes)
		case coordinator.PermissionActivityBlocked:
			a.webrtcIndicator.MarkBlocked(permTypes)
		}

		for _, permType := range permTypes {
			a.syncWebRTCPermissionLockState(ctx, origin, permType)
		}
	})

	a.webrtcIndicator.SetOnToggleLock(func(origin string, permType entity.PermissionType, state string, hasStored bool) {
		if a.deps == nil || a.deps.PermissionUC == nil || origin == "" {
			return
		}

		if hasStored {
			// Already locked: reset to prompt (will re-ask on next request).
			if err := a.deps.PermissionUC.ResetManualPermissionDecision(ctx, origin, permType); err != nil {
				log.Warn().Err(err).Str("origin", origin).Str("type", string(permType)).Msg("failed to reset manual permission decision")
				return
			}
		} else {
			// Not locked: flip the state.
			// Allowed/requesting → deny future requests.
			// Blocked → allow future requests.
			decision := entity.PermissionDenied
			if state == string(coordinator.PermissionActivityBlocked) {
				decision = entity.PermissionGranted
			}
			if err := a.deps.PermissionUC.SetManualPermissionDecision(ctx, origin, permType, decision); err != nil {
				log.Warn().Err(err).Str("origin", origin).Str("type", string(permType)).Msg("failed to set manual permission decision")
				return
			}
		}

		a.syncWebRTCPermissionLockState(ctx, origin, permType)
	})

	// Reset indicator when the active pane navigates away from the current origin.
	a.contentCoord.SetOnActiveNavigationCommitted(func(uri string) {
		newOrigin, err := urlutil.ExtractOrigin(uri)
		if err != nil {
			// Internal pages (dumb://, about:) — clear the indicator.
			a.webrtcIndicator.Reset()
			return
		}

		currentOrigin := a.webrtcIndicator.Origin()
		if currentOrigin != "" && currentOrigin != newOrigin {
			a.webrtcIndicator.Reset()
		}
	})
}

func (a *App) syncWebRTCPermissionLockState(ctx context.Context, origin string, permType entity.PermissionType) {
	if a.webrtcIndicator == nil || a.deps == nil || a.deps.PermissionUC == nil || origin == "" {
		return
	}

	record, err := a.deps.PermissionUC.GetManualPermissionDecision(ctx, origin, permType)
	if err != nil {
		logging.FromContext(ctx).Warn().
			Err(err).
			Str("origin", origin).
			Str("type", string(permType)).
			Msg("failed to read manual permission decision")
		return
	}

	if record == nil {
		a.webrtcIndicator.SetStoredDecision(permType, entity.PermissionPrompt, false)
		return
	}

	a.webrtcIndicator.SetStoredDecision(permType, record.Decision, true)
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

	// Show/hide mode indicator toaster based on config.
	a.updateModeIndicatorToaster(ctx, to)
}

// updateModeIndicatorToaster shows or hides the mode indicator toaster based on mode and config.
func (a *App) updateModeIndicatorToaster(ctx context.Context, mode input.Mode) {
	if a.modeToaster == nil {
		return
	}

	// Check if mode indicator toaster is enabled in config.
	if a.deps == nil || a.deps.Config == nil || !a.deps.Config.Workspace.Styling.ModeIndicatorToasterEnabled {
		a.modeToaster.Hide()
		return
	}

	if mode == input.ModeNormal {
		a.modeToaster.Hide()
		return
	}

	// Show persistent toaster at bottom-left with mode display name.
	// Mode class is applied atomically with Show() to avoid visual flicker.
	modeClass := getModeToastClass(mode)
	a.modeToaster.Show(ctx, mode.DisplayName(), component.ToastInfo,
		component.WithDuration(0), // Persistent until mode exits.
		component.WithPosition(component.ToastPositionBottomLeft),
		component.WithModeClass(modeClass),
	)
}

// getModeToastClass returns the CSS class for the given mode's toast styling.
func getModeToastClass(mode input.Mode) string {
	switch mode {
	case input.ModePane:
		return "toast-pane-mode"
	case input.ModeTab:
		return "toast-tab-mode"
	case input.ModeSession:
		return "toast-session-mode"
	case input.ModeResize:
		return "toast-resize-mode"
	default:
		return ""
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
	a.installFloatingOverlayPositioning(tab.ID, wsView.WorkspaceOverlayWidget())

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
	// Set auto-open omnibox on new pane
	wsView.SetAutoOpenOnNewPane(config.Get().Omnibox.AutoOpenOnNewPane)

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
	for key, session := range a.floatingSessions {
		if key.tabID != tab.ID || session == nil || session.pane == nil {
			continue
		}
		session.pane.SetParentOverlay(wsView.WorkspaceOverlayWidget())
		if session.widget != nil {
			wsView.AddWorkspaceOverlayWidget(session.widget)
			configureFloatingOverlayMeasurement(wsView.WorkspaceOverlayWidget(), session.widget)
			setFloatingWidgetShown(session.widget, session.pane.IsVisible())
		}
	}
	a.syncFloatingFocus()

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

// getActiveWebViewTarget returns a TextInputTarget for the active pane's WebView.
// Used by the accent picker to insert accented characters into web content.
func (a *App) getActiveWebViewTarget() port.TextInputTarget {
	if a.contentCoord == nil || a.deps == nil || a.deps.Clipboard == nil {
		return nil
	}

	wv := a.contentCoord.ActiveWebView(context.Background())
	if wv == nil {
		return nil
	}

	// Get the underlying webkit.WebView for the text input target
	return textinput.NewWebViewTarget(wv.Widget(), a.deps.Clipboard)
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
	a.syncFloatingFocus()

	log.Debug().Str("tab_id", string(tabID)).Msg("workspace view switched")
}

func normalizeFloatingSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return floatingSessionIDDefault
	}
	return sessionID
}

func floatingPaneIDForSession(tabID entity.TabID, sessionID string) entity.PaneID {
	return entity.PaneID(floatingPaneIDPrefix + string(tabID) + ":" + normalizeFloatingSessionID(sessionID))
}

func floatingSessionMapKey(tabID entity.TabID, sessionID string) floatingSessionKey {
	return floatingSessionKey{tabID: tabID, sessionID: normalizeFloatingSessionID(sessionID)}
}

func decorateFloatingOverlay(overlay layout.OverlayWidget) {
	if overlay == nil {
		return
	}

	overlay.SetHexpand(false)
	overlay.SetVexpand(false)
	overlay.AddCssClass("floating-pane-container")
	overlay.AddCssClass("pane-border")
	overlay.AddCssClass("pane-active")
}

// showFloatingWidget makes a floating pane widget interactive and visible.
// Visibility is controlled by CSS class to animate opacity while keeping the
// widget mapped so WebKit can redraw without a hard pop.
func showFloatingWidget(w layout.Widget) {
	if w == nil {
		return
	}
	w.AddCssClass(floatingPaneVisibleClass)
	w.SetCanTarget(true)
	w.SetCanFocus(true)
}

// hideFloatingWidget makes a floating pane widget invisible and non-interactive.
// Removes the visible CSS class instead of SetVisible(false) so GTK keeps the
// widget mapped and WebKit retains its rendered surface in GPU memory.
func hideFloatingWidget(w layout.Widget) {
	if w == nil {
		return
	}
	w.RemoveCssClass(floatingPaneVisibleClass)
	w.SetCanTarget(false)
	w.SetCanFocus(false)
}

// setFloatingWidgetShown applies show or hide based on the visible flag.
func setFloatingWidgetShown(w layout.Widget, visible bool) {
	if visible {
		showFloatingWidget(w)
	} else {
		hideFloatingWidget(w)
	}
}

func configureFloatingOverlayMeasurement(workspaceOverlay layout.OverlayWidget, floatingOverlay layout.Widget) {
	if workspaceOverlay == nil || floatingOverlay == nil {
		return
	}

	workspaceOverlay.SetMeasureOverlay(floatingOverlay, false)
	workspaceOverlay.SetClipOverlay(floatingOverlay, false)
}

func floatingAllocationRect(overlayWidth, overlayHeight, desiredWidth, desiredHeight int) (x, y, width, height int, ok bool) {
	if overlayWidth <= 0 || overlayHeight <= 0 || desiredWidth <= 0 || desiredHeight <= 0 {
		return 0, 0, 0, 0, false
	}

	width = desiredWidth
	height = desiredHeight
	if width > overlayWidth {
		width = overlayWidth
	}
	if height > overlayHeight {
		height = overlayHeight
	}

	x = (overlayWidth - width) / 2
	y = (overlayHeight - height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return x, y, width, height, true
}

type gtkAllocationLayout [4]int32

const gtkAllocationLayoutSize = 16

var (
	_ [gtkAllocationLayoutSize - int(unsafe.Sizeof(gtkAllocationLayout{}))]struct{}
	_ [int(unsafe.Sizeof(gtkAllocationLayout{})) - gtkAllocationLayoutSize]struct{}
)

// writeOverlayAllocation writes GTK-owned GtkAllocation fields in place.
// Assumes GTK4 C layout for GdkRectangle/GtkAllocation: four contiguous
// 32-bit signed integers {x, y, width, height} in native endianness.
// If GTK struct layout changes, revisit offsets and writes below.
//
//nolint:gosec // Intentional unsafe pointer math against GTK-owned allocation memory.
func writeOverlayAllocation(allocationPtr *uintptr, x, y, width, height int) bool {
	if allocationPtr == nil {
		return false
	}

	const (
		gtkAllocationOffsetX      = uintptr(0)
		gtkAllocationOffsetY      = uintptr(4)
		gtkAllocationOffsetWidth  = uintptr(8)
		gtkAllocationOffsetHeight = uintptr(12)
	)

	// GtkAllocation/GdkRectangle is four 32-bit signed integers in C.
	base := unsafe.Pointer(allocationPtr)
	*(*int32)(unsafe.Pointer(uintptr(base) + gtkAllocationOffsetX)) = int32(x)
	*(*int32)(unsafe.Pointer(uintptr(base) + gtkAllocationOffsetY)) = int32(y)
	*(*int32)(unsafe.Pointer(uintptr(base) + gtkAllocationOffsetWidth)) = int32(width)
	*(*int32)(unsafe.Pointer(uintptr(base) + gtkAllocationOffsetHeight)) = int32(height)
	return true
}

func (a *App) floatingAllocationForWidget(
	tabID entity.TabID,
	widgetPtr uintptr,
	overlayWidth, overlayHeight int,
) (x, y, width, height int, ok bool) {
	for key, session := range a.floatingSessions {
		if key.tabID != tabID || session == nil || session.widget == nil {
			continue
		}

		gtkWidget := session.widget.GtkWidget()
		if gtkWidget == nil || gtkWidget.GoPointer() != widgetPtr {
			continue
		}

		desiredWidth := session.appliedWidth
		desiredHeight := session.appliedHeight
		if (desiredWidth <= 0 || desiredHeight <= 0) && session.pane != nil {
			session.pane.Resize()
			desiredWidth, desiredHeight = session.pane.Dimensions()
		}

		return floatingAllocationRect(overlayWidth, overlayHeight, desiredWidth, desiredHeight)
	}

	return 0, 0, 0, 0, false
}

func (a *App) installFloatingOverlayPositioning(tabID entity.TabID, workspaceOverlay layout.OverlayWidget) {
	if workspaceOverlay == nil {
		return
	}

	workspaceWidget := workspaceOverlay.GtkWidget()
	if workspaceWidget == nil {
		return
	}

	gtkOverlay := gtk.OverlayNewFromInternalPtr(workspaceWidget.GoPointer())
	if gtkOverlay == nil {
		return
	}

	cb := func(_ gtk.Overlay, widgetPtr uintptr, allocationPtr *uintptr) bool {
		overlayWidth := workspaceOverlay.GetAllocatedWidth()
		overlayHeight := workspaceOverlay.GetAllocatedHeight()
		x, y, width, height, ok := a.floatingAllocationForWidget(tabID, widgetPtr, overlayWidth, overlayHeight)
		if !ok {
			return false
		}
		return writeOverlayAllocation(allocationPtr, x, y, width, height)
	}

	gtkOverlay.ConnectGetChildPosition(&cb)
}

func (a *App) currentFloatingConfig() config.FloatingPaneConfig {
	if a.deps != nil && a.deps.Config != nil {
		return a.deps.Config.Workspace.FloatingPane
	}
	return config.Get().Workspace.FloatingPane
}

func (a *App) ensureFloatingSession(
	ctx context.Context,
	tabID entity.TabID,
	sessionID string,
	wsView *component.WorkspaceView,
) (*floatingWorkspaceSession, error) {
	key := floatingSessionMapKey(tabID, sessionID)
	if session, ok := a.floatingSessions[key]; ok && session != nil {
		if wsView == nil {
			return session, nil
		}
		session.pane.SetParentOverlay(wsView.WorkspaceOverlayWidget())
		if session.widget != nil {
			wsView.AddWorkspaceOverlayWidget(session.widget)
			configureFloatingOverlayMeasurement(wsView.WorkspaceOverlayWidget(), session.widget)
		}
		if session.paneView != nil {
			wsView.RegisterPaneView(session.paneID, session.paneView)
			if session.focusWidget == nil {
				session.focusWidget = session.paneView.WebViewWidget()
			}
		}
		return session, nil
	}

	if wsView == nil {
		return nil, fmt.Errorf("workspace view not found for tab %s", tabID)
	}
	if a.contentCoord == nil {
		return nil, fmt.Errorf("content coordinator not initialized")
	}
	if a.widgetFactory == nil {
		return nil, fmt.Errorf("widget factory not initialized")
	}

	paneID := floatingPaneIDForSession(tabID, sessionID)
	wv, err := a.contentCoord.EnsureWebView(ctx, paneID)
	if err != nil {
		return nil, fmt.Errorf("ensure floating webview: %w", err)
	}

	webViewWidget := a.contentCoord.WrapWidget(ctx, wv)
	if webViewWidget == nil {
		return nil, fmt.Errorf("wrap floating webview widget")
	}
	webViewWidget.SetHexpand(true)
	webViewWidget.SetVexpand(true)

	// Wrap the WebView in a PaneView so it gets progress bar, loading
	// skeleton, and link status overlays — the same widget hierarchy as
	// regular workspace panes. This allows the content coordinator's
	// existing callbacks (onLoadStarted, onProgressChanged, etc.) to find
	// and update the floating pane automatically.
	pv := component.NewPaneView(a.widgetFactory, paneID, webViewWidget)
	pv.SetActive(true)
	// Hide loading skeleton — floating panes have their own themed container.
	pv.HideLoadingSkeleton()
	wsView.RegisterPaneView(paneID, pv)

	// Use the PaneView's overlay directly as the floating overlay.
	// This avoids nesting two overlays which causes GTK4 allocation
	// issues (inner overlay expanding to fill the parent's allocation).
	pvOverlay := pv.Overlay()
	decorateFloatingOverlay(pvOverlay)
	pvOverlay.SetHalign(gtk.AlignCenterValue)
	pvOverlay.SetValign(gtk.AlignCenterValue)
	hideFloatingWidget(pvOverlay)
	wsView.AddWorkspaceOverlayWidget(pvOverlay)
	configureFloatingOverlayMeasurement(wsView.WorkspaceOverlayWidget(), pvOverlay)

	floatingCfg := a.currentFloatingConfig()
	floatingPane := component.NewFloatingPane(wsView.WorkspaceOverlayWidget(), component.FloatingPaneOptions{
		WidthPct:       floatingCfg.WidthPct,
		HeightPct:      floatingCfg.HeightPct,
		FallbackWidth:  floatingPaneFallbackWidth,
		FallbackHeight: floatingPaneFallbackHeight,
		OnNavigate: func(navCtx context.Context, url string) error {
			return wv.LoadURI(navCtx, url)
		},
	})

	session := &floatingWorkspaceSession{
		paneID:      paneID,
		pane:        floatingPane,
		paneView:    pv,
		webView:     wv,
		overlay:     pvOverlay,
		widget:      pvOverlay,
		focusWidget: webViewWidget,
	}
	a.floatingSessions[key] = session
	return session, nil
}

func (a *App) floatingSessionsForTab(tabID entity.TabID) map[floatingSessionKey]*floatingWorkspaceSession {
	sessions := make(map[floatingSessionKey]*floatingWorkspaceSession)
	for key, session := range a.floatingSessions {
		if key.tabID != tabID || session == nil {
			continue
		}
		sessions[key] = session
	}
	return sessions
}

func (a *App) releaseFloatingSessionsForTab(ctx context.Context, tabID entity.TabID) {
	sessions := a.floatingSessionsForTab(tabID)
	if len(sessions) == 0 {
		return
	}

	for key, session := range sessions {
		a.releaseFloatingSession(ctx, key, session)
	}
	a.syncFloatingFocus()
}

func (a *App) activeFloatingSessionEntry() (floatingSessionKey, *floatingWorkspaceSession, bool) {
	activeTab := a.tabs.ActiveTab()
	if activeTab == nil {
		return floatingSessionKey{}, nil, false
	}
	for key, session := range a.floatingSessions {
		if key.tabID != activeTab.ID || session == nil || session.pane == nil {
			continue
		}
		if session.pane.IsVisible() {
			return key, session, true
		}
	}
	return floatingSessionKey{tabID: activeTab.ID}, nil, false
}

func (a *App) activeFloatingSession() (*floatingWorkspaceSession, entity.TabID) {
	key, session, ok := a.activeFloatingSessionEntry()
	if !ok {
		return nil, key.tabID
	}
	return session, key.tabID
}

func (a *App) releaseFloatingSession(ctx context.Context, key floatingSessionKey, session *floatingWorkspaceSession) {
	if session == nil {
		return
	}

	a.stopFloatingResizeWatcher(session)
	if session.pane != nil {
		session.pane.Hide(ctx)
	}
	a.hideFloatingOmnibox(ctx, session)
	if wsView := a.workspaceViews[key.tabID]; wsView != nil {
		if session.widget != nil {
			wsView.RemoveWorkspaceOverlayWidget(session.widget)
		}
		if session.paneID != "" {
			wsView.UnregisterPaneView(session.paneID)
		}
	}
	if a.contentCoord != nil && session.paneID != "" {
		a.contentCoord.ReleaseWebView(ctx, session.paneID)
	}
	delete(a.floatingSessions, key)

	session.appliedWidth = 0
	session.appliedHeight = 0
	session.focusWidget = nil
	session.paneView = nil
	session.widget = nil
	session.overlay = nil
	session.webView = nil
	session.pane = nil
	// Session is fully released/zeroed; caller must drop this reference and not reuse it.
}

func (a *App) updateFloatingSessionURI(paneID entity.PaneID, url string) {
	if paneID == "" {
		return
	}

	for _, session := range a.floatingSessions {
		if session == nil || session.pane == nil {
			continue
		}
		if session.paneID != paneID {
			continue
		}
		session.pane.RecordLoadedURL(url)
		return
	}
}

func (a *App) syncFloatingFocus() {
	for _, wsView := range a.workspaceViews {
		if wsView != nil {
			wsView.SetHoverFocusLocked(false)
		}
	}

	if a.contentCoord == nil {
		return
	}

	activeTab := a.tabs.ActiveTab()
	if activeTab == nil {
		a.contentCoord.ClearActivePaneOverride()
		return
	}

	if session, _ := a.activeFloatingSession(); session != nil {
		if wsView := a.workspaceViews[activeTab.ID]; wsView != nil {
			wsView.SetHoverFocusLocked(true)
		}
		a.startFloatingResizeWatcher(session)
		a.contentCoord.SetActivePaneOverride(session.paneID)
		return
	}

	a.contentCoord.ClearActivePaneOverride()
}

func (a *App) showFloatingOmnibox(ctx context.Context, session *floatingWorkspaceSession) {
	if session == nil || session.overlay == nil || a.widgetFactory == nil {
		return
	}

	if session.omnibox == nil {
		cfg := a.omniboxCfg
		cfg.OnToast = func(toastCtx context.Context, message string, level component.ToastLevel) {
			if a.appToaster != nil {
				a.appToaster.Show(toastCtx, message, level)
			}
		}

		omnibox := component.NewOmnibox(ctx, cfg)
		if omnibox == nil {
			return
		}

		omnibox.SetParentOverlay(session.overlay)
		omniboxWidget := omnibox.WidgetAsLayout(a.widgetFactory)
		if omniboxWidget == nil {
			return
		}

		session.overlay.AddOverlay(omniboxWidget)
		session.overlay.SetClipOverlay(omniboxWidget, false)
		session.overlay.SetMeasureOverlay(omniboxWidget, false)

		session.omnibox = omnibox
		session.omniboxWidget = omniboxWidget
	}

	session.pane.SetOmniboxVisible(true)
	session.omnibox.Show(ctx, "")
}

func (a *App) hideFloatingOmnibox(ctx context.Context, session *floatingWorkspaceSession) {
	if session == nil || session.pane == nil {
		return
	}

	session.pane.SetOmniboxVisible(false)
	if session.omnibox == nil {
		return
	}

	session.omnibox.Hide(ctx)
	if session.overlay != nil && session.omniboxWidget != nil {
		if parent := session.omniboxWidget.GetParent(); parent == session.overlay {
			session.overlay.RemoveOverlay(session.omniboxWidget)
		} else if parent != nil {
			session.omniboxWidget.Unparent()
		}
	}
	session.omnibox = nil
	session.omniboxWidget = nil
}

func floatingFocusWidget(session *floatingWorkspaceSession) layout.Widget {
	if session == nil {
		return nil
	}
	if session.focusWidget != nil {
		return session.focusWidget
	}
	if session.paneView != nil {
		return session.paneView.WebViewWidget()
	}
	return nil
}

func focusFloatingSessionContent(session *floatingWorkspaceSession) {
	if session == nil || session.pane == nil {
		return
	}
	if !session.pane.IsVisible() || session.pane.IsOmniboxVisible() {
		return
	}
	w := floatingFocusWidget(session)
	if w == nil {
		return
	}
	w.GrabFocus()
}

func (a *App) resizeFloatingWidget(session *floatingWorkspaceSession) {
	if session == nil || session.pane == nil {
		return
	}
	session.pane.Resize()
	if session.widget == nil {
		return
	}
	width, height := session.pane.Dimensions()
	if session.appliedWidth == width && session.appliedHeight == height {
		return
	}
	session.widget.SetSizeRequest(width, height)
	session.appliedWidth = width
	session.appliedHeight = height
}

func (a *App) handleFloatingViewportTick(session *floatingWorkspaceSession) bool {
	if session == nil || session.pane == nil {
		return false
	}
	if !session.pane.IsVisible() {
		session.resizeWatcherActive = false
		session.resizeTickID = 0
		return false
	}

	a.resizeFloatingWidget(session)
	return true
}

func (a *App) floatingSessionByPaneID(paneID entity.PaneID) *floatingWorkspaceSession {
	if paneID == "" {
		return nil
	}
	for _, session := range a.floatingSessions {
		if session == nil || session.paneID != paneID {
			continue
		}
		return session
	}
	return nil
}

func (a *App) startFloatingResizeWatcher(session *floatingWorkspaceSession) {
	if session == nil || session.overlay == nil || session.resizeWatcherActive {
		return
	}

	overlayWidget := session.overlay.GtkWidget()
	if overlayWidget == nil {
		return
	}

	session.resizeWatcherActive = true
	paneID := session.paneID
	tickCallback := gtk.TickCallback(func(_ uintptr, _ uintptr, _ uintptr) bool {
		liveSession := a.floatingSessionByPaneID(paneID)
		if liveSession == nil {
			session.resizeWatcherActive = false
			session.resizeTickID = 0
			return false
		}

		keepRunning := a.handleFloatingViewportTick(liveSession)
		if !keepRunning {
			liveSession.resizeWatcherActive = false
			liveSession.resizeTickID = 0
		}
		return keepRunning
	})
	session.resizeTickID = overlayWidget.AddTickCallback(&tickCallback, 0, nil)
}

func (a *App) stopFloatingResizeWatcher(session *floatingWorkspaceSession) {
	if session == nil {
		return
	}

	if session.resizeTickID != 0 && session.overlay != nil {
		if overlayWidget := session.overlay.GtkWidget(); overlayWidget != nil {
			overlayWidget.RemoveTickCallback(session.resizeTickID)
		}
	}
	session.resizeTickID = 0
	session.resizeWatcherActive = false
}

func (a *App) hideFloatingSession(ctx context.Context, session *floatingWorkspaceSession) {
	if session == nil || session.pane == nil {
		return
	}

	a.stopFloatingResizeWatcher(session)
	session.pane.Hide(ctx)
	a.hideFloatingOmnibox(ctx, session)
	hideFloatingWidget(session.widget)
	// Force a fresh size request on next show so WebKit gets a new
	// allocation cycle and repaints content after rapid toggles.
	session.appliedWidth = 0
	session.appliedHeight = 0
}

func (a *App) hideVisibleFloatingSessions(ctx context.Context, tabID entity.TabID, except *floatingWorkspaceSession) {
	for key, session := range a.floatingSessions {
		if key.tabID != tabID || session == nil || session == except || session.pane == nil {
			continue
		}
		if !session.pane.IsVisible() {
			continue
		}
		a.hideFloatingSession(ctx, session)
	}
}

func (a *App) closeActiveFloatingPane(ctx context.Context) bool {
	session, _ := a.activeFloatingSession()
	if session == nil {
		return false
	}
	a.hideFloatingSession(ctx, session)
	a.syncFloatingFocus()
	return true
}

func (a *App) closeAndReleaseActiveFloatingPane(ctx context.Context) bool {
	key, session, ok := a.activeFloatingSessionEntry()
	if !ok {
		return false
	}
	a.releaseFloatingSession(ctx, key, session)
	a.syncFloatingFocus()
	return true
}

// ToggleFloatingPane toggles the active workspace floating pane visibility.
func (a *App) ToggleFloatingPane(ctx context.Context) error {
	activeTab := a.tabs.ActiveTab()
	if activeTab == nil {
		return nil
	}
	if a.closeActiveFloatingPane(ctx) {
		return nil
	}
	return a.openFloatingPaneSession(ctx, floatingSessionIDDefault)
}

// OpenFloatingPaneURL opens the active workspace floating pane directly to a URL.
func (a *App) OpenFloatingPaneURL(ctx context.Context, url string) error {
	return a.openFloatingPaneSession(ctx, floatingSessionIDDefault, url)
}

// OpenFloatingPaneProfileURL opens a named floating pane profile session directly to a URL.
func (a *App) OpenFloatingPaneProfileURL(ctx context.Context, sessionID, url string) error {
	return a.openFloatingPaneSession(ctx, sessionID, url)
}

func (a *App) openFloatingPaneSession(ctx context.Context, sessionID string, url ...string) error {
	activeTab := a.tabs.ActiveTab()
	if activeTab == nil {
		return nil
	}

	wsView := a.workspaceViews[activeTab.ID]
	session, err := a.ensureFloatingSession(ctx, activeTab.ID, sessionID, wsView)
	if err != nil {
		return err
	}
	hasURL := len(url) > 0 && strings.TrimSpace(url[0]) != ""
	isProfileSession := normalizeFloatingSessionID(sessionID) != floatingSessionIDDefault

	if hasURL && isProfileSession && session.pane.IsVisible() {
		a.hideFloatingSession(ctx, session)
		a.syncFloatingFocus()
		return nil
	}

	a.hideVisibleFloatingSessions(ctx, activeTab.ID, session)

	if hasURL {
		if isProfileSession && session.pane.SessionStarted() {
			session.pane.Show()
		} else {
			if err := session.pane.ShowURL(ctx, url[0]); err != nil {
				return err
			}
		}
	} else {
		if err := session.pane.ShowToggle(ctx); err != nil {
			return err
		}
	}

	a.resizeFloatingWidget(session)
	setFloatingWidgetShown(session.widget, session.pane.IsVisible())
	if session.pane.IsOmniboxVisible() {
		a.showFloatingOmnibox(ctx, session)
	} else {
		a.hideFloatingOmnibox(ctx, session)
		focusFloatingSessionContent(session)
	}
	a.startFloatingResizeWatcher(session)
	a.syncFloatingFocus()

	return nil
}

func (a *App) navigateFromOmnibox(ctx context.Context, url string) error {
	session, _ := a.activeFloatingSession()
	if session != nil && session.pane != nil && session.pane.IsVisible() && session.pane.IsOmniboxVisible() {
		return session.pane.Navigate(ctx, url)
	}
	if a.omniboxNavigateFn != nil {
		a.omniboxNavigateFn(url)
		return nil
	}
	if a.navCoord == nil {
		return fmt.Errorf("navigation coordinator not initialized")
	}
	return a.navCoord.Navigate(ctx, url)
}

// ToggleOmnibox implements OmniboxProvider.
// Toggles the omnibox visibility in the active workspace view.
func (a *App) ToggleOmnibox(ctx context.Context) {
	log := logging.FromContext(ctx)
	if session, _ := a.activeFloatingSession(); session != nil && session.pane != nil && session.pane.IsVisible() {
		wsView := a.activeWorkspaceView()
		if wsView != nil && wsView.IsOmniboxVisible() {
			wsView.HideOmnibox()
		}

		if session.omnibox != nil && session.pane.IsOmniboxVisible() {
			a.hideFloatingOmnibox(ctx, session)
		} else {
			a.showFloatingOmnibox(ctx, session)
		}
		a.syncFloatingFocus()
		return
	}

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
	a.omniboxNavigateFn = fn
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

	// Hot-reload appearance and keybindings on config change.
	a.configManager.OnConfigChange(func(newCfg *config.Config) {
		cfgCopy := newCfg
		cb := glib.SourceFunc(func(_ uintptr) bool {
			a.applyAppearanceConfig(ctx, cfgCopy)
			// Reload keyboard shortcuts
			if a.keyboardHandler != nil {
				a.keyboardHandler.ReloadShortcuts(ctx, cfgCopy)
			}
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

		// Update find highlight CSS for future navigations
		if a.injector != nil {
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

package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	urlutil "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/shared/syncdispatch"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/dialog"
	"github.com/bnema/dumber/internal/ui/dispatcher"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/adw"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	// AppID is the application identifier for GTK.
	AppID    = "com.github.bnema.dumber"
	appTitle = "Dumber"
	// crashReportToastDurationMs keeps startup crash-report toast visible longer.
	crashReportToastDurationMs    = 5000
	downloadToastTerminalDuration = 2500
	downloadProgressRoundBias     = 0.5
	floatingPaneFallbackWidth     = 1200
	floatingPaneFallbackHeight    = 800
	floatingPaneIDPrefix          = "floating-pane:"
	floatingSessionIDDefault      = "default"
	floatingPaneVisibleClass      = "floating-pane-visible"
)

func gtkApplicationFlags() gio.ApplicationFlags {
	return gio.GApplicationNonUniqueValue
}

var adwaitaInitOnce sync.Once

// EnsureAdwaitaInitialized initializes libadwaita and GTK exactly once.
func EnsureAdwaitaInitialized() {
	adwaitaInitOnce.Do(func() {
		adw.Init()
	})
}

// defaultTabName returns a window-scoped default tab name.
// index is the 1-based tab position within the window.
func defaultTabName(index int) string {
	return fmt.Sprintf("Tab %d", index)
}

// App wraps the GTK Application and manages the browser lifecycle.
type App struct {
	deps       *Dependencies
	gtkApp     *gtk.Application
	mainWindow *window.MainWindow

	browserWindows       map[string]*browserWindow
	browserWindowOrder   []string // registration order of window IDs
	lastFocusedWindowID  string
	nativePopupWindows   map[port.WebViewID]*nativePopupWindow
	browserWindowFactory func(context.Context, string) (*browserWindow, error)
	dispatchOnMainThread func(string, func()) syncdispatch.SyncDispatchResult

	// State
	tabs   *entity.TabList
	tabsUC *usecase.ManageTabsUseCase

	// Coordinators (new architecture)
	contentCoord *content.Coordinator
	tabCoord     *coordinator.TabCoordinator
	wsCoord      *coordinator.WorkspaceCoordinator
	navCoord     *coordinator.NavigationCoordinator
	kbDispatcher *dispatcher.KeyboardDispatcher

	// Pane management (used by coordinators)
	panesUC                     *usecase.ManagePanesUseCase
	workspaceViews              map[entity.TabID]*component.WorkspaceView
	windowForTab                map[entity.TabID]*browserWindow
	widgetFactory               layout.WidgetFactory
	workspaceViewCreateOverride func(context.Context, *entity.Tab) bool
	stackedPaneMgr              *component.StackedPaneManager

	// Input handling
	keyboardHandler       *input.KeyboardHandler
	globalShortcutHandler *input.GlobalShortcutHandler

	// Focus tracking stays app-global.
	focusMgr *focus.Manager

	resizeModeBorderTarget layout.Widget

	// Omnibox configuration (omnibox is created per workspace view)
	omniboxCfg component.OmniboxConfig
	// Find bar configuration (find bar is created per workspace view)
	findBarCfg component.FindBarConfig
	// Floating pane sessions keyed by tab and profile session ID.
	floatingSessions map[floatingSessionKey]*floatingWorkspaceSession

	// Engine
	engine port.Engine

	// Web content (managed by content.Coordinator)
	faviconAdapter *adapter.FaviconAdapter

	snapshotService port.SnapshotService

	// Update management
	updateCoord *coordinator.UpdateCoordinator

	// ID generator for tabs/panes
	idCounter             uint64
	idMu                  sync.Mutex
	windowIDCounter       uint64
	windowIDMu            sync.Mutex
	firstWebViewShownOnce sync.Once

	movePaneToTabUC        *usecase.MovePaneToTabUseCase
	extractPaneToTabListUC *usecase.ExtractPaneToTabListUseCase

	// Accent picker for dead keys support
	accentFocusProvider port.FocusedInputProvider

	// Deferred initialization - runs after first load_started to avoid blocking initial navigation
	deferredInitOnce sync.Once
	deferredInitFn   func()

	// lifecycle
	cancel                   context.CancelCauseFunc
	browserLaunchRelayOnce   sync.Once
	browserLaunchRelayCloser io.Closer
}

type floatingWorkspaceSession struct {
	paneID              entity.PaneID
	pane                *component.FloatingPane
	paneView            *component.PaneView
	webView             port.WebView
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
		windowForTab:     make(map[entity.TabID]*browserWindow),
		floatingSessions: make(map[floatingSessionKey]*floatingWorkspaceSession),
		browserWindows:   make(map[string]*browserWindow),
		dispatchOnMainThread: func(label string, fn func()) syncdispatch.SyncDispatchResult {
			if fn != nil {
				fn()
			}
			return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchInline}
		},
		engine: deps.Engine,
		cancel: cancel,
	}
	var autoCopyConfig port.AutoCopyConfig
	var clipboardOrchestrator port.ClipboardTextOrchestrator
	if deps.Clipboard != nil {
		autoCopyConfig = &autoCopyConfigFn{fn: func() bool {
			if deps.Config == nil {
				return false
			}
			return deps.Config.Clipboard.AutoCopyOnSelection
		}}
		clipboardOrchestrator = usecase.NewClipboardTextOrchestrator(deps.Clipboard, autoCopyConfig, func(textLen int) {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				app.showToastOnLastFocusedBrowserWindow(ctx, "Copied to clipboard", component.ToastInfo,
					component.WithDuration(component.ToastBriefDurationMs),
					component.WithPosition(component.ToastPositionBottomRight),
				)
				return false
			})
			glib.IdleAdd(&cb, 0)
		})
	}
	if faviconSetter, ok := deps.Engine.(port.SystemviewFaviconResolverSetter); ok {
		resolver := deps.SystemviewFaviconResolver
		if resolver == nil {
			resolver = deps.FaviconResolver
		}
		if resolver != nil {
			faviconSetter.SetSystemviewFaviconResolver(resolver)
		}
	}

	// Register message handlers through the engine.
	if err := deps.Engine.RegisterHandlers(ctx, port.HandlerDependencies{
		HistoryUC:                 deps.HistoryUC,
		FavoritesUC:               deps.FavoritesUC,
		Clipboard:                 deps.Clipboard,
		AutoCopyConfig:            autoCopyConfig,
		ClipboardTextOrchestrator: clipboardOrchestrator,
		OnClipboardCopied: func(textLen int) {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				app.showToastOnLastFocusedBrowserWindow(ctx, "Copied to clipboard", component.ToastInfo,
					component.WithDuration(component.ToastBriefDurationMs),
					component.WithPosition(component.ToastPositionBottomRight),
				)
				return false
			})
			glib.IdleAdd(&cb, 0)
		},
		HandlerDeps: deps.HandlerDeps,
	}); err != nil {
		return nil, err
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
	EnsureAdwaitaInitialized()
	logging.Trace().Mark("gtk_init")

	// Mark adwaita detector as available now that adw.Init() is complete.
	// This enables the highest-priority color scheme detector.
	if a.deps != nil && a.deps.AdwaitaDetector != nil {
		a.deps.AdwaitaDetector.MarkAvailable()
		log.Debug().Msg("adwaita detector marked available")

		// Refresh scripts in pooled WebViews that were prewarmed before adw.Init().
		// They have the wrong dark mode preference injected; re-inject with correct value.
		if a.engine != nil {
			_ = a.engine.OnToolkitReady(ctx)
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
	a.dispatchOnMainThread = a.runOnMainThread

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
	a.initLayoutInfrastructure()

	if err := a.OpenFreshWindow(ctx, a.initialWindowURL()); err != nil {
		log.Error().Err(err).Msg("failed to create main window")
		return
	}
	logging.Trace().Mark("window_created")

	a.installCrashReportNotifier(ctx)
	a.initFocusManager()
	a.registerAccentHandlers(ctx)
	a.initDownloadHandler(ctx)

	a.initCoordinators(ctx)
	a.wireWebRTCPermissionIndicator()
	logging.Trace().Mark("coordinators_init")
	a.initKeyboardHandler(ctx)
	a.initOmniboxConfig(ctx)
	a.initFindBarConfig(ctx)
	a.wireSessionManagerShortcut()
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
	if a.engine == nil {
		log.Debug().Msg("engine not available; skipping background color setup")
		return
	}
	if a.deps == nil || a.deps.Theme == nil {
		log.Debug().Msg("theme not available; skipping webview pool background color setup")
		return
	}
	r, g, b, alpha := a.deps.Theme.GetBackgroundRGBA()
	if err := a.engine.UpdateAppearance(ctx, float64(r), float64(g), float64(b), float64(alpha)); err != nil {
		log.Warn().Err(err).Msg("failed to update engine appearance")
	}
	log.Debug().Msg("configured webview pool background color")
}

func (a *App) prewarmWebViewPoolAsync(ctx context.Context) {
	if a.engine == nil {
		return
	}
	pool := a.engine.Pool()
	if pool == nil {
		return
	}
	pool.PrewarmAsync(ctx, 0)
}

func (a *App) createBrowserWindow(ctx context.Context, initialURL string) (*browserWindow, error) {
	if a.browserWindowFactory != nil {
		created, err := a.browserWindowFactory(ctx, initialURL)
		if err != nil {
			return nil, err
		}
		created.ensureTabs()
		a.wireBrowserWindowActivationTracking(created)
		created.initChrome(ctx, a)
		return created, nil
	}

	log := logging.FromContext(ctx)
	mainWindow, err := window.New(ctx, a.gtkApp, a.deps.Config.Workspace.TabBarPosition)
	if err != nil {
		return nil, err
	}
	browserWindow := &browserWindow{
		id:         a.generateWindowID(),
		initialURL: initialURL,
		tabs:       entity.NewTabList(),
		mainWindow: mainWindow,
	}
	a.wireBrowserWindowActivationTracking(browserWindow)
	if a.mainWindow == nil {
		a.mainWindow = mainWindow
	}

	closeRequestCb := func(_ gtk.Window) bool {
		log.Info().Msg("browser window close requested")
		a.removeBrowserWindow(browserWindow.id)
		return false
	}
	mainWindow.Window().ConnectCloseRequest(&closeRequestCb)

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
				mainWindow.AddOverlay(w)
			}
			browserWindow.permissionDialog = dialog.NewPermissionDialog(permPopup)
		}
	}

	// Create top-right WebRTC permission activity indicator.
	indicator := component.NewWebRTCPermissionIndicator()
	if indicator != nil {
		if w := indicator.Widget(); w != nil {
			mainWindow.AddOverlay(w)
		}
		browserWindow.webrtcIndicator = indicator
	}
	a.wireBrowserWindowPermissionIndicator(browserWindow)
	if a.kbDispatcher != nil {
		a.initBrowserWindowInput(ctx, browserWindow)
	}
	if a.tabCoord != nil {
		a.wireBrowserWindowTabBar(ctx, browserWindow)
	}
	browserWindow.initChrome(ctx, a)

	// Apply GTK CSS styling from theme manager.
	if a.deps == nil || a.deps.Theme == nil {
		return browserWindow, nil
	}
	if display := mainWindow.Window().GetDisplay(); display != nil {
		a.deps.Theme.ApplyToDisplay(ctx, display)
	}
	return browserWindow, nil
}

func (a *App) ensureWindowForTabMap() {
	if a.windowForTab == nil {
		a.windowForTab = make(map[entity.TabID]*browserWindow)
	}
}

func (a *App) browserWindowForMainWindow(mainWindow *window.MainWindow) *browserWindow {
	if mainWindow == nil {
		return nil
	}
	for _, bw := range a.browserWindows {
		if bw != nil && bw.mainWindow == mainWindow {
			return bw
		}
	}
	return nil
}

func (a *App) browserWindowForTab(tabID entity.TabID) *browserWindow {
	if a.windowForTab == nil {
		return nil
	}
	return a.windowForTab[tabID]
}

func (a *App) browserWindowForTabTarget(target coordinator.TabTarget) *browserWindow {
	if target.Tabs != nil {
		for _, bw := range a.browserWindows {
			if bw != nil && bw.tabs == target.Tabs {
				return bw
			}
		}
	}
	if target.MainWindow != nil {
		return a.browserWindowForMainWindow(target.MainWindow)
	}
	return nil
}

func (a *App) tabCountForBrowserWindow(bw *browserWindow) int {
	if bw == nil || bw.tabs == nil {
		return 0
	}
	return len(bw.tabs.Tabs)
}

func (a *App) browserWindowForPane(paneID entity.PaneID) *browserWindow {
	if paneID == "" {
		return nil
	}
	for _, bw := range a.browserWindows {
		if bw == nil || bw.tabs == nil {
			continue
		}
		for _, tab := range bw.tabs.Tabs {
			if tab == nil || tab.Workspace == nil {
				continue
			}
			if tab.Workspace.FindPane(paneID) != nil {
				return bw
			}
		}
	}
	return nil
}

func (a *App) browserWindowForAnyPane(paneID entity.PaneID) *browserWindow {
	if bw := a.browserWindowForPane(paneID); bw != nil {
		return bw
	}
	for key, session := range a.floatingSessions {
		if session != nil && session.paneID == paneID {
			return a.browserWindowForTab(key.tabID)
		}
	}
	return nil
}

func (a *App) lastFocusedBrowserWindow() *browserWindow {
	if a.lastFocusedWindowID == "" || a.browserWindows == nil {
		return nil
	}
	return a.browserWindows[a.lastFocusedWindowID]
}

func (a *App) showToastOnBrowserWindow(
	ctx context.Context,
	bw *browserWindow,
	message string,
	level component.ToastLevel,
	opts ...component.ToastOption,
) {
	if bw == nil || bw.appToaster == nil {
		return
	}
	bw.appToaster.Show(ctx, message, level, opts...)
}

func (a *App) showToastOnLastFocusedBrowserWindow(
	ctx context.Context,
	message string,
	level component.ToastLevel,
	opts ...component.ToastOption,
) {
	a.showToastOnBrowserWindow(ctx, a.lastFocusedBrowserWindow(), message, level, opts...)
}

func (a *App) ownerOrLastFocusedBrowserWindow(tabID entity.TabID, paneID entity.PaneID) *browserWindow {
	if paneID != "" {
		if bw := a.browserWindowForPane(paneID); bw != nil {
			return bw
		}
	}
	if tabID != "" {
		if bw := a.browserWindowForTab(tabID); bw != nil {
			return bw
		}
	}
	return a.lastFocusedBrowserWindow()
}

func (a *App) createPopupTab(ctx context.Context, popupInput content.InsertPopupInput) error {
	bw := a.browserWindowForAnyPane(popupInput.ParentPaneID)
	if bw == nil {
		return fmt.Errorf("popup owner window not found for parent pane %q", popupInput.ParentPaneID)
	}
	tab, err := a.tabCoord.CreateWithPane(
		ctx,
		a.ensureTabTargetForBrowserWindow(bw),
		popupInput.PopupPane,
		popupInput.WebView,
		popupInput.TargetURI,
	)
	if err != nil {
		return err
	}
	logging.FromContext(ctx).Debug().Str("tab_id", string(tab.ID)).Msg("created tab for popup")
	return nil
}

func (a *App) handlePaneWindowTitleChanged(paneID entity.PaneID, title string) {
	a.updateWindowTitle(title, a.browserWindowForPane(paneID))
}

func (a *App) handlePaneFullscreenChanged(paneID entity.PaneID, entering bool) {
	bw := a.browserWindowForPane(paneID)
	if bw == nil || bw.mainWindow == nil || bw.mainWindow.TabBar() == nil {
		return
	}
	if entering {
		bw.mainWindow.TabBar().SetVisible(false)
		bw.mainWindow.SetTabBarContentInsetVisible(false)
		return
	}
	bw.mainWindow.TabBar().SetVisible(true)
	a.updateBrowserWindowTabBarVisibility(bw)
}

func (a *App) updateBrowserWindowTabBarVisibility(bw *browserWindow) {
	if bw == nil || bw.mainWindow == nil || bw.mainWindow.TabBar() == nil {
		return
	}
	if a.deps == nil || a.deps.Config == nil || !a.deps.Config.Workspace.HideTabBarWhenSingleTab {
		bw.mainWindow.TabBar().SetAutoHidden(false)
		bw.mainWindow.SetTabBarContentInsetVisible(true)
		return
	}
	shouldShow := bw.mainWindow.TabBar().Count() > 1
	bw.mainWindow.TabBar().SetAutoHidden(!shouldShow)
	bw.mainWindow.SetTabBarContentInsetVisible(shouldShow)
}

func (a *App) activateBrowserWindow(bw *browserWindow) {
	if bw == nil {
		return
	}
	a.lastFocusedWindowID = bw.id
	if bw.mainWindow != nil {
		a.mainWindow = bw.mainWindow
	}
	a.keyboardHandler = bw.keyboardHandler
	a.globalShortcutHandler = bw.globalShortcutHandler
	if a.tabCoord != nil {
		a.tabCoord.SetCurrentTarget(a.tabTargetForBrowserWindow(bw))
	}
	// Sync the derived global App.tabs mirror from the active browser window.
	// Some usecase and snapshot/export boundaries still accept a single TabList,
	// so the mirror follows per-window active tab state without selecting runtime targets.
	activeTab := a.activeTabForBrowserWindow(bw)
	activeID := entity.TabID("")
	if activeTab != nil {
		activeID = activeTab.ID
	}
	perWinTabs := a.tabListForBrowserWindow(bw)
	if a.tabs != nil && activeID != "" {
		if a.tabs.Find(activeID) == nil {
			if tab := perWinTabs.Find(activeID); tab != nil {
				a.tabs.Add(cloneTabForGlobalList(tab))
			}
		}
		if a.tabs.Find(activeID) != nil {
			a.tabs.SetActive(activeID)
		}
	}
	a.updateWindowTitleFromActivePane(activeID)
}

func (a *App) handleBrowserWindowActivationChanged(bw *browserWindow, active bool) {
	if !active || bw == nil {
		return
	}
	a.activateBrowserWindow(bw)
}

func (a *App) wireBrowserWindowActivationTracking(bw *browserWindow) {
	if bw == nil || bw.mainWindow == nil {
		return
	}
	bw.mainWindow.ConnectActiveNotify(func(active bool) {
		a.handleBrowserWindowActivationChanged(bw, active)
	})
}

func (a *App) setBrowserWindowForTab(tabID entity.TabID, bw *browserWindow) {
	if bw == nil {
		return
	}
	a.ensureWindowForTabMap()
	a.windowForTab[tabID] = bw
	tabs := a.ensureTabListForBrowserWindow(bw)
	if tabs.ActiveTabID == "" {
		tabs.SetActive(tabID)
	}
}

func (a *App) tabBarForBrowserWindow(bw *browserWindow) *component.TabBar {
	if bw != nil && bw.mainWindow != nil {
		return bw.mainWindow.TabBar()
	}
	if a.mainWindow != nil {
		return a.mainWindow.TabBar()
	}
	return nil
}

func (a *App) startBrowserLaunchRelayListener(ctx context.Context) {
	if a == nil || a.deps == nil || a.deps.BrowserLaunchRelay == nil {
		return
	}
	a.browserLaunchRelayOnce.Do(func() {
		closer, err := a.deps.BrowserLaunchRelay.Listen(ctx, a)
		if err != nil {
			logging.FromContext(ctx).Warn().Err(err).Msg("failed to start browser launch relay listener")
			return
		}
		a.browserLaunchRelayCloser = closer
	})
}

func (a *App) closeBrowserLaunchRelayListener() {
	if a.browserLaunchRelayCloser == nil {
		return
	}
	_ = a.browserLaunchRelayCloser.Close()
	a.browserLaunchRelayCloser = nil
}

func (a *App) openFreshWindow(ctx context.Context, url string) error {
	created, err := a.createBrowserWindow(ctx, url)
	if err != nil {
		return err
	}
	a.registerBrowserWindow(created)

	if len(a.browserWindows) == 1 {
		a.activateBrowserWindow(created)
		return nil
	}

	var openErr error
	if a.tabCoord != nil {
		openErr = a.openFreshWindowWithTabCoord(ctx, url, created)
	} else if a.tabsUC == nil {
		a.activateBrowserWindow(created)
		return nil
	} else {
		openErr = a.openFreshWindowWithTabsUC(ctx, url, created)
	}
	if openErr != nil {
		return openErr
	}
	a.activateBrowserWindow(created)
	return nil
}

func (a *App) openFreshWindowWithTabCoord(ctx context.Context, url string, created *browserWindow) error {
	previousTarget := a.tabCoord.CurrentTarget()
	a.tabCoord.SetCurrentTarget(a.ensureTabTargetForBrowserWindow(created))
	defer func() {
		a.tabCoord.SetCurrentTarget(previousTarget)
	}()

	createdTab, createErr := a.tabCoord.Create(ctx, a.ensureTabTargetForBrowserWindow(created), url)
	if createErr != nil {
		a.removeBrowserWindow(created.id)
		if created.mainWindow != nil {
			created.mainWindow.Destroy()
		}
		return createErr
	}
	if a.workspaceViews[createdTab.ID] == nil {
		if createdTabs := a.tabListForBrowserWindow(created); createdTabs != nil {
			createdTabs.Remove(createdTab.ID)
		}
		if created.mainWindow != nil {
			if tabBar := created.mainWindow.TabBar(); tabBar != nil {
				tabBar.RemoveTab(createdTab.ID)
				if activeTab := a.activeTabForBrowserWindow(created); activeTab != nil {
					tabBar.SetActive(activeTab.ID)
				}
			}
		}
		a.removeBrowserWindow(created.id)
		if created.mainWindow != nil {
			created.mainWindow.Destroy()
		}
		return fmt.Errorf("workspace view not created for tab %s", createdTab.ID)
	}
	a.setBrowserWindowForTab(createdTab.ID, created)
	// Tab was already created in the per-window TabList via tabCoord.Create;
	// active state is managed by TabList.SetActive.
	if created.mainWindow != nil {
		created.mainWindow.Show()
	}
	return nil
}

func (a *App) openFreshWindowWithTabsUC(ctx context.Context, url string, created *browserWindow) error {
	output, err := a.tabsUC.Create(ctx, usecase.CreateTabInput{
		TabList:    a.tabs,
		InitialURL: url,
	})
	if err != nil {
		a.removeBrowserWindow(created.id)
		if created.mainWindow != nil {
			created.mainWindow.Destroy()
		}
		return err
	}
	a.setBrowserWindowForTab(output.Tab.ID, created)
	// Mirror the tabsUC-created tab into the new browser window's tab list.
	// This path uses the tabs usecase directly instead of tabCoord.Create,
	// so the per-window TabList must be populated explicitly.
	a.ensureTabListForBrowserWindow(created).Add(output.Tab)
	if a.widgetFactory != nil {
		a.createWorkspaceView(ctx, output.Tab)
		a.switchWorkspaceView(ctx, output.Tab.ID)
	}
	if a.workspaceViews[output.Tab.ID] == nil {
		if createdTabs := a.tabListForBrowserWindow(created); createdTabs != nil {
			createdTabs.Remove(output.Tab.ID)
		}
		if a.tabs != nil {
			a.tabs.Remove(output.Tab.ID)
		}
		if created.mainWindow != nil {
			if tabBar := created.mainWindow.TabBar(); tabBar != nil {
				tabBar.RemoveTab(output.Tab.ID)
				if activeTab := a.activeTabForBrowserWindow(created); activeTab != nil {
					tabBar.SetActive(activeTab.ID)
				}
			}
		}
		a.removeBrowserWindow(created.id)
		if created.mainWindow != nil {
			created.mainWindow.Destroy()
		}
		return fmt.Errorf("workspace view not created for tab %s", output.Tab.ID)
	}
	if created.mainWindow != nil {
		created.mainWindow.Show()
	}
	return nil
}

func (a *App) initialWindowURL() string {
	if a.deps != nil {
		return urlutil.ResolveBrowserStartupURL(a.deps.InitialURL)
	}
	return urlutil.DefaultBrowserStartupURL()
}

func (a *App) initLayoutInfrastructure() {
	// Initialize widget factory for pane layout.
	a.widgetFactory = layout.NewGtkWidgetFactory()

	// Initialize stacked pane manager for incremental widget operations.
	a.stackedPaneMgr = component.NewStackedPaneManager(a.widgetFactory)
}

func (a *App) installCrashReportNotifier(ctx context.Context) {
	if a.deps == nil {
		return
	}

	a.deps.OnCrashReportsDetected = func(paths []string) {
		a.showCrashReportToast(ctx, paths)
	}
}

func (a *App) initFocusManager() {
	a.focusMgr = focus.NewManager(a.panesUC)
}

func (a *App) wireSessionManagerShortcut() {
	if a.kbDispatcher == nil {
		return
	}
	a.kbDispatcher.SetOnSessionOpen(func(ctx context.Context, paneID entity.PaneID) error {
		bw := a.ownerOrLastFocusedBrowserWindow("", paneID)
		if bw == nil || bw.sessionManager == nil {
			return nil
		}
		bw.sessionManager.Toggle(ctx)
		return nil
	})
}

// registerAccentHandlers registers the accent key press/release message handlers
// with the router.
func (a *App) registerAccentHandlers(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.engine == nil {
		log.Debug().Msg("engine not available, skipping accent handler registration")
		return
	}

	if err := a.engine.RegisterAccentHandlers(ctx, a); err != nil {
		log.Error().Err(err).Msg("failed to register accent handlers")
	} else {
		log.Info().Msg("registered accent key handlers")
	}
}

func (a *App) activeAccentHandler() input.AccentHandler {
	bw := a.lastFocusedBrowserWindow()
	if bw == nil {
		return nil
	}
	return bw.insertAccentUC
}

func (a *App) initDownloadHandler(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.deps == nil || a.engine == nil {
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
	eventAdapter := newDownloadEventAdapter(a)

	// Create use case for preparing download destinations with file deduplication.
	prepareDownloadUC := usecase.NewPrepareDownloadUseCase(a.deps.FileSystem)

	if err := a.engine.ConfigureDownloads(ctx, downloadPath, eventAdapter, prepareDownloadUC); err != nil {
		log.Error().Err(err).Msg("failed to configure downloads")
		return
	}

	log.Info().Str("path", downloadPath).Msg("download handler initialized")
}

// downloadEventAdapter implements port.DownloadEventHandler and shows toasts.
type downloadEventAdapter struct {
	app    *App
	mu     sync.Mutex
	active map[string]downloadToastState
}

type downloadToastState struct {
	filename string
	percent  int
}

type downloadToastSpec struct {
	message  string
	level    component.ToastLevel
	duration int
	show     bool
}

func newDownloadEventAdapter(app *App) *downloadEventAdapter {
	return &downloadEventAdapter{
		app:    app,
		active: make(map[string]downloadToastState),
	}
}

func (d *downloadEventAdapter) OnDownloadEvent(ctx context.Context, event port.DownloadEvent) {
	spec := d.toastSpecForEvent(event)
	if !spec.show {
		return
	}

	// Must schedule on GTK main thread.
	cb := glib.SourceFunc(func(_ uintptr) bool {
		d.app.showToastOnLastFocusedBrowserWindow(
			ctx,
			spec.message,
			spec.level,
			component.WithDuration(spec.duration),
		)
		return false
	})
	glib.IdleAdd(&cb, 0)
}

func (d *downloadEventAdapter) toastSpecForEvent(event port.DownloadEvent) downloadToastSpec {
	key := downloadToastKey(event)
	filename := event.Filename
	if filename == "" {
		filename = filepath.Base(event.Destination)
	}
	if filename == "" {
		filename = "download"
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	state := d.active[key]
	if state.filename == "" {
		state.filename = filename
	}

	switch event.Type {
	case port.DownloadEventStarted:
		state.percent = 0
		d.active[key] = state
		return downloadToastSpec{
			message:  fmt.Sprintf("Download started: %s (0%%)", state.filename),
			level:    component.ToastInfo,
			duration: 0,
			show:     true,
		}
	case port.DownloadEventProgress:
		percent := downloadProgressPercent(event.Progress)
		if percent <= state.percent {
			if _, ok := d.active[key]; !ok {
				d.active[key] = state
			}
			return downloadToastSpec{}
		}
		state.percent = percent
		d.active[key] = state
		return downloadToastSpec{
			message:  fmt.Sprintf("Downloading: %s (%d%%)", state.filename, percent),
			level:    component.ToastInfo,
			duration: 0,
			show:     true,
		}
	case port.DownloadEventFinished:
		delete(d.active, key)
		return downloadToastSpec{
			message:  "Download complete: " + state.filename,
			level:    component.ToastSuccess,
			duration: downloadToastTerminalDuration,
			show:     true,
		}
	case port.DownloadEventFailed:
		delete(d.active, key)
		return downloadToastSpec{
			message:  "Download failed: " + state.filename,
			level:    component.ToastError,
			duration: downloadToastTerminalDuration,
			show:     true,
		}
	default:
		return downloadToastSpec{}
	}
}

func downloadToastKey(event port.DownloadEvent) string {
	if event.Destination != "" {
		return event.Destination
	}
	if event.Filename != "" {
		return event.Filename
	}
	return "download"
}

func downloadProgressPercent(progress float64) int {
	if progress <= 0 {
		return 0
	}
	if progress >= 1 {
		return 100
	}
	percent := int(progress*100 + downloadProgressRoundBias)
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

// autoCopyConfigFn implements port.AutoCopyConfig using a function closure.
type autoCopyConfigFn struct {
	fn func() bool
}

func (a *autoCopyConfigFn) IsAutoCopyEnabled() bool {
	if a.fn == nil {
		return false
	}
	return a.fn()
}

func (a *App) initKeyboardHandler(ctx context.Context) {
	bw := a.browserWindowForMainWindow(a.mainWindow)
	if bw == nil {
		return
	}
	a.initBrowserWindowInput(ctx, bw)
}

func (a *App) initBrowserWindowInput(ctx context.Context, bw *browserWindow) {
	log := logging.FromContext(ctx)

	if bw == nil || bw.mainWindow == nil || a.deps == nil || a.deps.Config == nil || a.kbDispatcher == nil {
		return
	}
	if bw.keyboardHandler != nil || bw.globalShortcutHandler != nil {
		return
	}

	bw.keyboardHandler = input.NewKeyboardHandler(ctx, &a.deps.Config.Workspace, &a.deps.Config.Session)
	bw.keyboardHandler.SetOnAction(func(actionCtx context.Context, action input.Action) error {
		a.activateBrowserWindow(bw)
		if action == input.ActionClosePane {
			if a.closeAndReleaseActiveFloatingPane(actionCtx) {
				return nil
			}
		}
		return a.dispatchBrowserWindowAction(actionCtx, bw, action)
	})
	bw.keyboardHandler.SetOnEscape(func(escapeCtx context.Context) bool {
		a.activateBrowserWindow(bw)
		return a.handleGlobalEscape(escapeCtx)
	})
	bw.keyboardHandler.SetOnModeChange(func(from, to input.Mode) {
		a.activateBrowserWindow(bw)
		a.handleModeChange(ctx, from, to)
	})
	bw.keyboardHandler.SetRouteKey(func(kc input.KeyContext) input.KeyRoute {
		if bw.sessionManager != nil && bw.sessionManager.IsVisible() {
			return input.RoutePassToWidget
		}
		if bw.tabPicker != nil && bw.tabPicker.IsVisible() {
			return input.RoutePassToWidget
		}

		if a.accentFocusProvider != nil {
			if _, ok := a.accentFocusProvider.GetFocusedInput().(port.EntryInputTarget); ok {
				if kc.Modifiers&input.ModAlt != 0 {
					return input.RouteHandleShortcuts
				}
				return input.RoutePassToWidget
			}
		}

		if input.IsShortcutModified(kc.Modifiers) {
			return input.RouteHandleShortcuts
		}
		if input.IsTextInputKey(kc.Keyval) {
			return input.RoutePassToWidget
		}

		return input.RouteHandleShortcuts
	})
	bw.keyboardHandler.SetAccentHandler(a)
	bw.keyboardHandler.AttachTo(bw.mainWindow.Window())

	bw.globalShortcutHandler = input.NewGlobalShortcutHandler(
		ctx,
		bw.mainWindow.Window(),
		&a.deps.Config.Workspace,
		&a.deps.Config.Session,
		bw.keyboardHandler,
		func(actionCtx context.Context, action input.Action) error {
			a.activateBrowserWindow(bw)
			return a.dispatchBrowserWindowAction(actionCtx, bw, action)
		},
	)
	if bw.globalShortcutHandler == nil {
		log.Warn().Str("window_id", bw.id).Msg("global shortcut handler creation failed, shortcuts may not work when WebView has focus")
	}

	a.activateBrowserWindow(bw)
}

func (a *App) wireBrowserWindowTabBar(ctx context.Context, bw *browserWindow) {
	if bw == nil || bw.mainWindow == nil || bw.mainWindow.TabBar() == nil || a.tabCoord == nil {
		return
	}
	bw.mainWindow.TabBar().SetOnSwitch(func(tabID entity.TabID) {
		a.activateBrowserWindow(bw)
		if err := a.tabCoord.Switch(ctx, a.ensureTabTargetForBrowserWindow(bw), tabID); err != nil {
			logging.FromContext(ctx).Error().Err(err).Str("tab_id", string(tabID)).Str("window_id", bw.id).Msg("tab switch failed")
			return
		}
		// Active tab state is managed by TabCoordinator.Switch → TabList.SetActive.
	})
}

// handleAccentKeyPress handles accent key press events for GTK entry widgets
// (omnibox and find bar). Returns true if the key was consumed.
func (a *App) handleAccentKeyPress(ctx context.Context, keyval uint, state gdk.ModifierType) bool {
	handler := a.activeAccentHandler()
	if handler == nil {
		return false
	}
	if state&(gdk.ControlMaskValue|gdk.AltMaskValue) != 0 {
		return false
	}
	shiftHeld := state&gdk.ShiftMaskValue != 0
	if char := input.KeyvalToRune(keyval); char != 0 {
		return handler.OnKeyPressed(ctx, char, shiftHeld)
	}
	return false
}

// handleAccentKeyRelease handles accent key release events for GTK entry widgets
// (omnibox and find bar).
func (a *App) handleAccentKeyRelease(ctx context.Context, keyval uint) {
	handler := a.activeAccentHandler()
	if handler == nil {
		return
	}
	if char := input.KeyvalToRune(keyval); char != 0 {
		handler.OnKeyReleased(ctx, char)
	}
}

func (a *App) OnKeyPressed(ctx context.Context, char rune, shiftHeld bool) bool {
	handler := a.activeAccentHandler()
	if handler == nil {
		return false
	}
	return handler.OnKeyPressed(ctx, char, shiftHeld)
}

func (a *App) OnKeyReleased(ctx context.Context, char rune) {
	handler := a.activeAccentHandler()
	if handler == nil {
		return
	}
	handler.OnKeyReleased(ctx, char)
}

func (a *App) IsPickerVisible() bool {
	handler := a.activeAccentHandler()
	if handler == nil {
		return false
	}
	return handler.IsPickerVisible()
}

func (a *App) Cancel(ctx context.Context) {
	handler := a.activeAccentHandler()
	if handler == nil {
		return
	}
	handler.Cancel(ctx)
}

type omniboxCallbacks struct {
	OnNavigate         func(ctx context.Context, url string) error
	OnToast            func(ctx context.Context, message string, level component.ToastLevel)
	OnFocusIn          func(entry *gtk.SearchEntry)
	OnFocusOut         func()
	OnAccentKeyPress   func(keyval uint, state gdk.ModifierType) bool
	OnAccentKeyRelease func(keyval uint)
}

func buildOmniboxConfig(
	deps *Dependencies,
	faviconAdapter *adapter.FaviconAdapter,
	callbacks omniboxCallbacks,
) component.OmniboxConfig {
	if deps == nil || deps.Config == nil {
		return component.OmniboxConfig{}
	}

	shortcuts := make(map[string]usecase.SearchShortcut, len(deps.Config.SearchShortcuts))
	for key, shortcut := range deps.Config.SearchShortcuts {
		shortcuts[key] = usecase.SearchShortcut{
			URL:         shortcut.URL,
			Description: shortcut.Description,
		}
	}

	return component.OmniboxConfig{
		HistoryUC:           deps.HistoryUC,
		FavoritesUC:         deps.FavoritesUC,
		FaviconAdapter:      faviconAdapter,
		CopyURLUC:           deps.CopyURLUC,
		ShortcutsUC:         usecase.NewSearchShortcutsUseCase(shortcuts),
		DefaultSearch:       deps.Config.DefaultSearchEngine,
		InitialBehavior:     deps.Config.Omnibox.InitialBehavior,
		MostVisitedDays:     deps.Config.Omnibox.MostVisitedDays,
		SaveInitialBehavior: deps.HandlerDeps.SaveOmniboxInitialBehavior,
		UIScale:             deps.Config.DefaultUIScale,
		OnNavigate:          callbacks.OnNavigate,
		OnToast:             callbacks.OnToast,
		OnFocusIn:           callbacks.OnFocusIn,
		OnFocusOut:          callbacks.OnFocusOut,
		OnAccentKeyPress:    callbacks.OnAccentKeyPress,
		OnAccentKeyRelease:  callbacks.OnAccentKeyRelease,
	}
}

func registerFaviconInvalidator(resolver port.FaviconResolver, invalidator port.FaviconInvalidator) {
	registry, ok := resolver.(port.FaviconInvalidatorRegistry)
	if !ok || invalidator == nil {
		return
	}
	registry.RegisterFaviconInvalidator(invalidator)
}

func NewStandaloneOmniboxRuntime(
	ctx context.Context,
	deps *Dependencies,
	faviconDB port.FaviconDatabase,
) *StandaloneOmniboxRuntime {
	var faviconAdapter *adapter.FaviconAdapter
	if deps != nil && (deps.FaviconService != nil || deps.FaviconResolver != nil) {
		faviconAdapter = adapter.NewFaviconAdapterWithResolver(deps.FaviconService, deps.FaviconResolver, faviconDB, deps.FaviconAdapterConfig)
		registerFaviconInvalidator(deps.FaviconResolver, faviconAdapter)
	}

	omniboxCfg := buildOmniboxConfig(deps, faviconAdapter, omniboxCallbacks{
		OnNavigate: func(navCtx context.Context, url string) error {
			return handleStandaloneOmniboxNavigation(deps, navCtx, url)
		},
		OnToast: func(toastCtx context.Context, message string, level component.ToastLevel) {
			logging.FromContext(toastCtx).Debug().Str("message", message).Int("level", int(level)).Msg("standalone omnibox toast")
		},
		OnFocusIn:          func(*gtk.SearchEntry) {},
		OnFocusOut:         func() {},
		OnAccentKeyPress:   func(uint, gdk.ModifierType) bool { return false },
		OnAccentKeyRelease: func(uint) {},
	})

	return &StandaloneOmniboxRuntime{OmniboxCfg: omniboxCfg, ApplyTheme: func(display *gdk.Display) {
		if deps != nil && deps.Theme != nil && display != nil {
			deps.Theme.ApplyToDisplay(ctx, display)
		}
	}}
}

func handleStandaloneOmniboxNavigation(deps *Dependencies, ctx context.Context, rawURL string) error {
	if deps == nil {
		return fmt.Errorf("standalone omnibox dependencies are not configured")
	}
	if urlutil.IsExternalScheme(rawURL) {
		if deps.LaunchExternalURL == nil {
			return fmt.Errorf("external launcher is not configured")
		}
		deps.LaunchExternalURL(rawURL)
		return nil
	}
	if deps.LaunchBrowserURL == nil {
		return fmt.Errorf("browser launcher is not configured")
	}
	return deps.LaunchBrowserURL(ctx, rawURL)
}

func (a *App) initOmniboxConfig(ctx context.Context) {
	if a.deps == nil || a.deps.Config == nil {
		return
	}

	a.omniboxCfg = buildOmniboxConfig(a.deps, a.faviconAdapter, omniboxCallbacks{
		OnNavigate: func(navCtx context.Context, url string) error {
			return a.navigateFromOmnibox(navCtx, url)
		},
		OnToast: func(toastCtx context.Context, message string, level component.ToastLevel) {
			a.showToastOnLastFocusedBrowserWindow(toastCtx, message, level)
		},
		OnFocusIn: func(entry *gtk.SearchEntry) {
			// Set omnibox entry as the focused input for accent picker
			if a.accentFocusProvider != nil && entry != nil {
				if a.deps.NewGTKEntryTarget != nil {
					a.accentFocusProvider.SetFocusedInput(a.deps.NewGTKEntryTarget(entry))
				}
			}
		},
		OnFocusOut: func() {
			// When omnibox loses focus, set WebView as the focused input
			if a.accentFocusProvider != nil {
				a.accentFocusProvider.SetFocusedInput(a.getActiveWebViewTarget())
			}
		},
		OnAccentKeyPress: func(keyval uint, state gdk.ModifierType) bool {
			return a.handleAccentKeyPress(ctx, keyval, state)
		},
		OnAccentKeyRelease: func(keyval uint) {
			a.handleAccentKeyRelease(ctx, keyval)
		},
	})
	a.navCoord.SetOmniboxProvider(a)
	logging.FromContext(ctx).Debug().Msg("omnibox config stored, provider set")
}

func (a *App) initFindBarConfig(ctx context.Context) {
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
				if a.deps.NewGTKEntryTarget != nil {
					a.accentFocusProvider.SetFocusedInput(a.deps.NewGTKEntryTarget(entry))
				}
			}
		},
		OnFocusOut: func() {
			// When find bar loses focus, set WebView as the focused input
			if a.accentFocusProvider != nil {
				a.accentFocusProvider.SetFocusedInput(a.getActiveWebViewTarget())
			}
		},
		OnAccentKeyPress: func(keyval uint, state gdk.ModifierType) bool {
			return a.handleAccentKeyPress(ctx, keyval, state)
		},
		OnAccentKeyRelease: func(keyval uint) {
			a.handleAccentKeyRelease(ctx, keyval)
		},
	}
}

// ToggleSessionManager shows or hides the session manager.
func (a *App) ToggleSessionManager(ctx context.Context) {
	bw := a.lastFocusedBrowserWindow()
	if bw == nil || bw.sessionManager == nil {
		return
	}
	bw.sessionManager.Toggle(ctx)
}

func (a *App) attachTabPickerToActivePane() {
	bw := a.lastFocusedBrowserWindow()
	if bw == nil || bw.tabPicker == nil || a.widgetFactory == nil {
		return
	}
	wsView := a.activeWorkspaceView()
	if wsView == nil {
		return
	}

	activeTab := a.activeTabForBrowserWindow(bw)
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

	if bw.tabPickerWidget == nil {
		bw.tabPickerWidget = bw.tabPicker.WidgetAsLayout(a.widgetFactory)
		if bw.tabPickerWidget == nil {
			return
		}
	}

	// If currently attached to a different pane overlay, detach.
	if bw.tabPickerPaneID != "" && bw.tabPickerPaneID != activePaneID {
		for _, view := range a.workspaceViews {
			if view == nil {
				continue
			}
			if oldPV := view.GetPaneView(bw.tabPickerPaneID); oldPV != nil {
				if parent := bw.tabPickerWidget.GetParent(); parent == oldPV.Overlay() {
					oldPV.RemoveOverlayWidget(bw.tabPickerWidget)
				} else if parent != nil {
					bw.tabPickerWidget.Unparent()
				}
				break
			}
		}
	}

	// Ensure the widget can be reparented.
	if parent := bw.tabPickerWidget.GetParent(); parent != nil {
		bw.tabPickerWidget.Unparent()
	}

	bw.tabPicker.SetParentOverlay(pv.Overlay())
	pv.AddOverlayWidget(bw.tabPickerWidget)
	bw.tabPickerPaneID = activePaneID
}

func (a *App) HandleMovePaneToTab(ctx context.Context) error {
	if a.movePaneToTabUC == nil {
		return nil
	}
	bw := a.lastFocusedBrowserWindow()
	if bw == nil || bw.tabPicker == nil {
		return nil
	}

	sourceTabs := a.tabListForBrowserWindow(bw)
	sourceTab := a.activeTabForBrowserWindow(bw)
	if sourceTabs == nil || sourceTab == nil || sourceTab.Workspace == nil {
		return nil
	}

	// If there is only 1 tab, auto-create a new tab and move there.
	if sourceTabs.Count() <= 1 {
		return a.MoveActivePaneToTab(ctx, "")
	}

	items := make([]component.TabPickerItem, 0, sourceTabs.Count())
	for _, tab := range sourceTabs.Tabs {
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
	bw.tabPicker.Show(ctx, items)
	return nil
}

func (a *App) HandleMovePaneToNextTab(ctx context.Context) error {
	if a.movePaneToTabUC == nil {
		return nil
	}
	bw := a.lastFocusedBrowserWindow()
	sourceTabs := a.tabListForBrowserWindow(bw)
	active := a.activeTabForBrowserWindow(bw)
	if sourceTabs == nil || active == nil {
		return nil
	}

	nextPos := active.Position + 1
	if nextPos >= sourceTabs.Count() {
		// Create a new tab on the right.
		return a.MoveActivePaneToTab(ctx, "")
	}
	next := sourceTabs.TabAt(nextPos)
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

	sourceWindow := a.browserWindowForTab(sourceTab.ID)
	if sourceWindow == nil {
		sourceWindow = a.browserWindowForMainWindow(a.mainWindow)
	}
	targetWindow := sourceWindow
	if targetTabID != "" {
		if owner := a.browserWindowForTab(targetTabID); owner != nil {
			targetWindow = owner
		}
	}

	out, err := a.movePaneToTabUC.Execute(*in)
	if err != nil {
		return err
	}
	if out == nil || out.TargetTab == nil {
		return nil
	}
	if out.NewTabCreated && targetWindow != nil {
		a.setBrowserWindowForTab(out.TargetTab.ID, targetWindow)
	}
	a.syncCrossWindowMovePaneTabLists(out, sourceTab, sourceWindow, targetWindow)

	a.applyMovePaneToTabUI(ctx, out, sourceTab, sourceWindow, targetWindow)
	a.switchToTargetTabIfConfigured(ctx, out, targetWindow)
	if a.tabCoord != nil {
		seen := map[*browserWindow]struct{}{}
		for _, bw := range []*browserWindow{sourceWindow, targetWindow} {
			if bw == nil {
				continue
			}
			if _, ok := seen[bw]; ok {
				continue
			}
			seen[bw] = struct{}{}
			a.tabCoord.UpdateBarVisibility(ctx, a.tabTargetForBrowserWindow(bw))
		}
	}
	a.MarkDirty()
	return nil
}

func (a *App) moveActivePaneToTabFromBrowserWindow(ctx context.Context, bw *browserWindow, targetTabID entity.TabID) error {
	if bw != nil {
		a.activateBrowserWindow(bw)
	}
	return a.MoveActivePaneToTab(ctx, targetTabID)
}

func (a *App) buildMovePaneToTabInput(targetTabID entity.TabID) (*usecase.MovePaneToTabInput, *entity.Tab) {
	if a.movePaneToTabUC == nil {
		return nil, nil
	}
	sourceWindow := a.lastFocusedBrowserWindow()
	tabList := a.tabListForBrowserWindow(sourceWindow)
	activeTab := a.activeTabForBrowserWindow(sourceWindow)
	if tabList == nil || activeTab == nil || activeTab.Workspace == nil {
		return nil, nil
	}
	if targetTabID != "" {
		if targetWindow := a.browserWindowForTab(targetTabID); targetWindow != nil && targetWindow != sourceWindow {
			// Cross-window pane moves still pass a single derived tab list because
			// MovePaneToTabInput accepts one TabList. Ensure the derived global
			// mirror contains source and target tabs before passing a.tabs.
			a.syncDerivedGlobalTabMirror(activeTab)
			if targetWindowTabs := a.tabListForBrowserWindow(targetWindow); targetWindowTabs != nil {
				if targetTab := targetWindowTabs.Find(targetTabID); targetTab != nil {
					a.syncDerivedGlobalTabMirror(targetTab)
				}
			}
			if a.tabs == nil {
				return nil, nil
			}
			tabList = a.tabs
		}
	}
	sourcePaneID := activeTab.Workspace.ActivePaneID
	if sourcePaneID == "" {
		return nil, nil
	}

	in := &usecase.MovePaneToTabInput{
		TabList:      tabList,
		SourceTabID:  activeTab.ID,
		SourcePaneID: sourcePaneID,
		TargetTabID:  targetTabID,
	}
	return in, activeTab
}

func (a *App) syncCrossWindowMovePaneTabLists(
	out *usecase.MovePaneToTabOutput,
	sourceTab *entity.Tab,
	sourceWindow *browserWindow,
	targetWindow *browserWindow,
) {
	if out == nil || out.TargetTab == nil || sourceWindow == nil || targetWindow == nil {
		return
	}

	if out.NewTabCreated {
		targetTabs := a.ensureTabListForBrowserWindow(targetWindow)
		if targetTabs != nil && targetTabs.Find(out.TargetTab.ID) == nil {
			targetTabs.Add(out.TargetTab)
		}
	}

	if sourceWindow == targetWindow {
		return
	}

	if out.SourceTabClosed && sourceTab != nil {
		if sourceTabs := a.tabListForBrowserWindow(sourceWindow); sourceTabs != nil {
			sourceTabs.Remove(sourceTab.ID)
		}
	}

	if owner := a.browserWindowForTab(out.TargetTab.ID); owner == targetWindow {
		targetTabs := a.ensureTabListForBrowserWindow(targetWindow)
		if targetTabs != nil && targetTabs.Find(out.TargetTab.ID) == nil {
			targetTabs.Add(out.TargetTab)
		}
	}
}

func (a *App) applyMovePaneToTabUI(
	ctx context.Context,
	out *usecase.MovePaneToTabOutput,
	sourceTab *entity.Tab,
	sourceWindow *browserWindow,
	targetWindow *browserWindow,
) {
	if out == nil || out.TargetTab == nil {
		return
	}
	sourceTabID := entity.TabID("")
	if sourceTab != nil {
		sourceTabID = sourceTab.ID
	}

	if out.SourceTabClosed {
		a.removeSourceTabUI(sourceTabID, sourceWindow)
	}
	if out.NewTabCreated {
		a.ensureTargetTabUI(ctx, out.TargetTab, targetWindow)
	}

	if !out.SourceTabClosed {
		a.rebuildAndAttachWorkspace(ctx, sourceTabID, sourceTab)
	}
	a.rebuildAndAttachWorkspace(ctx, out.TargetTab.ID, out.TargetTab)
}

func (a *App) removeSourceTabUI(sourceTabID entity.TabID, sourceWindow *browserWindow) {
	a.releaseFloatingSessionsForTab(context.Background(), sourceTabID)
	delete(a.workspaceViews, sourceTabID)
	if tabBar := a.tabBarForBrowserWindow(sourceWindow); tabBar != nil {
		tabBar.RemoveTab(sourceTabID)
	}
	// Remove the tab from the derived global tab mirror after the per-window
	// TabList is modified.
	if a.tabs != nil {
		a.tabs.Remove(sourceTabID)
	}
}

func (a *App) ensureTargetTabUI(ctx context.Context, tab *entity.Tab, targetWindow *browserWindow) {
	if tab == nil {
		return
	}
	if tabBar := a.tabBarForBrowserWindow(targetWindow); tabBar != nil {
		tabBar.AddTab(tab)
	}
	if a.workspaceViews[tab.ID] == nil {
		if !a.createWorkspaceViewWithoutAttach(ctx, tab) {
			return
		}
	}
}

func (a *App) rebuildAndAttachWorkspace(ctx context.Context, tabID entity.TabID, tab *entity.Tab) {
	wsView := a.workspaceViews[tabID]
	if wsView == nil {
		return
	}
	_ = wsView.Rebuild(ctx)
	a.reattachFloatingSessions(tabID, wsView)
	a.syncFloatingFocus()

	if tab == nil || tab.Workspace == nil || a.contentCoord == nil {
		return
	}
	a.contentCoord.AttachToWorkspace(ctx, tab.Workspace, wsView)
	if a.wsCoord != nil {
		a.wsCoord.SetupStackedPaneCallbacks(ctx, tab.Workspace, wsView)
	}
}

func (a *App) reattachFloatingSessions(tabID entity.TabID, wsView *component.WorkspaceView) {
	if wsView == nil {
		return
	}

	for key, session := range a.floatingSessions {
		if key.tabID != tabID || session == nil || session.pane == nil {
			continue
		}

		session.pane.SetParentOverlay(wsView.WorkspaceOverlayWidget())
		if session.widget != nil {
			wsView.AddWorkspaceOverlayWidget(session.widget)
			configureFloatingOverlayMeasurement(wsView.WorkspaceOverlayWidget(), session.widget)
			setFloatingWidgetShown(session.widget, session.pane.IsVisible())
		}
		if session.paneView != nil {
			wsView.RegisterPaneView(session.paneID, session.paneView)
			if session.focusWidget == nil {
				session.focusWidget = session.paneView.WebViewWidget()
			}
		}
	}
}

func (a *App) switchToTargetTabIfConfigured(
	ctx context.Context,
	out *usecase.MovePaneToTabOutput,
	targetWindow *browserWindow,
) {
	if out == nil || out.TargetTab == nil {
		return
	}

	switchToTarget := false
	if a.deps != nil && a.deps.Config != nil {
		switchToTarget = a.deps.Config.Workspace.SwitchToTabOnMove
	}
	if out.SourceTabClosed {
		switchToTarget = true
	}
	if !switchToTarget {
		return
	}

	if targetWindow == nil {
		targetWindow = a.lastFocusedBrowserWindow()
	}
	if targetWindow != nil {
		a.ensureTabListForBrowserWindow(targetWindow).SetActive(out.TargetTab.ID)
	}
	// Sync derived global active state after the target window is resolved
	// and the per-window TabList has been updated.
	if a.tabs != nil && a.tabs.Find(out.TargetTab.ID) != nil {
		a.tabs.SetActive(out.TargetTab.ID)
	}
	if tabBar := a.tabBarForBrowserWindow(targetWindow); tabBar != nil {
		tabBar.SetActive(out.TargetTab.ID)
	}
	if targetWindow != nil {
		a.activateBrowserWindow(targetWindow)
	}
	a.switchWorkspaceView(ctx, out.TargetTab.ID)
}

func (a *App) initSnapshotService(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.deps == nil || a.deps.SnapshotServiceFactory == nil {
		log.Debug().Msg("snapshot service factory not available, skipping")
		return
	}

	intervalMs := 5000 // default
	if a.deps.Config != nil && a.deps.Config.Session.SnapshotIntervalMs > 0 {
		intervalMs = a.deps.Config.Session.SnapshotIntervalMs
	}

	a.snapshotService = a.deps.SnapshotServiceFactory(a, intervalMs)
	if a.snapshotService == nil {
		log.Warn().Msg("snapshot service factory returned nil")
		return
	}
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
		func(ctx context.Context, msg string, level component.ToastLevel) {
			a.showToastOnLastFocusedBrowserWindow(ctx, msg, level)
		},
		a.deps.Config.Update.EnableOnStartup,
		a.deps.Config.Update.AutoDownload,
	)

	// Start async update check
	a.updateCoord.CheckOnStartup(ctx)

	log.Debug().Msg("update coordinator initialized")
}

// GetSessionID implements port.WindowStateProvider.
func (a *App) GetSessionID() entity.SessionID {
	if a.deps == nil {
		return ""
	}
	return a.deps.CurrentSessionID
}

type windowSnapshotCapture struct {
	browserTabs         map[string]*entity.TabList
	windowOrder         []string
	globalTabs          *entity.TabList
	tabOwners           map[entity.TabID]string
	lastFocusedWindowID string
}

// GetWindowSnapshotState implements port.WindowStateProvider.
// Returns per-window tab lists and active window index from one main-thread capture.
func (a *App) GetWindowSnapshotState() ([]entity.WindowTabListState, int) {
	capture, ok := a.captureWindowSnapshotState()
	if !ok {
		return nil, -1
	}
	return buildWindowSnapshotState(capture)
}

func (a *App) captureWindowSnapshotState() (windowSnapshotCapture, bool) {
	var capture windowSnapshotCapture
	captureFn := func() {
		capture = a.captureWindowSnapshotStateOnMainThread()
	}
	if a.dispatchOnMainThread == nil {
		// nil dispatchOnMainThread is for single-threaded tests only;
		// production should marshal capture to GTK main thread.
		captureFn()
		return capture, true
	}

	result := a.dispatchOnMainThread("ui.snapshot_window_state", captureFn)
	if result.Completed() {
		return capture, true
	}

	ctx := context.Background()
	if a.deps != nil && a.deps.Ctx != nil {
		ctx = a.deps.Ctx
	}
	logging.FromContext(ctx).Warn().
		Dur("elapsed", result.Elapsed).
		Str("dispatch_status", string(result.Status)).
		Msg("ui: window state snapshot unavailable after main-thread dispatch did not complete")
	return windowSnapshotCapture{}, false
}

func (a *App) captureWindowSnapshotStateOnMainThread() windowSnapshotCapture {
	capture := windowSnapshotCapture{
		globalTabs: entity.NewTabList(),
	}
	if a.tabs != nil {
		capture.globalTabs = a.tabs.Snapshot()
	}

	capture.browserTabs = make(map[string]*entity.TabList, len(a.browserWindows))
	for id, bw := range a.browserWindows {
		if bw == nil {
			continue
		}
		if bw.tabs != nil {
			capture.browserTabs[id] = bw.tabs.Snapshot()
		} else {
			capture.browserTabs[id] = nil
		}
	}

	capture.windowOrder = append([]string(nil), a.browserWindowOrder...)
	capture.tabOwners = make(map[entity.TabID]string, len(a.windowForTab))
	for tabID, bw := range a.windowForTab {
		if bw != nil {
			capture.tabOwners[tabID] = bw.id
		}
	}
	capture.lastFocusedWindowID = a.lastFocusedWindowID
	return capture
}

func buildWindowSnapshotState(capture windowSnapshotCapture) ([]entity.WindowTabListState, int) {
	if len(capture.browserTabs) == 0 {
		return []entity.WindowTabListState{
			{WindowID: "", Tabs: capture.globalTabs},
		}, 0
	}

	windowIDs := windowOrderFrom(capture.browserTabs, capture.windowOrder)
	result := make([]entity.WindowTabListState, 0, len(windowIDs))
	activeWindowIndex := 0
	anyOwnedWindow := false
	for i, wid := range windowIDs {
		if wid == capture.lastFocusedWindowID {
			activeWindowIndex = i
		}

		tabs := capture.browserTabs[wid]
		if tabs == nil {
			tabs = tabsForWindowFromGlobalList(capture.globalTabs, capture.tabOwners, wid)
		}
		if tabs.Count() > 0 {
			anyOwnedWindow = true
		}

		result = append(result, entity.WindowTabListState{
			WindowID: entity.WindowID(wid),
			Tabs:     tabs,
		})
	}
	if !anyOwnedWindow && capture.globalTabs.Count() > 0 {
		// No window ownership populated; return the flat global list for
		// snapshot/export compatibility rather than snapshotting empty windows.
		return []entity.WindowTabListState{{WindowID: "", Tabs: capture.globalTabs}}, 0
	}

	return result, activeWindowIndex
}

func tabsForWindowFromGlobalList(globalTabs *entity.TabList, tabOwners map[entity.TabID]string, windowID string) *entity.TabList {
	tabs := entity.NewTabList()
	if globalTabs == nil {
		return tabs
	}
	for _, tab := range globalTabs.Tabs {
		if tabOwners[tab.ID] == windowID {
			tabs.Add(cloneTabForGlobalList(tab))
		}
	}

	// Derive active tab from the global list when no per-window TabList exists
	// only for snapshot/export compatibility with older flat snapshot state.
	globalActive := globalTabs.ActiveTabID
	if globalActive != "" && tabs.Find(globalActive) != nil {
		tabs.SetActive(globalActive)
	}
	return tabs
}

// GetWindowTabLists implements port.WindowStateProvider.
// Returns per-window tab lists in browser window registration order.
// Falls back to a single window with the shared tab list when no browser windows exist.
func (a *App) GetWindowTabLists() []entity.WindowTabListState {
	windows, _ := a.GetWindowSnapshotState()
	return windows
}

// GetActiveWindowIndex implements port.WindowStateProvider.
// Returns the index of the last focused window in registration order.
func (a *App) GetActiveWindowIndex() int {
	_, activeWindowIndex := a.GetWindowSnapshotState()
	return activeWindowIndex
}

// tabListForBrowserWindow returns the per-window TabList for a browser window.
// Returns nil when bw is nil or bw.tabs has not been initialized.
func (a *App) tabListForBrowserWindow(bw *browserWindow) *entity.TabList {
	if bw == nil {
		return nil
	}
	return bw.tabs
}

// ensureTabListForBrowserWindow returns the per-window TabList for a browser window,
// lazy-initializing bw.tabs when nil. Returns nil when bw is nil.
func (a *App) ensureTabListForBrowserWindow(bw *browserWindow) *entity.TabList {
	if bw == nil {
		return nil
	}
	if bw.tabs == nil {
		bw.tabs = entity.NewTabList()
	}
	return bw.tabs
}

// activeTabForBrowserWindow returns the active tab for a browser window.
// Returns nil when bw is nil or the window has no tabs.
func (a *App) activeTabForBrowserWindow(bw *browserWindow) *entity.Tab {
	tabs := a.tabListForBrowserWindow(bw)
	if tabs == nil {
		return nil
	}
	return tabs.ActiveTab()
}

// activeWorkspaceForBrowserWindow returns the active workspace for a browser window.
// Returns nil when bw is nil or the active tab has no workspace.
func (a *App) activeWorkspaceForBrowserWindow(bw *browserWindow) *entity.Workspace {
	tab := a.activeTabForBrowserWindow(bw)
	if tab == nil {
		return nil
	}
	return tab.Workspace
}

func (a *App) activeWorkspaceViewForBrowserWindow(bw *browserWindow) *component.WorkspaceView {
	tab := a.activeTabForBrowserWindow(bw)
	if tab == nil {
		return nil
	}
	return a.workspaceViews[tab.ID]
}

// tabTargetForBrowserWindow returns a coordinator.TabTarget scoped to the given browser window.
// It does not allocate a TabList; callers that mutate tabs should use
// ensureTabTargetForBrowserWindow instead.
func (a *App) tabTargetForBrowserWindow(bw *browserWindow) coordinator.TabTarget {
	if bw == nil {
		return coordinator.TabTarget{}
	}
	return coordinator.TabTarget{
		Tabs:       a.tabListForBrowserWindow(bw),
		MainWindow: bw.mainWindow,
	}
}

func (a *App) ensureTabTargetForBrowserWindow(bw *browserWindow) coordinator.TabTarget {
	if bw == nil {
		return coordinator.TabTarget{}
	}
	return coordinator.TabTarget{
		Tabs:       a.ensureTabListForBrowserWindow(bw),
		MainWindow: bw.mainWindow,
	}
}

// activePaneIDForBrowserWindow resolves the active pane ID for a browser window.
// A content-coordinator override wins so floating panes can receive navigation.
// Returns the empty string when bw is nil or the active tab has no workspace.
func (a *App) activePaneIDForBrowserWindow(bw *browserWindow) entity.PaneID {
	if a.contentCoord != nil && bw != nil {
		if paneID, ok := a.contentCoord.ActivePaneOverrideID(); ok && a.browserWindowForAnyPane(paneID) == bw {
			return paneID
		}
	}
	ws := a.activeWorkspaceForBrowserWindow(bw)
	if ws == nil {
		return ""
	}
	return ws.ActivePaneID
}

// activeWebViewForBrowserWindow resolves the active WebView for a browser window.
// Uses the active pane ID and contentCoord.GetWebView (public getter) — never
// contentCoord.ActiveWebView — so that stale global focus cannot be picked.
func (a *App) activeWebViewForBrowserWindow(bw *browserWindow) (entity.PaneID, port.WebView) {
	paneID := a.activePaneIDForBrowserWindow(bw)
	if paneID == "" {
		return "", nil
	}
	if a.contentCoord == nil {
		return paneID, nil
	}
	return paneID, a.contentCoord.GetWebView(paneID)
}

// omniboxNavigateForBrowserWindow returns an omnibox OnNavigate callback that routes
// navigation through navigate for the supplied browser window. The closure captures
// bw at creation time, so navigation targets the owning window even when another
// window is globally focused.
func omniboxNavigateForBrowserWindow(
	ctx context.Context,
	bw *browserWindow,
	navigate func(context.Context, *browserWindow, string) error,
) func(context.Context, string) error {
	return func(navCtx context.Context, url string) error {
		if navCtx == nil {
			navCtx = ctx
		}
		return navigate(navCtx, bw, url)
	}
}

// navigateFromBrowserWindow loads a URL in the active pane of the given browser window.
// Uses NavigationCoordinator.NavigateWebView for explicit target resolution.
func (a *App) navigateFromBrowserWindow(ctx context.Context, bw *browserWindow, rawURL string) error {
	if a.navCoord == nil {
		return fmt.Errorf("navigation coordinator not initialized")
	}
	paneID, wv := a.activeWebViewForBrowserWindow(bw)
	return a.navCoord.NavigateWebView(ctx, rawURL, paneID, wv)
}

// withBrowserWindowWebView resolves the active WebView for bw, checks that navCoord
// is initialized and wv is not nil, then runs fn with the WebView. Avoids nil bw
// panic in error formatting by falling back to empty window id.
func (a *App) withBrowserWindowWebView(_ context.Context, bw *browserWindow, fn func(wv port.WebView) error) error {
	if a.navCoord == nil {
		return fmt.Errorf("navigation coordinator not initialized")
	}
	_, wv := a.activeWebViewForBrowserWindow(bw)
	if wv == nil {
		windowID := ""
		if bw != nil {
			windowID = bw.id
		}
		return fmt.Errorf("no active webview for browser window %s", windowID)
	}
	return fn(wv)
}

// reloadBrowserWindow reloads the active page in the given browser window.
// When bypassCache is true, bypasses the cache (hard reload).
func (a *App) reloadBrowserWindow(ctx context.Context, bw *browserWindow, bypassCache bool) error {
	return a.withBrowserWindowWebView(ctx, bw, func(wv port.WebView) error {
		return a.navCoord.ReloadWebView(ctx, wv, bypassCache)
	})
}

// stopBrowserWindow stops loading in the active pane of the given browser window.
func (a *App) stopBrowserWindow(ctx context.Context, bw *browserWindow) error {
	return a.withBrowserWindowWebView(ctx, bw, func(wv port.WebView) error {
		return a.navCoord.StopWebView(ctx, wv)
	})
}

// goBackBrowserWindow navigates back in history for the active pane of the given browser window.
func (a *App) goBackBrowserWindow(ctx context.Context, bw *browserWindow) error {
	return a.withBrowserWindowWebView(ctx, bw, func(wv port.WebView) error {
		return a.navCoord.GoBackWebView(ctx, wv)
	})
}

// goForwardBrowserWindow navigates forward in history for the active pane of the given browser window.
func (a *App) goForwardBrowserWindow(ctx context.Context, bw *browserWindow) error {
	return a.withBrowserWindowWebView(ctx, bw, func(wv port.WebView) error {
		return a.navCoord.GoForwardWebView(ctx, wv)
	})
}

func (a *App) printBrowserWindow(ctx context.Context, bw *browserWindow) error {
	return a.withBrowserWindowWebView(ctx, bw, func(wv port.WebView) error {
		return a.navCoord.PrintWebView(ctx, wv)
	})
}

func (a *App) openDevToolsBrowserWindow(ctx context.Context, bw *browserWindow) error {
	return a.withBrowserWindowWebView(ctx, bw, func(wv port.WebView) error {
		return a.navCoord.OpenDevToolsWebView(ctx, wv)
	})
}

func (a *App) zoomBrowserWindow(ctx context.Context, bw *browserWindow, action string) error {
	if a.deps == nil || a.deps.ZoomUC == nil {
		logging.FromContext(ctx).Warn().Msg("zoom use case not available")
		return nil
	}

	paneID, wv := a.activeWebViewForBrowserWindow(bw)
	if wv == nil {
		logging.FromContext(ctx).Debug().Str("action", action).Msg("no active webview for zoom")
		return nil
	}

	zoomKey, err := usecase.ExtractZoomKey(wv.URI())
	if err != nil {
		logging.FromContext(ctx).Debug().Str("uri", wv.URI()).Msg("cannot extract zoom key")
		return nil
	}

	current := wv.GetZoomLevel()
	var newZoom *entity.ZoomLevel
	switch action {
	case "in":
		newZoom, err = a.deps.ZoomUC.ZoomIn(ctx, zoomKey, current)
	case "out":
		newZoom, err = a.deps.ZoomUC.ZoomOut(ctx, zoomKey, current)
	case "reset":
		err = a.deps.ZoomUC.ResetZoom(ctx, zoomKey)
		if err == nil {
			newZoom = entity.NewZoomLevel(zoomKey, a.deps.ZoomUC.DefaultZoom())
		}
	default:
		return fmt.Errorf("unknown zoom action %q", action)
	}
	if err != nil {
		return err
	}
	if newZoom == nil {
		return nil
	}
	if err := wv.SetZoomLevel(ctx, newZoom.ZoomFactor); err != nil {
		return err
	}
	if a.navCoord != nil {
		a.navCoord.NotifyZoomChanged(ctx, newZoom.ZoomFactor)
	}
	if wsView := a.activeWorkspaceViewForBrowserWindow(bw); wsView != nil {
		if paneView := wsView.GetPaneView(paneID); paneView != nil {
			paneView.ShowZoomToast(ctx, int(newZoom.ZoomFactor*100))
		}
	}
	return nil
}

const (
	tabSwitchIndex0 = iota // 0
	tabSwitchIndex1        // 1
	tabSwitchIndex2        // 2
	tabSwitchIndex3        // 3
	tabSwitchIndex4        // 4
	tabSwitchIndex5        // 5
	tabSwitchIndex6        // 6
	tabSwitchIndex7        // 7
	tabSwitchIndex8        // 8
	tabSwitchIndex9        // 9
)

// switchTabIndexActionIndex maps ActionSwitchTabIndex1..10 to 0..9.
func switchTabIndexActionIndex(action input.Action) (int, bool) {
	switch action {
	case input.ActionSwitchTabIndex1:
		return tabSwitchIndex0, true
	case input.ActionSwitchTabIndex2:
		return tabSwitchIndex1, true
	case input.ActionSwitchTabIndex3:
		return tabSwitchIndex2, true
	case input.ActionSwitchTabIndex4:
		return tabSwitchIndex3, true
	case input.ActionSwitchTabIndex5:
		return tabSwitchIndex4, true
	case input.ActionSwitchTabIndex6:
		return tabSwitchIndex5, true
	case input.ActionSwitchTabIndex7:
		return tabSwitchIndex6, true
	case input.ActionSwitchTabIndex8:
		return tabSwitchIndex7, true
	case input.ActionSwitchTabIndex9:
		return tabSwitchIndex8, true
	case input.ActionSwitchTabIndex10:
		return tabSwitchIndex9, true
	default:
		return 0, false
	}
}

// dispatchBrowserWindowAction routes keyboard/global shortcut actions through the
// source browser window helpers instead of falling back to the global active
// webview/tab. Browser-window-owning callbacks (keyboardHandler, globalShortcutHandler)
// must call this instead of activating the window then dispatching globally.
func (a *App) dispatchBrowserWindowAction(ctx context.Context, bw *browserWindow, action input.Action) error {
	switch action {
	case input.ActionReload:
		return a.reloadBrowserWindow(ctx, bw, false)
	case input.ActionHardReload:
		return a.reloadBrowserWindow(ctx, bw, true)
	case input.ActionStop:
		return a.stopBrowserWindow(ctx, bw)
	case input.ActionGoBack:
		return a.goBackBrowserWindow(ctx, bw)
	case input.ActionGoForward:
		return a.goForwardBrowserWindow(ctx, bw)
	case input.ActionPrintPage:
		return a.printBrowserWindow(ctx, bw)
	case input.ActionOpenDevTools:
		return a.openDevToolsBrowserWindow(ctx, bw)
	case input.ActionZoomIn:
		return a.zoomBrowserWindow(ctx, bw, "in")
	case input.ActionZoomOut:
		return a.zoomBrowserWindow(ctx, bw, "out")
	case input.ActionZoomReset:
		return a.zoomBrowserWindow(ctx, bw, "reset")
	case input.ActionSwitchTabIndex1, input.ActionSwitchTabIndex2, input.ActionSwitchTabIndex3,
		input.ActionSwitchTabIndex4, input.ActionSwitchTabIndex5, input.ActionSwitchTabIndex6,
		input.ActionSwitchTabIndex7, input.ActionSwitchTabIndex8, input.ActionSwitchTabIndex9,
		input.ActionSwitchTabIndex10:
		index, ok := switchTabIndexActionIndex(action)
		if !ok {
			return nil
		}
		return a.switchBrowserWindowTabIndex(ctx, bw, index)
	default:
		if a.kbDispatcher != nil {
			return a.kbDispatcher.Dispatch(ctx, action)
		}
		return nil
	}
}

// switchBrowserWindowTabIndex activates the given browser window and switches to
// the tab at the given 0-based index in that window.
//
// If the requested tab already exists, it switches to it. If the index is out
// of range, it creates exactly one new tab instead of backfilling all missing
// intermediate tab slots.
func (a *App) switchBrowserWindowTabIndex(ctx context.Context, bw *browserWindow, index int) error {
	if a.tabCoord == nil {
		logging.FromContext(ctx).Warn().Msg("switch tab index ignored: tab coordinator is nil")
		return nil
	}
	if bw == nil {
		logging.FromContext(ctx).Warn().Msg("switch tab index ignored: no browser window")
		return nil
	}
	if index < 0 {
		logging.FromContext(ctx).Debug().Int("index", index).Msg("switch tab index ignored: invalid negative index")
		return nil
	}
	target := a.tabTargetForBrowserWindow(bw)
	tabCount := 0
	if target.Tabs != nil {
		tabCount = target.Tabs.Count()
	}
	if index < tabCount {
		a.activateBrowserWindow(bw)
		return a.tabCoord.SwitchByIndex(ctx, target, index)
	}
	if a.deps == nil || a.deps.Config == nil || a.deps.Config.Workspace.NewPaneURL == "" {
		logging.FromContext(ctx).Warn().Msg("switch tab index ignored: new pane URL is not configured")
		return fmt.Errorf("newPaneURL is not configured")
	}
	a.activateBrowserWindow(bw)
	ensureTarget := a.ensureTabTargetForBrowserWindow(bw)
	_, err := a.tabCoord.Create(ctx, ensureTarget, urlutil.Normalize(a.deps.Config.Workspace.NewPaneURL))
	return err
}

// windowOrder returns window IDs in registration order.
// Falls back to sorted map keys when no registration order is recorded
// (e.g., in old tests or direct map manipulation).
func (a *App) windowOrder() []string {
	if len(a.browserWindowOrder) > 0 {
		// Deduplicate against browserWindows, skipping stale entries.
		seen := make(map[string]bool, len(a.browserWindowOrder))
		result := make([]string, 0, len(a.browserWindowOrder))
		for _, wid := range a.browserWindowOrder {
			if seen[wid] || a.browserWindows[wid] == nil {
				continue
			}
			seen[wid] = true
			result = append(result, wid)
		}
		// Include any live browserWindows not yet tracked in order
		// (inserted via direct map manipulation) deterministically.
		untracked := make([]string, 0)
		for id, bw := range a.browserWindows {
			if bw != nil && !seen[id] {
				untracked = append(untracked, id)
			}
		}
		sort.Strings(untracked)
		return append(result, untracked...)
	}

	// Fallback: sort map keys for deterministic output.
	windowIDs := make([]string, 0, len(a.browserWindows))
	for id := range a.browserWindows {
		windowIDs = append(windowIDs, id)
	}
	sort.Strings(windowIDs)
	return windowIDs
}

func windowOrderFrom(browserTabs map[string]*entity.TabList, browserWindowOrder []string) []string {
	if len(browserWindowOrder) > 0 {
		seen := make(map[string]bool, len(browserWindowOrder))
		result := make([]string, 0, len(browserWindowOrder))
		for _, wid := range browserWindowOrder {
			if seen[wid] {
				continue
			}
			if _, ok := browserTabs[wid]; !ok {
				continue
			}
			seen[wid] = true
			result = append(result, wid)
		}
		untracked := make([]string, 0)
		for id := range browserTabs {
			if !seen[id] {
				untracked = append(untracked, id)
			}
		}
		sort.Strings(untracked)
		return append(result, untracked...)
	}

	windowIDs := make([]string, 0, len(browserTabs))
	for id := range browserTabs {
		windowIDs = append(windowIDs, id)
	}
	sort.Strings(windowIDs)
	return windowIDs
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
			a.showToastOnLastFocusedBrowserWindow(ctx, "Session restore failed", component.ToastWarning)
		} else {
			log.Info().Str("session_id", a.deps.RestoreSessionID).Msg("session restored")
			// Show success toast for session restoration
			a.showToastOnLastFocusedBrowserWindow(ctx, "Session restored", component.ToastSuccess)
			return
		}
	}

	// Create an initial tab using coordinator.
	target := a.ensureTabTargetForBrowserWindow(a.lastFocusedBrowserWindow())
	if _, err := a.tabCoord.Create(ctx, target, a.initialWindowURL()); err != nil {
		log.Error().Err(err).Msg("failed to create initial tab")
	}
}

func (a *App) clearRuntimeWindowUI(runtimeWindows []*browserWindow) {
	for _, bw := range runtimeWindows {
		if bw == nil || bw.mainWindow == nil {
			continue
		}
		if tabBar := bw.mainWindow.TabBar(); tabBar != nil {
			if tabs := a.tabListForBrowserWindow(bw); tabs != nil {
				for _, tab := range tabs.Tabs {
					if tab != nil {
						tabBar.RemoveTab(tab.ID)
					}
				}
			}
		}
		bw.mainWindow.SetContent(nil)
	}
}

func (a *App) clearSessionRestoreUIState(ctx context.Context) {
	if a == nil {
		return
	}

	tabIDs := make(map[entity.TabID]struct{})
	for _, bw := range a.browserWindows {
		if tabs := a.tabListForBrowserWindow(bw); tabs != nil {
			for _, tab := range tabs.Tabs {
				if tab != nil {
					tabIDs[tab.ID] = struct{}{}
				}
			}
		}
	}
	for tabID := range a.workspaceViews {
		tabIDs[tabID] = struct{}{}
	}
	for tabID := range a.windowForTab {
		tabIDs[tabID] = struct{}{}
	}

	var tabBar *component.TabBar
	if a.mainWindow != nil {
		tabBar = a.mainWindow.TabBar()
	}
	for tabID := range tabIDs {
		if tabBar != nil {
			tabBar.RemoveTab(tabID)
		}
		a.releaseFloatingSessionsForTab(ctx, tabID)
		delete(a.workspaceViews, tabID)
		delete(a.windowForTab, tabID)
	}
	if a.mainWindow != nil {
		a.mainWindow.SetContent(nil)
	}
}

func (a *App) restoreSession(ctx context.Context, sessionID entity.SessionID) error {
	state, restoredWindows, err := a.loadRestoredWindowStates(ctx, sessionID)
	if err != nil {
		return err
	}

	logging.FromContext(ctx).Info().
		Int("windows", len(restoredWindows)).
		Int("panes", state.CountPanes()).
		Msg("restoring session state")

	if len(restoredWindows) == 0 {
		if state.Version >= entity.SessionStateVersion {
			// v2+ empty-window snapshots are valid: replace with a single empty window.
			restoredWindows = []entity.WindowTabListState{{
				WindowID: "",
				Tabs:     entity.NewTabList(),
			}}
		} else {
			return fmt.Errorf("restored session contains no windows")
		}
	}

	runtimeWindows, firstBWReused, err := a.prepareRuntimeWindows(ctx, len(restoredWindows))
	if err != nil {
		return err
	}
	a.clearSessionRestoreUIState(ctx)
	a.clearRuntimeWindowUI(runtimeWindows)
	a.assignRuntimeWindowTabLists(runtimeWindows, restoredWindows, firstBWReused)
	a.pruneStaleBrowserWindows(runtimeWindows)

	activeBW := runtimeWindows[safeWindowIndex(state.ActiveWindowIndex, len(runtimeWindows))]
	a.activateBrowserWindow(activeBW)
	a.replaceGlobalTabsFromRuntimeWindows(runtimeWindows, activeBW)
	a.buildRestoredWindowUI(ctx, runtimeWindows)
	a.showRestoredRuntimeWindows(runtimeWindows)

	return nil
}

func (a *App) loadRestoredWindowStates(
	ctx context.Context,
	sessionID entity.SessionID,
) (*entity.SessionState, []entity.WindowTabListState, error) {
	if a.deps == nil || a.deps.SessionStateRepo == nil {
		return nil, nil, fmt.Errorf("session state repo not available")
	}

	state, err := a.deps.SessionStateRepo.GetSnapshot(ctx, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("load session state: %w", err)
	}
	if state == nil {
		return nil, nil, fmt.Errorf("session state not found")
	}

	restoredWindows := entity.WindowTabListsFromSnapshot(state, a.generateID)
	return state, restoredWindows, nil
}

func (a *App) prepareRuntimeWindows(ctx context.Context, windowCount int) ([]*browserWindow, bool, error) {
	firstBW := a.browserWindowForMainWindow(a.mainWindow)
	if firstBW == nil {
		firstBW = a.lastFocusedBrowserWindow()
	}
	firstBWReused := firstBW != nil
	createdWindows := make([]*browserWindow, 0)

	if firstBW == nil {
		if a.mainWindow != nil {
			firstBW = a.wrapExistingMainWindowForRestore(ctx, a.mainWindow)
		} else {
			created, err := a.createBrowserWindow(ctx, "")
			if err != nil {
				return nil, false, fmt.Errorf("create browser window for restore: %w", err)
			}
			firstBW = created
			createdWindows = append(createdWindows, created)
		}
	}

	runtimeWindows := make([]*browserWindow, windowCount)
	runtimeWindows[0] = firstBW
	for i := 1; i < windowCount; i++ {
		extraBW, err := a.createBrowserWindow(ctx, "")
		if err != nil {
			a.cleanupCreatedBrowserWindows(createdWindows)
			return nil, false, fmt.Errorf("create browser window %d for restore: %w", i, err)
		}
		createdWindows = append(createdWindows, extraBW)
		runtimeWindows[i] = extraBW
	}

	return runtimeWindows, firstBWReused, nil
}

func (a *App) wrapExistingMainWindowForRestore(ctx context.Context, mainWindow *window.MainWindow) *browserWindow {
	bw := &browserWindow{id: a.generateWindowID(), tabs: entity.NewTabList(), mainWindow: mainWindow}
	a.wireBrowserWindowActivationTracking(bw)
	if a.kbDispatcher != nil {
		a.initBrowserWindowInput(ctx, bw)
	}
	if a.tabCoord != nil {
		a.wireBrowserWindowTabBar(ctx, bw)
	}
	bw.initChrome(ctx, a)
	return bw
}

func (a *App) cleanupCreatedBrowserWindows(createdWindows []*browserWindow) {
	for _, bw := range createdWindows {
		if bw == nil {
			continue
		}
		mainWindow := bw.mainWindow
		a.removeBrowserWindow(bw.id)
		if mainWindow != nil {
			if a.mainWindow == mainWindow {
				a.mainWindow = nil
			}
			mainWindow.Destroy()
		}
	}
}

func (a *App) assignRuntimeWindowTabLists(
	runtimeWindows []*browserWindow,
	restoredWindows []entity.WindowTabListState,
	firstBWReused bool,
) {
	for i, bw := range runtimeWindows {
		if i > 0 || !firstBWReused {
			a.registerBrowserWindow(bw)
		}
		a.assignRestoredWindowTabList(bw, restoredWindows[i])
	}
}

func (a *App) pruneStaleBrowserWindows(runtimeWindows []*browserWindow) {
	runtimeSet := make(map[string]bool, len(runtimeWindows))
	for _, bw := range runtimeWindows {
		runtimeSet[bw.id] = true
	}
	for id, bw := range a.browserWindows {
		if !runtimeSet[id] {
			if bw != nil && bw.mainWindow != nil {
				bw.mainWindow.Destroy()
			}
			a.removeBrowserWindow(id)
		}
	}

	a.browserWindowOrder = make([]string, len(runtimeWindows))
	for i, bw := range runtimeWindows {
		a.browserWindowOrder[i] = bw.id
	}
}

func safeWindowIndex(index, count int) int {
	if index < 0 || index >= count {
		return 0
	}
	return index
}

func (a *App) syncDerivedGlobalTabMirror(tab *entity.Tab) {
	if a.tabs != nil && tab != nil && a.tabs.Find(tab.ID) == nil {
		a.tabs.Add(cloneTabForGlobalList(tab))
	}
}

// workspace pointer is intentionally shared because App.tabs is a derived
// snapshot/export mirror, not runtime command target; runtime mutations must
// target browserWindow tabs.
func cloneTabForGlobalList(tab *entity.Tab) *entity.Tab {
	if tab == nil {
		return nil
	}
	return &entity.Tab{
		ID:        tab.ID,
		Name:      tab.Name,
		Workspace: tab.Workspace,
		IsPinned:  tab.IsPinned,
		CreatedAt: tab.CreatedAt,
	}
}

func (a *App) replaceGlobalTabsFromRuntimeWindows(runtimeWindows []*browserWindow, activeBW *browserWindow) {
	globalTabs := entity.NewTabList()
	for _, bw := range runtimeWindows {
		perWinTabs := a.tabListForBrowserWindow(bw)
		if perWinTabs == nil {
			continue
		}
		for _, tab := range perWinTabs.Tabs {
			if cloned := cloneTabForGlobalList(tab); cloned != nil {
				globalTabs.Add(cloned)
			}
		}
	}

	if activeTabs := a.tabListForBrowserWindow(activeBW); activeTabs != nil {
		activeID := activeTabs.ActiveTabID
		if activeID != "" && globalTabs.Find(activeID) != nil {
			globalTabs.SetActive(activeID)
		}
	}
	a.tabs.ReplaceFrom(globalTabs)
}

func (a *App) showRestoredRuntimeWindows(runtimeWindows []*browserWindow) {
	for _, bw := range runtimeWindows {
		if bw != nil && bw.mainWindow != nil {
			bw.mainWindow.Show()
		}
	}
}

func (a *App) buildRestoredWindowUI(ctx context.Context, runtimeWindows []*browserWindow) {
	for _, bw := range runtimeWindows {
		perWinTabs := a.tabListForBrowserWindow(bw)
		if perWinTabs == nil || perWinTabs.Count() == 0 {
			a.updateBrowserWindowTabBarVisibility(bw)
			continue
		}

		tabBar := bw.mainWindow.TabBar()
		activeTab := perWinTabs.ActiveTab()
		for _, tab := range perWinTabs.Tabs {
			a.buildRestoredTabUI(ctx, bw, tabBar, tab)
		}

		if activeTab != nil {
			a.switchWorkspaceView(ctx, activeTab.ID)
			if tabBar != nil {
				tabBar.SetActive(activeTab.ID)
			}
		}
		a.updateBrowserWindowTabBarVisibility(bw)
	}
}

func (a *App) buildRestoredTabUI(ctx context.Context, bw *browserWindow, tabBar *component.TabBar, tab *entity.Tab) {
	if tab == nil {
		return
	}
	if !a.createWorkspaceViewWithoutAttach(ctx, tab) {
		return
	}
	wsView := a.workspaceViews[tab.ID]
	if a.wsCoord != nil && wsView != nil {
		a.wsCoord.SetupStackedPaneCallbacks(ctx, tab.Workspace, wsView)
	}
	if tabBar != nil {
		tabBar.AddTab(tab)
	}
	logging.FromContext(ctx).Debug().
		Str("tab_id", string(tab.ID)).
		Str("name", tab.Name).
		Str("window_id", bw.id).
		Int("panes", tab.Workspace.PaneCount()).
		Msg("restored tab with workspace")
}

// assignRestoredWindowTabList assigns a restored WindowTabListState to a runtime browser window.
// It sets bw.tabs to the restored TabList (or a new empty one if nil) and registers each
// tab's ownership via windowForTab. It never inserts saved window IDs into browserWindows;
// the runtime window ID is owned by the caller.
func (a *App) assignRestoredWindowTabList(bw *browserWindow, restored entity.WindowTabListState) {
	if bw == nil {
		return
	}

	a.ensureWindowForTabMap()

	if restored.Tabs == nil {
		bw.tabs = entity.NewTabList()
		return
	}

	bw.tabs = restored.Tabs
	for _, tab := range restored.Tabs.Tabs {
		if tab != nil {
			a.windowForTab[tab.ID] = bw
		}
	}
}

func (a *App) finalizeActivation(ctx context.Context) {
	log := logging.FromContext(ctx)

	// Show the window
	if a.mainWindow != nil {
		a.mainWindow.Show()
	}
	a.startBrowserLaunchRelayListener(ctx)
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
		msg := fmt.Sprintf("Detected %d unexpected close report(s). Run: dumber crashes issue latest", len(paths))
		a.showToastOnLastFocusedBrowserWindow(ctx, msg, component.ToastWarning,
			component.WithDuration(crashReportToastDurationMs),
			component.WithPosition(component.ToastPositionBottomRight),
		)
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

// triggerDeferredInit is called from content.Coordinator on first load_started.
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

	// Stop accepting relaunches before teardown starts.
	a.closeBrowserLaunchRelayListener()

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
	if len(a.nativePopupWindows) > 0 {
		popupIDs := make([]port.WebViewID, 0, len(a.nativePopupWindows))
		for popupID := range a.nativePopupWindows {
			popupIDs = append(popupIDs, popupID)
		}
		for _, popupID := range popupIDs {
			a.releaseNativePopupWindow(popupID, false, true)
		}
	}
	if a.engine != nil {
		if err := a.engine.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close engine")
		}
	}
	// Close idle inhibitor to release D-Bus connection
	if a.deps.IdleInhibitor != nil {
		if err := a.deps.IdleInhibitor.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close idle inhibitor")
		}
	}

	log.Info().Msg("application shutdown complete")
}

// initContentCoordinator creates the content coordinator and wires its optional dependencies.
func (a *App) initContentCoordinator(
	ctx context.Context,
	getActiveWS func() (*entity.Workspace, *component.WorkspaceView),
) {
	a.contentCoord = content.NewCoordinator(
		ctx,
		a.engine.Pool(),
		a.engine.ContentInjector(),
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

	// Wire engine settings and filter appliers for hot-reload and late-binding filters
	if sa := a.deps.Engine.SettingsApplier(); sa != nil {
		a.contentCoord.SetSettingsApplier(sa)
	}
	if fa := a.deps.Engine.FilterApplier(); fa != nil {
		a.contentCoord.SetFilterApplier(fa)
	}

	// Wire external URL launcher (e.g. xdg-open for vscode://, spotify://)
	if a.deps.LaunchExternalURL != nil {
		a.contentCoord.SetOnLaunchExternalURL(a.deps.LaunchExternalURL)
	}

	// Wire deferred init trigger - runs after first navigation starts
	a.contentCoord.SetOnFirstLoadStarted(func() {
		a.triggerDeferredInit(ctx)
	})
}

// initTabCoordinator creates the TabCoordinator and wires all its callbacks.
func (a *App) initTabCoordinator(ctx context.Context) {
	a.tabCoord = coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{
		TabsUC:                  a.tabsUC,
		MainWindow:              a.mainWindow,
		HideTabBarWhenSingleTab: a.deps.Config.Workspace.HideTabBarWhenSingleTab,
	})
	a.tabCoord.SetOnTabCreated(func(ctx context.Context, target coordinator.TabTarget, tab *entity.Tab) {
		// Assign ownership from the callback target, not focus/global state.
		bw := a.browserWindowForTabTarget(target)
		if bw != nil {
			a.setBrowserWindowForTab(tab.ID, bw)
		}
		// Set a window-scoped default title so "Tab N" doesn't use global position.
		// Count only live tabs owned by this window via the helper.
		// Only assign a default name if the creator did not supply one.
		if strings.TrimSpace(tab.Name) == "" {
			count := a.tabCountForBrowserWindow(bw)
			if count <= 0 {
				count = 1
			}
			tab.Name = defaultTabName(count)
		}
		// Keep the derived global App.tabs mirror populated from per-window
		// tab creation for usecase and snapshot/export boundaries.
		// This must happen AFTER default tab name assignment so the clone
		// has the final window-scoped default name.
		if a.tabs != nil && a.tabs.Find(tab.ID) == nil {
			a.tabs.Add(cloneTabForGlobalList(tab))
		}
		a.createWorkspaceView(ctx, tab)
	})
	a.tabCoord.SetOnTabSwitched(func(ctx context.Context, target coordinator.TabTarget, tab *entity.Tab) {
		// TabList.SetActive (called by TabCoordinator.Switch → tabsUC.Switch) already
		// tracks ActiveTabID and PreviousActiveTabID on bw.tabs. No manual bw state needed.
		if bw := a.browserWindowForTabTarget(target); bw != nil {
			a.activateBrowserWindow(bw)
		}
		a.switchWorkspaceView(ctx, tab.ID)
	})
	a.tabCoord.SetOnStateChanged(a.MarkDirty)
	// Per-window tab ownership is now enforced through explicit TabTarget.
	// The per-window TabList (bw.tabs) is the source of truth; no scope filtering needed.
	a.tabCoord.SetOnCurrentWindowEmpty(func(ctx context.Context, target coordinator.TabTarget) {
		bw := a.browserWindowForTabTarget(target)
		if bw != nil {
			a.removeBrowserWindow(bw.id)
			if bw.mainWindow != nil {
				bw.mainWindow.Destroy()
			}
		}
		// Quit the app only when all browser windows are gone.
		if len(a.browserWindows) == 0 {
			a.Quit()
		}
	})
	// Wire popup tab WebView attachment
	a.tabCoord.SetOnAttachPopupToTab(func(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv port.WebView) {
		a.attachPopupToTab(ctx, tabID, pane, wv)
	})

	for _, bw := range a.browserWindows {
		a.wireBrowserWindowTabBar(ctx, bw)
	}
}

// initCoordinators initializes all coordinators and wires their callbacks.
func (a *App) initCoordinators(ctx context.Context) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("initializing coordinators")

	// Coordinators resolve the workspace and view through
	// lastFocusedBrowserWindow via activeWorkspace helpers.
	// Shared coordinator instances defer to window-scoped resolution;
	// the startup edge case (no focused window yet) is handled inside
	// activeWorkspace/activeWorkspaceView.
	getActiveWS := func() (*entity.Workspace, *component.WorkspaceView) {
		return a.activeWorkspace(), a.activeWorkspaceView()
	}

	// Create FaviconAdapter with resolver/service and engine FaviconDatabase.
	// Skip if favicon support is not wired (e.g. in tests or when disabled).
	if a.deps.FaviconService != nil || a.deps.FaviconResolver != nil {
		a.faviconAdapter = adapter.NewFaviconAdapterWithResolver(
			a.deps.FaviconService,
			a.deps.FaviconResolver,
			a.engine.FaviconDatabase(),
			a.deps.FaviconAdapterConfig,
		)
		registerFaviconInvalidator(a.deps.FaviconResolver, a.faviconAdapter)
		if invalidator, ok := a.engine.(port.FaviconInvalidator); ok {
			registerFaviconInvalidator(a.deps.FaviconResolver, invalidator)
		}
	}

	// 1. Content Coordinator (no dependencies on other coordinators)
	a.initContentCoordinator(ctx, getActiveWS)

	// 2. Tab Coordinator
	a.initTabCoordinator(ctx)

	// Set fullscreen callback to hide/show tab bar (after tabCoord is initialized)
	a.contentCoord.SetOnFullscreenChanged(func(paneID entity.PaneID, entering bool) {
		a.handlePaneFullscreenChanged(paneID, entering)
	})

	// 3. Workspace Coordinator
	a.wsCoord = coordinator.NewWorkspaceCoordinator(ctx, coordinator.WorkspaceCoordinatorConfig{
		PanesUC:              a.panesUC,
		FocusMgr:             a.focusMgr,
		StackedPaneMgr:       a.stackedPaneMgr,
		WidgetFactory:        a.widgetFactory,
		ContentCoord:         a.contentCoord,
		GetActiveWS:          getActiveWS,
		GenerateID:           a.generateID,
		NewPaneURL:           a.deps.Config.Workspace.NewPaneURL,
		ResizeStepPercent:    a.deps.Config.Workspace.ResizeMode.StepPercent,
		ResizeMinPanePercent: a.deps.Config.Workspace.ResizeMode.MinPanePercent,
	})
	a.wsCoord.SetOnCloseLastPane(func(ctx context.Context) error {
		bw := a.lastFocusedBrowserWindow()
		return a.tabCoord.Close(ctx, a.ensureTabTargetForBrowserWindow(bw))
	})
	a.wsCoord.SetOnStateChanged(a.MarkDirty)

	// Wire popup handling
	// Set theme background color on the engine's popup factory to eliminate white flash.
	if a.engine != nil && a.deps.Theme != nil {
		r, g, b, alpha := a.deps.Theme.GetBackgroundRGBA()
		_ = a.engine.UpdateAppearance(ctx, float64(r), float64(g), float64(b), float64(alpha))
	}
	a.contentCoord.SetPopupConfig(
		a.engine.Factory(),
		&a.deps.Config.Workspace.BrowsingContexts,
		a.generateID,
	)
	a.contentCoord.SetPopupWindowIDResolver(func(paneID entity.PaneID) (string, bool) {
		bw := a.browserWindowForAnyPane(paneID)
		if bw == nil {
			return "", false
		}
		return bw.id, true
	})
	a.contentCoord.SetOnInsertPopup(func(ctx context.Context, input content.InsertPopupInput) error {
		if bw := a.browserWindowForAnyPane(input.ParentPaneID); bw != nil {
			a.activateBrowserWindow(bw)
		}
		return a.wsCoord.InsertPopup(ctx, input)
	})
	a.contentCoord.SetOnClosePane(func(ctx context.Context, paneID entity.PaneID) error {
		if bw := a.browserWindowForAnyPane(paneID); bw != nil {
			a.activateBrowserWindow(bw)
		}
		return a.wsCoord.ClosePaneByID(ctx, paneID)
	})
	a.contentCoord.SetOnOpenNativePopup(a.openNativePopupWindow)
	// Wire tabbed popup behavior to create new tabs in the originating window.
	a.wsCoord.SetOnCreatePopupTab(a.createPopupTab)

	// Move pane use cases (cross-tab/cross-window)
	a.movePaneToTabUC = usecase.NewMovePaneToTabUseCase(a.generateID)
	a.extractPaneToTabListUC = usecase.NewExtractPaneToTabListUseCase(a.generateID)

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
	a.contentCoord.SetOnWindowTitleChanged(func(paneID entity.PaneID, title string) {
		a.handlePaneWindowTitleChanged(paneID, title)
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

		// This only updates focus when the shown pane belongs to the last-focused
		// window's active workspace. activeWorkspace() delegates to
		// lastFocusedBrowserWindow, so the accent-picker does not activate for
		// background windows.
		ws := a.activeWorkspace()
		if ws != nil && ws.ActivePaneID == paneID && a.accentFocusProvider != nil {
			a.accentFocusProvider.SetFocusedInput(a.getActiveWebViewTarget())
		}
	})

	// 5. Keyboard Dispatcher
	// Dispatcher receives an app-level active pane provider (contentCoord.ActivePaneID)
	// and per-window handlers route source-window actions through
	// dispatchBrowserWindowAction for proper multi-window isolation.
	a.kbDispatcher = dispatcher.NewKeyboardDispatcher(
		ctx,
		a.wsCoord,
		a.navCoord,
		a.deps.ZoomUC,
		a.deps.CopyURLUC,
		a.keyboardActions(),
		a.contentCoord.ActivePaneID,
	)
	a.wireKeyboardActions()
	for _, bw := range a.browserWindows {
		a.initBrowserWindowInput(ctx, bw)
	}

	log.Debug().Msg("coordinators initialized")
}

func (a *App) keyboardActions() dispatcher.KeyboardActions {
	return dispatcher.KeyboardActions{
		NewTab: func(ctx context.Context) error {
			if a.deps == nil || a.deps.Config == nil || a.deps.Config.Workspace.NewPaneURL == "" {
				logging.FromContext(ctx).Warn().Msg("new tab ignored: new pane URL is not configured")
				return fmt.Errorf("newPaneURL is not configured")
			}
			return a.withFocusedTabTarget(ctx, "new tab", true, func(target coordinator.TabTarget) error {
				_, err := a.tabCoord.Create(ctx, target, urlutil.Normalize(a.deps.Config.Workspace.NewPaneURL))
				return err
			})
		},
		CloseTab: func(ctx context.Context) error {
			return a.withFocusedTabTarget(ctx, "close tab", false, func(target coordinator.TabTarget) error {
				return a.tabCoord.Close(ctx, target)
			})
		},
		NextTab: func(ctx context.Context) error {
			return a.withFocusedTabTarget(ctx, "next tab", false, func(target coordinator.TabTarget) error {
				return a.tabCoord.SwitchNext(ctx, target)
			})
		},
		PreviousTab: func(ctx context.Context) error {
			return a.withFocusedTabTarget(ctx, "previous tab", false, func(target coordinator.TabTarget) error {
				return a.tabCoord.SwitchPrev(ctx, target)
			})
		},
		SwitchLastTab: func(ctx context.Context) error {
			return a.withFocusedTabTarget(ctx, "switch last tab", false, func(target coordinator.TabTarget) error {
				return a.tabCoord.SwitchToLastActive(ctx, target)
			})
		},
		SwitchTabIndex: func(ctx context.Context, index int) error {
			return a.switchBrowserWindowTabIndex(ctx, a.lastFocusedBrowserWindow(), index)
		},
		ActiveWebView: func(context.Context) port.WebView {
			_, wv := a.activeWebViewForBrowserWindow(a.lastFocusedBrowserWindow())
			return wv
		},
	}
}

func (a *App) withFocusedTabTarget(ctx context.Context, action string, ensure bool, fn func(coordinator.TabTarget) error) error {
	if a.tabCoord == nil {
		logging.FromContext(ctx).Warn().Str("action", action).Msg("tab action ignored: tab coordinator is nil")
		return nil
	}
	bw := a.lastFocusedBrowserWindow()
	if bw == nil {
		logging.FromContext(ctx).Warn().Str("action", action).Msg("tab action ignored: no focused browser window")
		return nil
	}
	target := a.tabTargetForBrowserWindow(bw)
	if ensure {
		target = a.ensureTabTargetForBrowserWindow(bw)
	}
	return fn(target)
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
	a.kbDispatcher.SetOnMovePaneToTab(func(ctx context.Context, paneID entity.PaneID) error {
		if bw := a.ownerOrLastFocusedBrowserWindow("", paneID); bw != nil {
			a.activateBrowserWindow(bw)
		}
		return a.HandleMovePaneToTab(ctx)
	})
	a.kbDispatcher.SetOnMovePaneToNextTab(func(ctx context.Context, paneID entity.PaneID) error {
		if bw := a.ownerOrLastFocusedBrowserWindow("", paneID); bw != nil {
			a.activateBrowserWindow(bw)
		}
		return a.HandleMovePaneToNextTab(ctx)
	})
	a.kbDispatcher.SetOnEjectPaneToWindow(func(ctx context.Context, paneID entity.PaneID) error {
		if bw := a.ownerOrLastFocusedBrowserWindow("", paneID); bw != nil {
			a.activateBrowserWindow(bw)
		}
		return a.EjectActivePaneToWindow(ctx, paneID)
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
	if a.contentCoord == nil {
		return
	}
	ctx := context.Background()
	if a.deps != nil && a.deps.Ctx != nil {
		ctx = a.deps.Ctx
	}

	a.contentCoord.SetOnPermissionActivity(func(
		paneID entity.PaneID,
		origin string,
		permTypes []entity.PermissionType,
		state content.PermissionActivityState,
	) {
		bw := a.browserWindowForPane(paneID)
		if bw == nil || bw.webrtcIndicator == nil {
			return
		}
		if state == content.PermissionActivityRequesting && a.deps != nil && a.deps.PermissionUC != nil && bw.permissionDialog != nil {
			a.deps.PermissionUC.SetDialogPresenter(bw.permissionDialog)
		}
		bw.webrtcIndicator.SetOrigin(origin)

		switch state {
		case content.PermissionActivityRequesting:
			bw.webrtcIndicator.MarkRequesting(permTypes)
		case content.PermissionActivityAllowed:
			bw.webrtcIndicator.MarkAllowed(permTypes)
		case content.PermissionActivityBlocked:
			bw.webrtcIndicator.MarkBlocked(permTypes)
		}

		for _, permType := range permTypes {
			a.syncWebRTCPermissionLockState(ctx, bw.webrtcIndicator, origin, permType)
		}
	})

	// Reset the owning window's indicator when that pane navigates away.
	a.contentCoord.SetOnActiveNavigationCommitted(func(paneID entity.PaneID, uri string) {
		bw := a.browserWindowForPane(paneID)
		if bw == nil || bw.webrtcIndicator == nil {
			return
		}
		newOrigin, err := urlutil.ExtractOrigin(uri)
		if err != nil {
			bw.webrtcIndicator.Reset()
			return
		}

		currentOrigin := bw.webrtcIndicator.Origin()
		if currentOrigin != "" && currentOrigin != newOrigin {
			bw.webrtcIndicator.Reset()
		}
	})
}

func (a *App) wireBrowserWindowPermissionIndicator(bw *browserWindow) {
	if bw == nil || bw.webrtcIndicator == nil {
		return
	}
	ctx := context.Background()
	if a.deps != nil && a.deps.Ctx != nil {
		ctx = a.deps.Ctx
	}
	log := logging.FromContext(ctx)

	bw.webrtcIndicator.SetOnToggleLock(func(origin string, permType entity.PermissionType, state string, hasStored bool) {
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
			if state == string(content.PermissionActivityBlocked) {
				decision = entity.PermissionGranted
			}
			if err := a.deps.PermissionUC.SetManualPermissionDecision(ctx, origin, permType, decision); err != nil {
				log.Warn().Err(err).Str("origin", origin).Str("type", string(permType)).Msg("failed to set manual permission decision")
				return
			}
		}

		a.syncWebRTCPermissionLockState(ctx, bw.webrtcIndicator, origin, permType)
	})
}

func (a *App) syncWebRTCPermissionLockState(
	ctx context.Context,
	indicator *component.WebRTCPermissionIndicator,
	origin string,
	permType entity.PermissionType,
) {
	if indicator == nil || a.deps == nil || a.deps.PermissionUC == nil || origin == "" {
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
		indicator.SetStoredDecision(permType, entity.PermissionPrompt, false)
		return
	}

	indicator.SetStoredDecision(permType, record.Decision, true)
}

// generateWindowID generates a unique ID for top-level browser windows.
func (a *App) generateWindowID() string {
	a.windowIDMu.Lock()
	defer a.windowIDMu.Unlock()
	a.windowIDCounter++
	return fmt.Sprintf("w%d", a.windowIDCounter)
}

const (
	uiMainThreadDispatchTimeout       = 2 * time.Second
	uiMainThreadDispatchSlowThreshold = 250 * time.Millisecond
)

// runOnMainThread executes fn inline when already on the GTK main context;
// otherwise it dispatches fn to the GTK main loop and waits for bounded
// completion so IPC and CEF callback paths do not wait forever on a wedged UI.
func (a *App) runOnMainThread(label string, fn func()) syncdispatch.SyncDispatchResult {
	result := syncdispatch.RunSynchronousDispatch(syncdispatch.SyncDispatchOptions{
		Label:   label,
		Timeout: uiMainThreadDispatchTimeout,
		IsOwner: func() bool {
			glibCtx := glib.MainContextDefault()
			return glibCtx != nil && glibCtx.IsOwner()
		},
		Dispatch: func(cb func()) {
			wrapped := new(glib.SourceOnceFunc)
			*wrapped = func(_ uintptr) { cb() }
			glib.IdleAddOnce(wrapped, 0)
		},
	}, fn)
	a.logMainThreadDispatchResult(result)
	return result
}

func (a *App) logMainThreadDispatchResult(result syncdispatch.SyncDispatchResult) {
	ctx := context.Background()
	if a != nil && a.deps != nil && a.deps.Ctx != nil {
		ctx = a.deps.Ctx
	}
	logger := logging.FromContext(ctx)
	switch result.Status {
	case syncdispatch.SyncDispatchTimedOut:
		logger.Warn().
			Str("dispatch_label", result.Label).
			Dur("elapsed", result.Elapsed).
			Dur("timeout", uiMainThreadDispatchTimeout).
			Msg("ui: main-thread dispatch timed out before callback started")
	case syncdispatch.SyncDispatchCompletedAfterTimeout:
		logger.Warn().
			Str("dispatch_label", result.Label).
			Dur("elapsed", result.Elapsed).
			Dur("timeout", uiMainThreadDispatchTimeout).
			Msg("ui: main-thread dispatch completed after timeout")
	case syncdispatch.SyncDispatchQueuedAfterTimeout:
		logger.Warn().
			Str("dispatch_label", result.Label).
			Dur("elapsed", result.Elapsed).
			Dur("timeout", uiMainThreadDispatchTimeout).
			Msg("ui: main-thread dispatch left queued after timeout")
	case syncdispatch.SyncDispatchCompleted:
		if result.Elapsed >= uiMainThreadDispatchSlowThreshold {
			logger.Debug().
				Str("dispatch_label", result.Label).
				Dur("elapsed", result.Elapsed).
				Msg("ui: main-thread dispatch completed slowly")
		}
	}
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
	if a == nil || a.gtkApp == nil {
		return
	}
	quit := func() {
		if a.gtkApp != nil {
			a.gtkApp.Quit()
		}
	}
	glibCtx := glib.MainContextDefault()
	if glibCtx != nil && glibCtx.IsOwner() {
		quit()
		return
	}
	cb := glib.SourceOnceFunc(func(_ uintptr) {
		quit()
	})
	glib.IdleAddOnce(&cb, 0)
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
func (a *App) updateWindowTitle(pageTitle string, target *browserWindow) {
	if target == nil {
		target = a.browserWindowForMainWindow(a.mainWindow)
	}
	if target == nil || target.mainWindow == nil {
		return
	}

	title := appTitle
	if pageTitle != "" {
		title = pageTitle + " - " + appTitle
	}
	target.mainWindow.SetTitle(title)
}

// updateWindowTitleFromActivePane updates the window title based on the current active pane.
func (a *App) updateWindowTitleFromActivePane(tabID entity.TabID) {
	var ws *entity.Workspace
	bw := a.browserWindowForTab(tabID)
	if tabs := a.tabListForBrowserWindow(bw); tabs != nil {
		if tab := tabs.Find(tabID); tab != nil {
			ws = tab.Workspace
		}
	}
	if ws == nil {
		ws = a.activeWorkspace()
	}
	if ws == nil || a.contentCoord == nil {
		a.updateWindowTitle("", a.browserWindowForTab(tabID))
		return
	}
	title := a.contentCoord.GetTitle(ws.ActivePaneID)
	a.updateWindowTitle(title, a.ownerOrLastFocusedBrowserWindow(tabID, ""))
}

// handleModeChange is called when the input mode changes.
func (a *App) handleModeChange(ctx context.Context, from, to input.Mode) {
	log := logging.FromContext(ctx)
	log.Debug().Str("from", from.String()).Str("to", to.String()).Msg("input mode changed")

	if from == input.ModeResize && to != input.ModeResize {
		a.clearResizeModeBorder()
	}
	if to == input.ModeResize {
		// Resize mode targets the last-focused browser window's active workspace.
		a.applyResizeModeBorder(ctx, a.activeWorkspace())
	}

	// Update global border overlay visibility based on mode.
	// Note: resize mode border is handled per-pane (stack container), not via global overlay.
	if bw := a.lastFocusedBrowserWindow(); bw != nil && bw.borderMgr != nil {
		bw.borderMgr.OnModeChange(ctx, from, to)
	}

	// Show/hide mode indicator toaster based on config.
	a.updateModeIndicatorToaster(ctx, to)
}

// updateModeIndicatorToaster shows or hides the mode indicator toaster based on mode and config.
func (a *App) updateModeIndicatorToaster(ctx context.Context, mode input.Mode) {
	bw := a.lastFocusedBrowserWindow()
	if bw == nil || bw.modeToaster == nil {
		return
	}

	// Check if mode indicator toaster is enabled in config.
	if a.deps == nil || a.deps.Config == nil || !a.deps.Config.Workspace.Styling.ModeIndicatorToasterEnabled {
		bw.modeToaster.Hide()
		return
	}

	if mode == input.ModeNormal {
		bw.modeToaster.Hide()
		return
	}

	// Show persistent toaster at bottom-left with mode display name.
	// Mode class is applied atomically with Show() to avoid visual flicker.
	modeClass := getModeToastClass(mode)
	bw.modeToaster.Show(ctx, mode.DisplayName(), component.ToastInfo,
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
	// activeWorkspaceView resolves via lastFocusedBrowserWindow; resize mode
	// always operates on the last-focused window.
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
	if !a.createWorkspaceViewWithoutAttach(ctx, tab) {
		return
	}

	target := a.browserWindowForTab(tab.ID)
	if target != nil {
		a.setBrowserWindowForTab(tab.ID, target)
	}

	// Attach to content area
	wsView := a.workspaceViews[tab.ID]
	if wsView != nil && target != nil && target.mainWindow != nil {
		widget := wsView.Widget()
		if widget != nil {
			gtkWidget := widget.GtkWidget()
			if gtkWidget != nil {
				target.mainWindow.SetContent(gtkWidget)
			}
		}
	}
}

// createWorkspaceViewWithoutAttach creates a WorkspaceView for a tab without attaching to content area.
// Used during session restoration where we create all views first, then attach only the active one.
func (a *App) createWorkspaceViewWithoutAttach(ctx context.Context, tab *entity.Tab) bool {
	if a.workspaceViewCreateOverride != nil {
		return a.workspaceViewCreateOverride(ctx, tab)
	}

	log := logging.FromContext(ctx)

	if a.widgetFactory == nil {
		log.Error().Msg("widget factory not initialized")
		return false
	}

	// Create workspace view
	wsView := component.NewWorkspaceView(ctx, a.widgetFactory)
	if wsView == nil {
		log.Error().Msg("failed to create workspace view")
		return false
	}
	a.installFloatingOverlayPositioning(tab.ID, wsView.WorkspaceOverlayWidget())
	if a.contentCoord != nil {
		syncCtx := context.Background()
		if a.deps != nil && a.deps.Ctx != nil {
			syncCtx = a.deps.Ctx
		}
		wsView.SetOnWebViewAttached(func(paneID entity.PaneID) {
			a.contentCoord.SyncWebViewViewport(syncCtx, paneID, "workspace-widget-attached")
		})
		wsView.SetOnActivePaneChanged(func(paneID entity.PaneID) {
			a.contentCoord.SyncWebViewViewport(syncCtx, paneID, "workspace-pane-activated")
		})
	}

	// Set the workspace
	if err := wsView.SetWorkspace(ctx, tab.Workspace); err != nil {
		log.Error().Err(err).Msg("failed to set workspace in view")
		return false
	}

	// Ensure WebViews are attached to panes
	if a.contentCoord != nil {
		a.contentCoord.AttachToWorkspace(ctx, tab.Workspace, wsView)
	}

	// Note: Mode borders for tab/pane/session are attached to MainWindow.
	// Resize mode border is attached to the active pane's stack container.

	// Set omnibox config for this workspace view.
	// When the owning browser window is known, bind navigation to the
	// owning window so that the omnibox always navigates in the correct
	// window, regardless of global focus.
	if owner := a.browserWindowForTab(tab.ID); owner != nil {
		cfg := a.omniboxCfg
		cfg.OnNavigate = omniboxNavigateForBrowserWindow(ctx, owner, a.navigateFromBrowserWindow)
		wsView.SetOmniboxConfig(cfg)
	} else {
		wsView.SetOmniboxConfig(a.omniboxCfg)
	}
	// Set find bar config for this workspace view
	wsView.SetFindBarConfig(a.findBarCfg)
	// Set auto-open omnibox on new pane
	if a.deps.Config != nil {
		wsView.SetAutoOpenOnNewPane(a.deps.Config.Omnibox.AutoOpenOnNewPane)
	}

	wsView.SetOnPaneFocused(func(paneID entity.PaneID) {
		if a.keyboardHandler != nil && a.keyboardHandler.Mode() == input.ModeResize {
			// Resize-mode pane focus tracks the last-focused window's workspace.
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
	a.reattachFloatingSessions(tab.ID, wsView)
	a.syncFloatingFocus()

	log.Debug().Str("tab_id", string(tab.ID)).Msg("workspace view created")
	return true
}

// activeWorkspace returns the workspace of the focused browser window's active tab.
func (a *App) activeWorkspace() *entity.Workspace {
	return a.activeWorkspaceForBrowserWindow(a.lastFocusedBrowserWindow())
}

// updatePaneURIInAllTabs finds a pane by ID across all tabs and updates its URI.
// This is necessary because panes in inactive tabs also need URI updates for session snapshots.
func (a *App) updatePaneURIInAllTabs(paneID entity.PaneID, url string) {
	for _, bw := range a.browserWindows {
		if bw == nil || bw.tabs == nil {
			continue
		}
		for _, tab := range bw.tabs.Tabs {
			if tab == nil || tab.Workspace == nil {
				continue
			}
			paneNode := tab.Workspace.FindPane(paneID)
			if paneNode != nil && paneNode.Pane != nil {
				paneNode.Pane.URI = url
				return // Pane IDs are unique, no need to continue
			}
		}
	}
}

// activeWorkspaceView returns the workspace view for the focused browser window's active tab.
func (a *App) activeWorkspaceView() *component.WorkspaceView {
	return a.activeWorkspaceViewForBrowserWindow(a.lastFocusedBrowserWindow())
}

// getActiveWebViewTarget returns a TextInputTarget for the active pane's WebView.
// Used by the accent picker to insert accented characters into web content.
func (a *App) getActiveWebViewTarget() port.TextInputTarget {
	_, wv := a.activeWebViewForBrowserWindow(a.lastFocusedBrowserWindow())
	if wv == nil {
		return nil
	}

	if provider, ok := wv.(port.TextInputTargetProvider); ok {
		return provider.TextInputTarget()
	}
	return nil
}

// attachPopupToTab attaches a popup WebView to a newly created tab.
// This is called when a popup uses tabbed behavior.
func (a *App) attachPopupToTab(ctx context.Context, tabID entity.TabID, pane *entity.Pane, wv port.WebView) {
	log := logging.FromContext(ctx)
	cleanupUnattachedPopup := func() {
		if wv != nil {
			wv.Destroy()
		}
	}

	wsView := a.workspaceViews[tabID]
	if wsView == nil {
		cleanupUnattachedPopup()
		log.Warn().Str("tab_id", string(tabID)).Msg("workspace view not found for popup tab")
		return
	}
	if pane == nil {
		cleanupUnattachedPopup()
		log.Warn().Str("tab_id", string(tabID)).Msg("popup pane is nil")
		return
	}
	if wsView.GetPaneView(pane.ID) == nil {
		cleanupUnattachedPopup()
		log.Warn().Str("pane_id", string(pane.ID)).Msg("pane view not found for popup")
		return
	}
	if a.contentCoord == nil {
		cleanupUnattachedPopup()
		log.Warn().Str("pane_id", string(pane.ID)).Msg("content coordinator not configured for popup")
		return
	}

	// Register WebView with content coordinator.
	a.contentCoord.RegisterPopupWebView(pane.ID, wv)

	// Wrap and attach widget.
	widget := a.contentCoord.WrapWidget(ctx, wv)
	if widget == nil {
		a.contentCoord.ReleaseWebView(ctx, pane.ID)
		log.Warn().Str("pane_id", string(pane.ID)).Msg("failed to wrap popup webview")
		return
	}
	if err := wsView.SetWebViewWidget(pane.ID, widget); err != nil {
		a.contentCoord.ReleaseWebView(ctx, pane.ID)
		log.Warn().Err(err).Str("pane_id", string(pane.ID)).Msg("pane view not found for popup")
		return
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

	// Swap content (MainWindow.SetContent now properly removes old content).
	// Active tab state is managed by TabList.SetActive; no per-window field needed.
	if target := a.browserWindowForTab(tabID); target != nil && target.mainWindow != nil {
		target.mainWindow.SetContent(gtkWidget)
	} else if a.mainWindow != nil {
		a.mainWindow.SetContent(gtkWidget)
	}

	// Update window title with the new active pane's title
	a.updateWindowTitleFromActivePane(tabID)
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
		widget := gtk.WidgetNewFromInternalPtr(widgetPtr)
		if widget != nil {
			widgetPtr = widget.GoPointer()
		}
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

// currentFloatingWidthPct returns the configured floating pane width percentage.
func (a *App) currentFloatingWidthPct() float64 {
	if a.deps != nil && a.deps.Config != nil {
		return a.deps.Config.Workspace.FloatingPane.WidthPct
	}
	return 0
}

// currentFloatingHeightPct returns the configured floating pane height percentage.
func (a *App) currentFloatingHeightPct() float64 {
	if a.deps != nil && a.deps.Config != nil {
		return a.deps.Config.Workspace.FloatingPane.HeightPct
	}
	return 0
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
	pv := component.NewPaneView(ctx, a.widgetFactory, paneID, webViewWidget)
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

	floatingPane := component.NewFloatingPane(wsView.WorkspaceOverlayWidget(), component.FloatingPaneOptions{
		WidthPct:       a.currentFloatingWidthPct(),
		HeightPct:      a.currentFloatingHeightPct(),
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
	activeTab := a.activeTabForBrowserWindow(a.lastFocusedBrowserWindow())
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

	activeTab := a.activeTabForBrowserWindow(a.lastFocusedBrowserWindow())
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
		cfg.OnNavigate = func(navCtx context.Context, url string) error {
			if session.pane == nil {
				return fmt.Errorf("floating pane is not available")
			}
			if navCtx == nil {
				navCtx = ctx
			}
			return session.pane.Navigate(navCtx, url)
		}
		cfg.OnToast = func(toastCtx context.Context, message string, level component.ToastLevel) {
			a.showToastOnLastFocusedBrowserWindow(toastCtx, message, level)
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
	if a.contentCoord != nil {
		syncCtx := context.Background()
		if a.deps != nil && a.deps.Ctx != nil {
			syncCtx = a.deps.Ctx
		}
		a.contentCoord.SyncWebViewViewport(syncCtx, session.paneID, "floating-resize")
	}
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

func (a *App) handleGlobalEscape(ctx context.Context) bool {
	return a.closeActiveFloatingPane(ctx)
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
	activeTab := a.activeTabForBrowserWindow(a.lastFocusedBrowserWindow())
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
	activeTab := a.activeTabForBrowserWindow(a.lastFocusedBrowserWindow())
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
	if bw := a.lastFocusedBrowserWindow(); bw != nil {
		return a.navigateFromBrowserWindow(ctx, bw, url)
	}
	return fmt.Errorf("no focused browser window for omnibox navigation")
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

// initFilteringAsync starts background filter loading with toast feedback.
func (a *App) initConfigWatcher(ctx context.Context) {
	log := logging.FromContext(ctx)

	if a.deps.WatchConfig == nil || a.deps.OnConfigChange == nil {
		log.Debug().Msg("no config watcher available, skipping")
		return
	}

	// Start viper watcher
	if err := a.deps.WatchConfig(); err != nil {
		log.Warn().Err(err).Msg("failed to start config watcher")
		return
	}

	// Hot-reload appearance and keybindings on config change.
	// deps.Config is updated in-place before this callback fires.
	a.deps.OnConfigChange(func() {
		cb := glib.SourceFunc(func(_ uintptr) bool {
			a.applyAppearanceConfig(ctx)
			for _, bw := range a.browserWindows {
				if bw == nil {
					continue
				}
				if bw.keyboardHandler != nil {
					bw.keyboardHandler.ReloadShortcuts(ctx, &a.deps.Config.Workspace, &a.deps.Config.Session)
				}
				if bw.globalShortcutHandler != nil {
					bw.globalShortcutHandler.ReloadShortcuts(ctx, &a.deps.Config.Workspace, &a.deps.Config.Session)
				}
			}
			return false
		})
		glib.IdleAdd(&cb, 0)
	})

	log.Debug().Msg("config watcher initialized")
}

func (a *App) applyAppearanceConfig(ctx context.Context) {
	log := logging.FromContext(ctx)
	if a.deps == nil || a.deps.Config == nil {
		return
	}
	cfg := a.deps.Config

	if a.engine != nil {
		if err := a.engine.UpdateSettings(ctx, port.EngineSettingsUpdate{
			Settings: port.EngineSettingsPayload{DefaultUIScale: cfg.DefaultUIScale},
			Raw:      cfg,
		}); err != nil {
			log.Warn().Err(err).Msg("failed to apply engine settings update")
		}
	}

	// Apply settings to existing webviews via coordinator
	if a.contentCoord != nil {
		a.contentCoord.ApplySettingsToAll(ctx)
	}

	// Update GTK theme and injected WebUI theme vars
	if a.deps.Theme != nil {
		var display *gdk.Display
		if a.mainWindow != nil && a.mainWindow.Window() != nil {
			display = a.mainWindow.Window().GetDisplay()
		}
		a.deps.Theme.UpdateFromConfig(ctx, &cfg.Appearance, cfg.DefaultUIScale, &cfg.Workspace.Styling, display)

		var inj port.ContentInjector
		if a.engine != nil {
			inj = a.engine.ContentInjector()
		}

		if inj != nil {
			findCSS := theme.GenerateFindHighlightCSS(a.deps.Theme.GetCurrentPalette())
			if err := inj.InjectFindHighlightCSS(ctx, findCSS); err != nil {
				log.Warn().Err(err).Msg("failed to update find highlight CSS")
			}
		}

		prepareThemeUC := usecase.NewPrepareWebUIThemeUseCase(inj)
		cssText := a.deps.Theme.GetWebUIThemeCSS()
		if err := prepareThemeUC.Execute(ctx, usecase.PrepareWebUIThemeInput{CSSVars: cssText}); err != nil {
			log.Warn().Err(err).Msg("failed to update WebUI theme CSS")
		}

		if a.contentCoord != nil && inj != nil {
			a.contentCoord.RefreshInjectedScriptsToAll(ctx)
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

	a.deps.FilterManager.SetStatusCallback(func(status port.FilterStatus) {
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
func (a *App) showFilterStatus(ctx context.Context, status port.FilterStatus) {
	log := logging.FromContext(ctx)

	switch status.State {
	case port.FilterStateLoading:
		a.showToastOnLastFocusedBrowserWindow(ctx, status.Message, component.ToastInfo)
	case port.FilterStateActive:
		// Apply filters to existing webviews that were created before filters loaded
		if a.contentCoord != nil && a.deps.FilterManager != nil {
			a.contentCoord.ApplyFiltersToAll(ctx)
			log.Debug().Msg("applied filters to all existing webviews")
		}
		a.showToastOnLastFocusedBrowserWindow(ctx, fmt.Sprintf("Ad blocker ready (%s)", status.Version), component.ToastInfo)
	case port.FilterStateError:
		a.showToastOnLastFocusedBrowserWindow(ctx, "Filter load failed: "+status.Message, component.ToastError)
	}
}

package content

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/rs/zerolog"
)

// Coordinator manages WebView lifecycle, title tracking, and content attachment.
type Coordinator struct {
	logger          zerolog.Logger
	pool            port.WebViewPool
	widgetFactory   layout.WidgetFactory
	faviconAdapter  *adapter.FaviconAdapter
	zoomUC          *usecase.ManageZoomUseCase
	permissionUC    *usecase.HandlePermissionUseCase
	injector        port.ContentInjector
	settingsApplier port.SettingsApplier // optional: nil if engine doesn't support
	filterApplier   port.FilterApplier   // optional: nil if engine doesn't support

	webViews       map[entity.PaneID]port.WebView
	webViewPaneIDs map[port.WebViewID]entity.PaneID
	webViewsMu     sync.RWMutex

	activePaneOverride   entity.PaneID
	activePaneOverrideMu sync.RWMutex

	paneTitles map[entity.PaneID]string
	titleMu    sync.RWMutex

	// Track original navigation URLs to handle cross-domain redirects
	// e.g., google.fr → google.com: cache favicon under both domains
	navOrigins  map[entity.PaneID]string
	navOriginMu sync.RWMutex

	// Callback to get active workspace state (avoids circular dependency)
	getActiveWS func() (*entity.Workspace, *component.WorkspaceView)

	// Callback when title changes (for history persistence)
	onTitleUpdated func(ctx context.Context, paneID entity.PaneID, url, title string)

	// Callback when page is committed (for history recording)
	onHistoryRecord func(ctx context.Context, paneID entity.PaneID, url string)

	// Callback when pane URI changes (for session snapshots)
	onPaneURIUpdated func(paneID entity.PaneID, url string)

	// Callback when active pane title changes (for window title updates)
	onWindowTitleChanged func(paneID entity.PaneID, title string)

	// Callback when media permission activity changes (requesting/allowed/blocked).
	onPermissionActivity func(paneID entity.PaneID, origin string, permTypes []entity.PermissionType, state PermissionActivityState)

	// Callback when the active pane commits a navigation (new page loading).
	onActiveNavigationCommitted func(paneID entity.PaneID, uri string)

	// Callback when the WebView becomes visible (first real commit)
	onWebViewShown func(paneID entity.PaneID)

	revealMu      sync.Mutex
	pendingReveal map[entity.PaneID]bool

	appearanceMu          sync.Mutex
	pendingScriptRefresh  map[entity.PaneID]bool
	pendingThemePanes     map[entity.PaneID]bool
	pendingThemeUpdate    pendingThemeUpdate
	hasPendingThemeUpdate bool
	currentTheme          pendingThemeUpdate
	hasCurrentTheme       bool

	// Gesture action handler for mouse button navigation
	gestureActionHandler input.ActionHandler

	// Popup handling stays in a dedicated UI-layer manager so popup-specific
	// pane state does not bloat the main coordinator.
	popups *popupManager

	// Idle inhibitor for fullscreen video playback
	idleInhibitor port.IdleInhibitor

	// Callback when fullscreen state changes (for hiding/showing tab bar)
	onFullscreenChanged func(paneID entity.PaneID, entering bool)

	// Callback when WebView gains focus (for accent picker text input targeting)
	onWebViewFocused func(paneID entity.PaneID, wv port.WebView)

	// Callback for first load_started event (triggers deferred initialization)
	onFirstLoadStarted func()
	loadStartedOnce    sync.Once

	// Callback to open a URL with the system's default handler (e.g. xdg-open).
	// Used for external URL schemes like vscode://, spotify://, etc.
	onLaunchExternalURL func(uri string)
}

type pendingThemeUpdate struct {
	prefersDark bool
	cssText     string
}

// PermissionActivityState represents the visible state for media permission activity.
type PermissionActivityState string

const (
	PermissionActivityRequesting PermissionActivityState = "requesting"
	PermissionActivityAllowed    PermissionActivityState = "allowed"
	PermissionActivityBlocked    PermissionActivityState = "blocked"
)

// NewCoordinator creates a new Coordinator.
func NewCoordinator(
	ctx context.Context,
	pool port.WebViewPool,
	injector port.ContentInjector,
	widgetFactory layout.WidgetFactory,
	faviconAdapter *adapter.FaviconAdapter,
	getActiveWS func() (*entity.Workspace, *component.WorkspaceView),
	zoomUC *usecase.ManageZoomUseCase,
	permissionUC *usecase.HandlePermissionUseCase,
) *Coordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating content coordinator")

	return &Coordinator{
		logger:               log.With().Str("component", "content-coordinator").Logger(),
		pool:                 pool,
		injector:             injector,
		widgetFactory:        widgetFactory,
		faviconAdapter:       faviconAdapter,
		zoomUC:               zoomUC,
		permissionUC:         permissionUC,
		webViews:             make(map[entity.PaneID]port.WebView),
		webViewPaneIDs:       make(map[port.WebViewID]entity.PaneID),
		paneTitles:           make(map[entity.PaneID]string),
		navOrigins:           make(map[entity.PaneID]string),
		pendingReveal:        make(map[entity.PaneID]bool),
		pendingScriptRefresh: make(map[entity.PaneID]bool),
		pendingThemePanes:    make(map[entity.PaneID]bool),
		getActiveWS:          getActiveWS,
		popups:               newPopupManager(),
	}
}

func (c *Coordinator) ensurePopupManager() *popupManager {
	if c == nil {
		return nil
	}
	if c.popups == nil {
		c.popups = newPopupManager()
	}
	c.popups.ensureInitialized()
	return c.popups
}

// SetOnTitleUpdated sets the callback for title changes (for history persistence).
func (c *Coordinator) SetOnTitleUpdated(fn func(ctx context.Context, paneID entity.PaneID, url, title string)) {
	c.onTitleUpdated = fn
}

// SetOnHistoryRecord sets the callback for recording history on page commit.
func (c *Coordinator) SetOnHistoryRecord(fn func(ctx context.Context, paneID entity.PaneID, url string)) {
	c.onHistoryRecord = fn
}

// SetOnPermissionActivity sets a callback for WebRTC permission activity changes.
func (c *Coordinator) SetOnPermissionActivity(
	fn func(paneID entity.PaneID, origin string, permTypes []entity.PermissionType, state PermissionActivityState),
) {
	c.onPermissionActivity = fn
}

// SetOnActiveNavigationCommitted sets a callback fired when the active pane commits a navigation.
func (c *Coordinator) SetOnActiveNavigationCommitted(fn func(paneID entity.PaneID, uri string)) {
	c.onActiveNavigationCommitted = fn
}

// SetOnPaneURIUpdated sets the callback for pane URI changes (for session snapshots).
func (c *Coordinator) SetOnPaneURIUpdated(fn func(paneID entity.PaneID, url string)) {
	c.onPaneURIUpdated = fn
}

// SetOnWindowTitleChanged sets the callback for active pane title changes (for window title updates).
func (c *Coordinator) SetOnWindowTitleChanged(fn func(paneID entity.PaneID, title string)) {
	c.onWindowTitleChanged = fn
}

// SetOnWebViewShown sets a callback that fires when a pane's WebView is shown.
func (c *Coordinator) SetOnWebViewShown(fn func(paneID entity.PaneID)) {
	c.onWebViewShown = fn
}

// SetGestureActionHandler sets the callback for mouse button navigation gestures.
func (c *Coordinator) SetGestureActionHandler(handler input.ActionHandler) {
	c.gestureActionHandler = handler
}

// SetIdleInhibitor sets the idle inhibitor for fullscreen video playback.
func (c *Coordinator) SetIdleInhibitor(inhibitor port.IdleInhibitor) {
	c.idleInhibitor = inhibitor
}

// SetOnFullscreenChanged sets the callback for fullscreen state changes.
func (c *Coordinator) SetOnFullscreenChanged(fn func(paneID entity.PaneID, entering bool)) {
	c.onFullscreenChanged = fn
}

// SetOnWebViewFocused sets the callback for when a WebView gains focus.
func (c *Coordinator) SetOnWebViewFocused(fn func(paneID entity.PaneID, wv port.WebView)) {
	c.onWebViewFocused = fn
}

// SetOnFirstLoadStarted sets the callback for when the first navigation starts.
// This is used to trigger deferred initialization after the initial load_uri()
// has been processed by the GTK main loop.
func (c *Coordinator) SetOnFirstLoadStarted(fn func()) {
	c.onFirstLoadStarted = fn
}

// SetOnLaunchExternalURL sets the callback invoked when an external URL scheme
// (e.g. vscode://, spotify://) is detected and must be handed off to the system.
func (c *Coordinator) SetOnLaunchExternalURL(fn func(uri string)) {
	c.onLaunchExternalURL = fn
}

// SetSettingsApplier sets the engine settings applier for config hot-reload.
func (c *Coordinator) SetSettingsApplier(sa port.SettingsApplier) {
	c.settingsApplier = sa
}

// SetFilterApplier sets the content filter applier for late-binding filters.
func (c *Coordinator) SetFilterApplier(fa port.FilterApplier) {
	c.filterApplier = fa
}

// ActivePaneID returns the currently active pane ID used by navigation.
func (c *Coordinator) ActivePaneID(ctx context.Context) entity.PaneID {
	if paneID, ok := c.activePaneOverrideID(); ok {
		return paneID
	}
	if c.getActiveWS == nil {
		return ""
	}

	ws, _ := c.getActiveWS()
	if ws == nil {
		return ""
	}
	return ws.ActivePaneID
}

// SetActivePaneOverride forces ActiveWebView/ActivePaneID to use a specific pane.
func (c *Coordinator) SetActivePaneOverride(paneID entity.PaneID) {
	c.activePaneOverrideMu.Lock()
	c.activePaneOverride = paneID
	c.activePaneOverrideMu.Unlock()
}

// ClearActivePaneOverride removes any forced active pane override.
func (c *Coordinator) ClearActivePaneOverride() {
	c.activePaneOverrideMu.Lock()
	c.activePaneOverride = ""
	c.activePaneOverrideMu.Unlock()
}

func (c *Coordinator) activePaneOverrideID() (entity.PaneID, bool) {
	c.activePaneOverrideMu.RLock()
	defer c.activePaneOverrideMu.RUnlock()
	if c.activePaneOverride == "" {
		return "", false
	}
	return c.activePaneOverride, true
}

func (c *Coordinator) webViewCount() int {
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	return len(c.webViews)
}

func (c *Coordinator) getWebViewLocked(paneID entity.PaneID) port.WebView {
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	return c.webViews[paneID]
}

func (c *Coordinator) setWebViewLocked(paneID entity.PaneID, wv port.WebView) {
	c.webViewsMu.Lock()
	if c.webViews == nil {
		c.webViews = make(map[entity.PaneID]port.WebView)
	}
	if existing := c.webViews[paneID]; existing != nil && c.webViewPaneIDs != nil {
		delete(c.webViewPaneIDs, existing.ID())
	}
	c.webViews[paneID] = wv
	if wv != nil && c.webViewPaneIDs != nil {
		c.webViewPaneIDs[wv.ID()] = paneID
	}
	c.webViewsMu.Unlock()
}

func (c *Coordinator) deleteWebViewLocked(paneID entity.PaneID) port.WebView {
	c.webViewsMu.Lock()
	defer c.webViewsMu.Unlock()
	wv := c.webViews[paneID]
	delete(c.webViews, paneID)
	if wv != nil && c.webViewPaneIDs != nil {
		delete(c.webViewPaneIDs, wv.ID())
	}
	return wv
}

func (c *Coordinator) paneIDByWebViewID(webViewID port.WebViewID) (entity.PaneID, bool) {
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	paneID, ok := c.webViewPaneIDs[webViewID]
	return paneID, ok
}

package content

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
)

// Coordinator manages WebView lifecycle, title tracking, and content attachment.
type Coordinator struct {
	pool           *webkit.WebViewPool
	widgetFactory  layout.WidgetFactory
	faviconAdapter *adapter.FaviconAdapter
	zoomUC         *usecase.ManageZoomUseCase
	permissionUC   *usecase.HandlePermissionUseCase
	injector       *webkit.ContentInjector

	webViews   map[entity.PaneID]*webkit.WebView
	webViewsMu sync.RWMutex

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
	onWindowTitleChanged func(title string)

	// Callback when media permission activity changes (requesting/allowed/blocked).
	onPermissionActivity func(origin string, permTypes []entity.PermissionType, state PermissionActivityState)

	// Callback when the active pane commits a navigation (new page loading).
	onActiveNavigationCommitted func(uri string)

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

	// Popup handling
	factory       *webkit.WebViewFactory
	popupConfig   *config.PopupBehaviorConfig
	pendingPopups map[port.WebViewID]*PendingPopup
	popupOAuth    map[port.WebViewID]*popupOAuthState
	popupRefresh  map[entity.PaneID]*time.Timer
	popupMu       sync.RWMutex

	// Callback to insert popup into workspace (avoids circular dependency)
	onInsertPopup func(ctx context.Context, input InsertPopupInput) error

	// Callback to close a pane when popup closes
	onClosePane func(ctx context.Context, paneID entity.PaneID) error

	// ID generator for popup panes
	generateID func() string

	// Idle inhibitor for fullscreen video playback
	idleInhibitor port.IdleInhibitor

	// Callback when fullscreen state changes (for hiding/showing tab bar)
	onFullscreenChanged func(entering bool)

	// Callback when WebView gains focus (for accent picker text input targeting)
	onWebViewFocused func(paneID entity.PaneID, wv *webkit.WebView)

	// Callback for first load_started event (triggers deferred initialization)
	onFirstLoadStarted func()
	loadStartedOnce    sync.Once
}

type pendingThemeUpdate struct {
	prefersDark bool
	cssText     string
}

type popupOAuthState struct {
	ParentPaneID entity.PaneID
	CallbackURI  string
	Success      bool
	Error        bool
	Seen         bool
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
	pool *webkit.WebViewPool,
	widgetFactory layout.WidgetFactory,
	faviconAdapter *adapter.FaviconAdapter,
	getActiveWS func() (*entity.Workspace, *component.WorkspaceView),
	zoomUC *usecase.ManageZoomUseCase,
	permissionUC *usecase.HandlePermissionUseCase,
) *Coordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating content coordinator")

	return &Coordinator{
		pool:           pool,
		widgetFactory:  widgetFactory,
		faviconAdapter: faviconAdapter,
		zoomUC:         zoomUC,
		permissionUC:   permissionUC,
		webViews:       make(map[entity.PaneID]*webkit.WebView),
		paneTitles:     make(map[entity.PaneID]string),
		navOrigins:     make(map[entity.PaneID]string),
		pendingReveal:  make(map[entity.PaneID]bool),
		getActiveWS:    getActiveWS,
		pendingPopups:  make(map[port.WebViewID]*PendingPopup),
		popupOAuth:     make(map[port.WebViewID]*popupOAuthState),
		popupRefresh:   make(map[entity.PaneID]*time.Timer),
	}
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
	fn func(origin string, permTypes []entity.PermissionType, state PermissionActivityState),
) {
	c.onPermissionActivity = fn
}

// SetOnActiveNavigationCommitted sets a callback fired when the active pane commits a navigation.
func (c *Coordinator) SetOnActiveNavigationCommitted(fn func(uri string)) {
	c.onActiveNavigationCommitted = fn
}

// SetOnPaneURIUpdated sets the callback for pane URI changes (for session snapshots).
func (c *Coordinator) SetOnPaneURIUpdated(fn func(paneID entity.PaneID, url string)) {
	c.onPaneURIUpdated = fn
}

// SetOnWindowTitleChanged sets the callback for active pane title changes (for window title updates).
func (c *Coordinator) SetOnWindowTitleChanged(fn func(title string)) {
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
func (c *Coordinator) SetOnFullscreenChanged(fn func(entering bool)) {
	c.onFullscreenChanged = fn
}

// SetOnWebViewFocused sets the callback for when a WebView gains focus.
func (c *Coordinator) SetOnWebViewFocused(fn func(paneID entity.PaneID, wv *webkit.WebView)) {
	c.onWebViewFocused = fn
}

// SetOnFirstLoadStarted sets the callback for when the first navigation starts.
// This is used to trigger deferred initialization after the initial load_uri()
// has been processed by the GTK main loop.
func (c *Coordinator) SetOnFirstLoadStarted(fn func()) {
	c.onFirstLoadStarted = fn
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

func (c *Coordinator) getWebViewLocked(paneID entity.PaneID) *webkit.WebView {
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	return c.webViews[paneID]
}

func (c *Coordinator) setWebViewLocked(paneID entity.PaneID, wv *webkit.WebView) {
	c.webViewsMu.Lock()
	c.webViews[paneID] = wv
	c.webViewsMu.Unlock()
}

func (c *Coordinator) deleteWebViewLocked(paneID entity.PaneID) *webkit.WebView {
	c.webViewsMu.Lock()
	defer c.webViewsMu.Unlock()
	wv := c.webViews[paneID]
	delete(c.webViews, paneID)
	return wv
}

func (c *Coordinator) snapshotWebViews() map[entity.PaneID]*webkit.WebView {
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	snapshot := make(map[entity.PaneID]*webkit.WebView, len(c.webViews))
	for paneID, wv := range c.webViews {
		snapshot[paneID] = wv
	}
	return snapshot
}

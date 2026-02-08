package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	urlutil "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	webkitlib "github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

const (
	aboutBlankURI              = "about:blank"
	crashPageURI               = "dumb://home/crash"
	logURLMaxLen               = 80
	oauthParentRefreshDebounce = 200 * time.Millisecond

	// Dark theme background color (#0a0a0b) as float32 RGBA values
	darkBgR = 0.039
	darkBgG = 0.039
	darkBgB = 0.043
	darkBgA = 1.0
)

func shouldRenderCrashPage(reason webkitlib.WebProcessTerminationReason) bool {
	switch reason {
	case webkitlib.WebProcessCrashedValue, webkitlib.WebProcessExceededMemoryLimitValue:
		return true
	case webkitlib.WebProcessTerminatedByApiValue:
		return false
	default:
		return true
	}
}

func extractOriginalURIFromCrashPage(uri string) string {
	if uri == "" {
		return ""
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return uri
	}

	if parsed.Scheme != "dumb" || parsed.Host != webkit.HomePath {
		return uri
	}

	if strings.Trim(parsed.Path, "/") != "crash" {
		return uri
	}

	original := strings.TrimSpace(parsed.Query().Get("url"))
	if original == "" {
		return ""
	}
	return original
}

func buildCrashPageURI(originalURI string) string {
	if strings.TrimSpace(originalURI) == "" {
		return crashPageURI
	}
	query := url.Values{}
	query.Set("url", originalURI)
	return crashPageURI + "?" + query.Encode()
}

// ContentCoordinator manages WebView lifecycle, title tracking, and content attachment.
type ContentCoordinator struct {
	pool           *webkit.WebViewPool
	widgetFactory  layout.WidgetFactory
	faviconAdapter *adapter.FaviconAdapter
	zoomUC         *usecase.ManageZoomUseCase
	permissionUC   *usecase.HandlePermissionUseCase
	injector       *webkit.ContentInjector

	webViews   map[entity.PaneID]*webkit.WebView
	webViewsMu sync.RWMutex
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

// NewContentCoordinator creates a new ContentCoordinator.
func NewContentCoordinator(
	ctx context.Context,
	pool *webkit.WebViewPool,
	widgetFactory layout.WidgetFactory,
	faviconAdapter *adapter.FaviconAdapter,
	getActiveWS func() (*entity.Workspace, *component.WorkspaceView),
	zoomUC *usecase.ManageZoomUseCase,
	permissionUC *usecase.HandlePermissionUseCase,
) *ContentCoordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating content coordinator")

	return &ContentCoordinator{
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
func (c *ContentCoordinator) SetOnTitleUpdated(fn func(ctx context.Context, paneID entity.PaneID, url, title string)) {
	c.onTitleUpdated = fn
}

// SetOnHistoryRecord sets the callback for recording history on page commit.
func (c *ContentCoordinator) SetOnHistoryRecord(fn func(ctx context.Context, paneID entity.PaneID, url string)) {
	c.onHistoryRecord = fn
}

// SetOnPermissionActivity sets a callback for WebRTC permission activity changes.
func (c *ContentCoordinator) SetOnPermissionActivity(
	fn func(origin string, permTypes []entity.PermissionType, state PermissionActivityState),
) {
	c.onPermissionActivity = fn
}

// SetOnActiveNavigationCommitted sets a callback fired when the active pane commits a navigation.
func (c *ContentCoordinator) SetOnActiveNavigationCommitted(fn func(uri string)) {
	c.onActiveNavigationCommitted = fn
}

// SetOnPaneURIUpdated sets the callback for pane URI changes (for session snapshots).
func (c *ContentCoordinator) SetOnPaneURIUpdated(fn func(paneID entity.PaneID, url string)) {
	c.onPaneURIUpdated = fn
}

// SetOnWindowTitleChanged sets the callback for active pane title changes (for window title updates).
func (c *ContentCoordinator) SetOnWindowTitleChanged(fn func(title string)) {
	c.onWindowTitleChanged = fn
}

// SetOnWebViewShown sets a callback that fires when a pane's WebView is shown.
func (c *ContentCoordinator) SetOnWebViewShown(fn func(paneID entity.PaneID)) {
	c.onWebViewShown = fn
}

// SetGestureActionHandler sets the callback for mouse button navigation gestures.
func (c *ContentCoordinator) SetGestureActionHandler(handler input.ActionHandler) {
	c.gestureActionHandler = handler
}

// SetIdleInhibitor sets the idle inhibitor for fullscreen video playback.
func (c *ContentCoordinator) SetIdleInhibitor(inhibitor port.IdleInhibitor) {
	c.idleInhibitor = inhibitor
}

// SetOnFullscreenChanged sets the callback for fullscreen state changes.
func (c *ContentCoordinator) SetOnFullscreenChanged(fn func(entering bool)) {
	c.onFullscreenChanged = fn
}

// SetOnWebViewFocused sets the callback for when a WebView gains focus.
func (c *ContentCoordinator) SetOnWebViewFocused(fn func(paneID entity.PaneID, wv *webkit.WebView)) {
	c.onWebViewFocused = fn
}

// SetOnFirstLoadStarted sets the callback for when the first navigation starts.
// This is used to trigger deferred initialization after the initial load_uri()
// has been processed by the GTK main loop.
func (c *ContentCoordinator) SetOnFirstLoadStarted(fn func()) {
	c.onFirstLoadStarted = fn
}

// EnsureWebView acquires or reuses a WebView for the given pane.
func (c *ContentCoordinator) EnsureWebView(ctx context.Context, paneID entity.PaneID) (*webkit.WebView, error) {
	log := logging.FromContext(ctx)

	if wv := c.getWebViewLocked(paneID); wv != nil && !wv.IsDestroyed() {
		return wv, nil
	}

	if c.pool == nil {
		return nil, fmt.Errorf("webview pool not configured")
	}

	// Mark tab_created on first webview (first tab)
	if c.webViewCount() == 0 {
		logging.Trace().Mark("tab_created")
	}

	wv, err := c.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	logging.Trace().Mark("webview_acquired")

	c.setWebViewLocked(paneID, wv)
	c.setupWebViewCallbacks(ctx, paneID, wv)

	log.Debug().Str("pane_id", string(paneID)).Msg("webview acquired for pane")
	return wv, nil
}

// ReleaseWebView returns the WebView for a pane to the pool.
func (c *ContentCoordinator) ReleaseWebView(ctx context.Context, paneID entity.PaneID) {
	log := logging.FromContext(ctx)

	wv := c.deleteWebViewLocked(paneID)
	if wv == nil {
		return
	}
	c.clearPendingAppearance(paneID)

	// CRITICAL: If this webview was inhibiting idle (fullscreen or audio playing),
	// we must release the inhibition before destroying the webview.
	// Otherwise the D-Bus inhibit request stays active forever.
	if c.idleInhibitor != nil {
		if wv.IsFullscreen() {
			log.Debug().Str("pane_id", string(paneID)).Msg("releasing idle inhibition (was fullscreen)")
			if err := c.idleInhibitor.Uninhibit(ctx); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to uninhibit idle on release (fullscreen)")
			}
		}
		if wv.IsPlayingAudio() {
			log.Debug().Str("pane_id", string(paneID)).Msg("releasing idle inhibition (was playing audio)")
			if err := c.idleInhibitor.Uninhibit(ctx); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to uninhibit idle on release (audio)")
			}
		}
	}

	// Clean up title tracking
	c.titleMu.Lock()
	delete(c.paneTitles, paneID)
	c.titleMu.Unlock()

	// Clean up navigation origin tracking
	c.navOriginMu.Lock()
	delete(c.navOrigins, paneID)
	c.navOriginMu.Unlock()

	if c.pool != nil {
		c.pool.Release(ctx, wv)
	} else {
		wv.Destroy()
	}

	log.Debug().Str("pane_id", string(paneID)).Msg("webview released")
}

// AttachToWorkspace ensures each pane in the workspace has a WebView widget attached.
func (c *ContentCoordinator) AttachToWorkspace(ctx context.Context, ws *entity.Workspace, wsView *component.WorkspaceView) {
	log := logging.FromContext(ctx)

	if ws == nil || wsView == nil || c.widgetFactory == nil {
		return
	}

	for _, pane := range ws.AllPanes() {
		if pane == nil {
			continue
		}

		wv, err := c.EnsureWebView(ctx, pane.ID)
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

		widget := c.WrapWidget(ctx, wv)
		if widget == nil {
			continue
		}

		if err := wsView.SetWebViewWidget(pane.ID, widget); err != nil {
			log.Warn().Err(err).Str("pane_id", string(pane.ID)).Msg("failed to attach webview widget")
		}
		logging.Trace().Mark("webview_attached")
	}
}

// WrapWidget converts a WebView to a layout.Widget for embedding.
// It also attaches gesture handlers for mouse button navigation.
func (c *ContentCoordinator) WrapWidget(ctx context.Context, wv *webkit.WebView) layout.Widget {
	log := logging.FromContext(ctx)

	if wv == nil || c.widgetFactory == nil {
		log.Debug().Msg("cannot wrap nil webview or factory")
		return nil
	}

	gtkView := wv.Widget()
	if gtkView == nil {
		return nil
	}

	widget := c.widgetFactory.WrapWidget(&gtkView.Widget)

	// Attach gesture handler for mouse button 8/9 navigation
	if widget != nil {
		gestureHandler := input.NewGestureHandler(ctx)
		// Pass WebView directly to preserve user gesture context (like Epiphany)
		gestureHandler.SetNavigator(wv)
		// Keep callback as fallback
		if c.gestureActionHandler != nil {
			gestureHandler.SetOnAction(c.gestureActionHandler)
		}
		gestureHandler.AttachTo(widget.GtkWidget())
		log.Debug().Msg("gesture handler attached to webview with direct navigator")
	}

	return widget
}

// ActiveWebView returns the WebView for the active pane.
func (c *ContentCoordinator) ActiveWebView(ctx context.Context) *webkit.WebView {
	log := logging.FromContext(ctx)

	ws, _ := c.getActiveWS()
	if ws == nil {
		log.Debug().Msg("no active workspace")
		return nil
	}

	pane := ws.ActivePane()
	if pane == nil || pane.Pane == nil {
		log.Debug().Msg("no active pane")
		return nil
	}

	return c.getWebViewLocked(pane.Pane.ID)
}

// GetWebView returns the WebView for a specific pane.
func (c *ContentCoordinator) GetWebView(paneID entity.PaneID) *webkit.WebView {
	return c.getWebViewLocked(paneID)
}

// RegisterPopupWebView registers a popup WebView that was created externally.
// This is used when popup tabs are created and the WebView needs to be tracked.
func (c *ContentCoordinator) RegisterPopupWebView(paneID entity.PaneID, wv *webkit.WebView) {
	if wv != nil && paneID != "" {
		c.setWebViewLocked(paneID, wv)
	}
}

func (c *ContentCoordinator) getWebViewLocked(paneID entity.PaneID) *webkit.WebView {
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	return c.webViews[paneID]
}

func (c *ContentCoordinator) setWebViewLocked(paneID entity.PaneID, wv *webkit.WebView) {
	c.webViewsMu.Lock()
	c.webViews[paneID] = wv
	c.webViewsMu.Unlock()
}

func (c *ContentCoordinator) deleteWebViewLocked(paneID entity.PaneID) *webkit.WebView {
	c.webViewsMu.Lock()
	defer c.webViewsMu.Unlock()
	wv := c.webViews[paneID]
	delete(c.webViews, paneID)
	return wv
}

func (c *ContentCoordinator) webViewCount() int {
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	return len(c.webViews)
}

func (c *ContentCoordinator) snapshotWebViews() map[entity.PaneID]*webkit.WebView {
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	snapshot := make(map[entity.PaneID]*webkit.WebView, len(c.webViews))
	for paneID, wv := range c.webViews {
		snapshot[paneID] = wv
	}
	return snapshot
}

// ApplyFiltersToAll applies content filters to all active webviews.
// Called when filters become available after webviews were already created.
func (c *ContentCoordinator) ApplyFiltersToAll(ctx context.Context, applier webkit.FilterApplier) {
	log := logging.FromContext(ctx)

	for paneID, wv := range c.snapshotWebViews() {
		if wv != nil && !wv.IsDestroyed() {
			applier.ApplyTo(ctx, wv.UserContentManager())
			log.Debug().Str("pane_id", string(paneID)).Msg("applied filters to existing webview")
		}
	}
}

// ApplySettingsToAll reapplies WebKit settings to all active WebViews.
func (c *ContentCoordinator) ApplySettingsToAll(ctx context.Context, sm *webkit.SettingsManager) {
	log := logging.FromContext(ctx)
	if sm == nil {
		return
	}

	for paneID, wv := range c.snapshotWebViews() {
		if wv == nil || wv.IsDestroyed() {
			continue
		}
		sm.ApplyToWebView(ctx, wv.Widget())
		log.Debug().Str("pane_id", string(paneID)).Msg("reapplied settings to webview")
	}
}

// RefreshInjectedScriptsToAll clears and re-injects user scripts into all active WebViews.
//
// WebKit user scripts are snapshotted when added to a WebKitUserContentManager, so when
// appearance settings change at runtime (dark mode, palettes, UI scale), we must refresh
// the scripts so future navigations pick up the latest values.
// Script refresh is deferred for any WebView that is currently loading to avoid
// removing scripts mid-navigation.
func (c *ContentCoordinator) RefreshInjectedScriptsToAll(ctx context.Context, injector *webkit.ContentInjector) {
	log := logging.FromContext(ctx)
	if injector == nil {
		return
	}

	c.injector = injector
	for paneID, wv := range c.webViews {
		if wv == nil || wv.IsDestroyed() {
			continue
		}
		if c.shouldDeferAppearance(wv) {
			c.queueScriptRefresh(paneID)
			log.Debug().Str("pane_id", string(paneID)).Msg("deferred script refresh until load finished")
			continue
		}

		c.refreshInjectedScripts(ctx, injector, paneID, wv)
	}
}

// ApplyWebUIThemeToAll updates theme CSS for already-loaded dumb:// pages.
// This is necessary because user scripts only run on navigation.
func (c *ContentCoordinator) ApplyWebUIThemeToAll(ctx context.Context, prefersDark bool, cssText string) {
	log := logging.FromContext(ctx)
	if cssText == "" {
		return
	}

	c.setCurrentTheme(prefersDark, cssText)

	script, err := buildWebUIThemeScript(prefersDark, cssText)
	if err != nil {
		log.Warn().Err(err).Msg("failed to build WebUI theme script")
		return
	}

	for paneID, wv := range c.webViews {
		if wv == nil || wv.IsDestroyed() {
			continue
		}
		if c.shouldDeferAppearance(wv) {
			c.queueThemeApply(paneID, prefersDark, cssText)
			log.Debug().Str("pane_id", string(paneID)).Msg("deferred WebUI theme apply until load committed")
			continue
		}
		c.applyWebUITheme(ctx, paneID, wv, script, prefersDark)
	}
}

func buildWebUIThemeScript(prefersDark bool, cssText string) (string, error) {
	cssJSON, err := json.Marshal(cssText)
	if err != nil {
		return "", err
	}

	script := fmt.Sprintf(`(function(){
  try {
    var cssText = %s;
    var prefersDark = %t;

    // Keep the global flag in sync
    window.__dumber_gtk_prefers_dark = prefersDark;

    // Update dark/light class
    if (prefersDark) {
      document.documentElement.classList.add('dark');
      document.documentElement.classList.remove('light');
    } else {
      document.documentElement.classList.add('light');
      document.documentElement.classList.remove('dark');
    }

    // Update or insert theme style
    var style = document.querySelector('style[data-dumber-theme-vars]');
    if (!style) {
      style = document.createElement('style');
      style.setAttribute('data-dumber-theme-vars', '');
      (document.head || document.documentElement).appendChild(style);
    }
    style.textContent = cssText;

    // Notify any running WebUI that theme changed
    try {
      window.dispatchEvent(new CustomEvent('dumber:theme-changed', {
        detail: { prefersDark: prefersDark }
      }));
    } catch (e) {
      // ignore
    }

    // Keep color-scheme consistent
    var meta = document.querySelector('meta[name="color-scheme"]');
    if (!meta) {
      meta = document.createElement('meta');
      meta.name = 'color-scheme';
      document.documentElement.appendChild(meta);
    }
    meta.content = prefersDark ? 'dark light' : 'light dark';
  } catch (e) {
    console.error('[dumber] failed to apply theme', e);
  }
})();`, string(cssJSON), prefersDark)

	return script, nil
}

func (c *ContentCoordinator) applyWebUITheme(
	ctx context.Context,
	paneID entity.PaneID,
	wv *webkit.WebView,
	script string,
	prefersDark bool,
) {
	if wv == nil || wv.IsDestroyed() {
		return
	}
	uri := wv.URI()
	if !strings.HasPrefix(uri, "dumb://") {
		return
	}
	wv.RunJavaScript(ctx, script, "")
	logging.FromContext(ctx).
		Debug().
		Str("pane_id", string(paneID)).
		Str("uri", uri).
		Bool("prefers_dark", prefersDark).
		Msg("applied WebUI theme")
}

func (c *ContentCoordinator) queueThemeApply(paneID entity.PaneID, prefersDark bool, cssText string) {
	c.appearanceMu.Lock()
	if c.pendingThemePanes == nil {
		c.pendingThemePanes = make(map[entity.PaneID]bool)
	}
	c.pendingThemePanes[paneID] = true
	c.pendingThemeUpdate = pendingThemeUpdate{
		prefersDark: prefersDark,
		cssText:     cssText,
	}
	c.hasPendingThemeUpdate = true
	c.appearanceMu.Unlock()
}

func (c *ContentCoordinator) setCurrentTheme(prefersDark bool, cssText string) {
	c.appearanceMu.Lock()
	c.currentTheme = pendingThemeUpdate{
		prefersDark: prefersDark,
		cssText:     cssText,
	}
	c.hasCurrentTheme = true
	c.appearanceMu.Unlock()
}

func (c *ContentCoordinator) getCurrentTheme() (pendingThemeUpdate, bool) {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()

	if !c.hasCurrentTheme {
		return pendingThemeUpdate{}, false
	}
	return c.currentTheme, true
}

func (c *ContentCoordinator) takePendingThemeApply(paneID entity.PaneID) (pendingThemeUpdate, bool) {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()

	if !c.hasPendingThemeUpdate || c.pendingThemePanes == nil || !c.pendingThemePanes[paneID] {
		return pendingThemeUpdate{}, false
	}
	delete(c.pendingThemePanes, paneID)
	update := c.pendingThemeUpdate
	if len(c.pendingThemePanes) == 0 {
		c.hasPendingThemeUpdate = false
	}
	return update, true
}

func (c *ContentCoordinator) applyPendingThemeUpdate(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) bool {
	update, ok := c.takePendingThemeApply(paneID)
	if !ok {
		return false
	}

	script, err := buildWebUIThemeScript(update.prefersDark, update.cssText)
	if err != nil {
		logging.FromContext(ctx).Warn().Err(err).Msg("failed to build deferred WebUI theme script")
		return false
	}
	c.applyWebUITheme(ctx, paneID, wv, script, update.prefersDark)
	return true
}

func (c *ContentCoordinator) applyCurrentTheme(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) bool {
	update, ok := c.getCurrentTheme()
	if !ok || update.cssText == "" {
		return false
	}

	script, err := buildWebUIThemeScript(update.prefersDark, update.cssText)
	if err != nil {
		logging.FromContext(ctx).Warn().Err(err).Msg("failed to build current WebUI theme script")
		return false
	}
	c.applyWebUITheme(ctx, paneID, wv, script, update.prefersDark)
	return true
}

func (c *ContentCoordinator) queueScriptRefresh(paneID entity.PaneID) {
	c.appearanceMu.Lock()
	if c.pendingScriptRefresh == nil {
		c.pendingScriptRefresh = make(map[entity.PaneID]bool)
	}
	c.pendingScriptRefresh[paneID] = true
	c.appearanceMu.Unlock()
}

func (c *ContentCoordinator) takePendingScriptRefresh(paneID entity.PaneID) bool {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()

	if c.pendingScriptRefresh == nil || !c.pendingScriptRefresh[paneID] {
		return false
	}
	delete(c.pendingScriptRefresh, paneID)
	return true
}

func (c *ContentCoordinator) refreshPendingScripts(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) {
	if wv == nil || wv.IsDestroyed() || c.shouldDeferAppearance(wv) {
		return
	}
	if !c.takePendingScriptRefresh(paneID) {
		return
	}
	if c.injector == nil {
		return
	}
	c.refreshInjectedScripts(ctx, c.injector, paneID, wv)
}

func (c *ContentCoordinator) shouldDeferAppearance(wv *webkit.WebView) bool {
	if wv == nil || wv.IsDestroyed() {
		return false
	}
	if wv.IsLoading() {
		return true
	}
	return wv.EstimatedProgress() < 1.0
}

func (c *ContentCoordinator) refreshInjectedScripts(
	ctx context.Context,
	injector *webkit.ContentInjector,
	paneID entity.PaneID,
	wv *webkit.WebView,
) {
	if injector == nil || wv == nil || wv.IsDestroyed() {
		return
	}
	ucm := wv.UserContentManager()
	if ucm == nil {
		return
	}
	ucm.RemoveAllScripts()
	ucm.RemoveAllStyleSheets()
	injector.InjectScripts(ctx, ucm, wv.ID())
	logging.FromContext(ctx).Debug().Str("pane_id", string(paneID)).Msg("refreshed injected scripts for webview")
}

func (c *ContentCoordinator) clearPendingAppearance(paneID entity.PaneID) {
	c.appearanceMu.Lock()
	if c.pendingScriptRefresh != nil {
		delete(c.pendingScriptRefresh, paneID)
	}
	if c.pendingThemePanes != nil {
		delete(c.pendingThemePanes, paneID)
		if len(c.pendingThemePanes) == 0 {
			c.hasPendingThemeUpdate = false
		}
	}
	c.appearanceMu.Unlock()
}

// GetTitle returns the current title for a pane.
func (c *ContentCoordinator) GetTitle(paneID entity.PaneID) string {
	c.titleMu.RLock()
	defer c.titleMu.RUnlock()
	return c.paneTitles[paneID]
}

// onTitleChanged updates title tracking when a WebView's title changes.
func (c *ContentCoordinator) onTitleChanged(ctx context.Context, paneID entity.PaneID, title string) {
	log := logging.FromContext(ctx)

	// Update title map
	c.titleMu.Lock()
	c.paneTitles[paneID] = title
	c.titleMu.Unlock()

	// Update domain model and check if this is the active pane
	isActivePaneTitle := false
	ws, wsView := c.getActiveWS()
	if ws != nil {
		paneNode := ws.FindPane(paneID)
		if paneNode != nil && paneNode.Pane != nil {
			paneNode.Pane.Title = title
		}
		// Check if this pane is the active one
		if ws.ActivePaneID == paneID {
			isActivePaneTitle = true
		}
	}

	// Update StackedView title bar if this pane is in a stack
	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(paneID))
			if stackedView != nil {
				c.updateStackedPaneTitle(ctx, stackedView, paneID, title)
			}
		}
	}

	// Notify history persistence (get URL from WebView)
	if c.onTitleUpdated != nil {
		if wv := c.getWebViewLocked(paneID); wv != nil {
			currentURI := wv.URI()
			if currentURI != "" && title != "" {
				c.onTitleUpdated(ctx, paneID, currentURI, title)
			}
		}
	}

	// Notify window title update if this is the active pane
	if isActivePaneTitle && c.onWindowTitleChanged != nil {
		c.onWindowTitleChanged(title)
	}

	log.Debug().
		Str("pane_id", string(paneID)).
		Str("title", title).
		Msg("pane title updated")
}

// updateStackedPaneTitle updates the title of a pane in a StackedView.
func (c *ContentCoordinator) updateStackedPaneTitle(
	ctx context.Context,
	sv *layout.StackedView,
	paneID entity.PaneID,
	title string,
) {
	log := logging.FromContext(ctx)

	// Find the pane's index directly in the StackedView
	index := sv.FindPaneIndex(string(paneID))
	if index < 0 {
		log.Debug().
			Str("pane_id", string(paneID)).
			Msg("pane not found in StackedView for title update")
		return
	}

	if err := sv.UpdateTitle(index, title); err != nil {
		log.Warn().Err(err).Int("index", index).Msg("failed to update stacked pane title")
	}
}

// syncStackedTitle updates the stacked title bar for a pane if it's in a stack.
// Called from onLoadCommitted to keep titles in sync during navigation.
func (c *ContentCoordinator) syncStackedTitle(ctx context.Context, paneID entity.PaneID, title string) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}
	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}
	if sv := tr.GetStackedViewForPane(string(paneID)); sv != nil {
		c.updateStackedPaneTitle(ctx, sv, paneID, title)
	}
}

// onFaviconChanged updates favicon tracking when a WebView's favicon changes.
func (c *ContentCoordinator) onFaviconChanged(ctx context.Context, paneID entity.PaneID, favicon *gdk.Texture) {
	log := logging.FromContext(ctx)

	// Get current URI to extract domain for caching
	wv := c.getWebViewLocked(paneID)
	if wv == nil {
		return
	}
	uri := wv.URI()

	// Update favicon cache with domain key (handles cross-domain redirects)
	if c.faviconAdapter != nil && favicon != nil && uri != "" {
		c.navOriginMu.RLock()
		originURL := c.navOrigins[paneID]
		c.navOriginMu.RUnlock()
		c.faviconAdapter.StoreFromWebKitWithOrigin(ctx, uri, originURL, favicon)
	}

	// Update StackedView favicon if this pane is in a stack
	_, wsView := c.getActiveWS()
	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(paneID))
			if stackedView != nil {
				c.updateStackedPaneFavicon(ctx, stackedView, paneID, favicon)
			}
		}
	}

	log.Debug().
		Str("pane_id", string(paneID)).
		Str("uri", uri).
		Bool("has_favicon", favicon != nil).
		Msg("pane favicon updated")
}

// updateStackedPaneFavicon updates the favicon of a pane in a StackedView.
func (c *ContentCoordinator) updateStackedPaneFavicon(
	ctx context.Context,
	sv *layout.StackedView,
	paneID entity.PaneID,
	favicon *gdk.Texture,
) {
	log := logging.FromContext(ctx)

	// Find the pane's index directly in the StackedView
	index := sv.FindPaneIndex(string(paneID))
	if index < 0 {
		log.Debug().
			Str("pane_id", string(paneID)).
			Msg("pane not found in StackedView for favicon update")
		return
	}

	if err := sv.UpdateFaviconTexture(index, favicon); err != nil {
		log.Warn().Err(err).Int("index", index).Msg("failed to update stacked pane favicon")
	}
}

// FaviconAdapter returns the favicon adapter for external use (e.g., omnibox).
func (c *ContentCoordinator) FaviconAdapter() *adapter.FaviconAdapter {
	return c.faviconAdapter
}

// SetNavigationOrigin records the original URL before navigation starts.
// This allows caching favicons under both original and final domains
// when cross-domain redirects occur (e.g., google.fr → google.com).
func (c *ContentCoordinator) SetNavigationOrigin(paneID entity.PaneID, uri string) {
	c.navOriginMu.Lock()
	c.navOrigins[paneID] = uri
	c.navOriginMu.Unlock()
}

// PreloadCachedFavicon checks the favicon cache and updates the stacked pane
// title bar immediately if a cached favicon exists for the URL.
// This provides instant favicon display without waiting for WebKit.
func (c *ContentCoordinator) PreloadCachedFavicon(ctx context.Context, paneID entity.PaneID, uri string) {
	if c.faviconAdapter == nil || uri == "" {
		return
	}

	// Check memory and disk cache (no external fetch)
	texture := c.faviconAdapter.PreloadFromCache(uri)

	// Update stacked pane favicon if applicable.
	// A nil texture triggers the default icon fallback, which avoids stale favicons.
	_, wsView := c.getActiveWS()
	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(paneID))
			if stackedView != nil {
				c.updateStackedPaneFavicon(ctx, stackedView, paneID, texture)
			}
		}
	}
}

// onLoadCommitted re-applies zoom when page content starts loading and records history.
// WebKit may reset zoom during document transitions, so we reapply after LoadCommitted.
// History is recorded here because the URI is guaranteed to be correct after commit.
// Also shows the WebView widget (it's hidden during creation to avoid white flash).
func (c *ContentCoordinator) onLoadCommitted(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) {
	log := logging.FromContext(ctx)
	logging.Trace().Mark("load_committed")

	uri := wv.URI()
	if uri == "" {
		return
	}

	// Set appropriate background color based on page type to prevent dark background bleeding.
	switch {
	case strings.HasPrefix(uri, "dumb://"):
		// Internal pages: apply themed background
		theme, ok := c.getCurrentTheme()
		if ok && theme.prefersDark {
			wv.SetBackgroundColor(darkBgR, darkBgG, darkBgB, darkBgA)
		} else {
			wv.ResetBackgroundToDefault()
		}
	case strings.HasPrefix(uri, "about:"):
		// Keep pool background (no action)
	default:
		// External pages: white background
		wv.ResetBackgroundToDefault()
	}

	// Show the WebView now that content is being painted
	// (WebViews are hidden on creation to avoid white flash)
	// Skip showing if this is about:blank but the pane is loading a different URL
	// This prevents the brief flash of about:blank during initial navigation
	shouldShow := true
	if uri == aboutBlankURI {
		// Get the pane's intended URI from the workspace
		ws, _ := c.getActiveWS()
		if ws != nil {
			if paneNode := ws.FindPane(paneID); paneNode != nil && paneNode.Pane != nil {
				// Don't show about:blank if the pane is supposed to load a different URL
				if paneNode.Pane.URI != "" && paneNode.Pane.URI != aboutBlankURI {
					shouldShow = false
					log.Debug().
						Str("pane_id", string(paneID)).
						Str("pane_uri", paneNode.Pane.URI).
						Msg("skipping webview show for about:blank (pane loading different URL)")
				}
			}
		}
	}

	if !shouldShow {
		// Avoid updating UI/domain state to about:blank when we know the pane is
		// navigating to a different URL. This prevents the omnibox/window title from
		// briefly showing about:blank on cold start.
		c.clearPendingReveal(paneID)
		return
	}

	if !c.applyPendingThemeUpdate(ctx, paneID, wv) {
		c.applyCurrentTheme(ctx, paneID, wv)
	}

	c.markPendingReveal(paneID)
	if wv.EstimatedProgress() > 0 {
		c.revealIfPending(ctx, paneID, uri, "progress-after-commit")
	}

	// Update domain model with current URI for session snapshots
	c.updatePaneURI(paneID, uri)

	// Sync StackedView title bar with the WebView's current title.
	// This keeps the stacked title bar up-to-date immediately on navigation,
	// before the asynchronous notify::title signal fires.
	if title := wv.Title(); title != "" {
		c.syncStackedTitle(ctx, paneID, title)
	}

	// Record history - URI is guaranteed to be correct at LoadCommitted
	if c.onHistoryRecord != nil {
		c.onHistoryRecord(ctx, paneID, uri)
	}

	// Notify active pane navigation for permission indicator reset.
	c.notifyActiveNavigation(paneID, uri)

	// Apply zoom
	if c.zoomUC == nil {
		return
	}

	domain, err := usecase.ExtractDomain(uri)
	if err != nil {
		return
	}

	_ = c.zoomUC.ApplyToWebView(ctx, wv, domain)
}

func (c *ContentCoordinator) notifyActiveNavigation(paneID entity.PaneID, uri string) {
	if c.onActiveNavigationCommitted == nil {
		return
	}
	ws, _ := c.getActiveWS()
	if ws != nil && ws.ActivePaneID == paneID {
		c.onActiveNavigationCommitted(uri)
	}
}

func (c *ContentCoordinator) shouldSkipAboutBlankAppearance(paneID entity.PaneID, wv *webkit.WebView) bool {
	if wv == nil || wv.IsDestroyed() {
		return false
	}
	if wv.URI() != aboutBlankURI {
		return false
	}
	ws, _ := c.getActiveWS()
	if ws == nil {
		return false
	}
	paneNode := ws.FindPane(paneID)
	if paneNode == nil || paneNode.Pane == nil {
		return false
	}
	if paneNode.Pane.URI != "" && paneNode.Pane.URI != aboutBlankURI {
		return true
	}
	return false
}

// onSPANavigation records history when URL changes via JavaScript (History API).
// This handles SPA navigation like YouTube search, where the URL changes without a page load.
func (c *ContentCoordinator) onSPANavigation(ctx context.Context, paneID entity.PaneID, uri string) {
	// Update domain model with current URI for session snapshots
	c.updatePaneURI(paneID, uri)

	// Record history for SPA navigation
	if c.onHistoryRecord != nil {
		c.onHistoryRecord(ctx, paneID, uri)
	}
}

// updatePaneURI updates the pane's URI in the domain model.
// This is called on navigation so that session snapshots capture the current URL.
func (c *ContentCoordinator) updatePaneURI(paneID entity.PaneID, uri string) {
	if c.onPaneURIUpdated != nil {
		c.onPaneURIUpdated(paneID, uri)
	}
}

// onLoadStarted shows the progress bar when page loading begins.
func (c *ContentCoordinator) onLoadStarted(paneID entity.PaneID) {
	logging.Trace().Mark("load_started")

	// Trigger deferred initialization on first load_started.
	// This ensures non-critical init runs after initial navigation starts.
	c.loadStartedOnce.Do(func() {
		if c.onFirstLoadStarted != nil {
			c.onFirstLoadStarted()
		}
	})

	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView != nil {
		paneView.SetLoading(true)
	}
}

// onLoadFinished hides the progress bar when page loading completes.
func (c *ContentCoordinator) onLoadFinished(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView != nil {
		paneView.SetLoading(false)
	}

	c.revealIfPending(context.Background(), paneID, "", "load-finished")
	if c.shouldSkipAboutBlankAppearance(paneID, wv) {
		return
	}
	c.applyPendingThemeUpdate(ctx, paneID, wv)
	c.refreshPendingScripts(ctx, paneID, wv)
}

// onProgressChanged updates the progress bar with current load progress.
func (c *ContentCoordinator) onProgressChanged(paneID entity.PaneID, progress float64) {
	if progress > 0 {
		c.revealIfPending(context.Background(), paneID, "", "progress")
	}

	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView != nil {
		paneView.SetLoadProgress(progress)
	}
}

func (c *ContentCoordinator) markPendingReveal(paneID entity.PaneID) {
	c.revealMu.Lock()
	c.pendingReveal[paneID] = true
	c.revealMu.Unlock()
}

func (c *ContentCoordinator) clearPendingReveal(paneID entity.PaneID) {
	c.revealMu.Lock()
	delete(c.pendingReveal, paneID)
	c.revealMu.Unlock()
}

func (c *ContentCoordinator) revealIfPending(ctx context.Context, paneID entity.PaneID, uri, reason string) {
	c.revealMu.Lock()
	pending := c.pendingReveal[paneID]
	if pending {
		delete(c.pendingReveal, paneID)
	}
	c.revealMu.Unlock()

	if !pending {
		return
	}

	wv := c.getWebViewLocked(paneID)
	if wv == nil || wv.IsDestroyed() {
		return
	}

	if inner := wv.Widget(); inner != nil {
		inner.SetVisible(true)
		logging.FromContext(ctx).
			Debug().
			Str("pane_id", string(paneID)).
			Str("uri", uri).
			Str("reason", reason).
			Msg("webview revealed")
	}

	// Mark first_paint and finish startup trace
	logging.Trace().Mark("first_paint")
	logging.Trace().Finish()

	if c.onWebViewShown != nil {
		c.onWebViewShown(paneID)
	}
}

// onLinkHover updates the link status overlay when hovering over links.
func (c *ContentCoordinator) onLinkHover(paneID entity.PaneID, uri string) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView == nil {
		return
	}

	if uri != "" {
		paneView.ShowLinkStatus(uri)
	} else {
		paneView.HideLinkStatus()
	}
}

// --- Popup Handling ---

// SetPopupConfig configures popup handling.
func (c *ContentCoordinator) SetPopupConfig(
	factory *webkit.WebViewFactory,
	popupConfig *config.PopupBehaviorConfig,
	generateID func() string,
) {
	c.factory = factory
	c.popupConfig = popupConfig
	c.generateID = generateID
}

// SetOnInsertPopup sets the callback to insert popups into the workspace.
func (c *ContentCoordinator) SetOnInsertPopup(fn func(ctx context.Context, input InsertPopupInput) error) {
	c.onInsertPopup = fn
}

// SetOnClosePane sets the callback to close a pane when its popup closes.
func (c *ContentCoordinator) SetOnClosePane(fn func(ctx context.Context, paneID entity.PaneID) error) {
	c.onClosePane = fn
}

// setupPopupHandling wires the popup create signal for a WebView.
// This should be called after acquiring a WebView for a pane.
func (c *ContentCoordinator) setupPopupHandling(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) {
	log := logging.FromContext(ctx)

	if wv == nil {
		return
	}

	// Wire the OnCreate callback for popup requests
	wv.OnCreate = func(req webkit.PopupRequest) *webkit.WebView {
		return c.handlePopupCreate(ctx, paneID, wv, req)
	}

	log.Debug().Str("pane_id", string(paneID)).Msg("popup handling configured for webview")
}

// createPopupPane creates a new pane entity for a popup window.
func (c *ContentCoordinator) createPopupPane(
	popupID port.WebViewID,
	parentPaneID entity.PaneID,
	targetURI string,
) (entity.PaneID, *entity.Pane) {
	var paneID entity.PaneID
	if c.generateID != nil {
		paneID = entity.PaneID(c.generateID())
	} else {
		paneID = entity.PaneID(fmt.Sprintf("popup_%d", popupID))
	}

	popupPane := entity.NewPane(paneID)
	popupPane.WindowType = entity.WindowPopup
	popupPane.IsRelated = true
	popupPane.ParentPaneID = &parentPaneID
	popupPane.URI = targetURI

	return paneID, popupPane
}

// handlePopupCreate handles the WebKit "create" signal for popup windows.
// Returns a new related WebView if popup is allowed, nil to block.
//
// IMPORTANT: The WebView MUST be added to a GtkWindow hierarchy BEFORE this
// signal handler returns. Otherwise WebKit won't establish window.opener.
// The WebView stays hidden until ready-to-show signal is received.
func (c *ContentCoordinator) handlePopupCreate(
	ctx context.Context,
	parentPaneID entity.PaneID,
	parentWV *webkit.WebView,
	req webkit.PopupRequest,
) *webkit.WebView {
	log := logging.FromContext(ctx)

	log.Debug().
		Str("parent_pane", string(parentPaneID)).
		Str("target_uri", req.TargetURI).
		Str("frame_name", req.FrameName).
		Bool("user_gesture", req.IsUserGesture).
		Msg("popup create request")

	// Check if popups are enabled
	if c.popupConfig != nil && !c.popupConfig.OpenInNewPane {
		log.Debug().Msg("popups disabled by config, blocking")
		return nil
	}

	// Check if factory is available
	if c.factory == nil {
		log.Warn().Msg("no webview factory, cannot create popup")
		return nil
	}

	// Create related WebView for session sharing (created hidden)
	parentID := parentWV.ID()
	popupWV, err := c.factory.CreateRelated(ctx, parentID)
	if err != nil {
		log.Error().Err(err).Msg("failed to create related webview for popup")
		return nil
	}

	// Detect popup type
	popupType := DetectPopupType(req.FrameName)
	popupID := popupWV.ID()

	// Determine behavior from config
	behavior := GetBehavior(popupType, c.popupConfig)
	placement := "right"
	if c.popupConfig != nil {
		placement = c.popupConfig.Placement
	}

	// Create popup pane entity
	paneID, popupPane := c.createPopupPane(popupID, parentPaneID, req.TargetURI)

	// Check OAuth configuration
	hasConfig := c.popupConfig != nil
	oauthEnabled := hasConfig && c.popupConfig.OAuthAutoClose
	isOAuth := IsOAuthURL(req.TargetURI)
	log.Debug().
		Bool("has_config", hasConfig).
		Bool("oauth_enabled", oauthEnabled).
		Bool("is_oauth", isOAuth).
		Str("uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
		Msg("popup OAuth check")

	// Insert into workspace IMMEDIATELY (WebView stays hidden)
	// This is required for WebKit to establish window.opener relationship
	if c.onInsertPopup != nil {
		popupInput := InsertPopupInput{
			ParentPaneID: parentPaneID,
			PopupPane:    popupPane,
			WebView:      popupWV,
			Behavior:     behavior,
			Placement:    placement,
			PopupType:    popupType,
			TargetURI:    req.TargetURI,
		}

		if err := c.onInsertPopup(ctx, popupInput); err != nil {
			log.Error().Err(err).Msg("failed to insert popup into workspace")
			popupWV.Destroy()
			return nil
		}
	}

	// Register WebView in our map (after successful insertion)
	c.setWebViewLocked(paneID, popupWV)

	// Setup standard callbacks (after successful insertion to avoid leak)
	c.setupWebViewCallbacks(ctx, paneID, popupWV)

	// Setup OAuth auto-close if configured
	if hasConfig && oauthEnabled && isOAuth {
		popupPane.AutoClose = true
		c.trackOAuthPopup(popupID, parentPaneID)
		c.setupOAuthAutoClose(ctx, paneID, popupID, popupWV)
		log.Debug().Str("pane_id", string(paneID)).Msg("OAuth auto-close enabled for popup")
	}

	// Store pending popup for ready-to-show handling (just visibility now)
	pending := &PendingPopup{
		WebView:         popupWV,
		ParentPaneID:    parentPaneID,
		ParentWebViewID: parentID,
		TargetURI:       req.TargetURI,
		FrameName:       req.FrameName,
		IsUserGesture:   req.IsUserGesture,
		PopupType:       popupType,
		CreatedAt:       time.Now(),
	}

	c.popupMu.Lock()
	c.pendingPopups[popupID] = pending
	c.popupMu.Unlock()

	// Wire ready-to-show (now just for visibility) and close signals
	popupWV.OnReadyToShow = func() {
		c.handlePopupReadyToShow(ctx, popupID)
	}
	popupWV.OnClose = composeOnClose(popupWV.OnClose, func() {
		c.handlePopupClose(ctx, popupID)
	})

	log.Info().
		Uint64("popup_id", uint64(popupID)).
		Str("pane_id", string(paneID)).
		Str("popup_type", popupType.String()).
		Str("target_uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
		Msg("popup inserted (hidden), awaiting ready-to-show for visibility")

	return popupWV
}

// handlePopupReadyToShow handles the WebKit "ready-to-show" signal.
// The popup WebView was already inserted into the workspace (hidden) during
// the create signal. This handler just makes it visible.
func (c *ContentCoordinator) handlePopupReadyToShow(ctx context.Context, popupID port.WebViewID) {
	log := logging.FromContext(ctx)

	// Get pending popup
	c.popupMu.Lock()
	pending, ok := c.pendingPopups[popupID]
	if ok {
		delete(c.pendingPopups, popupID)
	}
	c.popupMu.Unlock()

	if !ok || pending == nil {
		log.Warn().Uint64("popup_id", uint64(popupID)).Msg("ready-to-show for unknown popup")
		return
	}

	log.Debug().
		Uint64("popup_id", uint64(popupID)).
		Str("popup_type", pending.PopupType.String()).
		Msg("popup ready to show - making visible")

	// Make the WebView visible now that it's ready
	if pending.WebView != nil {
		pending.WebView.Show()
	}

	log.Info().
		Uint64("popup_id", uint64(popupID)).
		Str("target_uri", logging.TruncateURL(pending.TargetURI, logURLMaxLen)).
		Msg("popup now visible")
}

// handlePopupClose handles the WebKit "close" signal for popup windows.
func (c *ContentCoordinator) handlePopupClose(ctx context.Context, popupID port.WebViewID) {
	log := logging.FromContext(ctx)
	log.Debug().Uint64("popup_id", uint64(popupID)).Msg("popup close signal received")

	// Check if still pending (never shown)
	c.popupMu.Lock()
	pending, wasPending := c.pendingPopups[popupID]
	if wasPending {
		delete(c.pendingPopups, popupID)
	}
	c.popupMu.Unlock()

	if wasPending && pending != nil {
		c.handlePopupOAuthClose(ctx, popupID)
		// Was never shown, just destroy
		pending.WebView.Destroy()
		log.Debug().Msg("destroyed pending popup that was never shown")
		return
	}

	// Find pane by WebView ID
	var paneID entity.PaneID
	for pid, wv := range c.webViews {
		if wv != nil && wv.ID() == popupID {
			paneID = pid
			break
		}
	}

	if paneID == "" {
		c.handlePopupOAuthClose(ctx, popupID)
		log.Warn().Msg("popup close: could not find pane for webview")
		return
	}

	// Close the pane in workspace (this removes the UI element)
	if c.onClosePane != nil {
		if err := c.onClosePane(ctx, paneID); err != nil {
			log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to close popup pane")
		}
	}

	c.handlePopupOAuthClose(ctx, popupID)

	// Release the WebView (this will clean up tracking)
	c.ReleaseWebView(ctx, paneID)

	log.Info().Str("pane_id", string(paneID)).Msg("popup closed")
}

// setupWebViewCallbacks configures standard callbacks and popup handling.
func (c *ContentCoordinator) setupWebViewCallbacks(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) {
	log := logging.FromContext(ctx)

	// Title changes
	wv.OnTitleChanged = func(title string) {
		c.onTitleChanged(ctx, paneID, title)
	}

	// Favicon changes
	wv.OnFaviconChanged = func(favicon *gdk.Texture) {
		c.onFaviconChanged(ctx, paneID, favicon)
	}

	// Load events
	wv.OnLoadChanged = func(event webkit.LoadEvent) {
		switch event {
		case webkit.LoadStarted:
			c.onLoadStarted(paneID)
		case webkit.LoadCommitted:
			c.onLoadCommitted(ctx, paneID, wv)
		case webkit.LoadFinished:
			c.onLoadFinished(ctx, paneID, wv)
		}
	}

	// Progress
	wv.OnProgressChanged = func(progress float64) {
		c.onProgressChanged(paneID, progress)
	}

	// SPA navigation and external scheme handling
	wv.OnURIChanged = func(uri string) {
		if uri == "" {
			return
		}

		// Check for external URL schemes (vscode://, vscode-insiders://, spotify://, etc.)
		// These are typically triggered by JavaScript redirects (window.location)
		isExternal := urlutil.IsExternalScheme(uri)

		if isExternal {
			log.Info().Str("pane_id", string(paneID)).Str("uri", uri).Msg("external scheme detected, launching externally")

			// Launch externally
			desktop.LaunchExternalURL(uri)

			// Stop loading to prevent WebKit from showing an error page
			// The page stays on the previous URL before the JS redirect
			_ = wv.Stop(ctx)

			// Navigate back to avoid stale URI in omnibox/history
			if wv.CanGoBack() {
				_ = wv.GoBack(ctx)
			}
			return
		}

		if !wv.IsLoading() {
			c.onSPANavigation(ctx, paneID, uri)
		}
	}

	// Middle-click / Ctrl+click handler for opening links in new pane
	wv.OnLinkMiddleClick = func(uri string) bool {
		return c.handleLinkMiddleClick(ctx, paneID, uri)
	}

	// Link hover callback for status overlay
	wv.OnLinkHover = func(uri string) {
		c.onLinkHover(paneID, uri)
	}

	wv.OnWebProcessTerminated = func(reason webkitlib.WebProcessTerminationReason, reasonLabel string, uri string) {
		originalURI := extractOriginalURIFromCrashPage(uri)
		if !shouldRenderCrashPage(reason) {
			log.Info().
				Str("pane_id", string(paneID)).
				Str("reason", reasonLabel).
				Str("uri", uri).
				Msg("web process termination handled without crash page")
			return
		}

		crashURI := buildCrashPageURI(originalURI)
		log.Warn().
			Str("pane_id", string(paneID)).
			Str("reason", reasonLabel).
			Str("uri", uri).
			Str("crash_uri", crashURI).
			Msg("web process terminated, redirecting to crash page")

		if err := wv.LoadURI(ctx, crashURI); err != nil {
			log.Error().
				Err(err).
				Str("pane_id", string(paneID)).
				Str("reason", reasonLabel).
				Str("uri", uri).
				Str("crash_uri", crashURI).
				Msg("failed to load crash page after web process termination")
		}
	}

	// Permission request handler
	wv.OnPermissionRequest = func(origin string, permTypes []string, allow, deny func()) bool {
		return c.handlePermissionRequest(ctx, origin, permTypes, allow, deny)
	}
	// Fullscreen handlers for idle inhibition
	c.setupIdleInhibitionHandlers(ctx, paneID, wv)

	// Popup handling for nested popups
	c.setupPopupHandling(ctx, paneID, wv)
}

// setupOAuthAutoClose monitors the popup for OAuth callback URLs and auto-closes.
// It uses URL pattern detection for standard OAuth callbacks (code=, access_token=, etc.)
// For providers using postMessage (like Google Sign-In), we rely on the provider calling
// window.close() which triggers WebKit's close signal.
// A long safety timeout (30s) catches popups that get stuck.
func (c *ContentCoordinator) setupOAuthAutoClose(
	ctx context.Context,
	paneID entity.PaneID,
	popupID port.WebViewID,
	wv *webkit.WebView,
) {
	log := logging.FromContext(ctx)

	// Safety timeout - only triggers if popup gets stuck (provider should close via window.close)
	var safetyTimer *time.Timer
	var safetyTimerMu sync.Mutex
	var cancelSafetyTimerOnce sync.Once
	var requestCloseOnce sync.Once
	const oauthSafetyTimeout = 30 * time.Second

	startSafetyTimer := func() {
		safetyTimerMu.Lock()
		defer safetyTimerMu.Unlock()
		if safetyTimer != nil {
			safetyTimer.Stop()
		}
		safetyTimer = time.AfterFunc(oauthSafetyTimeout, func() {
			if wv != nil && !wv.IsDestroyed() {
				log.Warn().Str("pane", string(paneID)).Msg("oauth safety timeout, closing stuck popup")
				wv.Close()
			}
		})
	}

	cancelSafetyTimer := func() {
		cancelSafetyTimerOnce.Do(func() {
			safetyTimerMu.Lock()
			defer safetyTimerMu.Unlock()
			if safetyTimer != nil {
				safetyTimer.Stop()
				safetyTimer = nil
			}
		})
	}

	requestOAuthClose := func(uri string, reason string) {
		c.capturePopupOAuthState(popupID, uri)
		cancelSafetyTimer()
		log.Info().
			Str("pane", string(paneID)).
			Str("reason", reason).
			Msg("oauth callback detected, closing")
		requestCloseOnce.Do(func() {
			go func() {
				time.Sleep(500 * time.Millisecond)
				if wv != nil && !wv.IsDestroyed() {
					wv.Close()
				}
			}()
		})
	}

	// Start safety timer immediately.
	startSafetyTimer()

	// Wrap OnURIChanged to check for OAuth callbacks.
	wv.OnURIChanged = composeOnURIChanged(wv.OnURIChanged, func(uri string) {
		if ShouldAutoClose(uri) {
			requestOAuthClose(uri, "uri_changed")
		}
	})

	// Monitor load events for URL-based detection.
	wv.OnLoadChanged = composeOnLoadChanged(wv.OnLoadChanged, func(event webkit.LoadEvent) {
		if event == webkit.LoadCommitted {
			uri := wv.URI()
			if ShouldAutoClose(uri) {
				requestOAuthClose(uri, "load_committed")
			}
		}
	})

	// Cancel safety timer on any close path.
	wv.OnClose = composeOnClose(func() {
		cancelSafetyTimer()
	}, wv.OnClose)
}

func (c *ContentCoordinator) trackOAuthPopup(popupID port.WebViewID, parentPaneID entity.PaneID) {
	c.popupMu.Lock()
	defer c.popupMu.Unlock()
	if c.popupOAuth == nil {
		c.popupOAuth = make(map[port.WebViewID]*popupOAuthState)
	}
	c.popupOAuth[popupID] = &popupOAuthState{
		ParentPaneID: parentPaneID,
	}
}

func (c *ContentCoordinator) capturePopupOAuthState(popupID port.WebViewID, uri string) {
	c.popupMu.Lock()
	defer c.popupMu.Unlock()

	state, ok := c.popupOAuth[popupID]
	if !ok {
		return
	}

	state.Seen = true
	state.CallbackURI = uri
	state.Success = IsOAuthSuccess(uri)
	state.Error = IsOAuthError(uri)
}

func (c *ContentCoordinator) handlePopupOAuthClose(ctx context.Context, popupID port.WebViewID) {
	log := logging.FromContext(ctx)

	c.popupMu.Lock()
	state, ok := c.popupOAuth[popupID]
	if ok {
		delete(c.popupOAuth, popupID)
	}
	c.popupMu.Unlock()

	if !ok || state == nil || !state.Seen {
		return
	}

	log.Debug().
		Uint64("popup_id", uint64(popupID)).
		Str("parent_pane_id", string(state.ParentPaneID)).
		Bool("oauth_success", state.Success).
		Bool("oauth_error", state.Error).
		Msg("popup oauth result captured on close")

	if !state.Success || state.ParentPaneID == "" {
		return
	}

	c.scheduleParentPaneRefresh(ctx, state.ParentPaneID, popupID)
}

func (c *ContentCoordinator) scheduleParentPaneRefresh(
	ctx context.Context,
	parentPaneID entity.PaneID,
	popupID port.WebViewID,
) {
	c.popupMu.Lock()
	if c.popupRefresh == nil {
		c.popupRefresh = make(map[entity.PaneID]*time.Timer)
	}
	if existing := c.popupRefresh[parentPaneID]; existing != nil {
		existing.Stop()
	}
	c.popupRefresh[parentPaneID] = time.AfterFunc(oauthParentRefreshDebounce, func() {
		c.popupMu.Lock()
		delete(c.popupRefresh, parentPaneID)
		c.popupMu.Unlock()
		cb := glib.SourceFunc(func(_ uintptr) bool {
			c.refreshPaneAfterOAuth(ctx, parentPaneID, popupID)
			return false
		})
		glib.IdleAdd(&cb, 0)
	})
	c.popupMu.Unlock()
}

func (c *ContentCoordinator) refreshPaneAfterOAuth(
	ctx context.Context,
	parentPaneID entity.PaneID,
	popupID port.WebViewID,
) {
	log := logging.FromContext(ctx)
	wv := c.getWebViewLocked(parentPaneID)
	if wv == nil || wv.IsDestroyed() {
		log.Debug().
			Str("parent_pane_id", string(parentPaneID)).
			Uint64("popup_id", uint64(popupID)).
			Msg("skipping parent pane refresh after oauth close: parent webview unavailable")
		return
	}

	if err := wv.Reload(ctx); err != nil {
		log.Warn().
			Err(err).
			Str("parent_pane_id", string(parentPaneID)).
			Uint64("popup_id", uint64(popupID)).
			Msg("failed parent pane refresh after oauth popup close")
		return
	}

	log.Info().
		Str("parent_pane_id", string(parentPaneID)).
		Uint64("popup_id", uint64(popupID)).
		Msg("refreshed parent pane after oauth popup success")
}

// handleLinkMiddleClick handles middle-click / Ctrl+click on links.
// Opens the link in a new pane using blank_target_behavior config.
func (c *ContentCoordinator) handleLinkMiddleClick(ctx context.Context, parentPaneID entity.PaneID, uri string) bool {
	log := logging.FromContext(ctx)

	log.Info().
		Str("parent_pane", string(parentPaneID)).
		Str("uri", uri).
		Msg("middle-click/ctrl+click on link")

	// Check if popups are enabled
	if c.popupConfig != nil && !c.popupConfig.OpenInNewPane {
		log.Debug().Msg("popups disabled by config, ignoring middle-click")
		return false
	}

	// Check if factory is available
	if c.factory == nil {
		log.Warn().Msg("no webview factory, cannot handle middle-click")
		return false
	}

	// Create a new WebView (regular, not related - just opening a link)
	newWV, err := c.factory.Create(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to create webview for middle-click")
		return false
	}

	// Generate pane ID
	var paneID entity.PaneID
	if c.generateID != nil {
		paneID = entity.PaneID(c.generateID())
	} else {
		paneID = entity.PaneID(fmt.Sprintf("link_%d", newWV.ID()))
	}

	// Create pane entity - uses blank_target_behavior
	newPane := entity.NewPane(paneID)
	newPane.WindowType = entity.WindowPopup
	newPane.URI = uri

	// Register WebView
	c.setWebViewLocked(paneID, newWV)

	// Setup callbacks
	c.setupWebViewCallbacks(ctx, paneID, newWV)

	// Get behavior from config (same as _blank links)
	behavior := config.PopupBehaviorStacked // default
	if c.popupConfig != nil && c.popupConfig.BlankTargetBehavior != "" {
		behavior = config.PopupBehavior(c.popupConfig.BlankTargetBehavior)
	}
	placement := "right"
	if c.popupConfig != nil {
		placement = c.popupConfig.Placement
	}

	// Request insertion into workspace
	if c.onInsertPopup != nil {
		popupInput := InsertPopupInput{
			ParentPaneID: parentPaneID,
			PopupPane:    newPane,
			WebView:      newWV,
			Behavior:     behavior,
			Placement:    placement,
			PopupType:    PopupTypeTab, // Treat like _blank
			TargetURI:    uri,
		}

		if err := c.onInsertPopup(ctx, popupInput); err != nil {
			log.Error().Err(err).Msg("failed to insert middle-click pane into workspace")
			// Clean up on failure
			delete(c.webViews, paneID)
			newWV.Destroy()
			return false
		}
	}

	// Load the URI after insertion
	if err := newWV.LoadURI(ctx, uri); err != nil {
		log.Error().Err(err).Str("uri", uri).Msg("failed to load URI in new pane")
	}

	log.Info().
		Str("pane_id", string(paneID)).
		Str("behavior", string(behavior)).
		Str("uri", uri).
		Msg("middle-click link opened in new pane")

	return true
}

// handlePermissionRequest processes media permission requests from WebKit.
// It delegates to the permission use case which handles auto-allow, stored permissions, and dialogs.
func (c *ContentCoordinator) handlePermissionRequest(
	ctx context.Context,
	origin string,
	permTypes []string,
	allow, deny func(),
) bool {
	log := logging.FromContext(ctx)

	// Convert string permission types to entity types
	entityTypes := make([]entity.PermissionType, 0, len(permTypes))
	for _, pt := range permTypes {
		switch pt {
		case "microphone":
			entityTypes = append(entityTypes, entity.PermissionTypeMicrophone)
		case "camera":
			entityTypes = append(entityTypes, entity.PermissionTypeCamera)
		case "display":
			entityTypes = append(entityTypes, entity.PermissionTypeDisplay)
		case "device_info":
			entityTypes = append(entityTypes, entity.PermissionTypeDeviceInfo)
		default:
			log.Warn().Str("type", pt).Msg("unknown permission type, skipping")
		}
	}

	if len(entityTypes) == 0 {
		log.Warn().Str("origin", origin).Msg("permission request with no valid types, denying")
		deny()
		return true
	}

	trackedTypes := filterWebRTCPermissionTypes(entityTypes)
	notifyActivity := func(state PermissionActivityState) {
		if c.onPermissionActivity == nil || len(trackedTypes) == 0 {
			return
		}
		c.onPermissionActivity(origin, trackedTypes, state)
	}

	notifyActivity(PermissionActivityRequesting)

	wrappedAllow := func() {
		notifyActivity(PermissionActivityAllowed)
		allow()
	}
	wrappedDeny := func() {
		notifyActivity(PermissionActivityBlocked)
		deny()
	}

	// Check if permission use case is available
	if c.permissionUC == nil {
		log.Warn().Str("origin", origin).Msg("no permission use case available, auto-allowing low-risk permissions")
		// Auto-allow display and device_info, deny others
		allAutoAllow := true
		for _, pt := range entityTypes {
			if !entity.IsAutoAllow(pt) {
				allAutoAllow = false
				break
			}
		}
		if allAutoAllow {
			wrappedAllow()
		} else {
			wrappedDeny()
		}
		return true
	}

	// Delegate to use case
	callback := usecase.PermissionCallback{
		Allow: wrappedAllow,
		Deny:  wrappedDeny,
	}

	c.permissionUC.HandlePermissionRequest(ctx, origin, entityTypes, callback)
	return true
}

func filterWebRTCPermissionTypes(types []entity.PermissionType) []entity.PermissionType {
	filtered := make([]entity.PermissionType, 0, len(types))
	for _, permType := range types {
		switch permType {
		case entity.PermissionTypeMicrophone, entity.PermissionTypeCamera, entity.PermissionTypeDisplay:
			filtered = append(filtered, permType)
		}
	}
	return filtered
}

// setupIdleInhibitionHandlers configures fullscreen and audio callbacks for idle inhibition.
// Idle is inhibited when:
// - The webview enters fullscreen mode (e.g., fullscreen video)
// - The webview is playing audio (e.g., video/music playback)
// The inhibitor uses refcounting, so both can be active simultaneously.
func (c *ContentCoordinator) setupIdleInhibitionHandlers(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) {
	log := logging.FromContext(ctx)

	if wv == nil {
		return
	}

	// Fullscreen handling
	wv.OnEnterFullscreen = func() bool {
		if c.idleInhibitor != nil {
			if err := c.idleInhibitor.Inhibit(ctx, "Fullscreen video playback"); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to inhibit idle")
			}
		}
		if c.onFullscreenChanged != nil {
			c.onFullscreenChanged(true)
		}
		return false // Allow fullscreen
	}

	wv.OnLeaveFullscreen = func() bool {
		if c.idleInhibitor != nil {
			if err := c.idleInhibitor.Uninhibit(ctx); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to uninhibit idle")
			}
		}
		if c.onFullscreenChanged != nil {
			c.onFullscreenChanged(false)
		}
		return false // Allow leaving fullscreen
	}

	// Audio playback handling
	wv.OnAudioStateChanged = func(playing bool) {
		if c.idleInhibitor == nil {
			return
		}
		if playing {
			if err := c.idleInhibitor.Inhibit(ctx, "Media playback"); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to inhibit idle for audio")
			}
		} else {
			if err := c.idleInhibitor.Uninhibit(ctx); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to uninhibit idle for audio")
			}
		}
	}
}

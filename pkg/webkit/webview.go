package webkit

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const turnstileHost = "challenges.cloudflare.com"

var (
	viewIDCounter uint64
	viewRegistry  = make(map[uint64]*WebView)
	viewMu        sync.RWMutex
)

// WebView wraps a WebKitGTK WebView
type WebView struct {
	view      *webkit.WebView
	container *gtk.Box // Container that holds the WebView
	window    *Window  // Optional window (only if CreateWindow=true)
	id        uint64

	// State
	config    *Config
	destroyed bool
	mu        sync.RWMutex

	// Event handlers
	onScriptMessage       func(string)
	onTitleChanged        func(string)
	onURIChanged          func(string)
	onFaviconChanged      func([]byte)
	onFaviconURIChanged   func(pageURI, faviconURI string)
	onZoomChanged         func(float64)
	onLoadCommitted       func(string)                            // Called when page load is committed (safe to apply zoom)
	onLoadStarted         func()                                  // Called when a load starts
	onLoadFinished        func()                                  // Called when a load finishes
	onLoadProgress        func(float64)                           // Called when estimated-load-progress changes
	onPopupCreate         func(*webkit.NavigationAction) *WebView // New WebKit create signal handler
	onReadyToShow         func()                                  // WebKit ready-to-show signal handler
	onClose               func()
	onNavigationPolicy    func(url string, isUserGesture bool) bool
	onWindowTypeDetected  func(WindowType, *WindowFeatures)
	onWorkspaceNavigation func(direction string) bool // Workspace pane navigation
	onPaneModeShortcut    func(action string) bool    // Pane mode shortcuts (enter, actions, exit)
	isPaneModeActive      func() bool                 // Check if pane mode is active
	onMiddleClickLink     func(url string) bool       // Middle mouse click on link handler
}

// NewWebView creates a new WebView with the given configuration
func NewWebView(cfg *Config) (*WebView, error) {
	if cfg == nil {
		cfg = GetDefaultConfig()
	}

	// Generate unique ID immediately
	id := atomic.AddUint64(&viewIDCounter, 1)
	log.Printf("[webkit] Generated WebView ID: %d (CreateWindow=%v)", id, cfg.CreateWindow)

	InitMainThread()

	// Initialize persistent session - REQUIRED, no ephemeral fallback
	// This must be done BEFORE creating the WebView
	if cfg.DataDir == "" || cfg.CacheDir == "" {
		return nil, fmt.Errorf("data and cache directories are required for persistent storage")
	}

	if err := InitPersistentSession(cfg.DataDir, cfg.CacheDir); err != nil {
		return nil, fmt.Errorf("failed to initialize persistent storage: %w", err)
	}

	// Create WebView - it will automatically use the persistent session
	// Per WebKitGTK 6.0: newly created session becomes the default for all WebViews
	wkView := webkit.NewWebView()
	if wkView == nil {
		return nil, ErrWebViewNotInitialized
	}

	// Set background color based on user's theme preference to prevent white/black flash
	// This must be done before any content loads
	var bg gdk.RGBA
	if PrefersDarkTheme() {
		bg = gdk.NewRGBA(0.11, 0.11, 0.11, 1.0) // #1c1c1c - dark background
	} else {
		bg = gdk.NewRGBA(0.96, 0.96, 0.96, 1.0) // #f5f5f5 - light background
	}
	wkView.SetBackgroundColor(&bg)

	// Verify the WebView is using the persistent session
	viewSession := wkView.NetworkSession()
	if viewSession == nil {
		return nil, fmt.Errorf("WebView has no network session")
	}
	if viewSession.IsEphemeral() {
		return nil, fmt.Errorf("WebView is using ephemeral session despite persistent session initialization")
	}

	// CRITICAL: Configure CookieManager on the WebView's actual session
	// This ensures cookies are persisted regardless of whether the WebView
	// uses our global session or creates its own
	cookieManager := viewSession.CookieManager()
	if cookieManager == nil {
		return nil, fmt.Errorf("failed to get cookie manager from WebView's network session")
	}
	cookiePath := cfg.DataDir + "/cookies.db"
	cookieManager.SetPersistentStorage(cookiePath, webkit.CookiePersistentStorageSqlite)
	cookieManager.SetAcceptPolicy(webkit.CookiePolicyAcceptNoThirdParty)

	// Setup UserContentManager and inject GUI scripts (including webview ID)
	// This must be done BEFORE any pages are loaded
	if err := SetupUserContentManager(wkView, cfg.AppearanceConfigJSON, id); err != nil {
		return nil, fmt.Errorf("failed to setup user content manager: %w", err)
	}

	// Create container (GtkBox) to hold the WebView
	// This allows the WebView to be reparented into workspace panes
	container := gtk.NewBox(gtk.OrientationVertical, 0)
	container.SetHExpand(true)
	container.SetVExpand(true)

	// Configure WebView widget for expansion
	wkView.SetHExpand(true)
	wkView.SetVExpand(true)

	// Add WebView to container
	container.Append(wkView)

	wv := &WebView{
		view:      wkView,
		container: container,
		window:    nil, // Will be set below if CreateWindow is true
		id:        id,
		config:    cfg,
	}

	// Apply configuration
	if err := wv.applyConfig(); err != nil {
		return nil, err
	}

	// Setup event handlers
	wv.setupEventHandlers()

	// Attach keyboard bridge to forward keyboard events to JavaScript
	wv.AttachKeyboardBridge()

	// Attach mouse gesture controls for hardware navigation buttons
	wv.AttachMouseGestures()

	// Attach touchpad gesture controls for swipe navigation
	wv.AttachTouchpadGestures()

	// Only create window if requested (standalone WebViews)
	// Workspace panes will set CreateWindow=false and manage the container themselves
	if cfg.CreateWindow {
		window, err := NewWindow("Dumber Browser")
		if err != nil {
			return nil, err
		}
		wv.window = window
		// Add container as child of window
		window.SetChild(container)
		log.Printf("[webkit] Created standalone window for WebView ID %d", id)
	} else {
		log.Printf("[webkit] Created WebView ID %d without window (for workspace embedding)", id)
	}

	// Register in global registry
	viewMu.Lock()
	viewRegistry[id] = wv
	viewMu.Unlock()

	return wv, nil
}

// applyConfig applies the configuration to the WebView settings
func (w *WebView) applyConfig() error {
	settings := w.view.Settings()
	if settings == nil {
		return fmt.Errorf("webkit: failed to get settings")
	}

	// Apply settings from config
	settings.SetEnableJavascript(w.config.EnableJavaScript)
	settings.SetEnableWebgl(w.config.EnableWebGL)
	settings.SetDefaultFontSize(uint32(w.config.DefaultFontSize))
	settings.SetMinimumFontSize(uint32(w.config.MinimumFontSize))

	if w.config.UserAgent != "" {
		settings.SetUserAgent(w.config.UserAgent)
	}

	// Enable developer tools (F12, inspector)
	settings.SetEnableDeveloperExtras(true)

	// Enable hardware acceleration if configured
	settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyAlways)

	// Performance optimizations for faster page transitions
	// Enable page cache for instant back/forward navigation (bfcache)
	settings.SetEnablePageCache(w.config.EnablePageCache)

	// Enable smooth scrolling for better UX
	settings.SetEnableSmoothScrolling(w.config.EnableSmoothScrolling)

	// Disable console messages to stdout to prevent page script flooding
	// This stops malicious/buggy pages from spamming the terminal with console.log()
	// Inspector/DevTools console (F12) will still show all messages
	settings.SetEnableWriteConsoleMessagesToStdout(false)

	return nil
}

// setupEventHandlers connects GTK signals to internal handlers
func (w *WebView) setupEventHandlers() {
	// Title changed - connect to notify::title signal
	w.view.Connect("notify::title", func() {
		if w.onTitleChanged != nil {
			title := w.view.Title()
			w.onTitleChanged(title)
		}
	})

	// URI changed - connect to notify::uri signal
	w.view.Connect("notify::uri", func() {
		if w.onURIChanged != nil {
			uri := w.view.URI()
			w.onURIChanged(uri)
		}
	})

	// Zoom level changed - connect to notify::zoom-level signal
	w.view.Connect("notify::zoom-level", func() {
		if w.onZoomChanged != nil {
			zoomLevel := w.view.ZoomLevel()
			w.onZoomChanged(zoomLevel)
		}
	})

	// Load committed - connect to load-changed signal for WEBKIT_LOAD_COMMITTED
	w.view.ConnectLoadChanged(func(loadEvent webkit.LoadEvent) {
		if loadEvent == webkit.LoadStarted && w.onLoadStarted != nil {
			w.onLoadStarted()
		}
		if loadEvent == webkit.LoadCommitted && w.onLoadCommitted != nil {
			uri := w.view.URI()
			w.onLoadCommitted(uri)
		}
		if loadEvent == webkit.LoadFinished && w.onLoadFinished != nil {
			w.onLoadFinished()
		}
	})

	// Estimated load progress - notify::estimated-load-progress property
	w.view.Connect("notify::estimated-load-progress", func() {
		if w.onLoadProgress != nil {
			w.onLoadProgress(w.view.EstimatedLoadProgress())
		}
	})

	// Favicon changed - connect to FaviconDatabase
	w.setupFaviconHandlers()

	// TLS error handling - connect to load-failed-with-tls-errors signal
	w.setupTLSErrorHandler()

	// Close
	w.view.ConnectClose(func() {
		if w.onClose != nil {
			w.onClose()
		}
	})

	// Create signal - for popup lifecycle management
	w.view.ConnectCreate(func(navigationAction *webkit.NavigationAction) gtk.Widgetter {
		log.Printf("[webkit] ConnectCreate callback fired for parent WebView ID=%d", w.id)
		if w.onPopupCreate != nil {
			log.Printf("[webkit] Calling onPopupCreate handler")
			newWebView := w.onPopupCreate(navigationAction)
			if newWebView != nil {
				log.Printf("[webkit] onPopupCreate returned WebView wrapper ID=%d", newWebView.id)
				log.Printf("[webkit] Extracting underlying gotk4 WebView from wrapper")
				underlyingView := newWebView.view
				log.Printf("[webkit] Extracted gotk4 WebView=%p, about to return to WebKit", underlyingView)
				// Return the underlying WebView widget to WebKit
				return underlyingView
			}
			log.Printf("[webkit] onPopupCreate returned nil, blocking popup")
		}
		// Return nil to cancel popup creation
		log.Printf("[webkit] No onPopupCreate handler, blocking popup")
		return nil
	})

	// Ready-to-show signal - emitted when popup is ready to be displayed
	w.view.ConnectReadyToShow(func() {
		if w.onReadyToShow != nil {
			w.onReadyToShow()
		}
	})

	// Script message received - connect to UserContentManager's script-message-received signal
	// This receives messages from JavaScript via webkit.messageHandlers.dumber.postMessage()
	// The signal name includes the handler name as a detail: "script-message-received::dumber"
	ucm := w.view.UserContentManager()
	if ucm != nil {
		// GTK signals pass the sender as the first parameter, the actual signal data as subsequent parameters
		ucm.Connect("script-message-received::dumber", func(sender interface{}, jscValue interface{}) {
			if w.onScriptMessage != nil && jscValue != nil {
				// Convert JSCValue to string using gotk4 javascriptcore bindings
				valueStr := JSCValueToString(jscValue)
				if valueStr != "" {
					w.onScriptMessage(valueStr)
				}
			}
		})
	}

	// Setup navigation policy handler for middle-click and Ctrl+click interception
	w.setupNavigationPolicyHandler()

	// Log CORP/CORP headers for Cloudflare Turnstile resources to help diagnose COEP issues
	w.attachTurnstileResponseLogger()

	// Apply temporary Turnstile workaround until WebKit supports COEP: credentialless
	w.applyTurnstileCorsAllowlist()
}

// setupNavigationPolicyHandler sets up the decide-policy signal handler
// This intercepts navigation actions BEFORE they occur, allowing us to detect
// middle-click and Ctrl+click on links and open them in new panes
func (w *WebView) setupNavigationPolicyHandler() {
	w.view.ConnectDecidePolicy(func(decision webkit.PolicyDecisioner, decisionType webkit.PolicyDecisionType) bool {
		// Only handle navigation actions (not new window or response policies)
		if decisionType != webkit.PolicyDecisionTypeNavigationAction {
			return false // Let WebKit handle
		}

		// Cast to NavigationPolicyDecision to access NavigationAction
		navDecision, ok := decision.(*webkit.NavigationPolicyDecision)
		if !ok {
			return false
		}

		navAction := navDecision.NavigationAction()
		if navAction == nil {
			return false
		}

		// Check if this is a link click
		if navAction.NavigationType() != webkit.NavigationTypeLinkClicked {
			return false // Not a link click, let WebKit handle normally
		}

		// Get mouse button and modifiers
		mouseButton := navAction.MouseButton()
		modifiers := navAction.Modifiers()

		// Check for middle-click (button 2) or Ctrl+left-click (button 1 + Ctrl)
		isMiddleClick := (mouseButton == 2)
		isCtrlClick := (mouseButton == 1 && (gdk.ModifierType(modifiers)&gdk.ControlMask) != 0)

		if !isMiddleClick && !isCtrlClick {
			return false // Normal click, let WebKit handle
		}

		// Get the link URL
		request := navAction.Request()
		if request == nil {
			return false
		}

		linkURL := request.URI()
		if linkURL == "" {
			return false
		}

		clickType := "middle-click"
		if isCtrlClick {
			clickType = "Ctrl+click"
		}
		log.Printf("[navigation-policy] Detected %s on link: %s", clickType, linkURL)

		// Call the registered handler
		w.mu.RLock()
		handler := w.onMiddleClickLink
		w.mu.RUnlock()

		if handler != nil {
			handled := handler(linkURL)
			if handled {
				log.Printf("[navigation-policy] %s handled by workspace, blocking navigation", clickType)
				// Block the navigation by calling Ignore()
				navDecision.Ignore()
				return true // Prevent default behavior
			}
		}

		return false // Let WebKit handle if not handled
	})

	log.Printf("[webkit] Navigation policy handler attached to WebView ID %d", w.id)
}

func (w *WebView) applyTurnstileCorsAllowlist() {
	if w.view == nil || w.config == nil || !w.config.EnableTurnstileWorkaround {
		return
	}

	pattern := fmt.Sprintf("https://%s/*", turnstileHost)
	w.view.SetCorsAllowlist([]string{pattern})
	log.Printf("[turnstile] Applied CORS allowlist for %s on WebView ID %d", turnstileHost, w.id)
}

func (w *WebView) attachTurnstileResponseLogger() {
	if w.view == nil {
		return
	}

	w.view.ConnectResourceLoadStarted(func(resource *webkit.WebResource, request *webkit.URIRequest) {
		if resource == nil || request == nil {
			return
		}

		targetURL := request.URI()
		if targetURL == "" {
			targetURL = resource.URI()
		}
		if !isTurnstileURL(targetURL) {
			return
		}

		method := request.HTTPMethod()
		if method == "" {
			method = "GET"
		}

		referer := "<none>"
		if reqHeaders := request.HTTPHeaders(); reqHeaders != nil {
			if ref := strings.TrimSpace(reqHeaders.One("Referer")); ref != "" {
				referer = ref
			}
		}

		resource.Connect("notify::response", func() {
			response := resource.Response()
			if response == nil {
				log.Printf("[turnstile] resource=%s method=%s status=<unknown> referer=%s corp=<missing> coep=<missing> coop=<missing>", targetURL, method, referer)
				return
			}

			headers := response.HTTPHeaders()
			corp := headerValue(headers, "Cross-Origin-Resource-Policy")
			coep := headerValue(headers, "Cross-Origin-Embedder-Policy")
			coop := headerValue(headers, "Cross-Origin-Opener-Policy")

			log.Printf("[turnstile] resource=%s method=%s status=%d referer=%s corp=%s coep=%s coop=%s",
				targetURL, method, response.StatusCode(), referer, corp, coep, coop)
		})
	})
}

type headerLookup interface {
	One(string) string
}

func headerValue(headers headerLookup, name string) string {
	if headers == nil {
		return "<missing>"
	}
	value := strings.TrimSpace(headers.One(name))
	if value == "" {
		return "<missing>"
	}
	return value
}

func isTurnstileURL(raw string) bool {
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == turnstileHost
}

// LoadURL loads the given URL in the WebView
func (w *WebView) LoadURL(url string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	if url == "" {
		return ErrInvalidURL
	}

	w.view.LoadURI(url)
	return nil
}

// GetCurrentURL returns the current URL
func (w *WebView) GetCurrentURL() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ""
	}

	return w.view.URI()
}

// GetTitle returns the current page title
func (w *WebView) GetTitle() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ""
	}

	return w.view.Title()
}

// GoBack navigates back in history
func (w *WebView) GoBack() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	w.view.GoBack()
	return nil
}

// GoForward navigates forward in history
func (w *WebView) GoForward() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	w.view.GoForward()
	return nil
}

// Reload reloads the current page
func (w *WebView) Reload() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	w.view.Reload()
	return nil
}

// ReloadBypassCache reloads the current page, bypassing cache
func (w *WebView) ReloadBypassCache() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	w.view.ReloadBypassCache()
	return nil
}

// Show makes the WebView visible
func (w *WebView) Show() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	// Make the WebView widget visible
	w.view.SetVisible(true)

	// Show the window containing the WebView
	if w.window != nil {
		w.window.Show()
	}

	return nil
}

// Hide makes the WebView invisible
func (w *WebView) Hide() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	w.view.SetVisible(false)
	return nil
}

// Destroy destroys the WebView and releases resources
func (w *WebView) Destroy() error {
	// DO NOT acquire w.mu lock here to avoid deadlock!
	// After GTK main loop exits, any goroutine holding the lock and calling GTK methods
	// will block forever waiting for the main loop. This causes Destroy() to deadlock
	// trying to acquire the same lock.
	//
	// During shutdown, thread safety is not critical since we're tearing everything down.

	if w.destroyed {
		return nil
	}

	w.destroyed = true

	// Unregister from global registry
	viewMu.Lock()
	delete(viewRegistry, w.id)
	viewMu.Unlock()

	// DO NOT call StopLoading() or any other GTK methods here!
	// After the main loop exits, GTK calls will block forever.
	// The GTK widget will be cleaned up by Go GC.

	return nil
}

// IsDestroyed returns true if the WebView has been destroyed
func (w *WebView) IsDestroyed() bool {
	// DO NOT acquire lock - read of bool is atomic on all platforms Go supports
	// This prevents deadlock during shutdown when GTK signal handlers check IsDestroyed
	return w.destroyed
}

// ID returns the unique identifier for this WebView
func (w *WebView) ID() uint64 {
	return w.id
}

// IDString returns the WebView ID as a string
// This is a convenience method for compatibility with code expecting string IDs
func (w *WebView) IDString() string {
	return fmt.Sprintf("%d", w.id)
}

// ParseWebViewID converts a string WebView ID to uint64
// Returns 0 if the string cannot be parsed
func ParseWebViewID(idStr string) uint64 {
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// AsWidget returns the WebView as a gtk.Widgetter
func (w *WebView) AsWidget() gtk.Widgetter {
	if w == nil || w.view == nil {
		return nil
	}
	return w.view
}

// Event handler registration methods

// RegisterScriptMessageHandler registers a handler for script messages
func (w *WebView) RegisterScriptMessageHandler(handler func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onScriptMessage = handler
}

// RegisterTitleChangedHandler registers a handler for title changes
func (w *WebView) RegisterTitleChangedHandler(handler func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onTitleChanged = handler
}

// RegisterURIChangedHandler registers a handler for URI changes
func (w *WebView) RegisterURIChangedHandler(handler func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onURIChanged = handler
}

// RegisterFaviconChangedHandler registers a handler for favicon changes
func (w *WebView) RegisterFaviconChangedHandler(handler func([]byte)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onFaviconChanged = handler
}

// RegisterFaviconURIChangedHandler registers a handler for favicon URI changes
func (w *WebView) RegisterFaviconURIChangedHandler(handler func(pageURI, faviconURI string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onFaviconURIChanged = handler
}

// RegisterZoomChangedHandler registers a handler for zoom changes
func (w *WebView) RegisterZoomChangedHandler(handler func(float64)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onZoomChanged = handler
}

// RegisterLoadCommittedHandler registers a handler for load committed events
// This fires when the page actually starts loading new content (after URI change)
func (w *WebView) RegisterLoadCommittedHandler(handler func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onLoadCommitted = handler
}

// RegisterLoadStartedHandler registers a handler for load start events.
func (w *WebView) RegisterLoadStartedHandler(handler func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onLoadStarted = handler
}

// RegisterLoadFinishedHandler registers a handler for load finished events.
func (w *WebView) RegisterLoadFinishedHandler(handler func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onLoadFinished = handler
}

// RegisterLoadProgressHandler registers a handler for load progress updates (0.0 - 1.0).
func (w *WebView) RegisterLoadProgressHandler(handler func(float64)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onLoadProgress = handler
}

// RegisterCloseHandler registers a handler for close requests
func (w *WebView) RegisterCloseHandler(handler func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onClose = handler
}

// RegisterPopupCreateHandler registers a handler for WebKit's create signal
// This is called when JavaScript calls window.open() or a link with target="_blank" is clicked
// The handler should create and return a new WebView for the popup, or return nil to block it
// The returned WebView should NOT be shown yet - WebKit will emit ready-to-show when it's ready
func (w *WebView) RegisterPopupCreateHandler(handler func(*webkit.NavigationAction) *WebView) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onPopupCreate = handler
}

// RegisterReadyToShowHandler registers a handler for WebKit's ready-to-show signal
// This is called when a popup WebView is ready to be displayed
// At this point it's safe to insert the popup into the UI (workspace, window, etc.)
func (w *WebView) RegisterReadyToShowHandler(handler func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onReadyToShow = handler
}

// RegisterNavigationPolicyHandler registers a handler for navigation policy decisions
func (w *WebView) RegisterNavigationPolicyHandler(handler func(url string, isUserGesture bool) bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onNavigationPolicy = handler
}

// OnWindowTypeDetected registers a handler for window type detection
func (w *WebView) OnWindowTypeDetected(handler func(WindowType, *WindowFeatures)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onWindowTypeDetected = handler
}

// RegisterMiddleClickLinkHandler registers a handler for middle mouse button clicks on links
// The handler receives the link URL and returns true if handled
func (w *WebView) RegisterMiddleClickLinkHandler(handler func(url string) bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onMiddleClickLink = handler
}

// RunOnMainThread executes a function on the GTK main thread
func (w *WebView) RunOnMainThread(fn func()) {
	RunOnMainThread(fn)
}

// GetWebView returns the underlying webkit.WebView for advanced operations
func (w *WebView) GetWebView() *webkit.WebView {
	return w.view
}

// Widget returns the WebView widget (for compatibility with old code expecting uintptr)
// Deprecated: Use AsWidget() instead
func (w *WebView) Widget() gtk.Widgetter {
	return w.AsWidget()
}

// RootWidget returns the root container widget for this WebView
// This is the GtkBox container that holds the WebView and can be reparented
func (w *WebView) RootWidget() gtk.Widgetter {
	if w.container != nil {
		return w.container
	}
	// Fallback to WebView if container is not available (shouldn't happen)
	return w.AsWidget()
}

// SetZoom sets the zoom level of the WebView
func (w *WebView) SetZoom(zoom float64) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	w.view.SetZoomLevel(zoom)
	return nil
}

// GetZoom returns the current zoom level
func (w *WebView) GetZoom() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return 1.0
	}

	return w.view.ZoomLevel()
}

// UsesDomZoom indicates if this WebView uses DOM-based zoom
// In gotk4/WebKitGTK, zoom is always viewport-based
func (w *WebView) UsesDomZoom() bool {
	return false
}

// SeedDomZoom is a no-op in gotk4 as we use viewport zoom
func (w *WebView) SeedDomZoom(zoom float64) error {
	// Not needed in gotk4 - zoom is handled differently
	return nil
}

// InjectScript executes JavaScript in the WebView
func (w *WebView) InjectScript(script string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	// Execute JavaScript using our CGO wrapper
	EvaluateJavascript(w.view, script)
	return nil
}

// DispatchCustomEvent dispatches a custom event via JavaScript
// This works across JavaScript world boundaries because it uses document.dispatchEvent.
// GUI scripts running in the isolated world (see DumberIsolatedWorld in user_content.go)
// can listen for these events because they share the same Document object with the main world.
func (w *WebView) DispatchCustomEvent(eventName string, data interface{}) error {
	// Serialize data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	log.Printf("[webkit] Dispatching event '%s' to WebView ID %d with data: %s", eventName, w.id, string(jsonData))

	// Use document.dispatchEvent (not window) so events cross JavaScript world boundaries
	// The GUI scripts run in an isolated world but share the same Document object
	script := fmt.Sprintf(`
		(function() {
			try {
				var detail = %s;
				console.log('[dumber] Dispatching event: %s', detail);
				document.dispatchEvent(new CustomEvent('%s', { detail: detail }));
			} catch (e) {
				console.error('[dumber] Failed to dispatch %s', e);
			}
		})();
	`, string(jsonData), eventName, eventName, eventName)
	return w.InjectScript(script)
}

// ShowDevTools opens the WebKit inspector/developer tools
func (w *WebView) ShowDevTools() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	inspector := w.view.Inspector()
	if inspector != nil {
		inspector.Show()
	}
	return nil
}

// ShowPrintDialog shows the print dialog for the current page
func (w *WebView) ShowPrintDialog() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	// Create and run print operation with GTK print dialog
	printOp := webkit.NewPrintOperation(w.view)
	printOp.RunDialog(nil) // nil = no parent window, uses active window
	return nil
}

// RegisterKeyboardShortcut registers a keyboard shortcut handler
// This is a compatibility method - actual shortcut handling is done at the window level
func (w *WebView) RegisterKeyboardShortcut(key string, modifiers uint, handler func()) error {
	// TODO: Implement keyboard shortcut registration if needed
	// For now, shortcuts are handled at the window/application level
	return nil
}

// SetWindowFeatures sets window features for popup windows
func (w *WebView) SetWindowFeatures(features *WindowFeatures) {
	// This is typically used for popup windows
	// The features would be applied when creating the window
}

// IsActive returns whether this WebView is currently active/focused
func (w *WebView) IsActive() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return false
	}

	widget := w.view
	if widget != nil {
		return widget.IsFocus()
	}
	return false
}

// Window returns the parent Window of this WebView
func (w *WebView) Window() *Window {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return nil
	}

	return w.window
}

// GetFaviconDatabase returns the WebKit FaviconDatabase for this WebView
// Returns nil if the favicon database is not available
func (w *WebView) GetFaviconDatabase() *webkit.FaviconDatabase {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed || w.view == nil {
		return nil
	}

	session := w.view.NetworkSession()
	if session == nil {
		return nil
	}

	dataManager := session.WebsiteDataManager()
	if dataManager == nil {
		return nil
	}

	return dataManager.FaviconDatabase()
}

// UpdateContentFilters updates the content filtering rules
func (w *WebView) UpdateContentFilters(rules string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	// Note: Content filtering is handled via InitializeContentBlocking()
	// This method is kept for backward compatibility but is deprecated
	log.Printf("[webkit] UpdateContentFilters called but is deprecated - use InitializeContentBlocking instead")
	return nil
}

// InitializeContentBlocking initializes content blocking with filter lists
func (w *WebView) InitializeContentBlocking(filterJSON []byte) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	if len(filterJSON) == 0 {
		return fmt.Errorf("no filter rules provided")
	}

	// Apply filters using the content blocking API
	return ApplyFiltersToWebView(w, filterJSON)
}

// OnNavigate registers a navigation handler
func (w *WebView) OnNavigate(handler func(url string)) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// This wraps the URI changed handler
	w.onURIChanged = handler
}

// RegisterWorkspaceNavigationHandler registers a handler for workspace pane navigation
// The handler receives the direction ("up", "down", "left", "right") and returns true if handled
func (w *WebView) RegisterWorkspaceNavigationHandler(handler func(direction string) bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onWorkspaceNavigation = handler
}

// RegisterPaneModeHandler registers a handler for pane mode shortcuts
// The handler receives the action ("enter", "close", "split-right", etc.) and returns true if handled
func (w *WebView) RegisterPaneModeHandler(handler func(action string) bool, isActiveChecker func() bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onPaneModeShortcut = handler
	w.isPaneModeActive = isActiveChecker
}

// setupFaviconHandlers enables favicons for this WebView
// Note: The actual favicon-changed handler is registered ONCE at the FaviconService level
// to avoid duplicate handlers when multiple WebViews exist
func (w *WebView) setupFaviconHandlers() {
	// Get the NetworkSession from the WebView
	session := w.view.NetworkSession()
	if session == nil {
		log.Printf("[webkit] Warning: No NetworkSession available for favicon handling")
		return
	}

	// Get the WebsiteDataManager from the NetworkSession
	dataManager := session.WebsiteDataManager()
	if dataManager == nil {
		log.Printf("[webkit] Warning: No WebsiteDataManager available for favicon handling")
		return
	}

	// Enable favicons if not already enabled
	if !dataManager.FaviconsEnabled() {
		dataManager.SetFaviconsEnabled(true)
		log.Printf("[webkit] Enabled favicons for WebView ID %d", w.id)
	}

}

// GtkWebView returns the underlying gotk4 WebView for advanced operations
// This is used when creating related views (popups) that need to share the same session
func (w *WebView) GtkWebView() *webkit.WebView {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.view
}

// WrapBareWebView creates a minimal WebView wrapper around a bare gotk4 WebView
// This is used during popup creation to return a WebView to WebKit before initialization
// Full initialization (container, scripts, event handlers) should be done later
func WrapBareWebView(bareView *webkit.WebView) *WebView {
	if bareView == nil {
		return nil
	}

	// Generate unique ID
	id := atomic.AddUint64(&viewIDCounter, 1)
	log.Printf("[webkit] Created minimal wrapper for bare WebView (ID: %d)", id)

	// Create minimal wrapper - NO initialization yet
	wv := &WebView{
		view:      bareView,
		container: nil, // Will be created during full initialization
		window:    nil,
		id:        id,
		config:    nil, // Will be set during full initialization
	}

	// Register in global registry
	viewMu.Lock()
	viewRegistry[id] = wv
	viewMu.Unlock()

	return wv
}

// InitializeFromBare completes initialization of a bare WebView wrapper
// This should be called in ready-to-show after WebKit has configured the view
func (w *WebView) InitializeFromBare(cfg *Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.config != nil {
		return fmt.Errorf("WebView already initialized")
	}

	if cfg == nil {
		cfg = GetDefaultConfig()
	}

	w.config = cfg

	// Verify persistent session
	viewSession := w.view.NetworkSession()
	if viewSession == nil {
		return fmt.Errorf("WebView has no network session")
	}

	// Setup UserContentManager and inject GUI scripts
	if err := SetupUserContentManager(w.view, cfg.AppearanceConfigJSON, w.id); err != nil {
		return fmt.Errorf("failed to setup user content manager: %w", err)
	}

	// Create container (GtkBox) to hold the WebView
	container := gtk.NewBox(gtk.OrientationVertical, 0)
	container.SetHExpand(true)
	container.SetVExpand(true)

	// Configure WebView widget for expansion
	w.view.SetHExpand(true)
	w.view.SetVExpand(true)

	// Add WebView to container
	container.Append(w.view)

	w.container = container

	// Apply configuration
	if err := w.applyConfig(); err != nil {
		return err
	}

	// Setup event handlers
	w.setupEventHandlers()

	// Attach keyboard bridge
	w.AttachKeyboardBridge()

	// Attach mouse gesture controls
	w.AttachMouseGestures()

	// Attach touchpad gesture controls
	w.AttachTouchpadGestures()

	log.Printf("[webkit] Completed initialization for WebView ID %d", w.id)

	return nil
}

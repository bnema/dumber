package webkit

import (
	"fmt"
	"sync"
	"sync/atomic"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var (
	viewIDCounter uint64
	viewRegistry  = make(map[uint64]*WebView)
	viewMu        sync.RWMutex
)

// WebView wraps a WebKitGTK WebView
type WebView struct {
	view *webkit.WebView
	id   uint64

	// State
	config    *Config
	destroyed bool
	mu        sync.RWMutex

	// Event handlers
	onScriptMessage         func(string)
	onTitleChanged          func(string)
	onURIChanged            func(string)
	onFaviconChanged        func([]byte)
	onFaviconURIChanged     func(pageURI, faviconURI string)
	onZoomChanged           func(float64)
	onPopup                 func(string) *WebView
	onClose                 func()
	onNavigationPolicy      func(url string, isUserGesture bool) bool
	onWindowTypeDetected    func(WindowType, *WindowFeatures)
}

// NewWebView creates a new WebView with the given configuration
func NewWebView(cfg *Config) (*WebView, error) {
	if cfg == nil {
		cfg = GetDefaultConfig()
	}

	InitMainThread()

	// Create WebKitGTK WebView
	wkView := webkit.NewWebView()
	if wkView == nil {
		return nil, ErrWebViewNotInitialized
	}

	// Generate unique ID
	id := atomic.AddUint64(&viewIDCounter, 1)

	wv := &WebView{
		view:   wkView,
		id:     id,
		config: cfg,
	}

	// Apply configuration
	if err := wv.applyConfig(); err != nil {
		return nil, err
	}

	// Setup event handlers
	wv.setupEventHandlers()

	// Register in global registry
	viewMu.Lock()
	viewRegistry[id] = wv
	viewMu.Unlock()

	return wv, nil
}

// NewWebViewWithRelated creates a new WebView related to an existing one
// (shares process context)
func NewWebViewWithRelated(cfg *Config, related *WebView) (*WebView, error) {
	if related == nil {
		return NewWebView(cfg)
	}

	// TODO: Implement related view creation using webkit.NewWebViewWithRelatedView
	// For now, create a regular view
	return NewWebView(cfg)
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

	// Enable hardware acceleration if configured
	settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyAlways)

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

	// Load changed
	w.view.ConnectLoadChanged(func(event webkit.LoadEvent) {
		// Handle load events if needed
	})

	// Close
	w.view.ConnectClose(func() {
		if w.onClose != nil {
			w.onClose()
		}
	})
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

	w.view.SetVisible(true)
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
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.destroyed {
		return nil
	}

	w.destroyed = true

	// Unregister from global registry
	viewMu.Lock()
	delete(viewRegistry, w.id)
	viewMu.Unlock()

	// The GTK widget will be cleaned up by Go GC
	return nil
}

// IsDestroyed returns true if the WebView has been destroyed
func (w *WebView) IsDestroyed() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.destroyed
}

// ID returns the unique identifier for this WebView
func (w *WebView) ID() uint64 {
	return w.id
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

// RegisterPopupHandler registers a handler for popup requests
func (w *WebView) RegisterPopupHandler(handler func(string) *WebView) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onPopup = handler
}

// RegisterCloseHandler registers a handler for close requests
func (w *WebView) RegisterCloseHandler(handler func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onClose = handler
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
// In the new architecture, this is just the WebView itself
func (w *WebView) RootWidget() gtk.Widgetter {
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
func (w *WebView) DispatchCustomEvent(eventName string, data interface{}) error {
	// TODO: Implement proper event dispatching with data serialization
	script := fmt.Sprintf(`
		window.dispatchEvent(new CustomEvent('%s', { detail: %v }));
	`, eventName, data)
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

	// TODO: Implement print operation
	// printOp := webkit.NewPrintOperation(w.view)
	// printOp.RunDialog(nil)
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

	// Get the root window
	widget := getWidget(w.view)
	if widget != nil {
		root := widget.Root()
		if root != nil {
			// Try to cast the native widget to a gtk.Window
			if obj := root.Cast(); obj != nil {
				if gtkWin, ok := obj.(*gtk.Window); ok {
					return &Window{win: gtkWin}
				}
			}
		}
	}
	return nil
}

// UpdateContentFilters updates the content filtering rules
func (w *WebView) UpdateContentFilters(rules string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	// TODO: Implement content filtering using WebKit's UserContentManager
	return nil
}

// InitializeContentBlocking initializes content blocking with filter lists
func (w *WebView) InitializeContentBlocking(filterLists []string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.destroyed {
		return ErrWebViewDestroyed
	}

	// TODO: Implement content blocking initialization
	return nil
}

// OnNavigate registers a navigation handler
func (w *WebView) OnNavigate(handler func(url string)) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// This wraps the URI changed handler
	w.onURIChanged = handler
}

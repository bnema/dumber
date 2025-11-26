package browser

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/webext"
	"github.com/bnema/dumber/internal/webext/api"
	"github.com/bnema/dumber/pkg/webkit"
)

// ExtensionPopup represents an active extension popup
type ExtensionPopup struct {
	ExtensionID   string
	PopupNode     *paneNode
	WebView       *webkit.WebView
	URL           string
	BackgroundCtx *webext.BackgroundContext
	OpenedAt      time.Time
}

// ExtensionPopupManager manages extension popup lifecycle and API proxying
type ExtensionPopupManager struct {
	app          *BrowserApp
	extManager   *webext.Manager
	activePopups map[string]*ExtensionPopup // key: extensionID
	popupsByID   map[uint64]*ExtensionPopup // key: popup WebView ID
	mu           sync.RWMutex
}

// NewExtensionPopupManager creates a new popup manager
func NewExtensionPopupManager(app *BrowserApp, extMgr *webext.Manager) *ExtensionPopupManager {
	return &ExtensionPopupManager{
		app:          app,
		extManager:   extMgr,
		activePopups: make(map[string]*ExtensionPopup),
		popupsByID:   make(map[uint64]*ExtensionPopup),
	}
}

// OpenPopup opens an extension popup for the given extension
// Follows Chrome behavior: only one popup per extension at a time
func (pm *ExtensionPopupManager) OpenPopup(extID string, popupURL string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Capture the active pane's WebView ID BEFORE making any changes
	// This is critical for extensions like uBlock Origin that need to know
	// which pane (tab) the popup was opened from
	var activeWebViewID uint64
	if pm.app.workspace != nil {
		if activePane := pm.app.workspace.GetActivePane(); activePane != nil && activePane.webView != nil {
			activeWebViewID = activePane.webView.ID()
		}
	}

	// Append tabId query parameter if we have an active pane
	// Extensions expect this to know which "tab" they're operating on
	if activeWebViewID != 0 {
		parsedURL, err := url.Parse(popupURL)
		if err == nil {
			q := parsedURL.Query()
			q.Set("tabId", fmt.Sprintf("%d", activeWebViewID))
			parsedURL.RawQuery = q.Encode()
			popupURL = parsedURL.String()
			log.Printf("[popup-manager] Added tabId=%d to popup URL: %s", activeWebViewID, popupURL)
		}
	}

	// Close existing popup if any
	if existing := pm.activePopups[extID]; existing != nil {
		pm.closePopupLocked(extID)
	}

	// Get extension
	ext, ok := pm.extManager.GetExtension(extID)
	if !ok || ext == nil {
		return fmt.Errorf("extension not found: %s", extID)
	}

	// Get background context for API proxying
	bgCtx := pm.extManager.GetBackgroundContext(extID)
	if bgCtx == nil {
		log.Printf("[popup-manager] Warning: No background context for %s, APIs may not work", extID)
	}

	// Create popup WebView with extension configuration
	// Use the manifest's CSP or the default if not specified (matches Firefox/Epiphany behavior)
	csp := ext.Manifest.GetContentSecurityPolicy()
	log.Printf("[popup-manager] Using CSP for %s: %s", extID, csp)

	// Build CORS allowlist from extension permissions
	corsAllowlist := buildCORSAllowlist(ext)

	// Create extension popup WebView (no parent, since background is not a WebView anymore)
	bareView, err := webkit.NewExtensionWebView(&webkit.ExtensionViewConfig{
		Type:          webkit.ExtensionViewPopup,
		CSP:           csp,
		ParentView:    nil, // No parent - background runs in Goja context
		ExtensionID:   extID,
		CORSAllowlist: corsAllowlist,
	})

	if err != nil {
		return fmt.Errorf("failed to create popup WebView: %w", err)
	}

	// Wrap the bare WebView
	wrappedView := webkit.WrapBareWebView(bareView)
	if wrappedView == nil {
		return fmt.Errorf("failed to wrap popup WebView")
	}

	// Initialize using full browser defaults so JS/modules stay enabled.
	cfg, cfgErr := pm.app.buildWebkitConfig()
	if cfgErr != nil {
		return fmt.Errorf("failed to build webkit config: %w", cfgErr)
	}
	cfg.CreateWindow = false
	cfg.IsExtensionWebView = true // Critical: skip UserContentManager injection

	if err := wrappedView.InitializeFromBare(cfg); err != nil {
		return fmt.Errorf("failed to initialize popup: %w", err)
	}

	// Set CORS allowlist if needed
	if len(corsAllowlist) > 0 {
		webkit.SetCORSAllowlist(wrappedView.GetWebView(), corsAllowlist)
	}

	// Register extension message handler for popup WebView
	pm.app.registerExtensionMessageHandler(wrappedView)

	// Register popup-specific handlers for browser.* API bridge
	pm.setupPopupBridge(wrappedView, extID)

	// Create a BrowserPane from the WebView
	pane, err := pm.app.createPaneForView(wrappedView)
	if err != nil {
		return fmt.Errorf("failed to create pane for popup: %w", err)
	}

	// Get workspace and active node for splitting
	ws := pm.app.workspace
	if ws == nil {
		return fmt.Errorf("workspace not available")
	}

	activeNode := ws.GetActiveNode()
	if activeNode == nil {
		// Fallback to root if no active node
		activeNode = ws.root
	}
	if activeNode == nil {
		return fmt.Errorf("no target node for popup split")
	}

	// Default popup width (extension popups are typically narrow)
	popupWidth := 300

	// Split to the right with max-width constraint
	// MaxWidth is passed through to syncPanedDivider which sets the paned position
	// and prevents the popup pane from shrinking
	popupNode, err := ws.SplitPaneWithOptions(activeNode, SplitOptions{
		Direction:    DirectionRight,
		ExistingPane: pane,
		MaxWidth:     popupWidth,
	})
	if err != nil {
		return fmt.Errorf("failed to create popup split: %w", err)
	}

	// Set minimum size request on container as fallback hint
	if popupNode != nil && popupNode.container != nil {
		webkit.WidgetSetSizeRequest(popupNode.container, popupWidth, -1)
	}

	// Initialize popup node with focus/hover handlers that regular splits get via idle callback
	// Extension popups bypass the scheduleIdleGuarded callback in splitNode(), so we must add these manually
	if popupNode != nil {
		// Mark as popup to prevent browser exit on close
		popupNode.isPopup = true
		popupNode.windowType = webkit.WindowTypePopup

		// Attach GTK focus controller (enables focus enter/leave events)
		if ws.focusStateMachine != nil {
			ws.focusStateMachine.attachGTKController(popupNode)
		}

		// Attach hover handler (enables mouse-based focus)
		ws.ensureHover(popupNode)

		// Setup popup handling (enables nested popups from extension)
		ws.setupPopupHandling(wrappedView, popupNode)
	}

	// Create popup record
	popup := &ExtensionPopup{
		ExtensionID:   extID,
		PopupNode:     popupNode,
		WebView:       wrappedView,
		URL:           popupURL,
		BackgroundCtx: bgCtx,
		OpenedAt:      time.Now(),
	}

	// Store popup
	pm.activePopups[extID] = popup
	pm.popupsByID[wrappedView.ID()] = popup

	log.Printf("[popup-manager] Opened popup for %s as split pane (width=%d): %s", extID, popupWidth, popupURL)

	// Load the URL
	if err := wrappedView.LoadURL(popupURL); err != nil {
		log.Printf("[popup-manager] Warning: failed to load popup URL: %v", err)
	}

	// Set as active pane
	ws.SetActivePane(popupNode, SourceProgrammatic)

	return nil
}

// ClosePopup closes the popup for the given extension
func (pm *ExtensionPopupManager) ClosePopup(extID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.closePopupLocked(extID)
}

// closePopupLocked closes a popup (must be called with lock held)
func (pm *ExtensionPopupManager) closePopupLocked(extID string) error {
	popup := pm.activePopups[extID]
	if popup == nil {
		return fmt.Errorf("no active popup for extension: %s", extID)
	}

	// Remove from maps
	delete(pm.activePopups, extID)
	if popup.WebView != nil {
		delete(pm.popupsByID, popup.WebView.ID())
	}

	// Close the pane in workspace
	if popup.PopupNode != nil && pm.app.workspace != nil {
		if err := pm.app.workspace.ClosePane(popup.PopupNode); err != nil {
			log.Printf("[popup-manager] Warning: failed to close popup pane: %v", err)
		}
	}

	log.Printf("[popup-manager] Closed popup for %s", extID)

	return nil
}

// GetActivePopup returns the active popup for an extension, if any
func (pm *ExtensionPopupManager) GetActivePopup(extID string) *ExtensionPopup {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.activePopups[extID]
}

// GetPopupByViewID returns a popup by its WebView ID
func (pm *ExtensionPopupManager) GetPopupByViewID(viewID uint64) *ExtensionPopup {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.popupsByID[viewID]
}

// IsPopupView checks if a WebView ID belongs to an extension popup
func (pm *ExtensionPopupManager) IsPopupView(viewID uint64) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.popupsByID[viewID] != nil
}

// GetPopupInfoByViewID implements webext.PopupInfoProvider
// Returns popup info for runtime.connect sender context
func (pm *ExtensionPopupManager) GetPopupInfoByViewID(viewID uint64) *webext.PopupInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	popup := pm.popupsByID[viewID]
	if popup == nil {
		return nil
	}

	return &webext.PopupInfo{
		ExtensionID: popup.ExtensionID,
		URL:         popup.URL,
	}
}

// CloseAll closes all active popups (used during shutdown)
func (pm *ExtensionPopupManager) CloseAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for extID := range pm.activePopups {
		_ = pm.closePopupLocked(extID)
	}
}

// ProxyStorageToBackground proxies a storage API call from a popup to the background context
// This allows popups to use browser.storage.local.get() which will be handled by the
// background context's direct Goja bindings instead of going through the dispatcher
func (pm *ExtensionPopupManager) ProxyStorageToBackground(viewID uint64, operation string, args interface{}) (interface{}, error) {
	pm.mu.RLock()
	popup := pm.popupsByID[viewID]
	pm.mu.RUnlock()

	if popup == nil {
		return nil, fmt.Errorf("popup not found for view %d", viewID)
	}

	if popup.BackgroundCtx == nil {
		return nil, fmt.Errorf("no background context for extension %s", popup.ExtensionID)
	}

	// Get the extension's storage API
	ext, ok := pm.extManager.GetExtension(popup.ExtensionID)
	if !ok || ext == nil || ext.Storage == nil {
		return nil, fmt.Errorf("storage not available for extension %s", popup.ExtensionID)
	}

	storageArea := ext.Storage.Local()

	// Route to appropriate storage operation
	switch operation {
	case "get":
		return storageArea.Get(args)
	case "set":
		items, ok := args.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid arguments for storage.set")
		}
		return nil, storageArea.Set(items)
	case "remove":
		return nil, storageArea.Remove(args)
	case "clear":
		return nil, storageArea.Clear()
	default:
		return nil, fmt.Errorf("unsupported storage operation: %s", operation)
	}
}

// NotifyStorageChange dispatches storage change events to all active popups
// This is called when storage changes occur in the background
func (pm *ExtensionPopupManager) NotifyStorageChange(extID string, changes map[string]api.StorageChange, areaName string) {
	pm.mu.RLock()
	popup := pm.activePopups[extID]
	pm.mu.RUnlock()

	if popup == nil || popup.WebView == nil {
		return
	}

	// Dispatch storage change event to popup
	changesJSON, err := json.Marshal(changes)
	if err != nil {
		log.Printf("[popup-manager] Failed to marshal storage changes: %v", err)
		return
	}

	js := fmt.Sprintf(`
		if (window.browser && window.browser.storage && window.browser.storage.onChanged && window.browser.storage.onChanged._emit) {
			window.browser.storage.onChanged._emit(%s, %q);
		}
	`, string(changesJSON), areaName)

	popup.WebView.InjectScript(js)
}

// setupPopupBridge sets up the JavaScript bridge for popup browser.* APIs
func (pm *ExtensionPopupManager) setupPopupBridge(view *webkit.WebView, extID string) {
	viewID := view.ID()

	// Register handler for popup API messages from JavaScript
	// This receives messages from webkit.messageHandlers.webext.postMessage()
	view.RegisterWebExtMessageHandler(func(payload string) {
		pm.handlePopupAPIMessage(viewID, extID, payload)
	})

	// Inject bridge JS when page loads
	view.RegisterLoadCommittedHandler(func(uri string) {
		pm.injectPopupBridge(view, extID)
	})
}

// injectPopupBridge injects the browser.* API bridge into the popup
func (pm *ExtensionPopupManager) injectPopupBridge(view *webkit.WebView, extID string) {
	// Set the extension ID in the runtime object before injecting
	bridgeJS := webext.PopupBridgeJS

	// Inject the bridge
	view.InjectScript(bridgeJS)

	// Set the extension ID on browser.runtime
	setIDJS := fmt.Sprintf(`
		if (typeof browser !== 'undefined' && browser.runtime) {
			browser.runtime.id = %q;
		}
	`, extID)
	view.InjectScript(setIDJS)

	log.Printf("[popup-manager] Injected popup bridge for extension %s", extID)
}

// handlePopupAPIMessage handles API calls from the popup's JavaScript bridge
func (pm *ExtensionPopupManager) handlePopupAPIMessage(viewID uint64, extID string, payload string) {
	if pm.extManager == nil {
		log.Printf("[popup-manager] Extension manager not available")
		return
	}

	dispatcher := pm.extManager.GetDispatcher()
	if dispatcher == nil {
		log.Printf("[popup-manager] Dispatcher not available")
		return
	}

	// Route to dispatcher
	response := dispatcher.HandlePopupAPIRequest(extID, viewID, payload)

	// Send response back to popup via JavaScript
	pm.sendPopupResponse(viewID, response)
}

// sendPopupResponse sends an API response back to the popup
func (pm *ExtensionPopupManager) sendPopupResponse(viewID uint64, response *webext.PopupAPIResponse) {
	pm.mu.RLock()
	popup := pm.popupsByID[viewID]
	pm.mu.RUnlock()

	if popup == nil || popup.WebView == nil {
		return
	}

	// Marshal response to JSON
	responseJSON, err := json.Marshal(response)
	if err != nil {
		log.Printf("[popup-manager] Failed to marshal popup response: %v", err)
		return
	}

	// Call the response handler in JavaScript
	js := fmt.Sprintf(`window.__dumberPopupResponse(%s);`, string(responseJSON))
	popup.WebView.InjectScript(js)
}

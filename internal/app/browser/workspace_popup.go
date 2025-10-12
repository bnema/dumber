// workspace_popup.go - Popup window handling and related pane management
package browser

import (
	"log"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/pkg/webkit"
	webkitgtk "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

const (
	// Default popup window dimensions - sized for OAuth flows
	defaultPopupMinWidth  = 500 // Minimum width for OAuth popups
	defaultPopupMinHeight = 600 // Minimum height for OAuth popups
)

// HandlePopup is DEPRECATED and removed.
// Popup lifecycle now uses WebKit's native create/ready-to-show/close signals.
// See setupPopupHandling(), handlePopupReadyToShow(), and handlePopupClose() instead.

// registerOAuthAutoClose sets up OAuth auto-close functionality for popups
// Note: OAuth detection is now handled by the main-world.js injection script
func (wm *WorkspaceManager) registerOAuthAutoClose(view *webkit.WebView, url string) {
	log.Printf("[workspace] OAuth auto-close enabled for popup with URL: %s", url)
	log.Printf("[workspace] OAuth detection will be handled by main-world.js injection script")
}

// insertIndependentPane inserts a new independent pane next to the source
func (wm *WorkspaceManager) insertIndependentPane(sourceNode *paneNode, webView *webkit.WebView, url string) error {
	newPane, err := wm.createPane(webView)
	if err != nil {
		return err
	}
	direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
	if direction == "" {
		direction = "right"
	}
	if err := wm.insertPopupPane(sourceNode, newPane, direction); err != nil { // reuse insertion primitive
		return err
	}
	node := wm.viewToNode[webView]
	if node != nil {
		node.windowType = webkit.WindowTypeTab
		node.isRelated = false
	}
	if url != "" {
		_ = webView.LoadURL(url)
	}
	return nil
}

// configureRelatedPopup creates a related view and inserts it
func (wm *WorkspaceManager) configureRelatedPopup(sourceNode *paneNode, webView *webkit.WebView, url string, feat *webkit.WindowFeatures) {
	// Use the WebView that was already created and returned to WebKit
	related := webView
	newPane, err := wm.createPane(related)
	if err != nil {
		log.Printf("[workspace] failed to create related popup pane: %v", err)
		return
	}
	direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
	if direction == "" {
		direction = "right"
	}
	if err := wm.insertPopupPane(sourceNode, newPane, direction); err != nil {
		log.Printf("[workspace] failed to insert related popup pane: %v", err)
		return
	}
	node := wm.viewToNode[related]
	popupWebViewID := related.IDString()

	if node != nil {
		node.windowType = webkit.WindowTypePopup
		node.windowFeatures = feat
		node.isRelated = true
		node.parentPane = sourceNode
		node.isPopup = true
		// Heuristic + config for auto-close intent
		node.autoClose = wm.shouldAutoClose(url)

		// Apply minimum size constraints to prevent compression
		var width, height int
		if feat != nil {
			width = feat.Width
			height = feat.Height
		}
		wm.applyPopupSizeConstraints(related, width, height)
	}

	// Track this popup in the parent's activePopupChildren
	// This is critical to prevent OAuth flows from hijacking the parent pane via window.opener
	if sourceNode != nil && popupWebViewID != "" {
		if sourceNode.activePopupChildren == nil {
			sourceNode.activePopupChildren = make([]string, 0)
		}
		sourceNode.activePopupChildren = append(sourceNode.activePopupChildren, popupWebViewID)
		log.Printf("[workspace] Added popup %s to parent's activePopupChildren (count: %d)", popupWebViewID, len(sourceNode.activePopupChildren))

		// Register navigation policy handler for parent to block script-initiated navigations
		// when it has active popup children (prevents OAuth from hijacking parent pane)
		if sourceNode.pane != nil && sourceNode.pane.WebView() != nil {
			sourceNode.pane.WebView().RegisterNavigationPolicyHandler(func(url string, isUserGesture bool) bool {
				// Always allow user-initiated navigations
				if isUserGesture {
					return true
				}

				// Block script-initiated navigations if this pane has active popup children
				if len(sourceNode.activePopupChildren) > 0 {
					log.Printf("[workspace] BLOCKED script-initiated navigation in parent pane (has %d active popups): %s", len(sourceNode.activePopupChildren), url)
					return false
				}

				// Allow if no active popups
				return true
			})
			log.Printf("[workspace] Registered navigation policy handler for parent pane")
		}
	}

	// Pipe into existing auto-close flow only for popups (confirmed by detection)
	related.RegisterCloseHandler(func() {
		log.Printf("[workspace] Popup requesting close via window.close()")

		// Remove this popup from parent's activePopupChildren
		if sourceNode != nil && popupWebViewID != "" {
			for i, childID := range sourceNode.activePopupChildren {
				if childID == popupWebViewID {
					sourceNode.activePopupChildren = append(sourceNode.activePopupChildren[:i], sourceNode.activePopupChildren[i+1:]...)
					log.Printf("[workspace] Removed popup %s from parent's activePopupChildren (remaining: %d)", popupWebViewID, len(sourceNode.activePopupChildren))
					break
				}
			}
		}

		if n := wm.viewToNode[related]; n != nil && n.isPopup {
			time.AfterFunc(200*time.Millisecond, func() {
				wm.scheduleIdleGuarded(func() bool {
					if n == nil || !n.widgetValid {
						return false
					}
					if err := wm.ClosePane(n); err != nil {
						log.Printf("[workspace] Failed to close popup pane: %v", err)
					}
					return false
				}, n)
			})
		}
	})

	// URL-based fallback: if providers don't call window.close(), auto-close on OAuth callback URLs
	if node != nil && node.isPopup && node.autoClose {
		wm.registerOAuthAutoClose(related, url)
	}
	if url != "" {
		_ = related.LoadURL(url)
	}
}

// applyPopupSizeConstraints applies minimum size constraints to prevent OAuth popup compression
func (wm *WorkspaceManager) applyPopupSizeConstraints(view *webkit.WebView, width, height int) {
	minWidth := defaultPopupMinWidth
	minHeight := defaultPopupMinHeight

	// Use provided dimensions if valid
	if width > 0 {
		minWidth = width
	}
	if height > 0 {
		minHeight = height
	}

	// Apply to container
	if container := view.RootWidget(); container != nil {
		webkit.WidgetSetSizeRequest(container, minWidth, minHeight)
		webkit.WidgetQueueResize(container)
		log.Printf("[workspace] Applied minimum size %dx%d to popup container=%p", minWidth, minHeight, container)
	}

	// Apply to WebView widget
	if webViewWidget := view.Widget(); webViewWidget != nil {
		webkit.WidgetSetSizeRequest(webViewWidget, minWidth, minHeight)
		webkit.WidgetQueueResize(webViewWidget)
		log.Printf("[workspace] Applied minimum size %dx%d to popup webview=%p", minWidth, minHeight, webViewWidget)
	}
}

// shouldAutoClose checks simple OAuth-like URL patterns and config flag
func (wm *WorkspaceManager) shouldAutoClose(url string) bool {
	log.Printf("[workspace] shouldAutoClose called for URL: %s", url)

	if wm == nil || wm.app == nil || wm.app.config == nil {
		log.Printf("[workspace] shouldAutoClose: missing config, returning true")
		return true
	}
	if !wm.app.config.Workspace.Popups.OAuthAutoClose {
		log.Printf("[workspace] shouldAutoClose: OAuthAutoClose disabled in config, returning false")
		return false
	}

	u := strings.ToLower(url)
	log.Printf("[workspace] shouldAutoClose: checking lowercase URL: %s", u)

	// RFC 6749 compliant OAuth 2.0 URL patterns
	oauthPatterns := []string{
		// Standard OAuth endpoints
		"oauth", "authorize", "authorization",
		// Standard callback/redirect patterns
		"callback", "redirect", "auth/callback",
		// OpenID Connect patterns
		"oidc", "openid",
		// Common OAuth parameter indicators
		"response_type=", "client_id=", "redirect_uri=", "scope=", "state=",
		// Standard OAuth response parameters
		"code=", "access_token=", "id_token=", "token_type=",
		// Error response parameters
		"error=", "error_description=",
	}

	// Check for OAuth patterns in URL
	for _, pattern := range oauthPatterns {
		if strings.Contains(u, pattern) {
			log.Printf("[workspace] shouldAutoClose: MATCHED pattern '%s' in URL, returning true", pattern)
			return true
		}
	}

	log.Printf("[workspace] shouldAutoClose: no OAuth patterns matched, returning false")
	return false
}

// RunOnUI schedules a function; here simply executes inline as GTK main loop is single-threaded
func (wm *WorkspaceManager) RunOnUI(fn func()) {
	if fn != nil {
		fn()
	}
}

// handleDetectedWindowType handles window type detection from smart detection path
func (wm *WorkspaceManager) handleDetectedWindowType(sourceNode *paneNode, webView *webkit.WebView, url string, windowType webkit.WindowType, features *webkit.WindowFeatures) {
	if wm.viewToNode[webView] != nil {
		return // Already placed
	}

	log.Printf("[workspace] Smart detection result: type=%d url=%s", windowType, url)

	switch windowType {
	case webkit.WindowTypeTab:
		// For tabs, create a NEW independent WebView (can't use the related one)
		webkitCfg, err := wm.app.buildWebkitConfig()
		if err != nil {
			log.Printf("[workspace] failed to build webkit config for tab: %v", err)
			return
		}
		// Tabs are embedded in workspace paned containers - no separate window needed
		webkitCfg.CreateWindow = false

		// Create independent WebView matching our standard tab insertion path
		independentView, err := webkit.NewWebView(webkitCfg)
		if err != nil {
			log.Printf("[workspace] failed to create independent tab WebView: %v", err)
			return
		}

		// The related webView was just for detection - we don't use it for tabs
		// Insert the new independent view as a tab
		if err := wm.insertIndependentPane(sourceNode, independentView, url); err != nil {
			log.Printf("[workspace] Failed to insert independent pane: %v", err)
		}

	case webkit.WindowTypePopup:
		// For popups, use the related WebView we already created
		wm.configureRelatedPopup(sourceNode, webView, url, features)
	default:
		// Fallback to popup behavior
		wm.configureRelatedPopup(sourceNode, webView, url, features)
	}
}

// setupPopupHandling connects WebView's create signal for popup lifecycle management
// This implements the WebKit-native popup lifecycle as described in POPUP_REFACTORING_PLAN.md
func (wm *WorkspaceManager) setupPopupHandling(webView *webkit.WebView, parentNode *paneNode) {
	if webView == nil || parentNode == nil {
		return
	}

	webView.RegisterPopupCreateHandler(func(navAction *webkitgtk.NavigationAction) *webkit.WebView {
		// Get URL from navigation action
		req := navAction.Request()
		if req == nil {
			log.Printf("[workspace] create signal: no request in navigation action")
			return nil
		}
		url := req.URI()

		log.Printf("[workspace] create signal received for URL: %s", url)

		// Check if popups are enabled in config
		if wm.app.config != nil && !wm.app.config.Workspace.Popups.OpenInNewPane {
			log.Printf("[workspace] Popup creation disabled in config - blocking popup")
			return nil
		}

		// CRITICAL: Create a BARE gotk4 WebView as RELATED to parent (no wrapping, no initialization)
		// WebKit requires the related-view property to be set during construction for proper lifecycle
		// This ensures session/cookie sharing and proper WindowFeatures initialization
		// All initialization (wrapping, container, scripts) happens in ready-to-show
		log.Printf("[workspace] Step 1: Creating bare gotk4 WebView with related-view property")
		barePopupView := webkit.NewBareRelatedWebView(webView.GtkWebView())
		if barePopupView == nil {
			log.Printf("[workspace] Failed to create bare related popup WebView - blocking popup")
			return nil
		}
		log.Printf("[workspace] Step 1 complete: bare related gotk4 WebView created=%p (parent=%p)", barePopupView, webView.GtkWebView())

		// Wrap the bare view immediately to get a proper ID
		// BUT don't initialize it yet - that happens in ready-to-show
		log.Printf("[workspace] Step 2: Wrapping bare view (no initialization)")
		wrappedView := webkit.WrapBareWebView(barePopupView)
		if wrappedView == nil {
			log.Printf("[workspace] Failed to wrap bare popup WebView - blocking popup")
			return nil
		}

		// Use the wrapper's ID for tracking
		popupID := wrappedView.ID()
		log.Printf("[workspace] Step 2 complete: wrapped view created with ID=%d", popupID)

		// Store wrapped view (not initialized) for initialization in ready-to-show
		log.Printf("[workspace] Step 3: Storing in pendingPopups map")
		wm.pendingPopups[popupID] = &pendingPopup{
			wrappedView: wrappedView,
			parentView:  webView,
			parentNode:  parentNode,
			url:         url,
		}
		log.Printf("[workspace] Step 3 complete: stored in pendingPopups")

		// CRITICAL: Do NOT connect signals before returning to WebKit
		// Connecting signals might trigger WebKit to access WindowFeatures before it's set
		// Instead, we'll connect them after WebKit has processed the return
		log.Printf("[workspace] Step 4: Scheduling signal connections for next main loop iteration (AFTER WebKit configures the view)")

		// Use GLib idle_add to connect signals AFTER this callback returns
		// This ensures WebKit has finished configuring WindowFeatures before we touch the view
		glib.IdleAdd(func() bool {
			log.Printf("[workspace] Idle callback: Now connecting signals for popup ID=%d", popupID)

			// Connect ready-to-show signal NOW (after WebKit has configured the view)
			barePopupView.ConnectReadyToShow(func() {
				log.Printf("[workspace] ready-to-show signal received for popup ID=%d", popupID)
				wm.handlePopupReadyToShow(popupID)
			})
			log.Printf("[workspace] Idle callback: ready-to-show connected")

			// Connect close handler
			barePopupView.ConnectClose(func() {
				log.Printf("[workspace] close signal received for popup ID=%d", popupID)
				wm.handlePopupClose(popupID)
			})
			log.Printf("[workspace] Idle callback: close signal connected")

			return false // Don't repeat
		})

		log.Printf("[workspace] Step 4 complete: signal connections scheduled")

		log.Printf("[workspace] Step 5: Returning wrapped view to WebKit (will extract bare view in ConnectCreate)")

		// Return the wrapped view - WebKit will extract the underlying gotk4 view and configure it
		return wrappedView
	})
}

// handlePopupReadyToShow inserts popup pane when WebKit says it's ready
func (wm *WorkspaceManager) handlePopupReadyToShow(popupID uint64) {
	info, ok := wm.pendingPopups[popupID]
	if !ok {
		log.Printf("[workspace] ready-to-show for unknown popup ID: %d", popupID)
		return
	}
	delete(wm.pendingPopups, popupID)

	log.Printf("[workspace] ready-to-show received for popup ID=%d, starting full initialization", popupID)

	// NOW it's safe to initialize (WebKit has configured WindowProperties)
	// We already have the wrapped view from setupPopupHandling
	wrappedView := info.wrappedView

	// Build webkit config for initialization
	webkitCfg, err := wm.app.buildWebkitConfig()
	if err != nil {
		log.Printf("[workspace] Failed to build webkit config for popup initialization: %v", err)
		return
	}
	webkitCfg.CreateWindow = false // Embedded in workspace

	// Initialize the wrapped view with full configuration
	if err := wrappedView.InitializeFromBare(webkitCfg); err != nil {
		log.Printf("[workspace] Failed to initialize popup WebView: %v", err)
		return
	}

	log.Printf("[workspace] Successfully initialized popup WebView ID %d", wrappedView.ID())

	// Create pane for the fully initialized WebView
	popupPane, err := wm.createPane(wrappedView)
	if err != nil {
		log.Printf("[workspace] Failed to create popup pane: %v", err)
		return
	}

	log.Printf("[workspace] Created popup pane, now inserting into workspace")

	// NOW it's safe to insert into workspace based on configured behavior
	behavior := wm.app.config.Workspace.Popups.Behavior
	log.Printf("[workspace] Popup behavior: %s", behavior)

	switch behavior {
	case config.PopupBehaviorSplit:
		// Split pane behavior (default)
		direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
		if direction == "" {
			direction = "right"
		}
		if err := wm.insertPopupPane(info.parentNode, popupPane, direction); err != nil {
			log.Printf("[workspace] Failed to insert split popup pane: %v", err)
			return
		}

	case config.PopupBehaviorStacked:
		// Stacked pane behavior
		_, err := wm.stackedPaneManager.StackPane(info.parentNode)
		if err != nil {
			log.Printf("[workspace] Failed to stack existing pane: %v", err)
			return
		}
		// Insert the popup pane in the same container
		direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
		if direction == "" {
			direction = "right"
		}
		if err := wm.insertPopupPane(info.parentNode, popupPane, direction); err != nil {
			log.Printf("[workspace] Failed to insert stacked popup pane: %v", err)
			return
		}

	case config.PopupBehaviorTabbed:
		// Tabbed behavior - TODO: implement proper tabbed interface
		log.Printf("[workspace] WARNING: Tabbed popup behavior not yet implemented, falling back to split")
		direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
		if direction == "" {
			direction = "right"
		}
		if err := wm.insertPopupPane(info.parentNode, popupPane, direction); err != nil {
			log.Printf("[workspace] Failed to insert tabbed popup pane: %v", err)
			return
		}

	case config.PopupBehaviorWindowed:
		// Windowed behavior - opens in new workspace/window
		// TODO: implement new window creation with GTK
		log.Printf("[workspace] WARNING: Windowed popup behavior not yet implemented, falling back to split")
		direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
		if direction == "" {
			direction = "right"
		}
		if err := wm.insertPopupPane(info.parentNode, popupPane, direction); err != nil {
			log.Printf("[workspace] Failed to insert windowed popup pane: %v", err)
			return
		}

	default:
		// Fallback to split if unknown behavior
		log.Printf("[workspace] Unknown popup behavior '%s', falling back to split", behavior)
		direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
		if direction == "" {
			direction = "right"
		}
		if err := wm.insertPopupPane(info.parentNode, popupPane, direction); err != nil {
			log.Printf("[workspace] Failed to insert popup pane: %v", err)
			return
		}
	}

	// Configure the popup node
	node := wm.viewToNode[wrappedView]
	if node != nil {
		node.windowType = webkit.WindowTypePopup
		node.isRelated = true
		node.parentPane = info.parentNode
		node.isPopup = true
		node.autoClose = wm.shouldAutoClose(info.url)
		node.popupID = popupID // Store popupID for close signal lookup

		// Track this popup in the parent's activePopupChildren
		if info.parentNode != nil {
			popupWebViewID := wrappedView.IDString()
			if info.parentNode.activePopupChildren == nil {
				info.parentNode.activePopupChildren = make([]string, 0)
			}
			info.parentNode.activePopupChildren = append(info.parentNode.activePopupChildren, popupWebViewID)
			log.Printf("[workspace] Added popup %s to parent's activePopupChildren (count: %d)",
				popupWebViewID, len(info.parentNode.activePopupChildren))

			// Register navigation policy handler for parent
			if info.parentNode.pane != nil && info.parentNode.pane.WebView() != nil {
				info.parentNode.pane.WebView().RegisterNavigationPolicyHandler(func(url string, isUserGesture bool) bool {
					if isUserGesture {
						return true
					}
					if len(info.parentNode.activePopupChildren) > 0 {
						log.Printf("[workspace] BLOCKED script-initiated navigation in parent pane (has %d active popups): %s",
							len(info.parentNode.activePopupChildren), url)
						return false
					}
					return true
				})
			}
		}

		// URL-based auto-close for OAuth popups
		if node.autoClose {
			wm.registerOAuthAutoClose(wrappedView, info.url)
		}
	}

	wm.ensureGUIInPane(popupPane)

	// WebKit handles loading the URL automatically, just ensure visibility
	if err := wrappedView.Show(); err != nil {
		log.Printf("[workspace] failed to show popup WebView: %v", err)
	}

	log.Printf("[workspace] Popup pane inserted and visible for URL: %s", info.url)
}

// handlePopupClose responds to WebKit's close signal
func (wm *WorkspaceManager) handlePopupClose(popupID uint64) {
	log.Printf("[workspace] close signal received for popup native=%d", popupID)

	// Check if it's in pending (not yet shown)
	if info, ok := wm.pendingPopups[popupID]; ok {
		delete(wm.pendingPopups, popupID)
		log.Printf("[workspace] Popup closed before ready-to-show, cleaning up")
		// Just remove from pending, WebKit will handle cleanup
		_ = info // avoid unused variable
		return
	}

	// Find pane by popupID (stored in node during ready-to-show)
	var targetNode *paneNode
	for _, node := range wm.viewToNode {
		if node != nil && node.popupID == popupID {
			targetNode = node
			break
		}
	}

	if targetNode == nil {
		log.Printf("[workspace] close signal for unknown popup ID=%d, already closed?", popupID)
		return
	}

	// Remove from parent's activePopupChildren
	// Find the wrapped WebView for this node to get its IDString
	var wrappedViewID string
	for webView, node := range wm.viewToNode {
		if node == targetNode {
			wrappedViewID = webView.IDString()
			break
		}
	}

	if targetNode.parentPane != nil && wrappedViewID != "" {
		for i, childID := range targetNode.parentPane.activePopupChildren {
			if childID == wrappedViewID {
				targetNode.parentPane.activePopupChildren = append(
					targetNode.parentPane.activePopupChildren[:i],
					targetNode.parentPane.activePopupChildren[i+1:]...)
				log.Printf("[workspace] Removed popup from parent's activePopupChildren (remaining: %d)",
					len(targetNode.parentPane.activePopupChildren))
				break
			}
		}
	}

	// Clean close via existing ClosePane (WebKit already stopped)
	log.Printf("[workspace] Closing popup pane via ClosePane")
	if err := wm.ClosePane(targetNode); err != nil {
		log.Printf("[workspace] Error closing popup pane: %v", err)
	}
}

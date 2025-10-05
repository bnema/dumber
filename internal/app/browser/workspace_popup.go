// workspace_popup.go - Popup window handling and related pane management
package browser

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/pkg/webkit"
)

const (
	// Default popup window dimensions - sized for OAuth flows
	defaultPopupMinWidth  = 500 // Minimum width for OAuth popups
	defaultPopupMinHeight = 600 // Minimum height for OAuth popups
)

// HandlePopup handles popup window creation requests from WebViews
func (wm *WorkspaceManager) HandlePopup(source *webkit.WebView, url string) *webkit.WebView {
	log.Printf("[workspace] HandlePopup called for URL: %s", url)

	// Check for frame type markers added by WebKit layer
	isBlankTarget := strings.HasSuffix(url, "#__dumber_frame_blank")
	isPopupTarget := strings.HasSuffix(url, "#__dumber_frame_popup")

	// Clean the URL by removing our markers
	if isBlankTarget || isPopupTarget {
		if isBlankTarget {
			url = strings.TrimSuffix(url, "#__dumber_frame_blank")
			log.Printf("[workspace] Detected _blank target - will create regular pane for: %s", url)
		} else {
			url = strings.TrimSuffix(url, "#__dumber_frame_popup")
			log.Printf("[workspace] Detected popup target - will create popup pane for: %s", url)
		}
	}

	if wm == nil || source == nil {
		log.Printf("[workspace] HandlePopup: nil workspace manager or source - allowing native popup")
		return nil
	}

	node := wm.viewToNode[source]
	if node == nil {
		log.Printf("[workspace] popup from unknown webview - allowing native popup")
		return nil
	}

	// Note: HandlePopup is now obsolete - window.open is handled directly via JavaScript bypass

	cfg := wm.app.config
	if cfg == nil {
		log.Printf("[workspace] HandlePopup: nil config - allowing native popup")
		return nil
	}

	popCfg := cfg.Workspace.Popups
	log.Printf("[workspace] Popup config - OpenInNewPane: %v, Placement: %s", popCfg.OpenInNewPane, popCfg.Placement)

	if !popCfg.OpenInNewPane {
		log.Printf("[workspace] Popup creation disabled in config - allowing native popup")
		return nil
	}

	// Smart detection path: create temporary view and decide placement once type is known
	if popCfg.EnableSmartDetection {
		webkitCfg, err := wm.app.buildWebkitConfig()
		if err != nil {
			log.Printf("[workspace] failed to build webkit config: %v - allowing native popup", err)
			return nil
		}
		webkitCfg.CreateWindow = false
		// Create as related to avoid WindowFeatures crash; we'll decide final placement later
		newView, err := webkit.NewWebViewWithRelated(webkitCfg, source)
		if err != nil {
			log.Printf("[workspace] failed to create temp WebView: %v - allowing native popup", err)
			return nil
		}

		// Register detection callback
		newView.OnWindowTypeDetected(func(t webkit.WindowType, feat *webkit.WindowFeatures) {
			wm.RunOnUI(func() {
				wm.handleDetectedWindowType(node, newView, url, t, feat)
			})
		})

		// Fallback: if detection never fires, treat as popup as before
		go func() {
			time.Sleep(1500 * time.Millisecond)
			if newView != nil {
				wm.RunOnUI(func() {
					if wm.viewToNode[newView] == nil { // not yet placed
						wm.handleDetectedWindowType(node, newView, url, webkit.WindowTypePopup, nil)
					}
				})
			}
		}()

		return newView
	}

	// Legacy path preserved
	webkitCfg, err := wm.app.buildWebkitConfig()
	if err != nil {
		log.Printf("[workspace] failed to build webkit config: %v - allowing native popup", err)
		return nil
	}
	webkitCfg.CreateWindow = false
	newView, err := webkit.NewWebViewWithRelated(webkitCfg, source)
	if err != nil {
		log.Printf("[workspace] failed to create placeholder WebView: %v - allowing native popup", err)
		return nil
	}

	// Workspace navigation shortcuts are now handled globally by WindowShortcutHandler

	// Create a pane for the new WebView
	newPane, err := wm.createPane(newView)
	if err != nil {
		log.Printf("[workspace] failed to create popup pane: %v - allowing native popup", err)
		return nil
	}

	// Determine placement direction
	direction := strings.ToLower(popCfg.Placement)
	if direction == "" {
		direction = "right"
	}

	// Determine target node for splitting
	target := node
	if !popCfg.FollowPaneContext && wm.GetActiveNode() != nil {
		target = wm.GetActiveNode()
	}

	// Add the popup pane to the workspace using manual pane insertion
	if err := wm.insertPopupPane(target, newPane, direction); err != nil {
		log.Printf("[workspace] popup pane insertion failed: %v - allowing native popup", err)
		return nil
	}

	// Get the new node and popup WebView ID for tracking
	newNode := wm.viewToNode[newView]
	popupWebViewID := newView.ID()

	// Apply different behavior based on target type
	if isBlankTarget {
		log.Printf("[workspace] Treating _blank target as regular pane - no auto-close behavior")
		// For _blank targets, set up parent relationship but no auto-close
		if newNode != nil {
			newNode.parentPane = node
		}

		// Register close handler to clean up activePopupChildren tracking
		newView.RegisterCloseHandler(func() {
			log.Printf("[workspace] _blank popup requesting close via window.close()")
			// Remove this popup from parent's activePopupChildren
			if node != nil && popupWebViewID != "" {
				for i, childID := range node.activePopupChildren {
					if childID == popupWebViewID {
						node.activePopupChildren = append(node.activePopupChildren[:i], node.activePopupChildren[i+1:]...)
						log.Printf("[workspace] Removed _blank popup %s from parent's activePopupChildren (remaining: %d)", popupWebViewID, len(node.activePopupChildren))
						break
					}
				}
			}
		})
	} else {
		log.Printf("[workspace] Treating as popup pane - applying popup-specific behavior")
		// Mark as popup for auto-close handling (OAuth flows, etc.)
		if newNode != nil {
			newNode.isPopup = true
			newNode.parentPane = node
			log.Printf("[workspace] Marked pane as popup for auto-close handling")

			// Register close handler for popup auto-close on window.close()
			newView.RegisterCloseHandler(func() {
				log.Printf("[workspace] Popup requesting close via window.close()")

				// Remove this popup from parent's activePopupChildren
				if node != nil && popupWebViewID != "" {
					for i, childID := range node.activePopupChildren {
						if childID == popupWebViewID {
							node.activePopupChildren = append(node.activePopupChildren[:i], node.activePopupChildren[i+1:]...)
							log.Printf("[workspace] Removed popup %s from parent's activePopupChildren (remaining: %d)", popupWebViewID, len(node.activePopupChildren))
							break
						}
					}
				}

				// Look up the node at close time
				if node := wm.viewToNode[newView]; node != nil && node.isPopup {
					log.Printf("[workspace] Closing popup pane")
					// Brief delay to allow any final redirects to complete
					time.AfterFunc(200*time.Millisecond, func() {
						wm.scheduleIdleGuarded(func() bool {
							if node == nil || !node.widgetValid {
								return false
							}
							if err := wm.ClosePane(node); err != nil {
								log.Printf("[workspace] Failed to close popup pane: %v", err)
							}
							return false
						}, node)
					})
				} else {
					log.Printf("[workspace] Could not find popup node for close handler")
				}
			})
		} else {
			log.Printf("[workspace] Warning: could not find node for popup WebView in viewToNode map")
		}
	}

	// Track this popup in the parent's activePopupChildren (for both _blank and popup types)
	// This is critical to prevent OAuth flows from hijacking the parent pane via window.opener
	if node != nil && popupWebViewID != "" {
		if node.activePopupChildren == nil {
			node.activePopupChildren = make([]string, 0)
		}
		node.activePopupChildren = append(node.activePopupChildren, popupWebViewID)
		log.Printf("[workspace] Added popup %s to parent's activePopupChildren (count: %d)", popupWebViewID, len(node.activePopupChildren))

		// Register navigation policy handler for parent to block script-initiated navigations
		// when it has active popup children (prevents OAuth from hijacking parent pane)
		if node.pane != nil && node.pane.WebView() != nil {
			node.pane.WebView().RegisterNavigationPolicyHandler(func(url string, isUserGesture bool) bool {
				// Always allow user-initiated navigations
				if isUserGesture {
					return true
				}

				// Block script-initiated navigations if this pane has active popup children
				if len(node.activePopupChildren) > 0 {
					log.Printf("[workspace] BLOCKED script-initiated navigation in parent pane (has %d active popups): %s", len(node.activePopupChildren), url)
					return false
				}

				// Allow if no active popups
				return true
			})
			log.Printf("[workspace] Registered navigation policy handler for parent pane")
		}
	}

	// Ensure GUI components are available in the new pane
	wm.ensureGUIInPane(newPane)

	// Inject GUI components into the popup pane
	wm.ensureGUIInPane(newPane)

	// Load the URL if provided
	if url != "" {
		paneType := "popup"
		if isBlankTarget {
			paneType = "_blank target"
		}
		log.Printf("[webkit] LoadURL (%s): %s", paneType, url)
		if err := newView.LoadURL(url); err != nil {
			log.Printf("[workspace] failed to load %s URL: %v", paneType, err)
		}
		// Ensure the WebView is visible after loading URL
		if err := newView.Show(); err != nil {
			log.Printf("[workspace] failed to show popup WebView: %v", err)
		}
	}

	if isBlankTarget {
		log.Printf("[workspace] Created regular pane for _blank target URL: %s", url)
	} else {
		log.Printf("[workspace] Created popup pane for URL: %s", url)
	}
	return newView
}

// registerOAuthAutoClose sets up OAuth auto-close functionality for popups
// Note: OAuth detection is now handled by the main-world.js injection script
func (wm *WorkspaceManager) registerOAuthAutoClose(view *webkit.WebView, url string) {
	log.Printf("[workspace] OAuth auto-close enabled for popup with URL: %s", url)
	log.Printf("[workspace] OAuth detection will be handled by main-world.js injection script")
}

// applyWindowFeatures applies window features to a WebView based on intent
func (wm *WorkspaceManager) applyWindowFeatures(view *webkit.WebView, intent *messaging.WindowIntent, isPopup bool) {
	if intent == nil {
		return
	}

	features := &webkit.WindowFeatures{}

	// Apply dimensions if specified
	if intent.Width != nil {
		features.Width = *intent.Width
	}
	if intent.Height != nil {
		features.Height = *intent.Height
	}

	// Apply visibility features based on window type
	defaultToolbar := !isPopup
	defaultLocation := !isPopup
	defaultMenubar := !isPopup

	if intent.Toolbar != nil {
		features.ToolbarVisible = *intent.Toolbar
	} else {
		features.ToolbarVisible = defaultToolbar
	}

	if intent.Location != nil {
		features.LocationbarVisible = *intent.Location
	} else {
		features.LocationbarVisible = defaultLocation
	}

	if intent.Menubar != nil {
		features.MenubarVisible = *intent.Menubar
	} else {
		features.MenubarVisible = defaultMenubar
	}

	if intent.Resizable != nil {
		features.Resizable = *intent.Resizable
	} else {
		features.Resizable = true // Usually resizable unless explicitly disabled
	}

	view.SetWindowFeatures(features)
	windowTypeStr := "tab"
	if isPopup {
		windowTypeStr = "popup"
	}
	log.Printf("[workspace] Applied %s window features from intent: size=%dx%d, toolbar=%t, location=%t, menubar=%t, resizable=%t",
		windowTypeStr, features.Width, features.Height, features.ToolbarVisible, features.LocationbarVisible, features.MenubarVisible, features.Resizable)
}

// handleIntentAsTab creates a tab pane based on window.open intent
func (wm *WorkspaceManager) handleIntentAsTab(sourceNode *paneNode, url string, intent *messaging.WindowIntent) *webkit.WebView {
	log.Printf("[workspace] Handling intent as tab: %s", url)

	webkitCfg, err := wm.app.buildWebkitConfig()
	if err != nil {
		log.Printf("[workspace] failed to build webkit config: %v - allowing native popup", err)
		return nil
	}
	webkitCfg.CreateWindow = false

	newView, err := webkit.NewWebView(webkitCfg)
	if err != nil {
		log.Printf("[workspace] failed to create tab WebView: %v - allowing native popup", err)
		return nil
	}

	newPane, err := wm.createPane(newView)
	if err != nil {
		log.Printf("[workspace] failed to create tab pane: %v - allowing native popup", err)
		return nil
	}

	direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
	if direction == "" {
		direction = "right"
	}

	if err := wm.insertPopupPane(sourceNode, newPane, direction); err != nil {
		log.Printf("[workspace] tab pane insertion failed: %v - allowing native popup", err)
		return nil
	}

	node := wm.viewToNode[newView]
	if node != nil {
		node.windowType = webkit.WindowTypeTab
		node.isRelated = false

		// Apply window features from JavaScript intent
		wm.applyWindowFeatures(newView, intent, false)
	}

	wm.ensureGUIInPane(newPane)

	if url != "" {
		if err := newView.LoadURL(url); err != nil {
			log.Printf("[workspace] failed to load tab URL: %v", err)
		}
		if err := newView.Show(); err != nil {
			log.Printf("[workspace] failed to show tab WebView: %v", err)
		}
	}

	log.Printf("[workspace] Created tab pane for URL: %s", url)
	return newView
}

// handleIntentAsPopup creates a related popup pane based on window.open intent
func (wm *WorkspaceManager) handleIntentAsPopup(sourceNode *paneNode, url string, intent *messaging.WindowIntent) *webkit.WebView {
	log.Printf("[workspace] Handling intent as popup: %s", url)

	webkitCfg, err := wm.app.buildWebkitConfig()
	if err != nil {
		log.Printf("[workspace] failed to build webkit config: %v - allowing native popup", err)
		return nil
	}
	webkitCfg.CreateWindow = false

	newView, err := webkit.NewWebViewWithRelated(webkitCfg, sourceNode.pane.webView)
	if err != nil {
		log.Printf("[workspace] failed to create popup WebView: %v - allowing native popup", err)
		return nil
	}

	// Log the parent-popup WebView ID relationship for OAuth auto-close
	parentWebViewID := sourceNode.pane.webView.ID()
	popupWebViewID := newView.ID()
	log.Printf("[workspace] Created popup WebView: parentID=%s popupID=%s url=%s", parentWebViewID, popupWebViewID, url)

	// Store popup WebView ID in parent's localStorage for OAuth callback lookup
	storeScript := fmt.Sprintf(`
		try {
			const parentWebViewId = '%s';
			const popupWebViewId = '%s';
			const popupMapping = {
				parentId: parentWebViewId,
				popupId: popupWebViewId,
				timestamp: Date.now(),
				url: '%s'
			};
			localStorage.setItem('popup_mapping_' + parentWebViewId, JSON.stringify(popupMapping));
			console.log('[workspace] Stored popup mapping:', popupMapping);
		} catch(e) {
			console.warn('[workspace] Failed to store popup mapping:', e);
		}
	`, parentWebViewID, popupWebViewID, url)

	// Inject into parent WebView so it can find its popup later
	if err := sourceNode.pane.webView.InjectScript(storeScript); err != nil {
		log.Printf("[workspace] Failed to inject popup mapping script into parent: %v", err)
	}

	newPane, err := wm.createPane(newView)
	if err != nil {
		log.Printf("[workspace] failed to create popup pane: %v - allowing native popup", err)
		return nil
	}

	direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
	if direction == "" {
		direction = "right"
	}

	if err := wm.insertPopupPane(sourceNode, newPane, direction); err != nil {
		log.Printf("[workspace] popup pane insertion failed: %v - allowing native popup", err)
		return nil
	}

	node := wm.viewToNode[newView]
	var requestID string
	if node != nil {
		node.windowType = webkit.WindowTypePopup
		node.isRelated = true
		node.parentPane = sourceNode
		node.isPopup = true
		node.autoClose = wm.shouldAutoClose(url)

		// Track this popup in the parent's activePopupChildren
		if sourceNode != nil {
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

		// Store requestID for deduplication cleanup
		if intent != nil {
			requestID = intent.RequestID
			node.requestID = requestID
		}

		// Apply window features from JavaScript intent
		wm.applyWindowFeatures(newView, intent, true)

		// Apply minimum size constraints to prevent compression
		var width, height int
		if intent != nil {
			if intent.Width != nil {
				width = *intent.Width
			}
			if intent.Height != nil {
				height = *intent.Height
			}
		}
		wm.applyPopupSizeConstraints(newView, width, height)
	}

	// Register close handler for popup auto-close
	newView.RegisterCloseHandler(func() {
		log.Printf("[workspace] Popup requesting close via window.close()")

		// Clear the RequestID from deduplicator to allow new popups with same ID
		if requestID != "" && wm.paneDeduplicator != nil {
			wm.paneDeduplicator.ClearRequestID(requestID)
		}

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

		if n := wm.viewToNode[newView]; n != nil && n.isPopup {
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

	// URL-based auto-close for OAuth popups
	if node != nil && node.isPopup && node.autoClose {
		wm.registerOAuthAutoClose(newView, url)
	}

	wm.ensureGUIInPane(newPane)

	if url != "" {
		if err := newView.LoadURL(url); err != nil {
			log.Printf("[workspace] failed to load popup URL: %v", err)
		}
		if err := newView.Show(); err != nil {
			log.Printf("[workspace] failed to show popup WebView: %v", err)
		}
	}

	log.Printf("[workspace] Created popup pane for URL: %s", url)
	return newView
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
	popupWebViewID := related.ID()

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
	if container := view.RootWidget(); container != 0 && webkit.WidgetIsValid(container) {
		webkit.WidgetSetSizeRequest(container, minWidth, minHeight)
		webkit.WidgetQueueResize(container)
		log.Printf("[workspace] Applied minimum size %dx%d to popup container=%#x", minWidth, minHeight, container)
	}

	// Apply to WebView widget
	if webViewWidget := view.Widget(); webViewWidget != 0 && webkit.WidgetIsValid(webViewWidget) {
		webkit.WidgetSetSizeRequest(webViewWidget, minWidth, minHeight)
		webkit.WidgetQueueResize(webViewWidget)
		log.Printf("[workspace] Applied minimum size %dx%d to popup webview=%#x", minWidth, minHeight, webViewWidget)
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
		webkitCfg.CreateWindow = false

		// Create independent WebView like handleIntentAsTab does
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

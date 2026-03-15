package content

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

// oauthFlowPatterns are URL patterns that indicate an OAuth flow is in progress.
// These appear in authorization URLs and callback redirects.
var oauthFlowPatterns = []string{
	// OAuth flow indicators
	"oauth",
	"authorize",
	"authorization",
	"auth/",
	"/auth",
	"login",
	"signin",
	"sign-in",
	// OpenID Connect
	"oidc",
	"openid",
	// Callback/redirect endpoints
	"callback",
	"redirect",
	"/cb",
}

// oauthCallbackPatterns are URL query parameters that indicate an OAuth callback.
// These appear when the OAuth provider redirects back to the application.
var oauthCallbackPatterns = []string{
	// Success parameters
	"code=",
	"access_token=",
	"id_token=",
	"token_type=",
	"refresh_token=",
	// Error parameters
	"error=",
	"error_description=",
	"error_uri=",
}

// oauthRequestPatterns are query parameters found in OAuth authorization requests.
var oauthRequestPatterns = []string{
	"response_type=",
	"client_id=",
	"redirect_uri=",
	"scope=",
	"nonce=",
}

// IsOAuthURL checks if the URL is related to an OAuth flow.
// This includes authorization endpoints, login pages, and callback URLs.
func IsOAuthURL(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)

	// Check for OAuth flow patterns in URL path
	for _, pattern := range oauthFlowPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// Check for OAuth request parameters
	for _, pattern := range oauthRequestPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// IsOAuthCallback checks if the URL is an OAuth callback with response parameters.
// This indicates the OAuth flow has completed (successfully or with error).
func IsOAuthCallback(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)

	// Must contain callback patterns (code=, access_token=, error=, etc.)
	for _, pattern := range oauthCallbackPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// ShouldAutoClose determines if a popup at this URL should auto-close.
// Returns true for OAuth callbacks that indicate flow completion.
//
// This is comprehensive detection that handles:
// - Success: code=, access_token=, id_token=
// - Errors: error=, error_description=
// - Various OAuth providers (Google, GitHub, Auth0, etc.)
func ShouldAutoClose(url string) bool {
	return IsOAuthCallback(url)
}

// shouldForceCloseOnSafetyTimeout determines if a popup should be force-closed
// when the OAuth safety timeout is reached.
//
// Safety-timeout force close is disabled for stability. OAuth popups close on
// callback detection or manual user action.
func shouldForceCloseOnSafetyTimeout(url string) bool {
	_ = url
	return false
}

// IsOAuthSuccess checks if the callback indicates successful authentication.
func IsOAuthSuccess(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)

	// Check for success indicators
	successPatterns := []string{
		"code=",
		"access_token=",
		"id_token=",
	}

	for _, pattern := range successPatterns {
		if strings.Contains(lower, pattern) {
			// Make sure it's not an error response
			if !strings.Contains(lower, "error=") {
				return true
			}
		}
	}

	return false
}

// IsOAuthError checks if the callback indicates an authentication error.
func IsOAuthError(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)
	return strings.Contains(lower, "error=")
}

func composeOnClose(existing, next func()) func() {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	return func() {
		existing()
		next()
	}
}

func composeOnURIChanged(existing, next func(string)) func(string) {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	return func(uri string) {
		existing(uri)
		next(uri)
	}
}

func composeOnLoadChanged(
	existing func(port.LoadEvent),
	next func(port.LoadEvent),
) func(port.LoadEvent) {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	return func(event port.LoadEvent) {
		existing(event)
		next(event)
	}
}

// setupOAuthAutoClose monitors the popup for OAuth callback URLs and auto-closes.
// It uses URL pattern detection for standard OAuth callbacks (code=, access_token=, etc.)
// For providers using postMessage (like Google Sign-In), we rely on the provider calling
// window.close() which triggers WebKit's close signal.
// A long safety timeout (30s) catches popups that get stuck.
func (c *Coordinator) setupOAuthAutoClose(
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
	const oauthCloseDelay = 500 * time.Millisecond

	startSafetyTimer := func() {
		safetyTimerMu.Lock()
		defer safetyTimerMu.Unlock()
		if safetyTimer != nil {
			safetyTimer.Stop()
		}
		safetyTimer = time.AfterFunc(oauthSafetyTimeout, func() {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				if wv != nil && !wv.IsDestroyed() {
					uri := wv.URI()
					if shouldForceCloseOnSafetyTimeout(uri) {
						log.Warn().Str("pane", string(paneID)).Msg("oauth safety timeout, closing stuck popup")
						wv.Close()
						return false
					}
					log.Debug().
						Str("pane", string(paneID)).
						Str("uri", logging.TruncateURL(uri, logURLMaxLen)).
						Msg("oauth safety timeout reached during active auth flow, skipping forced close")
				}
				return false
			})
			glib.IdleAdd(&cb, 0)
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
			time.AfterFunc(oauthCloseDelay, func() {
				cb := glib.SourceFunc(func(_ uintptr) bool {
					if wv != nil && !wv.IsDestroyed() {
						wv.Close()
					}
					return false
				})
				glib.IdleAdd(&cb, 0)
			})
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

func (c *Coordinator) trackOAuthPopup(popupID port.WebViewID, parentPaneID entity.PaneID) {
	c.popupMu.Lock()
	defer c.popupMu.Unlock()
	if c.popupOAuth == nil {
		c.popupOAuth = make(map[port.WebViewID]*popupOAuthState)
	}
	c.popupOAuth[popupID] = &popupOAuthState{
		ParentPaneID: parentPaneID,
	}
}

func (c *Coordinator) capturePopupOAuthState(popupID port.WebViewID, uri string) {
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

func (c *Coordinator) handlePopupOAuthClose(ctx context.Context, popupID port.WebViewID) {
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

func (c *Coordinator) scheduleParentPaneRefresh(
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

func (c *Coordinator) refreshPaneAfterOAuth(
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

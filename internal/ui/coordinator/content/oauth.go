package content

import (
	"context"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/glib"
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
//
// The matching is intentionally broad: both oauthFlowPatterns (path-based terms like
// "login", "authorize", "callback") and oauthRequestPatterns (query parameters like
// "client_id=" and "scope=") use simple substring matching without anchoring.
// This maximizes recall — we prefer false positives over missed detections — so that
// edge-case provider URLs and non-standard redirect paths are still caught. The
// trade-off is reduced precision: some unrelated URLs that happen to contain these
// substrings will be classified as OAuth URLs. Callers that need higher confidence
// should combine this with IsOAuthCallback, which checks for concrete response params.
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
	u, err := neturl.Parse(url)
	if err != nil {
		return false
	}
	if hasOAuthCallbackParam(u.Query()) {
		return true
	}
	if u.Fragment != "" {
		if fragVals, err := neturl.ParseQuery(u.Fragment); err == nil {
			return hasOAuthCallbackParam(fragVals)
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
func shouldForceCloseOnSafetyTimeout() bool {
	return false
}

// IsOAuthSuccess checks if the callback indicates successful authentication.
// It parses query and fragment parameters (like IsOAuthCallback) to avoid
// false positives from substring matching.
func IsOAuthSuccess(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return false
	}
	if hasOAuthSuccessParam(u.Query()) {
		return true
	}
	if u.Fragment != "" {
		if fragVals, err := neturl.ParseQuery(u.Fragment); err == nil {
			return hasOAuthSuccessParam(fragVals)
		}
	}
	return false
}

// hasOAuthSuccessParam returns true if params contain a success token
// (code, access_token, id_token) and no error key.
func hasOAuthSuccessParam(params neturl.Values) bool {
	if params.Has("error") {
		return false
	}
	return params.Has("code") || params.Has("access_token") || params.Has("id_token")
}

// IsOAuthError checks if the callback indicates an authentication error.
func IsOAuthError(url string) bool {
	if url == "" {
		return false
	}
	u, err := neturl.Parse(url)
	if err != nil {
		return false
	}
	if _, ok := u.Query()["error"]; ok {
		return true
	}
	if u.Fragment != "" {
		if fragVals, err := neturl.ParseQuery(u.Fragment); err == nil {
			if _, ok := fragVals["error"]; ok {
				return true
			}
		}
	}
	return false
}

func hasOAuthCallbackParam(v neturl.Values) bool {
	for _, k := range []string{"code", "access_token", "id_token", "token_type", "refresh_token", "error", "error_description", "error_uri"} {
		if _, ok := v[k]; ok {
			return true
		}
	}
	return false
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

func composeOnLoadChanged(existing, next func(port.LoadEvent)) func(port.LoadEvent) {
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
func popupUsesSyntheticOpenerSignals(wv port.WebView) bool {
	if wv == nil {
		return false
	}
	opener, ok := wv.(port.PopupOpenerCapable)
	return ok && opener.HasActivePopupOpenerBridge()
}

func (c *Coordinator) setupOAuthAutoClose(
	ctx context.Context,
	paneID entity.PaneID,
	popupID port.WebViewID,
	wv port.WebView,
) {
	log := logging.FromContext(ctx)

	// Use the optional OAuthCallbackCapable capability instead of a webkit-specific assertion.
	oauthWV, ok := wv.(port.OAuthCallbackCapable)
	if !ok {
		log.Debug().Str("pane_id", string(paneID)).Msg("oauth auto-close: webview does not support oauth callbacks")
		return
	}

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
					if shouldForceCloseOnSafetyTimeout() {
						log.Warn().Str("pane", string(paneID)).Msg("oauth safety timeout, closing stuck popup")
						oauthWV.Close()
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

	requestOAuthClose := func(reason string) {
		cancelSafetyTimer()
		log.Info().
			Str("pane", string(paneID)).
			Str("reason", reason).
			Msg("oauth callback detected, closing")
		requestCloseOnce.Do(func() {
			time.AfterFunc(oauthCloseDelay, func() {
				cb := glib.SourceFunc(func(_ uintptr) bool {
					if wv != nil && !wv.IsDestroyed() {
						oauthWV.Close()
					}
					return false
				})
				glib.IdleAdd(&cb, 0)
			})
		})
	}

	// Start safety timer immediately.
	startSafetyTimer()
	deferCloseOnNavigation := popupUsesSyntheticOpenerSignals(wv)
	log.Debug().
		Str("pane", string(paneID)).
		Uint64("popup_id", uint64(popupID)).
		Bool("synthetic_opener_active", deferCloseOnNavigation).
		Msg("oauth auto-close configured")

	// Register navigation callback to check for OAuth callbacks on URI changes and committed loads.
	oauthWV.AddNavigationCallback(func(uri string) {
		if !ShouldAutoClose(uri) {
			return
		}
		c.capturePopupOAuthState(popupID, uri)
		if deferCloseOnNavigation {
			cancelSafetyTimer()
			return
		}
		requestOAuthClose("navigation")
	})
	if opener, ok := wv.(port.PopupOpenerCapable); ok {
		opener.AddOpenerNavigationCallback(func(uri string) {
			c.capturePopupOAuthState(popupID, uri)
			requestOAuthClose("opener-navigation")
		})
		opener.AddOpenerMessageCallback(func() {
			c.capturePopupOAuthMessage(popupID)
			requestOAuthClose("opener-message")
		})
	}

	// Cancel safety timer on any close path.
	oauthWV.AddCloseCallback(func() {
		cancelSafetyTimer()
	})
}

func (c *Coordinator) trackOAuthPopup(popupID port.WebViewID, parentPaneID entity.PaneID, parentURIAtOpen string) {
	c.ensurePopupManager().trackOAuthPopup(popupID, parentPaneID, parentURIAtOpen)
}

func (c *Coordinator) capturePopupOAuthState(popupID port.WebViewID, uri string) {
	c.ensurePopupManager().capturePopupOAuthState(popupID, uri)
}

func (c *Coordinator) capturePopupOAuthMessage(popupID port.WebViewID) {
	c.ensurePopupManager().capturePopupOAuthMessage(popupID)
}

func (c *Coordinator) handlePopupOAuthClose(ctx context.Context, popupID port.WebViewID) {
	log := logging.FromContext(ctx)

	state, ok := c.ensurePopupManager().takePopupOAuthState(popupID)

	if !ok || state == nil || !state.Seen {
		return
	}

	log.Debug().
		Uint64("popup_id", uint64(popupID)).
		Str("parent_pane_id", string(state.ParentPaneID)).
		Bool("oauth_success", state.Success).
		Bool("oauth_error", state.Error).
		Msg("popup oauth result captured on close")

	if state.ParentPaneID == "" {
		return
	}

	if !state.Success {
		return
	}

	c.scheduleParentPaneOAuthResume(ctx, state.ParentPaneID, popupID, state.ParentURIAtOpen, state.CallbackURI)
}

// oauthParentResumeGraceRetries gives the parent pane up to
// oauthParentResumeGraceRetries * oauthParentRefreshDebounce (~1s with the
// current 200ms debounce) to finish its own in-flight same-site OAuth
// navigation before scheduleParentPaneOAuthResume falls back to Reload(). Five
// retries keeps the UI responsive without racing the natural callback handoff.
const oauthParentResumeGraceRetries = 5

func (c *Coordinator) scheduleParentPaneOAuthResume(
	ctx context.Context,
	parentPaneID entity.PaneID,
	popupID port.WebViewID,
	parentURIAtOpen, callbackURI string,
) {
	c.scheduleParentPaneOAuthResumeAttempt(
		ctx,
		parentPaneID,
		popupID,
		parentURIAtOpen,
		callbackURI,
		oauthParentResumeGraceRetries,
	)
}

func (c *Coordinator) scheduleParentPaneOAuthResumeAttempt(
	ctx context.Context,
	parentPaneID entity.PaneID,
	popupID port.WebViewID,
	parentURIAtOpen, callbackURI string,
	remainingGrace int,
) {
	c.ensurePopupManager().schedulePopupRefresh(parentPaneID, oauthParentRefreshDebounce, func() {
		cb := glib.SourceFunc(func(_ uintptr) bool {
			c.resumeParentPaneAfterOAuthAttempt(ctx, parentPaneID, popupID, parentURIAtOpen, callbackURI, remainingGrace)
			return false
		})
		glib.IdleAdd(&cb, 0)
	})
}

func (c *Coordinator) resumeParentPaneAfterOAuth(
	ctx context.Context,
	parentPaneID entity.PaneID,
	popupID port.WebViewID,
	parentURIAtOpen, callbackURI string,
) {
	c.resumeParentPaneAfterOAuthAttempt(
		ctx,
		parentPaneID,
		popupID,
		parentURIAtOpen,
		callbackURI,
		oauthParentResumeGraceRetries,
	)
}

func (c *Coordinator) resumeParentPaneAfterOAuthAttempt(
	ctx context.Context,
	parentPaneID entity.PaneID,
	popupID port.WebViewID,
	parentURIAtOpen, callbackURI string,
	remainingGrace int,
) {
	log := logging.FromContext(ctx)
	wv := c.getWebViewLocked(parentPaneID)
	if wv == nil || wv.IsDestroyed() {
		log.Debug().
			Str("parent_pane_id", string(parentPaneID)).
			Uint64("popup_id", uint64(popupID)).
			Msg("skipping parent pane oauth resume: parent webview unavailable")
		return
	}

	parentURI := strings.TrimSpace(wv.URI())
	if parentURIAtOpen != "" && parentURI != "" && parentURI != parentURIAtOpen {
		log.Info().
			Str("parent_pane_id", string(parentPaneID)).
			Uint64("popup_id", uint64(popupID)).
			Str("parent_uri_at_open", logging.TruncateURL(parentURIAtOpen, logURLMaxLen)).
			Str("current_parent_uri", logging.TruncateURL(parentURI, logURLMaxLen)).
			Str("callback_uri", logging.TruncateURL(callbackURI, logURLMaxLen)).
			Msg("skipping parent pane oauth resume: parent already navigated")
		return
	}

	if shouldGraceWaitForParentPaneOAuthResume(parentURIAtOpen, parentURI, callbackURI, remainingGrace) {
		log.Debug().
			Str("parent_pane_id", string(parentPaneID)).
			Uint64("popup_id", uint64(popupID)).
			Int("remaining_grace", remainingGrace).
			Str("parent_uri_at_open", logging.TruncateURL(parentURIAtOpen, logURLMaxLen)).
			Str("callback_uri", logging.TruncateURL(callbackURI, logURLMaxLen)).
			Msg("deferring parent pane oauth resume to allow in-flight same-site navigation to settle")
		c.scheduleParentPaneOAuthResumeAttempt(
			ctx,
			parentPaneID,
			popupID,
			parentURIAtOpen,
			callbackURI,
			remainingGrace-1,
		)
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
		Str("callback_uri", logging.TruncateURL(callbackURI, logURLMaxLen)).
		Msg("refreshed parent pane after oauth popup close")
}

func shouldGraceWaitForParentPaneOAuthResume(
	parentURIAtOpen, currentParentURI, callbackURI string,
	remainingGrace int,
) bool {
	if remainingGrace <= 0 {
		return false
	}
	if parentURIAtOpen == "" || currentParentURI == "" || callbackURI == "" {
		return false
	}
	if currentParentURI != parentURIAtOpen {
		return false
	}
	return sameOAuthResumeSite(parentURIAtOpen, callbackURI)
}

func sameOAuthResumeSite(a, b string) bool {
	hostA := normalizeOAuthResumeHost(a)
	hostB := normalizeOAuthResumeHost(b)
	return hostA != "" && hostA == hostB
}

func normalizeOAuthResumeHost(raw string) string {
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	host = strings.TrimPrefix(host, "www.")
	return host
}

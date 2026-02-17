package coordinator

import (
	"strings"

	"github.com/bnema/dumber/internal/infrastructure/webkit"
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
	existing func(webkit.LoadEvent),
	next func(webkit.LoadEvent),
) func(webkit.LoadEvent) {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	return func(event webkit.LoadEvent) {
		existing(event)
		next(event)
	}
}

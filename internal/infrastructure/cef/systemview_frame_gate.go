package cef

import (
	"net/http"

	purecef "bnema.dev/purego-cef/cef"
)

// subframeDeniedAPIPaths are private system-view APIs that only the top-level
// system-view page may call. An iframe embedded in dumb://home (for example a
// dashboard panel) must never reach them, even if it ends up on the internal
// origin.
var subframeDeniedAPIPaths = map[string]bool{
	"/api/message":        true,
	"/api/config":         true,
	"/api/config/default": true,
}

// rejectSubframeSystemviewAPI denies private system-view API requests that come
// from a subframe. It is defense in depth on top of the Origin/Referer check:
// cross-origin iframes are already rejected there. A request without a frame
// is rejected too: system view pages always send these from their main frame.
func (h *dumbSchemeHandler) rejectSubframeSystemviewAPI(frame purecef.Frame, apiPath string) purecef.ResourceHandler {
	if !subframeDeniedAPIPaths[apiPath] || (frame != nil && frame.IsMain()) {
		return nil
	}
	return h.newPrivateAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"})
}

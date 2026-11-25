package webkit

import (
	"context"
	"net/http"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/infrastructure/webutil"
	"bnema.dev/puregotk/v4/glib"
	"bnema.dev/puregotk/v4/webkit"
)

const (
	homeIconAPIPath      = "/api/home-icon"
	homeIconCacheControl = "max-age=3600"
)

var _ port.HomeIconResolverSetter = (*Engine)(nil)

// SetHomeIconResolver exposes home service icons to the home systemview via
// dumb://home/api/home-icon.
func (e *Engine) SetHomeIconResolver(resolver port.HomeIconResolver) {
	if e == nil || e.schemeHandler == nil {
		return
	}
	e.schemeHandler.SetHomeIconResolver(resolver)
}

// SetHomeIconResolver wires the resolver used by /api/home-icon.
func (h *DumbSchemeHandler) SetHomeIconResolver(resolver port.HomeIconResolver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.homeIconResolver = resolver
}

// homeIconRequest validates a home icon request. It returns either an
// immediate response (errors) or a resolve function to run off the main loop.
func (h *DumbSchemeHandler) homeIconRequest(req *SchemeRequest) (*SchemeResponse, func(context.Context) *SchemeResponse) {
	if req != nil && req.Method != "" && req.Method != httpGET {
		return privateJSONErrorResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	if !isTrustedSystemviewImageRequest(req) {
		return privateJSONErrorResponse(http.StatusForbidden, "forbidden"), nil
	}
	h.mu.RLock()
	resolver := h.homeIconResolver
	h.mu.RUnlock()
	if resolver == nil {
		return privateJSONErrorResponse(http.StatusNotFound, "home icons unavailable"), nil
	}
	serviceID, err := webutil.ParseHomeIconRequest(req.URI)
	if err != nil {
		return privateJSONErrorResponse(http.StatusBadRequest, err.Error()), nil
	}
	return nil, func(ctx context.Context) *SchemeResponse {
		data, contentType, err := resolver.ResolveHomeIconByID(ctx, serviceID)
		if err != nil || len(data) == 0 {
			return privateJSONErrorResponse(http.StatusNotFound, "home icon not found")
		}
		if contentType == "" {
			contentType = "image/png"
		}
		return &SchemeResponse{
			Data:                   data,
			ContentType:            contentType,
			StatusCode:             http.StatusOK,
			Headers:                map[string]string{"Cache-Control": homeIconCacheControl},
			SuppressDefaultHeaders: true,
		}
	}
}

// serveHomeIconAsync resolves the icon on a goroutine and finishes the
// WebKit request back on the GTK main loop, so slow network discovery never
// blocks the UI.
func (h *DumbSchemeHandler) serveHomeIconAsync(req *webkit.URISchemeRequest, schemeReq *SchemeRequest) {
	immediate, resolve := h.homeIconRequest(schemeReq)
	if resolve == nil {
		h.sendResponse(req, immediate)
		return
	}
	ctx := h.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req.Ref()
	go func() {
		response := resolve(ctx)
		// glib copies the callback into its own registry and dispatches it
		// through one shared trampoline, so no per-call callback slot leaks.
		cb := glib.SourceFunc(func(_ uintptr) bool {
			h.sendResponse(req, response)
			req.Unref()
			return false
		})
		glib.IdleAdd(&cb, 0)
	}()
}

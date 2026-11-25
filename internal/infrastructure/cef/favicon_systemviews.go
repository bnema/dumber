package cef

import (
	"context"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/favicon"
)

var _ port.SystemviewFaviconResolverSetter = (*Engine)(nil)
var _ port.FaviconInvalidator = (*Engine)(nil)

// SetSystemviewFaviconResolver exposes the application favicon resolver to
// internal systemview pages without giving engine code disk-service access.
func (e *Engine) SetSystemviewFaviconResolver(resolver port.FaviconSystemviewResolver) {
	if e == nil || e.schemeHandler == nil {
		return
	}
	e.schemeHandler.setFaviconResolver(resolver)
}

func (e *Engine) Invalidate(ctx context.Context, key favicon.Key) error {
	if e == nil || e.schemeHandler == nil {
		return nil
	}
	return e.schemeHandler.invalidateFavicon(ctx, key)
}

func (h *dumbSchemeHandler) invalidateFavicon(_ context.Context, _ favicon.Key) error {
	// CEF systemview favicon requests resolve through the application usecase on
	// each request, so this adapter does not own a favicon response cache.
	return nil
}

var _ port.HomeIconResolverSetter = (*Engine)(nil)

// SetHomeIconResolver exposes home service icons to the home systemview via
// /api/home-icon.
func (e *Engine) SetHomeIconResolver(resolver port.HomeIconResolver) {
	if e == nil || e.schemeHandler == nil {
		return
	}
	e.schemeHandler.setHomeIconResolver(resolver)
}

package cef

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/favicon"
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

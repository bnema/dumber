package webkit

import "github.com/bnema/dumber/internal/application/port"

var _ port.SystemviewFaviconResolverSetter = (*Engine)(nil)

// SetSystemviewFaviconResolver exposes the application favicon resolver to
// systemview pages via dumb://api/favicon without disk-service access.
func (e *Engine) SetSystemviewFaviconResolver(resolver port.FaviconSystemviewResolver) {
	if e == nil || e.schemeHandler == nil {
		return
	}
	e.schemeHandler.SetFaviconResolver(resolver)
}

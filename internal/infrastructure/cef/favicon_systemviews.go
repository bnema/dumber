package cef

import "github.com/bnema/dumber/internal/application/port"

var _ port.SystemviewFaviconServiceSetter = (*Engine)(nil)

// SetSystemviewFaviconService exposes the local favicon cache to internal
// systemview pages. The scheme handler only serves cached PNGs; it does not
// fetch remote favicons on behalf of History rendering.
func (e *Engine) SetSystemviewFaviconService(service port.FaviconService) {
	if e == nil || e.schemeHandler == nil {
		return
	}
	e.schemeHandler.setFaviconService(service)
}

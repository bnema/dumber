package webkit

import "github.com/bnema/dumber/internal/application/port"

var _ port.SystemviewFaviconServiceSetter = (*Engine)(nil)

// SetSystemviewFaviconService exposes cached favicons to systemview pages via
// dumb://api/favicon without triggering remote favicon fetches during History rendering.
func (e *Engine) SetSystemviewFaviconService(service port.FaviconService) {
	if e == nil || e.schemeHandler == nil {
		return
	}
	e.schemeHandler.SetFaviconService(service)
}

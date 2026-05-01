package cef

import purecef "github.com/bnema/purego-cef/cef"

func syncWindowlessBrowserFocus(host purecef.BrowserHost) {
	if host == nil {
		return
	}
	const (
		cefFalse = 0
		cefTrue  = 1
	)
	host.WasHidden(cefFalse) // visible
	host.SetFocus(cefTrue)   // focused
	host.Invalidate(purecef.PaintElementTypePetView)
}

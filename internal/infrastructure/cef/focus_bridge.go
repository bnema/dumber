package cef

import purecef "github.com/bnema/purego-cef/cef"

func syncWindowlessBrowserFocus(host purecef.BrowserHost) {
	if host == nil {
		return
	}
	host.WasHidden(0)
	host.SetFocus(1)
	host.Invalidate(purecef.PaintElementTypePetView)
}

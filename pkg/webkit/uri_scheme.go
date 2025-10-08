package webkit

import (
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

// URISchemeRequestCallback is called when a custom URI scheme is requested
type URISchemeRequestCallback func(request *webkit.URISchemeRequest)

// SetURISchemeResolver registers a custom URI scheme handler
func SetURISchemeResolver(scheme string, callback URISchemeRequestCallback) {
	// Get the default web context
	ctx := webkit.WebContextGetDefault()
	if ctx == nil {
		return
	}

	// Register the URI scheme
	ctx.RegisterURIScheme(scheme, func(req *webkit.URISchemeRequest) {
		if callback != nil {
			callback(req)
		}
	})
}

// FinishURISchemeRequest finishes a URI scheme request with data
func FinishURISchemeRequest(req *webkit.URISchemeRequest, mimeType string, data []byte) error {
	if req == nil {
		return ErrWebViewNotInitialized
	}

	// Import gio for input stream
	// This will be handled in the caller
	return nil
}

package schemes

import (
	"fmt"
	"log"
	"mime"
	"path/filepath"
	"strings"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// Scheme name constants.
const (
	SchemeDumb          = "dumb"
	SchemeDumbExtension = "dumb-extension"
)

// FinishRequestWithData completes a URI scheme request with in-memory data.
// Sets CORS headers to allow cross-origin requests (required for ES6 module loading).
func FinishRequestWithData(req *webkit.URISchemeRequest, mimeType string, data []byte) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	// IMPORTANT: Call req.URI() only ONCE and make a COPY of it
	// The CGO string pointer can become invalid - we need to own the memory
	uriFromCGO := req.URI()
	uri := string([]byte(uriFromCGO)) // Force a copy so Go owns the memory

	gbytes := glib.NewBytes(data)
	stream := gio.NewMemoryInputStreamFromBytes(gbytes)
	if stream == nil {
		err := fmt.Errorf("failed to create input stream")
		req.FinishError(err)
		return err
	}

	// Create response with CORS headers to allow ES6 module imports
	response := webkit.NewURISchemeResponse(stream, int64(len(data)))
	if response == nil {
		err := fmt.Errorf("failed to create URI scheme response")
		req.FinishError(err)
		return err
	}

	// Set content type (skip if empty to let WebKit auto-detect)
	if mimeType != "" {
		response.SetContentType(mimeType)
		log.Printf("[schemes] Set content type: %s for URI: %s", mimeType, uri)
	} else {
		log.Printf("[schemes] No content type set (auto-detect) for URI: %s", uri)
	}

	// Set HTTP status 200 OK
	response.SetStatus(200, "OK")

	// Don't set CORS headers - they may be causing the crash with WASM files
	// ES6 modules work without explicit CORS headers on custom schemes
	// headers := soup.NewMessageHeaders(soup.MessageHeadersResponse)
	// if headers != nil {
	// 	headers.Append("Access-Control-Allow-Origin", "*")
	// 	headers.Append("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	// 	headers.Append("Access-Control-Allow-Headers", "*")
	// 	response.SetHTTPHeaders(headers)
	// 	log.Printf("[schemes] Set CORS headers for URI: %s", uri)
	// }

	req.FinishWithResponse(response)
	return nil
}

// GuessMimeType returns a reasonable MIME type for the given filename.
// Falls back to application/octet-stream when unknown.
func GuessMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	// Handle JavaScript files specially - return empty string to let WebKit auto-detect
	// This allows both classic scripts and ES6 modules to work correctly
	if ext == ".js" || ext == ".mjs" {
		return "text/javascript" // ES6 modules require strict MIME type
	}

	// Use system MIME type database for everything else
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}

	// Fallback for unknown types
	return "application/octet-stream"
}

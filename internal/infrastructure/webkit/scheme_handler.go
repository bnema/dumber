package webkit

import (
	"context"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/rs/zerolog"
)

// Scheme path constants
const (
	HomePath    = "home"
	BlockedPath = "blocked"
	IndexHTML   = "index.html"
)

// SchemeRequest represents a request to a custom URI scheme.
type SchemeRequest struct {
	inner  *webkit.URISchemeRequest
	URI    string
	Path   string
	Method string
	Scheme string
}

// SchemeResponse represents a response to a scheme request.
type SchemeResponse struct {
	Data        []byte
	ContentType string
	StatusCode  int
}

// PageHandler generates content for a specific page path.
type PageHandler interface {
	Handle(req *SchemeRequest) *SchemeResponse
}

// PageHandlerFunc is an adapter to allow use of ordinary functions as PageHandlers.
type PageHandlerFunc func(req *SchemeRequest) *SchemeResponse

func (f PageHandlerFunc) Handle(req *SchemeRequest) *SchemeResponse {
	return f(req)
}

// DumbSchemeHandler handles dumb:// URI scheme requests.
type DumbSchemeHandler struct {
	handlers map[string]PageHandler
	assets   embed.FS
	assetDir string // subdirectory within embed.FS (e.g., "assets/webui")
	logger   zerolog.Logger
	mu       sync.RWMutex
}

// NewDumbSchemeHandler creates a new handler for the dumb:// scheme.
func NewDumbSchemeHandler(ctx context.Context) *DumbSchemeHandler {
	log := logging.FromContext(ctx)

	h := &DumbSchemeHandler{
		handlers: make(map[string]PageHandler),
		assetDir: "webui",
		logger:   log.With().Str("component", "scheme-handler").Logger(),
	}

	// Register default pages
	h.registerDefaults()

	return h
}

// SetAssets sets the embedded filesystem containing webui assets.
func (h *DumbSchemeHandler) SetAssets(assets embed.FS) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.assets = assets
	h.logger.Debug().Msg("assets filesystem configured")
}

// registerDefaults sets up default page handlers.
func (h *DumbSchemeHandler) registerDefaults() {
	// Error page (static fallback)
	h.RegisterPage("/error", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		return &SchemeResponse{
			Data:        []byte(errorPageHTML),
			ContentType: "text/html",
			StatusCode:  http.StatusOK,
		}
	}))
}

// RegisterPage registers a handler for a specific path.
func (h *DumbSchemeHandler) RegisterPage(path string, handler PageHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[path] = handler
	h.logger.Debug().Str("path", path).Msg("registered page handler")
}

// HandleRequest processes a scheme request and sends the response.
func (h *DumbSchemeHandler) HandleRequest(reqPtr uintptr) {
	req := webkit.URISchemeRequestNewFromInternalPtr(reqPtr)
	if req == nil {
		return
	}

	uri := req.GetUri()
	schemeReq := &SchemeRequest{
		inner:  req,
		URI:    uri,
		Path:   req.GetPath(),
		Method: req.GetHttpMethod(),
		Scheme: req.GetScheme(),
	}

	h.logger.Debug().
		Str("uri", schemeReq.URI).
		Str("path", schemeReq.Path).
		Str("method", schemeReq.Method).
		Msg("handling scheme request")

	// Parse the URI to extract host and path
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "dumb" {
		h.logger.Error().Err(err).Str("uri", uri).Msg("invalid URI")
		h.sendResponse(req, &SchemeResponse{
			Data:        []byte("Invalid URI"),
			ContentType: "text/plain",
			StatusCode:  http.StatusBadRequest,
		})
		return
	}

	// Try to serve from embedded assets first
	if response := h.handleAsset(u); response != nil {
		h.sendResponse(req, response)
		return
	}

	// Fall back to registered handlers
	h.mu.RLock()
	handler, ok := h.handlers[schemeReq.Path]
	if !ok {
		handler, ok = h.handlers[strings.TrimPrefix(schemeReq.Path, "/")]
	}
	h.mu.RUnlock()

	var response *SchemeResponse
	if ok {
		response = handler.Handle(schemeReq)
	} else {
		response = &SchemeResponse{
			Data:        []byte(notFoundHTML),
			ContentType: "text/html",
			StatusCode:  http.StatusNotFound,
		}
	}

	h.sendResponse(req, response)
}

// handleAsset serves static assets from the embedded filesystem.
// Returns nil if no asset was found (allowing fallback to registered handlers).
func (h *DumbSchemeHandler) handleAsset(u *url.URL) *SchemeResponse {
	h.mu.RLock()
	hasAssets := h.assets != (embed.FS{})
	assetDir := h.assetDir
	h.mu.RUnlock()

	if !hasAssets {
		return nil
	}

	// Determine the target file based on host and path
	host := u.Host
	path := strings.TrimPrefix(u.Path, "/")

	var relPath string
	switch {
	// dumb://home or dumb://home/ → index.html
	case host == HomePath && (path == "" || path == "/"):
		relPath = IndexHTML
	// dumb://home/<asset> → serve asset
	case host == HomePath && path != "":
		relPath = path
	// dumb://blocked or dumb://blocked/ → blocked.html
	case host == BlockedPath && (path == "" || path == "/"):
		relPath = "blocked.html"
	// dumb://blocked/<asset> → serve asset
	case host == BlockedPath && path != "":
		relPath = path
	// dumb:home (opaque form) → index.html
	case u.Opaque == HomePath:
		relPath = IndexHTML
	// dumb:blocked (opaque form) → blocked.html
	case u.Opaque == BlockedPath:
		relPath = "blocked.html"
	default:
		// Not a recognized asset path
		return nil
	}

	// Read the asset from embedded FS
	fullPath := filepath.ToSlash(filepath.Join(assetDir, relPath))
	data, err := fs.ReadFile(h.assets, fullPath)
	if err != nil {
		h.logger.Debug().Str("path", fullPath).Err(err).Msg("asset not found")
		return nil
	}

	contentType := h.getMimeType(relPath)
	h.logger.Debug().
		Str("path", fullPath).
		Str("content_type", contentType).
		Int("size", len(data)).
		Msg("serving asset")

	return &SchemeResponse{
		Data:        data,
		ContentType: contentType,
		StatusCode:  http.StatusOK,
	}
}

// getMimeType determines the MIME type for a given file path.
func (h *DumbSchemeHandler) getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	// Try standard mime type first
	mt := mime.TypeByExtension(ext)
	if mt != "" {
		return mt
	}

	// Fallbacks for common web assets
	switch ext {
	case ".js", ".mjs":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	default:
		return "text/plain"
	}
}

// sendResponse sends the response back to WebKit.
func (h *DumbSchemeHandler) sendResponse(req *webkit.URISchemeRequest, response *SchemeResponse) {
	if response == nil {
		response = &SchemeResponse{
			Data:        []byte("Internal error"),
			ContentType: "text/plain",
			StatusCode:  http.StatusInternalServerError,
		}
	}

	contentType := response.ContentType
	if contentType == "" {
		contentType = "text/html"
	}

	// Create MemoryInputStream from data directly
	stream := gio.NewMemoryInputStreamFromData(response.Data, len(response.Data), nil)
	if stream == nil {
		h.logger.Error().Msg("failed to create MemoryInputStream for response")
		return
	}

	// Create response object for more control
	schemeResp := webkit.NewURISchemeResponse(&stream.InputStream, int64(len(response.Data)))
	if schemeResp == nil {
		h.logger.Error().Msg("failed to create URISchemeResponse")
		return
	}
	schemeResp.SetContentType(contentType)
	schemeResp.SetStatus(uint(response.StatusCode), nil)

	req.FinishWithResponse(schemeResp)
}

// RegisterWithContext registers the dumb:// scheme with a WebKitContext.
func (h *DumbSchemeHandler) RegisterWithContext(wkCtx *WebKitContext) {
	if wkCtx == nil || wkCtx.Context() == nil {
		h.logger.Error().Msg("cannot register scheme: context is nil")
		return
	}

	callback := webkit.URISchemeRequestCallback(func(reqPtr, userData uintptr) {
		h.HandleRequest(reqPtr)
	})

	wkCtx.Context().RegisterUriScheme("dumb", &callback, 0, nil)

	// Mark scheme as local, secure, and CORS-enabled for proper security policies
	secMgr := wkCtx.Context().GetSecurityManager()
	if secMgr != nil {
		secMgr.RegisterUriSchemeAsLocal("dumb")
		secMgr.RegisterUriSchemeAsSecure("dumb")
		secMgr.RegisterUriSchemeAsCorsEnabled("dumb")
		h.logger.Debug().Msg("dumb:// scheme registered as local, secure, and CORS-enabled")
	}

	h.logger.Info().Msg("dumb:// scheme registered")
}

// Default page templates (fallback when assets not available)

const errorPageHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Error</title>
    <style>
        body {
            font-family: system-ui, -apple-system, sans-serif;
            background: #1a1a2e;
            color: #eee;
            display: flex;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
        }
        .container {
            text-align: center;
        }
        h1 { color: #e74c3c; }
        p { color: #888; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Error</h1>
        <p>The page could not be loaded.</p>
    </div>
</body>
</html>`

const notFoundHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Not Found</title>
    <style>
        body {
            font-family: system-ui, -apple-system, sans-serif;
            background: #1a1a2e;
            color: #eee;
            display: flex;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
        }
        .container {
            text-align: center;
        }
        h1 { color: #f39c12; }
        p { color: #888; }
    </style>
</head>
<body>
    <div class="container">
        <h1>404</h1>
        <p>Page not found.</p>
    </div>
</body>
</html>`

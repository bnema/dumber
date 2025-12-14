package webkit

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/rs/zerolog"
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
	logger   zerolog.Logger
	mu       sync.RWMutex
}

// NewDumbSchemeHandler creates a new handler for the dumb:// scheme.
func NewDumbSchemeHandler(ctx context.Context) *DumbSchemeHandler {
	log := logging.FromContext(ctx)

	h := &DumbSchemeHandler{
		handlers: make(map[string]PageHandler),
		logger:   log.With().Str("component", "scheme-handler").Logger(),
	}

	// Register default pages
	h.registerDefaults()

	return h
}

// registerDefaults sets up default page handlers.
func (h *DumbSchemeHandler) registerDefaults() {
	// New tab page
	h.RegisterPage("/newtab", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		return &SchemeResponse{
			Data:        []byte(newTabHTML),
			ContentType: "text/html",
			StatusCode:  http.StatusOK,
		}
	}))

	// Homepage (alias for newtab)
	h.RegisterPage("/", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		return &SchemeResponse{
			Data:        []byte(newTabHTML),
			ContentType: "text/html",
			StatusCode:  http.StatusOK,
		}
	}))

	// Error page
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

	schemeReq := &SchemeRequest{
		inner:  req,
		URI:    req.GetUri(),
		Path:   req.GetPath(),
		Method: req.GetHttpMethod(),
		Scheme: req.GetScheme(),
	}

	h.logger.Debug().
		Str("uri", schemeReq.URI).
		Str("path", schemeReq.Path).
		Str("method", schemeReq.Method).
		Msg("handling scheme request")

	// Look up handler
	h.mu.RLock()
	handler, ok := h.handlers[schemeReq.Path]
	if !ok {
		// Try without leading slash
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

	// Send response
	h.sendResponse(req, response)
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

	// Create GBytes from response data
	gbytes := glib.NewBytes(response.Data, uint(len(response.Data)))
	if gbytes == nil {
		h.logger.Error().Msg("failed to create GBytes for response")
		return
	}

	// Create MemoryInputStream from GBytes
	stream := gio.NewMemoryInputStreamFromBytes(gbytes)
	if stream == nil {
		h.logger.Error().Msg("failed to create MemoryInputStream for response")
		return
	}

	// Finish the request with the stream
	contentType := response.ContentType
	if contentType == "" {
		contentType = "text/html"
	}

	req.Finish(&stream.InputStream, int64(len(response.Data)), &contentType)
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

	// Mark scheme as local and secure for proper security policies
	secMgr := wkCtx.Context().GetSecurityManager()
	if secMgr != nil {
		secMgr.RegisterUriSchemeAsLocal("dumb")
		secMgr.RegisterUriSchemeAsSecure("dumb")
	}

	h.logger.Info().Msg("dumb:// scheme registered")
}

// Default page templates

const newTabHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>New Tab</title>
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
        h1 {
            font-weight: 300;
            color: #888;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>dumber</h1>
    </div>
</body>
</html>`

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

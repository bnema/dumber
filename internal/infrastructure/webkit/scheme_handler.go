package webkit

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/infrastructure/webutil"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/soup"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/rs/zerolog"
)

// Scheme path constants
const (
	HomePath   = "home"
	ConfigPath = "config"
	WebRTCPath = "webrtc"
	ErrorPath  = "error"
	CrashPath  = "crash"
	IndexHTML  = "index.html"
	httpGET    = "GET"
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
	handlers   map[string]PageHandler
	assets     embed.FS
	assetDir   string // subdirectory within embed.FS (e.g., "assets/webui")
	logger     zerolog.Logger
	mu         sync.RWMutex
	hwSurveyor *env.HardwareSurveyor
	ctx        context.Context
}

// NewDumbSchemeHandler creates a new handler for the dumb:// scheme.
func NewDumbSchemeHandler(ctx context.Context) *DumbSchemeHandler {
	log := logging.FromContext(ctx)

	h := &DumbSchemeHandler{
		handlers:   make(map[string]PageHandler),
		assetDir:   "webui",
		logger:     log.With().Str("component", "scheme-handler").Logger(),
		hwSurveyor: env.NewHardwareSurveyor(),
		ctx:        ctx,
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
	h.RegisterPage("/error", PageHandlerFunc(func(_ *SchemeRequest) *SchemeResponse {
		return &SchemeResponse{
			Data:        []byte(errorPageHTML),
			ContentType: "text/html",
			StatusCode:  http.StatusOK,
		}
	}))

	// Crash page (web process termination fallback)
	h.RegisterPage("/"+CrashPath, PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		if req.Method != "" && req.Method != httpGET {
			return nil
		}
		originalURI := sanitizeCrashPageOriginalURI(crashOriginalURI(req.URI))
		return &SchemeResponse{
			Data:        []byte(buildCrashPageHTML(originalURI)),
			ContentType: "text/html; charset=utf-8",
			StatusCode:  http.StatusOK,
		}
	}))

	// API: Get current config (used by dumb://config)
	h.RegisterPage("/api/config", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		if req.Method != "" && req.Method != httpGET {
			return nil
		}

		return h.buildConfigResponse(config.Get())
	}))

	// API: Get default config (used by Reset Defaults in dumb://config)
	h.RegisterPage("/api/config/default", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		if req.Method != "" && req.Method != httpGET {
			return nil
		}

		return h.buildConfigResponse(config.DefaultConfig())
	}))
}

func crashOriginalURI(requestURI string) string {
	if requestURI == "" {
		return ""
	}
	parsed, err := url.Parse(requestURI)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("url"))
}

func sanitizeCrashPageOriginalURI(originalURI string) string {
	return webutil.SanitizeCrashPageOriginalURI(originalURI)
}

func buildCrashPageHTML(originalURI string) string {
	return webutil.BuildCrashPageHTML(originalURI)
}

func (h *DumbSchemeHandler) buildConfigResponse(cfg *config.Config) *SchemeResponse {
	// Get hardware info for display and profile resolution
	// Use background context since survey results are cached and we don't want
	// request context cancellation to affect this
	var hw *port.HardwareInfo
	if h.hwSurveyor != nil {
		hwInfo := h.hwSurveyor.Survey(context.Background())
		hw = &hwInfo
	}

	resp := config.BuildWebUIConfigPayload(cfg, hw)

	data, err := json.Marshal(resp)
	if err != nil {
		return &SchemeResponse{
			Data:        []byte(fmt.Sprintf(`{"error": %q}`, err)),
			ContentType: "application/json",
			StatusCode:  http.StatusInternalServerError,
		}
	}

	return &SchemeResponse{
		Data:        data,
		ContentType: "application/json",
		StatusCode:  http.StatusOK,
	}
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

	// API endpoints should never be treated as static assets
	if strings.HasPrefix(schemeReq.Path, "/api/") {
		h.mu.RLock()
		handler, ok := h.handlers[schemeReq.Path]
		if !ok {
			handler, ok = h.handlers[strings.TrimPrefix(schemeReq.Path, "/")]
		}
		h.mu.RUnlock()
		if ok {
			response := handler.Handle(schemeReq)
			h.sendResponse(req, response)
			return
		}
	}

	// Try to serve from embedded assets
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

	relPath, ok := resolveAssetPath(u)
	if !ok {
		return nil
	}

	// Read the asset from embedded FS
	fullPath := filepath.ToSlash(filepath.Join(assetDir, relPath))
	data, err := fs.ReadFile(h.assets, fullPath)
	if err != nil {
		h.logger.Debug().Str("path", fullPath).Err(err).Msg("asset not found")
		return nil
	}

	contentType := webutil.GetMimeType(relPath)
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

func resolveAssetPath(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}

	rootByHost := map[string]string{
		HomePath:   IndexHTML,
		ConfigPath: "config.html",
		WebRTCPath: "webrtc.html",
		ErrorPath:  "error.html",
	}

	if root, ok := rootByHost[u.Host]; ok {
		path := strings.TrimPrefix(u.Path, "/")
		if path == "" {
			return root, true
		}
		return path, true
	}

	switch u.Opaque {
	case HomePath:
		return IndexHTML, true
	case ConfigPath:
		return "config.html", true
	case ErrorPath:
		return "error.html", true
	case WebRTCPath:
		return "webrtc.html", true
	default:
		return "", false
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

	// WebKit can treat custom schemes as CORS-relevant even for same-origin fetch().
	// We only add CORS headers for our internal API endpoints.
	if strings.HasPrefix(req.GetPath(), "/api/") {
		hdrs := soup.NewMessageHeaders(soup.MessageHeadersResponseValue)
		hdrs.Append("Access-Control-Allow-Origin", "*")
		hdrs.Append("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		hdrs.Append("Access-Control-Allow-Headers", "Content-Type")
		hdrs.Append("Access-Control-Max-Age", "86400")
		schemeResp.SetHttpHeaders(hdrs)
	}

	req.FinishWithResponse(schemeResp)
}

// RegisterWithContext registers the dumb:// scheme with a WebKitContext.
// The scheme is always registered on the default WebContext to ensure WebViews
// (which use the default WebContext) can load dumb:// URLs.
func (h *DumbSchemeHandler) RegisterWithContext(wkCtx *WebKitContext) {
	if wkCtx == nil || wkCtx.Context() == nil {
		h.logger.Error().Msg("cannot register scheme: context is nil")
		return
	}

	callback := webkit.URISchemeRequestCallback(func(reqPtr, _ uintptr) {
		h.HandleRequest(reqPtr)
	})

	// Always register on the default WebContext since WebViews use it
	defaultCtx := webkit.WebContextGetDefault()
	if defaultCtx != nil {
		defaultCtx.RegisterUriScheme("dumb", &callback, 0, nil)

		// Mark scheme as local, secure, and CORS-enabled for proper security policies
		if secMgr := defaultCtx.GetSecurityManager(); secMgr != nil {
			secMgr.RegisterUriSchemeAsLocal("dumb")
			secMgr.RegisterUriSchemeAsSecure("dumb")
			secMgr.RegisterUriSchemeAsCorsEnabled("dumb")
		}
		h.logger.Debug().Msg("dumb:// scheme registered on default WebContext")
	}

	// Also register on custom WebContext if different from default
	customCtx := wkCtx.Context()
	if customCtx != nil && customCtx.GoPointer() != 0 && (defaultCtx == nil || customCtx.GoPointer() != defaultCtx.GoPointer()) {
		wkCtx.Context().RegisterUriScheme("dumb", &callback, 0, nil)

		if secMgr := customCtx.GetSecurityManager(); secMgr != nil {
			secMgr.RegisterUriSchemeAsLocal("dumb")
			secMgr.RegisterUriSchemeAsSecure("dumb")
			secMgr.RegisterUriSchemeAsCorsEnabled("dumb")
		}
		h.logger.Debug().Msg("dumb:// scheme also registered on custom WebContext")
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

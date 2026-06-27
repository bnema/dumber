package webkit

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/webutil"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/soup"
	"github.com/bnema/puregotk/v4/webkit"
	"github.com/rs/zerolog"
)

// Scheme path constants
const (
	HistoryPath             = "history"
	FavoritesPath           = "favorites"
	ConfigPath              = "config"
	ErrorPath               = "error"
	CrashPath               = "crash"
	IndexHTML               = "index.html"
	httpGET                 = "GET"
	maxSystemviewsWASMBytes = 64 * 1024 * 1024
	systemviewsAssetDir     = "systemviews"
)

// SchemeRequest represents a request to a custom URI scheme.
type SchemeRequest struct {
	inner   *webkit.URISchemeRequest
	URI     string
	Path    string
	Method  string
	Scheme  string
	Origin  string
	Referer string
}

// SchemeResponse represents a response to a scheme request.
type SchemeResponse struct {
	Data                   []byte
	ContentType            string
	StatusCode             int
	Headers                map[string]string
	SuppressDefaultHeaders bool
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
	ctx                  context.Context
	handlers             map[string]PageHandler
	assets               embed.FS
	faviconResolver      port.FaviconSystemviewResolver
	assetDir             string // default subdirectory within embed.FS (e.g., "systemviews")
	logger               zerolog.Logger
	mu                   sync.RWMutex
	currentConfigPayload func() ([]byte, error)
	defaultConfigPayload func() ([]byte, error)
}

// NewDumbSchemeHandler creates a new handler for the dumb:// scheme.
func NewDumbSchemeHandler(ctx context.Context) *DumbSchemeHandler {
	log := logging.FromContext(ctx)

	h := &DumbSchemeHandler{
		ctx:      ctx,
		handlers: make(map[string]PageHandler),
		assetDir: systemviewsAssetDir,
		logger:   log.With().Str("component", "scheme-handler").Logger(),
	}

	// Register default pages
	h.registerDefaults()

	return h
}

// SetAssets sets the embedded filesystem containing systemviews assets.
func (h *DumbSchemeHandler) SetAssets(assets embed.FS) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.assets = assets
	h.logger.Debug().Msg("assets filesystem configured")
}

// SetConfigPayloadBuilders wires the config payload builders used by /api/config.
func (h *DumbSchemeHandler) SetConfigPayloadBuilders(current, defaultPayload func() ([]byte, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.currentConfigPayload = current
	h.defaultConfigPayload = defaultPayload
	h.logger.Debug().Msg("config payload builders configured")
}

func (h *DumbSchemeHandler) SetFaviconResolver(resolver port.FaviconSystemviewResolver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.faviconResolver = resolver
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
		if !isTrustedSystemviewAPIRequest(req) {
			return privateJSONErrorResponse(http.StatusForbidden, "forbidden")
		}

		h.mu.RLock()
		build := h.currentConfigPayload
		h.mu.RUnlock()
		return buildConfigResponse(build)
	}))

	// API: Get default config (used by Reset Defaults in dumb://config)
	h.RegisterPage("/api/config/default", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		if req.Method != "" && req.Method != httpGET {
			return nil
		}
		if !isTrustedSystemviewAPIRequest(req) {
			return privateJSONErrorResponse(http.StatusForbidden, "forbidden")
		}

		h.mu.RLock()
		build := h.defaultConfigPayload
		h.mu.RUnlock()
		return buildConfigResponse(build)
	}))

	h.RegisterPage("/api/favicon", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		if req.Method != "" && req.Method != httpGET {
			return nil
		}
		return h.handleFaviconAPI(req)
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

const systemviewFaviconSize = 32

func (h *DumbSchemeHandler) handleFaviconAPI(req *SchemeRequest) *SchemeResponse {
	if !isTrustedSystemviewFaviconRequest(req) {
		return privateJSONErrorResponse(http.StatusForbidden, "forbidden")
	}
	h.mu.RLock()
	resolver := h.faviconResolver
	h.mu.RUnlock()
	if resolver == nil {
		return privateJSONErrorResponse(http.StatusNotFound, "favicon unavailable")
	}
	if req == nil {
		return privateJSONErrorResponse(http.StatusBadRequest, "invalid request")
	}
	parsed, err := url.Parse(req.URI)
	if err != nil {
		return privateJSONErrorResponse(http.StatusBadRequest, "invalid request URL")
	}
	domain := strings.TrimSpace(parsed.Query().Get("domain"))
	if domain == "" {
		return privateJSONErrorResponse(http.StatusBadRequest, "missing domain")
	}
	size := systemviewFaviconSize
	if rawSize := strings.TrimSpace(parsed.Query().Get("size")); rawSize != "" {
		parsedSize, parseErr := strconv.Atoi(rawSize)
		if parseErr != nil || parsedSize != systemviewFaviconSize {
			return privateJSONErrorResponse(http.StatusBadRequest, "unsupported favicon size")
		}
		size = parsedSize
	}

	resolveCtx := h.ctx
	if resolveCtx == nil {
		resolveCtx = context.Background()
	}
	resolved, err := resolver.ResolveSystemviewIcon(resolveCtx, domain, size)
	if err != nil || resolved == nil || len(resolved.Bytes) == 0 {
		return privateJSONErrorResponse(http.StatusNotFound, "favicon not cached")
	}
	contentType := resolved.ContentType
	if contentType == "" {
		contentType = "image/png"
	}
	return &SchemeResponse{
		Data:                   resolved.Bytes,
		ContentType:            contentType,
		StatusCode:             http.StatusOK,
		Headers:                map[string]string{"Cache-Control": "no-store"},
		SuppressDefaultHeaders: true,
	}
}

func isTrustedSystemviewFaviconRequest(req *SchemeRequest) bool {
	return isTrustedSystemviewAPIRequest(req)
}

func isTrustedSystemviewAPIRequest(req *SchemeRequest) bool {
	if req == nil {
		return false
	}
	origin := strings.TrimSpace(req.Origin)
	if origin != "" && !strings.EqualFold(origin, "null") {
		return isTrustedSystemviewURL(origin)
	}
	return isTrustedSystemviewURL(req.Referer)
}

func isTrustedSystemviewURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, "dumb") {
		return false
	}
	host := parsed.Host
	if host == "" {
		host = parsed.Opaque
	}
	if idx := strings.IndexAny(host, "/?#"); idx >= 0 {
		host = host[:idx]
	}
	switch host {
	case HistoryPath, FavoritesPath, ConfigPath, ErrorPath, CrashPath:
		return true
	default:
		return false
	}
}

func jsonErrorResponse(status int, message string) *SchemeResponse {
	return &SchemeResponse{
		Data:        []byte(fmt.Sprintf(`{"error":%q}`, message)),
		ContentType: "application/json",
		StatusCode:  status,
	}
}

func privateJSONErrorResponse(status int, message string) *SchemeResponse {
	resp := jsonErrorResponse(status, message)
	resp.Headers = map[string]string{"Cache-Control": "no-store"}
	resp.SuppressDefaultHeaders = true
	return resp
}

func trustedPrivateAPIHeaders() map[string]string {
	return map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type",
		"Cache-Control":                "no-store",
	}
}

func buildConfigResponse(build func() ([]byte, error)) *SchemeResponse {
	if build == nil {
		resp := privateJSONErrorResponse(http.StatusInternalServerError, "config payload builder not configured")
		resp.Headers = trustedPrivateAPIHeaders()
		return resp
	}

	data, err := build()
	if err != nil {
		resp := privateJSONErrorResponse(http.StatusInternalServerError, err.Error())
		resp.Headers = trustedPrivateAPIHeaders()
		return resp
	}

	return &SchemeResponse{
		Data:                   data,
		ContentType:            "application/json",
		StatusCode:             http.StatusOK,
		Headers:                trustedPrivateAPIHeaders(),
		SuppressDefaultHeaders: true,
	}
}

// RegisterPage registers a handler for a specific path.
func (h *DumbSchemeHandler) RegisterPage(pagePath string, handler PageHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[pagePath] = handler
	h.logger.Debug().Str("path", pagePath).Msg("registered page handler")
}

// HandleRequest processes a scheme request and sends the response.
func (h *DumbSchemeHandler) HandleRequest(reqPtr uintptr) {
	req := webkit.URISchemeRequestNewFromInternalPtr(reqPtr)
	if req == nil {
		return
	}

	uri := req.GetUri()
	requestHeaders := req.GetHttpHeaders()
	schemeReq := &SchemeRequest{
		inner:  req,
		URI:    uri,
		Path:   req.GetPath(),
		Method: req.GetHttpMethod(),
		Scheme: req.GetScheme(),
	}
	if requestHeaders != nil {
		schemeReq.Origin = strings.TrimSpace(requestHeaders.GetOne("Origin"))
		schemeReq.Referer = strings.TrimSpace(requestHeaders.GetOne("Referer"))
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
	h.mu.RUnlock()

	if !hasAssets {
		return nil
	}

	assetDir, relPath, ok := resolveAssetPath(u)
	if !ok {
		return nil
	}
	if assetDir == "" {
		assetDir = h.assetDir
	}

	fullPath, relPath, ok := safeSystemviewsAssetPath(assetDir, relPath)
	if !ok {
		return nil
	}

	data, err := readAssetWithEncoding(h.assets, fullPath, relPath)
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
		Headers:     nil,
	}
}

func safeSystemviewsAssetPath(assetDir, relPath string) (fullPath, cleanRelPath string, ok bool) {
	assetDir = strings.Trim(assetDir, "/")
	if assetDir != systemviewsAssetDir {
		return "", "", false
	}

	relPath = strings.TrimLeft(relPath, "/")
	if relPath == "" || strings.ContainsRune(relPath, '\x00') {
		return "", "", false
	}

	cleanRelPath = path.Clean(relPath)
	if cleanRelPath == "." || cleanRelPath == ".." || strings.HasPrefix(cleanRelPath, "../") || path.IsAbs(cleanRelPath) {
		return "", "", false
	}

	fullPath = path.Join(assetDir, cleanRelPath)
	if fullPath != assetDir && !strings.HasPrefix(fullPath, assetDir+"/") {
		return "", "", false
	}
	return fullPath, cleanRelPath, true
}

func readAssetWithEncoding(assets fs.FS, fullPath, relPath string) ([]byte, error) {
	var compressedErr error
	if strings.HasSuffix(relPath, ".wasm") {
		if compressed, err := fs.ReadFile(assets, fullPath+".br"); err == nil {
			data, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), maxSystemviewsWASMBytes+1))
			if err != nil {
				compressedErr = err
			} else if len(data) > maxSystemviewsWASMBytes {
				compressedErr = fmt.Errorf("decompressed asset %s exceeds %d bytes", fullPath, maxSystemviewsWASMBytes)
			} else {
				return data, nil
			}
		}
	}
	data, err := fs.ReadFile(assets, fullPath)
	if err == nil {
		return data, nil
	}
	if compressedErr != nil {
		return nil, compressedErr
	}
	return nil, err
}

func resolveAssetPath(u *url.URL) (assetDir, relPath string, ok bool) {
	if u == nil {
		return "", "", false
	}

	rootByHost := map[string]struct {
		assetDir string
		file     string
	}{
		HistoryPath:   {assetDir: systemviewsAssetDir, file: IndexHTML},
		FavoritesPath: {assetDir: systemviewsAssetDir, file: IndexHTML},
		ConfigPath:    {assetDir: systemviewsAssetDir, file: IndexHTML},
		ErrorPath:     {assetDir: systemviewsAssetDir, file: IndexHTML},
		CrashPath:     {assetDir: systemviewsAssetDir, file: IndexHTML},
	}

	if root, ok := rootByHost[u.Host]; ok {
		assetPath := strings.TrimPrefix(u.Path, "/")
		if assetPath == "" {
			return root.assetDir, root.file, true
		}
		return root.assetDir, assetPath, true
	}

	switch u.Opaque {
	case HistoryPath, FavoritesPath, ConfigPath, ErrorPath, CrashPath:
		return systemviewsAssetDir, IndexHTML, true
	default:
		return "", "", false
	}
}

func shouldAddCORSHeaders(requestPath string) bool {
	requestPath = strings.TrimSpace(requestPath)
	return strings.HasSuffix(requestPath, ".wasm")
}

func responseHeadersForPath(requestPath, contentType string) map[string]string {
	if !shouldAddCORSHeaders(requestPath) {
		return nil
	}

	headers := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type",
		"Access-Control-Max-Age":       "86400",
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	return headers
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

	// WebKit can treat custom schemes as CORS-relevant for the wasm runtime asset.
	// Private systemview APIs opt out with SuppressDefaultHeaders and validate callers.
	var headers map[string]string
	if !response.SuppressDefaultHeaders {
		headers = responseHeadersForPath(req.GetPath(), contentType)
	}
	for name, value := range response.Headers {
		if headers == nil {
			headers = map[string]string{}
		}
		headers[name] = value
	}
	if len(headers) > 0 {
		hdrs := soup.NewMessageHeaders(soup.MessageHeadersResponseValue)
		for name, value := range headers {
			hdrs.Append(name, value)
		}
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

package cef

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

// Scheme path constants matching WebKit's naming.
const (
	homePath   = "home"
	configPath = "config"
	webrtcPath = "webrtc"
	errorPath  = "error"
	indexHTML  = "index.html"
)

// dumbSchemeHandler implements purecef.SchemeHandlerFactory for the dumb:// scheme.
type dumbSchemeHandler struct {
	ctx           context.Context
	messageRouter *MessageRouter
	assets        embed.FS
	assetsSet     bool
	assetDir      string
	logger        zerolog.Logger
	mu            sync.RWMutex
}

// newDumbSchemeHandler creates a handler for the dumb:// scheme.
func newDumbSchemeHandler(ctx context.Context, router *MessageRouter) *dumbSchemeHandler {
	log := logging.FromContext(ctx)
	return &dumbSchemeHandler{
		ctx:           ctx,
		messageRouter: router,
		assetDir:      "webui",
		logger:        log.With().Str("component", "scheme-handler").Logger(),
	}
}

// setAssets sets the embedded filesystem containing webui assets.
func (h *dumbSchemeHandler) setAssets(assets embed.FS) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.assets = assets
	h.assetsSet = true
	h.logger.Debug().Msg("assets filesystem configured")
}

// Create implements purecef.SchemeHandlerFactory. Called on CEF IO thread for each
// request to the dumb:// scheme. Returns a ResourceHandler to serve the response.
func (h *dumbSchemeHandler) Create(_ purecef.Browser, _ purecef.Frame, _ string, request purecef.Request) purecef.ResourceHandler {
	reqURL := request.GetURL()
	method := request.GetMethod()

	h.logger.Debug().
		Str("url", reqURL).
		Str("method", method).
		Msg("scheme handler create")

	u, err := url.Parse(reqURL)
	if err != nil {
		h.logger.Error().Err(err).Str("url", reqURL).Msg("invalid URL")
		return h.newErrorResourceHandler(http.StatusBadRequest, "Invalid URL")
	}

	// Route API requests.
	path := u.Path
	if path == "" && u.Opaque != "" {
		// Handle opaque URLs like dumb:home — no path component.
		path = ""
	}

	if strings.HasPrefix(path, "/api/") {
		return h.handleAPI(method, path, request)
	}

	// Serve static assets.
	return h.handleAsset(u)
}

// handleAPI routes API requests to the message router or built-in handlers.
func (h *dumbSchemeHandler) handleAPI(method, path string, request purecef.Request) purecef.ResourceHandler {
	switch {
	case path == "/api/message" && strings.EqualFold(method, "POST"):
		body := readBodyFromHeader(request)
		if body == nil {
			return h.newJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "empty body"})
		}
		resp, err := h.messageRouter.HandleMessage(h.ctx, 0, body)
		if err != nil {
			return h.newJSONResourceHandler(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return h.newRawResourceHandler(http.StatusOK, "application/json", resp)

	case path == "/api/config" && (method == "" || strings.EqualFold(method, "GET")):
		return h.handleConfigAPI(config.Get())

	case path == "/api/config/default" && (method == "" || strings.EqualFold(method, "GET")):
		return h.handleConfigAPI(config.DefaultConfig())

	default:
		return h.newJSONResourceHandler(http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleConfigAPI returns a JSON response with the given config.
func (h *dumbSchemeHandler) handleConfigAPI(cfg *config.Config) purecef.ResourceHandler {
	data, err := json.Marshal(cfg)
	if err != nil {
		return h.newJSONResourceHandler(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return h.newRawResourceHandler(http.StatusOK, "application/json", data)
}

// handleAsset serves static files from the embedded filesystem.
func (h *dumbSchemeHandler) handleAsset(u *url.URL) purecef.ResourceHandler {
	h.mu.RLock()
	hasAssets := h.assetsSet
	assetDir := h.assetDir
	h.mu.RUnlock()

	if !hasAssets {
		return h.newErrorResourceHandler(http.StatusInternalServerError, "Assets not configured")
	}

	relPath, ok := resolveAssetPath(u)
	if !ok {
		return h.newErrorResourceHandler(http.StatusNotFound, "Page not found")
	}

	fullPath := filepath.ToSlash(filepath.Join(assetDir, relPath))
	data, err := fs.ReadFile(h.assets, fullPath)
	if err != nil {
		h.logger.Debug().Str("path", fullPath).Err(err).Msg("asset not found")
		return h.newErrorResourceHandler(http.StatusNotFound, "Asset not found")
	}

	contentType := getMimeType(relPath)
	h.logger.Debug().
		Str("path", fullPath).
		Str("content_type", contentType).
		Int("size", len(data)).
		Msg("serving asset")

	return h.newRawResourceHandler(http.StatusOK, contentType, data)
}

// resolveAssetPath maps a dumb:// URL to a relative asset path.
func resolveAssetPath(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}

	rootByHost := map[string]string{
		homePath:   indexHTML,
		configPath: "config.html",
		webrtcPath: "webrtc.html",
		errorPath:  "error.html",
	}

	if root, ok := rootByHost[u.Host]; ok {
		path := strings.TrimPrefix(u.Path, "/")
		if path == "" {
			return root, true
		}
		// Don't serve API paths as assets.
		if strings.HasPrefix(path, "api/") {
			return "", false
		}
		return path, true
	}

	// Handle opaque URLs like dumb:home.
	switch u.Opaque {
	case homePath:
		return indexHTML, true
	case configPath:
		return "config.html", true
	case webrtcPath:
		return "webrtc.html", true
	case errorPath:
		return "error.html", true
	default:
		return "", false
	}
}

// getMimeType determines the MIME type for a given file path.
func getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	mt := mime.TypeByExtension(ext)
	if mt != "" {
		return mt
	}

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

// ---------------------------------------------------------------------------
// ResourceHandler implementation
// ---------------------------------------------------------------------------

// staticResourceHandler serves a fixed byte buffer as a CEF ResourceHandler.
type staticResourceHandler struct {
	data        []byte
	contentType string
	statusCode  int
	offset      int
}

func (h *dumbSchemeHandler) newRawResourceHandler(status int, contentType string, data []byte) purecef.ResourceHandler {
	return purecef.NewResourceHandler(&staticResourceHandler{
		data:        data,
		contentType: contentType,
		statusCode:  status,
	})
}

func (h *dumbSchemeHandler) newErrorResourceHandler(status int, msg string) purecef.ResourceHandler {
	escaped := strings.ReplaceAll(strings.ReplaceAll(msg, "&", "&amp;"), "<", "&lt;")
	body := fmt.Sprintf(`<!DOCTYPE html><html><body><h1>%d</h1><p>%s</p></body></html>`, status, escaped)
	return h.newRawResourceHandler(status, "text/html; charset=utf-8", []byte(body))
}

func (h *dumbSchemeHandler) newJSONResourceHandler(status int, v any) purecef.ResourceHandler {
	data, err := json.Marshal(v)
	if err != nil {
		return h.newErrorResourceHandler(http.StatusInternalServerError, "JSON encoding failed")
	}
	return h.newRawResourceHandler(status, "application/json", data)
}

// Open handles the request immediately (synchronous).
func (rh *staticResourceHandler) Open(_ purecef.Request, handleRequest unsafe.Pointer, _ purecef.Callback) int32 {
	// Set handleRequest = true (1) to indicate we handle it immediately.
	*(*int32)(handleRequest) = 1
	return 1
}

// ProcessRequest is deprecated; Open is used instead.
func (rh *staticResourceHandler) ProcessRequest(_ purecef.Request, _ purecef.Callback) int32 {
	return 0
}

// GetResponseHeaders sets status code, MIME type, and content length.
func (rh *staticResourceHandler) GetResponseHeaders(response purecef.Response, responseLength unsafe.Pointer, _ uintptr) {
	response.SetStatus(int32(rh.statusCode))
	response.SetMimeType(rh.contentType)
	*(*int64)(responseLength) = int64(len(rh.data))
}

// Skip is not used for static content.
func (rh *staticResourceHandler) Skip(_ int64, _ unsafe.Pointer, _ purecef.ResourceSkipCallback) int32 {
	return 0
}

// Read copies data into the output buffer.
func (rh *staticResourceHandler) Read(
	dataOut unsafe.Pointer, bytesToRead int32,
	bytesRead unsafe.Pointer, _ purecef.ResourceReadCallback,
) int32 {
	if rh.offset >= len(rh.data) {
		return 0
	}

	remaining := len(rh.data) - rh.offset
	toRead := int(bytesToRead)
	if toRead > remaining {
		toRead = remaining
	}

	// Copy data to the output buffer.
	dst := unsafe.Slice((*byte)(dataOut), toRead)
	copy(dst, rh.data[rh.offset:rh.offset+toRead])
	rh.offset += toRead

	*(*int32)(bytesRead) = int32(toRead)
	return 1
}

// ReadResponse is deprecated; Read is used instead.
func (rh *staticResourceHandler) ReadResponse(_ unsafe.Pointer, _ int32, _ unsafe.Pointer, _ purecef.Callback) int32 {
	return 0
}

// Cancel is a no-op for static content.
func (rh *staticResourceHandler) Cancel() {}

// ---------------------------------------------------------------------------
// Request body reader
// ---------------------------------------------------------------------------

// readBodyFromHeader extracts the message body from the X-Dumber-Body header.
// The JS bridge base64-encodes the JSON body into this header to avoid the
// purego-cef PostData element wrapping limitation (wrapPostDataElement is unexported).
func readBodyFromHeader(request purecef.Request) []byte {
	encoded := request.GetHeaderByName("X-Dumber-Body")
	if encoded == "" {
		return nil
	}
	body, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	return body
}

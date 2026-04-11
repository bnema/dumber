package cef

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/webutil"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

// Scheme path constants matching WebKit's naming.
const (
	homePath                    = "home"
	configPath                  = "config"
	webrtcPath                  = "webrtc"
	errorPath                   = "error"
	indexHTML                   = "index.html"
	maxSchemeTruncatedURLLength = 240
)

// pageRootFiles maps internal page hosts/paths to their HTML entry points.
var pageRootFiles = map[string]string{
	homePath:   indexHTML,
	configPath: "config.html",
	webrtcPath: "webrtc.html",
	errorPath:  "error.html",
}

// dumbSchemeHandler serves both the conceptual dumb:// URLs and the actual
// internal https://dumber.invalid origin used by CEF.
type dumbSchemeHandler struct {
	ctx                  context.Context
	messageRouter        *MessageRouter
	transcoder           port.MediaTranscoder
	assets               embed.FS
	assetsSet            bool
	assetDir             string
	logger               zerolog.Logger
	currentConfigPayload func() ([]byte, error)
	defaultConfigPayload func() ([]byte, error)
	mu                   sync.RWMutex

	// onClipboardSet is called when JS sends copied text via /api/clipboard-set.
	// Set by the engine to write to the GDK system clipboard.
	onClipboardSet func(text string)
}

// newDumbSchemeHandler creates a handler for internal CEF pages.
func newDumbSchemeHandler(
	ctx context.Context,
	router *MessageRouter,
	transcoder port.MediaTranscoder,
	currentConfigPayload func() ([]byte, error),
	defaultConfigPayload func() ([]byte, error),
) *dumbSchemeHandler {
	log := logging.FromContext(ctx)
	return &dumbSchemeHandler{
		ctx:                  ctx,
		messageRouter:        router,
		transcoder:           transcoder,
		assetDir:             "webui",
		logger:               log.With().Str("component", "scheme-handler").Logger(),
		currentConfigPayload: currentConfigPayload,
		defaultConfigPayload: defaultConfigPayload,
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

// Create implements purecef.SchemeHandlerFactory. Called on the CEF IO thread
// for requests to either dumb:// or the internal https origin.
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

	if isCEFCrashPageURL(u) {
		originalURI := sanitizeCEFCrashPageOriginalURI(strings.TrimSpace(u.Query().Get("url")))
		return h.newRawResourceHandler(http.StatusOK, "text/html; charset=utf-8", []byte(buildCEFCrashPageHTML(originalURI)))
	}

	// Keep dumb:// as the app-level abstraction, but redirect CEF loads to a
	// normal HTTPS origin so Chromium applies the standard web security model.
	if isConceptualInternalURL(reqURL) {
		targetURL := toActualInternalURL(reqURL)
		if targetURL != "" && targetURL != reqURL {
			return h.newRedirectResourceHandler(http.StatusTemporaryRedirect, targetURL)
		}
	}

	// Route API requests.
	path := u.Path

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
		return h.handleConfigAPI(h.currentConfigPayload)

	case path == "/api/config/default" && (method == "" || strings.EqualFold(method, "GET")):
		return h.handleConfigAPI(h.defaultConfigPayload)

	case path == "/api/transcode" && (method == "" || strings.EqualFold(method, "GET")):
		return h.handleTranscodeAPI(request)

	case path == "/api/clipboard-set" && strings.EqualFold(method, "POST"):
		return h.handleClipboardSet(request)

	default:
		return h.newJSONResourceHandler(http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (h *dumbSchemeHandler) handleTranscodeAPI(request purecef.Request) purecef.ResourceHandler {
	if h.transcoder == nil {
		return h.newJSONResourceHandler(http.StatusServiceUnavailable, map[string]string{"error": "transcoder unavailable"})
	}

	reqURL := request.GetURL()
	parsed, err := url.Parse(reqURL)
	if err != nil {
		return h.newJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "invalid request URL"})
	}

	sourceURL := strings.TrimSpace(parsed.Query().Get("src"))
	if sourceURL == "" {
		return h.newJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "missing src"})
	}

	sourceParsed, err := url.Parse(sourceURL)
	if err != nil || sourceParsed.Host == "" || (sourceParsed.Scheme != "http" && sourceParsed.Scheme != "https") {
		return h.newJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "invalid src"})
	}

	headers := make(map[string]string)
	if userAgent := request.GetHeaderByName("User-Agent"); userAgent != "" {
		headers["User-Agent"] = userAgent
	}
	if referer := strings.TrimSpace(parsed.Query().Get("referer")); referer != "" {
		headers["Referer"] = referer
	} else if referer := request.GetReferrerURL(); referer != "" {
		headers["Referer"] = referer
	}
	if origin := strings.TrimSpace(parsed.Query().Get("origin")); origin != "" {
		headers["Origin"] = origin
	}

	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Minute)
	h.logger.Info().
		Str("source_url", logging.TruncateURL(sourceURL, maxSchemeTruncatedURLLength)).
		Int("forwarded_header_count", len(headers)).
		Msg("scheme handler returning transcoding stream")

	return purecef.NewResourceHandler(&transcodingResourceHandler{
		transcoder: h.transcoder,
		sourceURL:  sourceURL,
		headers:    headers,
		ctx:        ctx,
		cancel:     cancel,
		logf: func() zerolog.Logger {
			return h.logger.With().Str("component", "scheme-transcoding").Logger()
		},
	})
}

func resolveConfigPayload(build func() ([]byte, error)) ([]byte, error) {
	if build == nil {
		return nil, fmt.Errorf("config payload builder not configured")
	}
	return build()
}

// handleConfigAPI returns a JSON response with the given config payload builder.
func (h *dumbSchemeHandler) handleConfigAPI(build func() ([]byte, error)) purecef.ResourceHandler {
	data, err := resolveConfigPayload(build)
	if err != nil {
		return h.newJSONResourceHandler(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return h.newRawResourceHandler(http.StatusOK, "application/json", data)
}

// handleClipboardSet receives copied text from JS copy/cut events and writes
// it to the system clipboard via the engine callback.
const maxClipboardBytes = 10 << 20 // 10 MB

func (h *dumbSchemeHandler) handleClipboardSet(request purecef.Request) purecef.ResourceHandler {
	h.logger.Debug().Msg("cef: /api/clipboard-set request received")

	body := readBodyFromHeader(request)
	if body == nil {
		h.logger.Debug().Msg("cef: clipboard-set — empty body (no X-Dumber-Body header)")
		return h.newJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "empty body"})
	}
	if len(body) > maxClipboardBytes {
		h.logger.Warn().Int("body_len", len(body)).Msg("cef: clipboard-set — payload too large")
		return h.newJSONResourceHandler(http.StatusRequestEntityTooLarge, map[string]string{"error": "payload too large"})
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Text == "" {
		h.logger.Debug().Int("body_len", len(body)).Msg("cef: clipboard-set — invalid or empty payload")
		return h.newJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "invalid payload"})
	}

	h.logger.Debug().Int("text_len", len(payload.Text)).Msg("cef: clipboard-set — forwarding to GDK clipboard")

	if h.onClipboardSet != nil {
		h.onClipboardSet(payload.Text)
	} else {
		h.logger.Warn().Msg("cef: clipboard-set — onClipboardSet callback not wired")
	}

	return h.newJSONResourceHandler(http.StatusOK, map[string]any{"ok": true})
}

func (h *dumbSchemeHandler) newRedirectResourceHandler(status int, location string) purecef.ResourceHandler {
	return purecef.NewResourceHandler(&staticResourceHandler{
		contentType: "text/plain; charset=utf-8",
		statusCode:  status,
		headers: map[string]string{
			"Location":      location,
			"Cache-Control": "no-store",
		},
	})
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

// resolveAssetPath maps either a dumb:// URL or the actual internal HTTPS URL
// to a relative asset path inside assets/webui.
func resolveAssetPath(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}

	if strings.EqualFold(u.Scheme, actualInternalScheme) && strings.EqualFold(u.Host, actualInternalHost) {
		return resolveActualAssetPath(u)
	}

	return resolveConceptualAssetPath(u)
}

func resolveConceptualAssetPath(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}

	if root, ok := pageRootFiles[u.Host]; ok {
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
	if root, ok := pageRootFiles[u.Opaque]; ok {
		return root, true
	}
	return "", false
}

func resolveActualAssetPath(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}

	path := strings.Trim(u.Path, "/")
	if path == "" {
		return "", false
	}

	if root, ok := pageRootFiles[path]; ok {
		return root, true
	}

	if strings.HasPrefix(path, "api/") {
		return "", false
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 2 && isInternalPageHost(parts[0]) {
		if parts[1] == "crash" && parts[0] == homePath {
			return "", false
		}
	}

	// Serve root assets like homepage.min.js, style.css and favicon.png.
	return path, true
}

func isCEFCrashPageURL(u *url.URL) bool {
	if u == nil {
		return false
	}

	switch {
	case strings.EqualFold(u.Scheme, actualInternalScheme) && strings.EqualFold(u.Host, actualInternalHost):
		return strings.Trim(u.Path, "/") == homePath+"/crash"
	case strings.EqualFold(u.Scheme, "dumb") && u.Host == homePath:
		return strings.Trim(u.Path, "/") == "crash"
	default:
		return false
	}
}

func sanitizeCEFCrashPageOriginalURI(originalURI string) string {
	return webutil.SanitizeCrashPageOriginalURI(originalURI)
}

func buildCEFCrashPageHTML(originalURI string) string {
	return webutil.BuildCrashPageHTML(originalURI)
}

func getMimeType(filename string) string {
	return webutil.GetMimeType(filename)
}

// splitMimeCharset splits "text/html; charset=utf-8" into ("text/html", "utf-8").
// If no charset parameter is present, charset is empty.
func splitMimeCharset(contentType string) (mimeType, charset string) {
	parts := strings.SplitN(contentType, ";", 2)
	mimeType = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		param := strings.TrimSpace(parts[1])
		if strings.HasPrefix(strings.ToLower(param), "charset=") {
			charset = strings.TrimSpace(param[len("charset="):])
		}
	}
	return
}

// ---------------------------------------------------------------------------
// ResourceHandler implementation
// ---------------------------------------------------------------------------

// staticResourceHandler serves a fixed byte buffer as a CEF ResourceHandler.
type staticResourceHandler struct {
	data        []byte
	contentType string
	statusCode  int
	headers     map[string]string
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
	escaped := html.EscapeString(msg)
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
// CEF's SetMimeType expects the MIME type without charset parameters;
// the charset must be set separately via SetCharset.
func (rh *staticResourceHandler) GetResponseHeaders(response purecef.Response, responseLength unsafe.Pointer, _ uintptr) {
	response.SetStatus(int32(rh.statusCode))
	if text := http.StatusText(rh.statusCode); text != "" {
		response.SetStatusText(text)
	}
	mimeType, charset := splitMimeCharset(rh.contentType)
	response.SetMimeType(mimeType)
	if charset != "" {
		response.SetCharset(charset)
	}
	for name, value := range rh.headers {
		response.SetHeaderByName(name, value, 1)
	}
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

package cef

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/env"
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

// dumbSchemeHandler serves both the conceptual dumb:// URLs and the actual
// internal https://dumber.invalid origin used by CEF.
type dumbSchemeHandler struct {
	ctx           context.Context
	messageRouter *MessageRouter
	transcoder    port.MediaTranscoder
	assets        embed.FS
	assetsSet     bool
	assetDir      string
	logger        zerolog.Logger
	hwSurveyor    *env.HardwareSurveyor
	mu            sync.RWMutex

	// onClipboardSet is called when JS sends copied text via /api/clipboard-set.
	// Set by the engine to write to the GDK system clipboard.
	onClipboardSet func(text string)
}

// newDumbSchemeHandler creates a handler for internal CEF pages.
func newDumbSchemeHandler(ctx context.Context, router *MessageRouter, transcoder port.MediaTranscoder) *dumbSchemeHandler {
	log := logging.FromContext(ctx)
	return &dumbSchemeHandler{
		ctx:           ctx,
		messageRouter: router,
		transcoder:    transcoder,
		assetDir:      "webui",
		logger:        log.With().Str("component", "scheme-handler").Logger(),
		hwSurveyor:    env.NewHardwareSurveyor(),
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
		return h.handleConfigAPI(config.Get())

	case path == "/api/config/default" && (method == "" || strings.EqualFold(method, "GET")):
		return h.handleConfigAPI(config.DefaultConfig())

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
		Str("source_url", logging.TruncateURL(sourceURL, 240)).
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

// handleConfigAPI returns a JSON response with the given config.
func (h *dumbSchemeHandler) handleConfigAPI(cfg *config.Config) purecef.ResourceHandler {
	var hwInfo *port.HardwareInfo
	if h.hwSurveyor != nil {
		survey := h.hwSurveyor.Survey(context.Background())
		hwInfo = &survey
	}
	data, err := json.Marshal(config.BuildWebUIConfigPayload(cfg, hwInfo))
	if err != nil {
		return h.newJSONResourceHandler(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return h.newRawResourceHandler(http.StatusOK, "application/json", data)
}

// handleClipboardSet receives copied text from JS copy/cut events and writes
// it to the system clipboard via the engine callback.
func (h *dumbSchemeHandler) handleClipboardSet(request purecef.Request) purecef.ResourceHandler {
	body := readBodyFromHeader(request)
	if body == nil {
		return h.newJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "empty body"})
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Text == "" {
		return h.newJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "invalid payload"})
	}

	if h.onClipboardSet != nil {
		h.onClipboardSet(payload.Text)
	}

	return h.newJSONResourceHandler(http.StatusOK, map[string]string{"ok": "true"})
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

func resolveActualAssetPath(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}

	path := strings.Trim(u.Path, "/")
	if path == "" {
		return "", false
	}

	rootByPath := map[string]string{
		homePath:   indexHTML,
		configPath: "config.html",
		webrtcPath: "webrtc.html",
		errorPath:  "error.html",
	}
	if root, ok := rootByPath[path]; ok {
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
	if originalURI == "" {
		return ""
	}
	parsed, err := url.Parse(originalURI)
	if err != nil {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return ""
		}
		return parsed.String()
	case "dumb":
		if parsed.Host == "" && parsed.Opaque == "" {
			return ""
		}
		return parsed.String()
	default:
		return ""
	}
}

func buildCEFCrashPageHTML(originalURI string) string {
	escapedURI := html.EscapeString(originalURI)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Renderer crashed</title>
    <style>
        :root { color-scheme: dark; font-family: "IBM Plex Sans", "Segoe UI", sans-serif; }
        body {
            margin: 0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            background: radial-gradient(circle at top, #253447, #101622 55%%);
            color: #f2f6fa;
            padding: 24px;
        }
        .card {
            width: min(640px, 100%%);
            background: rgba(10, 16, 26, 0.86);
            border: 1px solid rgba(144, 173, 205, 0.35);
            border-radius: 16px;
            box-shadow: 0 24px 64px rgba(0, 0, 0, 0.45);
            padding: 28px;
        }
        .url {
            margin: 16px 0 20px;
            padding: 12px;
            border-radius: 10px;
            background: rgba(26, 38, 56, 0.85);
            border: 1px solid rgba(139, 167, 194, 0.28);
            font-family: "IBM Plex Mono", "Fira Code", monospace;
            overflow-wrap: anywhere;
        }
        .actions { display: flex; gap: 12px; flex-wrap: wrap; }
        button {
            border: 0;
            border-radius: 10px;
            padding: 10px 16px;
            cursor: pointer;
            font-size: 0.95rem;
            font-weight: 600;
        }
        .primary { background: #4dd0e1; color: #061018; }
        .secondary { background: #233346; color: #d6e5f5; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Renderer process ended</h1>
        <p>The current page was interrupted. You can reload it to continue browsing.</p>
        <div class="url">%s</div>
        <div class="actions">
            <button class="primary" id="reload-btn" data-target="%s">Reload page</button>
            <button class="secondary" id="stay-btn">Stay on this page</button>
        </div>
    </div>
    <script>
        const reloadButton = document.getElementById('reload-btn');
        const targetUrl = (reloadButton.getAttribute('data-target') || '').trim();
        reloadButton.addEventListener('click', function() {
            if (targetUrl) {
                window.location.href = targetUrl;
                return;
            }
            window.location.reload();
        });
        document.getElementById('stay-btn').addEventListener('click', function() {
            this.disabled = true;
            this.textContent = 'Staying on page';
        });
    </script>
</body>
</html>`, escapedURI, escapedURI)
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

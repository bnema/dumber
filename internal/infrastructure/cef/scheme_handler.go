package cef

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/andybalholm/brotli"
	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/webutil"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

// Scheme path constants matching WebKit's naming.
const (
	historyPath                 = "history"
	favoritesPath               = "favorites"
	configPath                  = "config"
	errorPath                   = "error"
	indexHTML                   = "index.html"
	maxSchemeTruncatedURLLength = 240
	maxSystemviewsWASMBytes     = 64 * 1024 * 1024
	systemviewsAssetDir         = "systemviews"
)

// pageRootFiles maps internal page hosts/paths to their HTML entry points.
var pageRootFiles = map[string]string{
	historyPath:   indexHTML,
	favoritesPath: indexHTML,
	configPath:    indexHTML,
	errorPath:     indexHTML,
}

var cefNewResourceHandler = purecef.NewResourceHandler

// dumbSchemeHandler serves both the conceptual dumb:// URLs and the actual
// internal https://dumber.invalid origin used by CEF.
type dumbSchemeHandler struct {
	ctx                  context.Context
	messageRouter        *MessageRouter
	transcoder           port.MediaTranscoder
	faviconService       port.FaviconService
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

	// onEditableFocus is called when JS reports that an editable element gained
	// DOM focus. The engine uses this to reassert CEF browser focus in OSR mode.
	onEditableFocus func(browser purecef.Browser)

	// onPopupOpen/onPopupNavigate/onPopupClose bridge synthetic window.open()
	// proxies from page JavaScript back into the browser process when native
	// related popups are not available (e.g. CEF OSR regular-webview fallback path).
	onPopupOpen              func(browser purecef.Browser, payload rendererBridgePopupOpenPayload)
	onPopupNavigate          func(browser purecef.Browser, payload rendererBridgePopupNavigatePayload)
	onPopupClose             func(browser purecef.Browser, payload rendererBridgePopupClosePayload)
	onPopupOpenerNavigate    func(browser purecef.Browser, payload popupOpenerNavigatePayload)
	onPopupOpenerPostMessage func(browser purecef.Browser, payload popupOpenerPostMessagePayload)

	// bridgeNonceValidator checks whether a bridge nonce belongs to the active
	// browser/navigation context that issued the request.
	bridgeNonceValidator func(browser purecef.Browser, bridgeNonce string) bool
}

// newDumbSchemeHandler creates a handler for internal CEF pages.
func newDumbSchemeHandler(
	ctx context.Context,
	router *MessageRouter,
	transcoder port.MediaTranscoder,
	currentConfigPayload func() ([]byte, error),
	defaultConfigPayload func() ([]byte, error),
) (*dumbSchemeHandler, error) {
	if currentConfigPayload == nil {
		return nil, fmt.Errorf("current config payload builder not configured")
	}
	if defaultConfigPayload == nil {
		return nil, fmt.Errorf("default config payload builder not configured")
	}

	log := logging.FromContext(ctx)
	return &dumbSchemeHandler{
		ctx:                  ctx,
		messageRouter:        router,
		transcoder:           transcoder,
		assetDir:             systemviewsAssetDir,
		logger:               log.With().Str("component", "scheme-handler").Logger(),
		currentConfigPayload: currentConfigPayload,
		defaultConfigPayload: defaultConfigPayload,
	}, nil
}

// setAssets sets the embedded filesystem containing systemviews assets.
func (h *dumbSchemeHandler) setAssets(assets embed.FS) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.assets = assets
	h.assetsSet = true
	h.logger.Debug().Msg("assets filesystem configured")
}

func (h *dumbSchemeHandler) setFaviconService(service port.FaviconService) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.faviconService = service
}

// Create implements purecef.SchemeHandlerFactory. Called on the CEF IO thread
// for requests to either dumb:// or the internal https origin.
func (h *dumbSchemeHandler) Create(browser purecef.Browser, _ purecef.Frame, _ string, request purecef.Request) purecef.ResourceHandler {
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

	// Route API requests before redirecting conceptual dumb:// URLs to the
	// internal HTTPS origin. External pages use dumb://api/... for clipboard,
	// focus, and popup bridges specifically to bypass page CSP; redirecting
	// those requests to https://dumber.invalid makes site connect-src policies
	// block them.
	if apiPath, ok := resolveAPIPath(u); ok {
		return h.handleAPI(browser, method, apiPath, request)
	}

	// Keep dumb:// as the app-level abstraction, but redirect CEF page/asset
	// loads to a normal HTTPS origin so Chromium applies the standard web
	// security model.
	if isConceptualInternalURL(reqURL) {
		targetURL := toActualInternalURL(reqURL)
		if targetURL != "" && targetURL != reqURL {
			return h.newRedirectResourceHandler(http.StatusTemporaryRedirect, targetURL)
		}
	}

	// Serve static assets.
	return h.handleAsset(u)
}

// handleAPI routes API requests to the message router or built-in handlers.
func (h *dumbSchemeHandler) handleAPI(
	browser purecef.Browser,
	method string,
	requestPath string,
	request purecef.Request,
) purecef.ResourceHandler {
	if strings.EqualFold(method, http.MethodOptions) {
		if requestPath == "/api/config" || requestPath == "/api/config/default" {
			if denied := h.rejectUntrustedConfigRequester(request); denied != nil {
				return denied
			}
			return h.newPrivateAPIRawResourceHandler(http.StatusNoContent, "text/plain; charset=utf-8", nil)
		}
		return h.newAPIRawResourceHandler(http.StatusNoContent, "text/plain; charset=utf-8", nil)
	}
	if isAPIGetMethod(method) {
		if handler, ok := h.handleAPIGet(requestPath, request); ok {
			return handler
		}
	}
	if isAPIPostMethod(method) {
		if handler, ok := h.handleAPIPost(browser, requestPath, request); ok {
			return handler
		}
	}
	return h.newAPIJSONResourceHandler(http.StatusNotFound, map[string]string{"error": "not found"})
}

func isAPIGetMethod(method string) bool {
	return method == "" || strings.EqualFold(method, http.MethodGet)
}

func isAPIPostMethod(method string) bool {
	return strings.EqualFold(method, http.MethodPost)
}

func (h *dumbSchemeHandler) handleAPIGet(requestPath string, request purecef.Request) (purecef.ResourceHandler, bool) {
	switch requestPath {
	case "/api/config":
		if denied := h.rejectUntrustedConfigRequester(request); denied != nil {
			return denied, true
		}
		return h.handleConfigAPI(h.currentConfigPayload), true
	case "/api/config/default":
		if denied := h.rejectUntrustedConfigRequester(request); denied != nil {
			return denied, true
		}
		return h.handleConfigAPI(h.defaultConfigPayload), true
	case "/api/favicon":
		return h.handleFaviconAPI(request), true
	default:
		return nil, false
	}
}

func (h *dumbSchemeHandler) handleAPIPost(
	browser purecef.Browser,
	requestPath string,
	request purecef.Request,
) (purecef.ResourceHandler, bool) {
	switch requestPath {
	case "/api/message":
		return h.handleMessageAPI(request), true
	case "/api/clipboard-set":
		return h.handleClipboardSet(request, browser), true
	case "/api/focus-sync":
		return h.handleFocusSync(request, browser), true
	case "/api/popup-open":
		return h.handlePopupOpen(request, browser), true
	case "/api/popup-navigate":
		return h.handlePopupNavigate(request, browser), true
	case "/api/popup-close":
		return h.handlePopupClose(request, browser), true
	case "/api/popup-opener-navigate":
		return h.handlePopupOpenerNavigate(request, browser), true
	case "/api/popup-opener-post-message":
		return h.handlePopupOpenerPostMessage(request, browser), true
	default:
		return nil, false
	}
}

func (h *dumbSchemeHandler) handleMessageAPI(request purecef.Request) purecef.ResourceHandler {
	if denied := h.rejectUntrustedSystemviewRequester(request); denied != nil {
		return denied
	}
	body := readBodyFromHeader(request)
	if body == nil {
		return h.newAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "empty body"})
	}
	resp, err := h.messageRouter.HandleMessage(h.ctx, 0, body)
	if err != nil {
		return h.newAPIJSONResourceHandler(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return h.newAPIRawResourceHandler(http.StatusOK, "application/json", resp)
}

func (h *dumbSchemeHandler) rejectForbiddenAPIOrigin(request purecef.Request) purecef.ResourceHandler {
	origin := strings.TrimSpace(request.GetHeaderByName("Origin"))
	if origin == "" || strings.EqualFold(origin, actualInternalOrigin) {
		return nil
	}
	return h.newAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden origin"})
}

func (h *dumbSchemeHandler) rejectUntrustedConfigRequester(request purecef.Request) purecef.ResourceHandler {
	return h.rejectUntrustedSystemviewRequester(request)
}

func (h *dumbSchemeHandler) rejectUntrustedSystemviewRequester(request purecef.Request) purecef.ResourceHandler {
	if request == nil {
		return h.newPrivateAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}
	origin := strings.TrimSpace(request.GetHeaderByName("Origin"))
	if origin != "" {
		if isTrustedSystemviewURL(origin) {
			return nil
		}
		return h.newPrivateAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}
	referrer := strings.TrimSpace(request.GetReferrerURL())
	if referrer == "" {
		referrer = strings.TrimSpace(request.GetHeaderByName("Referer"))
	}
	if !isTrustedSystemviewURL(referrer) {
		return h.newPrivateAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}
	return nil
}

const systemviewFaviconSize = 32

func (h *dumbSchemeHandler) handleFaviconAPI(request purecef.Request) purecef.ResourceHandler {
	if denied := h.rejectUntrustedFaviconRequester(request); denied != nil {
		return denied
	}

	h.mu.RLock()
	service := h.faviconService
	h.mu.RUnlock()
	if service == nil {
		return h.newPrivateAPIJSONResourceHandler(http.StatusNotFound, map[string]string{"error": "favicon unavailable"})
	}

	parsed, err := url.Parse(request.GetURL())
	if err != nil {
		return h.newPrivateAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "invalid request URL"})
	}
	domain := domainurl.CanonicalDomain(parsed.Query().Get("domain"))
	if domain == "" {
		return h.newPrivateAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "missing domain"})
	}
	size := systemviewFaviconSize
	if rawSize := strings.TrimSpace(parsed.Query().Get("size")); rawSize != "" {
		parsedSize, parseErr := strconv.Atoi(rawSize)
		if parseErr != nil || parsedSize != systemviewFaviconSize {
			return h.newPrivateAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "unsupported favicon size"})
		}
		size = parsedSize
	}

	return newFaviconResourceHandler(service, domain, size)
}

func (h *dumbSchemeHandler) rejectUntrustedFaviconRequester(request purecef.Request) purecef.ResourceHandler {
	return h.rejectUntrustedSystemviewRequester(request)
}

func isTrustedSystemviewURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" {
		return false
	}
	if strings.EqualFold(parsed.Scheme, actualInternalScheme) {
		return strings.EqualFold(parsed.Host, actualInternalHost)
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
	return isInternalPageHost(host)
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
		return h.newPrivateAPIJSONResourceHandler(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return h.newPrivateAPIRawResourceHandler(http.StatusOK, "application/json", data)
}

// handleClipboardSet receives copied text from JS copy/cut events and writes
// it to the system clipboard via the engine callback.
const (
	maxClipboardBytes            = 10 << 20 // 10 MB
	dumberBodyHeaderName         = "X-Dumber-Body"
	dumberBridgeActionHeaderName = "X-Dumber-Bridge-Action"
	dumberBridgeNonceHeaderName  = "X-Dumber-Bridge-Nonce"
	dumberBridgeActionFocusSync  = "focus-sync"
)

func (h *dumbSchemeHandler) handleClipboardSet(request purecef.Request, browser purecef.Browser) purecef.ResourceHandler {
	h.logger.Debug().Msg("cef: /api/clipboard-set request received")

	if !h.hasTrustedBridgeNonce(request, browser) {
		h.logger.Warn().Msg("cef: clipboard-set — rejected request without valid bridge nonce")
		return h.newAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}

	body := readBodyFromHeader(request)
	if body == nil {
		h.logger.Debug().Msg("cef: clipboard-set — empty body (no X-Dumber-Body header)")
		return h.newAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "empty body"})
	}
	if len(body) > maxClipboardBytes {
		h.logger.Warn().Int("body_len", len(body)).Msg("cef: clipboard-set — payload too large")
		return h.newAPIJSONResourceHandler(http.StatusRequestEntityTooLarge, map[string]string{"error": "payload too large"})
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Text == "" {
		h.logger.Debug().Int("body_len", len(body)).Msg("cef: clipboard-set — invalid or empty payload")
		return h.newAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "invalid payload"})
	}

	h.logger.Debug().Int("text_len", len(payload.Text)).Msg("cef: clipboard-set — forwarding to GDK clipboard")

	if h.onClipboardSet != nil {
		h.onClipboardSet(payload.Text)
	} else {
		h.logger.Warn().Msg("cef: clipboard-set — onClipboardSet callback not wired")
	}

	return h.newAPIJSONResourceHandler(http.StatusOK, map[string]any{"ok": true})
}

func (h *dumbSchemeHandler) handleFocusSync(request purecef.Request, browser purecef.Browser) purecef.ResourceHandler {
	bridgeAction := ""
	if request != nil {
		bridgeAction = strings.TrimSpace(request.GetHeaderByName(dumberBridgeActionHeaderName))
	}
	if !strings.EqualFold(bridgeAction, dumberBridgeActionFocusSync) {
		h.logger.Warn().Msg("cef: focus-sync — rejected request without trusted bridge action header")
		return h.newAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}
	if browser == nil {
		h.logger.Debug().Msg("cef: focus-sync — browser unavailable")
		return h.newAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "browser unavailable"})
	}
	if !h.hasTrustedBridgeNonce(request, browser) {
		h.logger.Warn().Msg("cef: focus-sync — rejected request without valid bridge nonce")
		return h.newAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}

	if h.onEditableFocus != nil {
		h.onEditableFocus(browser)
	} else {
		h.logger.Debug().Msg("cef: focus-sync — onEditableFocus callback not wired")
	}

	return h.newAPIJSONResourceHandler(http.StatusOK, map[string]any{"ok": true})
}

func handlePopupBridgeRequest[T any](
	h *dumbSchemeHandler,
	request purecef.Request,
	browser purecef.Browser,
	action string,
	decode func([]byte) (T, error),
	dispatch func(purecef.Browser, T),
) purecef.ResourceHandler {
	if browser == nil {
		h.logger.Debug().Msg("cef: " + action + " — browser unavailable")
		return h.newAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "browser unavailable"})
	}
	if !h.hasTrustedBridgeNonce(request, browser) {
		h.logger.Warn().Msg("cef: " + action + " — rejected request without valid bridge nonce")
		return h.newAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}
	body := readBodyFromHeader(request)
	if body == nil {
		return h.newAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": "empty body"})
	}
	payload, err := decode(body)
	if err != nil {
		return h.newAPIJSONResourceHandler(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if dispatch != nil {
		dispatch(browser, payload)
	} else {
		h.logger.Warn().Msg("cef: " + action + " — callback not wired")
	}
	return h.newAPIJSONResourceHandler(http.StatusOK, map[string]any{"ok": true})
}

func (h *dumbSchemeHandler) handlePopupOpen(request purecef.Request, browser purecef.Browser) purecef.ResourceHandler {
	return handlePopupBridgeRequest(h, request, browser, "popup-open", decodeRendererBridgePopupOpenPayload, h.onPopupOpen)
}

func (h *dumbSchemeHandler) handlePopupNavigate(request purecef.Request, browser purecef.Browser) purecef.ResourceHandler {
	return handlePopupBridgeRequest(h, request, browser, "popup-navigate", decodeRendererBridgePopupNavigatePayload, h.onPopupNavigate)
}

func (h *dumbSchemeHandler) handlePopupClose(request purecef.Request, browser purecef.Browser) purecef.ResourceHandler {
	return handlePopupBridgeRequest(h, request, browser, "popup-close", decodeRendererBridgePopupClosePayload, h.onPopupClose)
}

func decodePopupOpenerNavigatePayload(body []byte) (popupOpenerNavigatePayload, error) {
	var payload popupOpenerNavigatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, err
	}
	payload.URL = strings.TrimSpace(payload.URL)
	if payload.URL == "" {
		return payload, fmt.Errorf("missing url")
	}
	return payload, nil
}

func decodePopupOpenerPostMessagePayload(body []byte) (popupOpenerPostMessagePayload, error) {
	var payload popupOpenerPostMessagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, err
	}
	payload.DataKind = strings.TrimSpace(payload.DataKind)
	payload.TargetOrigin = strings.TrimSpace(payload.TargetOrigin)
	payload.SourceOrigin = strings.TrimSpace(payload.SourceOrigin)
	payload.SourceHref = strings.TrimSpace(payload.SourceHref)
	if payload.TargetOrigin == "" {
		return payload, fmt.Errorf("missing target origin")
	}
	return payload, nil
}

func (h *dumbSchemeHandler) handlePopupOpenerNavigate(request purecef.Request, browser purecef.Browser) purecef.ResourceHandler {
	return handlePopupBridgeRequest(h, request, browser, "popup-opener-navigate", decodePopupOpenerNavigatePayload, h.onPopupOpenerNavigate)
}

func (h *dumbSchemeHandler) handlePopupOpenerPostMessage(request purecef.Request, browser purecef.Browser) purecef.ResourceHandler {
	return handlePopupBridgeRequest(
		h,
		request,
		browser,
		"popup-opener-post-message",
		decodePopupOpenerPostMessagePayload,
		h.onPopupOpenerPostMessage,
	)
}

func (h *dumbSchemeHandler) hasTrustedBridgeNonce(request purecef.Request, browser purecef.Browser) bool {
	if browser == nil || h == nil || h.bridgeNonceValidator == nil || request == nil {
		return false
	}
	bridgeNonce := strings.TrimSpace(request.GetHeaderByName(dumberBridgeNonceHeaderName))
	if bridgeNonce == "" {
		return false
	}
	return h.bridgeNonceValidator(browser, bridgeNonce)
}

func (h *dumbSchemeHandler) newRedirectResourceHandler(status int, location string) purecef.ResourceHandler {
	return newStaticResourceHandler(status, "text/plain; charset=utf-8", nil, map[string]string{
		"Location":      location,
		"Cache-Control": "no-store",
	})
}

// handleAsset serves static files from the embedded filesystem.
func (h *dumbSchemeHandler) handleAsset(u *url.URL) purecef.ResourceHandler {
	h.mu.RLock()
	hasAssets := h.assetsSet
	h.mu.RUnlock()

	if !hasAssets {
		return h.newErrorResourceHandler(http.StatusInternalServerError, "Assets not configured")
	}

	assetDir, relPath, ok := resolveAssetPath(u)
	if !ok {
		return h.newErrorResourceHandler(http.StatusNotFound, "Page not found")
	}
	if assetDir == "" {
		assetDir = h.assetDir
	}

	fullPath, relPath, ok := safeSystemviewsAssetPath(assetDir, relPath)
	if !ok {
		return h.newErrorResourceHandler(http.StatusNotFound, "Asset not found")
	}

	data, err := readAssetWithEncoding(h.assets, fullPath, relPath)
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

	return newStaticResourceHandler(http.StatusOK, contentType, data, nil)
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

func readAssetWithEncoding(assets embed.FS, fullPath, relPath string) ([]byte, error) {
	if strings.HasSuffix(relPath, ".wasm") {
		if compressed, err := fs.ReadFile(assets, fullPath+".br"); err == nil {
			data, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), maxSystemviewsWASMBytes+1))
			if err != nil {
				return nil, err
			}
			if len(data) > maxSystemviewsWASMBytes {
				return nil, fmt.Errorf("decompressed asset %s exceeds %d bytes", fullPath, maxSystemviewsWASMBytes)
			}
			return data, nil
		}
	}
	data, err := fs.ReadFile(assets, fullPath)
	return data, err
}

// resolveAssetPath maps either a dumb:// URL or the actual internal HTTPS URL
// to a relative asset path inside assets/systemviews.
func resolveAssetPath(u *url.URL) (assetDir, relPath string, ok bool) {
	if u == nil {
		return "", "", false
	}

	if strings.EqualFold(u.Scheme, actualInternalScheme) && strings.EqualFold(u.Host, actualInternalHost) {
		return resolveActualAssetPath(u)
	}

	return resolveConceptualAssetPath(u)
}

func resolveConceptualAssetPath(u *url.URL) (assetDir, relPath string, ok bool) {
	if u == nil {
		return "", "", false
	}

	if root, ok := pageRootFiles[u.Host]; ok {
		assetPath := strings.TrimPrefix(u.Path, "/")
		if assetPath == "" {
			return assetDirForPageHost(u.Host), root, true
		}
		// Don't serve API paths as assets.
		if strings.HasPrefix(assetPath, "api/") {
			return "", "", false
		}
		return assetDirForPageHost(u.Host), assetPath, true
	}

	// Handle opaque URLs like dumb:history.
	if root, ok := pageRootFiles[u.Opaque]; ok {
		return assetDirForPageHost(u.Opaque), root, true
	}
	return "", "", false
}

func resolveActualAssetPath(u *url.URL) (assetDir, relPath string, ok bool) {
	if u == nil {
		return "", "", false
	}

	assetPath := strings.Trim(u.Path, "/")
	if assetPath == "" {
		return "", "", false
	}

	parts := strings.SplitN(assetPath, "/", 2)
	page := parts[0]
	if root, ok := pageRootFiles[page]; ok {
		if len(parts) == 1 {
			return assetDirForPageHost(page), root, true
		}
		if page == historyPath && parts[1] == "crash" {
			return "", "", false
		}
		return assetDirForPageHost(page), parts[1], true
	}

	if strings.HasPrefix(assetPath, "api/") {
		return "", "", false
	}

	if !strings.Contains(assetPath, "/") && strings.Contains(assetPath, ".") {
		return systemviewsAssetDir, assetPath, true
	}

	return "", "", false
}

func assetDirForPageHost(host string) string {
	switch host {
	case historyPath, favoritesPath, configPath, errorPath:
		return systemviewsAssetDir
	default:
		return ""
	}
}

func isCEFCrashPageURL(u *url.URL) bool {
	if u == nil {
		return false
	}

	switch {
	case strings.EqualFold(u.Scheme, actualInternalScheme) && strings.EqualFold(u.Host, actualInternalHost):
		return strings.Trim(u.Path, "/") == historyPath+"/crash"
	case strings.EqualFold(u.Scheme, "dumb") && u.Host == historyPath:
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

func apiResponseHeaders(extra map[string]string) map[string]string {
	headers := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": strings.Join([]string{
			"Content-Type",
			"X-Dumber-Body",
			"X-Dumber-Bridge-Action",
			"X-Dumber-Bridge-Nonce",
		}, ", "),
		"Access-Control-Max-Age": "86400",
		"Cache-Control":          "no-store",
	}
	for name, value := range extra {
		headers[name] = value
	}
	return headers
}

func newStaticResourceHandler(status int, contentType string, data []byte, headers map[string]string) purecef.ResourceHandler {
	return cefNewResourceHandler(&staticResourceHandler{
		data:        data,
		contentType: contentType,
		statusCode:  status,
		headers:     headers,
	})
}

func (h *dumbSchemeHandler) newRawResourceHandler(status int, contentType string, data []byte) purecef.ResourceHandler {
	return newStaticResourceHandler(status, contentType, data, nil)
}

func (h *dumbSchemeHandler) newAPIRawResourceHandler(status int, contentType string, data []byte) purecef.ResourceHandler {
	return newStaticResourceHandler(status, contentType, data, apiResponseHeaders(nil))
}

func (h *dumbSchemeHandler) newPrivateAPIRawResourceHandler(status int, contentType string, data []byte) purecef.ResourceHandler {
	return newStaticResourceHandler(status, contentType, data, map[string]string{"Cache-Control": "no-store"})
}

func (h *dumbSchemeHandler) newErrorResourceHandler(status int, msg string) purecef.ResourceHandler {
	escaped := html.EscapeString(msg)
	body := fmt.Sprintf(`<!DOCTYPE html><html><body><h1>%d</h1><p>%s</p></body></html>`, status, escaped)
	return h.newRawResourceHandler(status, "text/html; charset=utf-8", []byte(body))
}

func (h *dumbSchemeHandler) newAPIJSONResourceHandler(status int, v any) purecef.ResourceHandler {
	data, err := json.Marshal(v)
	if err != nil {
		fallback, _ := json.Marshal(map[string]string{"error": "JSON encoding failed"})
		return h.newAPIRawResourceHandler(http.StatusInternalServerError, "application/json", fallback)
	}
	return h.newAPIRawResourceHandler(status, "application/json", data)
}

func (h *dumbSchemeHandler) newPrivateAPIJSONResourceHandler(status int, v any) purecef.ResourceHandler {
	data, err := json.Marshal(v)
	if err != nil {
		fallback, _ := json.Marshal(map[string]string{"error": "JSON encoding failed"})
		return h.newPrivateAPIRawResourceHandler(http.StatusInternalServerError, "application/json", fallback)
	}
	return h.newPrivateAPIRawResourceHandler(status, "application/json", data)
}

func newFaviconResourceHandler(service port.FaviconService, domain string, size int) purecef.ResourceHandler {
	return cefNewResourceHandler(&faviconResourceHandler{
		service: service,
		domain:  domain,
		size:    size,
		done:    make(chan struct{}),
		headers: map[string]string{"Cache-Control": "no-store"},
	})
}

const maxFaviconMemoryCacheEntries = 256

var systemviewFaviconCache = newFaviconMemoryCache(maxFaviconMemoryCacheEntries)

type faviconCacheEntry struct {
	statusCode  int
	contentType string
	data        []byte
}

type faviconMemoryCache struct {
	mu      sync.Mutex
	max     int
	entries map[string]faviconCacheEntry
	order   []string
}

func newFaviconMemoryCache(maxEntries int) *faviconMemoryCache {
	return &faviconMemoryCache{
		max:     maxEntries,
		entries: make(map[string]faviconCacheEntry),
		order:   make([]string, 0, maxEntries),
	}
}

func faviconCacheKey(domain string, size int) string {
	return strings.ToLower(strings.TrimSpace(domain)) + "\x00" + strconv.Itoa(size)
}

func (c *faviconMemoryCache) get(key string) (faviconCacheEntry, bool) {
	if c == nil || key == "" {
		return faviconCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return faviconCacheEntry{}, false
	}
	c.moveToBackLocked(key)
	return entry, true
}

func (c *faviconMemoryCache) put(key string, entry faviconCacheEntry) {
	if c == nil || key == "" || c.max <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; !ok {
		c.order = append(c.order, key)
	} else {
		c.moveToBackLocked(key)
	}
	c.entries[key] = entry
	for len(c.order) > c.max {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

func (c *faviconMemoryCache) moveToBackLocked(key string) {
	for i, candidate := range c.order {
		if candidate != key {
			continue
		}
		copy(c.order[i:], c.order[i+1:])
		c.order[len(c.order)-1] = key
		return
	}
}

type faviconResourceHandler struct {
	service     port.FaviconService
	domain      string
	size        int
	headers     map[string]string
	once        sync.Once
	done        chan struct{}
	data        []byte
	contentType string
	statusCode  int
	offset      int
}

func (rh *faviconResourceHandler) load() {
	defer close(rh.done)
	rh.statusCode = http.StatusNotFound
	rh.contentType = "application/json"
	rh.data = []byte(`{"error":"favicon not cached"}`)
	if rh.service == nil {
		return
	}
	cacheKey := faviconCacheKey(rh.domain, rh.size)
	if entry, ok := systemviewFaviconCache.get(cacheKey); ok {
		rh.statusCode = entry.statusCode
		rh.contentType = entry.contentType
		rh.data = entry.data
		return
	}
	if !rh.service.HasPNGSizedOnDisk(rh.domain, rh.size) {
		return
	}
	diskPath := rh.service.DiskPathPNGSized(rh.domain, rh.size)
	if diskPath == "" {
		return
	}
	data, err := os.ReadFile(diskPath)
	if err != nil || len(data) == 0 {
		return
	}
	rh.statusCode = http.StatusOK
	rh.contentType = "image/png"
	rh.data = data
	systemviewFaviconCache.put(cacheKey, faviconCacheEntry{
		statusCode:  rh.statusCode,
		contentType: rh.contentType,
		data:        data,
	})
}

func (rh *faviconResourceHandler) Open(_ purecef.Request, handleRequest *int32, callback purecef.Callback) int32 {
	if handleRequest != nil {
		*handleRequest = 0
	}
	rh.once.Do(func() {
		go func() {
			rh.load()
			if callback != nil {
				callback.Cont()
			}
		}()
	})
	return 1
}

func (rh *faviconResourceHandler) ProcessRequest(_ purecef.Request, callback purecef.Callback) int32 {
	rh.once.Do(func() {
		go func() {
			rh.load()
			if callback != nil {
				callback.Cont()
			}
		}()
	})
	return 1
}

func (rh *faviconResourceHandler) GetResponseHeaders(response purecef.Response, responseLength *int64, _ uintptr) {
	<-rh.done
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
	if responseLength != nil {
		*responseLength = int64(len(rh.data))
	}
}

func (rh *faviconResourceHandler) Skip(_ int64, _ *int64, _ purecef.ResourceSkipCallback) int32 {
	return 0
}

func (rh *faviconResourceHandler) Read(dataOut unsafe.Pointer, bytesToRead int32, bytesRead *int32, _ purecef.ResourceReadCallback) int32 {
	<-rh.done
	if rh.offset >= len(rh.data) {
		return 0
	}
	remaining := len(rh.data) - rh.offset
	toRead := int(bytesToRead)
	if toRead > remaining {
		toRead = remaining
	}
	dst := unsafe.Slice((*byte)(dataOut), toRead)
	copy(dst, rh.data[rh.offset:rh.offset+toRead])
	rh.offset += toRead
	if bytesRead != nil {
		*bytesRead = int32(toRead)
	}
	return 1
}

func (rh *faviconResourceHandler) ReadResponse(_ unsafe.Pointer, _ int32, _ *int32, _ purecef.Callback) int32 {
	return 0
}

func (rh *faviconResourceHandler) Cancel() {}

// Open handles the request immediately (synchronous).
func (rh *staticResourceHandler) Open(_ purecef.Request, handleRequest *int32, _ purecef.Callback) int32 {
	// Set handleRequest = true (1) to indicate we handle it immediately.
	if handleRequest != nil {
		*handleRequest = 1
	}
	return 1
}

// ProcessRequest is deprecated; Open is used instead.
func (rh *staticResourceHandler) ProcessRequest(_ purecef.Request, _ purecef.Callback) int32 {
	return 0
}

// GetResponseHeaders sets status code, MIME type, and content length.
// CEF's SetMimeType expects the MIME type without charset parameters;
// the charset must be set separately via SetCharset.
func (rh *staticResourceHandler) GetResponseHeaders(response purecef.Response, responseLength *int64, _ uintptr) {
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
	if responseLength != nil {
		*responseLength = int64(len(rh.data))
	}
}

// Skip is not used for static content.
func (rh *staticResourceHandler) Skip(_ int64, _ *int64, _ purecef.ResourceSkipCallback) int32 {
	return 0
}

// Read copies data into the output buffer.
func (rh *staticResourceHandler) Read(
	dataOut unsafe.Pointer, bytesToRead int32,
	bytesRead *int32, _ purecef.ResourceReadCallback,
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

	if bytesRead != nil {
		*bytesRead = int32(toRead)
	}
	return 1
}

// ReadResponse is deprecated; Read is used instead.
func (rh *staticResourceHandler) ReadResponse(_ unsafe.Pointer, _ int32, _ *int32, _ purecef.Callback) int32 {
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
	encoded := request.GetHeaderByName(dumberBodyHeaderName)
	if encoded == "" {
		return nil
	}
	body, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	return body
}

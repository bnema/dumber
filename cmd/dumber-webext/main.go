package main

// #cgo pkg-config: webkitgtk-web-process-extension-6.0 glib-2.0
// #include <webkit/webkit-web-process-extension.h>
// #include <glib.h>
import "C"
import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/webext/api"
	"github.com/bnema/dumber/internal/webext/shared"
	"github.com/diamondburned/gotk4-webkitgtk/pkg/soup/v3"
	"github.com/diamondburned/gotk4-webkitgtk/pkg/webkitwebprocessextension/v6"
	"github.com/diamondburned/gotk4/pkg/core/gextras"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// Global state for WebProcess
var (
	extensionInfo           []shared.ExtensionInfo
	hasWebRequestListeners  bool
	enableWebRequestMetrics bool
	webRequestAllowCache    = newWebRequestAllowStore()
	webRequestBlockCache    = newWebRequestBlockStore()

	// Socket IPC for webRequest blocking (replaces GLib message IPC)
	webRequestSocketPath string
	webRequestSocket     net.Conn
	webRequestSocketMu   sync.Mutex
	webRequestReader     *bufio.Reader
	webRequestEncoder    *json.Encoder
)

// webRequestBlockStore caches blocking decisions to skip IPC for repeated URLs.
// Ad/tracker URLs are often repeated across pages - caching avoids redundant IPC.
type webRequestBlockStore struct {
	mu      sync.RWMutex
	blocked map[string]struct{}
}

func newWebRequestBlockStore() *webRequestBlockStore {
	return &webRequestBlockStore{blocked: make(map[string]struct{})}
}

func (c *webRequestBlockStore) isBlocked(url string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.blocked[url]
	return ok
}

func (c *webRequestBlockStore) markBlocked(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blocked[url] = struct{}{}
}

// webRequestAllowStore remembers allowed domain+resourceType combinations globally.
// Unlike per-page caching, this persists across navigations - once cdn.example.com|script
// is allowed, it stays allowed everywhere. Uses LRU eviction at 10k entries.
type webRequestAllowStore struct {
	mu      sync.RWMutex
	allowed map[string]struct{} // "origin|resourceType" -> {}
	order   []string            // LRU order tracking (oldest first)
	maxSize int
}

func newWebRequestAllowStore() *webRequestAllowStore {
	return &webRequestAllowStore{
		allowed: make(map[string]struct{}),
		order:   make([]string, 0, 1024),
		maxSize: 10000,
	}
}

func (c *webRequestAllowStore) isAllowed(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.allowed[key]
	return ok
}

func (c *webRequestAllowStore) markAllowed(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.allowed[key]; !exists {
		c.allowed[key] = struct{}{}
		c.order = append(c.order, key)
		// LRU eviction if over limit
		if len(c.order) > c.maxSize {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.allowed, oldest)
		}
	}
}

// size returns current cache size (for metrics)
func (c *webRequestAllowStore) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.allowed)
}

// hasHTTPScheme checks if a URI uses HTTP or HTTPS scheme.
// webkit_uri_request_get_http_headers returns NULL for non-HTTP requests,
// so we should only call HTTPHeaders() for HTTP/HTTPS URIs.
func hasHTTPScheme(uri string) bool {
	return strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")
}

// isMessageHeadersValid checks if the underlying C pointer of a soup.MessageHeaders
// is not NULL. The gotk4 binding for HTTPHeaders() has a bug where it creates a Go
// wrapper even when the C function returns NULL, causing crashes when ForEach is called.
func isMessageHeadersValid(hdrs *soup.MessageHeaders) bool {
	if hdrs == nil {
		return false
	}
	return gextras.StructNative(unsafe.Pointer(hdrs)) != nil
}

//export webkit_web_process_extension_initialize_with_user_data
func webkit_web_process_extension_initialize_with_user_data(
	ext *C.WebKitWebProcessExtension,
	userData *C.GVariant,
) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in WebProcess extension initialization: %v", r)
		}
	}()

	// Wrap the C extension pointer in a Go object
	if ext == nil {
		log.Printf("ERROR: WebProcessExtension pointer is nil")
		return
	}

	goExt := wrapWebProcessExtension(ext)
	if goExt == nil {
		log.Printf("ERROR: Failed to wrap WebProcessExtension")
		return
	}

	// Parse user data for extension information
	if userData != nil {
		goUserData := wrapVariant(userData)
		if goUserData != nil {
			userDataStr, err := variantToString(goUserData)
			if err == nil {
				if err := parseExtensionData(userDataStr); err != nil {
					log.Printf("Warning: failed to parse extension data: %v", err)
				}
			}
		}
	}

	log.Printf("Dumber WebProcess extension initializing...")
	log.Printf("Loaded %d extension(s) for content script injection", len(extensionInfo))

	// Connect page-created signal
	goExt.ConnectPageCreated(onPageCreated)

	// IMPORTANT: Connect window-object-cleared GLOBALLY during initialization, not per-page.
	// This ensures the handler is registered BEFORE any pages are created, so we catch
	// the signal for extension WebViews (which use web-extension-mode=ManifestV2).
	// Following Epiphany's pattern: they connect this once globally, not per-page.
	defaultWorld := webkitwebprocessextension.ScriptWorldGetDefault()
	defaultWorld.ConnectWindowObjectCleared(onWindowObjectCleared)

	// NOTE: We do NOT register a user-message-received handler here.
	// Messages sent via page.SendMessageToView() are automatically routed to
	// WebContext-level handlers registered in the UI process (see internal/app/browser/browser.go).
	// The previous stub handler was intercepting messages and only echoing them back,
	// which broke all async WebExtension APIs like runtime.connect() and runtime.sendMessage().

	log.Printf("Dumber WebProcess extension initialized successfully")
}

// parseExtensionData parses the JSON extension data from InitUserData
func parseExtensionData(jsonStr string) error {
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.Trim(jsonStr, "'")

	initData, err := shared.ParseInitData(jsonStr)
	if err != nil {
		return fmt.Errorf("failed to unmarshal init data: %w", err)
	}

	extensionInfo = initData.Extensions
	hasWebRequestListeners = initData.HasWebRequestListeners
	enableWebRequestMetrics = initData.EnableWebRequestMetrics
	webRequestSocketPath = initData.WebRequestSocketPath

	// Log to file for debugging (WebProcess stderr may not be visible)
	logToFile("[webRequest] Init data parsed: hasListeners=%v, socketPath=%q", hasWebRequestListeners, webRequestSocketPath)

	// Connect to webRequest socket if path provided
	if webRequestSocketPath != "" {
		if err := initWebRequestSocket(); err != nil {
			logToFile("[webRequest] Failed to connect to socket %s: %v", webRequestSocketPath, err)
			logToFile("[webRequest] webRequest blocking will be DISABLED")
		} else {
			logToFile("[webRequest] Connected to socket: %s", webRequestSocketPath)
		}
	} else {
		logToFile("[webRequest] No socket path provided, webRequest blocking DISABLED")
	}

	return nil
}

// initWebRequestSocket connects to the UI process socket for webRequest IPC.
func initWebRequestSocket() error {
	conn, err := net.Dial("unix", webRequestSocketPath)
	if err != nil {
		return err
	}

	webRequestSocketMu.Lock()
	webRequestSocket = conn
	webRequestReader = bufio.NewReader(conn)
	webRequestEncoder = json.NewEncoder(conn)
	webRequestSocketMu.Unlock()

	return nil
}

// variantToString safely extracts a Go string from a GVariant.
// Using String() directly can return a printed variant (with quotes) when the
// underlying type is not a plain string, which breaks JSON parsing.
func variantToString(v *glib.Variant) (string, error) {
	if v == nil {
		return "", fmt.Errorf("variant is nil")
	}

	// For string type variants, use String() directly
	if v.TypeString() == "s" {
		return unquoteSingle(v.String()), nil
	}

	// Fallback to printed variant (e.g., "'{...}'") and strip outer single quotes
	printed := v.Print(false)
	if len(printed) >= 2 && printed[0] == '\'' && printed[len(printed)-1] == '\'' {
		return printed[1 : len(printed)-1], nil
	}

	return "", fmt.Errorf("expected string variant, got type %s", v.TypeString())
}

// unquoteSingle removes a single pair of leading/trailing single quotes.
func unquoteSingle(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// wrapWebProcessExtension wraps a C WebKitWebProcessExtension pointer into a Go object
func wrapWebProcessExtension(ext *C.WebKitWebProcessExtension) *webkitwebprocessextension.WebProcessExtension {
	// Take ownership of the GObject and wrap it
	obj := coreglib.Take(unsafe.Pointer(ext))
	return &webkitwebprocessextension.WebProcessExtension{
		Object: obj,
	}
}

// wrapVariant wraps a C GVariant pointer into a Go object using glib v2 API
func wrapVariant(v *C.GVariant) *glib.Variant {
	if v == nil {
		return nil
	}

	// Handle floating references
	if C.g_variant_is_floating(v) != 0 {
		C.g_variant_ref_sink(v)
	} else {
		C.g_variant_ref(v)
	}

	// Use gextras.NewStructNative for v2 API (same pattern as NewVariantString)
	variant := (*glib.Variant)(gextras.NewStructNative(unsafe.Pointer(v)))
	C.g_variant_ref(v)
	runtime.SetFinalizer(
		gextras.StructIntern(unsafe.Pointer(variant)),
		func(intern *struct{ C unsafe.Pointer }) {
			C.g_variant_unref((*C.GVariant)(intern.C))
		},
	)

	return variant
}

// onWindowObjectCleared is called GLOBALLY for all pages when their window object is cleared.
// This is connected during initialization (not per-page) following Epiphany's pattern.
// This ensures we catch the signal for extension WebViews before their scripts execute.
func onWindowObjectCleared(webPage *webkitwebprocessextension.WebPage, frame *webkitwebprocessextension.Frame) {
	// Get both URIs for debugging - pageURI may be empty, but we try both
	pageURI := webPage.URI()
	frameURI := frame.URI()

	// Debug: log both URIs to understand timing
	LogDebug(webPage, "[native-api] window-object-cleared: pageURI=%q frameURI=%q", pageURI, frameURI)

	// Check if this is an extension page (dumb-extension://)
	// Try pageURI first (like Epiphany does), fallback to frameURI
	extensionURI := pageURI
	if !strings.HasPrefix(extensionURI, "dumb-extension://") {
		extensionURI = frameURI
	}
	if !strings.HasPrefix(extensionURI, "dumb-extension://") {
		return
	}

	LogDebug(webPage, "[native-api] Extension page detected at window-object-cleared: %s", extensionURI)

	// Extract extension ID from URI: dumb-extension://{id}/...
	parts := strings.SplitN(strings.TrimPrefix(extensionURI, "dumb-extension://"), "/", 2)
	if len(parts) > 0 && parts[0] != "" {
		extID := parts[0]
		LogDebug(webPage, "[native-api] Injecting native APIs for extension: %s", extID)

		// Inject native browser APIs for this extension page
		// Pass the frame from window-object-cleared so we get the correct JS context
		injectNativeAPIsForExtensionPage(webPage, frame, extID)
	} else {
		LogError(webPage, "[native-api] Failed to extract extension ID from URI=%s", extensionURI)
	}
}

func onPageCreated(page *webkitwebprocessextension.WebPage) {
	pageID := page.ID()
	uri := page.URI()

	LogDebug(page, "[page-lifecycle] Page created: ID=%d, URI=%s (empty at creation, will be set on document load)", pageID, uri)

	// Forward console messages from extension pages back to the UI process so errors aren't silent
	page.ConnectConsoleMessageSent(func(consoleMessage *webkitwebprocessextension.ConsoleMessage) {
		forwardConsoleMessageToUI(page, consoleMessage)
	})

	// NOTE: window-object-cleared is now connected GLOBALLY in initialization,
	// not per-page. This follows Epiphany's pattern and ensures we catch the signal
	// before extension scripts execute.

	// Hook document-loaded for general content script injection AND extension API fallback
	page.ConnectDocumentLoaded(func() {
		loadedURI := page.URI()
		LogDebug(page, "[page-lifecycle] Document loaded: page=%d, uri=%s", pageID, loadedURI)

		// FALLBACK: For extension pages, window-object-cleared doesn't fire on the default
		// ScriptWorld (likely due to web-extension-mode isolation). Inject APIs here instead.
		// ES6 modules load asynchronously, so this may still work for them.
		if strings.HasPrefix(loadedURI, "dumb-extension://") {
			LogDebug(page, "[native-api] Fallback: injecting APIs at document-loaded for extension page")
			parts := strings.SplitN(strings.TrimPrefix(loadedURI, "dumb-extension://"), "/", 2)
			if len(parts) > 0 && parts[0] != "" {
				extID := parts[0]
				// Get the main frame for injection
				mainFrame := page.MainFrame()
				if mainFrame != nil {
					LogDebug(page, "[native-api] Fallback: injecting native APIs for extension: %s", extID)
					injectNativeAPIsForExtensionPage(page, mainFrame, extID)
				} else {
					LogError(page, "[native-api] Fallback: main frame is nil for extension page")
				}
			}
		}

		// Call general content script injection
		onDocumentLoaded(page)
	})

	// Hook network requests for webRequest API only if any listeners were registered.
	if hasWebRequestListeners {
		page.ConnectSendRequest(func(request *webkitwebprocessextension.URIRequest, redirectedResponse *webkitwebprocessextension.URIResponse) bool {
			return onSendRequest(page, request, redirectedResponse)
		})
	} else {
		LogDebug(page, "[webRequest] Skipping request hook for page %d (no listeners registered)", pageID)
	}

	// Inject content scripts that should run at document_start
	injectContentScriptsForTiming(page, "document_start")
}

// forwardConsoleMessageToUI mirrors JS console messages from extension pages into the UI logs.
// This makes background/popup errors visible in dumber-webext.log for easier debugging.
func forwardConsoleMessageToUI(page *webkitwebprocessextension.WebPage, consoleMessage *webkitwebprocessextension.ConsoleMessage) {
	if page == nil || consoleMessage == nil {
		return
	}

	pageURI := page.URI()
	sourceID := consoleMessage.SourceID()
	if sourceID == "" {
		sourceID = pageURI
	}

	// Only capture extension contexts to avoid noisy site logs
	if sourceID == "" || (!strings.HasPrefix(sourceID, "dumb-extension://") && !strings.HasPrefix(pageURI, "dumb-extension://")) {
		return
	}

	level := consoleMessage.Level()
	levelLabel := strings.ToLower(level.String())
	message := consoleMessage.Text()
	line := consoleMessage.Line()
	source := strings.ToLower(consoleMessage.Source().String())

	logLine := fmt.Sprintf("[console:%s][%s:%d][%s] %s", levelLabel, sourceID, line, source, message)

	switch level {
	case webkitwebprocessextension.ConsoleMessageLevelError:
		LogError(page, "%s", logLine)
	case webkitwebprocessextension.ConsoleMessageLevelWarning:
		LogWarn(page, "%s", logLine)
	default:
		LogInfo(page, "%s", logLine)
	}
}

// shouldSkipWebRequest returns true for requests very unlikely to be blocked.
// This avoids expensive IPC for same-origin static assets.
func shouldSkipWebRequest(pageURI string, requestURI string, resourceType api.ResourceType, fetchSite string) bool {
	// Always check extension URLs (internal)
	if strings.HasPrefix(requestURI, "dumb-extension://") {
		return true
	}

	// Always check data URLs (can't be blocked by domain)
	if strings.HasPrefix(requestURI, "data:") {
		return true
	}

	// Always check blob URLs
	if strings.HasPrefix(requestURI, "blob:") {
		return true
	}

	// Explicit same-origin/same-site from fetch metadata: skip low-risk types
	if fetchSite == "same-origin" || fetchSite == "same-site" {
		switch resourceType {
		case api.ResourceTypeScript, api.ResourceTypeXMLHTTP, api.ResourceTypeSub,
			api.ResourceTypeWebSocket, api.ResourceTypePing, api.ResourceTypeMain:
			return false
		default:
			return true
		}
	}

	// High-priority types that uBlock actively filters - NEVER skip
	switch resourceType {
	case api.ResourceTypeScript, api.ResourceTypeXMLHTTP, api.ResourceTypeSub,
		api.ResourceTypeWebSocket, api.ResourceTypePing:
		return false
	}

	// For images, fonts, media, stylesheets - skip if same-origin
	// uBlock filters cross-origin ad/tracker resources, not same-origin content
	switch resourceType {
	case api.ResourceTypeImage, api.ResourceTypeFont, api.ResourceTypeMedia, api.ResourceTypeStylesheet:
		return isSameOrigin(pageURI, requestURI)
	}

	// Everything else goes through webRequest
	return false
}

// buildAllowCacheKey returns a domain-level cache key (origin|resourceType).
// This allows caching at domain level - once cdn.example.com|script is allowed,
// all scripts from that origin are allowed without IPC.
func buildAllowCacheKey(requestURI string, resourceType api.ResourceType) string {
	origin := extractOrigin(requestURI)
	return fmt.Sprintf("%s|%s", origin, resourceType)
}

// isSameOrigin checks if two URIs share the same origin (scheme + host)
func isSameOrigin(uri1, uri2 string) bool {
	origin1 := extractOrigin(uri1)
	origin2 := extractOrigin(uri2)
	return origin1 != "" && origin1 == origin2
}

// extractOrigin returns scheme://host from a URI
func extractOrigin(uri string) string {
	// Find scheme
	schemeEnd := strings.Index(uri, "://")
	if schemeEnd < 0 {
		return ""
	}

	// Find host end (next / or end of string)
	hostStart := schemeEnd + 3
	hostEnd := strings.Index(uri[hostStart:], "/")
	if hostEnd < 0 {
		return uri // No path, entire URI is origin
	}

	return uri[:hostStart+hostEnd]
}

func onSendRequest(page *webkitwebprocessextension.WebPage, request *webkitwebprocessextension.URIRequest, redirectedResponse *webkitwebprocessextension.URIResponse) bool {
	// If no extensions are enabled there is nothing to consult; allow immediately.
	if len(extensionInfo) == 0 {
		return false
	}

	requestURI := request.URI()
	pageURI := page.URI()

	// Fast-path: Build minimal details to check resource type
	// Only call HTTPHeaders() for HTTP/HTTPS requests - webkit returns NULL for other schemes
	// (e.g., dumb-extension://, data:, blob:) and gotk4 bindings don't handle NULL correctly
	headers := map[string]string{}
	if hasHTTPScheme(requestURI) {
		if httpHeaders := request.HTTPHeaders(); isMessageHeadersValid(httpHeaders) {
			httpHeaders.ForEach(func(name, value string) {
				headers[name] = value
			})
		}
	}
	resourceType := detectResourceType(headers)
	fetchSite := strings.ToLower(headers["Sec-Fetch-Site"])

	// NEVER block main document requests - this causes reload loops
	if resourceType == api.ResourceTypeMain {
		return false
	}

	// Skip webRequest for low-risk same-origin resources
	if shouldSkipWebRequest(pageURI, requestURI, resourceType, fetchSite) {
		return false
	}

	// Fast block-path: if we already know this URL is blocked, skip IPC entirely
	if webRequestBlockCache.isBlocked(requestURI) {
		LogMetrics(page, "[webRequest:metrics] cache=block_hit url=%s", requestURI)
		return true
	}

	// Fast allow-path: domain-level cache - if origin|type was allowed, skip IPC
	allowKey := buildAllowCacheKey(requestURI, resourceType)
	if webRequestAllowCache.isAllowed(allowKey) {
		LogMetrics(page, "[webRequest:metrics] cache=allow_hit key=%s", allowKey)
		return false
	}

	details := buildRequestDetailsFromParsed(page, request, headers, resourceType)

	// Blocking IPC call - cache will reduce future calls for same URLs
	ipcStart := time.Now()
	decision := dispatchBlockingWebRequest(page, "webRequest:onBeforeRequest", details)
	ipcDuration := time.Since(ipcStart)
	LogMetrics(page, "[webRequest:metrics] ipc duration=%v cancel=%v url=%s", ipcDuration, decision.Cancel, requestURI)

	if decision.RedirectURL != "" {
		request.SetURI(decision.RedirectURL)
	}
	if len(decision.RequestHeaders) > 0 && hasHTTPScheme(requestURI) {
		if hdrs := request.HTTPHeaders(); isMessageHeadersValid(hdrs) {
			for name, value := range decision.RequestHeaders {
				hdrs.Replace(name, value)
			}
		}
	}

	// Cache decisions to avoid repeated IPC for same URLs/domains
	if decision.Cancel {
		// Blocked URLs are cached globally by full URL (ad URLs repeat across pages)
		webRequestBlockCache.markBlocked(requestURI)
	} else if decision.RedirectURL == "" && len(decision.RequestHeaders) == 0 {
		// Allowed domains cached globally by origin|type
		// Once cdn.example.com|script is allowed, it stays allowed everywhere
		webRequestAllowCache.markAllowed(allowKey)
	}

	return decision.Cancel
}

// webRequestIPCRequest is the JSON structure sent to UI process socket
type webRequestIPCRequest struct {
	Details api.RequestDetails `json:"details"`
}

// dispatchBlockingWebRequest sends webRequest event to UI process via UNIX socket.
// This uses blocking socket I/O which does NOT iterate the GLib main loop,
// avoiding the re-entrancy deadlock that plagued the old GLib message IPC.
func dispatchBlockingWebRequest(page *webkitwebprocessextension.WebPage, name string, details api.RequestDetails) webRequestDecision {
	webRequestSocketMu.Lock()
	defer webRequestSocketMu.Unlock()

	// If socket not connected, allow the request (blocking disabled)
	if webRequestSocket == nil {
		return webRequestDecision{}
	}

	// Build request
	req := webRequestIPCRequest{Details: details}

	// Send request (JSON + newline)
	if err := webRequestEncoder.Encode(req); err != nil {
		LogError(page, "[webRequest] Socket write error: %v", err)
		// Try to reconnect for next request
		reconnectWebRequestSocket()
		return webRequestDecision{}
	}

	// Read response (blocking read - this is safe, doesn't iterate main loop!)
	line, err := webRequestReader.ReadBytes('\n')
	if err != nil {
		LogError(page, "[webRequest] Socket read error: %v", err)
		// Try to reconnect for next request
		reconnectWebRequestSocket()
		return webRequestDecision{}
	}

	var decision webRequestDecision
	if err := json.Unmarshal(line, &decision); err != nil {
		LogError(page, "[webRequest] Failed to parse response: %v", err)
		return webRequestDecision{}
	}

	return decision
}

// reconnectWebRequestSocket attempts to reconnect to the UI process socket.
// Called when a socket error occurs - next request will try with fresh connection.
func reconnectWebRequestSocket() {
	if webRequestSocket != nil {
		webRequestSocket.Close()
		webRequestSocket = nil
		webRequestReader = nil
		webRequestEncoder = nil
	}

	if webRequestSocketPath == "" {
		return
	}

	conn, err := net.Dial("unix", webRequestSocketPath)
	if err != nil {
		log.Printf("[webRequest] Reconnect failed: %v", err)
		return
	}

	webRequestSocket = conn
	webRequestReader = bufio.NewReader(conn)
	webRequestEncoder = json.NewEncoder(conn)
	log.Printf("[webRequest] Reconnected to socket")
}

// buildRequestDetailsFromParsed builds RequestDetails using pre-parsed headers and resource type
func buildRequestDetailsFromParsed(page *webkitwebprocessextension.WebPage, request *webkitwebprocessextension.URIRequest, headers map[string]string, resourceType api.ResourceType) api.RequestDetails {
	return api.RequestDetails{
		RequestID:      fmt.Sprintf("%d-%d", page.ID(), time.Now().UnixNano()),
		URL:            request.URI(),
		Method:         request.HTTPMethod(),
		FrameID:        int64(page.ID()),
		ParentFrameID:  -1, // Not available from WebKit API
		TabID:          int64(page.ID()),
		Type:           resourceType,
		TimeStamp:      float64(time.Now().UnixMilli()),
		Initiator:      page.URI(),
		RequestHeaders: headers,
	}
}

// detectResourceType determines the Chrome webRequest resource type from request headers
func detectResourceType(headers map[string]string) api.ResourceType {
	// Sec-Fetch-Dest is the most reliable indicator
	secFetchDest := headers["Sec-Fetch-Dest"]
	switch secFetchDest {
	case "document":
		return api.ResourceTypeMain
	case "iframe":
		return api.ResourceTypeSub
	case "style":
		return api.ResourceTypeStylesheet
	case "script":
		return api.ResourceTypeScript
	case "image":
		return api.ResourceTypeImage
	case "font":
		return api.ResourceTypeFont
	case "object", "embed":
		return api.ResourceTypeObject
	case "audio", "video", "track":
		return api.ResourceTypeMedia
	case "empty":
		// Could be XHR, fetch, ping, etc - check Accept header
		accept := headers["Accept"]
		if strings.Contains(accept, "application/json") || strings.Contains(accept, "*/*") {
			return api.ResourceTypeXMLHTTP
		}
		return api.ResourceTypeOther
	case "websocket":
		return api.ResourceTypeWebSocket
	}

	// Fallback: check Accept header for hints
	accept := headers["Accept"]
	switch {
	case strings.HasPrefix(accept, "text/html"):
		return api.ResourceTypeMain
	case strings.HasPrefix(accept, "text/css"):
		return api.ResourceTypeStylesheet
	case strings.HasPrefix(accept, "image/"):
		return api.ResourceTypeImage
	case strings.Contains(accept, "javascript"):
		return api.ResourceTypeScript
	}

	return api.ResourceTypeOther
}

// webRequestDecision represents the UI process decision for a request
type webRequestDecision struct {
	Cancel         bool              `json:"cancel"`
	RedirectURL    string            `json:"redirectUrl,omitempty"`
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`
}

func onDocumentLoaded(page *webkitwebprocessextension.WebPage) {
	LogDebug(page, "Document loaded: page=%d, uri=%s", page.ID(), page.URI())

	// Inject content scripts at document_end and document_idle
	injectContentScriptsForTiming(page, "document_end")
	injectContentScriptsForTiming(page, "document_idle")
	injectContentScriptsForTiming(page, "") // Empty means document_idle (default)
}

// injectContentScriptsForTiming injects all content scripts that match the page URL and timing
func injectContentScriptsForTiming(page *webkitwebprocessextension.WebPage, timing string) {
	pageURI := page.URI()
	if pageURI == "" {
		return
	}

	for _, ext := range extensionInfo {
		if !ext.Enabled {
			continue
		}

		for _, cs := range ext.ContentScripts {
			// Check timing
			runAt := cs.RunAt
			if runAt == "" {
				runAt = "document_idle" // Default
			}
			if runAt != timing {
				continue
			}

			// Check if URL matches
			if !matchesContentScript(pageURI, cs) {
				continue
			}

			LogDebug(page, "[inject] Injecting content scripts for %s at %s", ext.Name, timing)

			// Create isolated ScriptWorld for this extension
			worldName := fmt.Sprintf("dumber-ext-%s", ext.ID)
			world := webkitwebprocessextension.NewScriptWorldWithName(worldName)

			// Inject shim + content scripts into the world
			injectScriptsIntoWorld(page, world, ext, cs)
		}
	}
}

// matchesContentScript checks if a URL matches a content script's patterns
func matchesContentScript(url string, cs shared.ContentScript) bool {
	// Check excludes first
	if shared.ExcludesURL(url, cs.ExcludeMatch) {
		return false
	}

	// Include matches
	return shared.MatchURL(url, cs.Matches)
}

// injectScriptsIntoWorld injects content scripts into an isolated ScriptWorld
func injectScriptsIntoWorld(page *webkitwebprocessextension.WebPage, world *webkitwebprocessextension.ScriptWorld, ext shared.ExtensionInfo, cs shared.ContentScript) {
	// Get main frame
	frame := page.MainFrame()
	if frame == nil {
		LogWarn(page, "[inject] no main frame for page %d", page.ID())
		return
	}

	// Get JavaScript context for this world
	jsContext := frame.JsContextForScriptWorld(world)
	if jsContext == nil {
		LogWarn(page, "[inject] failed to get JS context for world %s", world.Name())
		return
	}

	// Inject shim first (provides chrome.* API)
	shim := getMinimalShim()
	result := jsContext.Evaluate(shim)
	if result != nil && result.IsString() {
		// Check for errors
		if exception := jsContext.Exception(); exception != nil {
			LogWarn(page, "[inject] shim injection error for %s: %v", ext.Name, exception.String())
		}
	}

	// Inject extension's content scripts
	for _, jsFile := range cs.JS {
		// Strip leading slash to avoid filepath.Join treating it as absolute path
		jsPath := filepath.Join(ext.Path, strings.TrimPrefix(jsFile, "/"))

		// Read script content
		content, err := os.ReadFile(jsPath)
		if err != nil {
			LogWarn(page, "[inject] failed to read %s: %v", jsPath, err)
			continue
		}

		// Inject into world
		result := jsContext.Evaluate(string(content))
		if result != nil {
			// Check for exceptions
			if exception := jsContext.Exception(); exception != nil {
				LogWarn(page, "[inject] failed to inject %s: %v", jsPath, exception.String())
			} else {
				LogDebug(page, "[inject] Injected %s into page %d", jsPath, page.ID())
			}
		}
	}

	// Inject CSS via JavaScript (WebProcess can't access UserContentManager)
	for _, cssFile := range cs.CSS {
		// Strip leading slash to avoid filepath.Join treating it as absolute path
		cssPath := filepath.Join(ext.Path, strings.TrimPrefix(cssFile, "/"))

		// Read CSS content
		cssContent, err := os.ReadFile(cssPath)
		if err != nil {
			LogWarn(page, "[inject] failed to read CSS %s: %v", cssPath, err)
			continue
		}

		// Escape backticks and template literals for JavaScript template string
		escapedCSS := strings.ReplaceAll(string(cssContent), "\\", "\\\\")
		escapedCSS = strings.ReplaceAll(escapedCSS, "`", "\\`")
		escapedCSS = strings.ReplaceAll(escapedCSS, "${", "\\${")

		// Inject CSS via <style> element
		injectScript := fmt.Sprintf(`
			(function() {
				'use strict';
				const style = document.createElement('style');
				style.textContent = `+"`%s`"+`;
				style.dataset.dumberExt = '%s';
				(document.head || document.documentElement).appendChild(style);
			})();
		`, escapedCSS, ext.ID)

		result := jsContext.Evaluate(injectScript)
		if result != nil {
			if exception := jsContext.Exception(); exception != nil {
				LogWarn(page, "[inject] CSS injection error for %s: %v", cssPath, exception.String())
			} else {
				LogDebug(page, "[inject] Injected CSS %s into page %d", cssPath, page.ID())
			}
		}
	}
}

// getMinimalShim returns a minimal chrome.* API shim for content scripts
func getMinimalShim() string {
	return `
// Minimal WebExtension API shim for content scripts
(function() {
	'use strict';

	// Create chrome namespace if it doesn't exist
	if (typeof chrome === 'undefined') {
		window.chrome = {};
	}

	// chrome.runtime API
	chrome.runtime = chrome.runtime || {
		sendMessage: function(message, callback) {
			console.log('[webext] chrome.runtime.sendMessage:', message);
			if (callback) {
				callback({success: false, error: 'Not implemented'});
			}
		},
		onMessage: {
			addListener: function(callback) {
				console.log('[webext] chrome.runtime.onMessage.addListener');
			}
		},
		getURL: function(path) {
			return 'extension://' + path;
		}
	};

	// chrome.storage API
	chrome.storage = chrome.storage || {
		local: {
			get: function(keys, callback) {
				callback({});
			},
			set: function(items, callback) {
				if (callback) callback();
			}
		}
	};

	// Firefox compatibility - provide 'browser' namespace as alias to chrome API
	if (typeof browser === 'undefined') {
		window.browser = window.chrome;
	}

	console.log('[webext] Chrome API shim loaded');
})();
`
}

// injectNativeAPIsForExtensionPage injects browser.* APIs into extension pages
func injectNativeAPIsForExtensionPage(page *webkitwebprocessextension.WebPage, frame *webkitwebprocessextension.Frame, extensionID string) {
	LogDebug(page, "[native-api] injectNativeAPIsForExtensionPage called for %s", extensionID)

	if frame == nil {
		LogError(page, "[native-api] No frame provided")
		return
	}

	// Find extension metadata
	var extInfo *shared.ExtensionInfo
	for i := range extensionInfo {
		if extensionInfo[i].ID == extensionID {
			extInfo = &extensionInfo[i]
			break
		}
	}

	if extInfo == nil {
		LogError(page, "[native-api] No metadata found for extension %s", extensionID)
		return
	}

	LogDebug(page, "[native-api] Found metadata for %s", extensionID)

	// Use manifest and translations from init data
	extData := &extensionPageData{
		extensionID:  extensionID,
		manifest:     extInfo.ManifestJSON,
		translations: extInfo.Translations,
		uiLanguage:   extInfo.UILanguage,
	}

	LogDebug(page, "[native-api] Calling installNativeBrowserAPIs...")
	// Install native APIs
	installNativeBrowserAPIs(page, frame, extData)
	LogDebug(page, "[native-api] installNativeBrowserAPIs completed")
}

func main() {
	// Required for CGO shared library, but never called
}

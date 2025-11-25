package main

// #cgo pkg-config: webkitgtk-web-process-extension-6.0 glib-2.0
// #include <webkit/webkit-web-process-extension.h>
// #include <glib.h>
import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/webext/api"
	"github.com/bnema/dumber/internal/webext/shared"
	"github.com/diamondburned/gotk4-webkitgtk/pkg/webkitwebprocessextension/v6"
	"github.com/diamondburned/gotk4/pkg/core/gextras"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// Global state for WebProcess
var (
	extensionInfo          []shared.ExtensionInfo
	hasWebRequestListeners bool
)

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

func onPageCreated(page *webkitwebprocessextension.WebPage) {
	pageID := page.ID()
	uri := page.URI()

	LogDebug(page, "[page-lifecycle] Page created: ID=%d, URI=%s (empty at creation, will be set on document load)", pageID, uri)

	// Forward console messages from extension pages back to the UI process so errors aren't silent
	page.ConnectConsoleMessageSent(func(consoleMessage *webkitwebprocessextension.ConsoleMessage) {
		forwardConsoleMessageToUI(page, consoleMessage)
	})

	// Connect to window-object-cleared on the default script world
	// This fires BEFORE page scripts execute, ensuring browser.* APIs are available immediately
	defaultWorld := webkitwebprocessextension.ScriptWorldGetDefault()
	defaultWorld.ConnectWindowObjectCleared(func(webPage *webkitwebprocessextension.WebPage, frame *webkitwebprocessextension.Frame) {
		// Only handle this specific page
		if webPage.ID() != pageID {
			return
		}

		pageURI := webPage.URI()

		// Check if this is an extension page (dumb-extension://)
		if !strings.HasPrefix(pageURI, "dumb-extension://") {
			return
		}

		LogDebug(webPage, "[native-api] Extension page detected at window-object-cleared: %s", pageURI)

		// Extract extension ID from URI: dumb-extension://{id}/...
		parts := strings.SplitN(strings.TrimPrefix(pageURI, "dumb-extension://"), "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			extID := parts[0]
			LogDebug(webPage, "[native-api] Injecting native APIs for extension: %s", extID)

			// Inject native browser APIs for this extension page
			// Pass the frame from window-object-cleared so we get the correct JS context
			injectNativeAPIsForExtensionPage(webPage, frame, extID)
		} else {
			LogError(webPage, "[native-api] Failed to extract extension ID from URI=%s", pageURI)
		}
	})

	// Also hook document-loaded for general content script injection
	page.ConnectDocumentLoaded(func() {
		loadedURI := page.URI()
		LogDebug(page, "[page-lifecycle] Document loaded: page=%d, uri=%s", pageID, loadedURI)

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

func onSendRequest(page *webkitwebprocessextension.WebPage, request *webkitwebprocessextension.URIRequest, redirectedResponse *webkitwebprocessextension.URIResponse) bool {
	// If no extensions are enabled there is nothing to consult; allow immediately.
	if len(extensionInfo) == 0 {
		return false
	}

	details := buildRequestDetails(page, request)
	beforeDecision := dispatchBlockingWebRequest(page, "webRequest:onBeforeRequest", details)
	if beforeDecision.RedirectURL != "" {
		request.SetURI(beforeDecision.RedirectURL)
	}
	if len(beforeDecision.RequestHeaders) > 0 {
		if headers := request.HTTPHeaders(); headers != nil {
			for name, value := range beforeDecision.RequestHeaders {
				headers.Replace(name, value)
			}
		}
	}
	if beforeDecision.Cancel {
		return true
	}

	details.URL = request.URI()
	details.RequestHeaders = map[string]string{}
	if headers := request.HTTPHeaders(); headers != nil {
		headers.ForEach(func(name, value string) {
			details.RequestHeaders[name] = value
		})
	}
	details.TimeStamp = float64(time.Now().UnixMilli())

	// Give extensions a chance to inspect modified headers before dispatch.
	sendHeadersDecision := dispatchBlockingWebRequest(page, "webRequest:onBeforeSendHeaders", details)
	if sendHeadersDecision.RedirectURL != "" {
		request.SetURI(sendHeadersDecision.RedirectURL)
	}
	if len(sendHeadersDecision.RequestHeaders) > 0 {
		if headers := request.HTTPHeaders(); headers != nil {
			for name, value := range sendHeadersDecision.RequestHeaders {
				headers.Replace(name, value)
			}
		}
	}

	return sendHeadersDecision.Cancel
}

func dispatchBlockingWebRequest(page *webkitwebprocessextension.WebPage, name string, details api.RequestDetails) webRequestDecision {
	payload, err := json.Marshal(details)
	if err != nil {
		LogError(page, "[webRequest] Failed to marshal %s details: %v", name, err)
		return webRequestDecision{}
	}

	variant := glib.NewVariantString(string(payload))
	msg := webkitwebprocessextension.NewUserMessage(name, variant)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resultCh := make(chan webRequestDecision, 1)

	page.SendMessageToView(ctx, msg, func(res gio.AsyncResulter) {
		defer close(resultCh)

		reply, replyErr := page.SendMessageToViewFinish(res)
		if replyErr != nil {
			LogError(page, "[webRequest] Failed to finish send-message for %s: %v", name, replyErr)
			resultCh <- webRequestDecision{}
			return
		}

		params := reply.Parameters()
		if params == nil {
			resultCh <- webRequestDecision{}
			return
		}

		replyStr, strErr := variantToString(params)
		if strErr != nil {
			LogError(page, "[webRequest] Invalid reply variant for %s: %v", name, strErr)
			resultCh <- webRequestDecision{}
			return
		}

		var decision webRequestDecision
		if err := json.Unmarshal([]byte(replyStr), &decision); err != nil {
			LogError(page, "[webRequest] Failed to parse reply for %s: %v", name, err)
			resultCh <- webRequestDecision{}
			return
		}

		resultCh <- decision
	})

	select {
	case decision, ok := <-resultCh:
		if ok {
			return decision
		}
	case <-ctx.Done():
		LogWarn(page, "[webRequest] %s handler timed out for %s", name, details.URL)
	}

	return webRequestDecision{}
}

// buildRequestDetails maps a WebKit URIRequest to our WebRequest API shape
func buildRequestDetails(page *webkitwebprocessextension.WebPage, request *webkitwebprocessextension.URIRequest) api.RequestDetails {
	headers := map[string]string{}
	if httpHeaders := request.HTTPHeaders(); httpHeaders != nil {
		httpHeaders.ForEach(func(name, value string) {
			headers[name] = value
		})
	}

	return api.RequestDetails{
		RequestID:      fmt.Sprintf("%d-%d", page.ID(), time.Now().UnixNano()),
		URL:            request.URI(),
		Method:         request.HTTPMethod(),
		FrameID:        int64(page.ID()),
		ParentFrameID:  -1, // Not available from WebKit API
		TabID:          int64(page.ID()),
		Type:           api.ResourceTypeOther,
		TimeStamp:      float64(time.Now().UnixMilli()),
		Initiator:      page.URI(),
		RequestHeaders: headers,
	}
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

	// Inject CSS
	for _, cssFile := range cs.CSS {
		// Strip leading slash to avoid filepath.Join treating it as absolute path
		cssPath := filepath.Join(ext.Path, strings.TrimPrefix(cssFile, "/"))

		// Check if file exists
		if _, err := os.Stat(cssPath); os.IsNotExist(err) {
			LogWarn(page, "[inject] CSS file not found: %s", cssPath)
			continue
		}

		// TODO: Use WebPage.AddUserStyleSheet() if available in gotk4 bindings
		LogDebug(page, "[inject] CSS injection for %s (not yet implemented in gotk4)", cssPath)
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

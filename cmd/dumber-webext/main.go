package main

// #cgo pkg-config: webkitgtk-web-process-extension-6.0 glib-2.0
// #include <webkit/webkit-web-process-extension.h>
// #include <glib.h>
import "C"
import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/diamondburned/gotk4-webkitgtk/pkg/webkitwebprocessextension/v6"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
)

// Global state for WebProcess
var (
	extensionInfo []ExtensionInfo
)

// ExtensionInfo mirrors internal/webext/init_data.go ExtensionInfo
type ExtensionInfo struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Enabled        bool            `json:"enabled"`
	Path           string          `json:"path"`
	ContentScripts []ContentScript `json:"content_scripts"`
}

// ContentScript mirrors internal/webext/manifest.go ContentScript
type ContentScript struct {
	Matches      []string `json:"matches"`
	ExcludeMatch []string `json:"exclude_matches,omitempty"`
	JS           []string `json:"js,omitempty"`
	CSS          []string `json:"css,omitempty"`
	RunAt        string   `json:"run_at,omitempty"`
	AllFrames    bool     `json:"all_frames,omitempty"`
}

//export webkit_web_process_extension_initialize_with_user_data
func webkit_web_process_extension_initialize_with_user_data(
	ext *C.WebKitWebProcessExtension,
	userData *C.GVariant,
) {
	// Wrap the C extension pointer in a Go object
	goExt := wrapWebProcessExtension(ext)

	log.Printf("Dumber WebProcess extension initializing...")

	// Parse extension data from userData
	if userData != nil {
		goUserData := wrapVariant(userData)
		userDataStr := goUserData.String()
		log.Printf("Received user data: %d bytes", len(userDataStr))

		if err := parseExtensionData(userDataStr); err != nil {
			log.Printf("Warning: failed to parse extension data: %v", err)
		} else {
			log.Printf("Loaded %d extension(s) for content script injection", len(extensionInfo))
		}
	}

	// Connect page-created signal
	goExt.ConnectPageCreated(onPageCreated)

	// Connect user-message-received signal
	goExt.ConnectUserMessageReceived(onUserMessage)

	log.Printf("Dumber WebProcess extension initialized successfully")
}

// parseExtensionData parses the JSON extension data from InitUserData
func parseExtensionData(jsonStr string) error {
	type InitData struct {
		Extensions []ExtensionInfo `json:"extensions"`
	}

	var initData InitData
	if err := json.Unmarshal([]byte(jsonStr), &initData); err != nil {
		return fmt.Errorf("failed to unmarshal init data: %w", err)
	}

	extensionInfo = initData.Extensions
	return nil
}

// wrapWebProcessExtension wraps a C WebKitWebProcessExtension pointer into a Go object
func wrapWebProcessExtension(ext *C.WebKitWebProcessExtension) *webkitwebprocessextension.WebProcessExtension {
	// Take ownership of the GObject and wrap it
	obj := coreglib.Take(unsafe.Pointer(ext))
	return &webkitwebprocessextension.WebProcessExtension{
		Object: obj,
	}
}

// wrapVariant wraps a C GVariant pointer into a Go object
func wrapVariant(v *C.GVariant) *coreglib.Variant {
	if v == nil {
		return nil
	}

	// Mirror coreglib.takeVariant: claim a ref and install a finalizer.
	if C.g_variant_is_floating(v) != 0 {
		C.g_variant_ref_sink(v)
	} else {
		C.g_variant_ref(v)
	}

	gv := &coreglib.Variant{}
	*(*unsafe.Pointer)(unsafe.Pointer(&gv.GVariant)) = unsafe.Pointer(v)
	runtime.SetFinalizer(gv, (*coreglib.Variant).Unref)
	return gv
}

func onPageCreated(page *webkitwebprocessextension.WebPage) {
	pageID := page.ID()
	uri := page.URI()

	log.Printf("Page created: ID=%d, URI=%s", pageID, uri)

	// Hook document loaded for content script injection
	page.ConnectDocumentLoaded(func() {
		onDocumentLoaded(page)
	})

	// Hook network requests for webRequest API
	page.ConnectSendRequest(func(request *webkitwebprocessextension.URIRequest, redirectedResponse *webkitwebprocessextension.URIResponse) bool {
		return onSendRequest(page, request, redirectedResponse)
	})

	// Inject content scripts that should run at document_start
	injectContentScriptsForTiming(page, "document_start")
}

func onSendRequest(page *webkitwebprocessextension.WebPage, request *webkitwebprocessextension.URIRequest, redirectedResponse *webkitwebprocessextension.URIResponse) bool {
	uri := request.URI()

	// Log network requests for debugging
	log.Printf("Network request: %s (page: %d)", uri, page.ID())

	// TODO: Implement webRequest API filtering here
	// - Call extension's onBeforeRequest handlers
	// - Check if request should be blocked
	// - Return false to cancel request, true to allow

	return true // Allow request
}

func onDocumentLoaded(page *webkitwebprocessextension.WebPage) {
	log.Printf("Document loaded: page=%d, uri=%s", page.ID(), page.URI())

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

			log.Printf("[inject] Injecting content scripts for %s at %s", ext.Name, timing)

			// Create isolated ScriptWorld for this extension
			worldName := fmt.Sprintf("dumber-ext-%s", ext.ID)
			world := webkitwebprocessextension.NewScriptWorldWithName(worldName)

			// Inject shim + content scripts into the world
			injectScriptsIntoWorld(page, world, ext, cs)
		}
	}
}

// matchesContentScript checks if a URL matches a content script's patterns
func matchesContentScript(url string, cs ContentScript) bool {
	// Check excludes first
	for _, exclude := range cs.ExcludeMatch {
		if matchPattern(url, exclude) {
			return false
		}
	}

	// Check includes
	for _, match := range cs.Matches {
		if matchPattern(url, match) {
			return true
		}
	}

	return false
}

// matchPattern checks if a URL matches a WebExtension match pattern
// Simplified version - for production use internal/webext/matcher.go logic
func matchPattern(url, pattern string) bool {
	// Handle special case
	if pattern == "<all_urls>" {
		return true
	}

	// Very simplified matching for now
	// TODO: Implement proper pattern matching (scheme://host/path)
	// For now, just check if pattern is contained (very permissive)
	return true
}

// injectScriptsIntoWorld injects content scripts into an isolated ScriptWorld
func injectScriptsIntoWorld(page *webkitwebprocessextension.WebPage, world *webkitwebprocessextension.ScriptWorld, ext ExtensionInfo, cs ContentScript) {
	// Get main frame
	frame := page.MainFrame()
	if frame == nil {
		log.Printf("[inject] Warning: no main frame for page %d", page.ID())
		return
	}

	// Get JavaScript context for this world
	jsContext := frame.JsContextForScriptWorld(world)
	if jsContext == nil {
		log.Printf("[inject] Warning: failed to get JS context for world %s", world.Name())
		return
	}

	// Inject shim first (provides chrome.* API)
	shim := getMinimalShim()
	result := jsContext.Evaluate(shim)
	if result != nil && result.IsString() {
		// Check for errors
		if exception := jsContext.Exception(); exception != nil {
			log.Printf("[inject] Warning: shim injection error for %s: %v", ext.Name, exception.String())
		}
	}

	// Inject extension's content scripts
	for _, jsFile := range cs.JS {
		jsPath := filepath.Join(ext.Path, jsFile)

		// Read script content
		content, err := os.ReadFile(jsPath)
		if err != nil {
			log.Printf("[inject] Warning: failed to read %s: %v", jsPath, err)
			continue
		}

		// Inject into world
		result := jsContext.Evaluate(string(content))
		if result != nil {
			// Check for exceptions
			if exception := jsContext.Exception(); exception != nil {
				log.Printf("[inject] Warning: failed to inject %s: %v", jsPath, exception.String())
			} else {
				log.Printf("[inject] Injected %s into page %d", jsPath, page.ID())
			}
		}
	}

	// Inject CSS
	for _, cssFile := range cs.CSS {
		cssPath := filepath.Join(ext.Path, cssFile)

		// Check if file exists
		if _, err := os.Stat(cssPath); os.IsNotExist(err) {
			log.Printf("[inject] Warning: CSS file not found: %s", cssPath)
			continue
		}

		// TODO: Use WebPage.AddUserStyleSheet() if available in gotk4 bindings
		log.Printf("[inject] CSS injection for %s (not yet implemented in gotk4)", cssPath)
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

	console.log('[webext] Chrome API shim loaded');
})();
`
}

func onUserMessage(message *webkitwebprocessextension.UserMessage) {
	name := message.Name()

	log.Printf("User message received: %s", name)

	// TODO: Route messages to appropriate extension handlers
	// Examples:
	// - "extension:sendMessage" - chrome.runtime.sendMessage
	// - "extension:getManifest" - chrome.runtime.getManifest
	// - "tabs:sendMessage" - chrome.tabs.sendMessage

	// For now, just echo back
	reply := webkitwebprocessextension.NewUserMessage(
		fmt.Sprintf("reply:%s", name),
		nil,
	)
	message.SendReply(reply)
}

func main() {
	// Required for CGO shared library, but never called
}

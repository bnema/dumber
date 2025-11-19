package main

// #cgo pkg-config: webkitgtk-web-process-extension-6.0
// #include <webkit/webkit-web-process-extension.h>
import "C"
import (
	"fmt"
	"log"
	"unsafe"

	"github.com/diamondburned/gotk4-webkitgtk/pkg/webkitwebprocessextension/v6"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
)

//export webkit_web_process_extension_initialize_with_user_data
func webkit_web_process_extension_initialize_with_user_data(
	ext *C.WebKitWebProcessExtension,
	userData *C.GVariant,
) {
	// Wrap the C extension pointer in a Go object
	goExt := wrapWebProcessExtension(ext)

	log.Printf("Dumber WebProcess extension initializing...")
	if userData != nil {
		goUserData := wrapVariant(userData)
		log.Printf("User data: %v", goUserData.String())
	}

	// Connect page-created signal
	goExt.ConnectPageCreated(onPageCreated)

	// Connect user-message-received signal
	goExt.ConnectUserMessageReceived(onUserMessage)

	log.Printf("Dumber WebProcess extension initialized successfully")
}

// wrapWebProcessExtension wraps a C WebKitWebProcessExtension pointer into a Go object
func wrapWebProcessExtension(ext *C.WebKitWebProcessExtension) *webkitwebprocessextension.WebProcessExtension {
	// Cast to unsafe.Pointer and create object
	ptr := coreglib.AssumeOwnership(unsafe.Pointer(ext))
	obj := ptr.Cast()
	// Type assert to WebProcessExtension
	return obj.(*webkitwebprocessextension.WebProcessExtension)
}

// wrapVariant wraps a C GVariant pointer into a Go object
func wrapVariant(v *C.GVariant) *coreglib.Variant {
	// GVariant is a simple type, just cast it
	return (*coreglib.Variant)(unsafe.Pointer(v))
}

func onPageCreated(page *webkitwebprocessextension.WebPage) {
	pageID := page.ID()
	uri := page.URI()

	log.Printf("Page created: ID=%d, URI=%s", pageID, uri)

	// TODO: Create isolated ScriptWorld for extensions
	// world := webkitwebprocessextension.ScriptWorldGetDefault()

	// TODO: Inject content scripts based on URL matching

	// Hook network requests
	page.ConnectSendRequest(func(request *webkitwebprocessextension.URIRequest, redirectedResponse *webkitwebprocessextension.URIResponse) bool {
		return onSendRequest(page, request, redirectedResponse)
	})

	// Hook document loaded
	page.ConnectDocumentLoaded(func() {
		onDocumentLoaded(page)
	})
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

	// TODO: Inject content scripts at document_end
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

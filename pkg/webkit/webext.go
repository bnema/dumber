package webkit

import (
	"log"

	webkitv6 "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// UserMessage is a type alias for webkit v6 UserMessage
type UserMessage = webkitv6.UserMessage

// WebExtensionConfig holds configuration for WebProcess extension initialization
type WebExtensionConfig struct {
	ExtensionsDirectory string
	InitUserData        string
	// SandboxPaths are additional paths to add to the WebProcess sandbox (read-write)
	SandboxPaths []string
}

// InitializeWebProcessExtensions sets up WebProcess extension loading.
// This must be called before any WebViews are created.
// Returns error if initialization fails.
func InitializeWebProcessExtensions(cfg *WebExtensionConfig) error {
	if cfg == nil {
		return nil // No extensions configured
	}

	// Get the default WebContext
	webContext := webkitv6.WebContextGetDefault()
	if webContext == nil {
		return ErrWebViewNotInitialized
	}

	log.Printf("[webext] Initializing WebProcess extension system")

	// CRITICAL: Add extensions directory to sandbox BEFORE connecting signal
	// The web process runs in a sandbox and needs explicit permission to access the .so file
	if cfg.ExtensionsDirectory != "" {
		webContext.AddPathToSandbox(cfg.ExtensionsDirectory, true) // read-only access
		log.Printf("[webext] Added extensions directory to sandbox: %s", cfg.ExtensionsDirectory)
	}

	// Add additional sandbox paths (e.g., socket directory for IPC)
	for _, path := range cfg.SandboxPaths {
		webContext.AddPathToSandbox(path, false) // read-write access for sockets
		log.Printf("[webext] Added path to sandbox (rw): %s", path)
	}

	// Connect to initialize-web-process-extensions signal
	// This is emitted when a new web process is about to be launched
	webContext.ConnectInitializeWebProcessExtensions(func() {
		log.Printf("[webext] Web process launching, configuring extensions...")

		// Set the directory where WebKit will look for .so files
		if cfg.ExtensionsDirectory != "" {
			webContext.SetWebProcessExtensionsDirectory(cfg.ExtensionsDirectory)
			log.Printf("[webext] Extensions directory set to: %s", cfg.ExtensionsDirectory)
		}

		// Pass initialization data to extensions (optional)
		if cfg.InitUserData != "" {
			variant := glib.NewVariantString(cfg.InitUserData)
			webContext.SetWebProcessExtensionsInitializationUserData(variant)
			log.Printf("[webext] Passed init data to extensions")
		}
	})

	log.Printf("[webext] WebProcess extension hooks installed")
	return nil
}

// RegisterWebExtensionMessageHandler registers a handler for messages from WebProcess extensions.
// The handler receives UserMessage objects from web process extensions.
// Returns true to indicate the message was handled (for async handling, call message.Reply later).
func RegisterWebExtensionMessageHandler(handler func(message *UserMessage) bool) error {
	webContext := webkitv6.WebContextGetDefault()
	if webContext == nil {
		return ErrWebViewNotInitialized
	}

	webContext.ConnectUserMessageReceived(handler)
	log.Printf("[webext] Registered WebProcess extension message handler")
	return nil
}

// SendMessageToWebExtensions sends a message to all WebProcess extensions.
// Messages are delivered to the onUserMessage handler in the extension.
func SendMessageToWebExtensions(message *UserMessage) error {
	webContext := webkitv6.WebContextGetDefault()
	if webContext == nil {
		return ErrWebViewNotInitialized
	}

	webContext.SendMessageToAllExtensions(message)
	return nil
}

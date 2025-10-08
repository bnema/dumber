package webkit

import (
	"fmt"
	"log"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

var (
	// globalNetworkSession holds the persistent network session to prevent garbage collection
	// This must be kept alive for the entire application lifetime
	globalNetworkSession *webkit.NetworkSession
)

// InitPersistentSession creates and initializes a persistent NetworkSession
// with the provided data and cache directories.
// This should be called before creating any WebViews to ensure they use persistent storage.
// Returns an error if initialization fails.
//
// IMPORTANT: Once created, this session is stored globally and will be used by all
// WebViews created afterward, as per WebKitGTK 6.0's design where the first created
// session becomes the default for the network process.
func InitPersistentSession(dataDir, cacheDir string) error {
	// If session already exists, return success (idempotent)
	// This ensures we only create ONE session for the entire application
	if globalNetworkSession != nil {
		log.Printf("[webkit] Using existing persistent network session")
		return nil
	}

	// Create persistent network session using gotk4 API
	// Per WebKitGTK 6.0 docs: "The first WebKitNetworkSession created becomes the default"
	session := webkit.NewNetworkSession(dataDir, cacheDir)
	if session == nil {
		return fmt.Errorf("failed to create persistent network session")
	}

	// Store the session globally to prevent garbage collection
	// This is CRITICAL - if the session is GC'd, WebKit falls back to ephemeral storage
	globalNetworkSession = session

	log.Printf("[webkit] Persistent network session created: data=%s, cache=%s", dataDir, cacheDir)

	// Verify the session is not ephemeral
	if session.IsEphemeral() {
		return fmt.Errorf("created session is ephemeral despite providing data directories")
	}

	// Verify the WebsiteDataManager is not ephemeral
	websiteDataManager := session.WebsiteDataManager()
	if websiteDataManager == nil {
		return fmt.Errorf("failed to get website data manager from network session")
	}
	if websiteDataManager.IsEphemeral() {
		return fmt.Errorf("website data manager is ephemeral despite providing data directories")
	}
	log.Printf("[webkit] WebsiteDataManager verified as non-ephemeral")

	// Configure CookieManager for persistent cookie storage
	// This is REQUIRED - without this, cookies are not persisted to disk
	cookieManager := session.CookieManager()
	if cookieManager == nil {
		return fmt.Errorf("failed to get cookie manager from network session")
	}

	// Set cookie persistent storage to SQLite database
	// Format: filepath.Join(dataDir, "cookies.db")
	cookiePath := dataDir + "/cookies.db"
	cookieManager.SetPersistentStorage(cookiePath, webkit.CookiePersistentStorageSqlite)

	// Set cookie accept policy to no third-party (default)
	cookieManager.SetAcceptPolicy(webkit.CookiePolicyAcceptNoThirdParty)

	log.Printf("[webkit] Cookie manager configured: storage=%s, policy=no-third-party", cookiePath)

	// Enable persistent credential storage (HTTP auth, etc.)
	session.SetPersistentCredentialStorageEnabled(true)

	// Verify this is indeed the default session now
	defaultSession := webkit.NetworkSessionGetDefault()
	if defaultSession == nil {
		return fmt.Errorf("default network session is nil after creating persistent session")
	}

	// Check if they're the same session by comparing ephemeral status
	if defaultSession.IsEphemeral() {
		return fmt.Errorf("default network session is ephemeral after creating persistent session")
	}

	log.Printf("[webkit] Network session verified as persistent and set as default")
	return nil
}

// GetGlobalNetworkSession returns the global network session.
// Returns nil if InitPersistentSession hasn't been called successfully.
func GetGlobalNetworkSession() *webkit.NetworkSession {
	return globalNetworkSession
}

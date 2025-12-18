package webkit

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/rs/zerolog"
)

// WebKitContext manages the shared WebContext and persistent NetworkSession.
// IMPORTANT: This MUST be initialized before creating any WebViews.
type WebKitContext struct {
	webContext     *webkit.WebContext
	networkSession *webkit.NetworkSession
	faviconDB      *webkit.FaviconDatabase

	dataDir  string
	cacheDir string

	logger      zerolog.Logger
	mu          sync.RWMutex
	initialized bool
}

// NewWebKitContext creates and initializes a WebKitContext with a persistent NetworkSession.
// The dataDir and cacheDir are used for cookie storage, cache, and other persistent data.
// This MUST be called before creating any WebViews to ensure they use persistent storage.
func NewWebKitContext(ctx context.Context, dataDir, cacheDir string) (*WebKitContext, error) {
	log := logging.FromContext(ctx).With().Str("component", "webkit-context").Logger()

	if dataDir == "" {
		return nil, fmt.Errorf("data directory cannot be empty")
	}
	if cacheDir == "" {
		return nil, fmt.Errorf("cache directory cannot be empty")
	}

	wkCtx := &WebKitContext{
		dataDir:  dataDir,
		cacheDir: cacheDir,
		logger:   log,
	}

	// Create persistent network session FIRST
	// Per WebKitGTK 6.0 docs: "The first WebKitNetworkSession created becomes the default"
	if err := wkCtx.initNetworkSession(); err != nil {
		return nil, fmt.Errorf("failed to init network session: %w", err)
	}

	// Get or create WebContext
	wkCtx.webContext = webkit.WebContextGetDefault()
	if wkCtx.webContext == nil {
		wkCtx.webContext = webkit.NewWebContext()
	}
	if wkCtx.webContext == nil {
		return nil, fmt.Errorf("failed to get or create WebContext")
	}

	// Set cache model for browser-style caching
	wkCtx.webContext.SetCacheModel(webkit.CacheModelWebBrowserValue)

	wkCtx.initialized = true
	log.Info().
		Str("data_dir", dataDir).
		Str("cache_dir", cacheDir).
		Msg("webkit context initialized")

	return wkCtx, nil
}

// initNetworkSession creates and configures the persistent network session.
func (c *WebKitContext) initNetworkSession() error {
	// Create persistent network session
	session := webkit.NewNetworkSession(&c.dataDir, &c.cacheDir)
	if session == nil {
		return fmt.Errorf("failed to create network session")
	}

	// Verify session is not ephemeral
	if session.IsEphemeral() {
		return fmt.Errorf("network session is ephemeral despite providing data directories")
	}

	// Verify WebsiteDataManager is not ephemeral
	dataManager := session.GetWebsiteDataManager()
	if dataManager == nil {
		return fmt.Errorf("failed to get website data manager")
	}
	if dataManager.IsEphemeral() {
		return fmt.Errorf("website data manager is ephemeral")
	}

	// Debug: verify the directories WebKit is actually using
	actualDataDir := dataManager.GetBaseDataDirectory()
	actualCacheDir := dataManager.GetBaseCacheDirectory()
	c.logger.Info().
		Str("expected_data_dir", c.dataDir).
		Str("actual_data_dir", actualDataDir).
		Str("expected_cache_dir", c.cacheDir).
		Str("actual_cache_dir", actualCacheDir).
		Msg("webkit directories configured")

	// Enable favicon collection and get database reference
	dataManager.SetFaviconsEnabled(true)
	c.faviconDB = dataManager.GetFaviconDatabase()
	if c.faviconDB != nil {
		c.logger.Debug().Msg("favicon database enabled")
	}

	// Configure cookie storage
	cookieManager := session.GetCookieManager()
	if cookieManager == nil {
		return fmt.Errorf("failed to get cookie manager")
	}

	cookiePath := filepath.Join(c.dataDir, "cookies.db")
	cookieManager.SetPersistentStorage(cookiePath, webkit.CookiePersistentStorageSqliteValue)
	cookieManager.SetAcceptPolicy(webkit.CookiePolicyAcceptNoThirdPartyValue)

	c.logger.Debug().
		Str("cookie_path", cookiePath).
		Msg("cookie storage configured")

	// Enable persistent credential storage
	session.SetPersistentCredentialStorageEnabled(true)

	// Configure TLS errors to emit signals (enables load-failed-with-tls-errors)
	session.SetTlsErrorsPolicy(webkit.TlsErrorsPolicyFailValue)

	// Verify default session
	defaultSession := webkit.NetworkSessionGetDefault()
	if defaultSession == nil || defaultSession.IsEphemeral() {
		c.logger.Warn().Msg("default session may not be persistent - created session might not be used by default")
	}

	c.networkSession = session
	c.logger.Debug().Msg("network session initialized as persistent")

	return nil
}

// Context returns the shared WebContext.
func (c *WebKitContext) Context() *webkit.WebContext {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.webContext
}

// NetworkSession returns the persistent NetworkSession.
func (c *WebKitContext) NetworkSession() *webkit.NetworkSession {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.networkSession
}

// FaviconDatabase returns the favicon database for persistent favicon storage.
func (c *WebKitContext) FaviconDatabase() *webkit.FaviconDatabase {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.faviconDB
}

// DataDir returns the data directory path.
func (c *WebKitContext) DataDir() string {
	return c.dataDir
}

// CacheDir returns the cache directory path.
func (c *WebKitContext) CacheDir() string {
	return c.cacheDir
}

// IsInitialized returns true if the context has been successfully initialized.
func (c *WebKitContext) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

// PrefetchDNS prefetches DNS for the given hostname to speed up future requests.
func (c *WebKitContext) PrefetchDNS(hostname string) {
	if hostname == "" || c.networkSession == nil {
		return
	}
	c.networkSession.PrefetchDns(hostname)
}

// Close performs cleanup. Currently a no-op as WebKit handles cleanup internally.
func (c *WebKitContext) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.initialized = false
	c.logger.Debug().Msg("webkit context closed")
	return nil
}

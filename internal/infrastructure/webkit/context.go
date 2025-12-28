package webkit

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/rs/zerolog"
)

// FaviconDatabase is a type alias for webkit.FaviconDatabase.
// Re-exported for use by UI layer without direct puregotk-webkit import.
type FaviconDatabase = webkit.FaviconDatabase

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
	return NewWebKitContextWithOptions(ctx, port.WebKitContextOptions{
		DataDir:  dataDir,
		CacheDir: cacheDir,
	})
}

// NewWebKitContextWithOptions creates and initializes a WebKitContext with the given options.
// This allows configuring memory pressure settings for web and network processes.
func NewWebKitContextWithOptions(ctx context.Context, opts port.WebKitContextOptions) (*WebKitContext, error) {
	log := logging.FromContext(ctx).With().Str("component", "webkit-context").Logger()

	if opts.DataDir == "" {
		return nil, fmt.Errorf("data directory cannot be empty")
	}
	if opts.CacheDir == "" {
		return nil, fmt.Errorf("cache directory cannot be empty")
	}

	wkCtx := &WebKitContext{
		dataDir:  opts.DataDir,
		cacheDir: opts.CacheDir,
		logger:   log,
	}

	// Apply network process memory pressure settings BEFORE creating session
	// Per WebKitGTK docs: must be called before creating any NetworkSession
	if opts.IsNetworkProcessMemoryConfigured() {
		applier := NewMemoryPressureApplier()
		if err := applier.ApplyNetworkProcessSettings(ctx, opts.NetworkProcessMemory); err != nil {
			log.Warn().Err(err).Msg("failed to apply network process memory pressure settings")
		}
	}

	// Create persistent network session FIRST
	// Per WebKitGTK 6.0 docs: "The first WebKitNetworkSession created becomes the default"
	if err := wkCtx.initNetworkSession(); err != nil {
		return nil, fmt.Errorf("failed to init network session: %w", err)
	}

	// WebViews in this codebase are created via WebKit's default WebContext.
	// If we create a separate WebContext here and only register our custom
	// dumb:// scheme on it, internal pages will break with "The URL can't be shown".
	//
	// Additionally, WebKit's "memory-pressure-settings" is a construct-only property,
	// so setting it after `webkit.NewWebContext()` triggers a GLib critical and has no effect.
	// Until we can reliably construct a WebContext with memory-pressure-settings AND ensure
	// all WebViews use that context, we intentionally stick to the default WebContext.
	if opts.IsWebProcessMemoryConfigured() {
		log.Warn().Msg("web process memory pressure settings requested but not applied (construct-only WebContext property; would break dumb:// scheme registration)")
	}

	wkCtx.webContext = webkit.WebContextGetDefault()
	if wkCtx.webContext == nil {
		return nil, fmt.Errorf("failed to get default WebContext")
	}

	// Set cache model for browser-style caching
	wkCtx.webContext.SetCacheModel(webkit.CacheModelWebBrowserValue)

	wkCtx.initialized = true
	log.Info().
		Str("data_dir", opts.DataDir).
		Str("cache_dir", opts.CacheDir).
		Msg("webkit context initialized")

	return wkCtx, nil
}

// createWebContextWithMemorySettings creates a WebContext with memory-pressure-settings.
// The memory-pressure-settings property is construct-only, so we need to set it at creation time.
func createWebContextWithMemorySettings(settings *webkit.MemoryPressureSettings) *webkit.WebContext {
	if settings == nil {
		return nil
	}

	// Create a new WebContext and set memory pressure settings
	// Note: memory-pressure-settings is construct-only, but we try setting it via property
	// as an alternative to g_object_new_with_properties which may not work reliably
	ctx := webkit.NewWebContext()
	if ctx != nil {
		// Try to set the property - this may not work for construct-only properties
		// but it's the best we can do without lower-level GObject access
		ctx.SetPropertyMemoryPressureSettings(settings.GoPointer())
	}
	return ctx
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

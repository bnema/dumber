// Package adapter provides UI-layer adapters that bridge domain services to GTK.
package adapter

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/rs/zerolog"
)

// FaviconAdapter bridges the domain FaviconService to GTK by providing
// gdk.Texture conversion and WebKit FaviconDatabase integration.
type FaviconAdapter struct {
	service      port.FaviconService
	faviconDB    port.FaviconDatabase
	textureCache map[string]*gdk.Texture
	mu           sync.RWMutex
	warnMu       sync.Mutex
	warnCounts   map[string]int

	// App-logo / internal-URL helpers (injected to decouple from infrastructure/favicon)
	isInternalURL      func(url string) bool
	internalDomain     string
	normalizedIconSize int
	getLogoBytes       func() []byte
}

type warningLogFunc func(log *zerolog.Logger, err error)

// FaviconAdapterConfig holds optional configuration for FaviconAdapter.
// Zero values disable the corresponding features (internal-URL logo, sized-PNG creation).
type FaviconAdapterConfig struct {
	// IsInternalURL reports whether a URL uses the internal app scheme (e.g. dumb://).
	IsInternalURL func(url string) bool
	// InternalDomain is the pseudo-domain used to cache the app logo favicon.
	InternalDomain string
	// NormalizedIconSize is the target size for dmenu/fuzzel icons (0 = skip).
	NormalizedIconSize int
	// GetLogoBytes returns the raw bytes of the app logo (nil = skip logo favicon).
	GetLogoBytes func() []byte
}

// NewFaviconAdapter creates a new FaviconAdapter.
// The faviconDB can be nil if a favicon database is not available.
func NewFaviconAdapter(service port.FaviconService, faviconDB port.FaviconDatabase, cfg FaviconAdapterConfig) *FaviconAdapter {
	return &FaviconAdapter{
		service:            service,
		faviconDB:          faviconDB,
		textureCache:       make(map[string]*gdk.Texture),
		warnCounts:         make(map[string]int),
		isInternalURL:      cfg.IsInternalURL,
		internalDomain:     cfg.InternalDomain,
		normalizedIconSize: cfg.NormalizedIconSize,
		getLogoBytes:       cfg.GetLogoBytes,
	}
}

// GetTexture returns a cached texture for the domain, or nil if not cached.
func (a *FaviconAdapter) GetTexture(domain string) *gdk.Texture {
	if domain == "" {
		return nil
	}

	a.mu.RLock()
	texture := a.textureCache[domain]
	a.mu.RUnlock()
	return texture
}

// GetTextureByURL returns a cached texture by extracting domain from URL.
// For internal dumb:// URLs, returns the app logo texture.
func (a *FaviconAdapter) GetTextureByURL(pageURL string) *gdk.Texture {
	// Handle internal dumb:// scheme URLs
	if a.isInternalURL != nil && a.isInternalURL(pageURL) {
		return a.GetTexture(a.internalDomain)
	}
	domain := domainurl.ExtractDomain(pageURL)
	return a.GetTexture(domain)
}

// GetOrFetch retrieves a favicon texture, checking caches and fetching if needed.
// The callback is invoked on the GTK main thread with the texture (or nil).
// For internal dumb:// URLs, returns the app logo texture.
func (a *FaviconAdapter) GetOrFetch(ctx context.Context, pageURL string, callback func(*gdk.Texture)) {
	if callback == nil {
		return
	}

	log := logging.FromContext(ctx)

	// Handle internal dumb:// scheme URLs - use app logo
	if a.isInternalURL != nil && a.isInternalURL(pageURL) {
		texture := a.getOrCreateLogoTexture()
		callback(texture)
		return
	}

	domain := domainurl.ExtractDomain(pageURL)

	// Check texture cache first
	if texture := a.GetTexture(domain); texture != nil {
		// Ensure sized PNG exists for CLI tools (async, idempotent)
		go a.ensureSizedPNG(ctx, domain)
		callback(texture)
		return
	}

	// Check service cache (memory + disk) and convert to texture
	if data, ok := a.service.GetCached(domain); ok && len(data) > 0 {
		texture := bytesToTexture(data)
		if texture != nil {
			a.setTexture(domain, texture)
			// Ensure sized PNG exists for CLI tools (async, idempotent)
			go a.ensureSizedPNG(ctx, domain)
			callback(texture)
			return
		}
	}

	// Helper to invoke callback on main thread
	invokeCallback := func(texture *gdk.Texture) {
		cb := glib.SourceFunc(func(uintptr) bool {
			callback(texture)
			return false
		})
		glib.IdleAdd(&cb, 0)
	}

	// Helper to fetch via service and invoke callback
	fetchViaService := func() {
		go func() {
			data, err := a.service.Get(ctx, domain)
			if err != nil || len(data) == 0 {
				invokeCallback(nil)
				return
			}

			texture := bytesToTexture(data)
			if texture != nil {
				a.setTexture(domain, texture)
			}
			invokeCallback(texture)
		}()
	}

	// Try engine FaviconDatabase if available
	if a.faviconDB != nil {
		a.faviconDB.GetFaviconAsync(pageURL, func(tex port.Texture) {
			if tex == nil {
				log.Debug().Str("domain", domain).Msg("favicon not in engine db, fetching via service")
				fetchViaService()
				return
			}
			// Convert port.Texture to *gdk.Texture
			gdkTex, ok := tex.(*gdk.Texture)
			if !ok {
				fetchViaService()
				return
			}
			a.setTexture(domain, gdkTex)
			invokeCallback(gdkTex)
			a.saveFaviconToDisk(ctx, domain, gdkTex)
		})
		return
	}

	// No WebKit DB, fetch via service directly
	fetchViaService()
}

// StoreFromWebKit stores a favicon texture received from WebKit signals.
// Also ensures the favicon is persisted to disk cache via the service.
func (a *FaviconAdapter) StoreFromWebKit(ctx context.Context, pageURL string, texture *gdk.Texture) {
	if texture == nil {
		return
	}

	domain := domainurl.ExtractDomain(pageURL)
	if domain == "" {
		return
	}

	// Store in texture cache
	a.setTexture(domain, texture)

	// Export as PNG and ensure disk cache
	a.saveFaviconToDisk(ctx, domain, texture)
}

// StoreFromWebKitWithOrigin stores a favicon for both current URL and original URL.
// Used when redirects occur (e.g., example.com → www.example.com).
func (a *FaviconAdapter) StoreFromWebKitWithOrigin(
	ctx context.Context, currentURL, originURL string, texture *gdk.Texture,
) {
	a.StoreFromWebKit(ctx, currentURL, texture)

	// Also store under original URL domain if different
	if originURL != "" && originURL != currentURL {
		originDomain := domainurl.ExtractDomain(originURL)
		currentDomain := domainurl.ExtractDomain(currentURL)
		if originDomain != "" && originDomain != currentDomain {
			a.setTexture(originDomain, texture)
			a.saveFaviconToDisk(ctx, originDomain, texture)
		}
	}
}

// saveFaviconToDisk exports a favicon texture to PNG and ensures disk cache is populated.
// Uses glib.IdleAdd to defer disk I/O until GTK main loop is idle, avoiding UI blocking.
func (a *FaviconAdapter) saveFaviconToDisk(ctx context.Context, domain string, texture *gdk.Texture) {
	log := logging.FromContext(ctx)
	cacheDirWarnKey := "cache-dir"
	savePNGWarnKey := "save-png:" + domain
	sizedPNGWarnKey := "sized-png:" + domain

	// Check if PNG save is needed before scheduling idle callback
	pngPath := a.service.DiskPathPNG(domain)
	needsPNGSave := pngPath != "" && !a.service.HasPNGOnDisk(domain)

	if needsPNGSave {
		// Ensure cache directory exists (cheap check, do it now)
		if err := a.service.EnsureCacheDir(); err != nil {
			a.logWarningDedup(ctx, cacheDirWarnKey, err, func(log *zerolog.Logger, warnErr error) {
				log.Warn().Err(warnErr).Str("domain", domain).Msg("failed to create favicon cache dir")
			})
			needsPNGSave = false
		} else {
			a.clearWarningDedup(cacheDirWarnKey)
		}
	}

	if needsPNGSave {
		// Schedule PNG save for when GTK main loop is idle (non-blocking)
		// texture.SaveToPng must run on main thread but we defer it to avoid blocking
		cb := glib.SourceFunc(func(_ uintptr) bool {
			if ok := texture.SaveToPng(pngPath); !ok {
				a.logWarningDedup(ctx, savePNGWarnKey, nil, func(log *zerolog.Logger, _ error) {
					log.Warn().Str("domain", domain).Str("path", pngPath).Msg("failed to save favicon PNG")
				})
			} else {
				a.clearWarningDedup(savePNGWarnKey)
				log.Debug().Str("domain", domain).Str("path", pngPath).Msg("saved favicon PNG")

				// Create normalized sized copy for dmenu/fuzzel (async, only if size is set)
				if a.normalizedIconSize > 0 {
					go func() {
						if err := a.service.EnsureSizedPNG(ctx, domain, a.normalizedIconSize); err != nil {
							a.logWarningDedup(ctx, sizedPNGWarnKey, err, func(log *zerolog.Logger, warnErr error) {
								log.Warn().Err(warnErr).Str("domain", domain).Msg("failed to create sized PNG")
							})
						} else {
							a.clearWarningDedup(sizedPNGWarnKey)
							log.Debug().Str("domain", domain).Msg("created sized PNG")
						}
					}()
				}
			}
			return false // don't repeat
		})
		glib.IdleAdd(&cb, 0)
	}

	// Ensure disk cache is populated (async, in background)
	go a.service.EnsureDiskCache(ctx, domain)
}

// PreloadFromCache attempts to load a favicon from cache without fetching.
// Returns the texture if found in memory or disk cache, nil otherwise.
func (a *FaviconAdapter) PreloadFromCache(pageURL string) *gdk.Texture {
	domain := domainurl.ExtractDomain(pageURL)
	if domain == "" {
		return nil
	}

	// Check texture cache
	if texture := a.GetTexture(domain); texture != nil {
		return texture
	}

	// Check service cache (memory + disk)
	if data, ok := a.service.GetCached(domain); ok && len(data) > 0 {
		texture := bytesToTexture(data)
		if texture != nil {
			a.setTexture(domain, texture)
			return texture
		}
	}

	return nil
}

// Service returns the underlying favicon service.
// Used by CLI components that need disk paths.
func (a *FaviconAdapter) Service() port.FaviconService {
	return a.service
}

// Close shuts down the adapter and underlying service.
func (a *FaviconAdapter) Close() {
	a.service.Close()
}

// Clear removes all entries from the texture cache.
func (a *FaviconAdapter) Clear() {
	a.mu.Lock()
	a.textureCache = make(map[string]*gdk.Texture)
	a.mu.Unlock()
}

// Size returns the number of entries in the texture cache.
func (a *FaviconAdapter) Size() int {
	a.mu.RLock()
	size := len(a.textureCache)
	a.mu.RUnlock()
	return size
}

// setTexture stores a texture in the cache.
func (a *FaviconAdapter) setTexture(domain string, texture *gdk.Texture) {
	if domain == "" || texture == nil {
		return
	}
	a.mu.Lock()
	a.textureCache[domain] = texture
	a.mu.Unlock()
}

// ensureSizedPNG ensures the sized PNG exists for CLI tools.
// This is idempotent - it only creates the file if it doesn't exist.
func (a *FaviconAdapter) ensureSizedPNG(ctx context.Context, domain string) {
	if domain == "" {
		return
	}

	log := logging.FromContext(ctx)

	hasPNG := a.service.HasPNGOnDisk(domain)
	hasSized := a.service.HasPNGSizedOnDisk(domain, a.normalizedIconSize)

	log.Debug().
		Str("domain", domain).
		Bool("has_png", hasPNG).
		Bool("has_sized", hasSized).
		Msg("checking sized PNG status")

	// Only create if original PNG exists but sized version doesn't
	if hasPNG && !hasSized {
		log.Debug().Str("domain", domain).Msg("creating sized PNG")
		if err := a.service.EnsureSizedPNG(ctx, domain, a.normalizedIconSize); err != nil {
			a.logWarningDedup(ctx, "sized-png:"+domain, err, func(log *zerolog.Logger, warnErr error) {
				log.Warn().Err(warnErr).Str("domain", domain).Msg("failed to create sized PNG")
			})
		} else {
			a.clearWarningDedup("sized-png:" + domain)
			log.Debug().Str("domain", domain).Msg("sized PNG created successfully")
		}
	}
}

func (a *FaviconAdapter) shouldLogWarningDedup(key string) (bool, int) {
	a.warnMu.Lock()
	defer a.warnMu.Unlock()

	count := a.warnCounts[key] + 1
	a.warnCounts[key] = count

	return count == 1, count - 1
}

func (a *FaviconAdapter) clearWarningDedup(key string) {
	a.warnMu.Lock()
	delete(a.warnCounts, key)
	a.warnMu.Unlock()
}

func (a *FaviconAdapter) logWarningDedup(
	ctx context.Context,
	key string,
	err error,
	warnFn warningLogFunc,
) {
	log := logging.FromContext(ctx)
	logWarning, suppressed := a.shouldLogWarningDedup(key)
	if logWarning {
		warnFn(log, err)
		return
	}

	if suppressed == 1 {
		event := log.Debug()
		if err != nil {
			event = event.Err(err)
		}
		event.Str("dedupe_key", key).Msg("suppressing repeated favicon warning")
	}
}

// getOrCreateLogoTexture returns the app logo texture, creating it if needed.
// The texture is cached under the internalDomain key.
func (a *FaviconAdapter) getOrCreateLogoTexture() *gdk.Texture {
	if a.internalDomain == "" || a.getLogoBytes == nil {
		return nil
	}

	// Check cache first
	if texture := a.GetTexture(a.internalDomain); texture != nil {
		return texture
	}

	// Create texture from logo bytes
	logoBytes := a.getLogoBytes()
	texture := bytesToTexture(logoBytes)
	if texture != nil {
		a.setTexture(a.internalDomain, texture)
	}
	return texture
}

// bytesToTexture converts raw favicon bytes to a GDK texture.
func bytesToTexture(data []byte) *gdk.Texture {
	if len(data) == 0 {
		return nil
	}

	bytes := glib.NewBytes(data, uint(len(data)))
	if bytes == nil {
		return nil
	}

	texture, err := gdk.NewTextureFromBytes(bytes)
	if err != nil {
		return nil
	}

	return texture
}

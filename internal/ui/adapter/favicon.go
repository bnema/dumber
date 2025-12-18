// Package adapter provides UI-layer adapters that bridge domain services to GTK.
package adapter

import (
	"context"
	"sync"

	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/favicon"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

// FaviconAdapter bridges the domain FaviconService to GTK by providing
// gdk.Texture conversion and WebKit FaviconDatabase integration.
type FaviconAdapter struct {
	service      *favicon.Service
	faviconDB    *webkit.FaviconDatabase
	textureCache map[string]*gdk.Texture
	mu           sync.RWMutex
}

// NewFaviconAdapter creates a new FaviconAdapter.
// The faviconDB can be nil if WebKit favicon database is not available.
func NewFaviconAdapter(service *favicon.Service, faviconDB *webkit.FaviconDatabase) *FaviconAdapter {
	return &FaviconAdapter{
		service:      service,
		faviconDB:    faviconDB,
		textureCache: make(map[string]*gdk.Texture),
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
func (a *FaviconAdapter) GetTextureByURL(pageURL string) *gdk.Texture {
	domain := domainurl.ExtractDomain(pageURL)
	return a.GetTexture(domain)
}

// GetOrFetch retrieves a favicon texture, checking caches and fetching if needed.
// The callback is invoked on the GTK main thread with the texture (or nil).
func (a *FaviconAdapter) GetOrFetch(ctx context.Context, pageURL string, callback func(*gdk.Texture)) {
	if callback == nil {
		return
	}

	log := logging.FromContext(ctx)
	domain := domainurl.ExtractDomain(pageURL)

	// Check texture cache first
	if texture := a.GetTexture(domain); texture != nil {
		callback(texture)
		return
	}

	// Check service cache (memory + disk) and convert to texture
	if data, ok := a.service.GetCached(domain); ok && len(data) > 0 {
		texture := bytesToTexture(data)
		if texture != nil {
			a.setTexture(domain, texture)
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

	// Try WebKit FaviconDatabase if available
	if a.faviconDB != nil {
		asyncCb := gio.AsyncReadyCallback(func(_ uintptr, resultPtr uintptr, _ uintptr) {
			if resultPtr == 0 {
				log.Debug().Str("domain", domain).Msg("favicon not in webkit db, fetching via service")
				fetchViaService()
				return
			}

			result := &gio.AsyncResultBase{Ptr: resultPtr}
			texture, err := a.faviconDB.GetFaviconFinish(result)
			if err != nil || texture == nil {
				log.Debug().Str("domain", domain).Msg("favicon not in webkit db, fetching via service")
				fetchViaService()
				return
			}

			// Found in WebKit database
			a.setTexture(domain, texture)
			invokeCallback(texture)

			// Ensure disk cache is populated (async)
			go a.service.EnsureDiskCache(ctx, domain)
		})

		a.faviconDB.GetFavicon(pageURL, nil, &asyncCb, 0)
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

	// Ensure disk cache is populated (async, in background)
	go a.service.EnsureDiskCache(ctx, domain)
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
			go a.service.EnsureDiskCache(ctx, originDomain)
		}
	}
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
func (a *FaviconAdapter) Service() *favicon.Service {
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

// Package services contains application services that orchestrate business logic.
package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
)

// FaviconService provides unified favicon access using WebKitGTK's native database.
// The service keeps WebKit's FaviconDatabase as the source of truth and augments it
// with fallback rasterisation for SVG favicons so that callers always receive a
// 32x32 texture or an explicit error.
type FaviconService struct {
	faviconDB    *webkit.FaviconDatabase
	dbQueries    db.DatabaseQuerier
	targetSize   int    // Target size for exported favicons (32x32 for high quality)
	exportDir    string // Directory for exporting favicons for CLI access
	dataDir      string // WebKit data directory
	enableExport bool   // Whether to export favicons for CLI tools
}

const (
	// DefaultFaviconSize is the target size for exported favicons (32x32 for high quality)
	DefaultFaviconSize = 32
	// FaviconTimeout is the maximum time to wait for a favicon to be available
	FaviconTimeout = 5 * time.Second
)

// ServiceName returns the service name for identification
func (fs *FaviconService) ServiceName() string {
	return "FaviconService"
}

// NewFaviconService creates a new favicon service
func NewFaviconService(faviconDB *webkit.FaviconDatabase, queries db.DatabaseQuerier, dataDir string) (*FaviconService, error) {
	if faviconDB == nil {
		return nil, fmt.Errorf("favicon database cannot be nil")
	}
	if queries == nil {
		return nil, fmt.Errorf("database queries cannot be nil")
	}

	// Create export directory for CLI access
	exportDir := filepath.Join(dataDir, "favicons-export")
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create favicon export directory: %w", err)
	}

	fs := &FaviconService{
		faviconDB:    faviconDB,
		dbQueries:    queries,
		targetSize:   DefaultFaviconSize,
		exportDir:    exportDir,
		dataDir:      dataDir,
		enableExport: true,
	}

	// Register the favicon changed handler ONCE at the service level
	// This is critical to avoid duplicate handlers when multiple WebViews exist
	// The FaviconDatabase is shared across all WebViews (at NetworkSession level)
	faviconDB.ConnectFaviconChanged(func(pageURI, faviconURI string) {
		if err := fs.OnFaviconChanged(pageURI, faviconURI); err != nil {
			log.Printf("[favicon] Handler error for %s: %v", pageURI, err)
		}
	})

	log.Printf("[favicon] Registered single shared favicon handler at FaviconService level")

	return fs, nil
}

// OnFaviconChanged handles favicon URI changes from WebKit's favicon database.
// We store the URI (PNG, SVG, etc.) so that callers can later load an appropriate texture.
func (fs *FaviconService) OnFaviconChanged(pageURL, faviconURI string) error {
	log.Printf("[favicon] Favicon changed for %s: %s", pageURL, faviconURI)

	ctx := context.Background()

	if !fs.shouldProcessURL(pageURL) {
		return nil
	}

	nullString := sql.NullString{String: faviconURI, Valid: faviconURI != ""}
	if err := fs.dbQueries.UpdateHistoryFavicon(ctx, nullString, pageURL); err != nil {
		log.Printf("[favicon] Failed to update favicon URI in database for %s: %v", pageURL, err)
		return fmt.Errorf("failed to update favicon in database: %w", err)
	}

	// Also update all other URLs from the same domain to propagate favicon
	// This handles multiple paths on the same domain (e.g., google.com/search?q=foo and google.com/search?q=bar)
	fs.propagateFaviconToDomain(pageURL, faviconURI)

	// Proactively render and export favicon for omnibox/CLI use
	// This ensures favicons are available ASAP for suggestions
	if fs.enableExport {
		go fs.preloadFavicon(pageURL, faviconURI)
	}

	return nil
}

// preloadFavicon renders and exports a favicon in the background
// This is called immediately when OnFaviconChanged fires to ensure
// favicons are ready for omnibox suggestions as soon as possible
// propagateFaviconToDomain updates all history entries from the same domain
// with the favicon. This ensures multiple paths on the same domain share the favicon
// even if WebKit only signals it once per domain (KISS + DRY principle).
func (fs *FaviconService) propagateFaviconToDomain(pageURL, faviconURI string) {
	ctx := context.Background()

	parsedURL, err := webkit.ParseURL(pageURL)
	if err != nil {
		return // Invalid URL, skip propagation
	}

	faviconNull := sql.NullString{String: faviconURI, Valid: true}

	// Search for exact domain match first
	domainPattern := sql.NullString{String: "%://" + parsedURL.Host + "%", Valid: true}
	entries, err := fs.dbQueries.SearchHistory(ctx, domainPattern, sql.NullString{}, 1000)
	if err != nil {
		log.Printf("[favicon] Failed to search history for domain %s: %v", parsedURL.Host, err)
		return
	}

	// Also search for www variant to handle www/non-www redirects
	// google.com should share favicon with www.google.com and vice versa
	var altPattern sql.NullString
	if len(parsedURL.Host) > 4 && parsedURL.Host[:4] == "www." {
		// Current is www.google.com, also search google.com
		hostWithoutWWW := parsedURL.Host[4:]
		altPattern = sql.NullString{String: "%://" + hostWithoutWWW + "%", Valid: true}
	} else {
		// Current is google.com, also search www.google.com
		altPattern = sql.NullString{String: "%://www." + parsedURL.Host + "%", Valid: true}
	}

	altEntries, _ := fs.dbQueries.SearchHistory(ctx, altPattern, sql.NullString{}, 1000)
	entries = append(entries, altEntries...)

	// Update all entries that don't already have a favicon
	for _, entry := range entries {
		if entry.FaviconUrl.String == "" {
			if err := fs.dbQueries.UpdateHistoryFavicon(ctx, faviconNull, entry.Url); err != nil {
				log.Printf("[favicon] Failed to propagate favicon to %s: %v", entry.Url, err)
			} else {
				log.Printf("[favicon] Propagated favicon to %s", entry.Url)
			}
		}
	}
}

func (fs *FaviconService) preloadFavicon(pageURL, faviconURI string) {
	ctx, cancel := context.WithTimeout(context.Background(), FaviconTimeout)
	defer cancel()

	texture, err := fs.renderFaviconTexture(ctx, pageURL, faviconURI)
	if err != nil {
		log.Printf("[favicon] Preload failed for %s: %v", pageURL, err)
		return
	}

	fs.maybeExportTexture(pageURL, texture)
	log.Printf("[favicon] Preloaded and exported favicon for %s", pageURL)
}

// GetFaviconTexture loads a favicon texture asynchronously and calls the callback with the result.
// The callback receives a 32x32 GdkTexture suitable for display in GTK widgets, or an error.
func (fs *FaviconService) GetFaviconTexture(pageURL string, callback func(*gdk.Texture, error)) {
	if callback == nil {
		return
	}
	if pageURL == "" {
		fs.dispatchTexture(callback, nil, fmt.Errorf("page URL is empty"))
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), FaviconTimeout)
		defer cancel()

		faviconURI, err := fs.lookupFaviconURI(ctx, pageURL)
		if err != nil {
			fs.dispatchTexture(callback, nil, err)
			return
		}

		texture, err := fs.renderFaviconTexture(ctx, pageURL, faviconURI)
		if err != nil {
			fs.dispatchTexture(callback, nil, err)
			return
		}

		fs.maybeExportTexture(pageURL, texture)

		fs.dispatchTexture(callback, texture, nil)
	}()
}

func (fs *FaviconService) renderFaviconTexture(ctx context.Context, pageURL, faviconURI string) (*gdk.Texture, error) {
	if faviconURI == "" {
		return nil, fmt.Errorf("no favicon URI available")
	}

	if fs.isSVGFavicon(faviconURI) {
		return fs.loadSVGFaviconTexture(ctx, faviconURI)
	}

	if fs.faviconDB == nil {
		return nil, fmt.Errorf("favicon database not initialized")
	}

	type result struct {
		texture *gdk.Texture
		err     error
	}

	resultCh := make(chan result, 1)

	fs.faviconDB.Favicon(ctx, pageURL, func(res gio.AsyncResulter) {
		if res == nil {
			select {
			case resultCh <- result{err: fmt.Errorf("favicon async result is nil")}:
			default:
			}
			return
		}

		texture, err := webkit.FaviconFinishSafe(fs.faviconDB, res)
		if err == nil && texture == nil {
			err = fmt.Errorf("favicon unavailable")
		}
		if err == nil {
			texture = fs.ensureTextureSize(texture)
			if texture == nil {
				err = fmt.Errorf("failed to scale favicon texture")
			}
		}

		select {
		case resultCh <- result{texture: texture, err: err}:
		default:
		}
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultCh:
		return res.texture, res.err
	}
}

func (fs *FaviconService) loadSVGFaviconTexture(ctx context.Context, faviconURI string) (*gdk.Texture, error) {
	file := gio.NewFileForURI(faviconURI)
	if file == nil {
		return nil, fmt.Errorf("invalid SVG favicon URI: %s", faviconURI)
	}

	data, _, err := file.LoadContents(ctx)
	if err != nil {
		return nil, fmt.Errorf("load SVG favicon: %w", err)
	}

	texture, err := webkit.TextureFromImageBytes(data)
	if err != nil {
		return nil, fmt.Errorf("decode SVG favicon: %w", err)
	}

	texture = fs.ensureTextureSize(texture)
	if texture == nil {
		return nil, fmt.Errorf("failed to scale SVG favicon")
	}

	return texture, nil
}

func (fs *FaviconService) lookupFaviconURI(ctx context.Context, pageURL string) (string, error) {
	entry, err := fs.dbQueries.GetHistoryEntry(ctx, pageURL)
	if err == nil {
		if entry.FaviconUrl.Valid && entry.FaviconUrl.String != "" {
			return entry.FaviconUrl.String, nil
		}
	} else if err != sql.ErrNoRows {
		log.Printf("[favicon] failed to query history entry for %s: %v", pageURL, err)
	}

	if fs.faviconDB != nil {
		if uri := fs.faviconDB.FaviconURI(pageURL); uri != "" {
			return uri, nil
		}
	}

	return "", fmt.Errorf("no favicon URI available for %s", pageURL)
}

func (fs *FaviconService) ensureTextureSize(texture *gdk.Texture) *gdk.Texture {
	if texture == nil {
		return nil
	}

	if texture.Width() == fs.targetSize && texture.Height() == fs.targetSize {
		return texture
	}

	return webkit.ScaleTexture(texture, fs.targetSize, fs.targetSize)
}

func (fs *FaviconService) dispatchTexture(callback func(*gdk.Texture, error), texture *gdk.Texture, err error) {
	_ = webkit.IdleAdd(func() bool {
		callback(texture, err)
		return false
	})
}

func (fs *FaviconService) maybeExportTexture(pageURL string, texture *gdk.Texture) {
	if !fs.enableExport || texture == nil {
		return
	}

	exportPath := fs.getExportPath(pageURL)

	_ = webkit.IdleAdd(func() bool {
		if err := webkit.SaveTextureAsPNG(texture, exportPath, fs.targetSize); err != nil {
			log.Printf("[favicon] Failed to export favicon PNG for %s: %v", pageURL, err)
		}
		return false
	})
}

// GetFaviconPath returns the exported PNG path for CLI tools (dmenu, etc.)
// This allows CLI tools to display favicons without WebKitGTK API access.
func (fs *FaviconService) GetFaviconPath(pageURL string) (string, error) {
	exportPath := fs.getExportPath(pageURL)
	if info, err := os.Stat(exportPath); err == nil {
		if time.Since(info.ModTime()) < 7*24*time.Hour {
			return exportPath, nil
		}
	}

	if !fs.enableExport {
		return "", fmt.Errorf("favicon export disabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), FaviconTimeout)
	defer cancel()

	faviconURI, err := fs.lookupFaviconURI(ctx, pageURL)
	if err != nil {
		return "", err
	}

	texture, err := fs.renderFaviconTexture(ctx, pageURL, faviconURI)
	if err != nil {
		return "", err
	}

	if err := webkit.SaveTextureAsPNG(texture, exportPath, fs.targetSize); err != nil {
		return "", fmt.Errorf("failed to save favicon PNG: %w", err)
	}

	return exportPath, nil
}

// getExportPath generates the export path for a page URL
func (fs *FaviconService) getExportPath(pageURL string) string {
	// Use the same hashing scheme as the old cache for compatibility
	hash := fmt.Sprintf("%x", webkit.HashURL(pageURL))
	return filepath.Join(fs.exportDir, hash+".png")
}

// shouldProcessURL checks if a URL should have its favicon processed
func (fs *FaviconService) shouldProcessURL(pageURL string) bool {
	parsedURL, err := webkit.ParseURL(pageURL)
	if err != nil {
		log.Printf("[favicon] Invalid page URL: %s", pageURL)
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}
	if parsedURL.Host == "localhost" || parsedURL.Host == "127.0.0.1" {
		return false
	}

	return true
}

// isSVGFavicon checks if a favicon URI points to an SVG file.
func (fs *FaviconService) isSVGFavicon(faviconURI string) bool {
	if faviconURI == "" {
		return false
	}

	parsedURL, err := webkit.ParseURL(faviconURI)
	if err != nil {
		return false
	}

	path := parsedURL.Path
	if len(path) >= 4 {
		ext := path[len(path)-4:]
		if ext == ".svg" || ext == ".SVG" {
			return true
		}
	}

	return false
}

// CleanOldExports removes exported favicons older than 30 days
func (fs *FaviconService) CleanOldExports() error {
	entries, err := os.ReadDir(fs.exportDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(fs.exportDir, entry.Name()))
		}
	}

	return nil
}

// GetExportedFaviconPath returns the expected export path for a page URL.
func GetExportedFaviconPath(dataDir, pageURL string) string {
	exportDir := filepath.Join(dataDir, "webkit", "favicons-export")
	hash := fmt.Sprintf("%x", webkit.HashURL(pageURL))
	return filepath.Join(exportDir, hash+".png")
}

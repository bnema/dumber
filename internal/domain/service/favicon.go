// Package service defines domain service interfaces.
package service

import "context"

// FaviconService provides favicon retrieval and caching operations.
// It works with raw bytes, leaving texture conversion to the UI layer.
type FaviconService interface {
	// Get returns favicon bytes for a domain.
	// Checks memory cache, then disk cache, then fetches from external API.
	Get(ctx context.Context, domain string) ([]byte, error)

	// GetCached returns favicon bytes only if already cached (no external fetch).
	// Returns the bytes and true if found, nil and false otherwise.
	GetCached(domain string) ([]byte, bool)

	// Store saves favicon bytes for a domain to cache.
	Store(domain string, data []byte) error

	// DiskPath returns the filesystem path where a domain's favicon is cached (ICO).
	DiskPath(domain string) string

	// DiskPathPNG returns the filesystem path for PNG favicon.
	// PNG format is required by CLI tools like rofi/fuzzel.
	DiskPathPNG(domain string) string

	// HasPNGOnDisk checks if a PNG favicon exists on disk for the domain.
	HasPNGOnDisk(domain string) bool

	// WritePNG writes raw PNG data to disk for a domain.
	// Used by UI layer to export WebKit textures for CLI tools.
	WritePNG(domain string, pngData []byte)

	// DiskPathPNGSized returns the filesystem path for a sized PNG favicon.
	// Used for normalized favicon export (e.g., 32x32) for CLI tools.
	DiskPathPNGSized(domain string, size int) string

	// HasPNGSizedOnDisk checks if a sized PNG favicon exists on disk.
	HasPNGSizedOnDisk(domain string, size int) bool

	// EnsureSizedPNG creates a resized PNG from the original if it doesn't exist.
	// This is used to generate normalized icons for dmenu/fuzzel.
	EnsureSizedPNG(ctx context.Context, domain string, size int) error

	// Close shuts down background workers and releases resources.
	Close()
}

package download

import (
	"fmt"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultFilename is used when no valid filename can be determined.
	DefaultFilename = "download"
)

// SanitizeFilename sanitizes a filename to prevent path traversal attacks.
// It extracts only the base name and handles edge cases like "." or "..".
func SanitizeFilename(name string) string {
	// Normalize Windows-style separators to forward slashes.
	// filepath.Base only handles the OS-native separator, so on Linux
	// backslashes would not be treated as path separators.
	name = strings.ReplaceAll(name, "\\", "/")

	// Get only the base name (removes any directory components).
	clean := filepath.Base(name)

	// If Base returns "." or ".." (edge cases), use fallback.
	if clean == "." || clean == ".." || clean == "" {
		return DefaultFilename
	}

	return clean
}

// SanitizeFilenameWithExtension sanitizes a filename and adds an extension
// inferred from the MIME type if the filename has no extension.
func SanitizeFilenameWithExtension(name, mimeType string) string {
	clean := SanitizeFilename(name)
	if filepath.Ext(clean) == "" {
		if ext := GetExtensionFromMimeType(mimeType); ext != "" {
			return clean + ext
		}
	}

	return clean
}

// GetExtensionFromMimeType returns a file extension for a given MIME type.
// Returns empty string if MIME type is unknown or empty.
func GetExtensionFromMimeType(mimeType string) string {
	if mimeType == "" {
		return ""
	}

	exts, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(exts) == 0 {
		return ""
	}

	return exts[0]
}

// ExtractFilenameFromURI extracts the filename from a URI path component.
// Handles both URIs and plain paths. Returns DefaultFilename for edge cases.
func ExtractFilenameFromURI(uri string) string {
	if uri == "" {
		return DefaultFilename
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		// Fall back to treating as plain path
		return extractFromPath(uri)
	}

	return extractFromPath(parsed.Path)
}

// ExtractFilenameFromDestination extracts the filename from a file:// URI or path.
// Used to get filename from WebKit download destination.
func ExtractFilenameFromDestination(dest string) string {
	// Remove file:// prefix if present.
	path := strings.TrimPrefix(dest, "file://")
	base := filepath.Base(path)

	// Handle edge cases consistently with SanitizeFilename.
	if base == "." || base == "" {
		return DefaultFilename
	}
	return base
}

// extractFromPath extracts filename from a path string.
func extractFromPath(path string) string {
	base := filepath.Base(path)
	if base == "." || base == "" || base == "/" {
		return DefaultFilename
	}
	return base
}

// MakeUniqueFilename generates a unique filename by appending _(N) if needed.
// The exists function should return true if the given path already exists.
func MakeUniqueFilename(dir, filename string, exists func(path string) bool) string {
	destPath := filepath.Join(dir, filename)
	if !exists(destPath) {
		return filename
	}

	// Split filename into base and extension
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)

	// Try incrementing numbers until we find a unique name
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_(%d)%s", base, i, ext)
		candidatePath := filepath.Join(dir, candidate)
		if !exists(candidatePath) {
			return candidate
		}
	}

	// Fallback: use timestamp
	candidate := fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
	return candidate
}

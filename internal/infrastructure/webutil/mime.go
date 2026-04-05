package webutil

import (
	"mime"
	"path/filepath"
	"strings"
)

// GetMimeType determines the MIME type for a given file path.
// It tries the standard library first, then falls back to common web asset types.
func GetMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	mt := mime.TypeByExtension(ext)
	if mt != "" {
		return mt
	}

	switch ext {
	case ".js", ".mjs":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".m3u8":
		return "application/vnd.apple.mpegurl" // RFC 8216
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	default:
		return "text/plain"
	}
}

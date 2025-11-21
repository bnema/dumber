package schemes

import (
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// Scheme name constants.
const (
	SchemeDumb          = "dumb"
	SchemeDumbExtension = "dumb-extension"
)

// FinishRequestWithData completes a URI scheme request with in-memory data.
func FinishRequestWithData(req *webkit.URISchemeRequest, mimeType string, data []byte) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	gbytes := glib.NewBytes(data)
	stream := gio.NewMemoryInputStreamFromBytes(gbytes)
	if stream == nil {
		err := fmt.Errorf("failed to create input stream")
		req.FinishError(err)
		return err
	}

	req.Finish(stream, int64(len(data)), mimeType)
	return nil
}

// GuessMimeType returns a reasonable MIME type for the given filename.
// Falls back to application/octet-stream when unknown.
func GuessMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if mt := mime.TypeByExtension(ext); mt != "" {
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
	case ".eot":
		return "application/vnd.ms-fontobject"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".wasm":
		return "application/wasm"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

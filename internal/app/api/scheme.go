package api

import (
	"embed"
	"fmt"
	"mime"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/services"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	gio "github.com/diamondburned/gotk4/pkg/gio/v2"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
)

// SchemeHandler handles custom dumb:// scheme resolution
type SchemeHandler struct {
	assets         embed.FS
	parserService  *services.ParserService
	browserService *services.BrowserService
	cfg            *config.Config
}

// NewSchemeHandler creates a new scheme handler
func NewSchemeHandler(
	assets embed.FS,
	parserService *services.ParserService,
	browserService *services.BrowserService,
) *SchemeHandler {
	return &SchemeHandler{
		assets:         assets,
		parserService:  parserService,
		browserService: browserService,
	}
}

// SetConfig sets the configuration for the scheme handler
func (s *SchemeHandler) SetConfig(cfg *config.Config) {
	s.cfg = cfg
}

// Handle processes dumb:// scheme requests using the new URISchemeRequest API
func (s *SchemeHandler) Handle(req *webkit.URISchemeRequest) {
	uri := req.URI()
	logging.Debug(fmt.Sprintf("[scheme] request: %s", uri))

	// Known forms:
	// - dumb://homepage or dumb:homepage → index.html
	// - dumb://app/index.html, dumb://app/<path> → serve from gui/<path>
	// - dumb://favicon/<hash>.png → serve cached favicon
	// - dumb://<anything> without path → index.html
	u, err := neturl.Parse(uri)
	if err != nil || u.Scheme != "dumb" {
		req.FinishError(fmt.Errorf("invalid URI: %s", uri))
		return
	}

	// Check if this is a favicon request
	if u.Host == "favicon" || (u.Host == "" && strings.HasPrefix(u.Path, "/favicon/")) {
		s.handleFavicon(req, u)
		return
	}

	// Static assets
	s.handleAsset(req, u)
}

// handleAsset serves static assets from embedded filesystem
func (s *SchemeHandler) handleAsset(req *webkit.URISchemeRequest, u *neturl.URL) {
	// Resolve target path inside embed FS
	var rel string
	if u.Opaque == constants.HomepagePath || (u.Host == constants.HomepagePath && (u.Path == "" || u.Path == "/")) || (u.Host == "" && (u.Path == "" || u.Path == "/")) {
		rel = constants.IndexHTML
	} else if u.Host == constants.BlockedPath && (u.Path == "" || u.Path == "/") {
		// dumb://blocked → blocked.html
		rel = constants.BlockedHTML
	} else {
		host := u.Host
		p := strings.TrimPrefix(u.Path, "/")
		switch {
		case host == "app" && p == "":
			rel = constants.IndexHTML
		case host == "app":
			rel = p
		case host == constants.HomepagePath && p != "":
			// dumb://homepage/<asset>
			rel = p
		case host == constants.BlockedPath && p != "":
			// dumb://blocked/<asset>
			rel = p
		case p != "":
			rel = p
		default:
			rel = constants.IndexHTML
		}
	}

	// Special-case homepage favicon: map .ico request to embedded PNG file
	if (u.Host == constants.HomepagePath || u.Opaque == constants.HomepagePath) && strings.EqualFold(rel, "favicon.ico") {
		logging.Debug(fmt.Sprintf("[scheme] asset: rel=%s (host=%s path=%s) → mapping to favicon.png", rel, u.Host, u.Path))
		data, rerr := s.assets.ReadFile(filepath.ToSlash(filepath.Join("assets", "gui", "favicon.png")))
		if rerr == nil {
			s.finishRequest(req, "image/png", data, "favicon.png")
			return
		}
	}

	logging.Debug(fmt.Sprintf("[scheme] asset: rel=%s (host=%s path=%s)", rel, u.Host, u.Path))

	// Try to read the requested asset
	data, rerr := s.assets.ReadFile(filepath.ToSlash(filepath.Join("assets", "gui", rel)))
	if rerr != nil {
		logging.Debug(fmt.Sprintf("[scheme] not found: %s", rel))
		req.FinishError(fmt.Errorf("asset not found: %s", rel))
		return
	}

	// Determine mime type
	mt := s.getMimeType(rel)

	// Finish the request with the data
	logging.Debug(fmt.Sprintf("[scheme] serving %s with mime-type: %s", rel, mt))
	s.finishRequest(req, mt, data, rel)
}

// finishRequest completes a URI scheme request with the provided data
func (s *SchemeHandler) finishRequest(req *webkit.URISchemeRequest, mimeType string, data []byte, filename string) {
	// Convert byte slice to GLib Bytes
	gbytes := glib.NewBytes(data)

	// Create an input stream from the bytes
	stream := gio.NewMemoryInputStreamFromBytes(gbytes)
	if stream == nil {
		req.FinishError(fmt.Errorf("failed to create input stream for: %s", filename))
		logging.Error(fmt.Sprintf("[scheme] failed to create stream for: %s", filename))
		return
	}

	// Finish the request with the stream
	contentLength := int64(len(data))
	req.Finish(stream, contentLength, mimeType)
}

// getMimeType determines the MIME type for a given file path
func (s *SchemeHandler) getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	// Try standard mime type first
	mt := mime.TypeByExtension(ext)
	if mt != "" {
		return mt
	}

	// Fallbacks for common web assets
	switch ext {
	case ".js":
		return "application/javascript"
	case ".mjs":
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
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	default:
		// Default to text/plain for unknown extensions
		return "text/plain"
	}
}

// handleFavicon serves cached favicon files
func (s *SchemeHandler) handleFavicon(req *webkit.URISchemeRequest, u *neturl.URL) {
	// Extract the filename from the URL
	// URL format: dumb://favicon/<hash>.png
	var filename string
	if u.Host == "favicon" {
		filename = strings.TrimPrefix(u.Path, "/")
	} else {
		filename = strings.TrimPrefix(u.Path, "/favicon/")
	}

	if filename == "" {
		logging.Debug(fmt.Sprintf("[scheme] favicon: empty filename"))
		req.FinishError(fmt.Errorf("invalid favicon path"))
		return
	}

	// Get the favicon cache directory path
	dataDir, err := config.GetDataDir()
	if err != nil {
		logging.Error(fmt.Sprintf("[scheme] favicon: failed to get data directory: %v", err))
		req.FinishError(fmt.Errorf("failed to get data directory"))
		return
	}

	faviconPath := filepath.Join(dataDir, "favicons", filename)

	// Read the favicon file
	data, err := os.ReadFile(faviconPath)
	if err != nil {
		logging.Debug(fmt.Sprintf("[scheme] favicon: file not found: %s", faviconPath))
		req.FinishError(fmt.Errorf("favicon not found: %s", filename))
		return
	}

	logging.Debug(fmt.Sprintf("[scheme] favicon: serving %s (%d bytes)", filename, len(data)))
	s.finishRequest(req, "image/png", data, filename)
}

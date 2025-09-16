package api

import (
	"embed"
	"log"
	"mime"
	neturl "net/url"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/services"
)

// SchemeHandler handles custom dumb:// scheme resolution
type SchemeHandler struct {
	assets         embed.FS
	parserService  *services.ParserService
	browserService *services.BrowserService
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

// Handle processes dumb:// scheme requests
func (s *SchemeHandler) Handle(uri string, cfg *config.Config) (string, []byte, bool) {
	log.Printf("[scheme] request: %s", uri)
	// Known forms:
	// - dumb://homepage or dumb:homepage → index.html
	// - dumb://app/index.html, dumb://app/<path> → serve from gui/<path>
	// - dumb://<anything> without path → index.html
	u, err := neturl.Parse(uri)
	if err != nil || u.Scheme != "dumb" {
		return "", nil, false
	}

	// Static assets
	return s.handleAsset(u)
}

// handleAsset serves static assets from embedded filesystem
func (s *SchemeHandler) handleAsset(u *neturl.URL) (string, []byte, bool) {
	// Resolve target path inside embed FS
	var rel string
	if u.Opaque == constants.HomepagePath || (u.Host == constants.HomepagePath && (u.Path == "" || u.Path == "/")) || (u.Host == "" && (u.Path == "" || u.Path == "/")) {
		rel = constants.IndexHTML
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
		case p != "":
			rel = p
		default:
			rel = constants.IndexHTML
		}
	}

	// Special-case homepage favicon: map .ico request to embedded SVG file
	if (u.Host == constants.HomepagePath || u.Opaque == constants.HomepagePath) && strings.EqualFold(rel, "favicon.ico") {
		log.Printf("[scheme] asset: rel=%s (host=%s path=%s) → mapping to favicon.svg", rel, u.Host, u.Path)
		data, rerr := s.assets.ReadFile(filepath.ToSlash(filepath.Join("assets", "gui", "favicon.svg")))
		if rerr == nil {
			return "image/svg+xml", data, true
		}
	}

	log.Printf("[scheme] asset: rel=%s (host=%s path=%s)", rel, u.Host, u.Path)
	data, rerr := s.assets.ReadFile(filepath.ToSlash(filepath.Join("assets", "gui", rel)))
	if rerr != nil {
		log.Printf("[scheme] not found: %s", rel)
		return "", nil, false
	}

	// Determine mime type
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(rel)))
	if mt == "" {
		// Fallbacks
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".js":
			mt = "application/javascript"
		case ".css":
			mt = "text/css"
		case ".svg":
			mt = "image/svg+xml"
		case ".ico":
			mt = "image/x-icon"
		default:
			mt = "text/html; charset=utf-8"
		}
	}
	return mt, data, true
}

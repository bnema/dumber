package schemes

import (
	"embed"
	"fmt"
	"log"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/services"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

// APIHandler handles the dumb:// scheme for internal browser assets.
type APIHandler struct {
	assets         embed.FS
	parserService  *services.ParserService
	browserService *services.BrowserService
	cfg            *config.Config
}

// NewAPIHandler creates a new API scheme handler.
func NewAPIHandler(
	assets embed.FS,
	parserService *services.ParserService,
	browserService *services.BrowserService,
	cfg *config.Config,
) *APIHandler {
	return &APIHandler{
		assets:         assets,
		parserService:  parserService,
		browserService: browserService,
		cfg:            cfg,
	}
}

// Handle processes dumb:// scheme requests using the new URISchemeRequest API.
func (h *APIHandler) Handle(req *webkit.URISchemeRequest) {
	uri := req.URI()
	log.Printf("[scheme] request: %s", uri)

	// Known forms:
	// - dumb://homepage or dumb:homepage → index.html
	// - dumb://app/index.html, dumb://app/<path> → serve from gui/<path>
	// - dumb://favicon/<hash>.png → serve cached favicon
	// - dumb://<anything> without path → index.html
	u, err := neturl.Parse(uri)
	if err != nil || u.Scheme != SchemeDumb {
		req.FinishError(fmt.Errorf("invalid URI: %s", uri))
		return
	}

	// Check if this is a favicon request
	if u.Host == "favicon" || (u.Host == "" && strings.HasPrefix(u.Path, "/favicon/")) {
		h.handleFavicon(req, u)
		return
	}

	// Static assets
	h.handleAsset(req, u)
}

// handleAsset serves static assets from embedded filesystem.
func (h *APIHandler) handleAsset(req *webkit.URISchemeRequest, u *neturl.URL) {
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

	// Special-case homepage favicon: map .ico request to embedded PNG file
	if (u.Host == constants.HomepagePath || u.Opaque == constants.HomepagePath) && strings.EqualFold(rel, "favicon.ico") {
		log.Printf("[scheme] asset: rel=%s (host=%s path=%s) → mapping to favicon.png", rel, u.Host, u.Path)
		data, rerr := h.assets.ReadFile(filepath.ToSlash(filepath.Join("assets", "gui", "favicon.png")))
		if rerr == nil {
			if err := FinishRequestWithData(req, "image/png", data); err != nil {
				log.Printf("[scheme] failed to finish request for favicon: %v", err)
			}
			return
		}
	}

	log.Printf("[scheme] asset: rel=%s (host=%s path=%s)", rel, u.Host, u.Path)

	// Try to read the requested asset
	data, rerr := h.assets.ReadFile(filepath.ToSlash(filepath.Join("assets", "gui", rel)))
	if rerr != nil {
		log.Printf("[scheme] not found: %s", rel)
		req.FinishError(fmt.Errorf("asset not found: %s", rel))
		return
	}

	// Determine mime type
	mt := GuessMimeType(rel)

	// Finish the request with the data
	log.Printf("[scheme] serving %s with mime-type: %s", rel, mt)
	if err := FinishRequestWithData(req, mt, data); err != nil {
		log.Printf("[scheme] failed to finish request for %s: %v", rel, err)
	}
}

// handleFavicon serves cached favicon files.
func (h *APIHandler) handleFavicon(req *webkit.URISchemeRequest, u *neturl.URL) {
	// Extract the filename from the URL
	// URL format: dumb://favicon/<hash>.png
	var filename string
	if u.Host == "favicon" {
		filename = strings.TrimPrefix(u.Path, "/")
	} else {
		filename = strings.TrimPrefix(u.Path, "/favicon/")
	}

	if filename == "" {
		log.Printf("[scheme] favicon: empty filename")
		req.FinishError(fmt.Errorf("invalid favicon path"))
		return
	}

	// Get the favicon cache directory path
	dataDir, err := config.GetDataDir()
	if err != nil {
		log.Printf("[scheme] favicon: failed to get data directory: %v", err)
		req.FinishError(fmt.Errorf("failed to get data directory"))
		return
	}

	faviconPath := filepath.Join(dataDir, "favicons", filename)

	// Read the favicon file
	data, readErr := os.ReadFile(faviconPath)
	if readErr != nil {
		log.Printf("[scheme] favicon: file not found: %s", faviconPath)
		req.FinishError(fmt.Errorf("favicon not found: %s", filename))
		return
	}

	log.Printf("[scheme] favicon: serving %s (%d bytes)", filename, len(data))
	if err := FinishRequestWithData(req, "image/png", data); err != nil {
		log.Printf("[scheme] failed to finish favicon request for %s: %v", filename, err)
	}
}

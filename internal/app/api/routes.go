package api

import (
	"context"
	"encoding/json"
	"log"
	neturl "net/url"
	"strconv"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/services"
)

// Handler provides API endpoint handling functionality
type Handler struct {
	browserService *services.BrowserService
}

// NewHandler creates a new API handler
func NewHandler(browserService *services.BrowserService) *Handler {
	return &Handler{
		browserService: browserService,
	}
}

// HandleRenderingStatus handles GET /rendering/status endpoint
func (h *Handler) HandleRenderingStatus(cfg *config.Config) (string, []byte, bool) {
	log.Printf("[api] GET /rendering/status")
	// Report configured rendering mode; runtime GPU state depends on WebKit internals
	resp := struct {
		Mode string `json:"mode"`
	}{Mode: string(cfg.RenderingMode)}
	b, _ := json.Marshal(resp)
	return constants.ContentTypeJSON, b, true
}

// HandleConfig handles GET /config endpoint
func (h *Handler) HandleConfig(cfg *config.Config) (string, []byte, bool) {
	log.Printf("[api] GET /config")
	// Build config info
	cfgPath, _ := config.GetConfigFile()
	info := struct {
		ConfigPath      string                           `json:"config_path"`
		DatabasePath    string                           `json:"database_path"`
		SearchShortcuts map[string]config.SearchShortcut `json:"search_shortcuts"`
		Appearance      config.AppearanceConfig          `json:"appearance"`
	}{
		ConfigPath:      cfgPath,
		DatabasePath:    cfg.Database.Path,
		SearchShortcuts: cfg.SearchShortcuts,
		Appearance:      cfg.Appearance,
	}
	b, _ := json.Marshal(info)
	return constants.ContentTypeJSON, b, true
}

// HandleHistoryRecent handles GET /history/recent endpoint
func (h *Handler) HandleHistoryRecent(u *neturl.URL) (string, []byte, bool) {
	log.Printf("[api] GET /history/recent%s", u.RawQuery)
	// Parse limit
	q := u.Query()
	limit := 50
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	ctx := context.Background()
	entries, err := h.browserService.GetRecentHistory(ctx, limit)
	if err != nil {
		return constants.ContentTypeJSON, []byte("[]"), true
	}
	b, _ := json.Marshal(entries)
	return constants.ContentTypeJSON, b, true
}

// HandleHistorySearch handles GET /history/search endpoint
func (h *Handler) HandleHistorySearch(u *neturl.URL) (string, []byte, bool) {
	log.Printf("[api] GET /history/search%s", u.RawQuery)
	q := u.Query()
	query := q.Get("q")
	limit := 50
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	ctx := context.Background()
	entries, err := h.browserService.SearchHistory(ctx, query, limit)
	if err != nil {
		return constants.ContentTypeJSON, []byte("[]"), true
	}
	b, _ := json.Marshal(entries)
	return constants.ContentTypeJSON, b, true
}

// HandleHistoryStats handles GET /history/stats endpoint
func (h *Handler) HandleHistoryStats() (string, []byte, bool) {
	log.Printf("[api] GET /history/stats")
	ctx := context.Background()
	stats, err := h.browserService.GetHistoryStats(ctx)
	if err != nil {
		return constants.ContentTypeJSON, []byte("{}"), true
	}
	b, _ := json.Marshal(stats)
	return constants.ContentTypeJSON, b, true
}

// HandleDefault returns empty JSON for unknown endpoints
func (h *Handler) HandleDefault() (string, []byte, bool) {
	return constants.ContentTypeJSON, []byte("{}"), true
}

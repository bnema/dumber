package webkit

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/soup"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/rs/zerolog"
)

// Scheme path constants
const (
	HomePath   = "home"
	ConfigPath = "config"
	ErrorPath  = "error"
	IndexHTML  = "index.html"
)

// SchemeRequest represents a request to a custom URI scheme.
type SchemeRequest struct {
	inner  *webkit.URISchemeRequest
	URI    string
	Path   string
	Method string
	Scheme string
}

// SchemeResponse represents a response to a scheme request.
type SchemeResponse struct {
	Data        []byte
	ContentType string
	StatusCode  int
}

// PageHandler generates content for a specific page path.
type PageHandler interface {
	Handle(req *SchemeRequest) *SchemeResponse
}

// PageHandlerFunc is an adapter to allow use of ordinary functions as PageHandlers.
type PageHandlerFunc func(req *SchemeRequest) *SchemeResponse

func (f PageHandlerFunc) Handle(req *SchemeRequest) *SchemeResponse {
	return f(req)
}

// DumbSchemeHandler handles dumb:// URI scheme requests.
type DumbSchemeHandler struct {
	handlers   map[string]PageHandler
	assets     embed.FS
	assetDir   string // subdirectory within embed.FS (e.g., "assets/webui")
	logger     zerolog.Logger
	mu         sync.RWMutex
	hwSurveyor *env.HardwareSurveyor
	ctx        context.Context
}

type configAppearancePayload struct {
	SansFont        string              `json:"sans_font"`
	SerifFont       string              `json:"serif_font"`
	MonospaceFont   string              `json:"monospace_font"`
	DefaultFontSize int                 `json:"default_font_size"`
	ColorScheme     string              `json:"color_scheme"`
	LightPalette    config.ColorPalette `json:"light_palette"`
	DarkPalette     config.ColorPalette `json:"dark_palette"`
}

type configHardwarePayload struct {
	CPUCores   int    `json:"cpu_cores"`
	CPUThreads int    `json:"cpu_threads"`
	TotalRAMMB int    `json:"total_ram_mb"`
	GPUVendor  string `json:"gpu_vendor"`
	GPUName    string `json:"gpu_name"`
	VRAMMB     int    `json:"vram_mb"`
}

type configPerformancePayload struct {
	Profile  string                    `json:"profile"`
	Resolved configResolvedPerformance `json:"resolved"`
	Hardware configHardwarePayload     `json:"hardware"`
}

// configResolvedPerformance shows the actual values that will be applied at startup.
type configResolvedPerformance struct {
	SkiaCPUThreads         int     `json:"skia_cpu_threads"`
	SkiaGPUThreads         int     `json:"skia_gpu_threads"`
	WebProcessMemoryMB     int     `json:"web_process_memory_mb"`
	NetworkProcessMemoryMB int     `json:"network_process_memory_mb"`
	WebViewPoolPrewarm     int     `json:"webview_pool_prewarm"`
	ConservativeThreshold  float64 `json:"conservative_threshold"`
	StrictThreshold        float64 `json:"strict_threshold"`
	KillThreshold          float64 `json:"kill_threshold"`
}

type configPayload struct {
	Appearance          configAppearancePayload          `json:"appearance"`
	Performance         configPerformancePayload         `json:"performance"`
	DefaultUIScale      float64                          `json:"default_ui_scale"`
	DefaultSearchEngine string                           `json:"default_search_engine"`
	SearchShortcuts     map[string]config.SearchShortcut `json:"search_shortcuts"`
}

// NewDumbSchemeHandler creates a new handler for the dumb:// scheme.
func NewDumbSchemeHandler(ctx context.Context) *DumbSchemeHandler {
	log := logging.FromContext(ctx)

	h := &DumbSchemeHandler{
		handlers:   make(map[string]PageHandler),
		assetDir:   "webui",
		logger:     log.With().Str("component", "scheme-handler").Logger(),
		hwSurveyor: env.NewHardwareSurveyor(),
		ctx:        ctx,
	}

	// Register default pages
	h.registerDefaults()

	return h
}

// SetAssets sets the embedded filesystem containing webui assets.
func (h *DumbSchemeHandler) SetAssets(assets embed.FS) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.assets = assets
	h.logger.Debug().Msg("assets filesystem configured")
}

// registerDefaults sets up default page handlers.
func (h *DumbSchemeHandler) registerDefaults() {
	// Error page (static fallback)
	h.RegisterPage("/error", PageHandlerFunc(func(_ *SchemeRequest) *SchemeResponse {
		return &SchemeResponse{
			Data:        []byte(errorPageHTML),
			ContentType: "text/html",
			StatusCode:  http.StatusOK,
		}
	}))

	// API: Get current config (used by dumb://config)
	h.RegisterPage("/api/config", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		if req.Method != "GET" {
			return nil
		}

		return h.buildConfigResponse(config.Get())
	}))

	// API: Get default config (used by Reset Defaults in dumb://config)
	h.RegisterPage("/api/config/default", PageHandlerFunc(func(req *SchemeRequest) *SchemeResponse {
		if req.Method != "GET" {
			return nil
		}

		return h.buildConfigResponse(config.DefaultConfig())
	}))
}

func (h *DumbSchemeHandler) buildConfigResponse(cfg *config.Config) *SchemeResponse {
	// Get hardware info for display and profile resolution
	// Use background context since survey results are cached and we don't want
	// request context cancellation to affect this
	var hw *port.HardwareInfo
	if h.hwSurveyor != nil {
		hwInfo := h.hwSurveyor.Survey(context.Background())
		hw = &hwInfo
	}

	// Resolve performance profile with hardware info
	resolved := config.ResolvePerformanceProfile(&cfg.Performance, hw)

	resp := configPayload{
		DefaultUIScale:      cfg.DefaultUIScale,
		DefaultSearchEngine: cfg.DefaultSearchEngine,
		SearchShortcuts:     cfg.SearchShortcuts,
		Appearance: configAppearancePayload{
			SansFont:        cfg.Appearance.SansFont,
			SerifFont:       cfg.Appearance.SerifFont,
			MonospaceFont:   cfg.Appearance.MonospaceFont,
			DefaultFontSize: cfg.Appearance.DefaultFontSize,
			ColorScheme:     cfg.Appearance.ColorScheme,
			LightPalette:    cfg.Appearance.LightPalette,
			DarkPalette:     cfg.Appearance.DarkPalette,
		},
		Performance: configPerformancePayload{
			Profile: string(cfg.Performance.Profile),
			Resolved: configResolvedPerformance{
				SkiaCPUThreads:         resolved.SkiaCPUPaintingThreads,
				SkiaGPUThreads:         resolved.SkiaGPUPaintingThreads,
				WebProcessMemoryMB:     resolved.WebProcessMemoryLimitMB,
				NetworkProcessMemoryMB: resolved.NetworkProcessMemoryLimitMB,
				WebViewPoolPrewarm:     resolved.WebViewPoolPrewarmCount,
				ConservativeThreshold:  resolved.WebProcessMemoryConservativeThreshold,
				StrictThreshold:        resolved.WebProcessMemoryStrictThreshold,
				KillThreshold:          resolved.WebProcessMemoryKillThreshold,
			},
			Hardware: buildHardwarePayload(hw),
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return &SchemeResponse{
			Data:        []byte(fmt.Sprintf(`{"error": %q}`, err)),
			ContentType: "application/json",
			StatusCode:  http.StatusInternalServerError,
		}
	}

	return &SchemeResponse{
		Data:        data,
		ContentType: "application/json",
		StatusCode:  http.StatusOK,
	}
}

// buildHardwarePayload converts HardwareInfo to JSON payload.
func buildHardwarePayload(hw *port.HardwareInfo) configHardwarePayload {
	if hw == nil {
		return configHardwarePayload{}
	}
	return configHardwarePayload{
		CPUCores:   hw.CPUCores,
		CPUThreads: hw.CPUThreads,
		TotalRAMMB: hw.TotalRAMMB(),
		GPUVendor:  string(hw.GPUVendor),
		GPUName:    hw.GPUName,
		VRAMMB:     hw.VRAMMB(),
	}
}

// RegisterPage registers a handler for a specific path.
func (h *DumbSchemeHandler) RegisterPage(path string, handler PageHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[path] = handler
	h.logger.Debug().Str("path", path).Msg("registered page handler")
}

// HandleRequest processes a scheme request and sends the response.
func (h *DumbSchemeHandler) HandleRequest(reqPtr uintptr) {
	req := webkit.URISchemeRequestNewFromInternalPtr(reqPtr)
	if req == nil {
		return
	}

	uri := req.GetUri()
	schemeReq := &SchemeRequest{
		inner:  req,
		URI:    uri,
		Path:   req.GetPath(),
		Method: req.GetHttpMethod(),
		Scheme: req.GetScheme(),
	}

	h.logger.Debug().
		Str("uri", schemeReq.URI).
		Str("path", schemeReq.Path).
		Str("method", schemeReq.Method).
		Msg("handling scheme request")

	// Parse the URI to extract host and path
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "dumb" {
		h.logger.Error().Err(err).Str("uri", uri).Msg("invalid URI")
		h.sendResponse(req, &SchemeResponse{
			Data:        []byte("Invalid URI"),
			ContentType: "text/plain",
			StatusCode:  http.StatusBadRequest,
		})
		return
	}

	// API endpoints should never be treated as static assets
	if strings.HasPrefix(schemeReq.Path, "/api/") {
		h.mu.RLock()
		handler, ok := h.handlers[schemeReq.Path]
		if !ok {
			handler, ok = h.handlers[strings.TrimPrefix(schemeReq.Path, "/")]
		}
		h.mu.RUnlock()
		if ok {
			response := handler.Handle(schemeReq)
			h.sendResponse(req, response)
			return
		}
	}

	// Try to serve from embedded assets
	if response := h.handleAsset(u); response != nil {
		h.sendResponse(req, response)
		return
	}

	// Fall back to registered handlers
	h.mu.RLock()
	handler, ok := h.handlers[schemeReq.Path]
	if !ok {
		handler, ok = h.handlers[strings.TrimPrefix(schemeReq.Path, "/")]
	}
	h.mu.RUnlock()

	var response *SchemeResponse
	if ok {
		response = handler.Handle(schemeReq)
	} else {
		response = &SchemeResponse{
			Data:        []byte(notFoundHTML),
			ContentType: "text/html",
			StatusCode:  http.StatusNotFound,
		}
	}

	h.sendResponse(req, response)
}

// handleAsset serves static assets from the embedded filesystem.
// Returns nil if no asset was found (allowing fallback to registered handlers).
func (h *DumbSchemeHandler) handleAsset(u *url.URL) *SchemeResponse {
	h.mu.RLock()
	hasAssets := h.assets != (embed.FS{})
	assetDir := h.assetDir
	h.mu.RUnlock()

	if !hasAssets {
		return nil
	}

	// Determine the target file based on host and path
	host := u.Host
	path := strings.TrimPrefix(u.Path, "/")

	var relPath string
	switch {
	// Home root maps to index.html.
	case host == HomePath && (path == "" || path == "/"):
		relPath = IndexHTML
	// Home asset paths map directly to assets.
	case host == HomePath && path != "":
		relPath = path
	// Config root maps to config.html.
	case host == ConfigPath && (path == "" || path == "/"):
		relPath = "config.html"
	// Config asset paths map directly to assets.
	case host == ConfigPath && path != "":
		relPath = path
	// Error root maps to error.html.
	case host == ErrorPath && (path == "" || path == "/"):
		relPath = "error.html"
	// Error asset paths map directly to assets.
	case host == ErrorPath && path != "":
		relPath = path
	// Opaque home form maps to index.html.
	case u.Opaque == HomePath:
		relPath = IndexHTML
	// Opaque error form maps to error.html.
	case u.Opaque == ErrorPath:
		relPath = "error.html"
	default:
		// Not a recognized asset path
		return nil
	}

	// Read the asset from embedded FS
	fullPath := filepath.ToSlash(filepath.Join(assetDir, relPath))
	data, err := fs.ReadFile(h.assets, fullPath)
	if err != nil {
		h.logger.Debug().Str("path", fullPath).Err(err).Msg("asset not found")
		return nil
	}

	contentType := h.getMimeType(relPath)
	h.logger.Debug().
		Str("path", fullPath).
		Str("content_type", contentType).
		Int("size", len(data)).
		Msg("serving asset")

	return &SchemeResponse{
		Data:        data,
		ContentType: contentType,
		StatusCode:  http.StatusOK,
	}
}

// getMimeType determines the MIME type for a given file path.
func (h *DumbSchemeHandler) getMimeType(filename string) string {
	if h == nil {
		return "application/octet-stream"
	}
	ext := strings.ToLower(filepath.Ext(filename))

	// Try standard mime type first
	mt := mime.TypeByExtension(ext)
	if mt != "" {
		return mt
	}

	// Fallbacks for common web assets
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

// sendResponse sends the response back to WebKit.
func (h *DumbSchemeHandler) sendResponse(req *webkit.URISchemeRequest, response *SchemeResponse) {
	if response == nil {
		response = &SchemeResponse{
			Data:        []byte("Internal error"),
			ContentType: "text/plain",
			StatusCode:  http.StatusInternalServerError,
		}
	}

	contentType := response.ContentType
	if contentType == "" {
		contentType = "text/html"
	}

	// Create MemoryInputStream from data directly
	stream := gio.NewMemoryInputStreamFromData(response.Data, len(response.Data), nil)
	if stream == nil {
		h.logger.Error().Msg("failed to create MemoryInputStream for response")
		return
	}

	// Create response object for more control
	schemeResp := webkit.NewURISchemeResponse(&stream.InputStream, int64(len(response.Data)))
	if schemeResp == nil {
		h.logger.Error().Msg("failed to create URISchemeResponse")
		return
	}
	schemeResp.SetContentType(contentType)
	schemeResp.SetStatus(uint(response.StatusCode), nil)

	// WebKit can treat custom schemes as CORS-relevant even for same-origin fetch().
	// We only add CORS headers for our internal API endpoints.
	if strings.HasPrefix(req.GetPath(), "/api/") {
		hdrs := soup.NewMessageHeaders(soup.MessageHeadersResponseValue)
		hdrs.Append("Access-Control-Allow-Origin", "*")
		hdrs.Append("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		hdrs.Append("Access-Control-Allow-Headers", "Content-Type")
		hdrs.Append("Access-Control-Max-Age", "86400")
		schemeResp.SetHttpHeaders(hdrs)
	}

	req.FinishWithResponse(schemeResp)
}

// RegisterWithContext registers the dumb:// scheme with a WebKitContext.
func (h *DumbSchemeHandler) RegisterWithContext(wkCtx *WebKitContext) {
	if wkCtx == nil || wkCtx.Context() == nil {
		h.logger.Error().Msg("cannot register scheme: context is nil")
		return
	}

	callback := webkit.URISchemeRequestCallback(func(reqPtr, _ uintptr) {
		h.HandleRequest(reqPtr)
	})

	wkCtx.Context().RegisterUriScheme("dumb", &callback, 0, nil)

	// Mark scheme as local, secure, and CORS-enabled for proper security policies
	secMgr := wkCtx.Context().GetSecurityManager()
	if secMgr != nil {
		secMgr.RegisterUriSchemeAsLocal("dumb")
		secMgr.RegisterUriSchemeAsSecure("dumb")
		secMgr.RegisterUriSchemeAsCorsEnabled("dumb")
		h.logger.Debug().Msg("dumb:// scheme registered as local, secure, and CORS-enabled")
	}

	h.logger.Info().Msg("dumb:// scheme registered")
}

// Default page templates (fallback when assets not available)

const errorPageHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Error</title>
    <style>
        body {
            font-family: system-ui, -apple-system, sans-serif;
            background: #1a1a2e;
            color: #eee;
            display: flex;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
        }
        .container {
            text-align: center;
        }
        h1 { color: #e74c3c; }
        p { color: #888; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Error</h1>
        <p>The page could not be loaded.</p>
    </div>
</body>
</html>`

const notFoundHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Not Found</title>
    <style>
        body {
            font-family: system-ui, -apple-system, sans-serif;
            background: #1a1a2e;
            color: #eee;
            display: flex;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
        }
        .container {
            text-align: center;
        }
        h1 { color: #f39c12; }
        p { color: #888; }
    </style>
</head>
<body>
    <div class="container">
        <h1>404</h1>
        <p>Page not found.</p>
    </div>
</body>
</html>`

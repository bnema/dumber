package main

import (
    "context"
    "embed"
    "encoding/json"
    "fmt"
    "mime"
    "log"
    "os"
    "runtime"
    neturl "net/url"
    "path/filepath"
    "strings"
    "strconv"
    "os/exec"
    

    "github.com/bnema/dumber/internal/cli"
    "github.com/bnema/dumber/internal/config"
    "github.com/bnema/dumber/internal/db"
    "github.com/bnema/dumber/services"
    "github.com/bnema/dumber/pkg/webkit"
)

// Build information set via ldflags
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	// Check if we should run the CLI mode
	if shouldRunCLI() {
		runCLI()
		return
	}

    // Otherwise run the GUI browser
    runBrowser()
}

// shouldRunCLI determines if we should run in CLI mode based on arguments
func shouldRunCLI() bool {
	// No args = GUI landing page mode
	if len(os.Args) <= 1 {
		return false
	}

	// Check for GUI-specific flags
	for _, arg := range os.Args[1:] {
		if arg == "--gui" || arg == "-g" {
			return false // Explicit GUI mode
		}
	}

	// Check for browse command - this should open GUI in direct navigation mode
	if len(os.Args) >= 2 && os.Args[1] == "browse" {
		return false // Browse command uses GUI mode but navigates directly
	}

	// Any other arguments mean CLI mode
	return true
}

// runCLI executes the CLI functionality using the existing CLI package
func runCLI() {
	// Initialize configuration system
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing configuration: %v\n", err)
		os.Exit(1)
	}

	// Start configuration watching for live reload
	if err := config.Watch(); err != nil {
		// Don't exit on watch error, just warn
		fmt.Fprintf(os.Stderr, "Warning: failed to start config watching: %v\n", err)
	}

	rootCmd := cli.NewRootCmd(version, commit, buildDate)

	// Handle dmenu flag at the root level for direct integration
	if len(os.Args) > 1 && os.Args[1] == "--dmenu" {
		// Create a temporary CLI instance to handle dmenu mode
		cliInstance, err := cli.NewCLI()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing CLI: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if closeErr := cliInstance.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", closeErr)
			}
		}()

		// Generate dmenu options
		dmenuCmd := cli.NewDmenuCmd()
		if err := dmenuCmd.RunE(dmenuCmd, []string{}); err != nil {
			fmt.Fprintf(os.Stderr, "Error in dmenu mode: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
    }
}

//go:embed frontend/dist
var assets embed.FS

// runBrowser launches a minimal native WebKit window and integrates services.
func runBrowser() {
    log.Printf("Starting GUI mode (webkit_cgo=%v)", webkit.IsNativeAvailable())
    // GTK requires all UI calls to run on the main OS thread
    if webkit.IsNativeAvailable() {
        runtime.LockOSThread()
        defer runtime.UnlockOSThread()
    }

    // Proactively harden media pipeline to avoid WebProcess crashes from buggy VAAPI/plugins.
    // Can be disabled with DUMBER_MEDIA_SAFE=0
    if os.Getenv("DUMBER_MEDIA_SAFE") != "0" {
        // Demote VAAPI-related elements so they won't be auto-picked
        if os.Getenv("GST_PLUGIN_FEATURE_RANK") == "" {
            os.Setenv("GST_PLUGIN_FEATURE_RANK", "vaapisink:0,vaapidecodebin:0,vaapih264dec:0,vaapivideoconvert:0")
        }
        // Provide a safe audio sink to avoid autoaudiosink failures
        if os.Getenv("GST_AUDIO_SINK") == "" {
            os.Setenv("GST_AUDIO_SINK", "fakesink")
        }
        // Optionally disable media stream features that can engage complex pipelines
        if os.Getenv("WEBKIT_DISABLE_MEDIA_STREAM") == "" {
            os.Setenv("WEBKIT_DISABLE_MEDIA_STREAM", "1")
        }
        log.Printf("[media] Safe mode enabled (override with DUMBER_MEDIA_SAFE=0)")
    }

    if err := config.Init(); err != nil {
        log.Fatalf("Failed to initialize config: %v", err)
    }
    cfg := config.Get()
    log.Printf("Config initialized")

    // Detect keyboard layout/locale and hint to webkit layer
    detectAndSetKeyboardLocale()

    // Initialize database
    database, err := db.InitDB(cfg.Database.Path)
    if err != nil {
        log.Fatalf("Failed to initialize database: %v", err)
    }
    defer database.Close()
    queries := db.New(database)
    log.Printf("Database opened at %s", cfg.Database.Path)

    // Initialize services
    parserService := services.NewParserService(cfg, queries)
    browserService := services.NewBrowserService(cfg, queries)
    // JS controls injection is disabled; using native shortcuts via GTK.

    // Track current URL for zoom persistence
    // Initialize to the initial homepage so shortcuts work immediately
    var currentURL string = "dumb://homepage"

    // Register custom scheme resolver for dumb://
    webkit.SetURISchemeResolver(func(uri string) (string, []byte, bool) {
        log.Printf("[scheme] request: %s", uri)
        // Known forms:
        // - dumb://homepage or dumb:homepage → index.html
        // - dumb://app/index.html, dumb://app/<path> → serve from frontend/dist/<path>
        // - dumb://<anything> without path → index.html
        u, err := neturl.Parse(uri)
        if err != nil || u.Scheme != "dumb" {
            return "", nil, false
        }
        // API routes
        if u.Host == "api" || strings.HasPrefix(u.Opaque, "api") || strings.HasPrefix(u.Path, "/api") {
            // Normalize path for opaque or hierarchical forms
            path := u.Path
            if path == "" && u.Opaque != "" {
                // e.g., dumb:api/config or dumb:api/history/recent
                parts := strings.SplitN(u.Opaque, ":", 2)
                if len(parts) == 2 {
                    path = "/" + parts[1]
                }
            }
            if strings.HasPrefix(path, "/api/") { path = strings.TrimPrefix(path, "/api") }
            if path == "/api" { path = "/" }
            switch {
            case strings.HasPrefix(path, "/rendering/status"):
                log.Printf("[api] GET /rendering/status")
                // Report configured rendering mode; runtime GPU state depends on WebKit internals
                resp := struct{
                    Mode string `json:"mode"`
                }{Mode: string(cfg.RenderingMode)}
                b, _ := json.Marshal(resp)
                return "application/json; charset=utf-8", b, true
            case strings.HasPrefix(path, "/config"):
                log.Printf("[api] GET /config")
                // Build config info
                cfgPath, _ := config.GetConfigFile()
                info := struct {
                    ConfigPath     string                                    `json:"config_path"`
                    DatabasePath   string                                    `json:"database_path"`
                    SearchShortcuts map[string]config.SearchShortcut          `json:"search_shortcuts"`
                    Appearance     config.AppearanceConfig                    `json:"appearance"`
                }{
                    ConfigPath: cfgPath,
                    DatabasePath: cfg.Database.Path,
                    SearchShortcuts: cfg.SearchShortcuts,
                    Appearance: cfg.Appearance,
                }
                b, _ := json.Marshal(info)
                return "application/json; charset=utf-8", b, true
            case strings.HasPrefix(path, "/history/recent"):
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
                entries, err := browserService.GetRecentHistory(ctx, limit)
                if err != nil {
                    return "application/json; charset=utf-8", []byte("[]"), true
                }
                b, _ := json.Marshal(entries)
                return "application/json; charset=utf-8", b, true
            case strings.HasPrefix(path, "/history/search"):
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
                entries, err := browserService.SearchHistory(ctx, query, limit)
                if err != nil {
                    return "application/json; charset=utf-8", []byte("[]"), true
                }
                b, _ := json.Marshal(entries)
                return "application/json; charset=utf-8", b, true
            case strings.HasPrefix(path, "/history/stats"):
                log.Printf("[api] GET /history/stats")
                ctx := context.Background()
                stats, err := browserService.GetHistoryStats(ctx)
                if err != nil { return "application/json; charset=utf-8", []byte("{}"), true }
                b, _ := json.Marshal(stats)
                return "application/json; charset=utf-8", b, true
            default:
                return "application/json; charset=utf-8", []byte("{}"), true
            }
        }
        // Resolve target path inside embed FS
        var rel string
        if u.Opaque == "homepage" || (u.Host == "homepage" && (u.Path == "" || u.Path == "/")) || (u.Host == "" && (u.Path == "" || u.Path == "/")) {
            rel = "index.html"
        } else {
            host := u.Host
            p := strings.TrimPrefix(u.Path, "/")
            if host == "app" && p == "" {
                rel = "index.html"
            } else if host == "app" {
                rel = p
            } else if host == "homepage" && p != "" {
                // dumb://homepage/<asset>
                rel = p
            } else if p != "" {
                rel = p
            } else {
                rel = "index.html"
            }
        }
        log.Printf("[scheme] asset: rel=%s (host=%s path=%s)", rel, u.Host, u.Path)
        data, rerr := assets.ReadFile(filepath.ToSlash(filepath.Join("frontend", "dist", rel)))
        if rerr != nil {
            log.Printf("[scheme] not found: %s", rel)
            return "", nil, false
        }
        // Determine mime type
        mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(rel)))
        if mt == "" {
            // Fallbacks
            switch strings.ToLower(filepath.Ext(rel)) {
            case ".js": mt = "application/javascript"
            case ".css": mt = "text/css"
            case ".svg": mt = "image/svg+xml"
            case ".ico": mt = "image/x-icon"
            default: mt = "text/html; charset=utf-8"
            }
        }
        return mt, data, true
    })

    // Create WebKit view
    log.Printf("Creating WebView (native backend expected: %v)", webkit.IsNativeAvailable())
    dataDir, _ := config.GetDataDir()
    stateDir, _ := config.GetStateDir()
    webkitData := filepath.Join(dataDir, "webkit")
    webkitCache := filepath.Join(stateDir, "webkit-cache")
    _ = os.MkdirAll(webkitData, 0o755)
    _ = os.MkdirAll(webkitCache, 0o755)
    view, err := webkit.NewWebView(&webkit.Config{
        InitialURL:        "dumb://homepage",
        ZoomDefault:       1.0,
        EnableDeveloperExtras: true,
        DataDir:           webkitData,
        CacheDir:          webkitCache,
        DefaultSansFont:      cfg.Appearance.SansFont,
        DefaultSerifFont:     cfg.Appearance.SerifFont,
        DefaultMonospaceFont: cfg.Appearance.MonospaceFont,
        DefaultFontSize:      cfg.Appearance.DefaultFontSize,
        Rendering:            webkit.RenderingConfig{Mode: string(cfg.RenderingMode)},
    })
    if err != nil {
        log.Printf("Warning: failed to create WebView: %v", err)
    } else {
        browserService.AttachWebView(view)
        // Use native window as title updater
        if win := view.Window(); win != nil {
            browserService.SetWindowTitleUpdater(win)
        }
        // Persist page titles to DB when they change
        view.RegisterTitleChangedHandler(func(title string) {
            ctx := context.Background()
            url := view.GetCurrentURL()
            if url != "" && title != "" {
                if err := browserService.UpdatePageTitle(ctx, url, title); err != nil {
                    log.Printf("Warning: failed to update page title: %v", err)
                }
            }
        })
        // Track current URL changes and apply saved zoom
        view.RegisterURIChangedHandler(func(u string) {
            currentURL = u
            if u == "" { return }
            ctx := context.Background()
            if z, err := browserService.GetZoomLevel(ctx, u); err == nil {
                _ = view.SetZoom(z)
                key := services.ZoomKeyForLog(u)
                log.Printf("[zoom] loaded %.2f for %s", z, key)
            }
        })
        // Persist zoom changes per-domain
        view.RegisterZoomChangedHandler(func(level float64) {
            url := view.GetCurrentURL()
            if url == "" { return }
            ctx := context.Background()
            if err := browserService.SetZoomLevel(ctx, url, level); err != nil {
                log.Printf("[zoom] failed to save level %.2f for %s: %v", level, url, err)
                return
            }
            key := services.ZoomKeyForLog(url)
            log.Printf("[zoom] saved %.2f for %s", level, key)
        })
        // Bridge UCM messages: navigate + history queries
        view.RegisterScriptMessageHandler(func(payload string) {
            type msg struct {
                Type   string `json:"type"`
                URL    string `json:"url"`
                Q      string `json:"q"`
                Limit  int    `json:"limit"`
                Value  string `json:"value"`
                // Wails fetch bridge
                ID     string          `json:"id"`
                Payload json.RawMessage `json:"payload"`
            }
            var m msg
            if err := json.Unmarshal([]byte(payload), &m); err != nil { return }
            switch m.Type {
            case "navigate":
                ctx := context.Background()
                res, err := parserService.ParseInput(ctx, m.URL)
                if err == nil {
                    currentURL = res.URL
                    browserService.Navigate(ctx, currentURL)
                    _ = view.LoadURL(currentURL)
                    if z, zerr := browserService.GetZoomLevel(ctx, currentURL); zerr == nil { _ = view.SetZoom(z) }
                }
            case "query":
                ctx := context.Background()
                lim := m.Limit
                if lim <= 0 || lim > 25 { lim = 10 }
                entries, err := browserService.SearchHistory(ctx, m.Q, lim)
                if err != nil { return }
                // Map to lightweight items
                type item struct { URL string `json:"url"`; Favicon string `json:"favicon"` }
                buildFavicon := func(raw string) string {
                    u, err := parserService.ParseInput(ctx, raw)
                    if err != nil || u.URL == "" { return "" }
                    parsed, perr := neturl.Parse(u.URL)
                    if perr != nil || parsed.Host == "" { return "" }
                    scheme := parsed.Scheme
                    if scheme == "" { scheme = "https" }
                    return scheme + "://" + parsed.Host + "/favicon.ico"
                }
                items := make([]item, 0, len(entries))
                for _, e := range entries { items = append(items, item{URL: e.URL, Favicon: buildFavicon(e.URL)}) }
                b, _ := json.Marshal(items)
                // Update suggestions in page
                _ = view.InjectScript("window.__dumber_setSuggestions && window.__dumber_setSuggestions(" + string(b) + ")")
            case "wails":
                // Handle Wails runtime fetch bridge calls for homepage
                // Payload contains { methodID, methodName?, args }
                var p struct {
                    MethodID uint32           `json:"methodID"`
                    MethodName string         `json:"methodName"`
                    Args     json.RawMessage  `json:"args"`
                }
                if err := json.Unmarshal(m.Payload, &p); err != nil { return }
                // Only implement the IDs we need
                switch p.MethodID {
                case 3708519028: // BrowserService.GetRecentHistory(limit)
                    var args []interface{}
                    _ = json.Unmarshal(p.Args, &args)
                    limit := 50
                    if len(args) > 0 {
                        if f, ok := args[0].(float64); ok { limit = int(f) }
                    }
                    ctx := context.Background()
                    entries, err := browserService.GetRecentHistory(ctx, limit)
                    if err != nil { return }
                    resp, _ := json.Marshal(entries)
                    _ = view.InjectScript("window.__dumber_wails_resolve('" + m.ID + "', " + string(resp) + ")")
                case 4078533762: // BrowserService.GetSearchShortcuts()
                    ctx := context.Background()
                    shortcuts, err := browserService.GetSearchShortcuts(ctx)
                    if err != nil { return }
                    resp, _ := json.Marshal(shortcuts)
                    _ = view.InjectScript("window.__dumber_wails_resolve('" + m.ID + "', " + string(resp) + ")")
                default:
                    // Return empty JSON to avoid breaking UI
                    _ = view.InjectScript("window.__dumber_wails_resolve('" + m.ID + "', '{}')")
                }
            case "theme":
                if m.Value != "" { log.Printf("[theme] page reported color-scheme: %s", m.Value) }
            }
        })
        // Apply initial zoom setting and log it before showing the window
        {
            ctx := context.Background()
            u := view.GetCurrentURL()
            if u == "" { u = "dumb://homepage" }
            if z, zerr := browserService.GetZoomLevel(ctx, u); zerr == nil {
                if err := view.SetZoom(z); err != nil {
                    log.Printf("Warning: failed to set initial zoom: %v", err)
                } else {
                    key := services.ZoomKeyForLog(u)
                    log.Printf("[zoom] loaded %.2f for %s", z, key)
                }
            }
        }

        log.Printf("Showing WebView window…")
        if err := view.Show(); err != nil {
            log.Printf("Warning: failed to show WebView: %v", err)
        } else {
            if !webkit.IsNativeAvailable() {
                log.Printf("Notice: running without webkit_cgo tag — no native window will be displayed.")
            }
        }
    }

    // Parse browse argument if present
    if len(os.Args) >= 3 && os.Args[1] == "browse" {
        log.Printf("Browse command detected: %s", os.Args[2])
        ctx := context.Background()
        result, err := parserService.ParseInput(ctx, os.Args[2])
        if err == nil {
            currentURL = result.URL
            log.Printf("Parsed input → URL: %s", currentURL)
            browserService.Navigate(ctx, currentURL)
            if view != nil {
                log.Printf("Loading URL in WebView: %s", currentURL)
                if err := view.LoadURL(currentURL); err != nil {
                    log.Printf("Warning: failed to load URL: %v", err)
                }
                if z, zerr := browserService.GetZoomLevel(ctx, currentURL); zerr == nil {
                    if err := view.SetZoom(z); err != nil {
                        log.Printf("Warning: failed to set zoom: %v", err)
                    } else {
                        key := services.ZoomKeyForLog(currentURL)
                        log.Printf("[zoom] loaded %.2f for %s", z, key)
                    }
                }
                // No JS injection; native accelerators handle zoom/devtools.
            }
        }
    }

    // Register basic keyboard shortcuts on the native view to preserve behavior
    if view != nil {
        // DevTools
        _ = view.RegisterKeyboardShortcut("F12", func() { log.Printf("Shortcut: F12 (devtools)"); _ = view.ShowDevTools() })
        // Omnibox (Ctrl+L): rely on injected script listener
        _ = view.RegisterKeyboardShortcut("cmdorctrl+l", func() {
            log.Printf("Shortcut: Omnibox toggle")
            _ = view.InjectScript("window.__dumber_toggle && window.__dumber_toggle()")
        })
        // Zoom handled natively in webkit package (built-in shortcuts)
    }

    // Enter GTK main loop only when native backend is available.
    if webkit.IsNativeAvailable() {
        log.Printf("Entering GTK main loop…")
        webkit.RunMainLoop()
        log.Printf("GTK main loop exited")
    } else {
        log.Printf("Not entering GUI loop (non-CGO build)")
    }
}

// detectAndSetKeyboardLocale attempts to determine the current keyboard layout
// and hints it to the webkit input layer for accelerator compatibility.
func detectAndSetKeyboardLocale() {
    // 0) Explicit override
    locale := os.Getenv("DUMBER_KEYBOARD_LOCALE")
    if locale == "" {
        locale = os.Getenv("DUMB_BROWSER_KEYBOARD_LOCALE") // legacy prefix
    }
    // 1) Environment
    if locale == "" { locale = os.Getenv("LC_ALL") }
    if locale == "" { locale = os.Getenv("LANG") }
    if locale == "" { locale = os.Getenv("LC_CTYPE") }
    // Trim variants
    if i := strings.IndexByte(locale, '.'); i > 0 { locale = locale[:i] }
    if i := strings.IndexByte(locale, '@'); i > 0 { locale = locale[:i] }
    if i := strings.IndexByte(locale, '_'); i > 0 { locale = locale[:i] }

    // 2) Try XKB env override
    if locale == "" {
        locale = os.Getenv("XKB_DEFAULT_LAYOUT")
    }

    // 3) Best-effort probe of setxkbmap/localectl (non-fatal)
    if locale == "" {
        if out, err := exec.Command("localectl", "status").Output(); err == nil {
            s := string(out)
            for _, line := range strings.Split(s, "\n") {
                line = strings.TrimSpace(line)
                if strings.HasPrefix(strings.ToLower(line), "x11 layout:") || strings.HasPrefix(strings.ToLower(line), "keyboard layout:") {
                    parts := strings.SplitN(line, ":", 2)
                    if len(parts) == 2 {
                        cand := strings.TrimSpace(parts[1])
                        if cand != "" { locale = cand }
                    }
                    break
                }
            }
        }
    }
    if locale == "" {
        if out, err := exec.Command("setxkbmap", "-query").Output(); err == nil {
            s := string(out)
            for _, line := range strings.Split(s, "\n") {
                line = strings.TrimSpace(line)
                if strings.HasPrefix(strings.ToLower(line), "layout:") {
                    parts := strings.SplitN(line, ":", 2)
                    if len(parts) == 2 {
                        cand := strings.TrimSpace(parts[1])
                        if cand != "" { locale = cand }
                    }
                    break
                }
            }
        }
    }

    if locale == "" { locale = "en" }
    // No layout-specific remaps; log for diagnostics only
    log.Printf("[locale] keyboard locale detected: %s", locale)
}

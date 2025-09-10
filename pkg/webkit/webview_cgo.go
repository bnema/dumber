//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4 javascriptcoregtk-6.0
#include <stdlib.h>
#include <string.h>
#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <glib-object.h>
#include <gdk/gdk.h>
#include <jsc/jsc.h>
#include <glib.h>
#include <gio/gio.h>

static GtkWidget* new_window() { return GTK_WIDGET(gtk_window_new()); }

// Forward declaration
extern void goQuitMainLoop();

// GTK4 close-request signal handler
static gboolean on_close_request(GtkWindow* window, gpointer user_data) {
    (void)window; (void)user_data;
    goQuitMainLoop();
    return FALSE; // Allow the window to close
}

// Connect window close signal to quit main loop
static void connect_destroy_quit(GtkWidget* w) {
    if (!w) return;
    g_signal_connect(G_OBJECT(w), "close-request", G_CALLBACK(on_close_request), NULL);
}
static WebKitWebView* as_webview(GtkWidget* w) { return WEBKIT_WEB_VIEW(w); }
// WebsiteDataManager creation will be handled via GTK4/WebKit6 APIs in Go code.

// Forward declare helpers used below
static void maybe_set_cookie_policy(WebKitCookieManager* cm, int policy);

// Construct a WebKitWebView via g_object_new with a fresh UserContentManager.
static GtkWidget* new_webview_with_ucm_and_session(const char* data_dir, const char* cache_dir, const char* cookie_path, WebKitUserContentManager** out_ucm, WebKitMemoryPressureSettings* pressure_settings) {
    WebKitUserContentManager* u = webkit_user_content_manager_new();
    if (!u) return NULL;
    // WebKitGTK 6: create NetworkSession with data/cache directories
    WebKitNetworkSession* sess = webkit_network_session_new(
        data_dir ? data_dir : "",
        cache_dir ? cache_dir : "");
    if (!sess) { g_object_unref(u); return NULL; }
    // Apply memory pressure settings to the session when provided
    if (pressure_settings) {
        // WebKitGTK 6.0 API: setter expects only the settings pointer
        webkit_network_session_set_memory_pressure_settings(pressure_settings);
    }
    // Configure cookie persistence and policy (no third-party by default)
    WebKitCookieManager* cm = webkit_network_session_get_cookie_manager(sess);
    if (cm) {
        if (cookie_path && cookie_path[0]) {
            webkit_cookie_manager_set_persistent_storage(cm, cookie_path, WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE);
        }
        // 2 => no-third-party (default), 0 => always, 1 => never
        maybe_set_cookie_policy(cm, 2);
    }
    // Persist credentials (HTTP auth, etc.)
    webkit_network_session_set_persistent_credential_storage_enabled(sess, TRUE);
    GtkWidget* w = GTK_WIDGET(g_object_new(WEBKIT_TYPE_WEB_VIEW,
        "user-content-manager", u,
        "network-session", sess,
        NULL));
    // WebView holds refs to provided objects; drop our temporary refs
    g_object_unref(sess);
    if (out_ucm) { *out_ucm = u; }
    return w;
}

static gboolean gtk_prefers_dark() {
    // Method 1: Check GNOME desktop interface color-scheme (primary method)
    GSettings* desktop_settings = g_settings_new("org.gnome.desktop.interface");
    if (desktop_settings) {
        gchar* color_scheme = g_settings_get_string(desktop_settings, "color-scheme");
        if (color_scheme) {
            gboolean prefer_dark = (g_strcmp0(color_scheme, "prefer-dark") == 0);
            g_free(color_scheme);
            g_object_unref(desktop_settings);
            if (prefer_dark) return TRUE;
        }
        g_object_unref(desktop_settings);
    }
    
    // Method 2: Check GTK theme name for dark variants
    GtkSettings* settings = gtk_settings_get_default();
    if (settings) {
        gchar* theme_name = NULL;
        g_object_get(settings, "gtk-theme-name", &theme_name, NULL);
        if (theme_name) {
            gboolean is_dark = (strstr(theme_name, "-dark") != NULL || 
                               strstr(theme_name, "-Dark") != NULL);
            g_free(theme_name);
            if (is_dark) return TRUE;
        }
        
        // Method 3: Check gtk-application-prefer-dark-theme (fallback)
        gboolean prefer = FALSE;
        g_object_get(settings, "gtk-application-prefer-dark-theme", &prefer, NULL);
        if (prefer) return TRUE;
    }
    
    return FALSE;
}

// Note: preferred color scheme is handled via a user script injection to support
// older WebKitGTK versions without the color-scheme API. See enableUserContentManager.

extern void goOnUcmMessage(unsigned long id, const char* json);
extern void goOnTitleChanged(unsigned long id, const char* title);
extern void goOnURIChanged(unsigned long id, const char* uri);
extern void goOnThemeChanged(unsigned long id, int prefer_dark);
extern void* goResolveURIScheme(char* uri, size_t* out_len, char** out_mime);

void on_title_notify(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec;
    WebKitWebView* view = WEBKIT_WEB_VIEW(obj);
    const gchar* title = webkit_web_view_get_title(view);
    GtkWindow* win = GTK_WINDOW(user_data);
    if (title && win) {
        gtk_window_set_title(win, title);
    }
}

void on_title_notify_id(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec; (void)obj;
    const gchar* title = webkit_web_view_get_title(WEBKIT_WEB_VIEW(obj));
    if (title) { goOnTitleChanged((unsigned long)user_data, title); }
}

void on_uri_notify(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec; (void)obj;
    const gchar* uri = webkit_web_view_get_uri(WEBKIT_WEB_VIEW(obj));
    if (uri) { goOnURIChanged((unsigned long)user_data, uri); }
}

// React to GTK theme preference changes at runtime
void on_theme_changed(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec;
    GtkSettings* settings = GTK_SETTINGS(obj);
    if (!settings) return;
    gboolean prefer = FALSE;
    g_object_get(settings, "gtk-application-prefer-dark-theme", &prefer, NULL);
    goOnThemeChanged((unsigned long)user_data, prefer ? 1 : 0);
}

// NOTE: UCM message callback signature changed in WebKitGTK 6; will be reimplemented later.

void on_uri_scheme(WebKitURISchemeRequest* request, gpointer user_data) {
    (void)user_data;
    const gchar* uri = webkit_uri_scheme_request_get_uri(request);
    size_t n = 0;
    char* mime = NULL;
    void* buf = goResolveURIScheme((char*)uri, &n, &mime);
    if (buf && n > 0 && mime) {
        GInputStream* stream = g_memory_input_stream_new_from_data(buf, (gssize)n, g_free);
        webkit_uri_scheme_request_finish(request, stream, (gint64)n, mime);
        g_object_unref(stream);
        g_free(mime);
    } else {
        GError* err = g_error_new_literal(g_quark_from_string("dumber"), 404, "Not found");
        webkit_uri_scheme_request_finish_error(request, err);
        g_error_free(err);
    }
}

// Script message handler wiring for WebKitGTK 6
// Forward declaration for callback used in g_signal_connect_data
// WebKitGTK 6 delivers JSCValue* for script-message-received::NAME
static void ucm_on_script_message_cb(WebKitUserContentManager* ucm, JSCValue* val, gpointer user_data);
// Uses WebKitScriptMessage and script-message-received::NAME signal.
static void ucm_on_script_message_cb(WebKitUserContentManager* ucm, JSCValue* val, gpointer user_data) {
    (void)ucm;
    if (!val) return;
    gchar* s = jsc_value_to_string(val);
    if (!s) return;
    goOnUcmMessage((unsigned long)user_data, s);
    g_free(s);
}

static gboolean register_ucm_handler(WebKitUserContentManager* ucm, const char* name, unsigned long id) {
    if (!ucm || !name) return FALSE;
    // WebKitGTK 6 signature: (ucm, name, world_name)
    webkit_user_content_manager_register_script_message_handler(ucm, name, NULL);
    gchar* signal = g_strdup_printf("script-message-received::%s", name);
    if (!signal) return FALSE;
    g_signal_connect_data(G_OBJECT(ucm), signal, G_CALLBACK(ucm_on_script_message_cb), (gpointer)id, NULL, 0);
    g_free(signal);
    return TRUE;
}

// ----- Conditional helpers for settings across WebKit versions -----
#ifndef WEBKIT_CHECK_VERSION
#define WEBKIT_CHECK_VERSION(major,minor,micro) (0)
#endif

static void maybe_set_hw_policy(WebKitSettings* settings, int mode) {
#if WEBKIT_CHECK_VERSION(2,40,0)
    // mode: 0=ON_DEMAND, 1=ALWAYS, 2=NEVER
    if (!settings) return;
    if (mode == 1) {
        webkit_settings_set_hardware_acceleration_policy(settings, WEBKIT_HARDWARE_ACCELERATION_POLICY_ALWAYS);
    } else if (mode == 2) {
        webkit_settings_set_hardware_acceleration_policy(settings, WEBKIT_HARDWARE_ACCELERATION_POLICY_NEVER);
    } else {
        // Leave default policy (auto) without forcing a value to avoid referencing
        // WEBKIT_HARDWARE_ACCELERATION_POLICY_ON_DEMAND on older headers.
    }
#else
    (void)settings; (void)mode;
#endif
}

static void maybe_set_draw_indicators(WebKitSettings* settings, int enable) {
#if WEBKIT_CHECK_VERSION(2,36,0)
    if (!settings) return;
    webkit_settings_set_draw_compositing_indicators(settings, enable ? 1 : 0);
#else
    (void)settings; (void)enable;
#endif
}

static void maybe_set_webgl(WebKitSettings* settings, int enable) {
#if WEBKIT_CHECK_VERSION(2,20,0)
    if (!settings) return;
    webkit_settings_set_enable_webgl(settings, enable ? 1 : 0);
#else
    (void)settings; (void)enable;
#endif
}

static void maybe_set_canvas_accel(WebKitSettings* settings, int enable) {
#if WEBKIT_CHECK_VERSION(2,20,0)
    if (!settings) return;
    webkit_settings_set_enable_2d_canvas_acceleration(settings, enable ? 1 : 0);
#else
    (void)settings; (void)enable;
#endif
}

static void maybe_set_media_user_gesture(WebKitSettings* settings, int require) {
#if WEBKIT_CHECK_VERSION(2,20,0)
    if (!settings) return;
    webkit_settings_set_media_playback_requires_user_gesture(settings, require ? 1 : 0);
#else
    (void)settings; (void)require;
#endif
}

// Enable two-finger swipe back/forward navigation gestures when supported
static void maybe_set_back_forward_gestures(WebKitSettings* settings, int enable) {
#if WEBKIT_CHECK_VERSION(2,38,0)
    if (!settings) return;
    webkit_settings_set_enable_back_forward_navigation_gestures(settings, enable ? 1 : 0);
#else
    (void)settings; (void)enable;
#endif
}

// Set cookie accept policy with compile-time compatibility across WebKit versions
static void maybe_set_cookie_policy(WebKitCookieManager* cm, int policy) {
    if (!cm) return;
#if defined(WEBKIT_COOKIE_ACCEPT_POLICY_ALWAYS)
    WebKitCookieAcceptPolicy p = WEBKIT_COOKIE_ACCEPT_POLICY_ALWAYS;
    if (policy == 1) p = WEBKIT_COOKIE_ACCEPT_POLICY_NEVER; // never
    else if (policy == 2) p = WEBKIT_COOKIE_ACCEPT_POLICY_NO_THIRD_PARTY; // no third-party
    webkit_cookie_manager_set_accept_policy(cm, p);
#elif defined(WEBKIT_COOKIE_POLICY_ACCEPT_ALWAYS)
    WebKitCookieAcceptPolicy p = WEBKIT_COOKIE_POLICY_ACCEPT_ALWAYS;
    if (policy == 1) p = WEBKIT_COOKIE_POLICY_ACCEPT_NEVER;
    else if (policy == 2) p = WEBKIT_COOKIE_POLICY_ACCEPT_NO_THIRD_PARTY;
    webkit_cookie_manager_set_accept_policy(cm, p);
#else
    (void)policy; // Not supported in this header version
#endif
}

// Memory pressure settings helpers (version-guarded)
static WebKitMemoryPressureSettings* create_memory_pressure_settings(
    unsigned int memory_limit_mb,
    double conservative_threshold,
    double strict_threshold,
    double kill_threshold,
    double poll_interval) {
    // If headers don't expose memory pressure APIs, return NULL to signal unsupported
#if WEBKIT_CHECK_VERSION(2,44,0)
    WebKitMemoryPressureSettings* settings = webkit_memory_pressure_settings_new();
    if (!settings) return NULL;

    if (memory_limit_mb > 0) {
        webkit_memory_pressure_settings_set_memory_limit(settings, (guint)(memory_limit_mb * 1024 * 1024));
    }

    webkit_memory_pressure_settings_set_conservative_threshold(settings, conservative_threshold);
    webkit_memory_pressure_settings_set_strict_threshold(settings, strict_threshold);

    if (kill_threshold > 0) {
        webkit_memory_pressure_settings_set_kill_threshold(settings, kill_threshold);
    }

    webkit_memory_pressure_settings_set_poll_interval(settings, poll_interval);

    return settings;
#else
    (void)memory_limit_mb; (void)conservative_threshold; (void)strict_threshold; (void)kill_threshold; (void)poll_interval;
    return NULL;
#endif
}

// Cache model configuration
static void maybe_set_cache_model(WebKitWebContext* context, int model) {
    if (!context) return;
    WebKitCacheModel cache_model = WEBKIT_CACHE_MODEL_WEB_BROWSER; // default

    switch (model) {
        case 0: cache_model = WEBKIT_CACHE_MODEL_DOCUMENT_VIEWER; break;
        case 1: cache_model = WEBKIT_CACHE_MODEL_WEB_BROWSER; break;
        case 2:
#ifdef WEBKIT_CACHE_MODEL_PRIMARY_WEB_BROWSER
            cache_model = WEBKIT_CACHE_MODEL_PRIMARY_WEB_BROWSER;
#else
            cache_model = WEBKIT_CACHE_MODEL_WEB_BROWSER;
#endif
            break;
    }

    webkit_web_context_set_cache_model(context, cache_model);
}

// Additional settings for memory optimization
static void maybe_set_page_cache(WebKitSettings* settings, int enable) {
#if WEBKIT_CHECK_VERSION(2,6,0)
    if (!settings) return;
    webkit_settings_set_enable_page_cache(settings, enable ? TRUE : FALSE);
#else
    (void)settings; (void)enable;
#endif
}

// Offline Web Application Cache is deprecated in modern WebKit; keep as no-op to avoid warnings.
static void maybe_set_offline_app_cache(WebKitSettings* settings, int enable) {
    (void)settings; (void)enable;
}

// Apply memory pressure settings to context if available
static void maybe_set_memory_pressure_settings(WebKitWebContext* context, WebKitMemoryPressureSettings* settings) {
    // No-op wrapper kept for compatibility; settings applied to NetworkSession instead.
    (void)context; (void)settings;
}

// Trigger JavaScript GC if supported by headers
static void maybe_collect_js(WebKitWebContext* context) {
    // Disabled by default due to API availability variance across 6.0 builds.
    (void)context;
}
*/
import "C"

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"
)

// Helper function to convert bool to int for C interop
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}


type nativeView struct {
	win  *C.GtkWidget
	view *C.GtkWidget
	wv   *C.WebKitWebView
	ucm  *C.WebKitUserContentManager
}

type memoryStats struct {
	pageLoadCount          int
	lastGCTime             time.Time
	memoryPressureSettings *C.WebKitMemoryPressureSettings
}

// WebView represents a browser view powered by WebKit2GTK.
type WebView struct {
	config       *Config
	zoom         float64
	url          string
	destroyed    bool
	native       *nativeView
	window       *Window
	id           uintptr
	msgHandler   func(payload string)
	titleHandler func(title string)
	uriHandler   func(uri string)
	zoomHandler  func(level float64)
	memStats     *memoryStats
	gcTicker     *time.Ticker
	gcDone       chan struct{}
}

// NewWebView constructs a new WebView instance with native WebKit2GTK widgets.
func NewWebView(cfg *Config) (*WebView, error) {
	log.Printf("[webkit] Initializing GTK and creating WebView (CGO)")
	if C.gtk_init_check() == 0 {
		return nil, errors.New("failed to initialize GTK")
	}

	if cfg == nil {
		cfg = &Config{}
	}

	// Prepare persistent website data/caches
	dataDir := cfg.DataDir
	cacheDir := cfg.CacheDir
	if dataDir == "" {
		dataDir = filepath.Join(os.TempDir(), "dumber-webkit-data")
	}
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "dumber-webkit-cache")
	}
	_ = os.MkdirAll(dataDir, 0o755)
	_ = os.MkdirAll(cacheDir, 0o755)

	cData := C.CString(dataDir)
	cCache := C.CString(cacheDir)
	cookieFile := filepath.Join(dataDir, "cookies.db")
	cCookie := C.CString(cookieFile)
	defer C.free(unsafe.Pointer(cData))
	defer C.free(unsafe.Pointer(cCache))
	defer C.free(unsafe.Pointer(cCookie))
	// Use default WebContext to register the custom URI scheme handler; the
	// WebView itself is created with a WebKitNetworkSession for persistence.
	ctx := C.webkit_web_context_get_default()

	// Apply cache model to WebContext
	// Default to WEBKIT_CACHE_MODEL_WEB_BROWSER to keep caching enabled
	cacheModel := 1
	if cfg.Memory.CacheModel != "" {
		switch cfg.Memory.CacheModel {
		case CacheModelWebBrowser:
			cacheModel = 1
		case CacheModelPrimaryWebBrowser:
			cacheModel = 2
		default: // CacheModelDocumentViewer or unset
			cacheModel = 0
		}
	}
	C.maybe_set_cache_model(ctx, C.int(cacheModel))

	// Configure memory pressure settings if specified
	var pressureSettings *C.WebKitMemoryPressureSettings
	// Only configure memory pressure if explicitly requested via config
	if cfg.Memory.MemoryLimitMB > 0 || cfg.Memory.ConservativeThreshold > 0 || cfg.Memory.StrictThreshold > 0 || cfg.Memory.KillThreshold > 0 || cfg.Memory.PollIntervalSeconds > 0 {
		memLimitMB := cfg.Memory.MemoryLimitMB
		conservativeThreshold := cfg.Memory.ConservativeThreshold
		strictThreshold := cfg.Memory.StrictThreshold
		killThreshold := cfg.Memory.KillThreshold
		pollInterval := cfg.Memory.PollIntervalSeconds

		pressureSettings = C.create_memory_pressure_settings(
			C.uint(memLimitMB),
			C.double(conservativeThreshold),
			C.double(strictThreshold),
			C.double(killThreshold),
			C.double(pollInterval))

		if pressureSettings != nil {
			C.maybe_set_memory_pressure_settings(ctx, pressureSettings)
			if cfg.Memory.EnableMemoryMonitoring {
				log.Printf("[webkit] Memory pressure settings applied: limit=%dMB, conservative=%.2f, strict=%.2f, poll=%.1fs",
					memLimitMB, conservativeThreshold, strictThreshold, pollInterval)
			}
		}
	}

	// Register custom URI scheme handler for "dumb://"
	{
		sch := C.CString("dumb")
		C.webkit_web_context_register_uri_scheme(ctx, sch, (C.WebKitURISchemeRequestCallback)(C.on_uri_scheme), nil, nil)
		C.free(unsafe.Pointer(sch))
	}
	// Cookie manager persistent storage handled via NetworkSession in new_webview_with_ucm_and_session

	// Create WebView through g_object_new with a fresh UCM (GTK4/WebKit 6 style)
	var createdUcm *C.WebKitUserContentManager
	viewWidget := C.new_webview_with_ucm_and_session(cData, cCache, cCookie, &createdUcm, pressureSettings)
	if viewWidget == nil {
		return nil, errors.New("failed to create WebKitWebView")
	}

	// Create a top-level window to host the view
	win := C.new_window()
	if win == nil {
		return nil, errors.New("failed to create GtkWindow")
	}

	// Pack view widget into the window
	// GTK4: containers removed; use gtk_window_set_child
	C.gtk_window_set_child((*C.GtkWindow)(unsafe.Pointer(win)), viewWidget)
	C.gtk_window_set_default_size((*C.GtkWindow)(unsafe.Pointer(win)), 1024, 768)
	C.connect_destroy_quit(win)

	v := &WebView{
		config: cfg,
		zoom:   1.0,
		native: &nativeView{win: win, view: viewWidget, wv: C.as_webview(viewWidget), ucm: createdUcm},
		window: &Window{win: win},
		memStats: &memoryStats{
			pageLoadCount:          0,
			lastGCTime:             time.Now(),
			memoryPressureSettings: pressureSettings,
		},
		gcDone: make(chan struct{}),
	}
	// Assign an ID for accelerator dispatch
	v.id = nextViewID()
	registerView(v.id, v)

	// Register with global memory manager if monitoring is enabled
	if cfg.Memory.EnableMemoryMonitoring {
		if globalMemoryManager == nil {
			InitializeGlobalMemoryManager(
				cfg.Memory.EnableMemoryMonitoring,
				cfg.Memory.ProcessRecycleThreshold,
				30*time.Second, // Default monitoring interval
			)
		}
		if globalMemoryManager != nil {
			globalMemoryManager.RegisterWebView(v)
		}
	}
	// Attach GTK4 input controllers (keyboard, mouse)
	AttachKeyboardControllers(v)

	// Watch for GTK theme changes and propagate to page at runtime
	if settings := C.gtk_settings_get_default(); settings != nil {
		cprop := C.CString("notify::gtk-application-prefer-dark-theme")
		C.g_signal_connect_data(C.gpointer(unsafe.Pointer(settings)), cprop, C.GCallback(C.on_theme_changed), C.gpointer(v.id), nil, 0)
		C.free(unsafe.Pointer(cprop))
	}

	// Native zoom shortcuts (independent of app services)
	// Ctrl+='=' and Ctrl+'+' → Zoom In; Ctrl+'-' → Zoom Out; Ctrl+'0' → Reset
	_ = v.RegisterKeyboardShortcut("cmdorctrl+=", func() {
		nz := v.zoom
		if nz <= 0 {
			nz = 1.0
		}
		nz *= 1.1
		if nz < 0.25 {
			nz = 0.25
		}
		if nz > 5.0 {
			nz = 5.0
		}
		_ = v.SetZoom(nz)
	})
	_ = v.RegisterKeyboardShortcut("cmdorctrl+plus", func() {
		nz := v.zoom
		if nz <= 0 {
			nz = 1.0
		}
		nz *= 1.1
		if nz < 0.25 {
			nz = 0.25
		}
		if nz > 5.0 {
			nz = 5.0
		}
		_ = v.SetZoom(nz)
	})
	_ = v.RegisterKeyboardShortcut("cmdorctrl-", func() {
		nz := v.zoom
		if nz <= 0 {
			nz = 1.0
		}
		nz /= 1.1
		if nz < 0.25 {
			nz = 0.25
		}
		if nz > 5.0 {
			nz = 5.0
		}
		_ = v.SetZoom(nz)
	})
	_ = v.RegisterKeyboardShortcut("cmdorctrl+0", func() { _ = v.SetZoom(1.0) })
	// Find in page (Ctrl/Cmd+F) reuses omnibox component in find mode
	_ = v.RegisterKeyboardShortcut("cmdorctrl+f", func() { _ = v.OpenFind("") })
	// Navigation with Alt+Arrow keys
	_ = v.RegisterKeyboardShortcut("alt+ArrowLeft", func() { _ = v.GoBack() })
	_ = v.RegisterKeyboardShortcut("alt+ArrowRight", func() { _ = v.GoForward() })
	// Update window title when page title changes
	cNotifyTitle1 := C.CString("notify::title")
	C.g_signal_connect_data(C.gpointer(unsafe.Pointer(viewWidget)), cNotifyTitle1, C.GCallback(C.on_title_notify), C.gpointer(unsafe.Pointer(win)), nil, 0)
	C.free(unsafe.Pointer(cNotifyTitle1))
	// Also dispatch title change to Go with view id
	cNotifyTitle2 := C.CString("notify::title")
	C.g_signal_connect_data(C.gpointer(unsafe.Pointer(viewWidget)), cNotifyTitle2, C.GCallback(C.on_title_notify_id), C.gpointer(v.id), nil, 0)
	C.free(unsafe.Pointer(cNotifyTitle2))
	// Notify URI changes to Go to keep current URL in sync
	cNotifyURI := C.CString("notify::uri")
	C.g_signal_connect_data(C.gpointer(unsafe.Pointer(viewWidget)), cNotifyURI, C.GCallback(C.on_uri_notify), C.gpointer(v.id), nil, 0)
	C.free(unsafe.Pointer(cNotifyURI))
	// Apply hardware acceleration and related settings based on cfg.Rendering
	if settings := C.webkit_web_view_get_settings(v.native.wv); settings != nil {
		// Hardware acceleration policy (guarded by version)
		switch cfg.Rendering.Mode {
		case "gpu":
			C.maybe_set_hw_policy(settings, 1)
			C.maybe_set_webgl(settings, 1)
			C.maybe_set_canvas_accel(settings, 1)
		case "cpu":
			C.maybe_set_hw_policy(settings, 2)
			C.maybe_set_webgl(settings, 0)
			C.maybe_set_canvas_accel(settings, 0)
		default: // auto
			C.maybe_set_hw_policy(settings, 0)
			C.maybe_set_webgl(settings, 1)
			C.maybe_set_canvas_accel(settings, 1)
		}
		// Optional compositing indicators for debugging (guarded by version)
		if cfg.Rendering.DebugGPU {
			C.maybe_set_draw_indicators(settings, 1)
		}
		// Reduce media pipeline churn by requiring a user gesture for playback
		C.maybe_set_media_user_gesture(settings, 1)
		// Enable trackpad back/forward gestures when available
		C.maybe_set_back_forward_gestures(settings, 1)

		// Keep page cache enabled by default for performance
		enablePageCache := true
		if cfg.Memory.EnablePageCache {
			enablePageCache = true
		}
		C.maybe_set_page_cache(settings, C.int(boolToInt(enablePageCache)))

		// Offline app cache is deprecated; keep as no-op
		C.maybe_set_offline_app_cache(settings, C.int(0))

		if cfg.Memory.EnableMemoryMonitoring {
			log.Printf("[webkit] Memory settings applied: page_cache=%v", enablePageCache)
		}
	}

	if cfg.ZoomDefault > 0 {
		v.zoom = cfg.ZoomDefault
		C.webkit_web_view_set_zoom_level(v.native.wv, C.gdouble(v.zoom))
	}
	if cfg.EnableDeveloperExtras {
		settings := C.webkit_web_view_get_settings(v.native.wv)
		if settings != nil {
			C.webkit_settings_set_enable_developer_extras(settings, C.gboolean(1))
		}
	}
	// Apply default fonts if provided in cfg
	if settings := C.webkit_web_view_get_settings(v.native.wv); settings != nil {
		if cfg.DefaultSansFont != "" {
			csans := C.CString(cfg.DefaultSansFont)
			C.webkit_settings_set_sans_serif_font_family(settings, (*C.gchar)(csans))
			C.free(unsafe.Pointer(csans))
			// Also set default font family to sans
			csans2 := C.CString(cfg.DefaultSansFont)
			C.webkit_settings_set_default_font_family(settings, (*C.gchar)(csans2))
			C.free(unsafe.Pointer(csans2))
		}
		if cfg.DefaultSerifFont != "" {
			cserif := C.CString(cfg.DefaultSerifFont)
			C.webkit_settings_set_serif_font_family(settings, (*C.gchar)(cserif))
			C.free(unsafe.Pointer(cserif))
		}
		if cfg.DefaultMonospaceFont != "" {
			cmono := C.CString(cfg.DefaultMonospaceFont)
			C.webkit_settings_set_monospace_font_family(settings, (*C.gchar)(cmono))
			C.free(unsafe.Pointer(cmono))
		}
		if cfg.DefaultFontSize > 0 {
			C.webkit_settings_set_default_font_size(settings, C.guint(cfg.DefaultFontSize))
		}
	}
	if cfg.InitialURL != "" {
		_ = v.LoadURL(cfg.InitialURL)
	}
	// Ensure native resources are released if user forgets to call Destroy
	runtime.SetFinalizer(v, func(v *WebView) { _ = v.Destroy() })
	log.Printf("[webkit] WebView created (CGO)")

	// Initialize UCM scripts and handlers
	if v.native != nil && v.native.ucm != nil {
		v.enableUserContentManager()
	}

	// Start periodic JavaScript garbage collection if configured
	if cfg.Memory.EnableGCInterval > 0 {
		v.startPeriodicGC(time.Duration(cfg.Memory.EnableGCInterval) * time.Second)
		if cfg.Memory.EnableMemoryMonitoring {
			log.Printf("[webkit] Periodic GC enabled: interval=%ds", cfg.Memory.EnableGCInterval)
		}
	}

	return v, nil
}

func (w *WebView) LoadURL(rawURL string) error {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return ErrNotImplemented
	}

	// Track page loads for process recycling
	if w.memStats != nil {
		w.memStats.pageLoadCount++

		// Check if we need to recycle this WebView
		if w.config.Memory.ProcessRecycleThreshold > 0 &&
			w.memStats.pageLoadCount >= w.config.Memory.ProcessRecycleThreshold {
			if w.config.Memory.EnableMemoryMonitoring {
				log.Printf("[webkit] Process recycle threshold reached (%d pages), consider creating new WebView",
					w.memStats.pageLoadCount)
			}
		}
	}

	w.url = rawURL
	curl := C.CString(rawURL)
	defer C.free(unsafe.Pointer(curl))
	C.webkit_web_view_load_uri(w.native.wv, (*C.gchar)(curl))
	log.Printf("[webkit] LoadURL: %s", rawURL)
	return nil
}

func (w *WebView) Show() error {
	if w == nil || w.destroyed || w.native == nil || w.native.win == nil {
		return ErrNotImplemented
	}
	C.gtk_widget_set_visible(w.native.view, C.gboolean(1))
	C.gtk_widget_set_visible(w.native.win, C.gboolean(1))
	C.gtk_window_present((*C.GtkWindow)(unsafe.Pointer(w.native.win)))
	log.Printf("[webkit] Show window")
	return nil
}

func (w *WebView) Hide() error {
	if w == nil || w.destroyed || w.native == nil || w.native.win == nil {
		return ErrNotImplemented
	}
	C.gtk_widget_set_visible(w.native.view, C.gboolean(0))
	C.gtk_widget_set_visible(w.native.win, C.gboolean(0))
	return nil
}

func (w *WebView) Destroy() error {
	if w == nil || w.native == nil || w.native.win == nil {
		return ErrNotImplemented
	}

	// Stop periodic GC if running
	w.stopPeriodicGC()

	// Clean up memory pressure settings
	if w.memStats != nil && w.memStats.memoryPressureSettings != nil {
		// WebKit will clean up the memory pressure settings when context is destroyed
		w.memStats.memoryPressureSettings = nil
	}

	// Unregister from memory manager
	if globalMemoryManager != nil {
		globalMemoryManager.UnregisterWebView(w.id)
	}

	// GTK4: destroy windows via gtk_window_destroy
	C.gtk_window_destroy((*C.GtkWindow)(unsafe.Pointer(w.native.win)))
	w.destroyed = true
	unregisterView(w.id)
	log.Printf("[webkit] Destroy window")
	return nil
}

// Window returns the associated native window.
func (w *WebView) Window() *Window { return w.window }

// GetCurrentURL returns the current URI from WebKit.
func (w *WebView) GetCurrentURL() string {
	if w == nil || w.native == nil || w.native.wv == nil {
		return ""
	}
	uri := C.webkit_web_view_get_uri(w.native.wv)
	if uri == nil {
		return ""
	}
	return C.GoString((*C.char)(unsafe.Pointer(uri)))
}

func (w *WebView) GoBack() error {
	if w == nil || w.native == nil || w.native.wv == nil {
		return ErrNotImplemented
	}
	C.webkit_web_view_go_back(w.native.wv)
	return nil
}

func (w *WebView) GoForward() error {
	if w == nil || w.native == nil || w.native.wv == nil {
		return ErrNotImplemented
	}
	C.webkit_web_view_go_forward(w.native.wv)
	return nil
}

// RegisterScriptMessageHandler registers a callback invoked when the content script posts a message.
func (w *WebView) RegisterScriptMessageHandler(cb func(payload string)) { w.msgHandler = cb }

func (w *WebView) dispatchScriptMessage(payload string) {
	if w != nil && w.msgHandler != nil {
		w.msgHandler(payload)
	}
}

// RegisterTitleChangedHandler registers a callback invoked when the page title changes.
func (w *WebView) RegisterTitleChangedHandler(cb func(title string)) { w.titleHandler = cb }

func (w *WebView) dispatchTitleChanged(title string) {
	if w != nil && w.titleHandler != nil {
		w.titleHandler(title)
	}
}

// RegisterURIChangedHandler registers a callback invoked when the current page URI changes.
func (w *WebView) RegisterURIChangedHandler(cb func(uri string)) { w.uriHandler = cb }

func (w *WebView) dispatchURIChanged(uri string) {
	if w != nil && w.uriHandler != nil {
		w.uriHandler(uri)
	}
}

// RegisterZoomChangedHandler registers a callback invoked when zoom level changes.
func (w *WebView) RegisterZoomChangedHandler(cb func(level float64)) { w.zoomHandler = cb }

func (w *WebView) dispatchZoomChanged(level float64) {
	if w != nil && w.zoomHandler != nil {
		w.zoomHandler(level)
	}
}

// enableUserContentManager registers the 'dumber' message handler and injects the omnibox script.
func (w *WebView) enableUserContentManager() {
	if w == nil || w.native == nil || w.native.ucm == nil {
		return
	}
	// Register handler "dumber"
	cname := C.CString("dumber")
	ok := C.register_ucm_handler(w.native.ucm, cname, C.ulong(w.id))
	if ok == 0 {
		log.Printf("[webkit] Failed to register UCM handler; fallback bridge will handle messages")
	}
	C.free(unsafe.Pointer(cname))

	// Inject color-scheme preference script at document-start to inform sites of system theme
	preferDark := C.gtk_prefers_dark() != 0
	if preferDark {
		log.Printf("[theme] GTK prefers: dark")
	} else {
		log.Printf("[theme] GTK prefers: light")
	}
	var schemeJS string
	if preferDark {
		schemeJS = "(() => { try { const d=true; const cs=d?'dark':'light'; console.log('[dumber] color-scheme set:' + cs); try{ window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify({type:'theme', value: cs})) }catch(_){} const meta=document.createElement('meta'); meta.name='color-scheme'; meta.content='dark light'; document.documentElement.appendChild(meta); const s=document.createElement('style'); s.textContent=':root{color-scheme:dark;}'; document.documentElement.appendChild(s); const qD='(prefers-color-scheme: dark)'; const qL='(prefers-color-scheme: light)'; const orig=window.matchMedia; window.matchMedia=function(q){ if(typeof q==='string'&&(q.includes(qD)||q.includes(qL))){ const m={matches:q.includes('dark')?d:!d,media:q,onchange:null,addListener(){},removeListener(){},addEventListener(){},removeEventListener(){},dispatchEvent(){return false;}}; return m;} return orig.call(window,q); }; } catch(e){ console.warn('[dumber] color-scheme injection failed', e) } })();"
	} else {
		schemeJS = "(() => { try { const d=false; const cs=d?'dark':'light'; console.log('[dumber] color-scheme set:' + cs); try{ window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify({type:'theme', value: cs})) }catch(_){} const meta=document.createElement('meta'); meta.name='color-scheme'; meta.content='light dark'; document.documentElement.appendChild(meta); const s=document.createElement('style'); s.textContent=':root{color-scheme:light;}'; document.documentElement.appendChild(s); const qD='(prefers-color-scheme: dark)'; const qL='(prefers-color-scheme: light)'; const orig=window.matchMedia; window.matchMedia=function(q){ if(typeof q==='string'&&(q.includes(qD)||q.includes(qL))){ const m={matches:q.includes('dark')?d:!d,media:q,onchange:null,addListener(){},removeListener(){},addEventListener(){},removeEventListener(){},dispatchEvent(){return false;}}; return m;} return orig.call(window,q); }; } catch(e){ console.warn('[dumber] color-scheme injection failed', e) } })();"
	}
	cScheme := C.CString(schemeJS)
	defer C.free(unsafe.Pointer(cScheme))
	schemeScript := C.webkit_user_script_new((*C.gchar)(cScheme), C.WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
	if schemeScript != nil {
		C.webkit_user_content_manager_add_script(w.native.ucm, schemeScript)
		C.webkit_user_script_unref(schemeScript)
	}

    // Add user script at document-start (omnibox/find reusable component)
    src := C.CString(getOmniboxScript())
	defer C.free(unsafe.Pointer(src))
	script := C.webkit_user_script_new((*C.gchar)(src), C.WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
	if script != nil {
		C.webkit_user_content_manager_add_script(w.native.ucm, script)
		C.webkit_user_script_unref(script)
		log.Printf("[webkit] UCM omnibox script injected at document-start")
	}

	// Add user script at document-start (toast notification system)
	toastSrc := C.CString(getToastScript())
	defer C.free(unsafe.Pointer(toastSrc))
	toastScript := C.webkit_user_script_new((*C.gchar)(toastSrc), C.WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
	if toastScript != nil {
		C.webkit_user_content_manager_add_script(w.native.ucm, toastScript)
		C.webkit_user_script_unref(toastScript)
		log.Printf("[webkit] UCM toast script injected at document-start")
	}

	// Inject Wails runtime fetch interceptor for homepage bridging
	wailsBridge := `(() => { try { const origFetch = window.fetch.bind(window); const waiters = Object.create(null); window.__dumber_wails_resolve = (id, json) => { const w = waiters[id]; if(!w) return; delete waiters[id]; try { const headers = new Headers({"Content-Type":"application/json"}); w.resolve(new Response(json, { headers })); } catch(e){ w.reject(e); } }; window.fetch = (input, init) => { try { const url = new URL(input instanceof Request ? input.url : input, window.location.origin); if (url.pathname === '/wails/runtime') { const args = url.searchParams.get('args'); let payload = {}; try { payload = args ? JSON.parse(args) : {}; } catch(_){} const id = String(Date.now()) + '-' + Math.random().toString(36).slice(2); return new Promise((resolve, reject) => { waiters[id] = { resolve, reject }; try { window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify({ type: 'wails', id, payload })); } catch (e) { reject(e); } }); } } catch(_){} return origFetch(input, init); }; } catch(_){} })();`
	cWails := C.CString(wailsBridge)
	defer C.free(unsafe.Pointer(cWails))
	wailsScript := C.webkit_user_script_new((*C.gchar)(cWails), C.WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
	if wailsScript != nil {
		C.webkit_user_content_manager_add_script(w.native.ucm, wailsScript)
		C.webkit_user_script_unref(wailsScript)
	}

	// No JS fallback bridge — native UCM is active
}

// startPeriodicGC starts a ticker to trigger JavaScript garbage collection periodically
func (w *WebView) startPeriodicGC(interval time.Duration) {
	if w == nil || w.gcTicker != nil {
		return // Already running or invalid
	}

	w.gcTicker = time.NewTicker(interval)

	go func() {
		defer func() {
			if w.gcTicker != nil {
				w.gcTicker.Stop()
				w.gcTicker = nil
			}
		}()

		for {
			select {
			case <-w.gcTicker.C:
				w.triggerGC()
			case <-w.gcDone:
				return
			}
		}
	}()
}

// stopPeriodicGC stops the periodic garbage collection
func (w *WebView) stopPeriodicGC() {
	if w == nil {
		return
	}

	if w.gcDone != nil {
		close(w.gcDone)
		w.gcDone = nil
	}

	if w.gcTicker != nil {
		w.gcTicker.Stop()
		w.gcTicker = nil
	}
}

// triggerGC manually triggers JavaScript garbage collection
func (w *WebView) triggerGC() {
	if w == nil || w.memStats == nil {
		return
	}

	// Get default WebContext and trigger GC
	ctx := C.webkit_web_context_get_default()
	if ctx != nil {
		C.maybe_collect_js(ctx)
		w.memStats.lastGCTime = time.Now()

		if w.config.Memory.EnableMemoryMonitoring {
			log.Printf("[webkit] JavaScript GC triggered (page loads: %d)", w.memStats.pageLoadCount)
		}
	}
}

// GetMemoryStats returns current memory statistics for this WebView
func (w *WebView) GetMemoryStats() map[string]interface{} {
	if w == nil || w.memStats == nil {
		return nil
	}

	return map[string]interface{}{
		"page_load_count":              w.memStats.pageLoadCount,
		"last_gc_time":                 w.memStats.lastGCTime,
		"has_memory_pressure_settings": w.memStats.memoryPressureSettings != nil,
	}
}

// TriggerMemoryCleanup manually triggers garbage collection and memory cleanup
func (w *WebView) TriggerMemoryCleanup() {
	if w == nil {
		return
	}

	w.triggerGC()

	if w.config.Memory.EnableMemoryMonitoring {
		log.Printf("[webkit] Manual memory cleanup triggered")
	}
}

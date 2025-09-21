//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4 javascriptcoregtk-6.0
#cgo CFLAGS: -I/usr/include/webkitgtk-6.0
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <glib-object.h>
#include <gdk/gdk.h>
#include <jsc/jsc.h>
#include <glib.h>
#include <gio/gio.h>
#include "webview_callbacks.h"

static GtkWidget* new_window() { return GTK_WIDGET(gtk_window_new()); }

// Forward declaration
extern void goQuitMainLoop();
extern void goInvokeHandle(uintptr_t handle);
extern gboolean goHandleNewWindowPolicy(unsigned long id, char* uri, int nav_type);
extern void goHandlePopupGeometry(unsigned long id, int x, int y, int width, int height);
extern GtkWidget* goHandleCreateWebView(unsigned long id, char* uri);
extern void goHandleLoadChanged(unsigned long id, char* uri, int load_event);
extern void goHandleWebViewClose(unsigned long id);
static gboolean invoke_handle_cb(gpointer data) {
    goInvokeHandle((uintptr_t)data);
    return G_SOURCE_REMOVE;
}

static void schedule_on_main_thread(uintptr_t handle) {
    g_idle_add_full(G_PRIORITY_DEFAULT, invoke_handle_cb, (gpointer)handle, NULL);
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

// TLS error handling forward declarations
extern gboolean goHandleTLSError(char* failing_uri, char* host, int error_flags, char* cert_info);
static gboolean on_load_failed_with_tls_errors(WebKitWebView *web_view,
                                               const char *failing_uri,
                                               GTlsCertificate *certificate,
                                               GTlsCertificateFlags errors,
                                               gpointer user_data);

// Handle ready-to-show signal for popup windows to properly initialize window properties
static void on_popup_ready_to_show(WebKitWebView* web_view, gpointer user_data) {
    // Get window properties to ensure they're properly initialized
    WebKitWindowProperties* props = webkit_web_view_get_window_properties(web_view);
    if (props) {
        // Access properties to ensure WindowFeatures are initialized
        GdkRectangle geometry;
        webkit_window_properties_get_geometry(props, &geometry);

        // Pass geometry to Go for workspace positioning
        unsigned long view_id = (unsigned long)user_data;
        goHandlePopupGeometry(view_id, geometry.x, geometry.y,
                             geometry.width, geometry.height);
    }
}

// Handle load_changed signal for OAuth completion detection
static void on_load_changed(WebKitWebView *web_view, WebKitLoadEvent load_event, gpointer user_data) {
    const char* uri = webkit_web_view_get_uri(web_view);
    unsigned long view_id = (unsigned long)user_data;

    // Only process WEBKIT_LOAD_FINISHED events
    if (load_event == WEBKIT_LOAD_FINISHED && uri) {
        printf("[webkit-load] Load finished for URI: %s\n", uri);
        goHandleLoadChanged(view_id, (char*)uri, load_event);
    }
}

// Handle close signal when JavaScript calls window.close()
static void on_webview_close(WebKitWebView* web_view, gpointer user_data) {
    unsigned long view_id = (unsigned long)user_data;
    printf("[webkit-close] WebView close signal from view_id: %lu\n", view_id);
    goHandleWebViewClose(view_id);
}

// Handle create signal for popup windows - essential for window.open() support
static GtkWidget* on_create_web_view(WebKitWebView *web_view,
                                     WebKitNavigationAction *navigation_action,
                                     gpointer user_data) {
    WebKitURIRequest *request = webkit_navigation_action_get_request(navigation_action);
    const char* uri = webkit_uri_request_get_uri(request);

    printf("[webkit-create] Create signal for URI: %s\n", uri ? uri : "NULL");

    unsigned long view_id = (unsigned long)user_data;
    GtkWidget* new_webview = goHandleCreateWebView(view_id, (char*)uri);

    if (new_webview) {
        printf("[webkit-create] Go handler provided WebView - using workspace pane\n");

        // Connect load_changed signal to the new popup WebView for OAuth detection
        WebKitWebView* popup_webview = WEBKIT_WEB_VIEW(new_webview);
        g_signal_connect_data(G_OBJECT(popup_webview), "load-changed",
                             G_CALLBACK(on_load_changed), user_data, NULL, 0);
        printf("[webkit-create] Connected load-changed signal to popup for OAuth detection\n");

        // Connect close signal to handle window.close() from OAuth providers
        g_signal_connect_data(G_OBJECT(popup_webview), "close",
                             G_CALLBACK(on_webview_close), user_data, NULL, 0);
        printf("[webkit-create] Connected close signal to popup for OAuth auto-close\n");

        return new_webview;
    } else {
        printf("[webkit-create] Go handler declined - allowing native popup window\n");
        return NULL; // Let WebKit create native popup window
    }
}

// Handle decide-policy signal for new window requests - more robust than create signal
static gboolean on_decide_policy(WebKitWebView *web_view,
                                 WebKitPolicyDecision *decision,
                                 WebKitPolicyDecisionType type,
                                 gpointer user_data) {
    // Debug: log all policy decisions
    const char* type_names[] = {"NAVIGATION_ACTION", "NEW_WINDOW_ACTION", "RESPONSE"};
    const char* type_name = (type >= 0 && type < 3) ? type_names[type] : "UNKNOWN";
    printf("[webkit-policy] Policy decision: type=%s (%d)\n", type_name, type);

    if (type == WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION) {
        WebKitNavigationPolicyDecision *nav_decision =
            WEBKIT_NAVIGATION_POLICY_DECISION(decision);
        WebKitNavigationAction *action =
            webkit_navigation_policy_decision_get_navigation_action(nav_decision);
        WebKitURIRequest *request =
            webkit_navigation_action_get_request(action);
        const char* uri = webkit_uri_request_get_uri(request);

        // Get navigation type to understand how popup was triggered
        WebKitNavigationType nav_type = webkit_navigation_action_get_navigation_type(action);

        // Call Go handler for popup creation
        unsigned long view_id = (unsigned long)user_data;
        gboolean handled = goHandleNewWindowPolicy(view_id, (char*)uri, nav_type);

        if (handled) {
            webkit_policy_decision_ignore(decision);
        } else {
            webkit_policy_decision_use(decision);
        }
        return TRUE;
    }
    return FALSE;
}

static void connect_tls_error_handler(WebKitWebView* wv);


// Dialog callback structure for modern AlertDialog async handling
typedef struct {
    int response;
    gboolean completed;
    GMainLoop *loop;
} AlertDialogResponse;

static void on_alert_dialog_choose_done(GObject *source, GAsyncResult *result, gpointer user_data);
static gboolean show_tls_warning_dialog_sync(GtkWindow *parent, const char* hostname, const char* error_msg);
static char* extract_certificate_info(GTlsCertificate* certificate);

// Setup video acceleration environment variables
static void setup_video_acceleration(const char* driver_name, int enable_all, int legacy) {
    if (legacy) {
        // Legacy mode: force old VA-API plugins, disable modern ones
        setenv("WEBKIT_GST_ENABLE_LEGACY_VAAPI", "1", 1);
        // Don't set other env vars in legacy mode to avoid conflicts
        return;
    }

    // Modern mode (default): use improved VA-API handling
    if (driver_name && strlen(driver_name) > 0) {
        if (strcmp(driver_name, "nvidia") == 0) {
            // NVIDIA uses different env vars for hardware video acceleration
            setenv("VDPAU_DRIVER", "nvidia", 1);
            setenv("LIBVA_DRIVER_NAME", "vdpau", 1);
        } else if (strcmp(driver_name, "vdpau") == 0) {
            // Explicit VDPAU backend (for NVIDIA via VA-API)
            setenv("LIBVA_DRIVER_NAME", "vdpau", 1);
        } else {
            // AMD/Intel use VA-API directly
            setenv("LIBVA_DRIVER_NAME", driver_name, 1);
        }
    }

    if (enable_all) {
        setenv("GST_VAAPI_ALL_DRIVERS", "1", 1);
    }
}

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
    // Configure TLS errors to emit signals instead of failing silently
    webkit_network_session_set_tls_errors_policy(sess, WEBKIT_TLS_ERRORS_POLICY_FAIL);
    GtkWidget* w = GTK_WIDGET(g_object_new(WEBKIT_TYPE_WEB_VIEW,
        "user-content-manager", u,
        "network-session", sess,
        NULL));
    // WebView holds refs to provided objects; drop our temporary refs
    g_object_unref(sess);
    // Set WebView background to black (easier on eyes when pages are loading)
    GdkRGBA black_color = { 0.0, 0.0, 0.0, 1.0 };
    webkit_web_view_set_background_color(WEBKIT_WEB_VIEW(w), &black_color);

    // Connect TLS error handler
    connect_tls_error_handler(WEBKIT_WEB_VIEW(w));

    if (out_ucm) { *out_ucm = u; }
    return w;
}

// Create a new WebView related to an existing one for process sharing
static GtkWidget* new_webview_related(WebKitWebView* related_view, WebKitUserContentManager** out_ucm) {
    if (!related_view) {
        // Fallback to regular WebView creation - we need to provide the required parameters
        return new_webview_with_ucm_and_session(NULL, NULL, NULL, out_ucm, NULL);
    }

    // Get the network session and user content manager from the related view
    WebKitNetworkSession* related_session = webkit_web_view_get_network_session(related_view);
    WebKitUserContentManager* related_ucm = webkit_web_view_get_user_content_manager(related_view);

    // Create content manager if needed
    WebKitUserContentManager* u = related_ucm;
    if (!u) {
        u = webkit_user_content_manager_new();
    } else {
        g_object_ref(u); // Add ref since we'll pass it to new WebView
    }

    // Create WebView with related view for process sharing (network session is inherited)
    GtkWidget* w = GTK_WIDGET(g_object_new(WEBKIT_TYPE_WEB_VIEW,
        "user-content-manager", u,
        "related-view", related_view,
        NULL));

    // Connect TLS error handler
    connect_tls_error_handler(WEBKIT_WEB_VIEW(w));

    // WebView holds refs to provided objects; drop our temporary refs
    g_object_unref(u);
    if (out_ucm) { *out_ucm = u; }
    return w;
}

static void connect_policy_handler(WebKitWebView* web_view, unsigned long id) {
    if (!web_view) return;
    g_signal_connect_data(G_OBJECT(web_view), "decide-policy", G_CALLBACK(on_decide_policy), (gpointer)id, NULL, 0);
    // Connect create signal for popup window support
    g_signal_connect_data(G_OBJECT(web_view), "create", G_CALLBACK(on_create_web_view), (gpointer)id, NULL, 0);
}

// Connect close signal to a WebView for popup auto-close functionality
static void connect_close_signal(WebKitWebView* web_view, unsigned long id) {
    if (!web_view) return;
    g_signal_connect_data(G_OBJECT(web_view), "close", G_CALLBACK(on_webview_close), (gpointer)id, NULL, 0);
    printf("[webkit-connect] Connected close signal to WebView with id: %lu\n", id);
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

extern void goOnUcmMessage(unsigned long id, char* json);

// Helper functions to connect signals using static callbacks
static void connect_title_notify(GtkWidget* widget, GtkWindow* window) {
    if (!widget) return;
    g_signal_connect_data(G_OBJECT(widget), "notify::title", G_CALLBACK(on_title_notify), G_OBJECT(window), NULL, 0);
}

static void connect_title_notify_with_id(GtkWidget* widget, unsigned long id) {
    if (!widget) return;
    g_signal_connect_data(G_OBJECT(widget), "notify::title", G_CALLBACK(on_title_notify_id), (gpointer)id, NULL, 0);
}

static void connect_uri_notify_with_id(GtkWidget* widget, unsigned long id) {
    if (!widget) return;
    g_signal_connect_data(G_OBJECT(widget), "notify::uri", G_CALLBACK(on_uri_notify), (gpointer)id, NULL, 0);
}

static void connect_theme_changed_with_id(GtkSettings* settings, unsigned long id) {
    if (!settings) return;
    g_signal_connect_data(G_OBJECT(settings), "notify::gtk-application-prefer-dark-theme", G_CALLBACK(on_theme_changed), (gpointer)id, NULL, 0);
}

static void register_uri_scheme_handler(WebKitWebContext* context, const char* scheme) {
    if (!context || !scheme) return;
    webkit_web_context_register_uri_scheme(context, scheme, (WebKitURISchemeRequestCallback)on_uri_scheme, NULL, NULL);
}
static void register_uri_scheme_security(WebKitWebContext* context, const char* scheme) {
	if (!context || !scheme) return;
	WebKitSecurityManager* sm = webkit_web_context_get_security_manager(context);
	if (!sm) return;

	// Mark as secure to avoid mixed-content warnings when requested by https pages
	webkit_security_manager_register_uri_scheme_as_secure(sm, scheme);
	// Treat as local scheme
	webkit_security_manager_register_uri_scheme_as_local(sm, scheme);
	// Allow cross-origin requests to this scheme (bypass CORS header checks for this scheme)
	webkit_security_manager_register_uri_scheme_as_cors_enabled(sm, scheme);
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

// Register a script message handler for a specific JavaScript world
static gboolean register_ucm_handler_world(WebKitUserContentManager* ucm, const char* name, const char* world_name, unsigned long id) {
	if (!ucm || !name || !world_name) return FALSE;
	webkit_user_content_manager_register_script_message_handler(ucm, name, world_name);
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

static void maybe_set_smooth_scrolling(WebKitSettings* settings, int enable) {
#if WEBKIT_CHECK_VERSION(2,10,0)
    if (!settings) return;
    webkit_settings_set_enable_smooth_scrolling(settings, enable ? TRUE : FALSE);
#else
    (void)settings; (void)enable;
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

// TLS error handling implementation
static gboolean on_load_failed_with_tls_errors(WebKitWebView *web_view,
                                               const char *failing_uri,
                                               GTlsCertificate *certificate,
                                               GTlsCertificateFlags errors,
                                               gpointer user_data) {
    (void)user_data;

    // Extract host from URI for the Go callback
    const char* host = failing_uri;
    char* host_copy = NULL;

    if (strstr(failing_uri, "://")) {
        host = strstr(failing_uri, "://") + 3;
        char* slash = strchr(host, '/');
        if (slash) {
            // Create a temporary null-terminated host string
            size_t host_len = slash - host;
            host_copy = malloc(host_len + 1);
            if (host_copy) {
                strncpy(host_copy, host, host_len);
                host_copy[host_len] = '\0';
                host = host_copy;
            }
        }
    }

    // Extract certificate information
    char* cert_info = extract_certificate_info(certificate);

    gboolean should_proceed = goHandleTLSError((char*)failing_uri, (char*)host, (int)errors, cert_info);

    // If user accepted, allow the certificate for this host
    if (should_proceed && certificate) {
        printf("[dumber] User accepted certificate - adding exception and triggering new load\n");
        fflush(stdout);
        // Get the network session from the web view
        WebKitNetworkSession* session = webkit_web_view_get_network_session(web_view);
        if (session) {
            webkit_network_session_allow_tls_certificate_for_host(session, certificate, host);
            printf("[dumber] Certificate exception added for host: %s\n", host);
            fflush(stdout);

            // Try loading the URL again after adding the exception
            webkit_web_view_load_uri(web_view, failing_uri);
            printf("[dumber] Triggered new load of %s with certificate exception\n", failing_uri);
            fflush(stdout);
        }
    }

    // Free the certificate info string
    if (cert_info) {
        g_free(cert_info);
    }

    if (host_copy) {
        free(host_copy);
    }

    return should_proceed;
}

// Extract certificate information for display
static char* extract_certificate_info(GTlsCertificate* certificate) {
    if (!certificate) {
        return g_strdup("Certificate information not available");
    }

    printf("[dumber] Extracting certificate information...\n");
    fflush(stdout);

    GString* info = g_string_new("Certificate Information:\n");

    // Try to get certificate properties - but these might not be available in all GTK versions
    gchar* subject = NULL;
    gchar* issuer = NULL;
    GDateTime* not_valid_before = NULL;
    GDateTime* not_valid_after = NULL;

    // Try property access but handle failures gracefully
    g_object_get(certificate,
                 "subject-name", &subject,
                 "issuer-name", &issuer,
                 "not-valid-before", &not_valid_before,
                 "not-valid-after", &not_valid_after,
                 NULL);

    if (subject) {
        g_string_append_printf(info, "Subject: %s\n", subject);
        g_free(subject);
        printf("[dumber] Found subject info\n");
    } else {
        g_string_append(info, "Subject: Information not available\n");
    }

    if (issuer) {
        g_string_append_printf(info, "Issued by: %s\n", issuer);
        g_free(issuer);
        printf("[dumber] Found issuer info\n");
    } else {
        g_string_append(info, "Issued by: Information not available\n");
    }

    if (not_valid_before) {
        gchar* date_str = g_date_time_format(not_valid_before, "%Y-%m-%d %H:%M:%S UTC");
        g_string_append_printf(info, "Valid from: %s\n", date_str);
        g_free(date_str);
        g_date_time_unref(not_valid_before);
    } else {
        g_string_append(info, "Valid from: Information not available\n");
    }

    if (not_valid_after) {
        gchar* date_str = g_date_time_format(not_valid_after, "%Y-%m-%d %H:%M:%S UTC");
        g_string_append_printf(info, "Valid until: %s\n", date_str);
        g_free(date_str);
        g_date_time_unref(not_valid_after);
    } else {
        g_string_append(info, "Valid until: Information not available\n");
    }

    char* result = g_string_free(info, FALSE);
    printf("[dumber] Certificate info: %s\n", result);
    fflush(stdout);
    return result;
}

static void connect_tls_error_handler(WebKitWebView* wv) {
    if (!wv) return;
    g_signal_connect(G_OBJECT(wv), "load-failed-with-tls-errors",
                     G_CALLBACK(on_load_failed_with_tls_errors), NULL);
}

// Modern AlertDialog async callback
static void on_alert_dialog_choose_done(GObject *source, GAsyncResult *result, gpointer user_data) {
    AlertDialogResponse *data = (AlertDialogResponse*)user_data;
    GError *error = NULL;
    data->response = gtk_alert_dialog_choose_finish(GTK_ALERT_DIALOG(source), result, &error);
    if (error) {
        printf("[dumber] AlertDialog error: %s\n", error->message);
        g_error_free(error);
        data->response = 0;
    }
    data->completed = TRUE;
    const char *response_names[] = {"GO_BACK", "PROCEED_ONCE", "ALWAYS_ACCEPT"};
    const char *response_name = (data->response >= 0 && data->response <= 2) ?
                               response_names[data->response] : "UNKNOWN";
    printf("[dumber] TLS dialog response: %s (button_index=%d)\n", response_name, data->response);
    fflush(stdout);
    if (data->loop) {
        g_main_loop_quit(data->loop);
    }
}

// Show TLS warning dialog synchronously using modern GtkAlertDialog API
static gboolean show_tls_warning_dialog_sync(GtkWindow *parent, const char* hostname, const char* error_msg) {
    printf("[dumber] Creating TLS warning dialog for hostname: %s\n", hostname);
    printf("[dumber] Error message: %s\n", error_msg);
    fflush(stdout);

    // Create the modern AlertDialog
    GtkAlertDialog *dialog = gtk_alert_dialog_new("Certificate Error for %s", hostname);

    // Set modal behavior and error details
    gtk_alert_dialog_set_modal(dialog, TRUE);
    gtk_alert_dialog_set_detail(dialog, error_msg);

    // Configure buttons (Go Back = 0, Proceed Once = 1, Always Accept = 2)
    const char *buttons[] = {"Go Back", "Proceed Once (Unsafe)", "Always Accept This Site", NULL};
    gtk_alert_dialog_set_buttons(dialog, buttons);
    gtk_alert_dialog_set_default_button(dialog, 0);
    gtk_alert_dialog_set_cancel_button(dialog, 0);

    // Set up response data with main loop for sync behavior
    AlertDialogResponse response_data = {0, FALSE, g_main_loop_new(NULL, FALSE)};

    // Start async operation
    gtk_alert_dialog_choose(dialog, parent, NULL,
                           on_alert_dialog_choose_done, &response_data);

    // Run event loop until dialog completes
    g_main_loop_run(response_data.loop);
    g_main_loop_unref(response_data.loop);

    // Clean up
    g_object_unref(dialog);

    // Return response code: 0=Go Back, 1=Proceed Once, 2=Always Accept
    return response_data.response;
}

*/
import "C"

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/cgo"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/logging"
	_ "github.com/ncruces/go-sqlite3"
)

// FilterManager interface for content blocking integration
type FilterManager interface {
	GetNetworkFilters() ([]byte, error)
	GetCosmeticScript() string
	GetCosmeticScriptForDomain(domain string) string
}

const domBridgeTemplate = `(() => { try {
              if (window.__dumber_page_bridge_installed) return; 
              window.__dumber_page_bridge_installed = true;
              
              const __dumberInitialZoom = __DOM_ZOOM_DEFAULT__;
              window.__dumber_dom_zoom_seed = __dumberInitialZoom;
              
              // Theme setter function for GTK theme integration
              window.__dumber_setTheme = (theme) => {
                window.__dumber_initial_theme = theme;
                console.log('[dumber] Setting theme to:', theme);
                if (theme === 'dark') {
                  document.documentElement.classList.add('dark');
                } else {
                  document.documentElement.classList.remove('dark');
                }
              };
              
              // Ensure unified API object
              window.__dumber = window.__dumber || {};
              // Toast namespace
              window.__dumber.toast = window.__dumber.toast || {
                show: function(message, duration, type) {
                  try {
                    document.dispatchEvent(new CustomEvent('dumber:toast:show', { detail: { message, duration, type } }));
                    // Legacy compatibility
                    document.dispatchEvent(new CustomEvent('dumber:showToast', { detail: { message, duration, type } }));
                  } catch(e) { /* ignore */ }
                },
                zoom: function(level) {
                  try {
                    document.dispatchEvent(new CustomEvent('dumber:toast:zoom', { detail: { level } }));
                    // Legacy compatibility
                    document.dispatchEvent(new CustomEvent('dumber:showZoomToast', { detail: { level } }));
                  } catch(e) { /* ignore */ }
                }
              };
              // Back-compat global helpers
              window.__dumber_showToast = function(message, duration, type) { window.__dumber.toast.show(message, duration, type); };
              window.__dumber_showZoomToast = function(level) { window.__dumber.toast.zoom(level); };

              // DOM zoom helpers (optional native override)
              if (typeof window.__dumber_dom_zoom_level !== 'number') {
                window.__dumber_dom_zoom_level = __dumberInitialZoom;
              }
              const __dumberApplyZoomStyles = function(node, level) {
                if (!node) return;
                if (Math.abs(level - 1.0) < 1e-6) {
                  node.style.removeProperty('zoom');
                  node.style.removeProperty('transform');
                  node.style.removeProperty('transform-origin');
                  node.style.removeProperty('width');
                  node.style.removeProperty('min-width');
                  node.style.removeProperty('height');
                  node.style.removeProperty('min-height');
                  return;
                }
                const scale = level;
                const inversePercent = 100 / scale;
                const widthValue = inversePercent + '%';
                node.style.removeProperty('zoom');
                node.style.transform = 'scale(' + scale + ')';
                node.style.transformOrigin = '0 0';
                node.style.width = widthValue;
                node.style.minWidth = widthValue;
                node.style.minHeight = '100%';
              };
              window.__dumber_applyDomZoom = function(level) {
                try {
                  window.__dumber_dom_zoom_level = level;
                  window.__dumber_dom_zoom_seed = level;
                  __dumberApplyZoomStyles(document.documentElement, level);
                  if (document.body) {
                    __dumberApplyZoomStyles(document.body, level);
                  }
                } catch (e) {
                  console.error('[dumber] DOM zoom error', e);
                }
              };
              // Apply immediately so first paint uses the desired zoom, then reapply when body exists.
              window.__dumber_applyDomZoom(window.__dumber_dom_zoom_level);
              if (!document.body) {
                document.addEventListener('DOMContentLoaded', function() {
                  if (typeof window.__dumber_dom_zoom_level === 'number') {
                    window.__dumber_applyDomZoom(window.__dumber_dom_zoom_level);
                  }
                }, { once: true });
              }

              // Omnibox suggestions bridge (with queue before ready)
              let __omniboxQueue = [];
              let __omniboxReady = false;
              function __omniboxDispatch(suggestions) {
                try {
                  document.dispatchEvent(new CustomEvent('dumber:omnibox:suggestions', { detail: { suggestions } }));
                  // Legacy compatibility event name
                  document.dispatchEvent(new CustomEvent('dumber:omnibox-suggestions', { detail: { suggestions } }));
                } catch (e) { /* ignore */ }
              }
              window.__dumber_omnibox_suggestions = function(suggestions) {
                if (__omniboxReady) {
                  __omniboxDispatch(suggestions);
                } else {
                  try { __omniboxQueue.push(suggestions); } catch (_) { /* ignore */ }
                }
              };
              document.addEventListener('dumber:omnibox-ready', function() {
                __omniboxReady = true;
                if (__omniboxQueue && __omniboxQueue.length) {
                  const items = __omniboxQueue.slice();
                  __omniboxQueue.length = 0;
                  for (const s of items) __omniboxDispatch(s);
                }
              });
              // Unified omnibox API for page-world callers
              window.__dumber.omnibox = window.__dumber.omnibox || {
                suggestions: function(suggestions) { window.__dumber_omnibox_suggestions(suggestions); }
              };
            } catch (e) { console.warn('[dumber] unified bridge init failed', e); } })();`

var registerSchemeOnce sync.Once

func registerDumbScheme(ctx *C.WebKitWebContext) {
	if ctx == nil {
		return
	}

	registerSchemeOnce.Do(func() {
		sch := C.CString("dumb")
		defer C.free(unsafe.Pointer(sch))
		C.register_uri_scheme_handler(ctx, sch)
		C.register_uri_scheme_security(ctx, sch)
	})
}

// Helper function to convert bool to int for C interop
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

//export goInvokeHandle
func goInvokeHandle(handle C.uintptr_t) {
	h := cgo.Handle(handle)
	if fn, ok := h.Value().(func()); ok {
		fn()
	}
	h.Delete()
}

// getCertificateHash generates a SHA256 hash of the certificate info for storage
func getCertificateHash(certificateInfo string) string {
	hash := sha256.Sum256([]byte(certificateInfo))
	return fmt.Sprintf("%x", hash)
}

// getDBConnection opens a database connection and returns queries
func getDBConnection() (*db.Queries, *sql.DB, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, nil, fmt.Errorf("config not available")
	}

	dbPath := cfg.Database.Path
	if dbPath == "" {
		var err error
		dbPath, err = config.GetDatabaseFile()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get database path: %w", err)
		}
	}

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	queries := db.New(database)
	return queries, database, nil
}

// checkStoredCertificateDecision checks if user has previously decided on this certificate
func checkStoredCertificateDecision(hostname, certificateInfo string) (string, bool) {
	queries, database, err := getDBConnection()
	if err != nil {
		log.Printf("[tls] Failed to connect to database: %v", err)
		return "", false
	}
	defer database.Close()

	certHash := getCertificateHash(certificateInfo)
	ctx := context.Background()

	validation, err := queries.GetCertificateValidation(ctx, hostname, certHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false // No previous decision
		}
		log.Printf("[tls] Failed to query certificate validation: %v", err)
		return "", false
	}

	log.Printf("[tls] Found stored certificate decision for %s: %s", hostname, validation.UserDecision)
	return validation.UserDecision, true
}

// storeCertificateDecision stores the user's decision about a certificate
func storeCertificateDecision(hostname, certificateInfo, decision string, permanent bool) error {
	queries, database, err := getDBConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer database.Close()

	certHash := getCertificateHash(certificateInfo)
	ctx := context.Background()

	var expiresAt sql.NullTime
	if !permanent {
		// For temporary decisions, expire after 24 hours
		expiresAt = sql.NullTime{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		}
	}

	err = queries.StoreCertificateValidation(ctx, hostname, certHash, decision, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to store certificate validation: %w", err)
	}

	log.Printf("[tls] Stored certificate decision for %s: %s (permanent: %t)", hostname, decision, permanent)
	return nil
}

type nativeView struct {
	win       *C.GtkWidget
	container *C.GtkWidget
	view      *C.GtkWidget
	wv        *C.WebKitWebView
	ucm       *C.WebKitUserContentManager
}

type memoryStats struct {
	pageLoadCount          int
	lastGCTime             time.Time
	memoryPressureSettings *C.WebKitMemoryPressureSettings
}

// WebView represents a browser view powered by WebKit2GTK.
type WebView struct {
	config          *Config
	zoom            float64
	url             string
	destroyed       bool
	native          *nativeView
	window          *Window
	id              uintptr
	msgHandler      func(payload string)
	titleHandler    func(title string)
	uriHandler      func(uri string)
	zoomHandler     func(level float64)
	popupHandler func(string) *WebView
	closeHandler func()
	memStats     *memoryStats
	gcTicker        *time.Ticker
	gcDone          chan struct{}
	tlsExceptions   map[string]bool // host -> whether user allowed certificate exception
	useDomZoom      bool
	domZoomSeed     float64
	domBridgeScript *C.WebKitUserScript
	wasReparented   bool // Track if this WebView has been reparented
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

	// Setup video acceleration environment variables before creating WebView
	if cfg.VideoAcceleration.EnableVAAPI {
		driverName := C.CString(cfg.VideoAcceleration.VAAPIDriverName)
		enableAll := 0
		if cfg.VideoAcceleration.EnableAllDrivers {
			enableAll = 1
		}
		legacy := 0
		if cfg.VideoAcceleration.LegacyVAAPI {
			legacy = 1
		}
		C.setup_video_acceleration(driverName, C.int(enableAll), C.int(legacy))
		C.free(unsafe.Pointer(driverName))
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

	registerDumbScheme(ctx)
	// Cookie manager persistent storage handled via NetworkSession in new_webview_with_ucm_and_session

	// Create WebView through g_object_new with a fresh UCM (GTK4/WebKit 6 style)
	var createdUcm *C.WebKitUserContentManager
	viewWidget := C.new_webview_with_ucm_and_session(cData, cCache, cCookie, &createdUcm, pressureSettings)
	if viewWidget == nil {
		return nil, errors.New("failed to create WebKitWebView")
	}

	var win *C.GtkWidget
	var window *Window
	container := C.gtk_box_new(C.GtkOrientation(C.GTK_ORIENTATION_VERTICAL), 0)
	if container == nil {
		return nil, errors.New("failed to create GtkBox container")
	}
	// Hold a strong reference so the WebView keeps the container alive across reparenting.
	C.g_object_ref_sink(C.gpointer(container))
	C.gtk_widget_set_hexpand(container, C.gboolean(1))
	C.gtk_widget_set_vexpand(container, C.gboolean(1))
	C.gtk_widget_set_hexpand(viewWidget, C.gboolean(1))
	C.gtk_widget_set_vexpand(viewWidget, C.gboolean(1))
	C.gtk_box_append((*C.GtkBox)(unsafe.Pointer(container)), viewWidget)

	// Only create window if requested
	if cfg.CreateWindow {
		// Create a top-level window to host the view
		win = C.new_window()
		if win == nil {
			return nil, errors.New("failed to create GtkWindow")
		}

		// Pack view widget into the window
		// GTK4: containers removed; use gtk_window_set_child
		C.gtk_window_set_child((*C.GtkWindow)(unsafe.Pointer(win)), container)
		C.gtk_window_set_default_size((*C.GtkWindow)(unsafe.Pointer(win)), 1024, 768)
		C.connect_destroy_quit(win)
		window = &Window{win: win}
	}

	seed := 1.0
	if cfg.ZoomDefault > 0 {
		seed = cfg.ZoomDefault
	}

	v := &WebView{
		config:      cfg,
		zoom:        1.0,
		useDomZoom:  cfg.UseDomZoom,
		domZoomSeed: seed,
		native:      &nativeView{win: win, container: container, view: viewWidget, wv: C.as_webview(viewWidget), ucm: createdUcm},
		window:      window,
		memStats: &memoryStats{
			pageLoadCount:          0,
			lastGCTime:             time.Now(),
			memoryPressureSettings: pressureSettings,
		},
		gcDone:        make(chan struct{}),
		tlsExceptions: make(map[string]bool),
	}
	// Assign an ID for accelerator dispatch
	v.id = nextViewID()
	registerView(v.id, v)
	C.connect_policy_handler(v.native.wv, C.ulong(v.id))

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
		C.connect_theme_changed_with_id(settings, C.ulong(v.id))
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
	// Find in page (Ctrl/Cmd+F): handled by window-level shortcuts in window_shortcuts.go
	// Navigation with Ctrl/Cmd + Arrow keys: forward to keyboard service bridge
	_ = v.RegisterKeyboardShortcut("cmdorctrl+ArrowLeft", func() {
		_ = v.InjectScript("document.dispatchEvent(new CustomEvent('dumber:key',{detail:{shortcut:'cmdorctrl+arrowleft'}}))")
	})
	_ = v.RegisterKeyboardShortcut("cmdorctrl+ArrowRight", func() {
		_ = v.InjectScript("document.dispatchEvent(new CustomEvent('dumber:key',{detail:{shortcut:'cmdorctrl+arrowright'}}))")
	})
	// Update window title when page title changes
	C.connect_title_notify(viewWidget, (*C.GtkWindow)(unsafe.Pointer(win)))
	// Also dispatch title change to Go with view id
	C.connect_title_notify_with_id(viewWidget, C.ulong(v.id))
	// Notify URI changes to Go to keep current URL in sync
	C.connect_uri_notify_with_id(viewWidget, C.ulong(v.id))
	// Apply hardware acceleration and related settings based on cfg.Rendering
	if settings := C.webkit_web_view_get_settings(v.native.wv); settings != nil {
		// Hardware acceleration policy (guarded by version)
		switch cfg.Rendering.Mode {
		case "gpu":
			log.Printf("[webkit] Applying GPU rendering mode")
			C.maybe_set_hw_policy(settings, 1)
			C.maybe_set_webgl(settings, 1)
			C.maybe_set_canvas_accel(settings, 1)
		case "cpu":
			log.Printf("[webkit] Applying CPU rendering mode")
			C.maybe_set_hw_policy(settings, 2)
			C.maybe_set_webgl(settings, 0)
			C.maybe_set_canvas_accel(settings, 0)
		default: // auto
			log.Printf("[webkit] Applying AUTO rendering mode - detecting actual backend...")
			C.maybe_set_hw_policy(settings, 0)
			C.maybe_set_webgl(settings, 1)
			C.maybe_set_canvas_accel(settings, 1)
			// Detect actual rendering backend after applying auto settings
			v.detectAndLogRenderingBackend()
		}
		// Optional compositing indicators for debugging (guarded by version)
		if cfg.Rendering.DebugGPU {
			C.maybe_set_draw_indicators(settings, 1)
		}
		// Reduce media pipeline churn by requiring a user gesture for playback
		C.maybe_set_media_user_gesture(settings, 1)
		// Enable trackpad back/forward gestures when available
		C.maybe_set_back_forward_gestures(settings, 1)
		// Smooth scrolling for wheel/pinch gestures when supported
		C.maybe_set_smooth_scrolling(settings, 1)

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

	// Enable JavaScript popup windows for workspace popup handling
	settings := C.webkit_web_view_get_settings(v.native.wv)
	if settings != nil {
		C.webkit_settings_set_javascript_can_open_windows_automatically(settings, C.gboolean(1))
		log.Printf("[webkit] Enabled JavaScript popup windows")
	}

	// Apply custom User-Agent for codec negotiation
	if settings := C.webkit_web_view_get_settings(v.native.wv); settings != nil {
		if cfg.CodecPreferences.CustomUserAgent != "" {
			cUserAgent := C.CString(cfg.CodecPreferences.CustomUserAgent)
			C.webkit_settings_set_user_agent(settings, (*C.gchar)(cUserAgent))
			C.free(unsafe.Pointer(cUserAgent))
			log.Printf("[webkit] Set custom User-Agent: %s", cfg.CodecPreferences.CustomUserAgent)
		} else if cfg.CodecPreferences.ForceAV1 {
			// Use modern Chrome UA that signals AV1 support
			av1UA := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
			cAV1UA := C.CString(av1UA)
			C.webkit_settings_set_user_agent(settings, (*C.gchar)(cAV1UA))
			C.free(unsafe.Pointer(cAV1UA))
			log.Printf("[webkit] Set AV1-optimized User-Agent")
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
		v.enableUserContentManager(cfg)
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

// NewWebViewWithRelated creates a new WebView related to an existing one for process sharing
func NewWebViewWithRelated(cfg *Config, relatedView *WebView) (*WebView, error) {
	if relatedView == nil || relatedView.native == nil || relatedView.native.wv == nil {
		// Fallback to regular WebView creation
		return NewWebView(cfg)
	}

	log.Printf("[webkit] Creating related WebView for process sharing (CGO)")
	if C.gtk_init_check() == 0 {
		return nil, errors.New("failed to initialize GTK")
	}

	if cfg == nil {
		cfg = &Config{}
	}

	// Create WebView related to the existing one
	var createdUcm *C.WebKitUserContentManager
	viewWidget := C.new_webview_related(relatedView.native.wv, &createdUcm)
	if viewWidget == nil {
		return nil, errors.New("failed to create related WebKitWebView")
	}

	var win *C.GtkWidget
	var window *Window
	container := C.gtk_box_new(C.GtkOrientation(C.GTK_ORIENTATION_VERTICAL), 0)
	if container == nil {
		return nil, errors.New("failed to create GtkBox container")
	}

	C.gtk_box_append((*C.GtkBox)(unsafe.Pointer(container)), viewWidget)

	// Make the WebView widget visible and expandable for popup rendering
	C.gtk_widget_set_visible(viewWidget, C.gboolean(1))
	C.gtk_widget_set_visible(container, C.gboolean(1))
	C.gtk_widget_set_hexpand(viewWidget, C.gboolean(1))
	C.gtk_widget_set_vexpand(viewWidget, C.gboolean(1))
	C.gtk_widget_set_hexpand(container, C.gboolean(1))
	C.gtk_widget_set_vexpand(container, C.gboolean(1))
	// Remove any size constraints from popup WebView to let it fill the pane
	C.gtk_widget_set_size_request(viewWidget, -1, -1)
	C.gtk_widget_set_size_request(container, -1, -1)

	if cfg.CreateWindow {
		win = C.new_window()
		if win == nil {
			return nil, errors.New("failed to create native window")
		}
		C.connect_destroy_quit(win)
		C.gtk_window_set_child((*C.GtkWindow)(unsafe.Pointer(win)), container)
		window = &Window{win: win}
	} else {
		window = &Window{}
	}

	v := &WebView{
		config: cfg,
		window: window,
		zoom:   cfg.ZoomDefault,
		native: &nativeView{
			win:       win,
			container: container,
			view:      viewWidget,
			wv:        (*C.WebKitWebView)(unsafe.Pointer(viewWidget)),
			ucm:       createdUcm,
		},
		useDomZoom:    cfg.UseDomZoom,
		domZoomSeed:   cfg.ZoomDefault,
		gcDone:        make(chan struct{}),
		tlsExceptions: make(map[string]bool),
	}

	// Enable JavaScript popup windows for workspace popup handling
	settings := C.webkit_web_view_get_settings(v.native.wv)
	if settings != nil {
		C.webkit_settings_set_javascript_can_open_windows_automatically(settings, C.gboolean(1))
		log.Printf("[webkit] Enabled JavaScript popup windows for related WebView")
	}

	// Assign an ID for accelerator dispatch
	v.id = nextViewID()
	registerView(v.id, v)
	C.connect_policy_handler(v.native.wv, C.ulong(v.id))

	// Register with memory manager if enabled
	if cfg.Memory.EnableMemoryMonitoring {
		if globalMemoryManager == nil {
			InitializeGlobalMemoryManager(
				cfg.Memory.EnableMemoryMonitoring,
				cfg.Memory.ProcessRecycleThreshold,
				30*time.Second,
			)
		}
		if globalMemoryManager != nil {
			globalMemoryManager.RegisterWebView(v)
		}
	}

	if cfg.ZoomDefault <= 0 {
		v.zoom = 1.0
	}
	v.domZoomSeed = v.zoom

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
	if w == nil || w.destroyed || w.native == nil {
		return ErrNotImplemented
	}
	if w.native.container != nil {
		C.gtk_widget_set_visible(w.native.container, C.gboolean(1))
	}
	if w.native.view != nil {
		C.gtk_widget_set_visible(w.native.view, C.gboolean(1))
	}
	if w.native.win == nil {
		return ErrNotImplemented
	}
	C.gtk_widget_set_visible(w.native.win, C.gboolean(1))
	C.gtk_window_present((*C.GtkWindow)(unsafe.Pointer(w.native.win)))
	log.Printf("[webkit] Show window")
	return nil
}

func (w *WebView) Hide() error {
	if w == nil || w.destroyed || w.native == nil {
		return ErrNotImplemented
	}
	if w.native.container != nil {
		C.gtk_widget_set_visible(w.native.container, C.gboolean(0))
	}
	if w.native.view != nil {
		C.gtk_widget_set_visible(w.native.view, C.gboolean(0))
	}
	if w.native.win == nil {
		return ErrNotImplemented
	}
	C.gtk_widget_set_visible(w.native.win, C.gboolean(0))
	return nil
}

func (w *WebView) Destroy() error {
	if w == nil || w.native == nil {
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

	if w.native != nil && w.native.ucm != nil && w.domBridgeScript != nil {
		C.webkit_user_content_manager_remove_script(w.native.ucm, w.domBridgeScript)
		C.webkit_user_script_unref(w.domBridgeScript)
		w.domBridgeScript = nil
	}

	// GTK4: destroy windows via gtk_window_destroy if we created one
	if w.native.win != nil {
		C.gtk_window_destroy((*C.GtkWindow)(unsafe.Pointer(w.native.win)))
		w.native.win = nil
		if w.window != nil {
			w.window.win = nil
		}
		log.Printf("[webkit] Destroy window")
	}

	w.releaseNativeWidgets()
	w.destroyed = true
	unregisterView(w.id)
	return nil
}

func (w *WebView) releaseNativeWidgets() {
	if w == nil || w.native == nil {
		return
	}
	if w.native.container != nil {
		C.g_object_unref(C.gpointer(w.native.container))
		w.native.container = nil
	}
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

func (w *WebView) Reload() error {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return ErrNotImplemented
	}
	C.webkit_web_view_reload(w.native.wv)
	log.Printf("[webkit] Reload page")
	return nil
}

func (w *WebView) ReloadBypassCache() error {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return ErrNotImplemented
	}
	C.webkit_web_view_reload_bypass_cache(w.native.wv)
	log.Printf("[webkit] Hard reload page (bypass cache)")
	return nil
}

// RegisterScriptMessageHandler registers a callback invoked when the content script posts a message.
func (w *WebView) RegisterScriptMessageHandler(cb func(payload string)) { w.msgHandler = cb }

func (w *WebView) RegisterPopupHandler(cb func(string) *WebView) { w.popupHandler = cb }

func (w *WebView) RegisterCloseHandler(cb func()) {
	w.closeHandler = cb
	// Also connect the CGO close signal to ensure window.close() is detected
	if w.native != nil && w.native.wv != nil {
		C.connect_close_signal(w.native.wv, C.ulong(w.id))
	}
}

func (w *WebView) dispatchScriptMessage(payload string) {
	if w != nil && w.msgHandler != nil {
		w.msgHandler(payload)
	}
}

func (w *WebView) dispatchPopupRequest(uri string) *WebView {
	if w != nil && w.popupHandler != nil {
		return w.popupHandler(uri)
	}
	return nil
}

// GetNativePointer returns the native WebView pointer for CGO callbacks
func (w *WebView) GetNativePointer() unsafe.Pointer {
	if w != nil && w.native != nil && w.native.wv != nil {
		return unsafe.Pointer(w.native.wv)
	}
	return nil
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

// RunOnMainThread schedules fn to execute on the GTK main thread.
func (w *WebView) RunOnMainThread(fn func()) {
	if w == nil || fn == nil {
		return
	}
	h := cgo.NewHandle(fn)
	C.schedule_on_main_thread(C.uintptr_t(h))
}

// RegisterZoomChangedHandler registers a callback invoked when zoom level changes.
func (w *WebView) RegisterZoomChangedHandler(cb func(level float64)) { w.zoomHandler = cb }

func (w *WebView) dispatchZoomChanged(level float64) {
	if w != nil && w.zoomHandler != nil {
		w.zoomHandler(level)
	}
}

// Widget returns the underlying GtkWidget pointer for the WebView.
func (w *WebView) Widget() uintptr {
	if w == nil || w.native == nil || w.native.view == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(w.native.view))
}

// RootWidget returns the container widget that should be reparented for pane management.
func (w *WebView) RootWidget() uintptr {
	if w == nil || w.native == nil || w.native.container == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(w.native.container))
}

// DestroyWindow destroys the toplevel window hosting this WebView, if any.
func (w *WebView) DestroyWindow() {
	if w == nil || w.native == nil || w.native.win == nil {
		return
	}
	C.gtk_window_destroy((*C.GtkWindow)(unsafe.Pointer(w.native.win)))
	w.native.win = nil
	if w.window != nil {
		w.window.win = nil
	}
}

// PrepareForReparenting prepares the WebView widget for being moved to a new parent
func (w *WebView) PrepareForReparenting() {
	if w == nil || w.native == nil || w.native.container == nil {
		return
	}

	// Mark that this view is being reparented
	w.wasReparented = true

	// Ensure the widget is visible and properly sized for reparenting
	C.gtk_widget_set_visible(w.native.container, C.gboolean(1))
	C.gtk_widget_set_size_request(w.native.container, 100, 100)
	C.gtk_widget_queue_draw(w.native.container)
}

// RefreshAfterReparenting refreshes the WebView after it has been reparented
func (w *WebView) RefreshAfterReparenting() {
	if w == nil || w.native == nil || w.native.wv == nil {
		return
	}

	if w.wasReparented {
		widgetHandle := w.RootWidget()
		parentHandle := WidgetGetParent(widgetHandle)
		if parentHandle == 0 {
			log.Printf("[workspace] RefreshAfterReparenting deferred: widget=%#x parent=%#x url=%s", widgetHandle, parentHandle, w.url)
			return
		}
		log.Printf("[workspace] RefreshAfterReparenting: widget=%#x parent=%#x url=%s", widgetHandle, parentHandle, w.url)
		// Force WebKit to refresh its rendering context by triggering a reload
		// This ensures the rendering context is properly reinitialized after reparenting
		if w.url != "" {
			w.LoadURL(w.url) // Reload current URL to refresh rendering context
		}
		w.wasReparented = false
	}
}

// enableUserContentManager registers the 'dumber' message handler and injects the omnibox script.
func (w *WebView) enableUserContentManager(cfg *Config) {
	if w == nil || w.native == nil || w.native.ucm == nil {
		return
	}
	// Register handler "dumber" for main world
	cname := C.CString("dumber")
	ok := C.register_ucm_handler(w.native.ucm, cname, C.ulong(w.id))
	if ok == 0 {
		log.Printf("[webkit] Failed to register UCM handler; fallback bridge will handle messages")
	}
	C.free(unsafe.Pointer(cname))

	// Also register handler for the isolated GUI world so GUI can post messages to native
	cworld := C.CString("dumber-gui")
	cname2 := C.CString("dumber")
	ok2 := C.register_ucm_handler_world(w.native.ucm, cname2, cworld, C.ulong(w.id))
	if ok2 == 0 {
		log.Printf("[webkit] Failed to register UCM handler for isolated world 'dumber-gui'")
	}
	C.free(unsafe.Pointer(cworld))
	C.free(unsafe.Pointer(cname2))

	// Inject color-scheme preference script at document-start to inform sites of system theme
	preferDark := C.gtk_prefers_dark() != 0
	if preferDark {
		log.Printf("[theme] GTK prefers: dark")
	} else {
		log.Printf("[theme] GTK prefers: light")
	}
	// Load color scheme script from compiled GUI assets instead of inline
	var schemeJS string
	if cfg != nil && cfg.Assets != nil {
		if schemeBytes, err := cfg.Assets.ReadFile("assets/gui/color-scheme.js"); err == nil {
			baseScript := string(schemeBytes)
			// Inject the GTK preference as a parameter
			if preferDark {
				schemeJS = fmt.Sprintf("window.__dumber_gtk_prefers_dark = true; %s", baseScript)
			} else {
				schemeJS = fmt.Sprintf("window.__dumber_gtk_prefers_dark = false; %s", baseScript)
			}
		} else {
			log.Printf("[webkit] Warning: Failed to load color-scheme.js from assets, falling back to basic theme detection: %v", err)
			// Simple fallback if the compiled script is not available
			if preferDark {
				schemeJS = `console.log('[dumber] GTK detected color mode: dark'); document.documentElement.classList.add('dark');`
			} else {
				schemeJS = `console.log('[dumber] GTK detected color mode: light'); document.documentElement.classList.remove('dark');`
			}
		}
	}
	cScheme := C.CString(schemeJS)
	defer C.free(unsafe.Pointer(cScheme))
	schemeScript := C.webkit_user_script_new((*C.gchar)(cScheme), C.WEBKIT_USER_CONTENT_INJECT_TOP_FRAME, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
	if schemeScript != nil {
		C.webkit_user_content_manager_add_script(w.native.ucm, schemeScript)
		C.webkit_user_script_unref(schemeScript)
	}

	// Add GUI bundle as user script at document-start (contains toast, omnibox, and controls)
	if cfg != nil && cfg.Assets != nil {
		log.Printf("[webkit] Attempting to load GUI bundle from assets/gui/gui.min.js")
		if guiBytes, err := cfg.Assets.ReadFile("assets/gui/gui.min.js"); err == nil {
			guiScript := string(guiBytes)
			log.Printf("[webkit] GUI bundle loaded successfully, size: %d bytes", len(guiBytes))

			cGui := C.CString(guiScript)
			defer C.free(unsafe.Pointer(cGui))

			log.Printf("[webkit] Creating GUI user script for document-start injection in isolated world 'dumber-gui'")
			world := C.CString("dumber-gui")
			defer C.free(unsafe.Pointer(world))
			guiUserScript := C.webkit_user_script_new_for_world((*C.gchar)(cGui), C.WEBKIT_USER_CONTENT_INJECT_TOP_FRAME, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, (*C.gchar)(world), nil, nil)
			if guiUserScript != nil {
				C.webkit_user_content_manager_add_script(w.native.ucm, guiUserScript)
				C.webkit_user_script_unref(guiUserScript)
				log.Printf("[webkit] GUI bundle successfully injected as user script at document-start in isolated world")
			} else {
				log.Printf("[webkit] ERROR: Failed to create GUI user script")
			}

			// Inject a unified page-world bridge to dispatch standard events and expose unified API
			w.installDomBridgeScript()

			// If an API token is provided, inject a fetch wrapper in the isolated GUI world
			if cfg.APIToken != "" {
				wrapper := "(() => { try {" +
					"const TOKEN='" + cfg.APIToken + "';" +
					"const appendToken=(u)=>{ try { let url; try { url = new URL(u); } catch(_) { url = new URL(u, 'dumb://homepage'); } if(url.protocol==='dumb:' && url.pathname.startsWith('/api')){ url.searchParams.set('token', TOKEN); return url.toString(); } } catch(_){} return u; };" +
					"const _fetch=window.fetch.bind(window); window.fetch=(input, init)=>{ try { if(typeof input==='string'){ input=appendToken(input); } else if(input && input.url){ const nu=appendToken(input.url); if(nu!==input.url){ input=new Request(nu, input); } } } catch(_){} return _fetch(input, init); };" +
					"} catch(e) { console.warn('[dumber] api token wrapper failed', e); } })();"
				cWrap := C.CString(wrapper)
				defer C.free(unsafe.Pointer(cWrap))
				world := C.CString("dumber-gui")
				defer C.free(unsafe.Pointer(world))
				wrapScript := C.webkit_user_script_new_for_world((*C.gchar)(cWrap), C.WEBKIT_USER_CONTENT_INJECT_TOP_FRAME, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, (*C.gchar)(world), nil, nil)
				if wrapScript != nil {
					C.webkit_user_content_manager_add_script(w.native.ucm, wrapScript)
					C.webkit_user_script_unref(wrapScript)
					log.Printf("[webkit] API token fetch wrapper injected in isolated GUI world")
				}
			}
		} else {
			log.Printf("[webkit] ERROR: Failed to load GUI bundle from assets: %v", err)
		}

		// Add GUI CSS styles as user stylesheet when a legacy bundle is present.
		// Modern builds inline Tailwind output inside gui.min.js, so this file is optional.
		if cssBytes, err := cfg.Assets.ReadFile("assets/gui/style.css"); err == nil {
			cssContent := string(cssBytes)
			cCss := C.CString(cssContent)
			defer C.free(unsafe.Pointer(cCss))

			styleSheet := C.webkit_user_style_sheet_new(
				(*C.gchar)(cCss),
				C.WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,
				C.WEBKIT_USER_STYLE_LEVEL_AUTHOR,
				nil, // allow_list
				nil, // block_list
			)
			if styleSheet != nil {
				C.webkit_user_content_manager_add_style_sheet(w.native.ucm, styleSheet)
				C.webkit_user_style_sheet_unref(styleSheet)
			} else {
				log.Printf("[webkit] ERROR: Failed to create GUI stylesheet")
			}
		} else if errors.Is(err, os.ErrNotExist) {
			log.Printf("[webkit] GUI stylesheet not found; assuming inline Tailwind bundle and skipping separate injection")
		} else {
			log.Printf("[webkit] Warning: Failed to load GUI CSS from assets, skipping separate stylesheet injection: %v", err)
		}
	}

	// Inject Wails runtime fetch interceptor for homepage bridging
	wailsBridge := `(() => { try { const origFetch = window.fetch.bind(window); const waiters = Object.create(null); window.__dumber_wails_resolve = (id, json) => { const w = waiters[id]; if(!w) return; delete waiters[id]; try { const headers = new Headers({"Content-Type":"application/json"}); w.resolve(new Response(json, { headers })); } catch(e){ w.reject(e); } }; window.fetch = (input, init) => { try { const url = new URL(input instanceof Request ? input.url : input, window.location.origin); if (url.pathname === '/wails/runtime') { const args = url.searchParams.get('args'); let payload = {}; try { payload = args ? JSON.parse(args) : {}; } catch(_){} const id = String(Date.now()) + '-' + Math.random().toString(36).slice(2); return new Promise((resolve, reject) => { waiters[id] = { resolve, reject }; try { window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify({ type: 'wails', id, payload })); } catch (e) { reject(e); } }); } } catch(_){} return origFetch(input, init); }; } catch(_){} })();`
	cWails := C.CString(wailsBridge)
	defer C.free(unsafe.Pointer(cWails))
	wailsScript := C.webkit_user_script_new((*C.gchar)(cWails), C.WEBKIT_USER_CONTENT_INJECT_TOP_FRAME, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
	if wailsScript != nil {
		C.webkit_user_content_manager_add_script(w.native.ucm, wailsScript)
		C.webkit_user_script_unref(wailsScript)
	}

	// Inject codec control script if codec preferences are configured
	if cfg != nil && (cfg.CodecPreferences.ForceAV1 || len(cfg.CodecPreferences.BlockedCodecs) > 0 || len(cfg.CodecPreferences.PreferredCodecs) > 0) {
		codecScript := GenerateCodecControlScript(cfg.CodecPreferences)
		cCodec := C.CString(codecScript)
		defer C.free(unsafe.Pointer(cCodec))
		codecUserScript := C.webkit_user_script_new(
			(*C.gchar)(cCodec),
			C.WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES,
			C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START,
			nil, nil)
		if codecUserScript != nil {
			C.webkit_user_content_manager_add_script(w.native.ucm, codecUserScript)
			C.webkit_user_script_unref(codecUserScript)
			log.Printf("[webkit] Codec control script injected")
		}
	}

	// No JS fallback bridge — native UCM is active
}

func clampDomZoom(level float64) float64 {
	if level < 0.25 {
		return 0.25
	}
	if level > 5.0 {
		return 5.0
	}
	return level
}

func buildDomBridgeScript(initialZoom float64) string {
	zoom := clampDomZoom(initialZoom)
	zoomStr := strconv.FormatFloat(zoom, 'f', -1, 64)
	return strings.ReplaceAll(domBridgeTemplate, "__DOM_ZOOM_DEFAULT__", zoomStr)
}

func (w *WebView) UsesDomZoom() bool {
	return w != nil && w.useDomZoom
}

func (w *WebView) installDomBridgeScript() {
	if w == nil || w.native == nil || w.native.ucm == nil {
		return
	}

	w.domZoomSeed = clampDomZoom(w.domZoomSeed)

	if w.domBridgeScript != nil {
		C.webkit_user_content_manager_remove_script(w.native.ucm, w.domBridgeScript)
		C.webkit_user_script_unref(w.domBridgeScript)
		w.domBridgeScript = nil
	}

	source := buildDomBridgeScript(w.domZoomSeed)
	cBridge := C.CString(source)
	defer C.free(unsafe.Pointer(cBridge))

	script := C.webkit_user_script_new((*C.gchar)(cBridge), C.WEBKIT_USER_CONTENT_INJECT_TOP_FRAME, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
	if script == nil {
		log.Printf("[webkit] ERROR: Failed to create GUI user script")
		return
	}

	C.webkit_user_content_manager_add_script(w.native.ucm, script)
	w.domBridgeScript = script
	log.Printf("[webkit] Page-world toast bridge injected (dom zoom seed=%.2f)", w.domZoomSeed)
}

// SeedDomZoom updates the document-start bridge so the upcoming page paints with the saved DOM zoom level.
func (w *WebView) SeedDomZoom(level float64) {
	if w == nil || w.destroyed || !w.useDomZoom {
		return
	}

	seed := clampDomZoom(level)
	if math.Abs(w.domZoomSeed-seed) < 1e-6 {
		return
	}
	w.domZoomSeed = seed
	w.RunOnMainThread(func() {
		w.installDomBridgeScript()
	})
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

//export goHandleTLSError
func goHandleTLSError(failingURI *C.char, host *C.char, errorFlags C.int, certInfo *C.char) C.gboolean {
	uri := C.GoString(failingURI)
	hostname := C.GoString(host)
	certificateInfo := C.GoString(certInfo)

	log.Printf("[tls] Certificate error for %s (URI: %s, flags: %d)", hostname, uri, int(errorFlags))

	// Find the WebView that triggered this error
	// For now, we'll get the most recently created view
	// In a more sophisticated implementation, we'd pass the view ID through user_data
	var webView *WebView
	regMu.RLock()
	for _, v := range viewByID {
		if v != nil {
			webView = v
			break
		}
	}
	regMu.RUnlock()

	if webView == nil {
		log.Printf("[tls] No WebView found to handle TLS error")
		return C.FALSE // Don't proceed
	}

	// Check if we've already allowed this host
	if allowed, exists := webView.tlsExceptions[hostname]; exists && allowed {
		log.Printf("[tls] Certificate exception already granted for %s", hostname)
		return C.TRUE // Proceed with the load
	}

	// Show warning dialog and get user decision
	if webView.showTLSWarningDialog(hostname, uri, int(errorFlags), certificateInfo) {
		webView.tlsExceptions[hostname] = true
		log.Printf("[tls] User accepted certificate exception for %s", hostname)
		return C.TRUE // Proceed with the load
	}

	log.Printf("[tls] User rejected certificate for %s", hostname)
	return C.FALSE // Don't proceed
}

// showTLSWarningDialog displays a warning dialog for TLS certificate errors with storage capability
func (w *WebView) showTLSWarningDialog(hostname, uri string, errorFlags int, certificateInfo string) bool {
	if w == nil || w.native == nil || w.native.win == nil {
		log.Printf("[tls] Cannot show dialog: WebView or window is nil")
		return false
	}

	// Check if we have a stored decision for this certificate
	if storedDecision, hasStored := checkStoredCertificateDecision(hostname, certificateInfo); hasStored {
		switch storedDecision {
		case "accepted":
			log.Printf("[tls] Using stored ACCEPT decision for %s", hostname)
			return true
		case "rejected":
			log.Printf("[tls] Using stored REJECT decision for %s", hostname)
			return false
		default:
			log.Printf("[tls] Unknown stored decision '%s' for %s, showing dialog", storedDecision, hostname)
		}
	}

	log.Printf("[tls] Showing certificate error dialog for %s", hostname)

	// Format the error message with certificate information
	errorMsg := formatTLSErrorMessage(hostname, uri, errorFlags, certificateInfo)

	// Convert strings to C strings
	cHostname := C.CString(hostname)
	defer C.free(unsafe.Pointer(cHostname))

	cErrorMsg := C.CString(errorMsg)
	defer C.free(unsafe.Pointer(cErrorMsg))

	// Show the dialog and get user response (0=Go Back, 1=Proceed Once, 2=Always Accept)
	result := C.show_tls_warning_dialog_sync(
		(*C.GtkWindow)(unsafe.Pointer(w.native.win)),
		cHostname,
		cErrorMsg,
	)

	responseCode := int(result)

	switch responseCode {
	case 0: // Go Back
		log.Printf("[tls] User chose to GO BACK for %s", hostname)
		return false

	case 1: // Proceed Once
		log.Printf("[tls] User chose to PROCEED ONCE for %s", hostname)
		// Store temporary decision (expires in 24 hours)
		if err := storeCertificateDecision(hostname, certificateInfo, "accepted", false); err != nil {
			log.Printf("[tls] Warning: Failed to store temporary certificate decision: %v", err)
		}
		return true

	case 2: // Always Accept
		log.Printf("[tls] User chose to ALWAYS ACCEPT for %s", hostname)
		// Store permanent decision
		if err := storeCertificateDecision(hostname, certificateInfo, "accepted", true); err != nil {
			log.Printf("[tls] Warning: Failed to store permanent certificate decision: %v", err)
		}
		return true

	default:
		log.Printf("[tls] Unknown response code %d for %s, defaulting to reject", responseCode, hostname)
		return false
	}
}

// formatTLSErrorMessage creates a detailed error message based on the error flags
func formatTLSErrorMessage(hostname, uri string, errorFlags int, certificateInfo string) string {
	msg := "Website: " + hostname + "\n\n"

	// Add certificate information
	if certificateInfo != "" {
		msg += "Certificate Details:\n" + certificateInfo + "\n"
	}

	msg += "Security Issues:\n"
	// Decode GTlsCertificateFlags (common values)
	if errorFlags&1 != 0 { // G_TLS_CERTIFICATE_UNKNOWN_CA
		msg += "• Certificate authority is not trusted\n"
	}
	if errorFlags&2 != 0 { // G_TLS_CERTIFICATE_BAD_IDENTITY
		msg += "• Certificate does not match the website identity\n"
	}
	if errorFlags&4 != 0 { // G_TLS_CERTIFICATE_NOT_ACTIVATED
		msg += "• Certificate is not yet valid\n"
	}
	if errorFlags&8 != 0 { // G_TLS_CERTIFICATE_EXPIRED
		msg += "• Certificate has expired\n"
	}
	if errorFlags&16 != 0 { // G_TLS_CERTIFICATE_REVOKED
		msg += "• Certificate has been revoked\n"
	}
	if errorFlags&32 != 0 { // G_TLS_CERTIFICATE_INSECURE
		msg += "• Certificate uses weak cryptography\n"
	}

	msg += "\nProceeding may expose your data to attackers.\nOnly continue if you trust this website."

	return msg
}

// InitializeContentBlocking initializes WebKit content blocking with filter manager
func (w *WebView) InitializeContentBlocking(filterManager FilterManager) error {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return ErrWebViewNotInitialized
	}

	// Load filters asynchronously with proper error recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Error(fmt.Sprintf("[webkit] Content blocking initialization panic: %v", r))
			}
		}()

		// Wait longer for WebView to be ready before applying filters on first load
		// This prevents interference with WebKit's preconnect operations
		time.Sleep(1500 * time.Millisecond)

		// Retry mechanism with exponential backoff
		maxRetries := 5
		baseDelay := 200 * time.Millisecond

		// Apply network filters with error recovery
		for i := 0; i < maxRetries; i++ {
			if w.destroyed {
				logging.Debug("[webkit] WebView destroyed, aborting filter loading")
				return
			}

			if filters, err := filterManager.GetNetworkFilters(); err == nil {
				if len(filters) > 10 && string(filters) != "null" {
					logging.Info("[webkit] Loading network filters from filter manager (" + fmt.Sprintf("%d", len(filters)) + " bytes)")
					if err := w.ApplyContentFilters(filters, "network-filters"); err != nil {
						logging.Error("Failed to apply network filters (retry " + fmt.Sprintf("%d", i+1) + "): " + err.Error())
						if i == maxRetries-1 {
							logging.Warn("[webkit] Network filtering disabled due to errors")
							break
						}
						time.Sleep(baseDelay * time.Duration(1<<uint(i))) // Exponential backoff
						continue
					}
					break
				}
			} else if i == maxRetries-1 {
				logging.Error("Failed to get network filters after retries: " + err.Error())
			}
		}

		// Apply cosmetic filtering with error recovery
		time.Sleep(200 * time.Millisecond) // Additional delay to ensure network setup is complete

		for i := 0; i < maxRetries; i++ {
			if w.destroyed {
				logging.Debug("[webkit] WebView destroyed, aborting cosmetic filter loading")
				return
			}

			script := filterManager.GetCosmeticScript()
			if len(script) > 0 {
				logging.Info("[webkit] Loading cosmetic script from filter manager (" + fmt.Sprintf("%d", len(script)) + " chars)")
				if err := w.InjectCosmeticFilter(script); err != nil {
					logging.Error("Failed to inject cosmetic filter (retry " + fmt.Sprintf("%d", i+1) + "): " + err.Error())
					if i == maxRetries-1 {
						logging.Warn("[webkit] Cosmetic filtering disabled due to errors")
					} else {
						time.Sleep(baseDelay * time.Duration(1<<uint(i)))
						continue
					}
				}
				break
			}
		}

		logging.Info("Content blocking initialization completed")
	}()

	return nil
}

// OnNavigate sets up domain-specific cosmetic filtering on navigation
func (w *WebView) OnNavigate(url string, filterManager FilterManager) {
	if w == nil || w.destroyed {
		return
	}

	domain := extractDomain(url)
	if domain == "" {
		return
	}

	// Get domain-specific cosmetic rules
	script := filterManager.GetCosmeticScriptForDomain(domain)
	if script != "" {
		// Inject domain-specific cosmetic rules
		logging.Info("[webkit] Applying domain-specific cosmetic rules for: " + domain + " (" + fmt.Sprintf("%d", len(script)) + " chars)")
		w.InjectScript(fmt.Sprintf(
			"if (typeof window.__dumber_cosmetic_init === 'function') { %s }",
			script,
		))
	} else {
		logging.Debug("[webkit] No domain-specific cosmetic rules found for: " + domain)
	}
}

// UpdateContentFilters updates the content filters dynamically
func (w *WebView) UpdateContentFilters(filterManager FilterManager) error {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return ErrWebViewNotInitialized
	}

	// Clear existing filters first
	if err := w.ClearAllFilters(); err != nil {
		return fmt.Errorf("failed to clear existing filters: %w", err)
	}

	// Apply new filters
	if filters, err := filterManager.GetNetworkFilters(); err == nil {
		logging.Info("[webkit] Updating network filters (" + fmt.Sprintf("%d", len(filters)) + " bytes)")
		if err := w.ApplyContentFilters(filters, "network-filters-updated"); err != nil {
			return fmt.Errorf("failed to apply updated network filters: %w", err)
		}
	}

	// Update cosmetic filtering
	script := filterManager.GetCosmeticScript()
	logging.Info("[webkit] Updating cosmetic script (" + fmt.Sprintf("%d", len(script)) + " chars)")
	if err := w.InjectCosmeticFilter(script); err != nil {
		return fmt.Errorf("failed to inject updated cosmetic filter: %w", err)
	}

	logging.Info("Content filters updated successfully")
	return nil
}

// extractDomain extracts the domain from a URL
func extractDomain(url string) string {
	if url == "" {
		return ""
	}

	// Simple domain extraction - remove protocol and path
	start := strings.Index(url, "://")
	if start != -1 {
		url = url[start+3:]
	}

	end := strings.Index(url, "/")
	if end != -1 {
		url = url[:end]
	}

	// Remove port if present
	if portIdx := strings.Index(url, ":"); portIdx != -1 {
		url = url[:portIdx]
	}

	return url
}

// detectAndLogRenderingBackend detects and logs the actual rendering backend being used
func (w *WebView) detectAndLogRenderingBackend() {
	if w == nil {
		return
	}

	// Use a small delay to let WebKit initialize
	go func() {
		time.Sleep(500 * time.Millisecond)

		// Inject a script to detect rendering capabilities
		detectionScript := `
			(() => {
				try {
					const canvas = document.createElement('canvas');
					const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
					const info = {
						webgl_available: !!gl,
						renderer: gl ? gl.getParameter(gl.RENDERER) : 'N/A',
						vendor: gl ? gl.getParameter(gl.VENDOR) : 'N/A',
						version: gl ? gl.getParameter(gl.VERSION) : 'N/A',
						shading_language_version: gl ? gl.getParameter(gl.SHADING_LANGUAGE_VERSION) : 'N/A',
						max_texture_size: gl ? gl.getParameter(gl.MAX_TEXTURE_SIZE) : 0,
						hardware_accelerated: false
					};
					
					// Check if renderer indicates hardware acceleration
					if (gl && info.renderer) {
						const renderer = info.renderer.toLowerCase();
						// Look for GPU indicators
						info.hardware_accelerated = (
							renderer.includes('nvidia') ||
							renderer.includes('amd') ||
							renderer.includes('intel') ||
							renderer.includes('radeon') ||
							renderer.includes('geforce') ||
							renderer.includes('quadro') ||
							(renderer.includes('mesa') && !renderer.includes('software')) ||
							!renderer.includes('software')
						);
					}
					
					window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify({
						type: 'rendering_backend_detection',
						data: info
					}));
				} catch (e) {
					console.error('[dumber] Rendering backend detection failed:', e);
				}
			})();
		`

		w.InjectScript(detectionScript)
	}()
}

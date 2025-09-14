//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#cgo CFLAGS: -I/usr/include/webkitgtk-6.0
#include <webkit/webkit.h>
#include <stdlib.h>

// WebKit debug logging callback
extern void webkit_debug_log(char* message);

// Forward declaration
static void webkit_debug_log_handler(const gchar *log_domain,
                                   GLogLevelFlags log_level,
                                   const gchar *message,
                                   gpointer user_data);

static void setup_webkit_debug_logging() {
    // Enable WebKit debug logging if WEBKIT_DEBUG environment variable is set
    const char* debug_env = g_getenv("WEBKIT_DEBUG");
    if (debug_env) {
        g_setenv("G_MESSAGES_DEBUG", "WebKit", TRUE);
        g_setenv("WEBKIT_DEBUG", debug_env, TRUE);

        // Set up custom log handler for better integration
        g_log_set_handler("WebKit",
                         G_LOG_LEVEL_DEBUG | G_LOG_LEVEL_INFO | G_LOG_LEVEL_MESSAGE,
                         (GLogFunc)webkit_debug_log_handler, NULL);
    }
}

static void webkit_debug_log_handler(const gchar *log_domain,
                                   GLogLevelFlags log_level,
                                   const gchar *message,
                                   gpointer user_data) {
    // Format log message and send to Go
    char* formatted_msg = g_strdup_printf("[%s] %s", log_domain, message);
    webkit_debug_log(formatted_msg);
    g_free(formatted_msg);
}

// Enable detailed content filtering debug logs
static void enable_filtering_debug() {
    g_setenv("WEBKIT_DEBUG", "Network:preconnectTo,ContentFilters", TRUE);
}

// Check if WebView is in a valid state for operations
static gboolean check_webview_state(WebKitWebView *webview) {
    if (!webview) return FALSE;
    if (!WEBKIT_IS_WEB_VIEW(webview)) return FALSE;

    // Check if WebView is not being destroyed
    GObject *obj = G_OBJECT(webview);
    if (g_object_get_data(obj, "webkit-destroying")) return FALSE;

    return TRUE;
}
*/
import "C"

import (
	"os"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
)

//export webkit_debug_log
func webkit_debug_log(message *C.char) {
	msg := C.GoString(message)
	logging.Debug("[webkit-internal] " + msg)
}

// SetupWebKitDebugLogging initializes WebKit debug logging based on config and environment variables
func SetupWebKitDebugLogging(cfg *config.Config) {
	// Check if debug mode is enabled via config or environment variables
	debugEnabled := (cfg != nil && cfg.Debug.EnableWebKitDebug) ||
		os.Getenv("WEBKIT_DEBUG") != "" ||
		os.Getenv("DUMBER_DEBUG") != "" ||
		(cfg != nil && cfg.Debug.EnableGeneralDebug)

	if debugEnabled {
		logging.Info("[webkit] Enabling WebKit debug logging")

		// Set debug categories from config or environment
		var debugCategories string
		if cfg != nil && cfg.Debug.WebKitDebugCategories != "" {
			debugCategories = cfg.Debug.WebKitDebugCategories
		} else if envDebug := os.Getenv("WEBKIT_DEBUG"); envDebug != "" {
			debugCategories = envDebug
		} else {
			debugCategories = "Network:preconnectTo,ContentFilters"
		}

		// Set the environment variable for WebKit
		os.Setenv("WEBKIT_DEBUG", debugCategories)

		C.setup_webkit_debug_logging()

		// Enable filtering-specific debug logs if configured
		if cfg != nil && cfg.Debug.EnableFilteringDebug {
			C.enable_filtering_debug()
			logging.Info("[webkit] Content filtering debug logging enabled")
		}

		logging.Info("[webkit] WebKit debug logging enabled with categories: " + debugCategories)
	}
}

// CheckWebViewState validates that a WebView is in a safe state for operations
func (w *WebView) CheckWebViewState() bool {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return false
	}

	return C.check_webview_state(w.native.wv) != 0
}

// LogDebugInfo logs debug information about the current WebView state
func (w *WebView) LogDebugInfo() {
	if !w.CheckWebViewState() {
		logging.Debug("[webkit] WebView is not in valid state")
		return
	}

	logging.Debug("[webkit] WebView state: valid and ready for operations")

	// Log content manager state
	manager := C.webkit_web_view_get_user_content_manager(w.native.wv)
	if manager != nil {
		logging.Debug("[webkit] User content manager: available")
	} else {
		logging.Debug("[webkit] User content manager: not available")
	}
}

//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#cgo CFLAGS: -I/usr/include/webkitgtk-6.0
#include <webkit/webkit.h>
#include <stdlib.h>

// Global filter store instance
static WebKitUserContentFilterStore *filter_store = NULL;

// External Go functions
extern char* get_webkit_filter_store_path();
extern void log_filter_success(char* identifier);
extern void log_filter_error(char* error_msg, char* identifier);

// Callback for save operation completion
static void filter_save_callback(GObject *source_object,
                                 GAsyncResult *result,
                                 gpointer user_data) {
    WebKitUserContentFilterStore *store = WEBKIT_USER_CONTENT_FILTER_STORE(source_object);
    WebKitUserContentManager *manager = WEBKIT_USER_CONTENT_MANAGER(user_data);
    GError *error = NULL;

    WebKitUserContentFilter *filter = webkit_user_content_filter_store_save_finish(store, result, &error);

    if (filter && !error) {
        webkit_user_content_manager_add_filter(manager, filter);
        const char *id = webkit_user_content_filter_get_identifier(filter);
        log_filter_success((char*)id);
        webkit_user_content_filter_unref(filter);
    }

    if (error) {
        char *msg = g_strdup(error->message);
        log_filter_error(msg, "unknown");
        g_free(msg);
        g_error_free(error);
    }
}

// Initialize filter store if not already done
static void ensure_filter_store() {
    if (filter_store == NULL) {
        // Get XDG-compliant path from Go (with tmp fallback)
        char *store_path = get_webkit_filter_store_path();
        filter_store = webkit_user_content_filter_store_new(store_path);
        free(store_path);  // Free the C string returned by Go
    }
}

static void apply_content_filter(WebKitUserContentManager *manager,
                                 const char *filter_json,
                                 const char *identifier) {
    ensure_filter_store();

    GBytes *bytes = g_bytes_new(filter_json, strlen(filter_json));

    // Save filter to store asynchronously - it will be applied in the callback
    webkit_user_content_filter_store_save(filter_store,
                                          identifier,
                                          bytes,
                                          NULL,
                                          filter_save_callback,
                                          manager);

    g_bytes_unref(bytes);
}

static void inject_cosmetic_script(WebKitUserContentManager *manager,
                                   const char *script_source) {
    // Use isolated script world to prevent interference with page JavaScript
    WebKitUserScript *script = webkit_user_script_new_for_world(
        script_source,
        WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES,
        WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_END, // Changed from DOCUMENT_START to avoid preconnect interference
        "dumber-cosmetic", // Isolated world name
        NULL, NULL
    );

    webkit_user_content_manager_add_script(manager, script);
    webkit_user_script_unref(script);
}

static void clear_all_filters(WebKitUserContentManager *manager) {
    webkit_user_content_manager_remove_all_filters(manager);
}

static void clear_all_scripts(WebKitUserContentManager *manager) {
    webkit_user_content_manager_remove_all_scripts(manager);
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
)

//export get_webkit_filter_store_path
func get_webkit_filter_store_path() *C.char {
	// Try XDG directories first
	if filterCacheDir, err := config.GetFilterCacheDir(); err == nil {
		webkitFilterDir := filepath.Join(filterCacheDir, "webkit-store")
		if err := os.MkdirAll(webkitFilterDir, 0755); err == nil {
			return C.CString(webkitFilterDir)
		}
	}

	// Fallback to tmp if XDG fails
	tmpDir := os.TempDir()
	fallbackDir := filepath.Join(tmpDir, "dumber-filters")
	if err := os.MkdirAll(fallbackDir, 0755); err == nil {
		logging.Warn("[webkit] Using fallback tmp directory for filter store: " + fallbackDir)
		return C.CString(fallbackDir)
	}

	// Last resort - return tmp dir
	logging.Error("[webkit] Failed to create filter store directory, using tmp")
	return C.CString(tmpDir)
}

//export log_filter_success
func log_filter_success(identifier *C.char) {
	id := C.GoString(identifier)
	logging.Info("[webkit] Network filter applied successfully: " + id)
}

//export log_filter_error
func log_filter_error(errorMsg *C.char, identifier *C.char) {
	msg := C.GoString(errorMsg)
	id := C.GoString(identifier)
	logging.Error("[webkit] Failed to apply network filter '" + id + "': " + msg)
}

// ApplyContentFilters applies WebKit native content filters with error recovery
func (w *WebView) ApplyContentFilters(filters []byte, identifier string) error {
	// Check WebView state with debug support
	if !w.CheckWebViewState() {
		w.LogDebugInfo()
		return ErrWebViewNotInitialized
	}

	// Validate input
	if len(filters) == 0 || identifier == "" {
		return fmt.Errorf("invalid filter parameters: empty filters or identifier")
	}

	// Check if filters are valid JSON
	if !json.Valid(filters) {
		return fmt.Errorf("invalid filter JSON for identifier: %s", identifier)
	}

	defer func() {
		if r := recover(); r != nil {
			logging.Error(fmt.Sprintf("[webkit] Panic in ApplyContentFilters: %v", r))
		}
	}()

	manager := C.webkit_web_view_get_user_content_manager(w.native.wv)
	if manager == nil {
		return ErrContentManagerNotFound
	}

	cFilters := C.CString(string(filters))
	cID := C.CString(identifier)
	defer C.free(unsafe.Pointer(cFilters))
	defer C.free(unsafe.Pointer(cID))

	logging.Info("[webkit] Applying network filters: " + identifier + " (" + fmt.Sprintf("%d", len(filters)) + " bytes)")

	// Apply filter with error checking
	C.apply_content_filter(manager, cFilters, cID)
	return nil
}

// InjectCosmeticFilter injects JavaScript for cosmetic filtering with error recovery
func (w *WebView) InjectCosmeticFilter(script string) error {
	// Check WebView state with debug support
	if !w.CheckWebViewState() {
		w.LogDebugInfo()
		return ErrWebViewNotInitialized
	}

	// Validate input
	if len(script) == 0 {
		return fmt.Errorf("empty cosmetic filter script")
	}

	defer func() {
		if r := recover(); r != nil {
			logging.Error(fmt.Sprintf("[webkit] Panic in InjectCosmeticFilter: %v", r))
		}
	}()

	manager := C.webkit_web_view_get_user_content_manager(w.native.wv)
	if manager == nil {
		return ErrContentManagerNotFound
	}

	cScript := C.CString(script)
	defer C.free(unsafe.Pointer(cScript))

	logging.Info("[webkit] Injecting cosmetic filter script (" + fmt.Sprintf("%d", len(script)) + " chars)")

	// Inject script with error recovery
	C.inject_cosmetic_script(manager, cScript)
	return nil
}

// ClearAllFilters removes all content filters
func (w *WebView) ClearAllFilters() error {
	if w.native == nil || w.native.wv == nil {
		return ErrWebViewNotInitialized
	}

	manager := C.webkit_web_view_get_user_content_manager(w.native.wv)
	if manager == nil {
		return ErrContentManagerNotFound
	}

	logging.Info("[webkit] Clearing all content filters")
	C.clear_all_filters(manager)
	return nil
}

// ClearAllScripts removes all injected scripts
func (w *WebView) ClearAllScripts() error {
	if w.native == nil || w.native.wv == nil {
		return ErrWebViewNotInitialized
	}

	manager := C.webkit_web_view_get_user_content_manager(w.native.wv)
	if manager == nil {
		return ErrContentManagerNotFound
	}

	logging.Info("[webkit] Clearing all injected scripts")
	C.clear_all_scripts(manager)
	return nil
}

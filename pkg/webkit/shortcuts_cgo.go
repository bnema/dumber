//go:build webkit_cgo

package webkit

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <stdint.h>
#include <stdlib.h>

extern void goHandleWindowShortcut(uintptr_t handle);

static GtkShortcutController* create_global_shortcut_controller(GtkWindow* window) {
    if (!window) {
        return NULL;
    }
    GtkEventController* event_controller = gtk_shortcut_controller_new();
    if (!event_controller) {
        return NULL;
    }
    GtkShortcutController* controller = GTK_SHORTCUT_CONTROLLER(event_controller);
    gtk_shortcut_controller_set_scope(controller, GTK_SHORTCUT_SCOPE_GLOBAL);
    gtk_widget_add_controller(GTK_WIDGET(window), event_controller);
    return controller;
}

static gboolean shortcut_callback_bridge(GtkWidget* widget, GVariant* args, gpointer user_data) {
    (void)widget;
    (void)args;
    uintptr_t handle = (uintptr_t)user_data;
    if (handle == 0) {
        return FALSE;
    }
    goHandleWindowShortcut(handle);
    return TRUE;  // Indicate we handled the shortcut
}

static void add_shortcut_to_controller(GtkShortcutController* controller,
                                const char* trigger_str,
                                uintptr_t handle) {
    if (!controller || !trigger_str || handle == 0) {
        return;
    }
    GtkShortcutTrigger* trigger = gtk_shortcut_trigger_parse_string(trigger_str);
    if (!trigger) {
        return;
    }
    GtkShortcutAction* action = gtk_callback_action_new((GtkShortcutFunc)shortcut_callback_bridge,
                                                        (gpointer)handle,
                                                        NULL);
    if (!action) {
        g_object_unref(trigger);
        return;
    }
    GtkShortcut* shortcut = gtk_shortcut_new(trigger, action);
    gtk_shortcut_controller_add_shortcut(controller, shortcut);
}
*/
import "C"

var (
	globalShortcutMu        sync.Mutex
	globalShortcutCounter   uint64
	globalShortcutCallbacks = make(map[uintptr]func())
)

func registerWindowShortcutCallback(cb func()) uintptr {
	if cb == nil {
		return 0
	}
	id := uintptr(atomic.AddUint64(&globalShortcutCounter, 1))
	if id == 0 {
		// Should never happen, but guard against wrapping to 0
		id = uintptr(atomic.AddUint64(&globalShortcutCounter, 1))
	}
	globalShortcutMu.Lock()
	globalShortcutCallbacks[id] = cb
	globalShortcutMu.Unlock()
	return id
}

func unregisterWindowShortcutCallback(id uintptr) {
	if id == 0 {
		return
	}
	globalShortcutMu.Lock()
	delete(globalShortcutCallbacks, id)
	globalShortcutMu.Unlock()
}

func invokeWindowShortcutCallback(id uintptr) {
	if id == 0 {
		return
	}
	globalShortcutMu.Lock()
	cb := globalShortcutCallbacks[id]
	globalShortcutMu.Unlock()
	if cb == nil {
		log.Printf("[shortcuts] no handler for shortcut id=%d", id)
		return
	}
	cb()
}

//export goHandleWindowShortcut
func goHandleWindowShortcut(handle C.uintptr_t) {
	invokeWindowShortcutCallback(uintptr(handle))
}

// Go wrapper functions for CGO - these can be called from other Go files

// CreateGlobalShortcutController creates a GTK4 global shortcut controller for a window
func CreateGlobalShortcutController(window uintptr) uintptr {
	if window == 0 {
		return 0
	}
	controller := C.create_global_shortcut_controller((*C.GtkWindow)(unsafe.Pointer(window)))
	return uintptr(unsafe.Pointer(controller))
}

// AddShortcutToController adds a shortcut to the controller with a callback handle
func AddShortcutToController(controller uintptr, key string, handle uintptr) error {
	if controller == 0 || key == "" || handle == 0 {
		return ErrNotImplemented
	}

	keyStr := C.CString(key)
	defer C.free(unsafe.Pointer(keyStr))

	C.add_shortcut_to_controller(
		(*C.GtkShortcutController)(unsafe.Pointer(controller)),
		keyStr,
		C.uintptr_t(handle),
	)

	return nil
}

// Global shortcuts registry to track all registered shortcuts for automatic blocking
var (
	globalShortcutsRegistry = make(map[string]bool)
	shortcutsRegistryMu     sync.RWMutex
	// Map from shortcut key to callback handle for C-level triggering
	globalShortcutHandles = make(map[string]uintptr)
)

// WebView tracking for dynamic protection refresh
var (
	activeWebViews   = make(map[*WebView]bool)
	activeWebViewsMu sync.RWMutex
)

// RegisterGlobalShortcut adds a shortcut to the global registry for automatic webpage blocking
func RegisterGlobalShortcut(shortcut string) {
	shortcutsRegistryMu.Lock()
	// Normalize shortcut format (convert to lowercase, standardize modifiers)
	normalized := normalizeShortcut(shortcut)

	// Only proceed if this is a new shortcut registration
	wasNew := !globalShortcutsRegistry[normalized]
	if wasNew {
		globalShortcutsRegistry[normalized] = true
		log.Printf("[shortcuts] Registered global shortcut for blocking: %s -> %s", shortcut, normalized)
	}
	shortcutsRegistryMu.Unlock()

	// If we added a new shortcut, refresh protection on all active WebViews
	if wasNew {
		RefreshGlobalShortcutsProtection()
	}
}

// RegisterGlobalShortcutWithHandle adds a shortcut with its callback handle for C-level triggering
func RegisterGlobalShortcutWithHandle(shortcut string, handle uintptr) {
	// First register the shortcut for blocking
	RegisterGlobalShortcut(shortcut)

	// Then store the callback handle for C-level triggering
	shortcutsRegistryMu.Lock()
	normalized := normalizeShortcut(shortcut)
	globalShortcutHandles[normalized] = handle
	log.Printf("[shortcuts] Registered callback handle for shortcut: %s", shortcut)
	shortcutsRegistryMu.Unlock()
}

// GetRegisteredShortcuts returns a copy of all registered shortcuts
func GetRegisteredShortcuts() []string {
	shortcutsRegistryMu.RLock()
	defer shortcutsRegistryMu.RUnlock()

	shortcuts := make([]string, 0, len(globalShortcutsRegistry))
	for shortcut := range globalShortcutsRegistry {
		shortcuts = append(shortcuts, shortcut)
	}
	return shortcuts
}

// RegisterActiveWebView adds a WebView to the active registry for dynamic protection updates
func RegisterActiveWebView(w *WebView) {
	if w == nil {
		return
	}
	activeWebViewsMu.Lock()
	defer activeWebViewsMu.Unlock()
	activeWebViews[w] = true
	// WebView registered for protection updates
}

// UnregisterActiveWebView removes a WebView from the active registry
func UnregisterActiveWebView(w *WebView) {
	if w == nil {
		return
	}
	activeWebViewsMu.Lock()
	defer activeWebViewsMu.Unlock()
	delete(activeWebViews, w)
	// WebView unregistered from protection updates
}

// RefreshGlobalShortcutsProtection updates all active WebViews with current shortcut protection
func RefreshGlobalShortcutsProtection() {
	activeWebViewsMu.RLock()
	webViews := make([]*WebView, 0, len(activeWebViews))
	for wv := range activeWebViews {
		webViews = append(webViews, wv)
	}
	activeWebViewsMu.RUnlock()

	if len(webViews) == 0 {
		return
	}

	for _, wv := range webViews {
		if wv != nil && !wv.destroyed {
			// Re-enable protection which will inject updated shortcut list
			if err := wv.EnableGlobalShortcutsProtection(); err != nil {
				log.Printf("[shortcuts] Failed to refresh protection for WebView: %v", err)
			}
		}
	}
}

// normalizeShortcut converts shortcuts to a standardized format for JavaScript blocking
func normalizeShortcut(shortcut string) string {
	// Convert to lowercase and replace common variations
	normalized := strings.ToLower(shortcut)

	// Standardize modifier keys
	normalized = strings.ReplaceAll(normalized, "cmdorctrl+", "ctrl+")
	normalized = strings.ReplaceAll(normalized, "cmd+", "ctrl+") // Treat cmd as ctrl for blocking
	normalized = strings.ReplaceAll(normalized, "control+", "ctrl+")

	// Standardize arrow key names (handle both formats)
	normalized = strings.ReplaceAll(normalized, "left", "arrowleft")
	normalized = strings.ReplaceAll(normalized, "right", "arrowright")
	normalized = strings.ReplaceAll(normalized, "up", "arrowup")
	normalized = strings.ReplaceAll(normalized, "down", "arrowdown")

	// Fix double arrow prefixes that might occur from above replacements
	normalized = strings.ReplaceAll(normalized, "arrowarrowleft", "arrowleft")
	normalized = strings.ReplaceAll(normalized, "arrowarrowright", "arrowright")
	normalized = strings.ReplaceAll(normalized, "arrowarrowup", "arrowup")
	normalized = strings.ReplaceAll(normalized, "arrowarrowdown", "arrowdown")

	// Other key normalizations
	normalized = strings.ReplaceAll(normalized, "equal", "equal") // For Ctrl+=
	normalized = strings.ReplaceAll(normalized, "plus", "equal")  // Normalize + to =

	return normalized
}

// Keyboard event blocking functionality for global shortcuts protection

// keyboardBlockerScript holds reference to the main-world keyboard blocking script
var keyboardBlockerScript *C.WebKitUserScript = nil

// EnableGlobalShortcutsProtection injects a script to block all registered global shortcuts from reaching webpages
func (w *WebView) EnableGlobalShortcutsProtection() error {
	if w == nil || w.destroyed || w.native == nil || w.native.ucm == nil {
		return ErrWebViewNotInitialized
	}

	// Get all registered shortcuts
	shortcuts := GetRegisteredShortcuts()
	if len(shortcuts) == 0 {
		log.Printf("[shortcuts] No global shortcuts registered, skipping protection")
		return nil
	}

	// Generate JavaScript array of shortcuts to block
	shortcutsJS := "["
	for i, shortcut := range shortcuts {
		if i > 0 {
			shortcutsJS += ","
		}
		shortcutsJS += `"` + shortcut + `"`
	}
	shortcutsJS += "]"

	// Create a version-based blocking script that can be updated dynamically
	protectionVersion := len(shortcuts) // Use shortcuts count as version
	globalProtectionScript := `
		(() => {
			const newVersion = ` + fmt.Sprintf("%d", protectionVersion) + `;
			const currentVersion = window.__dumber_shortcuts_protection_version || 0;

			// Only update if this is a newer version or first time
			if (newVersion > currentVersion) {
				// Clean up old protection if it exists
				if (window.__dumber_cleanup_global_shortcuts) {
					window.__dumber_cleanup_global_shortcuts();
				}

				window.__dumber_shortcuts_protection_version = newVersion;

				// List of global shortcuts to block from reaching the webpage
				const globalShortcuts = ` + shortcutsJS + `;
				console.log('[dumber] Protecting global shortcuts (v' + newVersion + '):', globalShortcuts);

				// Function to check if a key event matches any global shortcut
				const isGlobalShortcut = (e) => {
					const key = e.key.toLowerCase();
					const code = e.code.toLowerCase();

					// Build shortcut string (similar to GTK format)
					let shortcut = '';
					if (e.ctrlKey) shortcut += 'ctrl+';
					if (e.altKey) shortcut += 'alt+';
					if (e.shiftKey) shortcut += 'shift+';
					if (e.metaKey) shortcut += 'meta+';

					// Try both key and code for matching
					const keyShortcut = shortcut + key;
					const codeShortcut = shortcut + code;

					// Special case mappings for common shortcuts
					const specialMappings = {
						'ctrl+equal': ['ctrl+=', 'ctrl+plus'],
						'ctrl+minus': ['ctrl+-'],
						'f12': ['f12'],
						'ctrl+r': ['ctrl+r'],
						'ctrl+shift+r': ['ctrl+shift+r'],
						'f5': ['f5'],
						// Alt+Arrow key mappings (handle both key and code variations)
						'alt+arrowleft': ['alt+left', 'alt+arrowleft'],
						'alt+arrowright': ['alt+right', 'alt+arrowright'],
						'alt+arrowup': ['alt+up', 'alt+arrowup'],
						'alt+arrowdown': ['alt+down', 'alt+arrowdown']
					};

					// Check direct matches
					if (globalShortcuts.includes(keyShortcut) || globalShortcuts.includes(codeShortcut)) {
						return true;
					}

					// Check special mappings
					for (const [canonical, variants] of Object.entries(specialMappings)) {
						if (variants.includes(keyShortcut) || variants.includes(codeShortcut)) {
							if (globalShortcuts.includes(canonical)) {
								return true;
							}
						}
					}

					return false;
				};

				// Block only global shortcuts at capture phase
				const blockGlobalShortcut = (e) => {
					if (isGlobalShortcut(e)) {
						console.log('[dumber] Blocked global shortcut from reaching page:', e.key, e.ctrlKey, e.altKey, e.shiftKey);
						e.stopImmediatePropagation();
						e.preventDefault();
						e.stopPropagation();
						return false;
					}
					// Let other key events through normally
				};

				// Install listeners for keyboard events
				document.addEventListener('keydown', blockGlobalShortcut, true);
				document.addEventListener('keyup', blockGlobalShortcut, true);
				document.addEventListener('keypress', blockGlobalShortcut, true);

				// Store cleanup function
				window.__dumber_cleanup_global_shortcuts = () => {
					document.removeEventListener('keydown', blockGlobalShortcut, true);
					document.removeEventListener('keyup', blockGlobalShortcut, true);
					document.removeEventListener('keypress', blockGlobalShortcut, true);
					delete window.__dumber_shortcuts_protection_version;
					delete window.__dumber_cleanup_global_shortcuts;
					console.log('[dumber] Global shortcuts protection disabled');
				};

				console.log('[dumber] Global shortcuts protection enabled for', globalShortcuts.length, 'shortcuts (v' + newVersion + ')');
			} else {
				console.log('[dumber] Global shortcuts protection already up to date (v' + currentVersion + ')');
			}
		})();
	`

	// Inject the script into the main world at document start
	cScript := C.CString(globalProtectionScript)
	defer C.free(unsafe.Pointer(cScript))

	script := C.webkit_user_script_new(
		(*C.gchar)(cScript),
		C.WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,
		C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START,
		nil, nil) // nil = main world

	if script == nil {
		return ErrNotImplemented
	}

	C.webkit_user_content_manager_add_script(w.native.ucm, script)
	C.webkit_user_script_unref(script) // Clean up reference

	log.Printf("[shortcuts] Global shortcuts protection enabled for %d shortcuts", len(shortcuts))
	return nil
}

// DisableGlobalShortcutsProtection removes the global shortcuts protection
func (w *WebView) DisableGlobalShortcutsProtection() error {
	if w == nil || w.destroyed || w.native == nil {
		return ErrWebViewNotInitialized
	}

	// Execute cleanup script
	cleanupScript := `
		if (window.__dumber_cleanup_global_shortcuts) {
			window.__dumber_cleanup_global_shortcuts();
		}
	`

	return w.InjectScript(cleanupScript)
}

// EnablePageKeyboardBlocking injects a script into the main world to block all keyboard events
// This prevents page JavaScript from receiving keyboard events while omnibox is active
func (w *WebView) EnablePageKeyboardBlocking() error {
	if w == nil || w.destroyed || w.native == nil || w.native.ucm == nil {
		return ErrWebViewNotInitialized
	}

	// Remove any existing blocker script first
	w.DisablePageKeyboardBlocking()

	// Script to block all keyboard and related events in the main world
	blockingScript := `
		(() => {
			// Store original state to restore later
			if (!window.__dumber_event_blocker_installed) {
				window.__dumber_event_blocker_installed = true;

				// List of events to block
				const eventTypes = [
					'keydown', 'keyup', 'keypress',
					'input', 'beforeinput',
					'compositionstart', 'compositionupdate', 'compositionend'
				];

				// Block all keyboard-related events at capture phase
				const blockEvent = (e) => {
					e.stopImmediatePropagation();
					e.preventDefault();
					e.stopPropagation();
					return false;
				};

				// Install capture-phase event blockers
				eventTypes.forEach(type => {
					document.addEventListener(type, blockEvent, true);
				});

				// Store cleanup function
				window.__dumber_cleanup_event_blocker = () => {
					eventTypes.forEach(type => {
						document.removeEventListener(type, blockEvent, true);
					});
					delete window.__dumber_event_blocker_installed;
					delete window.__dumber_cleanup_event_blocker;
				};

				console.log('[dumber] Page keyboard event blocking enabled');
			}
		})();
	`

	cScript := C.CString(blockingScript)
	defer C.free(unsafe.Pointer(cScript))

	// Inject into main world (nil world parameter = main world)
	keyboardBlockerScript = C.webkit_user_script_new(
		(*C.gchar)(cScript),
		C.WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,
		C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START,
		nil, nil) // nil = main world

	if keyboardBlockerScript == nil {
		return ErrNotImplemented
	}

	C.webkit_user_content_manager_add_script(w.native.ucm, keyboardBlockerScript)
	log.Printf("[webkit] Page keyboard event blocking enabled")
	return nil
}

// DisablePageKeyboardBlocking removes the keyboard blocking script and restores normal page behavior
func (w *WebView) DisablePageKeyboardBlocking() error {
	if w == nil || w.destroyed || w.native == nil || w.native.ucm == nil {
		return ErrWebViewNotInitialized
	}

	// Execute cleanup script to remove event listeners
	cleanupScript := `
		if (window.__dumber_cleanup_event_blocker) {
			window.__dumber_cleanup_event_blocker();
			console.log('[dumber] Page keyboard event blocking disabled');
		}
	`
	w.InjectScript(cleanupScript)

	// Remove the user script if it exists
	if keyboardBlockerScript != nil {
		C.webkit_user_content_manager_remove_script(w.native.ucm, keyboardBlockerScript)
		C.webkit_user_script_unref(keyboardBlockerScript)
		keyboardBlockerScript = nil
		log.Printf("[webkit] Keyboard blocker script removed")
	}

	return nil
}

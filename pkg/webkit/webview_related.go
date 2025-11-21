package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#include <webkit/webkit.h>
#include <gtk/gtk.h>
#include <stdlib.h>

// create_related_web_view creates a WebView with the related-view property set
// This is the WebKitGTK 6.0 way to share session/cookies between views (for popups)
// The related-view property is construct-only, so it must be set during g_object_new
static inline WebKitWebView* create_related_web_view(WebKitWebView* parent) {
	return WEBKIT_WEB_VIEW(g_object_new(
		WEBKIT_TYPE_WEB_VIEW,
		"related-view", parent,
		NULL
	));
}

// create_extension_web_view creates a WebView for extension popup/options pages with:
// - related-view to share session/process with parent
// - web-extension-mode set to ManifestV2
// - default-content-security-policy provided by the manifest (can be NULL)
static inline WebKitWebView* create_extension_web_view(WebKitWebView* parent, const gchar* csp) {
	return WEBKIT_WEB_VIEW(g_object_new(
		WEBKIT_TYPE_WEB_VIEW,
		"related-view", parent,
		"web-extension-mode", WEBKIT_WEB_EXTENSION_MODE_MANIFESTV2,
		"default-content-security-policy", csp,
		NULL
	));
}

// create_extension_background_web_view creates a WebView for extension background pages with:
// - web-extension-mode set to ManifestV2
// - default-content-security-policy provided by the manifest (can be NULL)
// - NO related-view (background pages are the root for popups to relate to)
static inline WebKitWebView* create_extension_background_web_view(const gchar* csp) {
	return WEBKIT_WEB_VIEW(g_object_new(
		WEBKIT_TYPE_WEB_VIEW,
		"web-extension-mode", WEBKIT_WEB_EXTENSION_MODE_MANIFESTV2,
		"default-content-security-policy", csp,
		NULL
	));
}
*/
import "C"
import (
	"log"
	"runtime"
	"unsafe"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// NewBareRelatedWebView creates a bare gotk4 WebView with the related-view property set
// This ensures the popup shares the same session/process context as the parent
// The returned WebView is bare (no wrapper, no initialization) and must be initialized later
//
// Since gotk4-webkitgtk's wrapWebView() is internal and cannot be accessed,
// we manually construct the WebView struct hierarchy using CGO.
func NewBareRelatedWebView(parentView *webkit.WebView) *webkit.WebView {
	if parentView == nil {
		log.Printf("[webkit] NewBareRelatedWebView: parent is nil, creating unrelated WebView")
		return webkit.NewWebView()
	}

	// Get the native C pointer from the gotk4 WebView
	parentObj := glib.BaseObject(parentView)
	if parentObj == nil {
		log.Printf("[webkit] NewBareRelatedWebView: failed to get parent object, creating unrelated WebView")
		return webkit.NewWebView()
	}
	parentNative := (*C.WebKitWebView)(unsafe.Pointer(parentObj.Native()))
	if parentNative == nil {
		log.Printf("[webkit] NewBareRelatedWebView: parent native pointer is nil, creating unrelated WebView")
		return webkit.NewWebView()
	}

	log.Printf("[webkit] Creating bare WebView with related-view property (parent=%p)", parentNative)

	// Create related WebView using CGO helper with g_object_new
	log.Printf("[webkit] Step 3: About to call C.create_related_web_view")
	webViewNative := C.create_related_web_view(parentNative)
	log.Printf("[webkit] Step 4: C.create_related_web_view returned: %p", webViewNative)

	if webViewNative == nil {
		log.Printf("[webkit] NewBareRelatedWebView: C.create_related_web_view returned nil")
		return nil
	}

	// Wrap the C object in a coreglib.Object
	// This takes ownership of the floating reference
	log.Printf("[webkit] Step 5: About to call coreglib.Take")
	obj := coreglib.Take(unsafe.Pointer(webViewNative))
	log.Printf("[webkit] Step 6: coreglib.Take succeeded, obj=%p", obj)

	// Manually construct the WebView struct hierarchy
	// WebView -> WebViewBase -> gtk.Widget -> coreglib.InitiallyUnowned -> coreglib.Object
	//
	// Since we cannot call the internal wrapWebView() function, we must
	// manually construct the full struct hierarchy using the same pattern
	// that gotk4 uses internally.

	log.Printf("[webkit] Step 7: About to create gtk.Widget base")
	// Create the gtk.Widget base
	widget := gtk.Widget{
		InitiallyUnowned: coreglib.InitiallyUnowned{
			Object: obj,
		},
		Object: obj,
		Accessible: gtk.Accessible{
			Object: obj,
		},
		Buildable: gtk.Buildable{
			Object: obj,
		},
		ConstraintTarget: gtk.ConstraintTarget{
			Object: obj,
		},
	}
	log.Printf("[webkit] Step 8: Created gtk.Widget base")

	// Create the WebViewBase
	log.Printf("[webkit] Step 9: About to create WebViewBase")
	webViewBase := webkit.WebViewBase{
		Widget: widget,
	}
	log.Printf("[webkit] Step 10: Created WebViewBase")

	// Create the final WebView
	log.Printf("[webkit] Step 11: About to create final WebView")
	relatedView := &webkit.WebView{
		WebViewBase: webViewBase,
	}
	log.Printf("[webkit] Step 12: Created final WebView: %p", relatedView)

	// Ensure parentView is kept alive until after the CGO call completes
	runtime.KeepAlive(parentView)

	log.Printf("[webkit] Created bare related WebView (parent=%p)", parentNative)
	return relatedView
}

// NewBareExtensionWebView builds a bare WebView for extension popup/options pages.
// It sets related-view (to reuse the parent session), web-extension-mode=ManifestV2,
// and the manifest-provided default CSP at construction time.
func NewBareExtensionWebView(parentView *webkit.WebView, csp string) *webkit.WebView {
	// For extension UI we require a parent (background page or opener) to share context.
	if parentView == nil {
		return nil
	}

	parentObj := glib.BaseObject(parentView)
	if parentObj == nil {
		log.Printf("[webkit] NewBareExtensionWebView: failed to get parent object")
		return nil
	}
	parentNative := (*C.WebKitWebView)(unsafe.Pointer(parentObj.Native()))
	if parentNative == nil {
		log.Printf("[webkit] NewBareExtensionWebView: parent native pointer is nil")
		return nil
	}

	var cspC *C.gchar
	if csp != "" {
		cspC = (*C.gchar)(unsafe.Pointer(C.CString(csp)))
		defer C.free(unsafe.Pointer(cspC))
	}

	webViewNative := C.create_extension_web_view(parentNative, cspC)
	if webViewNative == nil {
		log.Printf("[webkit] NewBareExtensionWebView: create_extension_web_view returned nil")
		return nil
	}

	obj := coreglib.Take(unsafe.Pointer(webViewNative))

	widget := gtk.Widget{
		InitiallyUnowned: coreglib.InitiallyUnowned{
			Object: obj,
		},
		Object: obj,
		Accessible: gtk.Accessible{
			Object: obj,
		},
		Buildable: gtk.Buildable{
			Object: obj,
		},
		ConstraintTarget: gtk.ConstraintTarget{
			Object: obj,
		},
	}

	webViewBase := webkit.WebViewBase{
		Widget: widget,
	}

	extensionView := &webkit.WebView{
		WebViewBase: webViewBase,
	}

	runtime.KeepAlive(parentView)
	return extensionView
}

// NewBareExtensionBackgroundWebView creates a bare WebView for extension background pages.
// Sets web-extension-mode=ManifestV2 and the manifest-provided default CSP at construction time.
// Background pages do NOT have a related-view parent - they ARE the parent for popups.
//
// This aligns with Epiphany's approach where background pages are created in extension mode
// and popups use them as related-view to share session/cookies.
func NewBareExtensionBackgroundWebView(csp string) *webkit.WebView {
	var cspC *C.gchar
	if csp != "" {
		cspC = (*C.gchar)(unsafe.Pointer(C.CString(csp)))
		defer C.free(unsafe.Pointer(cspC))
	}

	webViewNative := C.create_extension_background_web_view(cspC)
	if webViewNative == nil {
		log.Printf("[webkit] NewBareExtensionBackgroundWebView: create_extension_background_web_view returned nil")
		return nil
	}

	obj := coreglib.Take(unsafe.Pointer(webViewNative))

	widget := gtk.Widget{
		InitiallyUnowned: coreglib.InitiallyUnowned{
			Object: obj,
		},
		Object: obj,
		Accessible: gtk.Accessible{
			Object: obj,
		},
		Buildable: gtk.Buildable{
			Object: obj,
		},
		ConstraintTarget: gtk.ConstraintTarget{
			Object: obj,
		},
	}

	webViewBase := webkit.WebViewBase{
		Widget: widget,
	}

	backgroundView := &webkit.WebView{
		WebViewBase: webViewBase,
	}

	log.Printf("[webkit] Created bare extension background WebView (extension mode, CSP=%v)", csp != "")
	return backgroundView
}

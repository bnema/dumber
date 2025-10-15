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
*/
import "C"
import (
	"log"
	"runtime"
	"unsafe"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
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
	parentObj := coreglib.InternObject(parentView)
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

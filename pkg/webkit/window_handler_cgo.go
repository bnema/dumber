//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#include <gdk/gdk.h>
#include <webkit/webkit.h>

// Declarations for Go callbacks
extern GtkWidget* goHandleCreateWebView(unsigned long id, char* uri);

// Handle create signal for popup windows - essential for window.open() support
static GtkWidget* on_create_web_view(WebKitWebView *web_view,
                                     WebKitNavigationAction *navigation_action,
                                     gpointer user_data) {
    WebKitURIRequest *request = webkit_navigation_action_get_request(navigation_action);
    const char* uri = webkit_uri_request_get_uri(request);

    printf("[webkit-create] Create signal for URI: %s\n", uri ? uri : "NULL");

    unsigned long parent_view_id = (unsigned long)user_data;

    // Let Go handler create WebView and return it like Epiphany does
    GtkWidget* new_webview = goHandleCreateWebView(parent_view_id, (char*)uri);

    if (new_webview) {
        printf("[webkit-create] Go handler provided WebView - using workspace pane\n");
        return new_webview;
    } else {
        printf("[webkit-create] Go handler declined - allowing native popup window\n");
        return NULL; // Let WebKit create native popup window
    }
}

static void connect_create_signal(WebKitWebView* web_view, unsigned long id) {
    if (!web_view) return;
    g_signal_connect_data(G_OBJECT(web_view), "create", G_CALLBACK(on_create_web_view), (gpointer)id, NULL, 0);
    printf("[webkit-debug] create-web-view callback ENABLED with WebKit-created WebViews\n");
}
*/
import "C"
import (
	"log"
	"unsafe"
)

//export goHandleCreateWebView
func goHandleCreateWebView(parentID C.ulong, curi *C.char) *C.GtkWidget {
	uri := ""
	if curi != nil {
		uri = C.GoString(curi)
	}

	log.Printf("[webkit] Create signal bypassed: URI=%s (handled by JavaScript bypass)", uri)

	// Always return NULL - window.open is now handled entirely by JavaScript bypass
	// This prevents duplicate pane creation from WebKit's create signal
	return nil
}

// ConnectCreateSignal connects the create signal to a WebView
func ConnectCreateSignal(view *WebView) {
	if view == nil || view.native == nil || view.native.view == nil {
		log.Printf("[webkit] ConnectCreateSignal: invalid WebView")
		return
	}

	webkitView := (*C.WebKitWebView)(unsafe.Pointer(view.native.view))
	C.connect_create_signal(webkitView, C.ulong(view.id))
}

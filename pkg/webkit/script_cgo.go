//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#cgo CFLAGS: -I/usr/include/webkitgtk-6.0
#include <stdlib.h>
#include <webkit/webkit.h>

static gboolean dumber_is_valid_web_view(WebKitWebView *wv) {
    return WEBKIT_IS_WEB_VIEW(wv);
}
*/
import "C"

import "unsafe"

func (w *WebView) canEvaluateJavaScript() bool {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return false
	}
	if C.dumber_is_valid_web_view(w.native.wv) == C.gboolean(0) {
		if w.native != nil {
			w.native.wv = nil
			w.native.view = nil
		}
		w.destroyed = true
		return false
	}
	return true
}

func (w *WebView) InjectScript(js string) error {
	if !w.canEvaluateJavaScript() {
		return ErrNotImplemented
	}
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))
	// Fire-and-forget evaluation using the modern API
	// length: -1 (NUL-terminated), world_name/source_uri: NULL, starting_line_number: 0
	C.webkit_web_view_evaluate_javascript(
		w.native.wv,
		(*C.gchar)(cjs),
		C.gssize(-1),
		nil, // world_name
		nil, // source_uri
		nil, // cancellable
		nil, // callback
		nil, // user_data
	)
	return nil
}

// InjectScriptIntoWorld evaluates JavaScript in the specified script world.
func (w *WebView) InjectScriptIntoWorld(js string, worldName string) error {
	if !w.canEvaluateJavaScript() {
		return ErrNotImplemented
	}
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))

	var cworld *C.gchar
	if worldName != "" {
		cworld = (*C.gchar)(C.CString(worldName))
		defer C.free(unsafe.Pointer(cworld))
	}

	// Evaluate JavaScript in the specified world
	C.webkit_web_view_evaluate_javascript(
		w.native.wv,
		(*C.gchar)(cjs),
		C.gssize(-1),
		cworld, // world_name
		nil,    // source_uri
		nil,    // cancellable
		nil,    // callback
		nil,    // user_data
	)
	return nil
}

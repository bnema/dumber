//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#cgo CFLAGS: -I/usr/include/webkitgtk-6.0
#include <stdlib.h>
#include <webkit/webkit.h>
*/
import "C"

import "unsafe"

func (w *WebView) InjectScript(js string) error {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
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
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
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

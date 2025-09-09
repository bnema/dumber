//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkit2gtk-4.0 gtk+-3.0
#include <stdlib.h>
#include <webkit2/webkit2.h>
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

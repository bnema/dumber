//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkit2gtk-4.0 gtk+-3.0
#include <webkit2/webkit2.h>
*/
import "C"

func (w *WebView) ShowDevTools() error {
    if w == nil || w.native == nil || w.native.wv == nil { return ErrNotImplemented }
    insp := C.webkit_web_view_get_inspector(w.native.wv)
    if insp == nil { return ErrNotImplemented }
    C.webkit_web_inspector_show(insp)
    return nil
}

func (w *WebView) CloseDevTools() error {
    if w == nil || w.native == nil || w.native.wv == nil { return ErrNotImplemented }
    insp := C.webkit_web_view_get_inspector(w.native.wv)
    if insp == nil { return ErrNotImplemented }
    C.webkit_web_inspector_close(insp)
    return nil
}


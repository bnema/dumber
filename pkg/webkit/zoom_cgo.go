//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkit2gtk-4.0 gtk+-3.0
#include <webkit2/webkit2.h>
*/
import "C"

func (w *WebView) SetZoom(level float64) error {
    if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
        return ErrNotImplemented
    }
    if level < 0.25 {
        level = 0.25
    }
    if level > 5.0 {
        level = 5.0
    }
    w.zoom = level
    C.webkit_web_view_set_zoom_level(w.native.wv, C.gdouble(level))
    return nil
}

func (w *WebView) GetZoom() (float64, error) {
    if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
        return 1.0, ErrNotImplemented
    }
    z := float64(C.webkit_web_view_get_zoom_level(w.native.wv))
    w.zoom = z
    return z, nil
}


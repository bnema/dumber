//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#cgo CFLAGS: -I/usr/include/webkitgtk-6.0
#include <webkit/webkit.h>
*/
import "C"

import "fmt"

func (w *WebView) SetZoom(level float64) error {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return ErrNotImplemented
	}
	level = clampDomZoom(level)
	w.zoom = level
	if w.useDomZoom {
		w.SeedDomZoom(level)
		script := fmt.Sprintf(`(function(){try{const lvl=%[1]f;window.__dumber_dom_zoom_level=lvl;if(window.__dumber_applyDomZoom){window.__dumber_applyDomZoom(lvl);}else{if(Math.abs(lvl-1.0)<1e-6){document.documentElement.style.zoom='';if(document.body){document.body.style.zoom='';}}else{const value=lvl.toString();document.documentElement.style.zoom=value;if(document.body){document.body.style.zoom=value;}}}}catch(e){console.error('[dumber] DOM zoom error',e);}})();`, level)
		if err := w.InjectScript(script); err != nil {
			C.webkit_web_view_set_zoom_level(w.native.wv, C.gdouble(level))
		}
	} else {
		C.webkit_web_view_set_zoom_level(w.native.wv, C.gdouble(level))
	}
	w.dispatchZoomChanged(w.zoom)
	return nil
}

func (w *WebView) GetZoom() (float64, error) {
	if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
		return 1.0, ErrNotImplemented
	}
	if w.useDomZoom {
		return w.zoom, nil
	}
	z := float64(C.webkit_web_view_get_zoom_level(w.native.wv))
	w.zoom = z
	return z, nil
}

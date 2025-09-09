//go:build !webkit_cgo

package webkit

const (
    zoomMin = 0.25
    zoomMax = 5.0
)

// SetZoom sets the current zoom factor for the WebView.
func (w *WebView) SetZoom(level float64) error {
    if w == nil || w.destroyed {
        return ErrNotImplemented
    }
    if level < zoomMin {
        level = zoomMin
    }
    if level > zoomMax {
        level = zoomMax
    }
    w.zoom = level
    return nil
}

// GetZoom returns the current zoom factor.
func (w *WebView) GetZoom() (float64, error) {
    if w == nil || w.destroyed {
        return 1.0, ErrNotImplemented
    }
    return w.zoom, nil
}

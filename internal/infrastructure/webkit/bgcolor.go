package webkit

import (
	"sync"

	"github.com/jwijenbergh/puregotk/v4/gdk"
)

// bgColor holds RGBA components for WebView background color (eliminates white flash).
// It is thread-safe and can be embedded in pool/factory structs.
type bgColor struct {
	r, g, b, a float32
	mu         sync.RWMutex
}

// set stores the background color components.
func (c *bgColor) set(r, g, b, a float32) {
	c.mu.Lock()
	c.r, c.g, c.b, c.a = r, g, b, a
	c.mu.Unlock()
}

// get returns the background color components.
func (c *bgColor) get() (r, g, b, a float32) {
	c.mu.RLock()
	r, g, b, a = c.r, c.g, c.b, c.a
	c.mu.RUnlock()
	return
}

// toGdkRGBA returns a *gdk.RGBA if a valid color is configured (alpha > 0), nil otherwise.
func (c *bgColor) toGdkRGBA() *gdk.RGBA {
	r, g, b, a := c.get()
	if a <= 0 {
		return nil
	}
	return &gdk.RGBA{Red: r, Green: g, Blue: b, Alpha: a}
}

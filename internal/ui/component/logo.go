// Package component provides reusable GTK UI components.
package component

import (
	"sync"

	"github.com/bnema/dumber/assets"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

var (
	logoTexture     *gdk.Texture
	logoTextureOnce sync.Once
)

// GetLogoTexture returns a cached GDK texture of the dumber logo.
// Safe to call from any goroutine; lazily initializes on first call.
// Returns nil if the logo cannot be loaded.
func GetLogoTexture() *gdk.Texture {
	logoTextureOnce.Do(func() {
		data := assets.LogoSVG
		if len(data) == 0 {
			return
		}
		bytes := glib.NewBytes(data, uint(len(data)))
		if bytes == nil {
			return
		}
		texture, err := gdk.NewTextureFromBytes(bytes)
		if err != nil {
			return
		}
		logoTexture = texture
	})
	return logoTexture
}

// LogoSVGBytes returns the raw SVG bytes of the dumber logo.
// Useful for infrastructure services that need the logo data without GTK dependencies.
func LogoSVGBytes() []byte {
	return assets.LogoSVG
}

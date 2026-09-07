// Package component provides reusable GTK UI components.
package component

import (
	"errors"
	"sync"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
)

var errLogoDecode = errors.New("logo decode failed")

var (
	logoTexture     *gdk.Texture
	logoTextureOnce sync.Once
	// logoAssetBytes supplies the raster logo bytes. Overridden in tests to
	// count requests without native decoding.
	logoAssetBytes = func() []byte { return assets.LogoPNG512 }
	// logoDecodeTexture converts raster bytes to a texture. Overridden in
	// tests with a fake; production keeps GBytes/texture lifetime by
	// releasing the temporary byte reference after the native constructor
	// no longer needs it.
	logoDecodeTexture = func(data []byte) (*gdk.Texture, error) {
		bytes := glib.NewBytes(data, uint(len(data)))
		if bytes == nil {
			return nil, errLogoDecode
		}
		texture, err := gdk.NewTextureFromBytes(bytes)
		bytes.Unref()
		if err != nil {
			return nil, err
		}
		return texture, nil
	}
)

// GetLogoTexture returns a cached GDK texture of the dumber logo.
// Safe to call from any goroutine; lazily initializes on first call.
// Decodes the packaged 512-pixel PNG raster asset; the displayed size and
// one-texture-per-process lifetime are unchanged.
// Returns nil if the logo cannot be loaded.
func GetLogoTexture() *gdk.Texture {
	logoTextureOnce.Do(func() {
		data := logoAssetBytes()
		if len(data) == 0 {
			return
		}
		texture, err := logoDecodeTexture(data)
		if err != nil || texture == nil {
			return
		}
		logoTexture = texture
	})
	return logoTexture
}

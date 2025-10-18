package webkit

/*
#cgo pkg-config: webkitgtk-6.0
#include <webkit/webkit.h>
*/
import "C"

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/url"
	"runtime"
	"unsafe"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/core/gerror"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	gio "github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// GdkTexture is an alias for gotk4's Texture type
type GdkTexture = gdk.Texture

// FaviconDatabase is an alias for WebKit's FaviconDatabase
type FaviconDatabase = webkit.FaviconDatabase

// SaveTextureAsPNG saves a GdkTexture as a PNG file at target size
// WebKit provides GdkTexture which can be any format (ICO, PNG, JPEG, SVG)
// This function normalizes everything to PNG at the target size (32x32)
func SaveTextureAsPNG(texture *gdk.Texture, path string, targetSize int) error {
	if texture == nil {
		return fmt.Errorf("texture cannot be nil")
	}

	width := texture.Width()
	height := texture.Height()

	// If texture needs scaling, create scaled version
	if width != targetSize || height != targetSize {
		texture = ScaleTexture(texture, targetSize, targetSize)
		if texture == nil {
			return fmt.Errorf("failed to scale texture")
		}
	}

	// Save texture as PNG using built-in method
	if !texture.SaveToPNG(path) {
		return fmt.Errorf("failed to save PNG to %s", path)
	}

	return nil
}

// ScaleTexture scales a texture to the target size using GdkPixbuf
// Uses high-quality bilinear interpolation
func ScaleTexture(texture *gdk.Texture, targetWidth, targetHeight int) *gdk.Texture {
	if texture == nil {
		return nil
	}

	// Get PNG bytes from texture
	pngBytes := texture.SaveToPNGBytes()
	if pngBytes == nil {
		return nil
	}

	// Load as pixbuf using memory input stream
	ctx := context.Background()
	stream := gio.NewMemoryInputStreamFromBytes(pngBytes)
	pixbuf, err := gdkpixbuf.NewPixbufFromStream(ctx, stream)
	if err != nil || pixbuf == nil {
		return nil
	}

	// Scale pixbuf
	scaled := pixbuf.ScaleSimple(targetWidth, targetHeight, gdkpixbuf.InterpBilinear)
	if scaled == nil {
		return nil
	}

	// Convert back to texture
	return gdk.NewTextureForPixbuf(scaled)
}

// HashURL generates an MD5 hash of a URL for filename generation
func HashURL(pageURL string) []byte {
	hash := md5.Sum([]byte(pageURL))
	return hash[:]
}

// ParseURL is a convenience wrapper around url.Parse
func ParseURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

// FaviconFinishSafe mirrors gotk4's FaviconFinish but guards against NULL returns.
func FaviconFinishSafe(database *webkit.FaviconDatabase, result gio.AsyncResulter) (*gdk.Texture, error) {
	if database == nil || result == nil {
		return nil, fmt.Errorf("favicon finish: invalid arguments")
	}

	cDatabase := (*C.WebKitFaviconDatabase)(unsafe.Pointer(glib.BaseObject(database).Native()))
	cResult := (*C.GAsyncResult)(unsafe.Pointer(glib.BaseObject(result).Native()))

	var cErr *C.GError
	cTexture := C.webkit_favicon_database_get_favicon_finish(cDatabase, cResult, &cErr)
	runtime.KeepAlive(database)
	runtime.KeepAlive(result)

	if cErr != nil {
		err := gerror.Take(unsafe.Pointer(cErr))
		return nil, err
	}
	if cTexture == nil {
		return nil, nil
	}

	object := coreglib.AssumeOwnership(unsafe.Pointer(cTexture))
	casted := object.WalkCast(func(obj coreglib.Objector) bool {
		_, ok := obj.(gdk.Texturer)
		return ok
	})

	texturer, ok := casted.(gdk.Texturer)
	if !ok {
		return nil, fmt.Errorf("favicon finish: unexpected type %T", casted)
	}

	texture := gdk.BaseTexture(texturer)
	if texture == nil {
		return nil, fmt.Errorf("favicon finish: texture unavailable")
	}

	return texture, nil
}

// TextureFromImageBytes creates a texture from raw image bytes.
// Supported formats depend on the available GdkPixbuf loaders (PNG, SVG, etc.).
func TextureFromImageBytes(data []byte) (*gdk.Texture, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("image data is empty")
	}

	// Convert []byte to glib.Bytes for gio API
	glibBytes := glib.NewBytes(data)
	stream := gio.NewMemoryInputStreamFromBytes(glibBytes)
	pixbuf, err := gdkpixbuf.NewPixbufFromStream(context.Background(), stream)
	if err != nil {
		return nil, fmt.Errorf("load pixbuf: %w", err)
	}

	texture := gdk.NewTextureForPixbuf(pixbuf)
	if texture == nil {
		return nil, fmt.Errorf("create texture from pixbuf failed")
	}

	return texture, nil
}

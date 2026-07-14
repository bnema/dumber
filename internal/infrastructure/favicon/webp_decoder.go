package favicon

import (
	"errors"
	"image"
	"math"

	"github.com/bnema/purego-webp/libwebp"
)

const (
	// maxWebPInputBytes bounds untrusted compressed favicon input before it is
	// handed to libwebp.
	maxWebPInputBytes = 5 << 20
	// maxWebPDimension and maxWebPPixels bound the final RGBA allocation.
	maxWebPDimension = 4096
	maxWebPPixels    = 4 << 20
)

var (
	errWebPInputTooLarge   = errors.New("webp favicon input exceeds size limit")
	errWebPImageTooLarge   = errors.New("webp favicon dimensions exceed size limit")
	errWebPInvalidMetadata = errors.New("invalid webp favicon metadata")
)

// webpRuntime is the narrow infrastructure seam around the dynamically loaded
// libwebp binding. It deliberately exposes only Go values, keeping backend
// types out of ports and the domain.
type webpRuntime interface {
	Info(data []byte) (width, height int, err error)
	DecodeRGBAInto(data, pix []byte, stride, width, height int) error
}

type libwebpRuntime struct{}

func (libwebpRuntime) Info(data []byte) (int, int, error) {
	width, height, ok, err := libwebp.WebPGetInfo(data)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, errWebPInvalidMetadata
	}
	return width, height, nil
}

func (libwebpRuntime) DecodeRGBAInto(data, pix []byte, stride, width, height int) error {
	return libwebp.WebPDecodeRGBAIntoWithInfo(data, pix, stride, width, height)
}

// webPDecoder owns the failure boundary for the optional libwebp runtime.
type webPDecoder struct{ runtime webpRuntime }

func newWebPDecoder(runtime webpRuntime) *webPDecoder { return &webPDecoder{runtime: runtime} }

func newDefaultWebPDecoder() *webPDecoder { return newWebPDecoder(libwebpRuntime{}) }

func (d *webPDecoder) Decode(data []byte) (*image.NRGBA, error) {
	if len(data) == 0 {
		return nil, errWebPInvalidMetadata
	}
	if len(data) > maxWebPInputBytes {
		return nil, errWebPInputTooLarge
	}
	if d == nil || d.runtime == nil {
		return nil, errors.New("webp decoder unavailable")
	}
	width, height, err := d.runtime.Info(data)
	if err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, errWebPInvalidMetadata
	}
	if width > maxWebPDimension || height > maxWebPDimension || width > maxWebPPixels/height {
		return nil, errWebPImageTooLarge
	}
	if width > math.MaxInt/4 {
		return nil, errWebPImageTooLarge
	}
	stride := width * 4
	if height > math.MaxInt/stride {
		return nil, errWebPImageTooLarge
	}

	// Allocate exactly the destination buffer used by libwebp; no native image
	// buffer or second decoded copy crosses this adapter boundary.
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	if err := d.runtime.DecodeRGBAInto(data, img.Pix, img.Stride, width, height); err != nil {
		return nil, err
	}
	return img, nil
}

// Package favicon provides favicon fetching and caching infrastructure.
package favicon

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"golang.org/x/image/draw"
)

const (
	// NormalizedIconSize is the standard size for dmenu/fuzzel icons.
	NormalizedIconSize = 32
)

// ResizePNG loads a PNG from srcPath, resizes it to a square of the given size,
// and saves the result to dstPath. Uses golang.org/x/image/draw for high-quality
// resizing with CatmullRom interpolation.
func ResizePNG(srcPath, dstPath string, size int) error {
	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	// Decode PNG
	srcImg, err := png.Decode(srcFile)
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}

	// Calculate crop region to maintain aspect ratio (center crop to square)
	srcBounds := srcImg.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	var cropRect image.Rectangle
	if srcW > srcH {
		// Wider than tall - crop sides
		offset := (srcW - srcH) / 2
		cropRect = image.Rect(srcBounds.Min.X+offset, srcBounds.Min.Y, srcBounds.Min.X+offset+srcH, srcBounds.Max.Y)
	} else if srcH > srcW {
		// Taller than wide - crop top/bottom
		offset := (srcH - srcW) / 2
		cropRect = image.Rect(srcBounds.Min.X, srcBounds.Min.Y+offset, srcBounds.Max.X, srcBounds.Min.Y+offset+srcW)
	} else {
		// Already square
		cropRect = srcBounds
	}

	// Create cropped subimage
	croppedImg := cropImage(srcImg, cropRect)

	// Create destination image
	dstImg := image.NewRGBA(image.Rect(0, 0, size, size))

	// Scale using CatmullRom for high-quality interpolation
	draw.CatmullRom.Scale(dstImg, dstImg.Bounds(), croppedImg, croppedImg.Bounds(), draw.Over, nil)

	// Create destination file
	dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, diskCacheFilePerm)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	// Encode as PNG
	if err := png.Encode(dstFile, dstImg); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}

	return nil
}

// cropImage returns a cropped portion of the source image.
func cropImage(src image.Image, rect image.Rectangle) image.Image {
	// If the source supports SubImage, use it for efficiency
	if subImager, ok := src.(interface {
		SubImage(r image.Rectangle) image.Image
	}); ok {
		return subImager.SubImage(rect)
	}

	// Otherwise, copy pixels manually
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			dst.Set(x, y, src.At(rect.Min.X+x, rect.Min.Y+y))
		}
	}
	return dst
}

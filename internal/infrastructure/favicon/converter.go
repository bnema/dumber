package favicon

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime"
	"strings"

	appport "github.com/bnema/dumber/internal/application/port"
	"golang.org/x/image/draw"
)

// ImageConverter converts safe favicon image formats into normalized PNGs.
type ImageConverter struct{}

func NewImageConverter() *ImageConverter { return &ImageConverter{} }

func (*ImageConverter) Convert(ctx context.Context, original []byte, contentType string, sizes []int) (*appport.ConvertedFavicon, error) {
	_ = ctx
	if len(original) == 0 {
		return nil, fmt.Errorf("%w: empty image", appport.ErrFaviconMiss)
	}

	img, err := decodeFaviconImage(original, normalizeContentType(contentType))
	if err != nil {
		return nil, err
	}
	pngBytes, err := encodePNG(img)
	if err != nil {
		return nil, err
	}
	out := &appport.ConvertedFavicon{PNG: pngBytes, SizedPNG: make(map[int][]byte)}
	for _, size := range sizes {
		if size <= 0 {
			continue
		}
		resized, err := resizeToPNG(img, size)
		if err != nil {
			return nil, err
		}
		out.SizedPNG[size] = resized
	}
	return out, nil
}

func decodeFaviconImage(data []byte, ct string) (image.Image, error) {
	switch ct {
	case "image/svg+xml":
		return nil, fmt.Errorf("%w: svg favicon conversion is disabled", appport.ErrFaviconMiss)
	case "image/x-icon", "image/vnd.microsoft.icon", "image/ico":
		img, err := decodeICO(data)
		if err != nil {
			return nil, err
		}
		return img, nil
	case "image/webp":
		return nil, fmt.Errorf("%w: webp favicon conversion is disabled", appport.ErrFaviconMiss)
	}
	if ct != "" && ct != "image/png" && ct != "image/jpeg" && ct != "image/gif" && ct != "image/webp" {
		return nil, fmt.Errorf("%w: unsupported favicon content type %s", appport.ErrFaviconMiss, ct)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: decode favicon: %w", appport.ErrFaviconMiss, err)
	}
	return img, nil
}

func normalizeContentType(contentType string) string {
	ct, _, err := mime.ParseMediaType(strings.TrimSpace(strings.ToLower(contentType)))
	if err == nil {
		return ct
	}
	return strings.TrimSpace(strings.ToLower(contentType))
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func resizeToPNG(src image.Image, size int) ([]byte, error) {
	crop := cropImage(src, centerSquare(src.Bounds()))
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), crop, crop.Bounds(), draw.Over, nil)
	return encodePNG(dst)
}

func centerSquare(bounds image.Rectangle) image.Rectangle {
	w, h := bounds.Dx(), bounds.Dy()
	if w > h {
		o := (w - h) / 2
		return image.Rect(bounds.Min.X+o, bounds.Min.Y, bounds.Min.X+o+h, bounds.Max.Y)
	}
	if h > w {
		o := (h - w) / 2
		return image.Rect(bounds.Min.X, bounds.Min.Y+o, bounds.Max.X, bounds.Min.Y+o+w)
	}
	return bounds
}

func decodeICO(data []byte) (image.Image, error) {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil, fmt.Errorf("%w: malformed ico header", appport.ErrFaviconMiss)
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count <= 0 || len(data) < 6+count*16 {
		return nil, fmt.Errorf("%w: malformed ico directory", appport.ErrFaviconMiss)
	}
	var best []byte
	bestBitCount := 0
	bestArea := -1
	for i := 0; i < count; i++ {
		entry := data[6+i*16 : 6+(i+1)*16]
		w, h := int(entry[0]), int(entry[1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		size := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if size <= 0 || offset < 0 || offset+size > len(data) {
			continue
		}
		payload := data[offset : offset+size]
		area := w * h
		if area > bestArea {
			bestArea = area
			best = payload
			bestBitCount = int(binary.LittleEndian.Uint16(entry[6:8]))
		}
	}
	if len(best) == 0 {
		return nil, fmt.Errorf("%w: ico contains no image", appport.ErrFaviconMiss)
	}
	if bytes.HasPrefix(best, []byte("\x89PNG\r\n\x1a\n")) {
		img, err := png.Decode(bytes.NewReader(best))
		if err != nil {
			return nil, fmt.Errorf("%w: decode ico png: %w", appport.ErrFaviconMiss, err)
		}
		return img, nil
	}
	img, err := decodeICODIB(best, bestBitCount)
	if err != nil {
		return nil, fmt.Errorf("%w: decode ico bitmap: %w", appport.ErrFaviconMiss, err)
	}
	return img, nil
}

func decodeICODIB(data []byte, bitCountHint int) (image.Image, error) {
	const dibHeaderSize = 40
	if len(data) < dibHeaderSize {
		return nil, fmt.Errorf("malformed dib header")
	}
	headerSize := int(binary.LittleEndian.Uint32(data[0:4]))
	if headerSize < dibHeaderSize || len(data) < headerSize {
		return nil, fmt.Errorf("unsupported dib header")
	}
	width := int(int32(binary.LittleEndian.Uint32(data[4:8])))
	heightAll := int(int32(binary.LittleEndian.Uint32(data[8:12])))
	planes := binary.LittleEndian.Uint16(data[12:14])
	bitCount := int(binary.LittleEndian.Uint16(data[14:16]))
	compression := binary.LittleEndian.Uint32(data[16:20])
	if bitCount == 0 {
		bitCount = bitCountHint
	}
	if width <= 0 || heightAll <= 0 || planes != 1 || compression != 0 {
		return nil, fmt.Errorf("unsupported dib metadata")
	}
	height := heightAll / 2
	if height <= 0 {
		return nil, fmt.Errorf("invalid dib height")
	}
	bytesPerPixel := bitCount / 8
	if bitCount != 24 && bitCount != 32 {
		return nil, fmt.Errorf("unsupported dib bit depth %d", bitCount)
	}
	stride := ((width*bitCount + 31) / 32) * 4
	pixelOffset := headerSize
	if len(data) < pixelOffset+stride*height {
		return nil, fmt.Errorf("truncated dib pixels")
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	const opaqueAlpha = 255
	for y := 0; y < height; y++ {
		srcY := height - 1 - y
		row := data[pixelOffset+srcY*stride:]
		for x := 0; x < width; x++ {
			i := x * bytesPerPixel
			b, g, r := row[i], row[i+1], row[i+2]
			a := byte(opaqueAlpha)
			if bitCount == 32 {
				a = row[i+3]
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	return img, nil
}

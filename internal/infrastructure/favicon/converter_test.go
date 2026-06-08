package favicon

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	appport "github.com/bnema/dumber/internal/application/port"
)

func TestConverterPNGNormalizesAndSizes(t *testing.T) {
	conv := NewImageConverter()
	out, err := conv.Convert(context.Background(), testPNG(t, 64, 48), "image/png", []int{32})
	if err != nil {
		t.Fatal(err)
	}
	if _, decodeErr := png.Decode(bytes.NewReader(out.PNG)); decodeErr != nil {
		t.Fatalf("normalized png invalid: %v", decodeErr)
	}
	img, err := png.Decode(bytes.NewReader(out.SizedPNG[32]))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("size = %v", img.Bounds())
	}
}

func TestConverterAcceptedFaviconFormatsProducePNGs(t *testing.T) {
	conv := NewImageConverter()
	for _, tc := range []struct {
		name, ct string
		data     []byte
	}{
		{"png", "image/png", testPNG(t, 16, 16)},
		{"ico with png payload", "image/x-icon", testICOWithPNG(t, testPNG(t, 16, 16))},
		{"ico with bitmap payload", "image/x-icon", testICOWithDIB(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := conv.Convert(context.Background(), tc.data, tc.ct, []int{32})
			if err != nil {
				t.Fatal(err)
			}
			if _, decodeErr := png.Decode(bytes.NewReader(out.PNG)); decodeErr != nil {
				t.Fatalf("normalized png invalid: %v", decodeErr)
			}
			img, err := png.Decode(bytes.NewReader(out.SizedPNG[32]))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
				t.Fatalf("size = %v", img.Bounds())
			}
		})
	}
}

func TestConverterRejectsFormatsThatWouldProducePlaceholders(t *testing.T) {
	conv := NewImageConverter()
	for _, tc := range []struct {
		name, ct string
		data     []byte
	}{
		{"svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="#ff0000"/></svg>`)},
		{"webp", "image/webp", []byte("RIFF\x04\x00\x00\x00WEBPVP8 ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := conv.Convert(context.Background(), tc.data, tc.ct, []int{32}); !errors.Is(err, appport.ErrFaviconMiss) {
				t.Fatalf("err = %v, want favicon miss", err)
			}
		})
	}
}

func TestConverterMalformedInputsAreMisses(t *testing.T) {
	conv := NewImageConverter()
	for _, tc := range []struct {
		name, ct string
		data     []byte
	}{
		{"ico", "image/x-icon", []byte{0, 0, 1, 0}},
		{"svg", "image/svg+xml", []byte(`not svg`)},
		{"webp", "image/webp", []byte("not webp")},
		{"malformed", "image/png", []byte("not an image")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := conv.Convert(context.Background(), tc.data, tc.ct, []int{32}); !errors.Is(err, appport.ErrFaviconMiss) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func testICOWithDIB(t *testing.T) []byte {
	t.Helper()
	const width, height, bitCount = 2, 2, 32
	var dib bytes.Buffer
	writeLE32 := func(v uint32) { dib.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}) }
	writeLE16 := func(v uint16) { dib.Write([]byte{byte(v), byte(v >> 8)}) }
	writeLE32(40)                 // BITMAPINFOHEADER size
	writeLE32(width)              // width
	writeLE32(height * 2)         // xor + and mask height
	writeLE16(1)                  // planes
	writeLE16(bitCount)           // bit count
	writeLE32(0)                  // BI_RGB
	writeLE32(width * height * 4) // image size
	writeLE32(0)
	writeLE32(0) // pels per meter
	writeLE32(0)
	writeLE32(0)                                      // colors
	dib.Write([]byte{0, 0, 255, 255, 0, 255, 0, 255}) // bottom row BGRA
	dib.Write([]byte{255, 0, 0, 255, 0, 0, 0, 255})   // top row BGRA
	return testICOWithPayload(t, dib.Bytes(), bitCount)
}

func testICOWithPNG(t *testing.T, pngData []byte) []byte {
	t.Helper()
	return testICOWithPayload(t, pngData, 32)
}

func testICOWithPayload(t *testing.T, payload []byte, bitCount byte) []byte {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	buf.Write([]byte{0, 0, 1, 0, 1, 0})  // reserved, type icon, count
	buf.Write([]byte{16, 16, 0, 0})      // width, height, colors, reserved
	buf.Write([]byte{1, 0, bitCount, 0}) // planes, bit count
	size := uint32(len(payload))
	offset := uint32(6 + 16)
	buf.Write([]byte{byte(size), byte(size >> 8), byte(size >> 16), byte(size >> 24)})
	buf.Write([]byte{byte(offset), byte(offset >> 8), byte(offset >> 16), byte(offset >> 24)})
	buf.Write(payload)
	return buf.Bytes()
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

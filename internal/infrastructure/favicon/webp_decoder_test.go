package favicon

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	appport "github.com/bnema/dumber/internal/application/port"
)

func TestConverterWebPConvertsAlphaFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/alpha.webp")
	if err != nil {
		t.Fatal(err)
	}
	conv := NewImageConverter()
	out, err := conv.Convert(context.Background(), data, "image/webp", []int{32})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out.PNG))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); got.A != 64 || got.R != 255 {
		t.Fatalf("alpha pixel = %#v, want red alpha 64", got)
	}
	sized, err := png.Decode(bytes.NewReader(out.SizedPNG[32]))
	if err != nil {
		t.Fatal(err)
	}
	if sized.Bounds() != image.Rect(0, 0, 32, 32) {
		t.Fatalf("sized WebP bounds = %v, want 32x32", sized.Bounds())
	}
}

func TestConverterWebPFailuresAreFaviconMisses(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		dec  webpRuntime
	}{
		{name: "truncated", data: []byte("RIFF\x04\x00\x00\x00WEBPVP8 "), dec: &fakeWebPRuntime{infoErr: errors.New("truncated")}},
		{name: "unavailable", data: []byte("webp"), dec: &fakeWebPRuntime{infoErr: errors.New("libwebp unavailable")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conv := newImageConverterWithWebPRuntime(tc.dec)
			if _, err := conv.Convert(context.Background(), tc.data, "image/webp", nil); !errors.Is(err, appport.ErrFaviconMiss) {
				t.Fatalf("err = %v, want favicon miss", err)
			}
		})
	}
}

func TestWebPDecoderRejectsInputAndDecodedLimitsBeforeAllocation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		data          []byte
		width, height int
	}{
		{name: "input", data: bytes.Repeat([]byte("x"), maxWebPInputBytes+1), width: 1, height: 1},
		{name: "dimension", data: []byte("webp"), width: maxWebPDimension + 1, height: 1},
		{name: "pixels", data: []byte("webp"), width: maxWebPDimension, height: maxWebPDimension},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := &fakeWebPRuntime{width: tc.width, height: tc.height}
			_, err := newWebPDecoder(runtime).Decode(tc.data)
			if err == nil {
				t.Fatal("Decode succeeded")
			}
			if runtime.decodeCalls != 0 {
				t.Fatalf("decode calls = %d, want no allocation/decode", runtime.decodeCalls)
			}
		})
	}
}

type fakeWebPRuntime struct {
	width, height int
	infoErr       error
	decodeCalls   int
}

func (r *fakeWebPRuntime) Info(_ []byte) (int, int, error) { return r.width, r.height, r.infoErr }
func (r *fakeWebPRuntime) DecodeRGBAInto(_, pix []byte, _, _, _ int) error {
	r.decodeCalls++
	for i := range pix {
		pix[i] = 0xff
	}
	return nil
}

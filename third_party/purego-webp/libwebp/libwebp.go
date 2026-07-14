// Package libwebp exposes C-first bindings to libwebp using purego.
//
// The API mirrors libwebp function naming and behavior as closely as possible,
// while using Go slices and errors for safety in common call paths.
package libwebp

import (
	"errors"
	"fmt"
	"math"
	"unsafe"

	lowlevel "github.com/bnema/purego-webp/internal/libwebp"
)

var (
	// ErrInvalidData indicates invalid or empty input bitstream/data.
	ErrInvalidData = errors.New("libwebp: invalid webp data")
	// ErrDecodeFailed indicates libwebp decode failure.
	ErrDecodeFailed = errors.New("libwebp: decode failed")
	// ErrEncodeFailed indicates libwebp encode failure.
	ErrEncodeFailed = errors.New("libwebp: encode failed")
	// ErrInvalidDimension indicates invalid image width/height.
	ErrInvalidDimension = errors.New("libwebp: invalid dimensions")
	// ErrInvalidStride indicates invalid row stride for the provided buffer.
	ErrInvalidStride = errors.New("libwebp: invalid stride")
	// ErrBufferTooSmall indicates the destination buffer cannot hold output.
	ErrBufferTooSmall = errors.New("libwebp: output buffer too small")
	// ErrNotAvailable indicates the function is not available in the loaded
	// libwebp version. Use the corresponding Available() helper to check first.
	ErrNotAvailable = errors.New("libwebp: function not available in loaded library version")
)

// VP8StatusCode is the status enum used by libwebp decode APIs.
type VP8StatusCode int32

const (
	VP8StatusOK              VP8StatusCode = 0
	VP8StatusOutOfMemory     VP8StatusCode = 1
	VP8StatusInvalidParam    VP8StatusCode = 2
	VP8StatusBitstreamError  VP8StatusCode = 3
	VP8StatusUnsupportedFeat VP8StatusCode = 4
	VP8StatusSuspended       VP8StatusCode = 5
	VP8StatusUserAbort       VP8StatusCode = 6
	VP8StatusNotEnoughData   VP8StatusCode = 7
)

type BitstreamFeatures struct {
	Width        int
	Height       int
	HasAlpha     bool
	HasAnimation bool
	Format       int
}

// DecBuffer is the low-level decode output buffer struct from libwebp.
type DecBuffer = lowlevel.WebPDecBuffer

// DecoderOptions is the low-level decoder options struct from libwebp.
type DecoderOptions = lowlevel.WebPDecoderOptions

// DecoderConfig is the low-level decoder config struct from libwebp.
type DecoderConfig = lowlevel.WebPDecoderConfig

// Config is the low-level encoder config struct from libwebp.
type Config = lowlevel.WebPConfig

// MemoryWriter is libwebp's in-memory writer struct.
type MemoryWriter = lowlevel.WebPMemoryWriter

// Picture is the low-level picture struct used by advanced encode APIs.
type Picture = lowlevel.WebPPicture

const (
	// Encoder presets used by WebPConfigPreset.
	PresetDefault = 0
	PresetPicture = 1
	PresetPhoto   = 2
	PresetDrawing = 3
	PresetIcon    = 4
	PresetText    = 5

	HintDefault = 0
	HintPicture = 1
	HintPhoto   = 2
	HintGraph   = 3
	HintLast    = 4

	// Decode output colorspace/mode constants from decode.h.
	ModeRGB      = 0
	ModeRGBA     = 1
	ModeBGR      = 2
	ModeBGRA     = 3
	ModeARGB     = 4
	ModeRGBA4444 = 5
	ModeRGB565   = 6
	ModergbA     = 7
	ModebgrA     = 8
	ModeArgb     = 9
	ModergbA4444 = 10
	ModeYUV      = 11
	ModeYUVA     = 12
	ModeLast     = 13
)

// Available reports whether libwebp can be loaded in the current environment.
func Available() bool {
	return lowlevel.Available()
}

// Version returns decoder and encoder library versions (packed hex format).
func Version() (decoder uint32, encoder uint32, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, 0, err
	}

	return uint32(lowlevel.WebPGetDecoderVersion()), uint32(lowlevel.WebPGetEncoderVersion()), nil
}

// WebPGetInfo validates the bitstream and returns image dimensions.
func WebPGetInfo(data []byte) (width, height int, ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, 0, false, err
	}
	if len(data) == 0 {
		return 0, 0, false, nil
	}

	var w, h int32
	ret := lowlevel.WebPGetInfo(&data[0], uintptr(len(data)), &w, &h)
	return int(w), int(h), ret != 0, nil
}

// WebPGetFeatures returns parsed bitstream features and decode status.
func WebPGetFeatures(data []byte) (features BitstreamFeatures, status VP8StatusCode, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return BitstreamFeatures{}, 0, err
	}
	if len(data) == 0 {
		return BitstreamFeatures{}, VP8StatusNotEnoughData, nil
	}

	var raw lowlevel.WebPBitstreamFeatures
	status = VP8StatusCode(lowlevel.WebPGetFeaturesInternal(&data[0], uintptr(len(data)), &raw, lowlevel.WebPDecoderABIVersion))

	return BitstreamFeatures{
		Width:        int(raw.Width),
		Height:       int(raw.Height),
		HasAlpha:     raw.HasAlpha != 0,
		HasAnimation: raw.HasAnimation != 0,
		Format:       int(raw.Format),
	}, status, nil
}

// WebPInitDecBuffer initializes a decode buffer with ABI-checked defaults.
func WebPInitDecBuffer(buffer *DecBuffer) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if buffer == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPInitDecBufferInternal(buffer, lowlevel.WebPDecoderABIVersion) != 0, nil
}

// WebPFreeDecBuffer releases any memory owned by the decode buffer.
func WebPFreeDecBuffer(buffer *DecBuffer) error {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return err
	}
	if buffer == nil {
		return nil
	}

	lowlevel.WebPFreeDecBuffer(buffer)
	return nil
}

// WebPInitDecoderConfig initializes a decoder config with ABI-checked defaults.
func WebPInitDecoderConfig(config *DecoderConfig) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if config == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPInitDecoderConfigInternal(config, lowlevel.WebPDecoderABIVersion) != 0, nil
}

// WebPValidateDecoderConfigAvailable reports whether WebPValidateDecoderConfig
// is available in the loaded libwebp. The symbol was added in libwebp 1.6.0
// (released 2025-03); most distributions still ship an earlier version.
func WebPValidateDecoderConfigAvailable() bool {
	return lowlevel.ValidateDecoderConfigAvailable()
}

// WebPValidateDecoderConfig validates decoder config values.
// It returns ErrNotAvailable if the loaded libwebp predates 1.6.0.
func WebPValidateDecoderConfig(config *DecoderConfig) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if !lowlevel.ValidateDecoderConfigAvailable() {
		return false, ErrNotAvailable
	}
	if config == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPValidateDecoderConfig(config) != 0, nil
}

// WebPConfigInit initializes encoder config to default preset/quality.
func WebPConfigInit(config *Config) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if config == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPConfigInitInternal(config, PresetDefault, 75, lowlevel.WebPEncoderABIVersion) != 0, nil
}

// WebPConfigPreset initializes encoder config with the given preset and quality.
func WebPConfigPreset(config *Config, preset int32, quality float32) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if config == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPConfigInitInternal(config, preset, quality, lowlevel.WebPEncoderABIVersion) != 0, nil
}

// WebPConfigLosslessPreset applies a built-in lossless level preset.
func WebPConfigLosslessPreset(config *Config, level int32) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if config == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPConfigLosslessPreset(config, level) != 0, nil
}

// WebPValidateConfig validates encoder config values.
func WebPValidateConfig(config *Config) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if config == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPValidateConfig(config) != 0, nil
}

// WebPPictureInit initializes a picture struct with ABI-checked defaults.
func WebPPictureInit(picture *Picture) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if picture == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPPictureInitInternal(picture, lowlevel.WebPEncoderABIVersion) != 0, nil
}

// WebPMemoryWriterInit initializes a memory writer instance.
func WebPMemoryWriterInit(writer *MemoryWriter) error {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return err
	}
	if writer == nil {
		return ErrInvalidData
	}

	lowlevel.WebPMemoryWriterInit(writer)
	return nil
}

// WebPMemoryWriterClear frees memory owned by a memory writer.
func WebPMemoryWriterClear(writer *MemoryWriter) error {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return err
	}
	if writer == nil {
		return nil
	}

	lowlevel.WebPMemoryWriterClear(writer)
	return nil
}

// WebPEncode runs advanced encoding with explicit config and picture structs.
func WebPEncode(config *Config, picture *Picture) (ok bool, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return false, err
	}
	if config == nil || picture == nil {
		return false, ErrInvalidData
	}

	return lowlevel.WebPEncode(config, picture) != 0, nil
}

// WebPINewDecoder creates an incremental decoder using the provided output buffer.
func WebPINewDecoder(outputBuffer *DecBuffer) (uintptr, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}

	idec := lowlevel.WebPINewDecoder(outputBuffer)
	if idec == 0 {
		return 0, ErrDecodeFailed
	}

	return idec, nil
}

// WebPINewRGB creates an incremental decoder producing packed RGB-family output.
func WebPINewRGB(csp int32, outputBuffer []byte, outputStride int32) (uintptr, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}

	ptr, size := ptrAndSize(outputBuffer)
	idec := lowlevel.WebPINewRGB(csp, ptr, size, outputStride)
	if idec == 0 {
		return 0, ErrDecodeFailed
	}

	return idec, nil
}

// WebPINewYUV creates an incremental decoder producing YUV planes.
func WebPINewYUV(luma []byte, lumaStride int32, u []byte, uStride int32, v []byte, vStride int32) (uintptr, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}

	lumaPtr, lumaSize := ptrAndSize(luma)
	uPtr, uSize := ptrAndSize(u)
	vPtr, vSize := ptrAndSize(v)

	idec := lowlevel.WebPINewYUV(lumaPtr, lumaSize, lumaStride, uPtr, uSize, uStride, vPtr, vSize, vStride)
	if idec == 0 {
		return 0, ErrDecodeFailed
	}

	return idec, nil
}

// WebPINewYUVA creates an incremental decoder producing YUVA planes.
func WebPINewYUVA(luma []byte, lumaStride int32, u []byte, uStride int32, v []byte, vStride int32, a []byte, aStride int32) (uintptr, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}

	lumaPtr, lumaSize := ptrAndSize(luma)
	uPtr, uSize := ptrAndSize(u)
	vPtr, vSize := ptrAndSize(v)
	aPtr, aSize := ptrAndSize(a)

	idec := lowlevel.WebPINewYUVA(lumaPtr, lumaSize, lumaStride, uPtr, uSize, uStride, vPtr, vSize, vStride, aPtr, aSize, aStride)
	if idec == 0 {
		return 0, ErrDecodeFailed
	}

	return idec, nil
}

// WebPIDelete destroys an incremental decoder instance.
func WebPIDelete(idec uintptr) error {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return err
	}
	if idec == 0 {
		return nil
	}

	lowlevel.WebPIDelete(idec)
	return nil
}

// WebPIAppend appends data to an incremental decoder.
func WebPIAppend(idec uintptr, data []byte) (VP8StatusCode, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}
	if idec == 0 || len(data) == 0 {
		return VP8StatusInvalidParam, ErrInvalidData
	}

	return VP8StatusCode(lowlevel.WebPIAppend(idec, &data[0], uintptr(len(data)))), nil
}

// WebPIUpdate updates an incremental decoder with a full prefix buffer.
func WebPIUpdate(idec uintptr, data []byte) (VP8StatusCode, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}
	if idec == 0 || len(data) == 0 {
		return VP8StatusInvalidParam, ErrInvalidData
	}

	return VP8StatusCode(lowlevel.WebPIUpdate(idec, &data[0], uintptr(len(data)))), nil
}

// WebPIDecode creates an incremental decoder with optional config and input.
func WebPIDecode(data []byte, config *DecoderConfig) (uintptr, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, ErrInvalidData
	}

	idec := lowlevel.WebPIDecode(&data[0], uintptr(len(data)), config)
	if idec == 0 {
		return 0, ErrDecodeFailed
	}

	return idec, nil
}

// WebPIDecodedArea returns decoded displayable area metadata.
func WebPIDecodedArea(idec uintptr, left, top, width, height *int32) (*DecBuffer, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return nil, err
	}
	if idec == 0 {
		return nil, ErrInvalidData
	}

	buf := lowlevel.WebPIDecodedArea(idec, left, top, width, height)
	if buf == nil {
		return nil, ErrDecodeFailed
	}

	return buf, nil
}

// WebPIDecGetRGB returns current incremental packed RGB-family output pointer.
func WebPIDecGetRGB(idec uintptr, lastY, width, height, stride *int32) (uintptr, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}
	if idec == 0 {
		return 0, ErrInvalidData
	}

	ptr := lowlevel.WebPIDecGetRGB(idec, lastY, width, height, stride)
	if ptr == nil {
		return 0, ErrDecodeFailed
	}

	return uintptr(unsafe.Pointer(ptr)), nil
}

// WebPIDecGetYUVA returns current incremental YUVA luma pointer.
func WebPIDecGetYUVA(idec uintptr, lastY *int32, u, v, a **byte, width, height, stride, uvStride, aStride *int32) (uintptr, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}
	if idec == 0 {
		return 0, ErrInvalidData
	}

	ptr := lowlevel.WebPIDecGetYUVA(idec, lastY, u, v, a, width, height, stride, uvStride, aStride)
	if ptr == nil {
		return 0, ErrDecodeFailed
	}

	return uintptr(unsafe.Pointer(ptr)), nil
}

// WebPIDecGetYUV is a compatibility wrapper for YUV output without alpha.
func WebPIDecGetYUV(idec uintptr, lastY *int32, u, v **byte, width, height, stride, uvStride *int32) (uintptr, error) {
	var a *byte
	return WebPIDecGetYUVA(idec, lastY, u, v, &a, width, height, stride, uvStride, nil)
}

// WebPDecode performs advanced one-shot decode using DecoderConfig.
func WebPDecode(data []byte, config *DecoderConfig) (status VP8StatusCode, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return VP8StatusNotEnoughData, nil
	}
	if config == nil {
		return VP8StatusInvalidParam, ErrInvalidData
	}

	return VP8StatusCode(lowlevel.WebPDecode(&data[0], uintptr(len(data)), config)), nil
}

// WebPDecodeRGBA decodes to packed RGBA and returns an owned Go buffer.
func WebPDecodeRGBA(data []byte) (pix []byte, width, height, stride int, err error) {
	return decodeToOwnedBuffer(data, 4, lowlevel.WebPDecodeRGBA)
}

// WebPDecodeARGB decodes to packed ARGB and returns an owned Go buffer.
func WebPDecodeARGB(data []byte) (pix []byte, width, height, stride int, err error) {
	return decodeToOwnedBuffer(data, 4, lowlevel.WebPDecodeARGB)
}

// WebPDecodeBGRA decodes to packed BGRA and returns an owned Go buffer.
func WebPDecodeBGRA(data []byte) (pix []byte, width, height, stride int, err error) {
	return decodeToOwnedBuffer(data, 4, lowlevel.WebPDecodeBGRA)
}

// WebPDecodeRGB decodes to packed RGB and returns an owned Go buffer.
func WebPDecodeRGB(data []byte) (pix []byte, width, height, stride int, err error) {
	return decodeToOwnedBuffer(data, 3, lowlevel.WebPDecodeRGB)
}

// WebPDecodeBGR decodes to packed BGR and returns an owned Go buffer.
func WebPDecodeBGR(data []byte) (pix []byte, width, height, stride int, err error) {
	return decodeToOwnedBuffer(data, 3, lowlevel.WebPDecodeBGR)
}

// WebPDecodeRGBAInto decodes into a caller-provided RGBA buffer.
func WebPDecodeRGBAInto(data []byte, outputBuffer []byte, outputStride int) (width, height int, err error) {
	return decodeInto(data, outputBuffer, outputStride, 4, lowlevel.WebPDecodeRGBAInto)
}

// WebPDecodeRGBAIntoWithInfo decodes into a caller-provided RGBA buffer using
// dimensions already validated by WebPGetInfo, avoiding a second parse.
func WebPDecodeRGBAIntoWithInfo(data []byte, outputBuffer []byte, outputStride, width, height int) error {
	return decodeIntoWithInfo(data, outputBuffer, outputStride, width, height, 4, lowlevel.WebPDecodeRGBAInto)
}

// WebPDecodeARGBInto decodes into a caller-provided ARGB buffer.
func WebPDecodeARGBInto(data []byte, outputBuffer []byte, outputStride int) (width, height int, err error) {
	return decodeInto(data, outputBuffer, outputStride, 4, lowlevel.WebPDecodeARGBInto)
}

// WebPDecodeBGRAInto decodes into a caller-provided BGRA buffer.
func WebPDecodeBGRAInto(data []byte, outputBuffer []byte, outputStride int) (width, height int, err error) {
	return decodeInto(data, outputBuffer, outputStride, 4, lowlevel.WebPDecodeBGRAInto)
}

// WebPDecodeRGBInto decodes into a caller-provided RGB buffer.
func WebPDecodeRGBInto(data []byte, outputBuffer []byte, outputStride int) (width, height int, err error) {
	return decodeInto(data, outputBuffer, outputStride, 3, lowlevel.WebPDecodeRGBInto)
}

// WebPDecodeBGRInto decodes into a caller-provided BGR buffer.
func WebPDecodeBGRInto(data []byte, outputBuffer []byte, outputStride int) (width, height int, err error) {
	return decodeInto(data, outputBuffer, outputStride, 3, lowlevel.WebPDecodeBGRInto)
}

// WebPDecodeYUV decodes to planar YUV and returns owned Go buffers.
func WebPDecodeYUV(data []byte) (y, u, v []byte, width, height, yStride, uvStride int, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return nil, nil, nil, 0, 0, 0, 0, err
	}
	if len(data) == 0 {
		return nil, nil, nil, 0, 0, 0, 0, ErrInvalidData
	}

	var w, h int32
	var uPtr, vPtr *byte
	var ys, uvs int32
	yPtr := lowlevel.WebPDecodeYUV(&data[0], uintptr(len(data)), &w, &h, &uPtr, &vPtr, &ys, &uvs)
	if yPtr == nil {
		return nil, nil, nil, 0, 0, 0, 0, ErrDecodeFailed
	}
	defer lowlevel.WebPFree(uintptr(unsafe.Pointer(yPtr)))

	width = int(w)
	height = int(h)
	if width <= 0 || height <= 0 {
		return nil, nil, nil, 0, 0, 0, 0, ErrInvalidDimension
	}

	yStride = int(ys)
	uvStride = int(uvs)
	uvHeight := (height + 1) / 2

	y = make([]byte, yStride*height)
	u = make([]byte, uvStride*uvHeight)
	v = make([]byte, uvStride*uvHeight)

	copy(y, unsafe.Slice(yPtr, len(y)))
	copy(u, unsafe.Slice(uPtr, len(u)))
	copy(v, unsafe.Slice(vPtr, len(v)))

	return y, u, v, width, height, yStride, uvStride, nil
}

// WebPDecodeYUVInto decodes into caller-provided Y, U and V planes.
func WebPDecodeYUVInto(data []byte, luma []byte, lumaStride int, u []byte, uStride int, v []byte, vStride int) (width, height int, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, 0, err
	}
	if len(data) == 0 {
		return 0, 0, ErrInvalidData
	}
	if len(luma) == 0 || len(u) == 0 || len(v) == 0 {
		return 0, 0, ErrBufferTooSmall
	}

	w, h, ok, err := WebPGetInfo(data)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, ErrInvalidData
	}
	if lumaStride < w {
		return 0, 0, ErrInvalidStride
	}
	uvWidth := (w + 1) / 2
	uvHeight := (h + 1) / 2
	if uStride < uvWidth || vStride < uvWidth {
		return 0, 0, ErrInvalidStride
	}
	if len(luma) < lumaStride*h || len(u) < uStride*uvHeight || len(v) < vStride*uvHeight {
		return 0, 0, ErrBufferTooSmall
	}

	ptr := lowlevel.WebPDecodeYUVInto(
		&data[0],
		uintptr(len(data)),
		&luma[0],
		uintptr(len(luma)),
		int32(lumaStride),
		&u[0],
		uintptr(len(u)),
		int32(uStride),
		&v[0],
		uintptr(len(v)),
		int32(vStride),
	)
	if ptr == nil {
		return 0, 0, ErrDecodeFailed
	}

	return w, h, nil
}

// WebPEncodeRGBA encodes packed RGBA pixels with lossy quality.
func WebPEncodeRGBA(rgba []byte, width, height, stride int, quality float32) ([]byte, error) {
	return encodeWithQuality(rgba, width, height, stride, 4, quality, lowlevel.WebPEncodeRGBA)
}

// WebPEncodeBGRA encodes packed BGRA pixels with lossy quality.
func WebPEncodeBGRA(bgra []byte, width, height, stride int, quality float32) ([]byte, error) {
	return encodeWithQuality(bgra, width, height, stride, 4, quality, lowlevel.WebPEncodeBGRA)
}

// WebPEncodeRGB encodes packed RGB pixels with lossy quality.
func WebPEncodeRGB(rgb []byte, width, height, stride int, quality float32) ([]byte, error) {
	return encodeWithQuality(rgb, width, height, stride, 3, quality, lowlevel.WebPEncodeRGB)
}

// WebPEncodeBGR encodes packed BGR pixels with lossy quality.
func WebPEncodeBGR(bgr []byte, width, height, stride int, quality float32) ([]byte, error) {
	return encodeWithQuality(bgr, width, height, stride, 3, quality, lowlevel.WebPEncodeBGR)
}

// WebPEncodeLosslessRGBA encodes packed RGBA pixels in lossless mode.
func WebPEncodeLosslessRGBA(rgba []byte, width, height, stride int) ([]byte, error) {
	return encodeLossless(rgba, width, height, stride, 4, lowlevel.WebPEncodeLosslessRGBA)
}

// WebPEncodeLosslessBGRA encodes packed BGRA pixels in lossless mode.
func WebPEncodeLosslessBGRA(bgra []byte, width, height, stride int) ([]byte, error) {
	return encodeLossless(bgra, width, height, stride, 4, lowlevel.WebPEncodeLosslessBGRA)
}

// WebPEncodeLosslessRGB encodes packed RGB pixels in lossless mode.
func WebPEncodeLosslessRGB(rgb []byte, width, height, stride int) ([]byte, error) {
	return encodeLossless(rgb, width, height, stride, 3, lowlevel.WebPEncodeLosslessRGB)
}

// WebPEncodeLosslessBGR encodes packed BGR pixels in lossless mode.
func WebPEncodeLosslessBGR(bgr []byte, width, height, stride int) ([]byte, error) {
	return encodeLossless(bgr, width, height, stride, 3, lowlevel.WebPEncodeLosslessBGR)
}

type decodeFunc func(data *byte, dataSize uintptr, width *int32, height *int32) *byte

type decodeIntoFunc func(data *byte, dataSize uintptr, outputBuffer *byte, outputBufferSize uintptr, outputStride int32) *byte

func decodeInto(data []byte, outputBuffer []byte, outputStride int, bytesPerPixel int, fn decodeIntoFunc) (width, height int, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return 0, 0, err
	}
	if len(data) == 0 {
		return 0, 0, ErrInvalidData
	}
	if len(outputBuffer) == 0 {
		return 0, 0, ErrBufferTooSmall
	}
	w, h, ok, err := WebPGetInfo(data)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, ErrInvalidData
	}
	if err := decodeIntoWithInfo(data, outputBuffer, outputStride, w, h, bytesPerPixel, fn); err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

func decodeIntoWithInfo(data []byte, outputBuffer []byte, outputStride, width, height, bytesPerPixel int, fn decodeIntoFunc) error {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return err
	}
	if len(data) == 0 {
		return ErrInvalidData
	}
	if len(outputBuffer) == 0 {
		return ErrBufferTooSmall
	}
	minimumStride, _, err := checkedDecodeLayout(width, height, bytesPerPixel)
	if err != nil {
		return err
	}
	if outputStride < minimumStride || outputStride > math.MaxInt32 {
		return ErrInvalidStride
	}
	required, ok := checkedProduct(outputStride, height)
	if !ok || len(outputBuffer) < required {
		return ErrBufferTooSmall
	}
	if fn(&data[0], uintptr(len(data)), &outputBuffer[0], uintptr(len(outputBuffer)), int32(outputStride)) == nil {
		return ErrDecodeFailed
	}
	return nil
}

func decodeToOwnedBuffer(data []byte, bytesPerPixel int, fn decodeFunc) (pix []byte, width, height, stride int, err error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return nil, 0, 0, 0, err
	}
	if len(data) == 0 {
		return nil, 0, 0, 0, ErrInvalidData
	}

	var w, h int32
	ptr := fn(&data[0], uintptr(len(data)), &w, &h)
	if ptr == nil {
		return nil, 0, 0, 0, ErrDecodeFailed
	}
	defer lowlevel.WebPFree(uintptr(unsafe.Pointer(ptr)))

	width = int(w)
	height = int(h)
	stride, bufLen, err := checkedDecodeLayout(width, height, bytesPerPixel)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	pix = make([]byte, bufLen)
	copy(pix, unsafe.Slice(ptr, bufLen))

	return pix, width, height, stride, nil
}

type encodeLossyFunc func(pix *byte, width int32, height int32, stride int32, quality float32, output **byte) uintptr

func encodeWithQuality(pix []byte, width, height, stride, bytesPerPixel int, quality float32, fn encodeLossyFunc) ([]byte, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return nil, err
	}
	if err := validatePixelInput(pix, width, height, stride, bytesPerPixel); err != nil {
		return nil, err
	}

	var out *byte
	size := fn(&pix[0], int32(width), int32(height), int32(stride), quality, &out)
	if size == 0 || out == nil {
		return nil, ErrEncodeFailed
	}
	defer lowlevel.WebPFree(uintptr(unsafe.Pointer(out)))

	b := make([]byte, int(size))
	copy(b, unsafe.Slice(out, int(size)))

	return b, nil
}

type encodeLosslessFunc func(pix *byte, width int32, height int32, stride int32, output **byte) uintptr

func encodeLossless(pix []byte, width, height, stride, bytesPerPixel int, fn encodeLosslessFunc) ([]byte, error) {
	if err := lowlevel.EnsureLoaded(); err != nil {
		return nil, err
	}
	if err := validatePixelInput(pix, width, height, stride, bytesPerPixel); err != nil {
		return nil, err
	}

	var out *byte
	size := fn(&pix[0], int32(width), int32(height), int32(stride), &out)
	if size == 0 || out == nil {
		return nil, ErrEncodeFailed
	}
	defer lowlevel.WebPFree(uintptr(unsafe.Pointer(out)))

	b := make([]byte, int(size))
	copy(b, unsafe.Slice(out, int(size)))

	return b, nil
}

func validatePixelInput(pix []byte, width, height, stride, bytesPerPixel int) error {
	minimumStride, _, err := checkedDecodeLayout(width, height, bytesPerPixel)
	if err != nil {
		return err
	}
	if stride < minimumStride || stride > math.MaxInt32 {
		return ErrInvalidStride
	}
	required, ok := checkedProduct(stride, height)
	if !ok {
		return ErrInvalidDimension
	}
	if len(pix) < required {
		return fmt.Errorf("libwebp: pixel buffer too small: got=%d need>=%d", len(pix), required)
	}
	return nil
}

// checkedDecodeLayout validates the dimensions and packed output layout before
// values are passed to libwebp, whose output stride parameter is a C int.
func checkedDecodeLayout(width, height, bytesPerPixel int) (stride, size int, err error) {
	if width <= 0 || height <= 0 {
		return 0, 0, ErrInvalidDimension
	}
	if bytesPerPixel <= 0 || width > math.MaxInt32/bytesPerPixel {
		return 0, 0, ErrInvalidStride
	}
	stride = width * bytesPerPixel
	size, ok := checkedProduct(stride, height)
	if !ok {
		return 0, 0, ErrInvalidDimension
	}
	return stride, size, nil
}

func checkedProduct(a, b int) (int, bool) {
	if a < 0 || b < 0 || (a != 0 && b > int(^uint(0)>>1)/a) {
		return 0, false
	}
	return a * b, true
}

// WebPIsPremultipliedMode reports whether the decode colorspace is premultiplied.
func WebPIsPremultipliedMode(mode int) bool {
	return mode == ModergbA || mode == ModebgrA || mode == ModeArgb || mode == ModergbA4444
}

// WebPIsAlphaMode reports whether the decode colorspace contains alpha.
func WebPIsAlphaMode(mode int) bool {
	return mode == ModeRGBA || mode == ModeBGRA || mode == ModeARGB || mode == ModeRGBA4444 || mode == ModeYUVA || WebPIsPremultipliedMode(mode)
}

// WebPIsRGBMode reports whether the decode colorspace is RGB-family.
func WebPIsRGBMode(mode int) bool {
	return mode < ModeYUV
}

func ptrAndSize(b []byte) (*byte, uintptr) {
	if len(b) == 0 {
		return nil, 0
	}
	return &b[0], uintptr(len(b))
}

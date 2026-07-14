package capi

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	portout "github.com/bnema/purego-pipewire/internal/ports/out"
)

// SPA POD type identifiers (from spa/utils/type.h).
const (
	spaTypeNone      uint32 = iota + 1 // 1; reserved so iota matches SPA type numbering
	spaTypeBool                        // 2
	spaTypeId                          // 3
	spaTypeInt                         // 4
	spaTypeLong                        // 5
	spaTypeFloat                       // 6
	spaTypeDouble                      // 7
	spaTypeString                      // 8
	spaTypeBytes                       // 9
	spaTypeRectangle                   // 10
	spaTypeFraction                    // 11
	spaTypeBitmap                      // 12
	spaTypeArray                       // 13
	spaTypeStruct                      // 14
	spaTypeObject                      // 15
)

// SPA object type identifiers (from spa/utils/type.h).
const (
	spaTypeObjectFormat uint32 = 0x40003 // SPA_TYPE_OBJECT_Format
)

// SPA param type identifiers (from spa/param/param.h).
const (
	spaParamEnumFormat uint32 = 3 // SPA_PARAM_EnumFormat
)

// SPA format property keys (from spa/param/format.h).
const (
	spaFormatMediaSubtype uint32 = 2 // SPA_FORMAT_mediaSubtype
	spaFormatMediaType    uint32 = 1 // SPA_FORMAT_mediaType

	// Audio format keys (SPA_FORMAT_START_Audio = 0x10000).
	spaFormatAudioFormat   uint32 = 0x10001 // SPA_FORMAT_AUDIO_format
	spaFormatAudioRate     uint32 = 0x10003 // SPA_FORMAT_AUDIO_rate
	spaFormatAudioChannels uint32 = 0x10004 // SPA_FORMAT_AUDIO_channels
)

// SPA media type and subtype identifiers (from spa/param/format.h).
const (
	spaMediaTypeAudio  uint32 = 1 // SPA_MEDIA_TYPE_audio
	spaMediaSubtypeRaw uint32 = 1 // SPA_MEDIA_SUBTYPE_raw
)

// SPA audio format identifiers (from spa/param/audio/raw.h).
// Planar formats start at SPA_AUDIO_FORMAT_START_Planar = 0x200.
const (
	spaAudioFormatF32P uint32 = 0x206 // SPA_AUDIO_FORMAT_F32P
)

// buildRawAudioParams constructs one minimal SPA POD for SPA_PARAM_EnumFormat
// describing a raw audio stream with planar float32 sample format. The returned
// connectParams holds the storage buffer and the params slice suitable for
// passing to pw_stream_connect.
//
// This is a focused, handwritten builder for the first playable target format.
// It is intentionally minimal: no choice PODs, no format negotiation, no
// general-purpose SPA param generation.
func buildRawAudioParams(format portout.PlaybackFormat) (*connectParams, error) {
	if format.SampleRate <= 0 {
		return nil, fmt.Errorf("%w: sample rate must be positive, got %d",
			ErrInvalidPlaybackFormat, format.SampleRate)
	}
	if format.Channels <= 0 {
		return nil, fmt.Errorf("%w: channels must be positive, got %d",
			ErrInvalidPlaybackFormat, format.Channels)
	}
	if format.FramesPerBuffer <= 0 {
		return nil, fmt.Errorf("%w: frames per buffer must be positive, got %d",
			ErrInvalidPlaybackFormat, format.FramesPerBuffer)
	}

	// Layout of an SPA POD Object for EnumFormat with 5 properties:
	//
	//   spa_pod       header   (8 bytes): {size, type}
	//   object_body   body    (8 bytes): {objType, id}
	//   prop[0] mediaType     (24 bytes): {key, flags, {4,Id}, value, pad}
	//   prop[1] mediaSubtype  (24 bytes): {key, flags, {4,Id}, value, pad}
	//   prop[2] audioFormat   (24 bytes): {key, flags, {4,Id}, value, pad}
	//   prop[3] audioRate     (24 bytes): {key, flags, {4,Int}, value, pad}
	//   prop[4] audioChannels (24 bytes): {key, flags, {4,Int}, value, pad}
	//
	// Total: 8 + 8 + 5×24 = 136 bytes.
	// Pod body size (excl. header): 8 + 120 = 128 bytes.

	const propSize = 24                // key(4) + flags(4) + pod{4,4}(8) + value(4) + pad(4)
	const objBodySize = 8 + 5*propSize // 128
	const totalSize = 8 + objBodySize  // 136

	buf := make([]byte, totalSize)

	// POD header
	binary.LittleEndian.PutUint32(buf[0:4], objBodySize)   // size
	binary.LittleEndian.PutUint32(buf[4:8], spaTypeObject) // type

	// Object body
	binary.LittleEndian.PutUint32(buf[8:12], spaTypeObjectFormat) // objType
	binary.LittleEndian.PutUint32(buf[12:16], spaParamEnumFormat) // id

	// Properties
	off := 16
	off += writeIdProp(buf[off:], spaFormatMediaType, spaMediaTypeAudio)
	off += writeIdProp(buf[off:], spaFormatMediaSubtype, spaMediaSubtypeRaw)
	off += writeIdProp(buf[off:], spaFormatAudioFormat, spaAudioFormatF32P)
	off += writeIntProp(buf[off:], spaFormatAudioRate, int32(format.SampleRate))
	_ = writeIntProp(buf[off:], spaFormatAudioChannels, int32(format.Channels))

	cp := &connectParams{
		storage: buf,
		params:  []unsafe.Pointer{unsafe.Pointer(&buf[0])},
	}

	return cp, nil
}

// connectParams holds the backing storage and parameter slice for a
// pw_stream_connect call. The storage buffer contains the SPA POD data,
// and params points into that storage.
type connectParams struct {
	storage []byte
	params  []unsafe.Pointer
}

// Pointer returns a pointer to the params array, suitable for passing as
// the params argument to pw_stream_connect (which expects spa_pod **).
func (cp *connectParams) Pointer() unsafe.Pointer {
	if len(cp.params) == 0 {
		return nil
	}
	return unsafe.Pointer(&cp.params[0])
}

// Count returns the number of params in the params slice.
func (cp *connectParams) Count() uint32 {
	return uint32(len(cp.params))
}

// writeIdProp writes an SPA POD property with an Id value into buf.
// Layout: key(4) | flags(4) | pod{4,Id}(8) | value(4) | pad(4) = 24 bytes.
func writeIdProp(buf []byte, key uint32, value uint32) int {
	binary.LittleEndian.PutUint32(buf[0:4], key)
	binary.LittleEndian.PutUint32(buf[4:8], 0)  // flags
	binary.LittleEndian.PutUint32(buf[8:12], 4) // pod body size
	binary.LittleEndian.PutUint32(buf[12:16], spaTypeId)
	binary.LittleEndian.PutUint32(buf[16:20], value)
	binary.LittleEndian.PutUint32(buf[20:24], 0) // padding
	return 24
}

// writeIntProp writes an SPA POD property with an Int value into buf.
// Layout: key(4) | flags(4) | pod{4,Int}(8) | value(4) | pad(4) = 24 bytes.
func writeIntProp(buf []byte, key uint32, value int32) int {
	binary.LittleEndian.PutUint32(buf[0:4], key)
	binary.LittleEndian.PutUint32(buf[4:8], 0)  // flags
	binary.LittleEndian.PutUint32(buf[8:12], 4) // pod body size
	binary.LittleEndian.PutUint32(buf[12:16], spaTypeInt)
	binary.LittleEndian.PutUint32(buf[16:20], uint32(value))
	binary.LittleEndian.PutUint32(buf[20:24], 0) // padding
	return 24
}

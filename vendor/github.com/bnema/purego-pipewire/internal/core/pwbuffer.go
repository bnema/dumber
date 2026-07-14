package core

import (
	"errors"
	"fmt"
	"unsafe"
)

// PipeWire buffer layout structs matching the C ABI.
// These are low-level mappings that allow interpreting a dequeue'd buffer
// pointer as writable planar float32 data. They are intentionally minimal
// and local — not a general SPA buffer parser.

// spaChunk mirrors struct spa_chunk from <spa/buffer/buffer.h>.
type spaChunk struct {
	Offset uint32
	Size   uint32
	Stride int32
	Flags  int32
}

// spaData mirrors struct spa_data from <spa/buffer/buffer.h>.
type spaData struct {
	Type      uint32
	Flags     uint32
	Fd        int64
	MapOffset uint32
	Maxsize   uint32
	Data      unsafe.Pointer
	Chunk     *spaChunk
}

// spaBuffer mirrors struct spa_buffer from <spa/buffer/buffer.h>.
type spaBuffer struct {
	NMetas uint32
	NDatas uint32
	Metas  unsafe.Pointer // struct spa_meta * — pointer to array of spa_meta
	Datas  unsafe.Pointer // struct spa_data * — pointer to array of spa_data
}

// pwBuffer mirrors struct pw_buffer from <pipewire/stream.h>.
type pwBuffer struct {
	Buffer    *spaBuffer
	UserData  unsafe.Pointer
	Size      uint64
	Requested uint64
	Time      uint64
}

// pwBufferView provides a safe Go view over a dequeue'd PipeWire buffer,
// exposing planar float32 channel data. Call Commit after writing frames
// to update chunk size/stride and pw buffer size.
type pwBufferView struct {
	buf      *pwBuffer
	channels int
	frames   int
	data     []spaData
	pcm      PCMBuffer
	samples  [][]float32
}

var (
	ErrNilBufferPointer = errors.New("nil buffer pointer")
	ErrChannelMismatch  = errors.New("buffer data count does not match channels")
	ErrUnmappedData     = errors.New("buffer data pointer is nil")
	ErrFrameOverflow    = errors.New("requested frames exceed buffer capacity")
)

// newPWBufferView interprets bufPtr as a *pwBuffer and returns a view that
// exposes planar float32 samples for each channel. The caller must ensure
// bufPtr is a valid pw_buffer pointer returned by pw_stream_dequeue_buffer.
// The view holds a pointer directly into the original pwBuffer so that Commit
// writes back into the shared memory structure, not a Go copy.
func newPWBufferView(bufPtr unsafe.Pointer, channels, frames int) (*pwBufferView, error) {
	if bufPtr == nil {
		return nil, ErrNilBufferPointer
	}

	pw := (*pwBuffer)(bufPtr)
	if pw.Buffer == nil {
		return nil, ErrNilBufferPointer
	}

	if pw.Buffer.NDatas != uint32(channels) {
		return nil, ErrChannelMismatch
	}

	// Datas is a pointer to a C array of spaData structs.
	dataSlice := unsafe.Slice((*spaData)(pw.Buffer.Datas), int(pw.Buffer.NDatas))

	samples := make([][]float32, channels)
	for i := 0; i < channels; i++ {
		data := dataSlice[i]
		if data.Data == nil {
			return nil, ErrUnmappedData
		}
		// Reject if the buffer cannot hold the requested number of frames.
		maxFrames := int(data.Maxsize) / 4
		if frames > maxFrames {
			return nil, fmt.Errorf("%w: requested %d frames but buffer holds %d", ErrFrameOverflow, frames, maxFrames)
		}
		samples[i] = unsafe.Slice((*float32)(data.Data), frames)
	}

	return &pwBufferView{
		buf:      pw,
		channels: channels,
		frames:   frames,
		data:     dataSlice,
		pcm: PCMBuffer{
			Frames:   frames,
			Channels: channels,
			Stride:   4,
			Samples:  samples,
		},
		samples: samples,
	}, nil
}

// PCM returns a PCMBuffer pointing at the buffer's planar float32 data.
// The caller can write into Samples and then call Commit.
// Stride is set to 4 (bytes per float32 sample) for planar float32 layout.
func (v *pwBufferView) PCM() *PCMBuffer {
	return &v.pcm
}

// Commit updates each data chunk's size and stride to reflect the number of
// frames written, and sets the pw buffer size field. Call this after writing
// audio data into the PCM buffer.
func (v *pwBufferView) Commit(frames int) {
	v.buf.Size = uint64(frames)
	for i := 0; i < v.channels; i++ {
		data := &v.data[i]
		if data.Chunk != nil {
			data.Chunk.Size = uint32(frames * 4) // float32 = 4 bytes
			data.Chunk.Stride = 4                // stride for planar float32
		}
	}
}

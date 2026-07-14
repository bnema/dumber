// internal/core/helpers.go
package core

import "unsafe"

// DecodeAudioPacket converts a CEF audio data pointer (float** layout) into
// a Go [][]float32 slice indexed by [channel][frame].
func DecodeAudioPacket(data unsafe.Pointer, channels, frames int32) [][]float32 {
	if data == nil || channels <= 0 || frames <= 0 {
		return nil
	}
	channelPtrs := unsafe.Slice((**float32)(data), int(channels))
	result := make([][]float32, int(channels))
	for i, cp := range channelPtrs {
		if cp != nil {
			result[i] = unsafe.Slice(cp, int(frames))
		}
	}
	return result
}

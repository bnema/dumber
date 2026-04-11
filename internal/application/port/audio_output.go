// Package port defines application-layer interfaces for external capabilities.
// This file defines the Audio Output port for CEF audio streaming on Linux.
package port

import "context"

// AudioStreamFormat represents the audio stream format configuration.
// This is a value object that describes how audio data should be streamed.
type AudioStreamFormat struct {
	// SampleRate is the audio sample rate in Hz (e.g., 48000, 44100).
	SampleRate int
	// ChannelCount is the number of audio channels (e.g., 1 for mono, 2 for stereo).
	ChannelCount int
	// FramesPerBuffer is the number of frames per buffer (e.g., 512, 1024).
	FramesPerBuffer int
}

// AudioOutputStream represents an active audio output stream.
// Implementations handle the low-level audio playback.
type AudioOutputStream interface {
	// Write sends audio samples to the output device.
	// Samples are provided as a 2D slice: [channel][frame]float32.
	// This matches CEF's planar float32 audio format.
	Write(samples [][]float32) error
	// Close releases the audio stream and associated resources.
	Close() error
}

// AudioOutputFactory creates audio output streams.
// This is the primary interface for the audio output port.
type AudioOutputFactory interface {
	// NewStream creates a new audio output stream with the given format.
	NewStream(ctx context.Context, format AudioStreamFormat) (AudioOutputStream, error)
}

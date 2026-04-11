// Package null provides a null audio backend that discards all audio data.
// This is useful as a fallback when no real audio backend is available.
package null

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
)

// Factory creates null audio output streams that discard all audio data.
type Factory struct{}

// NewStream creates a new null audio output stream.
// The stream accepts writes but discards all data silently.
func (*Factory) NewStream(_ context.Context, _ port.AudioStreamFormat) (port.AudioOutputStream, error) {
	return &Stream{}, nil
}

// Stream is a null audio output stream that discards all audio data.
type Stream struct{}

// Write accepts audio samples but discards them silently.
// This method never returns an error.
func (*Stream) Write(_ [][]float32) error {
	return nil
}

// Close releases the audio stream.
// This method is idempotent and never returns an error.
func (*Stream) Close() error {
	return nil
}

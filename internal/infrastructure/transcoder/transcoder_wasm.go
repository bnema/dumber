//go:build js && wasm

package transcoder

import (
	"context"
	"errors"

	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/application/port"
)

// Compile-time check: Transcoder implements port.MediaTranscoder.
var _ port.MediaTranscoder = (*Transcoder)(nil)

// Transcoder is unavailable in js/wasm builds because FFmpeg is not available.
type Transcoder struct{}

// New creates an unavailable Transcoder for js/wasm builds.
func New(_ any, _ *zerolog.Logger) *Transcoder {
	return &Transcoder{}
}

// Available reports whether transcoding is available.
func (*Transcoder) Available() bool {
	return false
}

// Capabilities returns empty hardware capabilities for js/wasm builds.
func (*Transcoder) Capabilities() port.HWCapabilities {
	return port.HWCapabilities{}
}

// Start always fails for js/wasm builds.
func (*Transcoder) Start(_ context.Context, _ string, _ map[string]string) (port.TranscodeSession, error) {
	return nil, errors.New("transcoder: FFmpeg unavailable in js/wasm")
}

// Close is a no-op for js/wasm builds.
func (*Transcoder) Close() {}

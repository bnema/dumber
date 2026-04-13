// Package pipewire provides a PipeWire-based audio output implementation.
// This wraps purego-pipewire's high-level Player API.
package pipewire

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	pwpipewire "github.com/bnema/purego-pipewire/pipewire"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface checks.
var (
	_ port.AudioOutputFactory = (*Factory)(nil)
	_ port.AudioOutputStream  = (*playerStreamAdapter)(nil)
)

// ErrInvalidFormat is returned when the audio stream format is invalid.
var ErrInvalidFormat = errors.New("invalid audio stream format")

// ErrStreamClosed is returned when writing to a closed stream.
var ErrStreamClosed = errors.New("audio stream closed")

// playerCreator is a function type that creates a Player from config and callbacks.
// This exists to allow dependency injection in tests.
type playerCreator func(config pwpipewire.PlayerConfig, callbacks pwpipewire.PlayerCallbacks) (pwpipewire.Player, error)

// Factory creates PipeWire audio output streams.
// Each call to NewStream creates an independent purego-pipewire Player.
type Factory struct {
	createPlayer playerCreator
}

// NewFactory creates a new PipeWire audio factory.
// The factory is stateless; players are created per-stream.
func NewFactory() (*Factory, error) {
	return &Factory{
		createPlayer: pwpipewire.NewPlayer,
	}, nil
}

// NewStream creates a new audio output stream with the given format.
// It validates the Dumber format, creates a purego-pipewire Player configured
// from the format, and returns a push-to-pull adapter that bridges Write()
// calls into the Player's Fill callback.
func (f *Factory) NewStream(ctx context.Context, format port.AudioStreamFormat) (port.AudioOutputStream, error) {
	// Check context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate format
	if err := validateFormat(format); err != nil {
		return nil, err
	}

	log := logging.FromContext(ctx)
	log.Info().
		Int("sample_rate", format.SampleRate).
		Int("channels", format.ChannelCount).
		Int("frames_per_buffer", format.FramesPerBuffer).
		Msg("pipewire: creating player")

	// Create the adapter with its ring buffer before creating the player,
	// because the Fill callback captures the adapter.
	adapter := &playerStreamAdapter{
		ctx:    ctx,
		buf:    make(chan [][]float32, 4), // small buffer for push→pull bridging
		closed: make(chan struct{}),
	}

	config := pwpipewire.PlayerConfig{
		SampleRate:      format.SampleRate,
		Channels:        format.ChannelCount,
		FramesPerBuffer: format.FramesPerBuffer,
		SampleFormat:    pwpipewire.SampleFormatF32,
		UnderrunPolicy:  pwpipewire.UnderrunFillSilence,
	}

	callbacks := pwpipewire.PlayerCallbacks{
		Fill: adapter.fill,
	}

	player, err := f.createPlayer(config, callbacks)
	if err != nil {
		log.Warn().Err(err).Msg("pipewire: failed to create player")
		return nil, fmt.Errorf("failed to create pipewire player: %w", err)
	}
	adapter.player = player

	// Start the player so the Fill callback begins being called.
	if err := player.Start(); err != nil {
		_ = player.Close()
		log.Warn().Err(err).Msg("pipewire: failed to start player")
		return nil, fmt.Errorf("failed to start pipewire player: %w", err)
	}

	log.Info().Msg("pipewire: player started successfully")

	return adapter, nil
}

// validateFormat checks if the audio format parameters are valid.
func validateFormat(format port.AudioStreamFormat) error {
	// Sample rate must be reasonable (8kHz to 384kHz)
	if format.SampleRate < 8000 || format.SampleRate > 384000 {
		return fmt.Errorf("%w: sample rate %d Hz out of range [8000, 384000]",
			ErrInvalidFormat, format.SampleRate)
	}

	// Channel count must be 1 to 1024 (reasonable upper limit)
	if format.ChannelCount < 1 || format.ChannelCount > 1024 {
		return fmt.Errorf("%w: channel count %d out of range [1, 1024]",
			ErrInvalidFormat, format.ChannelCount)
	}

	// Frames per buffer must be positive and reasonable
	if format.FramesPerBuffer < 1 || format.FramesPerBuffer > 8192 {
		return fmt.Errorf("%w: frames per buffer %d out of range [1, 8192]",
			ErrInvalidFormat, format.FramesPerBuffer)
	}

	return nil
}

// playerStreamAdapter bridges Dumber's push-style Write(samples) interface
// to purego-pipewire's pull-style Fill callback.
//
// Write() sends sample buffers into a channel. The Fill callback drains them
// into the PCMBuffer that purego-pipewire passes in. If no data is available,
// the player's UnderrunFillSilence policy handles it.
type playerStreamAdapter struct {
	player pwpipewire.Player
	ctx    context.Context
	buf    chan [][]float32 // push→pull handoff channel

	closeOnce sync.Once
	closed    chan struct{} // closed when Close is called

	// Diagnostic counters (atomic, no mutex needed).
	fillCount     atomic.Uint64 // total Fill callbacks
	underrunCount atomic.Uint64 // Fill callbacks with no data available
	dropCount     atomic.Uint64 // Write calls that dropped data (buffer full)
}

// Write sends audio samples to the PipeWire output.
// Samples are provided as [channel][frame]float32 matching CEF's planar format.
// If the internal buffer is full the packet is dropped silently so the caller
// (CEF audio callback thread) is never blocked.
// Safe for concurrent use with Close.
func (a *playerStreamAdapter) Write(samples [][]float32) error {
	// Fast path: already closed.
	select {
	case <-a.closed:
		return ErrStreamClosed
	default:
	}
	// Non-blocking send: drop packet when buffer is full.
	select {
	case a.buf <- samples:
		return nil
	case <-a.closed:
		return ErrStreamClosed
	default:
		// Buffer full — drop to avoid blocking the CEF audio thread.
		n := a.dropCount.Add(1)
		if n == 1 || n&(n-1) == 0 { // first drop, then powers of two
			if a.ctx != nil {
				logging.FromContext(a.ctx).Warn().
					Uint64("total_drops", n).
					Msg("pipewire: dropped audio packet (buffer full)")
			}
		}
		return nil
	}
}

// fill is the PlayerCallbacks.Fill function called by purego-pipewire
// when it needs audio data. It pulls from the internal buffer channel.
func (a *playerStreamAdapter) fill(pcm *pwpipewire.PCMBuffer) (int, error) {
	select {
	case samples := <-a.buf:
		fillNum := a.fillCount.Add(1)
		if fillNum == 1 && a.ctx != nil {
			logging.FromContext(a.ctx).Info().
				Msg("pipewire: first Fill callback with data")
		}
		// Copy available samples into the PCM buffer
		copied := copyToPCM(pcm, samples)
		return copied, nil
	default:
		// No data available — return 0 frames, let underrun policy handle it
		underruns := a.underrunCount.Add(1)
		if underruns == 1 && a.ctx != nil {
			logging.FromContext(a.ctx).Debug().
				Msg("pipewire: first Fill underrun (no data available)")
		}
		return 0, nil
	}
}

// copyToPCM copies planar samples into the PCMBuffer.
// Returns the number of frames actually copied.
func copyToPCM(pcm *pwpipewire.PCMBuffer, samples [][]float32) int {
	if pcm == nil || len(samples) == 0 {
		return 0
	}

	frames := pcm.Frames
	channels := len(samples)
	if channels > pcm.Channels {
		channels = pcm.Channels
	}
	if channels > len(pcm.Samples) {
		channels = len(pcm.Samples)
	}
	if channels == 0 {
		return 0
	}

	minFrames := frames

	for ch := 0; ch < channels && ch < len(samples); ch++ {
		dst := pcm.Samples[ch]
		src := samples[ch]
		if len(dst) == 0 || len(src) == 0 {
			minFrames = 0
			continue
		}
		srcFrames := len(src)
		n := frames
		if srcFrames < n {
			n = srcFrames
		}
		if len(dst) < n {
			n = len(dst)
		}
		if n < minFrames {
			minFrames = n
		}
		copy(dst[:n], src[:n])
	}

	return minFrames
}

// Close stops the player and releases resources.
// Safe to call multiple times.
func (a *playerStreamAdapter) Close() error {
	var err error
	a.closeOnce.Do(func() {
		if a.ctx != nil {
			logging.FromContext(a.ctx).Info().
				Uint64("fills", a.fillCount.Load()).
				Uint64("underruns", a.underrunCount.Load()).
				Uint64("drops", a.dropCount.Load()).
				Msg("pipewire: closing player")
		}

		// Signal writers to stop
		close(a.closed)

		if a.player != nil {
			// Stop playback then close the player
			if stopErr := a.player.Stop(); stopErr != nil {
				err = stopErr
			}
			if closeErr := a.player.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
	})
	return err
}

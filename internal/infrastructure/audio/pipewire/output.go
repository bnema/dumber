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

var (
	_ port.AudioOutputFactory = (*Factory)(nil)
	_ port.AudioOutputStream  = (*playerStreamAdapter)(nil)
)

// ErrInvalidFormat is returned when the audio stream format is invalid.
var ErrInvalidFormat = errors.New("invalid audio stream format")

// ErrStreamClosed is returned when writing to a closed stream.
var ErrStreamClosed = errors.New("audio stream closed")

const packetQueueCapacity = 4

// playerCreator is a function type that creates a Player from config and callbacks.
// This exists to allow dependency injection in tests.
type playerCreator func(config pwpipewire.PlayerConfig, callbacks pwpipewire.PlayerCallbacks) (pwpipewire.Player, error)

// Factory creates PipeWire audio output streams.
// Each call to NewStream creates an independent purego-pipewire Player.
type Factory struct {
	createPlayer playerCreator
}

// NewFactory creates a new PipeWire audio factory.
func NewFactory() (*Factory, error) {
	return &Factory{createPlayer: pwpipewire.NewPlayer}, nil
}

// NewStream creates a push-to-pull adapter for a PipeWire player.
func (f *Factory) NewStream(ctx context.Context, format port.AudioStreamFormat) (port.AudioOutputStream, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := validateFormat(format); err != nil {
		return nil, err
	}

	log := logging.FromContext(ctx)
	log.Info().
		Int("sample_rate", format.SampleRate).
		Int("channels", format.ChannelCount).
		Int("frames_per_buffer", format.FramesPerBuffer).
		Msg("pipewire: creating player")

	// Every slot owns storage sized from the negotiated CEF format. Write copies
	// directly into an available slot; Fill copies that same slot into PipeWire.
	// Callback slices are therefore never queued or retained.
	adapter := newPlayerStreamAdapter(ctx, format.ChannelCount, format.FramesPerBuffer)
	config := pwpipewire.PlayerConfig{
		SampleRate: format.SampleRate, Channels: format.ChannelCount, FramesPerBuffer: format.FramesPerBuffer,
		SampleFormat: pwpipewire.SampleFormatF32, UnderrunPolicy: pwpipewire.UnderrunFillSilence,
	}
	player, err := f.createPlayer(config, pwpipewire.PlayerCallbacks{Fill: adapter.fill})
	if err != nil {
		log.Warn().Err(err).Msg("pipewire: failed to create player")
		return nil, fmt.Errorf("failed to create pipewire player: %w", err)
	}
	adapter.player = player
	if err := player.Start(); err != nil {
		_ = player.Close()
		log.Warn().Err(err).Msg("pipewire: failed to start player")
		return nil, fmt.Errorf("failed to start pipewire player: %w", err)
	}
	log.Info().Msg("pipewire: player started successfully")
	return adapter, nil
}

func validateFormat(format port.AudioStreamFormat) error {
	if format.SampleRate < 8000 || format.SampleRate > 384000 {
		return fmt.Errorf("%w: sample rate %d Hz out of range [8000, 384kHz]", ErrInvalidFormat, format.SampleRate)
	}
	if format.ChannelCount < 1 || format.ChannelCount > 1024 {
		return fmt.Errorf("%w: channel count %d out of range [1, 1024]", ErrInvalidFormat, format.ChannelCount)
	}
	if format.FramesPerBuffer < 1 || format.FramesPerBuffer > 8192 {
		return fmt.Errorf("%w: frames per buffer %d out of range [1, 8192]", ErrInvalidFormat, format.FramesPerBuffer)
	}
	return nil
}

// audioPacket is a reusable, adapter-owned packet slot. Its backing arrays are
// allocated only when the stream is created and never alias CEF callback data.
type audioPacket struct {
	samples  [][]float32
	channels int
	frames   int
}

// playerStreamAdapter bridges callback-owned CEF audio to PipeWire's Fill
// callback. available and ready form a bounded reusable packet ring.
type playerStreamAdapter struct {
	player pwpipewire.Player
	ctx    context.Context

	available chan *audioPacket
	ready     chan *audioPacket
	// TryLock serializes only simultaneous producers. The CEF callback never
	// waits: contention and saturation both drop the packet. Fill never takes it.
	writeMu sync.Mutex
	closed  atomic.Bool

	closeOnce sync.Once

	fillCount     atomic.Uint64
	underrunCount atomic.Uint64
	dropCount     atomic.Uint64
}

func newPlayerStreamAdapter(ctx context.Context, channels, frames int) *playerStreamAdapter {
	a := &playerStreamAdapter{
		ctx: ctx, available: make(chan *audioPacket, packetQueueCapacity), ready: make(chan *audioPacket, packetQueueCapacity),
	}
	for range packetQueueCapacity {
		packet := &audioPacket{samples: make([][]float32, channels)}
		for ch := range packet.samples {
			packet.samples[ch] = make([]float32, frames)
		}
		a.available <- packet
	}
	return a
}

// Write makes exactly one owned copy for each accepted packet. It never blocks:
// full queues or another producer cause a silent drop, preserving CEF timing.
// The supplied slices are callback-owned and are not retained after return.
func (a *playerStreamAdapter) Write(samples [][]float32) error {
	if a.closed.Load() {
		return ErrStreamClosed
	}
	if !a.writeMu.TryLock() {
		a.recordDrop()
		return nil
	}
	defer a.writeMu.Unlock()
	if a.closed.Load() {
		return ErrStreamClosed
	}

	select {
	case packet := <-a.available:
		copyPacket(packet, samples)
		// A close racing after the first check must not publish a packet to a
		// stopped player. Returning the slot keeps the ring bounded.
		if a.closed.Load() {
			a.available <- packet
			return ErrStreamClosed
		}
		select {
		case a.ready <- packet:
			return nil
		default:
			// This cannot occur while ring invariants hold, but keep Write
			// non-blocking if an implementation change violates them.
			a.available <- packet
			a.recordDrop()
			return nil
		}
	default:
		a.recordDrop()
		return nil
	}
}

func copyPacket(packet *audioPacket, samples [][]float32) {
	packet.channels = min(len(packet.samples), len(samples))
	packet.frames = 0
	if packet.channels == 0 {
		return
	}
	frames := len(packet.samples[0])
	for ch := 0; ch < packet.channels; ch++ {
		n := min(len(packet.samples[ch]), len(samples[ch]))
		if n < frames {
			frames = n
		}
	}
	packet.frames = frames
	for ch := 0; ch < packet.channels; ch++ {
		copy(packet.samples[ch][:frames], samples[ch][:frames])
	}
}

func (a *playerStreamAdapter) recordDrop() {
	n := a.dropCount.Add(1)
	if (n == 1 || n&(n-1) == 0) && a.ctx != nil {
		logging.FromContext(a.ctx).Warn().Uint64("total_drops", n).Msg("pipewire: dropped audio packet (buffer full)")
	}
}

// fill is the PlayerCallbacks.Fill function. It performs the only post-handoff
// full-packet copy, directly into PipeWire's destination buffer.
func (a *playerStreamAdapter) fill(pcm *pwpipewire.PCMBuffer) (int, error) {
	select {
	case packet := <-a.ready:
		fillNum := a.fillCount.Add(1)
		if fillNum == 1 && a.ctx != nil {
			logging.FromContext(a.ctx).Info().Msg("pipewire: first Fill callback with data")
		}
		copied := copyToPCM(pcm, packet.samples[:packet.channels], packet.frames)
		a.available <- packet
		return copied, nil
	default:
		underruns := a.underrunCount.Add(1)
		if underruns == 1 && a.ctx != nil {
			logging.FromContext(a.ctx).Debug().Msg("pipewire: first Fill underrun (no data available)")
		}
		return 0, nil
	}
}

func copyToPCM(pcm *pwpipewire.PCMBuffer, samples [][]float32, packetFrames int) int {
	if pcm == nil || len(samples) == 0 || packetFrames == 0 {
		return 0
	}
	channels := min(min(len(samples), pcm.Channels), len(pcm.Samples))
	if channels == 0 {
		return 0
	}
	frames := min(pcm.Frames, packetFrames)
	for ch := 0; ch < channels; ch++ {
		frames = min(frames, min(len(pcm.Samples[ch]), len(samples[ch])))
	}
	for ch := 0; ch < channels; ch++ {
		copy(pcm.Samples[ch][:frames], samples[ch][:frames])
	}
	return frames
}

// Close stops the player. It is safe with Write: Write observes closed before
// publishing, and no channel is closed while a callback may be selecting on it.
func (a *playerStreamAdapter) Close() error {
	var err error
	a.closeOnce.Do(func() {
		a.closed.Store(true)
		if a.ctx != nil {
			logging.FromContext(a.ctx).Info().
				Uint64("fills", a.fillCount.Load()).
				Uint64("underruns", a.underrunCount.Load()).
				Uint64("drops", a.dropCount.Load()).
				Msg("pipewire: closing player")
		}
		if a.player != nil {
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

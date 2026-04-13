// Package pipewire provides a PipeWire-based audio output implementation.
// This wraps purego-pipewire's high-level Player API.
package pipewire

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	pwpipewire "github.com/bnema/purego-pipewire/pipewire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test doubles ---

// fakePlayer is a test double for purego-pipewire Player.
type fakePlayer struct {
	started    bool
	paused     bool
	stopped    bool
	closedFlag bool
	state      pwpipewire.PlayerState

	startErr error
	stopErr  error
	closeErr error
}

func (p *fakePlayer) Start() error {
	if p.startErr != nil {
		return p.startErr
	}
	p.started = true
	p.state = pwpipewire.PlayerStatePlaying
	return nil
}

func (p *fakePlayer) Pause() error {
	p.paused = true
	p.state = pwpipewire.PlayerStatePaused
	return nil
}

func (p *fakePlayer) Stop() error {
	if p.stopErr != nil {
		return p.stopErr
	}
	p.stopped = true
	p.state = pwpipewire.PlayerStateStopped
	return nil
}

func (p *fakePlayer) Close() error {
	if p.closeErr != nil {
		return p.closeErr
	}
	p.closedFlag = true
	p.state = pwpipewire.PlayerStateClosed
	return nil
}

func (p *fakePlayer) State() pwpipewire.PlayerState {
	return p.state
}

// --- Factory tests ---

func TestFactory_NewStream_ValidFormat_CreatesStream(t *testing.T) {
	var capturedConfig pwpipewire.PlayerConfig
	player := &fakePlayer{}

	factory := &Factory{
		createPlayer: func(config pwpipewire.PlayerConfig, _ pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
			capturedConfig = config
			return player, nil
		},
	}

	format := port.AudioStreamFormat{
		SampleRate:      48000,
		ChannelCount:    2,
		FramesPerBuffer: 512,
	}

	stream, err := factory.NewStream(context.Background(), format)
	require.NoError(t, err)
	require.NotNil(t, stream)

	// Verify config mapping
	assert.Equal(t, 48000, capturedConfig.SampleRate)
	assert.Equal(t, 2, capturedConfig.Channels)
	assert.Equal(t, 512, capturedConfig.FramesPerBuffer)
	assert.Equal(t, pwpipewire.SampleFormatF32, capturedConfig.SampleFormat)
	assert.Equal(t, pwpipewire.UnderrunFillSilence, capturedConfig.UnderrunPolicy)

	// Verify player was started
	assert.True(t, player.started)

	// Clean up
	require.NoError(t, stream.Close())
}

func TestFactory_NewStream_InvalidFormat_ReturnsError(t *testing.T) {
	factory := &Factory{
		createPlayer: func(_ pwpipewire.PlayerConfig, _ pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
			t.Fatal("createPlayer should not be called for invalid format")
			return nil, nil
		},
	}

	tests := []struct {
		name   string
		format port.AudioStreamFormat
	}{
		{
			name:   "zero sample rate",
			format: port.AudioStreamFormat{SampleRate: 0, ChannelCount: 2, FramesPerBuffer: 512},
		},
		{
			name:   "zero channels",
			format: port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 0, FramesPerBuffer: 512},
		},
		{
			name:   "negative sample rate",
			format: port.AudioStreamFormat{SampleRate: -48000, ChannelCount: 2, FramesPerBuffer: 512},
		},
		{
			name:   "negative channels",
			format: port.AudioStreamFormat{SampleRate: 48000, ChannelCount: -2, FramesPerBuffer: 512},
		},
		{
			name:   "excessive sample rate",
			format: port.AudioStreamFormat{SampleRate: 1000000, ChannelCount: 2, FramesPerBuffer: 512},
		},
		{
			name:   "excessive channels",
			format: port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 1025, FramesPerBuffer: 512},
		},
		{
			name:   "zero frames per buffer",
			format: port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 0},
		},
		{
			name:   "excessive frames per buffer",
			format: port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 10000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := factory.NewStream(context.Background(), tt.format)
			require.Error(t, err)
			assert.Nil(t, stream)
			assert.ErrorIs(t, err, ErrInvalidFormat)
		})
	}
}

func TestFactory_NewStream_PlayerCreationFails_ReturnsError(t *testing.T) {
	expectedErr := errors.New("pipewire not available")
	factory := &Factory{
		createPlayer: func(_ pwpipewire.PlayerConfig, _ pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
			return nil, expectedErr
		},
	}

	format := port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 512}
	stream, err := factory.NewStream(context.Background(), format)

	require.Error(t, err)
	assert.Nil(t, stream)
	assert.ErrorIs(t, err, expectedErr)
}

func TestFactory_NewStream_PlayerStartFails_ClosesAndReturnsError(t *testing.T) {
	player := &fakePlayer{startErr: errors.New("start failed")}
	factory := &Factory{
		createPlayer: func(_ pwpipewire.PlayerConfig, _ pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
			return player, nil
		},
	}

	format := port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 512}
	stream, err := factory.NewStream(context.Background(), format)

	require.Error(t, err)
	assert.Nil(t, stream)
	assert.True(t, player.closedFlag, "player should be closed after start failure")
}

func TestFactory_NewStream_ContextCancelled_ReturnsError(t *testing.T) {
	factory := &Factory{
		createPlayer: func(_ pwpipewire.PlayerConfig, _ pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
			t.Fatal("createPlayer should not be called when context is canceled")
			return nil, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	format := port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 512}
	stream, err := factory.NewStream(ctx, format)

	require.Error(t, err)
	assert.Nil(t, stream)
}

func TestFactory_NewStream_MultipleFormats_Accepted(t *testing.T) {
	formats := []port.AudioStreamFormat{
		{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 512},
		{SampleRate: 44100, ChannelCount: 2, FramesPerBuffer: 512},
		{SampleRate: 48000, ChannelCount: 1, FramesPerBuffer: 256},
		{SampleRate: 96000, ChannelCount: 2, FramesPerBuffer: 1024},
		{SampleRate: 48000, ChannelCount: 6, FramesPerBuffer: 512},
	}

	for _, fmt := range formats {
		t.Run("", func(t *testing.T) {
			factory := &Factory{
				createPlayer: func(config pwpipewire.PlayerConfig, _ pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
					assert.Equal(t, fmt.SampleRate, config.SampleRate)
					assert.Equal(t, fmt.ChannelCount, config.Channels)
					assert.Equal(t, fmt.FramesPerBuffer, config.FramesPerBuffer)
					return &fakePlayer{}, nil
				},
			}

			stream, err := factory.NewStream(context.Background(), fmt)
			require.NoError(t, err)
			assert.NotNil(t, stream)

			_ = stream.Close()
		})
	}
}

func TestFactory_ImplementsPortInterface(_ *testing.T) {
	var _ port.AudioOutputFactory = (*Factory)(nil)
}

// --- Stream adapter tests ---

func TestPlayerStreamAdapter_Write_SendsToBuffer(t *testing.T) {
	player := &fakePlayer{}
	adapter := &playerStreamAdapter{
		player: player,
		buf:    make(chan [][]float32, 4),
		closed: make(chan struct{}),
	}

	samples := [][]float32{
		{0.1, 0.2},
		{0.3, 0.4},
	}

	err := adapter.Write(samples)
	require.NoError(t, err)

	// Verify data is in the channel
	received := <-adapter.buf
	assert.Equal(t, samples, received)
}

func TestPlayerStreamAdapter_Write_AfterClose_ReturnsError(t *testing.T) {
	player := &fakePlayer{}
	adapter := &playerStreamAdapter{
		player: player,
		buf:    make(chan [][]float32, 4),
		closed: make(chan struct{}),
	}

	require.NoError(t, adapter.Close())

	err := adapter.Write([][]float32{{0.1}})
	assert.ErrorIs(t, err, ErrStreamClosed)
}

func TestPlayerStreamAdapter_Close_StopsAndClosesPlayer(t *testing.T) {
	player := &fakePlayer{}
	adapter := &playerStreamAdapter{
		player: player,
		buf:    make(chan [][]float32, 4),
		closed: make(chan struct{}),
	}

	err := adapter.Close()
	require.NoError(t, err)

	assert.True(t, player.stopped)
	assert.True(t, player.closedFlag)
}

func TestPlayerStreamAdapter_Close_IsIdempotent(t *testing.T) {
	player := &fakePlayer{}
	adapter := &playerStreamAdapter{
		player: player,
		buf:    make(chan [][]float32, 4),
		closed: make(chan struct{}),
	}

	require.NoError(t, adapter.Close())
	require.NoError(t, adapter.Close())
}

func TestPlayerStreamAdapter_Close_PropagatesStopError(t *testing.T) {
	player := &fakePlayer{stopErr: errors.New("stop error")}
	adapter := &playerStreamAdapter{
		player: player,
		buf:    make(chan [][]float32, 4),
		closed: make(chan struct{}),
	}

	err := adapter.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stop error")
}

func TestPlayerStreamAdapter_Close_PropagatesCloseError(t *testing.T) {
	player := &fakePlayer{closeErr: errors.New("close error")}
	adapter := &playerStreamAdapter{
		player: player,
		buf:    make(chan [][]float32, 4),
		closed: make(chan struct{}),
	}

	err := adapter.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "close error")
}

// --- Fill callback tests ---

func TestFill_PullsFromBuffer(t *testing.T) {
	adapter := &playerStreamAdapter{
		buf: make(chan [][]float32, 4),
	}

	// Push samples
	samples := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}
	adapter.buf <- samples

	// Simulate Fill call
	pcm := &pwpipewire.PCMBuffer{
		Frames:   3,
		Channels: 2,
		Stride:   1,
		Samples: [][]float32{
			make([]float32, 3),
			make([]float32, 3),
		},
	}

	frames, err := adapter.fill(pcm)
	require.NoError(t, err)
	assert.Equal(t, 3, frames)
	assert.InEpsilon(t, 0.1, pcm.Samples[0][0], 0.0001)
	assert.InEpsilon(t, 0.4, pcm.Samples[1][0], 0.0001)
}

func TestFill_ReturnsZeroWhenEmpty(t *testing.T) {
	adapter := &playerStreamAdapter{
		buf: make(chan [][]float32, 4),
	}

	pcm := &pwpipewire.PCMBuffer{
		Frames:   256,
		Channels: 2,
		Stride:   1,
		Samples: [][]float32{
			make([]float32, 256),
			make([]float32, 256),
		},
	}

	frames, err := adapter.fill(pcm)
	require.NoError(t, err)
	assert.Equal(t, 0, frames)
}

func TestCopyToPCM_HandlesChannelMismatch(t *testing.T) {
	// More source channels than PCM channels — truncates
	pcm := &pwpipewire.PCMBuffer{
		Frames:   2,
		Channels: 1,
		Samples:  [][]float32{make([]float32, 2)},
	}
	samples := [][]float32{
		{1.0, 2.0},
		{3.0, 4.0}, // should be ignored
	}
	frames := copyToPCM(pcm, samples)
	assert.Equal(t, 2, frames)
	assert.InEpsilon(t, 1.0, pcm.Samples[0][0], 0.0001)
}

func TestCopyToPCM_HandlesFrameMismatch(t *testing.T) {
	// Fewer source frames than PCM expects
	pcm := &pwpipewire.PCMBuffer{
		Frames:   4,
		Channels: 1,
		Samples:  [][]float32{make([]float32, 4)},
	}
	samples := [][]float32{
		{1.0, 2.0}, // only 2 frames
	}
	frames := copyToPCM(pcm, samples)
	assert.Equal(t, 2, frames) // reports partial fill
}

func TestCopyToPCM_NilPCM_ReturnsZero(t *testing.T) {
	assert.Equal(t, 0, copyToPCM(nil, [][]float32{{1.0}}))
}

func TestCopyToPCM_EmptySamples_ReturnsZero(t *testing.T) {
	pcm := &pwpipewire.PCMBuffer{Frames: 1, Channels: 1, Samples: [][]float32{make([]float32, 1)}}
	assert.Equal(t, 0, copyToPCM(pcm, nil))
	assert.Equal(t, 0, copyToPCM(pcm, [][]float32{}))
}

// --- Non-blocking Write tests ---

func TestPlayerStreamAdapter_Write_DropsWhenBufferFull(t *testing.T) {
	adapter := &playerStreamAdapter{
		player: &fakePlayer{},
		buf:    make(chan [][]float32, 4),
		closed: make(chan struct{}),
	}

	// Fill the buffer to capacity
	for i := 0; i < 4; i++ {
		require.NoError(t, adapter.Write([][]float32{{float32(i)}}))
	}

	// The 5th write must NOT block — it should drop the packet and return nil.
	err := adapter.Write([][]float32{{99.0}})
	require.NoError(t, err, "Write must not return an error when dropping due to full buffer")

	// Verify the drop was counted
	assert.Equal(t, uint64(1), adapter.dropCount.Load(), "one packet should have been dropped")

	// Verify buffer still contains the original 4 packets (not the dropped one)
	assert.Len(t, adapter.buf, 4)
}

func TestPlayerStreamAdapter_Write_DropsLogOnlyOnFirstAndPowerOfTwo(t *testing.T) {
	adapter := &playerStreamAdapter{
		player: &fakePlayer{},
		buf:    make(chan [][]float32, 1), // tiny buffer to force drops
		closed: make(chan struct{}),
	}

	// Fill buffer
	require.NoError(t, adapter.Write([][]float32{{0.0}}))

	// Drop 10 packets
	for i := 0; i < 10; i++ {
		_ = adapter.Write([][]float32{{float32(i)}})
	}

	// Verify all drops were counted
	assert.Equal(t, uint64(10), adapter.dropCount.Load())
}

// --- Concurrent Write / Close tests ---

func TestPlayerStreamAdapter_ConcurrentWriteClose_NoRace(t *testing.T) {
	player := &fakePlayer{}
	adapter := &playerStreamAdapter{
		player: player,
		buf:    make(chan [][]float32, 4),
		closed: make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			err := adapter.Write([][]float32{{float32(i)}})
			if err != nil {
				assert.ErrorIs(t, err, ErrStreamClosed)
				return
			}
		}
	}()

	// Let a few writes land, then close.
	adapter.Close()
	<-done
}

// --- NewFactory tests ---

func TestNewFactory_Succeeds(t *testing.T) {
	factory, err := NewFactory()
	require.NoError(t, err)
	require.NotNil(t, factory)
	assert.NotNil(t, factory.createPlayer)
}

// --- Integration-style test: full write-through ---

func TestStream_WriteFlowsThroughToFill(t *testing.T) {
	var capturedCallbacks pwpipewire.PlayerCallbacks
	player := &fakePlayer{}

	factory := &Factory{
		createPlayer: func(_ pwpipewire.PlayerConfig, callbacks pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
			capturedCallbacks = callbacks
			return player, nil
		},
	}

	format := port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 4}
	stream, err := factory.NewStream(context.Background(), format)
	require.NoError(t, err)

	// Write samples through the stream
	samples := [][]float32{
		{0.1, 0.2, 0.3, 0.4},
		{0.5, 0.6, 0.7, 0.8},
	}
	require.NoError(t, stream.Write(samples))

	// Simulate the Fill callback being called by the player
	pcm := &pwpipewire.PCMBuffer{
		Frames:   4,
		Channels: 2,
		Stride:   1,
		Samples: [][]float32{
			make([]float32, 4),
			make([]float32, 4),
		},
	}

	frames, err := capturedCallbacks.Fill(pcm)
	require.NoError(t, err)
	assert.Equal(t, 4, frames)
	assert.InEpsilon(t, 0.1, pcm.Samples[0][0], 0.0001)
	assert.InEpsilon(t, 0.5, pcm.Samples[1][0], 0.0001)

	require.NoError(t, stream.Close())
}

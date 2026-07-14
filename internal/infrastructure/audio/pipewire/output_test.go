package pipewire

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	pipewiremocks "github.com/bnema/dumber/internal/infrastructure/audio/pipewire/mocks"
	pwpipewire "github.com/bnema/purego-pipewire/pipewire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type playerRecorder struct {
	*pipewiremocks.MockPlayer
	started, stopped, closedFlag bool
	startErr, stopErr, closeErr  error
}

func newPlayerRecorder(t *testing.T) *playerRecorder {
	t.Helper()
	p := &playerRecorder{MockPlayer: pipewiremocks.NewMockPlayer(t)}
	p.EXPECT().Start().RunAndReturn(func() error {
		if p.startErr != nil {
			return p.startErr
		}
		p.started = true
		return nil
	}).Maybe()
	p.EXPECT().Stop().RunAndReturn(func() error {
		if p.stopErr != nil {
			return p.stopErr
		}
		p.stopped = true
		return nil
	}).Maybe()
	p.EXPECT().Close().RunAndReturn(func() error {
		if p.closeErr != nil {
			return p.closeErr
		}
		p.closedFlag = true
		return nil
	}).Maybe()
	return p
}

func testAdapter(frames int) *playerStreamAdapter {
	return newPlayerStreamAdapter(context.Background(), 2, frames)
}

func stereoPCM(frames int) *pwpipewire.PCMBuffer {
	return &pwpipewire.PCMBuffer{Frames: frames, Channels: 2, Samples: [][]float32{make([]float32, frames), make([]float32, frames)}}
}

func TestFactory_NewStreamValidatesAndStartsPlayer(t *testing.T) {
	player := newPlayerRecorder(t)
	factory := &Factory{createPlayer: func(config pwpipewire.PlayerConfig, callbacks pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
		assert.Equal(t, 48000, config.SampleRate)
		assert.Equal(t, 2, config.Channels)
		assert.NotNil(t, callbacks.Fill)
		return player, nil
	}}
	stream, err := factory.NewStream(context.Background(), port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 512})
	require.NoError(t, err)
	assert.True(t, player.started)
	require.NoError(t, stream.Close())
}

func TestFactory_NewStreamRejectsInvalidFormat(t *testing.T) {
	factory := &Factory{createPlayer: func(pwpipewire.PlayerConfig, pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
		t.Fatal("unexpected player")
		return nil, nil
	}}
	for _, format := range []port.AudioStreamFormat{
		{SampleRate: 0, ChannelCount: 2, FramesPerBuffer: 512},
		{SampleRate: 48000, ChannelCount: 0, FramesPerBuffer: 512},
		{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 0},
	} {
		stream, err := factory.NewStream(context.Background(), format)
		require.ErrorIs(t, err, ErrInvalidFormat)
		assert.Nil(t, stream)
	}
}

func TestFactory_NewStreamClosesPlayerWhenStartFails(t *testing.T) {
	player := newPlayerRecorder(t)
	player.startErr = errors.New("start failed")
	factory := &Factory{createPlayer: func(pwpipewire.PlayerConfig, pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
		return player, nil
	}}
	stream, err := factory.NewStream(context.Background(), port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 512})
	require.Error(t, err)
	assert.Nil(t, stream)
	assert.True(t, player.closedFlag)
}

func TestPlayerStreamAdapter_FillGetsOwnedPacketAfterCallbackMutation(t *testing.T) {
	adapter := testAdapter(3)
	samples := [][]float32{{1, 2, 3}, {4, 5, 6}}
	require.NoError(t, adapter.Write(samples))
	samples[0][0], samples[1][0] = 99, 99
	pcm := stereoPCM(3)
	frames, err := adapter.fill(pcm)
	require.NoError(t, err)
	assert.Equal(t, 3, frames)
	assert.InEpsilon(t, 1, pcm.Samples[0][0], 0.0001)
	assert.InEpsilon(t, 4, pcm.Samples[1][0], 0.0001)
}

func TestPlayerStreamAdapter_SaturationDropsWithoutBlocking(t *testing.T) {
	adapter := testAdapter(2)
	for i := 0; i < packetQueueCapacity; i++ {
		require.NoError(t, adapter.Write([][]float32{{float32(i)}, {float32(i)}}))
	}
	done := make(chan error, 1)
	go func() { done <- adapter.Write([][]float32{{99}, {99}}) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Write blocked while the bounded queue was saturated")
	}
	assert.Equal(t, uint64(1), adapter.dropCount.Load())
	assert.Len(t, adapter.ready, packetQueueCapacity)
}

func TestPlayerStreamAdapter_EmptyFillCountsUnderrun(t *testing.T) {
	adapter := testAdapter(2)
	frames, err := adapter.fill(stereoPCM(2))
	require.NoError(t, err)
	assert.Zero(t, frames)
	assert.Equal(t, uint64(1), adapter.underrunCount.Load())
}

func TestPlayerStreamAdapter_CopyToPCMHandlesMismatches(t *testing.T) {
	pcm := &pwpipewire.PCMBuffer{Frames: 4, Channels: 1, Samples: [][]float32{make([]float32, 4)}}
	assert.Equal(t, 2, copyToPCM(pcm, [][]float32{{1, 2}, {3, 4}}, 2))
	assert.InEpsilon(t, 1, pcm.Samples[0][0], 0.0001)
	assert.Zero(t, copyToPCM(nil, [][]float32{{1}}, 1))
}

func TestPlayerStreamAdapter_CloseVsWrite(t *testing.T) {
	adapter := testAdapter(2)
	player := newPlayerRecorder(t)
	adapter.player = player
	require.NoError(t, adapter.Close())
	require.ErrorIs(t, adapter.Write([][]float32{{1}, {2}}), ErrStreamClosed)
	assert.True(t, player.stopped)
	assert.True(t, player.closedFlag)
}

func TestPlayerStreamAdapter_ConcurrentWriteClose_NoRace(t *testing.T) {
	adapter := testAdapter(4)
	var writers sync.WaitGroup
	for range 4 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for range 1000 {
				err := adapter.Write([][]float32{{1, 2}, {3, 4}})
				if err != nil {
					assert.ErrorIs(t, err, ErrStreamClosed)
					return
				}
			}
		}()
	}
	adapter.Close()
	writers.Wait()
}

func TestStream_WriteFlowsThroughToFill(t *testing.T) {
	var callbacks pwpipewire.PlayerCallbacks
	player := newPlayerRecorder(t)
	factory := &Factory{createPlayer: func(_ pwpipewire.PlayerConfig, got pwpipewire.PlayerCallbacks) (pwpipewire.Player, error) {
		callbacks = got
		return player, nil
	}}
	stream, err := factory.NewStream(context.Background(), port.AudioStreamFormat{SampleRate: 48000, ChannelCount: 2, FramesPerBuffer: 4})
	require.NoError(t, err)
	require.NoError(t, stream.Write([][]float32{{.1, .2, .3, .4}, {.5, .6, .7, .8}}))
	pcm := stereoPCM(4)
	frames, err := callbacks.Fill(pcm)
	require.NoError(t, err)
	assert.Equal(t, 4, frames)
	assert.InEpsilon(t, .1, pcm.Samples[0][0], .0001)
	assert.InEpsilon(t, .5, pcm.Samples[1][0], .0001)
	require.NoError(t, stream.Close())
}

func BenchmarkPlayerStreamAdapterStereo(b *testing.B) {
	adapter := testAdapter(512)
	samples := [][]float32{make([]float32, 512), make([]float32, 512)}
	pcm := stereoPCM(512)
	b.ReportAllocs()
	b.SetBytes(2 * 512 * 4)
	for i := 0; i < b.N; i++ {
		if err := adapter.Write(samples); err != nil {
			b.Fatal(err)
		}
		if _, err := adapter.fill(pcm); err != nil {
			b.Fatal(err)
		}
	}
}

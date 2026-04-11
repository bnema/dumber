package cef

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/audio/null"
	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEngine_WiresAudioOutputFactory verifies that the audio output factory
// is properly injected through engine initialization.
func TestEngine_WiresAudioOutputFactory(t *testing.T) {
	// This test verifies the wiring pattern. Since we can't easily
	// initialize a full CEF engine in unit tests, we verify the
	// webViewFactoryOptions struct accepts the audio factory.
	audioFactory := &null.Factory{}

	// Verify webViewFactoryOptions can hold the audio factory
	opts := webViewFactoryOptions{
		scale:               1,
		windowlessFrameRate: 60,
		audioOutputFactory:  audioFactory,
	}

	require.NotNil(t, opts.audioOutputFactory)
	assert.Equal(t, audioFactory, opts.audioOutputFactory)

	// Verify the factory is passed through
	factory := newWebViewFactory(&Engine{}, &glLoader{}, opts)
	require.NotNil(t, factory.audioOutputFactory)
}

// TestWebViewFactory_AudioFactoryIsOptional verifies that nil audio factory
// is handled gracefully (no panic, no audio output).
func TestWebViewFactory_AudioFactoryIsOptional(t *testing.T) {
	// Arrange
	opts := webViewFactoryOptions{
		scale:               1,
		windowlessFrameRate: 60,
		audioOutputFactory:  nil,
	}

	// Act
	factory := newWebViewFactory(&Engine{}, &glLoader{}, opts)

	// Assert
	require.NotNil(t, factory)
	assert.Nil(t, factory.audioOutputFactory)
}

// TestHandlerSet_ReceivesAudioFactory verifies the handler set receives
// the audio factory from the WebView.
func TestHandlerSet_ReceivesAudioFactory(t *testing.T) {
	// This test verifies that handlerSet can access the audio factory
	// through its associated WebView.
	audioFactory := &null.Factory{}

	wv := &WebView{
		ctx:                context.Background(),
		audioOutputFactory: audioFactory,
	}

	handlers := &handlerSet{wv: wv}

	// Verify the handler can access the audio factory through the WebView
	require.NotNil(t, handlers.wv)
	assert.Equal(t, audioFactory, handlers.wv.audioOutputFactory)
}

// TestWebView_AudioStreamLifecycleFieldsExist verifies the WebView has
// the necessary fields for audio stream lifecycle management.
func TestWebView_AudioStreamLifecycleFieldsExist(t *testing.T) {
	wv := &WebView{
		ctx:                context.Background(),
		audioOutputFactory: &null.Factory{},
	}

	// Verify the fields exist and can be set
	wv.activeAudioStream = &mockAudioStream{}

	assert.NotNil(t, wv.audioOutputFactory)
	assert.NotNil(t, wv.activeAudioStream)
}

// mockAudioStream is a test double for port.AudioOutputStream
type mockAudioStream struct {
	writeCalled  bool
	closeCalled  bool
	writeSamples [][]float32
}

func (m *mockAudioStream) Write(samples [][]float32) error {
	m.writeCalled = true
	m.writeSamples = samples
	return nil
}

func (m *mockAudioStream) Close() error {
	m.closeCalled = true
	return nil
}

// Verify mock implements the interface
var _ port.AudioOutputStream = (*mockAudioStream)(nil)

// racyMockAudioStream blocks inside Write until doneCh is closed, allowing
// tests to verify that the caller holds a lock across the Write call.
type racyMockAudioStream struct {
	writeCh     chan struct{} // closed when Write is entered
	doneCh      chan struct{} // Write blocks until this is closed
	closeCalled bool
}

func (m *racyMockAudioStream) Write(_ [][]float32) error {
	close(m.writeCh) // signal that Write has been entered
	<-m.doneCh       // block until test unblocks us
	return nil
}

func (m *racyMockAudioStream) Close() error {
	m.closeCalled = true
	return nil
}

var _ port.AudioOutputStream = (*racyMockAudioStream)(nil)

// ============================================================================
// Task 6: Audio Stream Lifecycle Handling Tests
// ============================================================================

// TestHandlerSet_OnAudioStreamStarted_CreatesStream verifies that
// OnAudioStreamStarted creates a new audio stream from the factory.
func TestHandlerSet_OnAudioStreamStarted_CreatesStream(t *testing.T) {
	ctx := context.Background()
	mockStream := &mockAudioStream{}
	mockFactory := &mockAudioFactory{stream: mockStream}

	wv := &WebView{
		ctx:                ctx,
		audioOutputFactory: mockFactory,
	}

	handlers := &handlerSet{wv: wv}

	params := &purecef.AudioParameters{
		SampleRate:      48000,
		ChannelLayout:   purecef.ChannelLayoutStereo,
		FramesPerBuffer: 512,
	}

	// Act — third arg is channels, not framesPerBuffer
	handlers.OnAudioStreamStarted(nil, params, 2)

	// Assert
	require.NotNil(t, wv.activeAudioStream)
	assert.Equal(t, mockStream, wv.activeAudioStream)
	assert.True(t, wv.audioPlaying.Load())

	// Verify factory was called with correct format
	require.NotNil(t, mockFactory.lastFormat)
	assert.Equal(t, 48000, mockFactory.lastFormat.SampleRate)
	assert.Equal(t, 2, mockFactory.lastFormat.ChannelCount)
	assert.Equal(t, 512, mockFactory.lastFormat.FramesPerBuffer)
}

// TestHandlerSet_OnAudioStreamStarted_NoFactory_DoesNotPanic verifies
// that OnAudioStreamStarted handles nil factory gracefully.
func TestHandlerSet_OnAudioStreamStarted_NoFactory_DoesNotPanic(t *testing.T) {
	wv := &WebView{
		ctx:                context.Background(),
		audioOutputFactory: nil,
	}

	handlers := &handlerSet{wv: wv}

	params := &purecef.AudioParameters{
		SampleRate:      48000,
		ChannelLayout:   purecef.ChannelLayoutStereo,
		FramesPerBuffer: 512,
	}

	// Should not panic
	assert.NotPanics(t, func() {
		handlers.OnAudioStreamStarted(nil, params, 512)
	})

	assert.False(t, wv.audioPlaying.Load())
}

// TestHandlerSet_OnAudioStreamStarted_ClosesExistingStream verifies
// that starting a new stream closes any existing one.
func TestHandlerSet_OnAudioStreamStarted_ClosesExistingStream(t *testing.T) {
	ctx := context.Background()
	oldStream := &mockAudioStream{}
	newStream := &mockAudioStream{}
	mockFactory := &mockAudioFactory{stream: newStream}

	wv := &WebView{
		ctx:                ctx,
		audioOutputFactory: mockFactory,
		activeAudioStream:  oldStream,
	}

	handlers := &handlerSet{wv: wv}

	params := &purecef.AudioParameters{
		SampleRate:      44100,
		ChannelLayout:   purecef.ChannelLayoutMono,
		FramesPerBuffer: 256,
	}

	// Act — third arg is channels (1 for mono)
	handlers.OnAudioStreamStarted(nil, params, 1)

	// Assert
	assert.True(t, oldStream.closeCalled)
	assert.Equal(t, newStream, wv.activeAudioStream)
}

// TestHandlerSet_OnAudioStreamStarted_FactoryError_HandlesGracefully verifies
// that factory errors are handled without panic and audioPlaying is reset to false.
func TestHandlerSet_OnAudioStreamStarted_FactoryError_HandlesGracefully(t *testing.T) {
	ctx := context.Background()
	mockFactory := &mockAudioFactory{err: errors.New("factory error")}

	wv := &WebView{
		ctx:                ctx,
		audioOutputFactory: mockFactory,
	}

	handlers := &handlerSet{wv: wv}

	params := &purecef.AudioParameters{
		SampleRate:      48000,
		ChannelLayout:   purecef.ChannelLayoutStereo,
		FramesPerBuffer: 512,
	}

	// Should not panic
	assert.NotPanics(t, func() {
		handlers.OnAudioStreamStarted(nil, params, 512)
	})

	// audioPlaying must be false when NewStream fails — if it stays true,
	// the UI/state layer incorrectly believes audio is active.
	assert.False(t, wv.audioPlaying.Load(), "audioPlaying must be false after NewStream failure")
	assert.Nil(t, wv.activeAudioStream, "activeAudioStream must be nil after NewStream failure")
}

// ============================================================================
// Task 7: PCM Packet Forwarding Tests
// ============================================================================

// TestHandlerSet_OnAudioStreamPacket_CopiesAndWrites verifies that
// OnAudioStreamPacket copies samples before writing to avoid data races.
func TestHandlerSet_OnAudioStreamPacket_CopiesAndWrites(t *testing.T) {
	ctx := context.Background()
	mockStream := &mockAudioStream{}

	wv := &WebView{
		ctx:               ctx,
		activeAudioStream: mockStream,
	}

	handlers := &handlerSet{wv: wv}

	// Create sample data: 2 channels, 512 frames each
	originalData := [][]float32{
		make([]float32, 512), // Channel 0
		make([]float32, 512), // Channel 1
	}
	originalData[0][0] = 0.5
	originalData[1][0] = -0.5

	// Act
	handlers.OnAudioStreamPacket(nil, originalData, 512, 0)

	// Assert
	require.True(t, mockStream.writeCalled)
	require.Len(t, mockStream.writeSamples, 2)
	require.Len(t, mockStream.writeSamples[0], 512)

	// Verify data was copied (values match)
	assert.InDelta(t, 0.5, mockStream.writeSamples[0][0], 0.000001)
	assert.InDelta(t, -0.5, mockStream.writeSamples[1][0], 0.000001)

	// Verify it's a copy (different underlying array)
	assert.NotSame(t, &originalData[0][0], &mockStream.writeSamples[0][0])
}

// TestHandlerSet_OnAudioStreamPacket_NoActiveStream_DoesNothing verifies
// that packets are silently discarded when no stream is active.
func TestHandlerSet_OnAudioStreamPacket_NoActiveStream_DoesNothing(t *testing.T) {
	wv := &WebView{
		ctx:               context.Background(),
		activeAudioStream: nil,
	}

	handlers := &handlerSet{wv: wv}

	data := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		handlers.OnAudioStreamPacket(nil, data, 3, 0)
	})
}

// TestHandlerSet_OnAudioStreamPacket_NilData_DoesNotPanic verifies
// that nil data is handled gracefully.
func TestHandlerSet_OnAudioStreamPacket_NilData_DoesNotPanic(t *testing.T) {
	mockStream := &mockAudioStream{}
	wv := &WebView{
		ctx:               context.Background(),
		activeAudioStream: mockStream,
	}

	handlers := &handlerSet{wv: wv}

	// Should not panic with nil data
	assert.NotPanics(t, func() {
		handlers.OnAudioStreamPacket(nil, nil, 0, 0)
	})
}

// ============================================================================
// Task 8: Stream Cleanup Tests
// ============================================================================

// TestHandlerSet_OnAudioStreamStopped_ClosesStream verifies that
// OnAudioStreamStopped closes the active stream.
func TestHandlerSet_OnAudioStreamStopped_ClosesStream(t *testing.T) {
	mockStream := &mockAudioStream{}
	wv := &WebView{
		ctx:               context.Background(),
		activeAudioStream: mockStream,
	}
	wv.audioPlaying.Store(true)

	handlers := &handlerSet{wv: wv}

	// Act
	handlers.OnAudioStreamStopped(nil)

	// Assert
	assert.True(t, mockStream.closeCalled)
	assert.Nil(t, wv.activeAudioStream)
	assert.False(t, wv.audioPlaying.Load())
}

// TestHandlerSet_OnAudioStreamStopped_NoActiveStream_DoesNotPanic verifies
// that stopping when no stream exists doesn't panic.
func TestHandlerSet_OnAudioStreamStopped_NoActiveStream_DoesNotPanic(t *testing.T) {
	wv := &WebView{
		ctx:               context.Background(),
		activeAudioStream: nil,
	}
	wv.audioPlaying.Store(true)

	handlers := &handlerSet{wv: wv}

	// Should not panic
	assert.NotPanics(t, func() {
		handlers.OnAudioStreamStopped(nil)
	})

	assert.False(t, wv.audioPlaying.Load())
}

// TestHandlerSet_OnAudioStreamError_ClosesStream verifies that
// OnAudioStreamError closes the active stream.
func TestHandlerSet_OnAudioStreamError_ClosesStream(t *testing.T) {
	mockStream := &mockAudioStream{}
	wv := &WebView{
		ctx:               context.Background(),
		activeAudioStream: mockStream,
	}
	wv.audioPlaying.Store(true)

	handlers := &handlerSet{wv: wv}

	// Act
	handlers.OnAudioStreamError(nil, "test error")

	// Assert
	assert.True(t, mockStream.closeCalled)
	assert.Nil(t, wv.activeAudioStream)
	assert.False(t, wv.audioPlaying.Load())
}

// TestHandlerSet_OnAudioStreamError_NoActiveStream_DoesNotPanic verifies
// that error handling when no stream exists doesn't panic.
func TestHandlerSet_OnAudioStreamError_NoActiveStream_DoesNotPanic(t *testing.T) {
	wv := &WebView{
		ctx:               context.Background(),
		activeAudioStream: nil,
	}
	wv.audioPlaying.Store(true)

	handlers := &handlerSet{wv: wv}

	// Should not panic
	assert.NotPanics(t, func() {
		handlers.OnAudioStreamError(nil, "test error")
	})

	assert.False(t, wv.audioPlaying.Load())
}

// TestWebView_Destroy_ClosesAudioStream verifies that destroying
// the WebView closes any active audio stream.
func TestWebView_Destroy_ClosesAudioStream(t *testing.T) {
	mockStream := &mockAudioStream{}
	wv := &WebView{
		ctx:               context.Background(),
		activeAudioStream: mockStream,
		pipeline:          &renderPipeline{},
	}
	wv.audioPlaying.Store(true)

	// Act — Destroy must close the audio stream as part of teardown.
	wv.Destroy()

	// Assert
	assert.True(t, mockStream.closeCalled, "Destroy must close the active audio stream")
	assert.Nil(t, wv.activeAudioStream)
}

// TestWebView_Destroy_NoAudioStream_DoesNotPanic verifies that Destroy
// handles the case where no audio stream is active.
func TestWebView_Destroy_NoAudioStream_DoesNotPanic(t *testing.T) {
	wv := &WebView{
		ctx:      context.Background(),
		pipeline: &renderPipeline{},
	}

	assert.NotPanics(t, func() {
		wv.Destroy()
	})
}

// ============================================================================
// Callback parameter mapping tests
// ============================================================================

// TestBuildAudioStreamFormat_UsesParamsFramesPerBuffer verifies that
// buildAudioStreamFormat reads FramesPerBuffer from the AudioParameters struct,
// NOT from the callback's third argument (which is channels).
func TestBuildAudioStreamFormat_UsesParamsFramesPerBuffer(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	handlers := &handlerSet{wv: wv}

	params := &purecef.AudioParameters{
		SampleRate:      48000,
		ChannelLayout:   purecef.ChannelLayoutStereo,
		FramesPerBuffer: 1024,
	}
	channels := int32(2) // third callback arg is channels, not framesPerBuffer

	format := handlers.buildAudioStreamFormat(params, channels)

	assert.Equal(t, 48000, format.SampleRate, "sample rate should come from params")
	assert.Equal(t, 2, format.ChannelCount, "channel count should match callback channels arg")
	assert.Equal(t, 1024, format.FramesPerBuffer, "frames_per_buffer should come from params.FramesPerBuffer, not the channels arg")
}

// TestBuildAudioStreamFormat_ChannelsArgDiffersFromLayout verifies correct
// behavior when the callback's channels arg and the layout-derived count differ.
// The channels callback arg is the authoritative channel count.
func TestBuildAudioStreamFormat_ChannelsArgDiffersFromLayout(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	handlers := &handlerSet{wv: wv}

	params := &purecef.AudioParameters{
		SampleRate:      44100,
		ChannelLayout:   purecef.ChannelLayoutStereo, // layout says 2
		FramesPerBuffer: 512,
	}
	channels := int32(1) // but CEF says 1 channel

	format := handlers.buildAudioStreamFormat(params, channels)

	assert.Equal(t, 44100, format.SampleRate)
	assert.Equal(t, 1, format.ChannelCount, "should use callback channels arg, not layout-derived count")
	assert.Equal(t, 512, format.FramesPerBuffer, "should use params.FramesPerBuffer")
}

// TestOnAudioStreamStarted_CorrectParameterMapping verifies end-to-end that
// OnAudioStreamStarted passes the correct values to the factory:
// - channels from the callback arg (not from params.FramesPerBuffer)
// - framesPerBuffer from params.FramesPerBuffer
func TestOnAudioStreamStarted_CorrectParameterMapping(t *testing.T) {
	ctx := context.Background()
	mockStream := &mockAudioStream{}
	mockFactory := &mockAudioFactory{stream: mockStream}

	wv := &WebView{
		ctx:                ctx,
		audioOutputFactory: mockFactory,
	}

	handlers := &handlerSet{wv: wv}

	params := &purecef.AudioParameters{
		SampleRate:      48000,
		ChannelLayout:   purecef.ChannelLayoutStereo,
		FramesPerBuffer: 1024,
	}

	// Third arg is channels=2, NOT framesPerBuffer.
	handlers.OnAudioStreamStarted(nil, params, 2)

	require.NotNil(t, mockFactory.lastFormat)
	assert.Equal(t, 48000, mockFactory.lastFormat.SampleRate)
	assert.Equal(t, 2, mockFactory.lastFormat.ChannelCount, "channel count should be 2 (from callback arg)")
	assert.Equal(t, 1024, mockFactory.lastFormat.FramesPerBuffer, "frames_per_buffer should be 1024 (from params.FramesPerBuffer)")
}

// ============================================================================
// Race-window tests (write/close interleave)
// ============================================================================

// TestOnAudioStreamPacket_WriteHoldsLock verifies that closeAudioStream cannot
// interleave between the stream snapshot and Write. If the lock is NOT held
// across Write, the goroutine calling closeAudioStream will Close the stream
// while Write is in-flight, producing a write-after-close.
func TestOnAudioStreamPacket_WriteHoldsLock(t *testing.T) {
	ctx := context.Background()

	// racyStream blocks inside Write until we signal, giving closeAudioStream
	// a window to race if the mutex isn't held.
	stream := &racyMockAudioStream{
		writeCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	wv := &WebView{
		ctx:               ctx,
		activeAudioStream: stream,
	}
	handlers := &handlerSet{wv: wv}

	data := [][]float32{{0.1, 0.2}, {0.3, 0.4}}

	// Start a packet write — it will block inside Write.
	go handlers.OnAudioStreamPacket(nil, data, 2, 0)

	// Wait for Write to be entered.
	<-stream.writeCh

	// Now try to close. If the implementation holds the lock across Write,
	// this will block until Write returns. If not, Close races with Write.
	closeDone := make(chan struct{})
	go func() {
		wv.closeAudioStream()
		close(closeDone)
	}()

	// Close should NOT have completed yet because Write holds the lock.
	// Give the goroutine time to acquire the lock if it can.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-closeDone:
		t.Fatal("closeAudioStream completed while Write was in-flight — lock not held across Write")
	default:
		// expected: closeAudioStream is blocked
	}

	// Unblock Write.
	close(stream.doneCh)

	// Now closeAudioStream should complete.
	<-closeDone

	assert.True(t, stream.closeCalled)
	assert.Nil(t, wv.activeAudioStream)
}

// TestOnAudioStreamPacket_StreamClosedBetweenPackets verifies that when a
// stream is closed between two OnAudioStreamPacket calls, the second call
// sees nil and silently drops the packet.
func TestOnAudioStreamPacket_StreamClosedBetweenPackets(t *testing.T) {
	ctx := context.Background()
	stream := &mockAudioStream{}

	wv := &WebView{
		ctx:               ctx,
		activeAudioStream: stream,
	}
	handlers := &handlerSet{wv: wv}
	data := [][]float32{{0.1, 0.2}, {0.3, 0.4}}

	// First packet succeeds.
	handlers.OnAudioStreamPacket(nil, data, 2, 0)
	assert.True(t, stream.writeCalled)

	// Close the stream.
	wv.closeAudioStream()

	// Second packet should be silently dropped.
	assert.NotPanics(t, func() {
		handlers.OnAudioStreamPacket(nil, data, 2, 1)
	})
}

// ============================================================================
// Mock Factories
// ============================================================================

// mockAudioFactory is a test double for port.AudioOutputFactory
type mockAudioFactory struct {
	stream     port.AudioOutputStream
	err        error
	lastFormat *port.AudioStreamFormat
	lastCtx    context.Context
}

func (m *mockAudioFactory) NewStream(ctx context.Context, format port.AudioStreamFormat) (port.AudioOutputStream, error) {
	m.lastCtx = ctx
	m.lastFormat = &format
	return m.stream, m.err
}

// Verify mock implements the interface
var _ port.AudioOutputFactory = (*mockAudioFactory)(nil)

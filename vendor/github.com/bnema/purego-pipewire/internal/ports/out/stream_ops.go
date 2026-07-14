package out

import "unsafe"

// PlaybackFormat describes the audio format used when connecting a playback
// stream.
type PlaybackFormat struct {
	SampleRate      int
	Channels        int
	FramesPerBuffer int
}

// StreamOps defines the outbound interface for PipeWire stream lifecycle
// operations that the player needs. This abstracts the raw C API so the
// player can be tested without a real PipeWire daemon.
type StreamOps interface {
	// CreatePlaybackStream creates a new playback stream attached to the given
	// main loop. The onProcess callback is invoked by PipeWire each time the
	// stream needs more audio data. Returns the stream pointer or an error.
	CreatePlaybackStream(loopPtr unsafe.Pointer, name string, onProcess func()) (streamPtr unsafe.Pointer, err error)

	// ConnectPlaybackStream connects the stream for playback output using the
	// supplied format.
	ConnectPlaybackStream(streamPtr unsafe.Pointer, format PlaybackFormat) error

	// SetStreamActive activates or deactivates the stream.
	SetStreamActive(streamPtr unsafe.Pointer, active bool) error

	// DequeueBuffer dequeues a buffer from the stream for writing.
	// Returns nil if no buffer is available.
	DequeueBuffer(streamPtr unsafe.Pointer) unsafe.Pointer

	// QueueBuffer queues a filled buffer back to the stream.
	QueueBuffer(streamPtr unsafe.Pointer, bufPtr unsafe.Pointer) error

	// DisconnectStream disconnects the stream.
	// Returns an error if the operation fails.
	DisconnectStream(streamPtr unsafe.Pointer) error

	// DestroyStream destroys the stream and frees its resources.
	DestroyStream(streamPtr unsafe.Pointer)

	// CreateMainLoop creates a new PipeWire main loop.
	// Returns the loop pointer or an error.
	CreateMainLoop() (loopPtr unsafe.Pointer, err error)

	// RunMainLoop runs the main loop (blocks until quit).
	// Returns an error if the loop fails to run.
	RunMainLoop(loopPtr unsafe.Pointer) error

	// QuitMainLoop signals the main loop to stop.
	QuitMainLoop(loopPtr unsafe.Pointer)

	// DestroyMainLoop destroys the main loop.
	DestroyMainLoop(loopPtr unsafe.Pointer)
}

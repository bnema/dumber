package capi

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/bnema/purego"

	portout "github.com/bnema/purego-pipewire/internal/ports/out"
)

// PWError represents a PipeWire C API error with the function name and return code.
type PWError struct {
	Func string
	Code int32
}

func (e *PWError) Error() string {
	return fmt.Sprintf("%s failed: %d", e.Func, e.Code)
}

// ErrInvalidPlaybackFormat is returned by ConnectPlaybackStream when the
// provided PlaybackFormat has zero or negative fields. Callers can use
// errors.Is to check for this condition specifically.
var ErrInvalidPlaybackFormat = errors.New("invalid playback format")

var (
	errStreamCreate       = &PWError{Func: "pw_stream_new_simple", Code: 0}
	errMainLoopCreate     = &PWError{Func: "pw_main_loop_new", Code: 0}
	errPWLoopFromMainLoop = &PWError{Func: "pw_main_loop_get_loop", Code: 0}
)

const pwVersionStreamEvents = 2

const (
	pwDirectionOutput                 = 1
	pwIDAny                           = 0xffffffff
	pwStreamFlagAutoConnectMapBuffers = 0x05
)

// streamCallbackStorage keeps the pw_stream_events struct and its
// associated Go callback alive for the lifetime of the stream.
// Without this, the stack-allocated values in CreatePlaybackStream
// would be collected while PipeWire still holds pointers to them.
type streamCallbackStorage struct {
	processCb uintptr
	events    pw_stream_events
}

// streamOpsImpl implements portout.StreamOps using the real PipeWire C API
// bindings registered via purego.
//
// Ownership/lifetime contract:
//   - pinned holds streamCallbackStorage for each stream created via CreatePlaybackStream.
//     It keeps Go callback closures alive so PipeWire can call them safely.
//   - pinnedParams holds connectParams for each stream connected via ConnectPlaybackStream.
//     PipeWire reads the SPA POD params asynchronously during format negotiation, so the
//     backing byte storage must remain live until DestroyStream releases it.
//   - Both maps are keyed by the stream pointer and must be cleaned up in DestroyStream.
type streamLoopContext struct {
	mainLoop unsafe.Pointer
	pwLoop   unsafe.Pointer
}

type streamOpsImpl struct {
	mu           sync.Mutex
	pinned       map[unsafe.Pointer]*streamCallbackStorage // keyed by stream ptr
	pinnedParams map[unsafe.Pointer]*connectParams         // keyed by stream ptr
	streamLoops  map[unsafe.Pointer]streamLoopContext      // keyed by stream ptr
	runningLoops map[unsafe.Pointer]bool                   // keyed by main loop ptr
}

// Verify interface compliance at compile time.
var _ portout.StreamOps = (*streamOpsImpl)(nil)

func (s *streamOpsImpl) CreatePlaybackStream(loopPtr unsafe.Pointer, name string, onProcess func()) (unsafe.Pointer, error) {
	// Obtain the inner pw_loop from the main loop; pw_stream_new_simple
	// expects a pw_loop, not a pw_main_loop.
	pwLoop := pw_main_loop_get_loop(loopPtr)
	if pwLoop == nil {
		return nil, errPWLoopFromMainLoop
	}

	nameBytes := append([]byte(name), 0)

	// Build persistent callback storage that outlives this function.
	storage := &streamCallbackStorage{}
	storage.processCb = purego.NewCallback(func(_ unsafe.Pointer) {
		onProcess()
	})
	storage.events = pw_stream_events{
		version: pwVersionStreamEvents,
		process: storage.processCb,
	}

	ptr := pw_stream_new_simple(
		pwLoop,
		&nameBytes[0],
		nil,                             // props
		unsafe.Pointer(&storage.events), // events
		nil,                             // data
	)
	if ptr == nil {
		return nil, errStreamCreate
	}

	// Pin the storage so GC cannot collect it while PipeWire holds references.
	s.mu.Lock()
	if s.pinned == nil {
		s.pinned = make(map[unsafe.Pointer]*streamCallbackStorage)
	}
	if s.streamLoops == nil {
		s.streamLoops = make(map[unsafe.Pointer]streamLoopContext)
	}
	s.pinned[ptr] = storage
	s.streamLoops[ptr] = streamLoopContext{mainLoop: loopPtr, pwLoop: pwLoop}
	s.mu.Unlock()

	return ptr, nil
}

func (s *streamOpsImpl) ConnectPlaybackStream(streamPtr unsafe.Pointer, format portout.PlaybackFormat) error {
	// Build the SPA POD params that describe the stream's audio format.
	cp, err := buildRawAudioParams(format)
	if err != nil {
		return err
	}

	// PW_DIRECTION_OUTPUT = 1, PW_ID_ANY = 0xffffffff
	// PW_STREAM_FLAG_AUTOCONNECT | PW_STREAM_FLAG_MAP_BUFFERS = 0x01 | 0x04
	ret := pw_stream_connect(streamPtr, pwDirectionOutput, pwIDAny, pwStreamFlagAutoConnectMapBuffers, cp.Pointer(), cp.Count())
	if ret < 0 {
		return &PWError{Func: "pw_stream_connect", Code: ret}
	}

	// Pin the connectParams for the lifetime of the stream. PipeWire reads
	// the SPA POD params asynchronously during format negotiation, so the
	// backing storage must remain alive until the stream is destroyed.
	s.mu.Lock()
	if s.pinnedParams == nil {
		s.pinnedParams = make(map[unsafe.Pointer]*connectParams)
	}
	s.pinnedParams[streamPtr] = cp
	s.mu.Unlock()

	return nil
}

func (s *streamOpsImpl) SetStreamActive(streamPtr unsafe.Pointer, active bool) error {
	ret := s.callStreamWithLoopLock(streamPtr, func() int32 {
		return pw_stream_set_active(streamPtr, active)
	})
	if ret < 0 {
		return &PWError{Func: "pw_stream_set_active", Code: ret}
	}
	return nil
}

func (s *streamOpsImpl) DequeueBuffer(streamPtr unsafe.Pointer) unsafe.Pointer {
	return pw_stream_dequeue_buffer(streamPtr)
}

func (s *streamOpsImpl) QueueBuffer(streamPtr unsafe.Pointer, bufPtr unsafe.Pointer) error {
	ret := pw_stream_queue_buffer(streamPtr, bufPtr)
	if ret < 0 {
		return &PWError{Func: "pw_stream_queue_buffer", Code: ret}
	}
	return nil
}

// DisconnectStream disconnects the stream from its port.
// Returns a PWError if the operation fails.
func (s *streamOpsImpl) DisconnectStream(streamPtr unsafe.Pointer) error {
	ret := s.callStreamWithLoopLock(streamPtr, func() int32 {
		return pw_stream_disconnect(streamPtr)
	})
	if ret < 0 {
		return &PWError{Func: "pw_stream_disconnect", Code: ret}
	}
	return nil
}

// DestroyStream releases the pw_stream and associated pinned Go storage.
// Callers must ensure the owning main loop has already stopped, or that they
// otherwise satisfy PipeWire's required stream context, before invoking this.
func (s *streamOpsImpl) DestroyStream(streamPtr unsafe.Pointer) {
	s.mu.Lock()
	storage, alive := s.pinned[streamPtr]
	if !alive {
		// Already destroyed or never tracked — skip.
		s.mu.Unlock()
		return
	}
	params := s.pinnedParams[streamPtr]
	delete(s.pinned, streamPtr)
	delete(s.pinnedParams, streamPtr)
	delete(s.streamLoops, streamPtr)
	s.mu.Unlock()

	pw_stream_destroy(streamPtr)

	// Keep callback and format storage alive until after pw_stream_destroy returns.
	runtime.KeepAlive(storage)
	runtime.KeepAlive(params)
}

func (s *streamOpsImpl) CreateMainLoop() (unsafe.Pointer, error) {
	ptr := pw_main_loop_new(nil)
	if ptr == nil {
		return nil, errMainLoopCreate
	}
	return ptr, nil
}

func (s *streamOpsImpl) RunMainLoop(loopPtr unsafe.Pointer) error {
	s.mu.Lock()
	if s.runningLoops == nil {
		s.runningLoops = make(map[unsafe.Pointer]bool)
	}
	s.runningLoops[loopPtr] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.runningLoops, loopPtr)
		s.mu.Unlock()
	}()

	ret := pw_main_loop_run(loopPtr)
	if ret < 0 {
		return &PWError{Func: "pw_main_loop_run", Code: ret}
	}
	return nil
}

func (s *streamOpsImpl) QuitMainLoop(loopPtr unsafe.Pointer) {
	// Use the generated pw_main_loop_quit binding to signal loop exit.
	pw_main_loop_quit(loopPtr)
}

func (s *streamOpsImpl) DestroyMainLoop(loopPtr unsafe.Pointer) {
	pw_main_loop_destroy(loopPtr)
}

func (s *streamOpsImpl) callStreamWithLoopLock(streamPtr unsafe.Pointer, fn func() int32) int32 {
	s.mu.Lock()
	ctx, running := s.streamLoopContextLocked(streamPtr)
	s.mu.Unlock()
	if running && ctx.pwLoop != nil {
		return withLoopLock(ctx.pwLoop, fn)
	}
	return fn()
}

func (s *streamOpsImpl) streamLoopContextLocked(streamPtr unsafe.Pointer) (streamLoopContext, bool) {
	if s.streamLoops == nil {
		return streamLoopContext{}, false
	}
	ctx, ok := s.streamLoops[streamPtr]
	if !ok {
		return streamLoopContext{}, false
	}
	return ctx, s.runningLoops != nil && s.runningLoops[ctx.mainLoop]
}

// DefaultStreamOps returns a StreamOps implementation backed by the real
// PipeWire C API. Must be called after Register().
func DefaultStreamOps() portout.StreamOps {
	return &streamOpsImpl{}
}

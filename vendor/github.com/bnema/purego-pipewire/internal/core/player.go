package core

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/bnema/purego-pipewire/internal/capi"
	portout "github.com/bnema/purego-pipewire/internal/ports/out"
)

var (
	ErrInvalidPlayerState = errors.New("invalid player state transition")
	ErrPlayerClosed       = errors.New("player is closed")

	registerPipeWire    = capi.Register
	defaultPipeWireCAPI = capi.Default
	defaultPipeWireOps  = capi.DefaultStreamOps
	initializePipeWire  = initPipeWire
)

// player is the internal player shell with state machine.
type player struct {
	// config is immutable after construction. It is safe to read from
	// any goroutine — including the PipeWire process callback — without
	// holding mu.
	config    playerConfig
	callbacks playerCallbacks
	state     atomic.Int32
	paused    atomic.Bool
	mu        sync.Mutex

	// pwView reuses Go slice headers for PipeWire callback buffers. Its native
	// references are cleared when each callback completes.
	pwView pwBufferView

	// Stream cleanup fields — nil when no stream is active.
	streamOps portout.StreamOps
	streamPtr unsafe.Pointer
	loopPtr   unsafe.Pointer
	loopDone  chan struct{}
}

// newPlayer creates a new player instance
func newPlayer(config PlayerConfig, callbacks PlayerCallbacks) *player {
	p := &player{
		config:    config,
		callbacks: callbacks,
		pwView:    newReusablePWBufferView(config.Channels, config.FramesPerBuffer),
	}
	p.state.Store(int32(PlayerStateIdle))
	return p
}

// State returns the current player state
func (p *player) State() PlayerState {
	return PlayerState(p.state.Load())
}

// setState atomically sets the player state
func (p *player) setState(next PlayerState) {
	p.state.Store(int32(next))
}

// transition attempts to transition to the next state
func (p *player) transition(next PlayerState) error {
	p.mu.Lock()

	current := p.State()

	// Validate transition
	if !isValidTransition(current, next) {
		p.mu.Unlock()
		return ErrInvalidPlayerState
	}

	// Perform transition
	p.setState(next)

	// Capture callback under lock
	cb := p.callbacks.OnStateChange

	p.mu.Unlock()

	// Invoke callback after releasing lock to avoid deadlock on reentry
	if cb != nil {
		cb(next)
	}

	return nil
}

// isValidTransition checks if a state transition is valid
func isValidTransition(current, next PlayerState) bool {
	switch current {
	case PlayerStateIdle:
		return next == PlayerStateStarting || next == PlayerStateClosing
	case PlayerStateStarting:
		return next == PlayerStatePlaying || next == PlayerStateError
	case PlayerStatePlaying:
		return next == PlayerStatePaused || next == PlayerStateStopped || next == PlayerStateClosing || next == PlayerStateError
	case PlayerStatePaused:
		return next == PlayerStatePlaying || next == PlayerStateStopped || next == PlayerStateClosing || next == PlayerStateError
	case PlayerStateStopped:
		return next == PlayerStateStarting || next == PlayerStateClosing
	case PlayerStateClosing:
		return next == PlayerStateClosed
	case PlayerStateClosed:
		return false // Terminal state
	case PlayerStateError:
		return next == PlayerStateClosing
	default:
		return false
	}
}

// Start begins playback, transitioning from Idle or Stopped to Playing.
func (p *player) Start() error {
	current := p.State()
	if current == PlayerStateClosed || current == PlayerStateClosing {
		return ErrPlayerClosed
	}

	// Only valid from Idle or Stopped
	if current != PlayerStateIdle && current != PlayerStateStopped {
		return ErrInvalidPlayerState
	}

	// Ensure runtime is available
	if err := p.ensureRuntime(); err != nil {
		return err
	}

	// Transition to Starting
	if err := p.transition(PlayerStateStarting); err != nil {
		return err
	}

	// Check if we already have PipeWire resources (restart after Stop)
	p.mu.Lock()
	hasResources := p.loopPtr != nil && atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&p.streamPtr))) != nil
	p.mu.Unlock()

	if !hasResources {
		// First start — create PipeWire resources
		if err := p.startCreateResourcesAndActivate(); err != nil {
			return err
		}
	} else {
		// Restart — just reactivate existing stream
		streamPtr := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&p.streamPtr)))

		if err := p.streamOps.SetStreamActive(streamPtr, true); err != nil {
			// Activation failed — clean up owned resources so stale pointers
			// are not left behind, then transition to Error.
			p.teardown()
			return p.failWithErrorState("reactivate stream", err)
		}
	}

	// Clear paused flag
	p.setPaused(false)

	// Transition to Playing
	return p.transition(PlayerStatePlaying)
}

// startCreateResourcesAndActivate creates the main loop, playback stream, connects
// and activates it, stores owned resources, and starts the main loop goroutine.
// On failure, all created resources are destroyed and the state transitions to Error.
// No owned pointers are left set on failure.
func (p *player) startCreateResourcesAndActivate() error {
	// Create main loop
	loopPtr, err := p.streamOps.CreateMainLoop()
	if err != nil {
		return p.failWithErrorState("create main loop", err)
	}

	// Create playback stream
	streamPtr, err := p.streamOps.CreatePlaybackStream(loopPtr, "purego-pipewire-player", p.onProcess)
	if err != nil {
		p.streamOps.DestroyMainLoop(loopPtr)
		return p.failWithErrorState("create playback stream", err)
	}

	// Connect stream
	format := portout.PlaybackFormat{
		SampleRate:      p.config.SampleRate,
		Channels:        p.config.Channels,
		FramesPerBuffer: p.config.FramesPerBuffer,
	}
	if err := p.streamOps.ConnectPlaybackStream(streamPtr, format); err != nil {
		p.streamOps.DestroyStream(streamPtr)
		p.streamOps.DestroyMainLoop(loopPtr)
		return p.failWithErrorState("connect playback stream", err)
	}

	// Activate stream
	if err := p.streamOps.SetStreamActive(streamPtr, true); err != nil {
		// Best-effort disconnect before destroying
		_ = p.streamOps.DisconnectStream(streamPtr)
		p.streamOps.DestroyStream(streamPtr)
		p.streamOps.DestroyMainLoop(loopPtr)
		return p.failWithErrorState("activate playback stream", err)
	}

	// All steps succeeded — store owned resources
	loopDone := make(chan struct{})
	p.mu.Lock()
	p.loopPtr = loopPtr
	p.loopDone = loopDone
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&p.streamPtr)), streamPtr)
	p.mu.Unlock()

	// Start main loop in an internal goroutine
	go func() {
		defer close(loopDone)
		if err := p.streamOps.RunMainLoop(loopPtr); err != nil {
			if state := p.State(); state != PlayerStateClosing && state != PlayerStateClosed {
				p.fail(err)
			}
		}
	}()

	return nil
}

// Pause temporarily pauses playback
func (p *player) Pause() error {
	current := p.State()
	if current == PlayerStateClosed || current == PlayerStateClosing {
		return ErrPlayerClosed
	}

	// Only valid from Playing
	if current != PlayerStatePlaying {
		return ErrInvalidPlayerState
	}

	p.setPaused(true)
	return p.transition(PlayerStatePaused)
}

// Stop stops playback but allows restart.
// Returns an error if stream deactivation fails, leaving the state unchanged.
func (p *player) Stop() error {
	current := p.State()
	if current == PlayerStateClosed || current == PlayerStateClosing {
		return ErrPlayerClosed
	}

	// Valid from Playing or Paused.
	if current != PlayerStatePlaying && current != PlayerStatePaused {
		return ErrInvalidPlayerState
	}

	// Deactivate stream - if this fails, return the error and don't transition
	if err := p.deactivateStream(); err != nil {
		return err
	}

	// Clear paused flag only after successful deactivation
	p.setPaused(false)

	return p.transition(PlayerStateStopped)
}

// Close permanently shuts down the player
func (p *player) Close() error {
	current := p.State()

	// Idempotent on already closed
	if current == PlayerStateClosed {
		return nil
	}

	// Transition to Closing if not already there
	if current != PlayerStateClosing {
		if err := p.transition(PlayerStateClosing); err != nil {
			return err
		}
	}

	// Teardown the player-owned stream and main-loop resources.
	p.teardown()

	// Transition to Closed
	return p.transition(PlayerStateClosed)
}

// ensureRuntime ensures the player has a usable StreamOps implementation.
// If streamOps is already set (e.g., by tests), it is used as-is.
// Otherwise, PipeWire is registered via capi.Register(), the default CAPI
// instance is retrieved, and the shared process-level init guard is used
// before obtaining the default StreamOps implementation.
func (p *player) ensureRuntime() error {
	if p.streamOps != nil {
		return nil
	}
	if err := registerPipeWire(); err != nil {
		return fmt.Errorf("register pipewire: %w", err)
	}
	capiInstance := defaultPipeWireCAPI()
	if capiInstance == nil {
		return errors.New("failed to get default CAPI instance")
	}
	if err := initializePipeWire(capiInstance); err != nil {
		return fmt.Errorf("initialize pipewire runtime: %w", err)
	}
	ops := defaultPipeWireOps()
	if ops == nil {
		return errors.New("failed to obtain default stream operations")
	}
	p.streamOps = ops
	return nil
}

func (p *player) failWithErrorState(context string, err error) error {
	if transitionErr := p.transition(PlayerStateError); transitionErr != nil {
		return errors.Join(fmt.Errorf("%s: %w", context, err), fmt.Errorf("transition player to error: %w", transitionErr))
	}
	return fmt.Errorf("%s: %w", context, err)
}

// setPaused sets the paused flag
func (p *player) setPaused(paused bool) {
	p.paused.Store(paused)
}

// deactivateStream deactivates the stream via StreamOps.
// Returns an error if deactivation fails. No-op when streamOps is nil.
func (p *player) deactivateStream() error {
	p.mu.Lock()
	streamOps := p.streamOps
	streamPtr := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&p.streamPtr)))
	p.mu.Unlock()

	if streamOps == nil || streamPtr == nil {
		return nil
	}
	return streamOps.SetStreamActive(streamPtr, false)
}

// teardown releases player-owned stream and main-loop resources through
// StreamOps. Safe to call when fields are nil or on repeated invocation.
// When a main loop exists, DisconnectStream is issued first while the loop is
// still running, then the loop is quit and awaited, and only then is the
// stream destroyed. This ordering avoids destroying a pw_stream while its
// main loop is still active.
func (p *player) teardown() {
	p.mu.Lock()
	streamOps := p.streamOps
	streamPtr := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&p.streamPtr)))
	loopPtr := p.loopPtr
	loopDone := p.loopDone
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&p.streamPtr)), nil)
	p.loopPtr = nil
	p.loopDone = nil
	p.mu.Unlock()

	if streamOps == nil {
		return
	}

	// If a stream exists, disconnect it first while the loop is still alive.
	if streamPtr != nil {
		_ = streamOps.DisconnectStream(streamPtr) // Best-effort cleanup
	}

	// Quit then destroy the main loop.
	// Never destroy the main loop until RunMainLoop has actually returned;
	// destroying while pw_main_loop_run is still active can crash in PipeWire.
	if loopPtr != nil {
		streamOps.QuitMainLoop(loopPtr)
		if loopDone != nil {
			<-loopDone
		}
	}

	// Only destroy the stream after the loop has stopped so pw_stream_destroy
	// can safely flush any queued loop callbacks before freeing stream state.
	if streamPtr != nil {
		streamOps.DestroyStream(streamPtr)
	}
	if loopPtr != nil {
		streamOps.DestroyMainLoop(loopPtr)
	}
}

// Player is the exported wrapper for the internal player
type Player struct {
	p *player
}

// NewPlayer creates a new exported Player instance
func NewPlayer(config playerConfig, callbacks playerCallbacks) *Player {
	return &Player{p: newPlayer(config, callbacks)}
}

// State returns the current player state
func (p *Player) State() PlayerState {
	if p.p == nil {
		return PlayerStateIdle
	}
	return p.p.State()
}

// Start begins playback
func (p *Player) Start() error {
	if p.p == nil {
		return ErrPlayerClosed
	}
	return p.p.Start()
}

// Pause temporarily pauses playback
func (p *Player) Pause() error {
	if p.p == nil {
		return ErrPlayerClosed
	}
	return p.p.Pause()
}

// Stop stops playback but allows restart
func (p *Player) Stop() error {
	if p.p == nil {
		return ErrPlayerClosed
	}
	return p.p.Stop()
}

// Close permanently shuts down the player
func (p *Player) Close() error {
	if p.p == nil {
		return nil
	}
	return p.p.Close()
}

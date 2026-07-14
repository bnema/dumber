package pipewire

import (
	"errors"

	"github.com/bnema/purego-pipewire/internal/core"
)

// SampleFormat represents the audio sample format type.
type SampleFormat int

const (
	// SampleFormatF32 is the 32-bit floating point sample format.
	SampleFormatF32 SampleFormat = iota
)

// UnderrunPolicy defines the behavior when a buffer underrun occurs.
type UnderrunPolicy int

const (
	// UnderrunFillSilence fills the buffer with silence on underrun.
	UnderrunFillSilence UnderrunPolicy = iota
	// UnderrunFail fails playback on underrun.
	UnderrunFail
)

// PlayerState represents the current state of the audio player.
type PlayerState int

const (
	// PlayerStateIdle is the initial state before the player is started.
	PlayerStateIdle PlayerState = iota
	// PlayerStateStarting indicates the player is initializing.
	PlayerStateStarting
	// PlayerStatePlaying indicates the player is actively outputting audio.
	PlayerStatePlaying
	// PlayerStatePaused indicates the player is temporarily stopped.
	PlayerStatePaused
	// PlayerStateStopped indicates the player has been stopped.
	PlayerStateStopped
	// PlayerStateClosing indicates the player is shutting down.
	PlayerStateClosing
	// PlayerStateClosed indicates the player has been fully closed.
	PlayerStateClosed
	// PlayerStateError indicates the player encountered an error.
	PlayerStateError
)

// Legacy aliases for backwards compatibility (deprecated)
const (
	Idle     = PlayerStateIdle
	Starting = PlayerStateStarting
	Playing  = PlayerStatePlaying
	Paused   = PlayerStatePaused
	Stopped  = PlayerStateStopped
	Closing  = PlayerStateClosing
	Closed   = PlayerStateClosed
	Error    = PlayerStateError
)

// PlayerConfig holds the configuration for a new audio player.
type PlayerConfig struct {
	SampleRate      int
	Channels        int
	FramesPerBuffer int
	SampleFormat    SampleFormat
	UnderrunPolicy  UnderrunPolicy
}

// PCMBuffer represents a buffer of PCM audio data.
type PCMBuffer struct {
	Frames   int
	Channels int
	Stride   int
	Samples  [][]float32
}

// PlayerCallbacks contains all callback functions for player events.
type PlayerCallbacks struct {
	Fill          func(*PCMBuffer) (int, error)
	OnUnderrun    func(int)
	OnDrain       func()
	OnError       func(error)
	OnStateChange func(PlayerState)
}

// Player is the public interface for audio playback.
type Player interface {
	Start() error
	Pause() error
	Stop() error
	Close() error
	State() PlayerState
}

// ErrInvalidPlayerConfig is returned when the player configuration is invalid.
var ErrInvalidPlayerConfig = errors.New("invalid player config")

// playerImpl wraps the internal core player
type playerImpl struct {
	config     PlayerConfig
	callbacks  PlayerCallbacks
	internal   *core.Player
	scratchBuf PCMBuffer // reusable buffer for Fill callback to avoid allocation
}

func (p *playerImpl) Start() error {
	if p.internal == nil {
		return errors.New("player not initialized")
	}
	return p.internal.Start()
}

func (p *playerImpl) Pause() error {
	if p.internal == nil {
		return errors.New("player not initialized")
	}
	return p.internal.Pause()
}

func (p *playerImpl) Stop() error {
	if p.internal == nil {
		return errors.New("player not initialized")
	}
	return p.internal.Stop()
}

func (p *playerImpl) Close() error {
	if p.internal == nil {
		return nil
	}
	return p.internal.Close()
}

func (p *playerImpl) State() PlayerState {
	if p.internal == nil {
		return PlayerStateIdle
	}
	return PlayerState(p.internal.State())
}

// NewPlayer creates a new audio player with the given configuration and callbacks.
// Returns ErrInvalidPlayerConfig if the configuration is invalid.
func NewPlayer(config PlayerConfig, callbacks PlayerCallbacks) (Player, error) {
	if config.SampleRate <= 0 {
		return nil, ErrInvalidPlayerConfig
	}
	if config.Channels <= 0 {
		return nil, ErrInvalidPlayerConfig
	}
	if config.FramesPerBuffer <= 0 {
		return nil, ErrInvalidPlayerConfig
	}
	if config.SampleFormat != SampleFormatF32 {
		return nil, ErrInvalidPlayerConfig
	}

	// Convert public UnderrunPolicy to internal UnderrunPolicy
	var coreUnderrunPolicy core.UnderrunPolicy
	switch config.UnderrunPolicy {
	case UnderrunFail:
		coreUnderrunPolicy = core.UnderrunFailFast
	default:
		coreUnderrunPolicy = core.UnderrunFillSilence
	}

	// Create the player implementation early so we can use its scratch buffer
	p := &playerImpl{
		config:    config,
		callbacks: callbacks,
	}

	// Convert public callbacks to internal callbacks
	coreCallbacks := core.PlayerCallbacks{
		OnStateChange: func(state core.PlayerState) {
			if callbacks.OnStateChange != nil {
				callbacks.OnStateChange(PlayerState(state))
			}
		},
		Fill: func(buf *core.PCMBuffer) (int, error) {
			if callbacks.Fill == nil {
				return buf.Frames, nil
			}
			// Reuse scratch buffer by updating its fields
			p.scratchBuf.Frames = buf.Frames
			p.scratchBuf.Channels = buf.Channels
			p.scratchBuf.Stride = buf.Stride
			p.scratchBuf.Samples = buf.Samples
			return callbacks.Fill(&p.scratchBuf)
		},
		OnUnderrun: func(frames int) {
			if callbacks.OnUnderrun != nil {
				callbacks.OnUnderrun(frames)
			}
		},
		OnDrain: func() {
			if callbacks.OnDrain != nil {
				callbacks.OnDrain()
			}
		},
		OnError: func(err error) {
			if callbacks.OnError != nil {
				callbacks.OnError(err)
			}
		},
	}

	// Create internal player with mapped config
	coreConfig := core.PlayerConfig{
		SampleRate:      config.SampleRate,
		FramesPerBuffer: config.FramesPerBuffer,
		Channels:        config.Channels,
		UnderrunPolicy:  coreUnderrunPolicy,
	}
	p.internal = core.NewPlayer(coreConfig, coreCallbacks)

	return p, nil
}

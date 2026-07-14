package core

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"unsafe"
)

// UnderrunPolicy determines how to handle underruns
type UnderrunPolicy int

const (
	UnderrunFillSilence UnderrunPolicy = iota
	UnderrunFailFast
)

// PCMBuffer represents a PCM audio buffer with deinterleaved samples
// Samples[channel][frame] format for cache efficiency
type PCMBuffer struct {
	Frames   int
	Channels int
	Stride   int
	Samples  [][]float32
}

// allocate allocates the Samples slice structure
func (b *PCMBuffer) allocate() {
	if len(b.Samples) != b.Channels {
		b.Samples = make([][]float32, b.Channels)
	}
	for ch := 0; ch < b.Channels; ch++ {
		if len(b.Samples[ch]) != b.Frames {
			b.Samples[ch] = make([]float32, b.Frames)
		}
	}
}

// PlayerConfig holds configuration for the player
type PlayerConfig struct {
	// SampleRate is the playback sample rate used when opening the stream.
	SampleRate      int
	FramesPerBuffer int
	Channels        int
	UnderrunPolicy  UnderrunPolicy
}

// PlayerCallbacks holds callbacks for player events
type PlayerCallbacks struct {
	OnStateChange func(PlayerState)
	Fill          func(*PCMBuffer) (int, error)
	OnUnderrun    func(int)
	OnDrain       func()
	OnError       func(error)
}

// playerConfig is the internal alias for PlayerConfig
type playerConfig = PlayerConfig

// playerCallbacks is the internal alias for PlayerCallbacks
type playerCallbacks = PlayerCallbacks

// processPCM processes PCM audio data for the given buffer
// Returns the number of frames processed, or an error
func (p *player) processPCM(buf *PCMBuffer) (int, error) {
	// Ensure buffer is allocated
	buf.allocate()

	state := p.State()

	// If paused or stopped, fill with silence and return
	if state == PlayerStatePaused || state == PlayerStateStopped || state == PlayerStateIdle {
		p.fillSilence(buf, 0, buf.Frames)
		return buf.Frames, nil
	}

	// If not playing, fill with silence
	if state != PlayerStatePlaying {
		p.fillSilence(buf, 0, buf.Frames)
		return buf.Frames, nil
	}

	// Call Fill callback
	if p.callbacks.Fill == nil {
		p.fillSilence(buf, 0, buf.Frames)
		return buf.Frames, nil
	}

	frames, err := p.callbacks.Fill(buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// Drain condition - emit drain callback and return frames
			p.emitDrain()
			return frames, nil
		}
		// Other error - fail the player
		p.fail(err)
		return 0, err
	}

	// Handle underrun if Fill returned fewer frames than requested
	if frames < buf.Frames {
		return p.handleUnderrun(buf, frames)
	}

	return frames, nil
}

// fillSilence fills the buffer with silence (0.0) from startFrame to endFrame
func (p *player) fillSilence(buf *PCMBuffer, startFrame, endFrame int) {
	if startFrame < 0 {
		startFrame = 0
	}
	if endFrame > buf.Frames {
		endFrame = buf.Frames
	}
	for ch := 0; ch < buf.Channels; ch++ {
		clear(buf.Samples[ch][startFrame:endFrame])
	}
}

// handleUnderrun applies the underrun policy when Fill returns fewer frames than requested
func (p *player) handleUnderrun(buf *PCMBuffer, filledFrames int) (int, error) {
	// Emit underrun callback
	if p.callbacks.OnUnderrun != nil {
		p.callbacks.OnUnderrun(buf.Frames - filledFrames)
	}

	switch p.config.UnderrunPolicy {
	case UnderrunFailFast:
		err := errors.New("underrun occurred")
		p.fail(err)
		return 0, err
	case UnderrunFillSilence:
		fallthrough
	default:
		p.fillSilence(buf, filledFrames, buf.Frames)
		return buf.Frames, nil
	}
}

// emitDrain invokes the OnDrain callback
func (p *player) emitDrain() {
	if p.callbacks.OnDrain != nil {
		p.callbacks.OnDrain()
	}
}

// fail transitions the player to error state and invokes OnError
func (p *player) fail(err error) {
	if transitionErr := p.transition(PlayerStateError); transitionErr != nil {
		err = errors.Join(err, fmt.Errorf("transition player to error: %w", transitionErr))
	}
	if p.callbacks.OnError != nil {
		p.callbacks.OnError(err)
	}
}

// onProcess is the PipeWire process callback. It is invoked by PipeWire
// each time the stream needs more audio data. It dequeues a buffer,
// fills it with PCM data via processPCM, commits the result, and
// queues the buffer back. If any step fails, it routes the error
// through p.fail.
func (p *player) onProcess() {
	// Drop native references before returning from each callback.
	defer p.pwView.clear()

	ops := p.streamOps
	if ops == nil {
		return
	}

	sp := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&p.streamPtr)))
	if sp == nil {
		return
	}

	bufPtr := ops.DequeueBuffer(sp)
	if bufPtr == nil {
		return
	}

	queueOnError := true
	defer func() {
		err := ops.QueueBuffer(sp, bufPtr)
		if err != nil && queueOnError {
			// Clear before OnError can reenter player methods.
			p.pwView.clear()
			p.fail(fmt.Errorf("queue buffer: %w", err))
		}
	}()

	if err := p.pwView.refresh(bufPtr); err != nil {
		queueOnError = false
		p.fail(fmt.Errorf("buffer view: %w", err))
		return
	}

	frames, err := p.processPCM(p.pwView.PCM())
	if err != nil {
		queueOnError = false
		return
	}

	p.pwView.Commit(frames)
}

package input

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

// Mode represents the current input mode.
type Mode int

const (
	// ModeNormal is the default mode where keys pass through to WebView.
	ModeNormal Mode = iota
	// ModeTab is the modal tab management mode.
	ModeTab
	// ModePane is the modal pane management mode.
	ModePane
)

// String returns a human-readable mode name.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeTab:
		return "tab"
	case ModePane:
		return "pane"
	default:
		return "unknown"
	}
}

// ModalState manages the current input mode with optional timeout.
type ModalState struct {
	mode    Mode
	timeout time.Duration
	timer   *time.Timer

	// Callback for mode changes (called synchronously under lock).
	onModeChange func(from, to Mode)

	mu sync.RWMutex
}

// NewModalState creates a new modal state manager.
func NewModalState(ctx context.Context) *ModalState {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating modal state")
	return &ModalState{
		mode: ModeNormal,
	}
}

// Mode returns the current mode (thread-safe).
func (m *ModalState) Mode() Mode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}

// EnterTabMode switches to tab mode with an optional timeout.
// If timeout is 0, the mode stays until explicitly exited.
func (m *ModalState) EnterTabMode(ctx context.Context, timeout time.Duration) {
	log := logging.FromContext(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode == ModeTab {
		// Already in tab mode, just reset timeout
		m.resetTimeoutLocked(ctx, timeout)
		return
	}

	m.cancelTimerLocked()
	oldMode := m.mode
	m.mode = ModeTab
	m.timeout = timeout

	if timeout > 0 {
		m.timer = time.AfterFunc(timeout, func() {
			m.ExitMode(ctx)
		})
	}

	log.Debug().
		Str("from", oldMode.String()).
		Str("to", m.mode.String()).
		Dur("timeout", timeout).
		Msg("entered tab mode")

	if m.onModeChange != nil {
		m.onModeChange(oldMode, m.mode)
	}
}

// EnterPaneMode switches to pane mode with an optional timeout.
// If timeout is 0, the mode stays until explicitly exited.
func (m *ModalState) EnterPaneMode(ctx context.Context, timeout time.Duration) {
	log := logging.FromContext(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode == ModePane {
		// Already in pane mode, just reset timeout
		m.resetTimeoutLocked(ctx, timeout)
		return
	}

	m.cancelTimerLocked()
	oldMode := m.mode
	m.mode = ModePane
	m.timeout = timeout

	if timeout > 0 {
		m.timer = time.AfterFunc(timeout, func() {
			m.ExitMode(ctx)
		})
	}

	log.Debug().
		Str("from", oldMode.String()).
		Str("to", m.mode.String()).
		Dur("timeout", timeout).
		Msg("entered pane mode")

	if m.onModeChange != nil {
		m.onModeChange(oldMode, m.mode)
	}
}

// ExitMode returns to normal mode.
func (m *ModalState) ExitMode(ctx context.Context) {
	log := logging.FromContext(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode == ModeNormal {
		return
	}

	m.cancelTimerLocked()
	oldMode := m.mode
	m.mode = ModeNormal
	m.timeout = 0

	log.Debug().
		Str("from", oldMode.String()).
		Str("to", "normal").
		Msg("exited modal mode")

	if m.onModeChange != nil {
		m.onModeChange(oldMode, ModeNormal)
	}
}

// ResetTimeout restarts the mode timeout (e.g., after a valid keystroke).
// Does nothing if not in a modal mode or if no timeout was set.
func (m *ModalState) ResetTimeout(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode == ModeNormal || m.timeout == 0 {
		return
	}

	m.resetTimeoutLocked(ctx, m.timeout)
}

// SetOnModeChange sets the callback for mode changes.
// The callback is invoked synchronously under the lock.
func (m *ModalState) SetOnModeChange(fn func(from, to Mode)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onModeChange = fn
}

// cancelTimerLocked stops any active timeout timer.
// Must be called with m.mu held.
func (m *ModalState) cancelTimerLocked() {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}

// resetTimeoutLocked resets the timeout timer.
// Must be called with m.mu held.
func (m *ModalState) resetTimeoutLocked(ctx context.Context, timeout time.Duration) {
	m.cancelTimerLocked()
	m.timeout = timeout
	if timeout > 0 {
		m.timer = time.AfterFunc(timeout, func() {
			m.ExitMode(ctx)
		})
	}
}

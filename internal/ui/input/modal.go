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
	// ModeSession is the modal session management mode.
	ModeSession
	// ModeResize is the modal pane resizing mode.
	ModeResize
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
	case ModeSession:
		return "session"
	case ModeResize:
		return "resize"
	default:
		return "unknown"
	}
}

// DisplayName returns the mode name for display in UI (e.g., toaster).
func (m Mode) DisplayName() string {
	switch m {
	case ModeTab:
		return "TAB MODE"
	case ModePane:
		return "PANE MODE"
	case ModeSession:
		return "SESSION MODE"
	case ModeResize:
		return "RESIZE MODE"
	default:
		return ""
	}
}

// ModalState manages the current input mode with optional timeout.
type ModalState struct {
	mode     Mode
	timeout  time.Duration
	timer    *time.Timer
	timerGen int64 // incremented each time a timer is started; used to ignore stale callbacks

	// Callback for mode changes (called synchronously under lock).
	onModeChange func(from, to Mode)

	// scheduleOnMainThread dispatches a function to the GTK main thread.
	// Timer callbacks use this so that onModeChange (which may make GTK
	// calls like switching controller phase) always runs on the GTK thread.
	// Defaults to direct execution (suitable for tests without a GTK loop).
	// The app sets this to a glib.IdleAdd wrapper via SetMainThreadScheduler.
	scheduleOnMainThread func(fn func())

	mu sync.RWMutex
}

// NewModalState creates a new modal state manager.
func NewModalState(ctx context.Context) *ModalState {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating modal state")
	return &ModalState{
		mode:                 ModeNormal,
		scheduleOnMainThread: func(fn func()) { fn() },
	}
}

// SetMainThreadScheduler sets the function used to dispatch timer callbacks
// to the GTK main thread. In the real app this should be a glib.IdleAdd
// wrapper. In tests, the default (direct execution) is used.
func (m *ModalState) SetMainThreadScheduler(fn func(func())) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduleOnMainThread = fn
}

// Mode returns the current mode (thread-safe).
func (m *ModalState) Mode() Mode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}

// EnterTabMode switches to tab mode with an optional timeout.
func (m *ModalState) EnterTabMode(ctx context.Context, timeout time.Duration) {
	m.enterMode(ctx, ModeTab, timeout)
}

// EnterPaneMode switches to pane mode with an optional timeout.
func (m *ModalState) EnterPaneMode(ctx context.Context, timeout time.Duration) {
	m.enterMode(ctx, ModePane, timeout)
}

// EnterSessionMode switches to session mode with an optional timeout.
func (m *ModalState) EnterSessionMode(ctx context.Context, timeout time.Duration) {
	m.enterMode(ctx, ModeSession, timeout)
}

// EnterResizeMode switches to resize mode with an optional timeout.
func (m *ModalState) EnterResizeMode(ctx context.Context, timeout time.Duration) {
	m.enterMode(ctx, ModeResize, timeout)
}

// enterMode is the shared implementation for all mode-enter methods.
// If already in the target mode, it resets the timeout instead.
func (m *ModalState) enterMode(ctx context.Context, target Mode, timeout time.Duration) {
	log := logging.FromContext(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode == target {
		m.resetTimeoutLocked(ctx, timeout)
		return
	}

	m.cancelTimerLocked()
	oldMode := m.mode
	m.mode = target
	m.timeout = timeout

	if timeout > 0 {
		m.startTimeoutLocked(ctx, timeout)
	}

	log.Debug().
		Str("from", oldMode.String()).
		Str("to", target.String()).
		Dur("timeout", timeout).
		Msg("entered " + target.String() + " mode")

	if m.onModeChange != nil {
		m.onModeChange(oldMode, target)
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

// startTimeoutLocked starts a new timeout timer that exits the mode via the
// main thread scheduler. Timer goroutines must not call ExitMode directly
// because onModeChange may make GTK calls (e.g., switching controller phase).
// Must be called with m.mu held.
func (m *ModalState) startTimeoutLocked(ctx context.Context, timeout time.Duration) {
	m.timerGen++
	gen := m.timerGen
	schedule := m.scheduleOnMainThread
	m.timer = time.AfterFunc(timeout, func() {
		schedule(func() {
			// Ignore stale callback: time.Timer.Stop can race with the
			// timer firing, so a canceled timer's callback may still run.
			m.mu.RLock()
			stale := m.timerGen != gen
			m.mu.RUnlock()
			if stale {
				return
			}
			m.ExitMode(ctx)
		})
	})
}

// resetTimeoutLocked resets the timeout timer.
// Must be called with m.mu held.
func (m *ModalState) resetTimeoutLocked(ctx context.Context, timeout time.Duration) {
	m.cancelTimerLocked()
	m.timeout = timeout
	if timeout > 0 {
		m.startTimeoutLocked(ctx, timeout)
	}
}

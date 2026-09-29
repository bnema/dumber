package ui

import (
	"sync"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

// vimModeState holds the app-wide Vim Mode bookkeeping that is not tied to a
// single browser window: which panes report an editable page element focused,
// and the pulse debounce clock. Per-window ownership (the accented pane) stays
// on browserWindow.vimModePaneID.
type vimModeState struct {
	editableFocus map[entity.PaneID]bool

	pulseMu   sync.Mutex
	pulseLast time.Time
}

func (s *vimModeState) editableFocused(paneID entity.PaneID) bool {
	return paneID != "" && s.editableFocus[paneID]
}

func (s *vimModeState) setEditableFocused(paneID entity.PaneID, editable bool) {
	if paneID == "" {
		return
	}
	if !editable {
		delete(s.editableFocus, paneID)
		return
	}
	if s.editableFocus == nil {
		s.editableFocus = make(map[entity.PaneID]bool)
	}
	s.editableFocus[paneID] = true
}

// allowPulse reports whether a pulse may run at now, and records it when so.
// Pulses closer than vimModePulseInterval are skipped so held-key repeats do
// not restart CSS animations at scroll cadence.
func (s *vimModeState) allowPulse(now time.Time) bool {
	s.pulseMu.Lock()
	defer s.pulseMu.Unlock()
	if now.Sub(s.pulseLast) < vimModePulseInterval {
		return false
	}
	s.pulseLast = now
	return true
}

// resetPulse clears the debounce so the first pulse after re-entering Vim
// Mode is never skipped.
func (s *vimModeState) resetPulse() {
	s.pulseMu.Lock()
	s.pulseLast = time.Time{}
	s.pulseMu.Unlock()
}

func (s *vimModeState) pulseIdle() bool {
	s.pulseMu.Lock()
	defer s.pulseMu.Unlock()
	return s.pulseLast.IsZero()
}

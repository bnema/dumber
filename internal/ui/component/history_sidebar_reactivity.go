package component

import (
	"time"

	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
)

type relativeTimeLabelBinding struct {
	label       *gtk.Label
	lastVisited time.Time
}

func (hs *HistorySidebar) RequestReloadIfVisible(reason string) {
	_ = reason
	if hs == nil {
		return
	}
	hs.mu.RLock()
	if hs.destroyed || !hs.visible {
		hs.mu.RUnlock()
		return
	}
	hs.mu.RUnlock()
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.requestReloadIfVisibleOnMain()
		return false
	})
	hs.scheduleIdle(cb)
}

func (hs *HistorySidebar) requestReloadIfVisibleOnMain() {
	hs.mu.Lock()
	if hs.destroyed || !hs.visible {
		hs.mu.Unlock()
		return
	}
	oldTimer := hs.reloadDebounceTimer
	hs.reloadDebounceTimer = 0
	hs.mu.Unlock()

	if oldTimer != 0 {
		hs.removeSource(oldTimer)
	}

	var timerID uint
	reloadCb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.Lock()
		if hs.reloadDebounceTimer != timerID {
			hs.mu.Unlock()
			return false
		}
		hs.reloadDebounceTimer = 0
		if hs.destroyed || !hs.visible {
			hs.mu.Unlock()
			return false
		}
		hs.mu.Unlock()
		hs.Reload()
		return false
	})
	timerID = hs.addTimeout(sidebarReloadDebounceMs, reloadCb)

	hs.mu.Lock()
	if hs.destroyed || !hs.visible {
		hs.mu.Unlock()
		if timerID != 0 {
			hs.removeSource(timerID)
		}
		return
	}
	hs.reloadDebounceTimer = timerID
	hs.mu.Unlock()
}

func (hs *HistorySidebar) cancelReloadDebounceLocked() uint {
	timerID := hs.reloadDebounceTimer
	hs.reloadDebounceTimer = 0
	return timerID
}

func (hs *HistorySidebar) bindRelativeTimeLabel(label *gtk.Label, visited time.Time) {
	if label == nil {
		return
	}
	hs.mu.Lock()
	if !hs.destroyed {
		hs.relativeTimeLabelBinds = append(hs.relativeTimeLabelBinds, relativeTimeLabelBinding{label: label, lastVisited: visited})
	}
	hs.mu.Unlock()
}

func (hs *HistorySidebar) clearRelativeTimeBindingsLocked() {
	hs.relativeTimeLabelBinds = nil
}

func (hs *HistorySidebar) startRelativeTimeTicker() {
	hs.mu.Lock()
	if hs.destroyed || !hs.visible {
		hs.mu.Unlock()
		return
	}
	oldTicker := hs.relativeTimeTicker
	hs.relativeTimeTicker = 0
	hs.relativeTimeDayKey = hs.currentDayKey()
	hs.relativeTimeDayKeySet = true
	hs.mu.Unlock()

	if oldTicker != 0 {
		hs.removeSource(oldTicker)
	}

	hs.scheduleRelativeTimeRefresh()

	var tickerID uint
	tickCb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.RLock()
		keep := !hs.destroyed && hs.visible && hs.relativeTimeTicker == tickerID
		hs.mu.RUnlock()
		if !keep {
			return false
		}
		hs.scheduleRelativeTimeRefresh()
		return true
	})
	tickerID = hs.addTimeout(sidebarRelativeTickMs, tickCb)

	hs.mu.Lock()
	if hs.destroyed || !hs.visible {
		hs.mu.Unlock()
		if tickerID != 0 {
			hs.removeSource(tickerID)
		}
		return
	}
	hs.relativeTimeTicker = tickerID
	hs.mu.Unlock()
}

func (hs *HistorySidebar) scheduleRelativeTimeRefresh() {
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.updateRelativeTimeLabelsOnMain()
		return false
	})
	hs.scheduleIdle(cb)
}

func (hs *HistorySidebar) updateRelativeTimeLabelsOnMain() {
	now := hs.currentTime()
	hs.mu.RLock()
	if hs.destroyed || !hs.visible {
		hs.mu.RUnlock()
		return
	}
	currentDay := dayKeyFromTime(now)
	lastDay := hs.relativeTimeDayKey
	daySet := hs.relativeTimeDayKeySet
	bindings := append([]relativeTimeLabelBinding(nil), hs.relativeTimeLabelBinds...)
	hs.mu.RUnlock()

	if daySet && currentDay != lastDay {
		shouldReload := false
		hs.mu.Lock()
		if !hs.destroyed && hs.visible && hs.relativeTimeDayKeySet && hs.relativeTimeDayKey != currentDay {
			hs.relativeTimeDayKey = currentDay
			shouldReload = true
		}
		hs.mu.Unlock()
		if shouldReload {
			hs.RequestReloadIfVisible("day-boundary")
		}
		return
	}

	for _, binding := range bindings {
		if binding.label == nil {
			continue
		}
		binding.label.SetText(relativeTimeAt(binding.lastVisited, now))
	}
}

func (hs *HistorySidebar) currentTime() time.Time {
	if hs != nil && hs.now != nil {
		return hs.now()
	}
	return time.Now()
}

func (hs *HistorySidebar) currentDayKey() dayKey {
	return dayKeyFromTime(hs.currentTime())
}

func dayKeyFromTime(t time.Time) dayKey {
	return dayKey{year: t.Year(), month: t.Month(), day: t.Day()}
}

func (hs *HistorySidebar) addTimeout(ms uint, cb glib.SourceFunc) uint {
	if hs != nil && hs.timeoutAdd != nil {
		return hs.timeoutAdd(ms, cb)
	}
	return glib.TimeoutAdd(ms, &cb, 0)
}

func (hs *HistorySidebar) removeSource(id uint) {
	if id == 0 {
		return
	}
	if hs != nil && hs.sourceRemove != nil {
		hs.sourceRemove(id)
		return
	}
	glib.SourceRemove(id)
}

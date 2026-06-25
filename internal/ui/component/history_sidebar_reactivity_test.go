package component

import (
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sidebarReactivityHarness struct {
	hs      *HistorySidebar
	idle    []glib.SourceFunc
	timers  map[uint]glib.SourceFunc
	removed []uint
	nextID  uint
	idleMu  sync.Mutex
	timerMu sync.Mutex
}

func newSidebarReactivityHarness() *sidebarReactivityHarness {
	h := &sidebarReactivityHarness{timers: make(map[uint]glib.SourceFunc), nextID: 10}
	hs := newTestSidebarSearchHarness()
	hs.now = func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC) }
	hs.idleScheduler = func(cb glib.SourceFunc) {
		h.idleMu.Lock()
		defer h.idleMu.Unlock()
		h.idle = append(h.idle, cb)
	}
	hs.timeoutAdd = func(_ uint, cb glib.SourceFunc) uint {
		h.timerMu.Lock()
		defer h.timerMu.Unlock()
		h.nextID++
		h.timers[h.nextID] = cb
		return h.nextID
	}
	hs.sourceRemove = func(id uint) {
		h.timerMu.Lock()
		defer h.timerMu.Unlock()
		h.removed = append(h.removed, id)
		delete(h.timers, id)
	}
	h.hs = hs
	return h
}

func (h *sidebarReactivityHarness) runIdle() int {
	h.idleMu.Lock()
	callbacks := append([]glib.SourceFunc(nil), h.idle...)
	h.idle = nil
	h.idleMu.Unlock()
	for _, cb := range callbacks {
		cb(0)
	}
	return len(callbacks)
}

func (h *sidebarReactivityHarness) singleTimer(t *testing.T) (uint, glib.SourceFunc) {
	t.Helper()
	h.timerMu.Lock()
	defer h.timerMu.Unlock()
	require.Len(t, h.timers, 1)
	for id, cb := range h.timers {
		return id, cb
	}
	return 0, nil
}

func TestHistorySidebar_RequestReloadIfVisible_HiddenNoop(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.RequestReloadIfVisible("hidden")
	require.Zero(t, h.runIdle())
	assert.Empty(t, h.timers)
	assert.Zero(t, h.hs.reloadDebounceTimer)
}

func TestHistorySidebar_RequestReloadIfVisible_CoalescesVisible(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.visible = true
	h.hs.RequestReloadIfVisible("a")
	h.hs.RequestReloadIfVisible("b")
	require.Equal(t, 2, h.runIdle())
	assert.Len(t, h.timers, 1)
	assert.Len(t, h.removed, 1, "first debounce source should be removed")
	assert.NotZero(t, h.hs.reloadDebounceTimer)
}

func TestHistorySidebar_ReloadDebounceCallbackReloadsOnceAndClearsTimer(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.visible = true
	h.hs.RequestReloadIfVisible("history-change")
	require.Equal(t, 1, h.runIdle())
	timerID, cb := h.singleTimer(t)
	require.Equal(t, timerID, h.hs.reloadDebounceTimer)

	keep := cb(0)

	assert.False(t, keep, "debounce timeout should be one-shot")
	assert.Zero(t, h.hs.reloadDebounceTimer)
	assert.True(t, h.hs.loadStarted, "debounce should call Reload and start a history load")
}

func TestHistorySidebar_ReloadDebounceCallbackSkipsHiddenSidebar(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.visible = true
	h.hs.RequestReloadIfVisible("history-change")
	require.Equal(t, 1, h.runIdle())
	_, cb := h.singleTimer(t)
	h.hs.visible = false

	keep := cb(0)

	assert.False(t, keep, "debounce timeout should be one-shot")
	assert.Zero(t, h.hs.reloadDebounceTimer)
	assert.False(t, h.hs.loadStarted, "hidden sidebar should not reload when debounce fires")
}

func TestHistorySidebar_ReloadDebounceCallbackIgnoresSupersededTimer(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.visible = true
	h.hs.RequestReloadIfVisible("first")
	require.Equal(t, 1, h.runIdle())
	firstID, staleCB := h.singleTimer(t)

	h.hs.RequestReloadIfVisible("second")
	require.Equal(t, 1, h.runIdle())
	currentID, currentCB := h.singleTimer(t)
	require.NotEqual(t, firstID, currentID)
	require.Equal(t, currentID, h.hs.reloadDebounceTimer)

	keep := staleCB(0)

	assert.False(t, keep, "stale debounce timeout should be one-shot")
	assert.Equal(t, currentID, h.hs.reloadDebounceTimer)
	assert.False(t, h.hs.loadStarted, "superseded debounce should not reload")

	keep = currentCB(0)
	assert.False(t, keep)
	assert.Zero(t, h.hs.reloadDebounceTimer)
	assert.True(t, h.hs.loadStarted, "current debounce should still reload")
}

func TestHistorySidebar_DestroyCancelsReloadTickerAndClearsBindings(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.visible = true
	h.hs.reloadDebounceTimer = 51
	h.hs.relativeTimeTicker = 52
	h.hs.relativeTimeLabelBinds = []relativeTimeLabelBinding{{lastVisited: h.hs.currentTime()}}

	h.hs.Destroy()

	h.hs.mu.RLock()
	destroyed := h.hs.destroyed
	reloadTimer := h.hs.reloadDebounceTimer
	ticker := h.hs.relativeTimeTicker
	bindings := h.hs.relativeTimeLabelBinds
	h.hs.mu.RUnlock()
	assert.True(t, destroyed)
	assert.Zero(t, reloadTimer)
	assert.Zero(t, ticker)
	assert.Empty(t, bindings)
	assert.Contains(t, h.removed, uint(51))
	assert.Contains(t, h.removed, uint(52))
}

func TestHistorySidebar_HideCancelsReloadTickerAndClearsBindingsWithoutDestroying(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.visible = true
	h.hs.reloadDebounceTimer = 61
	h.hs.relativeTimeTicker = 62
	h.hs.relativeTimeLabelBinds = []relativeTimeLabelBinding{{lastVisited: h.hs.currentTime()}}

	h.hs.Hide()

	h.hs.mu.RLock()
	destroyed := h.hs.destroyed
	visible := h.hs.visible
	reloadTimer := h.hs.reloadDebounceTimer
	ticker := h.hs.relativeTimeTicker
	bindings := h.hs.relativeTimeLabelBinds
	h.hs.mu.RUnlock()
	assert.False(t, destroyed)
	assert.False(t, visible)
	assert.Zero(t, reloadTimer)
	assert.Zero(t, ticker)
	assert.Empty(t, bindings)
	assert.Contains(t, h.removed, uint(61))
	assert.Contains(t, h.removed, uint(62))
}

func TestHistorySidebar_RelativeTimeBindingsLifecycleAndTicker(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.visible = true
	h.hs.bindRelativeTimeLabel(nil, h.hs.currentTime())
	assert.Empty(t, h.hs.relativeTimeLabelBinds, "nil labels are skipped")
	h.hs.relativeTimeLabelBinds = []relativeTimeLabelBinding{{lastVisited: h.hs.currentTime().Add(-time.Minute)}}
	h.hs.mu.Lock()
	h.hs.clearRelativeTimeBindingsLocked()
	h.hs.mu.Unlock()
	assert.Empty(t, h.hs.relativeTimeLabelBinds)

	h.hs.startRelativeTimeTicker()
	assert.NotZero(t, h.hs.relativeTimeTicker)
	assert.Len(t, h.timers, 1)
	assert.Equal(t, 1, h.runIdle(), "initial label refresh is scheduled through idle path")

	ticker := h.hs.relativeTimeTicker
	h.hs.visible = false
	h.hs.relativeTimeTicker = 0
	h.hs.removeSource(ticker)
	assert.Contains(t, h.removed, ticker)
}

func TestHistorySidebar_RelativeTimeTickerIgnoresSupersededTimer(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.visible = true

	h.hs.startRelativeTimeTicker()
	firstID, staleCB := h.singleTimer(t)
	require.Equal(t, 1, h.runIdle(), "clear initial refresh")

	h.hs.startRelativeTimeTicker()
	currentID, currentCB := h.singleTimer(t)
	require.NotEqual(t, firstID, currentID)
	require.Equal(t, currentID, h.hs.relativeTimeTicker)
	require.Equal(t, 1, h.runIdle(), "clear replacement initial refresh")

	keep := staleCB(0)

	assert.False(t, keep, "stale ticker should stop")
	assert.Equal(t, currentID, h.hs.relativeTimeTicker)
	assert.Zero(t, h.runIdle(), "stale ticker should not schedule refresh")

	keep = currentCB(0)
	assert.True(t, keep, "current ticker should keep running while visible")
	assert.Equal(t, 1, h.runIdle(), "current ticker schedules refresh")
}

func TestHistorySidebar_RelativeTimeDayBoundaryRequestsReloadNoRepoCall(t *testing.T) {
	h := newSidebarReactivityHarness()
	day1 := time.Date(2026, 6, 24, 23, 59, 0, 0, time.UTC)
	day2 := day1.Add(2 * time.Minute)
	now := day1
	h.hs.now = func() time.Time { return now }
	h.hs.visible = true
	h.hs.relativeTimeDayKey = h.hs.currentDayKey()
	h.hs.relativeTimeDayKeySet = true
	h.hs.relativeTimeLabelBinds = []relativeTimeLabelBinding{{lastVisited: day1.Add(-time.Hour)}}
	now = day2
	h.hs.updateRelativeTimeLabelsOnMain()
	assert.False(t, h.hs.loadStarted, "day boundary should not reload synchronously")
	assert.Equal(t, 1, h.runIdle(), "day boundary schedules debounced reload via RequestReloadIfVisible")
	assert.False(t, h.hs.loadStarted, "day boundary should wait for debounce before loading")
	assert.Equal(t, h.hs.currentDayKey(), h.hs.relativeTimeDayKey)
	assert.Len(t, h.timers, 1)
}

func TestRelativeTimeAtDeterministic(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, "1m ago", relativeTimeAt(now.Add(-time.Minute), now))
	assert.Equal(t, "2h ago", relativeTimeAt(now.Add(-2*time.Hour), now))
}

func TestHistorySidebar_RelativeRefreshDoesNotTouchHistoryRepo(t *testing.T) {
	h := newSidebarReactivityHarness()
	h.hs.historyUC = nil
	h.hs.visible = true
	h.hs.relativeTimeLabelBinds = []relativeTimeLabelBinding{{lastVisited: h.hs.currentTime().Add(-time.Minute)}}
	h.hs.updateRelativeTimeLabelsOnMain()
	assert.Empty(t, h.timers, "label-only refresh does not schedule reload/repo work without day boundary")
}

func TestHistorySidebar_ReloadStillPreservesDisplayRowsSemantics(t *testing.T) {
	hs := newTestSidebarSearchHarness()
	entry := &entity.HistoryEntry{URL: "https://example.com", LastVisited: time.Now()}
	hs.setDisplayGroupsLocked(groupHistoryByDay([]*entity.HistoryEntry{entry}))
	require.Len(t, hs.displayRows, 2)
	assert.Equal(t, entry.URL, hs.entryURLAtIndex(1))
}

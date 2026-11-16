package cef

import (
	"time"

	"github.com/bnema/puregotk/v4/glib"
)

const (
	navigationFrameKickTimeout        = 5 * time.Second
	navigationFrameFallbackIntervalMS = 25
	navigationFrameTickStaleAfter     = 50 * time.Millisecond
)

// All state is guarded by WebView.mu; GTK resources are manipulated only on GTK.
type navigationFrameKick struct {
	deadline time.Time
	lastTick time.Time
	paints   int
}

func (k navigationFrameKick) active(now time.Time, loading bool, paints int) bool {
	return !k.deadline.IsZero() && now.Before(k.deadline) && (loading || paints <= k.paints)
}

func (k navigationFrameKick) fallbackDue(now time.Time) bool {
	return k.lastTick.IsZero() || now.Sub(k.lastTick) >= navigationFrameTickStaleAfter
}

func (wv *WebView) sendExternalBeginFrame() {
	if wv.destroyed.Load() {
		return
	}
	wv.mu.RLock()
	host, bridge := wv.host, wv.viewBridge
	wv.mu.RUnlock()
	if host == nil {
		return
	}
	host.SendExternalBeginFrame()
	if bridge != nil {
		bridge.RecordExternalBeginFrameSent()
	}
	wv.mu.Lock()
	wv.lastExternalBeginFrame = time.Now()
	wv.mu.Unlock()
}

// kickNavigationFrame runs on GTK, including when invoked by CEF load handlers.
func (wv *WebView) kickNavigationFrame() {
	if wv.destroyed.Load() || !externalBeginFrameEnabled() {
		return
	}
	wv.mu.RLock()
	bridge, host := wv.viewBridge, wv.host
	wv.mu.RUnlock()
	if bridge == nil || host == nil {
		return
	}
	paints := bridge.Diagnostics().AcceleratedPaints
	wv.mu.Lock()
	wv.beginFrameKick = navigationFrameKick{deadline: time.Now().Add(navigationFrameKickTimeout), paints: paints}
	wv.mu.Unlock()
	wv.startBeginFrameLoop()
	wv.sendExternalBeginFrame()
	if !wv.refreshNavigationFrameKick() {
		return
	}
	wv.mu.RLock()
	hasFallback := wv.beginFrameFallbackID != 0
	wv.mu.RUnlock()
	if hasFallback {
		return
	}
	cb := new(glib.SourceFunc)
	*cb = func(_ uintptr) bool {
		if !wv.refreshNavigationFrameKick() {
			return false
		}
		wv.mu.RLock()
		due := wv.beginFrameKick.fallbackDue(time.Now())
		wv.mu.RUnlock()
		if due {
			wv.sendExternalBeginFrame()
		}
		return true
	}
	id := glib.TimeoutAdd(navigationFrameFallbackIntervalMS, cb, 0)
	wv.mu.Lock()
	wv.beginFrameFallback, wv.beginFrameFallbackID = cb, id
	wv.mu.Unlock()
	if id == 0 {
		wv.stopNavigationFrameKick()
	}
}

func (wv *WebView) refreshNavigationFrameKick() bool {
	wv.mu.RLock()
	bridge, host := wv.viewBridge, wv.host
	kick, loading := wv.beginFrameKick, wv.isLoading
	clock := wv.beginFrameClock
	idle := kick.deadline.IsZero() && clock == nil && wv.beginFrameFallbackID == 0
	wv.mu.RUnlock()
	if idle {
		// Fast path for every steady-state tick: no kick, nothing to release.
		return false
	}
	paints := 0
	if bridge != nil {
		paints = bridge.Diagnostics().AcceleratedPaints
	}
	if wv.destroyed.Load() || host == nil || !kick.active(time.Now(), loading, paints) {
		wv.stopNavigationFrameKick()
		return false
	}
	if clock == nil && bridge != nil {
		if widget := bridge.Widget(); widget != nil {
			if clock = widget.GetFrameClock(); clock != nil {
				clock.Ref()
				clock.BeginUpdating()
				wv.mu.Lock()
				wv.beginFrameClock = clock
				wv.mu.Unlock()
			}
		}
	}
	return true
}

// Safe to repeat, including after the widget has already gone away.
func (wv *WebView) stopNavigationFrameKick() {
	wv.mu.Lock()
	clock, id, cb := wv.beginFrameClock, wv.beginFrameFallbackID, wv.beginFrameFallback
	wv.beginFrameClock, wv.beginFrameFallbackID, wv.beginFrameFallback = nil, 0, nil
	wv.beginFrameKick = navigationFrameKick{}
	wv.mu.Unlock()
	if clock != nil {
		clock.EndUpdating()
		clock.Unref()
	}
	if id != 0 {
		glib.SourceRemove(id)
	}
	if cb != nil {
		_ = unrefTickCallback(cb)
	}
}

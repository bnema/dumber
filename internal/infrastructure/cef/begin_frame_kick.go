package cef

import (
	"time"

	"bnema.dev/puregotk/v4/gdk"
	"bnema.dev/puregotk/v4/glib"
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
	// GLib owns cb through its source registry; removing the source releases it.
	id := glib.TimeoutAdd(navigationFrameFallbackIntervalMS, cb, 0)
	wv.mu.Lock()
	wv.beginFrameFallbackID = id
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
	if wv.destroyed.Load() || host == nil || bridge == nil {
		wv.stopNavigationFrameKick()
		return false
	}
	paints := kick.paints // While loading the paint count cannot end the kick.
	if !loading {
		paints = bridge.Diagnostics().AcceleratedPaints
	}
	if !kick.active(time.Now(), loading, paints) {
		wv.stopNavigationFrameKick()
		return false
	}
	wv.holdNavigationFrameClock(bridge, clock)
	return true
}

// holdNavigationFrameClock keeps the widget's current frame clock in
// BeginUpdating, following clock changes after unrealize or reparenting.
func (wv *WebView) holdNavigationFrameClock(bridge *Cef2gtkAdapter, held *gdk.FrameClock) {
	widget := bridge.Widget()
	var current *gdk.FrameClock
	if widget != nil {
		current = widget.GetFrameClock() // Returns a new reference.
	}
	if current != nil && held != nil && current.Ptr == held.Ptr {
		current.Unref()
		return
	}
	if current != nil {
		current.BeginUpdating()
	}
	wv.mu.Lock()
	wv.beginFrameClock = current
	wv.mu.Unlock()
	if held != nil {
		held.EndUpdating()
		held.Unref()
	}
}

// Safe to repeat, including after the widget has already gone away.
func (wv *WebView) stopNavigationFrameKick() {
	wv.mu.Lock()
	clock, id := wv.beginFrameClock, wv.beginFrameFallbackID
	wv.beginFrameClock, wv.beginFrameFallbackID = nil, 0
	wv.beginFrameKick = navigationFrameKick{}
	wv.mu.Unlock()
	if clock != nil {
		clock.EndUpdating()
		clock.Unref()
	}
	if id != 0 {
		glib.SourceRemove(id)
	}
}

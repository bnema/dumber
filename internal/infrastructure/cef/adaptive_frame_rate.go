package cef

import (
	"strings"

	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/logging"
)

const adaptiveFrameRatePollIntervalMS = 1000

func adaptiveFrameRateForRefresh(refreshRateMilliHz int, fallbackFPS, maxFPS int32) int32 {
	return cef2gtk.WindowlessFrameRateForMonitorRefresh(refreshRateMilliHz, cef2gtk.RefreshRateOptions{
		DefaultFPS: fallbackFPS,
		MinFPS:     fallbackFPS,
		MaxFPS:     maxFPS,
	})
}

func (wv *WebView) scheduleStartAdaptiveFrameRatePolling() {
	if wv == nil || !wv.adaptiveWindowlessFrameRate {
		return
	}
	wv.runOnGTK(func() { wv.startAdaptiveFrameRatePolling() })
}

func (wv *WebView) scheduleStopAdaptiveFrameRatePolling() {
	if wv == nil {
		return
	}
	wv.runOnGTK(func() { wv.stopAdaptiveFrameRatePolling() })
}

func (wv *WebView) startAdaptiveFrameRatePolling() {
	if wv == nil || wv.destroyed.Load() || !wv.adaptiveWindowlessFrameRate || wv.viewBridge == nil || wv.viewBridge.Widget() == nil {
		return
	}

	wv.mu.Lock()
	if wv.adaptiveFrameRatePollID != 0 || wv.host == nil {
		wv.mu.Unlock()
		return
	}
	cb := new(glib.SourceFunc)
	*cb = func(uintptr) bool {
		if wv.destroyed.Load() {
			return false
		}
		wv.applyAdaptiveWindowlessFrameRate()
		return true
	}
	wv.adaptiveFrameRatePoll = cb
	wv.adaptiveFrameRatePollID = glib.TimeoutAdd(adaptiveFrameRatePollIntervalMS, cb, 0)
	wv.mu.Unlock()

	wv.applyAdaptiveWindowlessFrameRate()
}

func (wv *WebView) stopAdaptiveFrameRatePolling() {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	pollID := wv.adaptiveFrameRatePollID
	wv.adaptiveFrameRatePollID = 0
	wv.adaptiveFrameRatePoll = nil
	wv.mu.Unlock()
	if pollID != 0 {
		glib.SourceRemove(pollID)
	}
}

func (wv *WebView) applyAdaptiveWindowlessFrameRate() {
	if wv == nil || !wv.adaptiveWindowlessFrameRate || wv.viewBridge == nil {
		return
	}
	widget := wv.viewBridge.Widget()
	refreshRateMilliHz, displayName, ok := widgetMonitorRefreshRateMilliHz(widget)
	if !ok || !isWaylandDisplayName(displayName) {
		return
	}
	fps := adaptiveFrameRateForRefresh(refreshRateMilliHz, wv.windowlessFrameRate, wv.windowlessFrameRateMax)

	wv.mu.Lock()
	host := wv.host
	if host == nil || fps == wv.lastAdaptiveFrameRate {
		wv.mu.Unlock()
		return
	}
	old := wv.lastAdaptiveFrameRate
	wv.lastAdaptiveFrameRate = fps
	wv.mu.Unlock()

	host.SetWindowlessFrameRate(fps)
	if wv.ctx != nil {
		logging.FromContext(wv.ctx).Info().
			Int32("old_windowless_frame_rate", old).
			Int32("windowless_frame_rate", fps).
			Int("monitor_refresh_millihz", refreshRateMilliHz).
			Str("display", displayName).
			Msg("cef: adaptive windowless frame rate updated")
	}
}

func widgetMonitorRefreshRateMilliHz(widget *gtk.Widget) (int, string, bool) {
	if widget == nil {
		return 0, "", false
	}
	native := widget.GetNative()
	if native == nil {
		return 0, "", false
	}
	surface := native.GetSurface()
	if surface == nil {
		return 0, "", false
	}
	display := surface.GetDisplay()
	if display == nil {
		return 0, "", false
	}
	monitor := display.GetMonitorAtSurface(surface)
	if monitor == nil {
		return 0, display.GetName(), false
	}
	return monitor.GetRefreshRate(), display.GetName(), true
}

func isWaylandDisplayName(name string) bool {
	return strings.Contains(strings.ToLower(name), "wayland")
}

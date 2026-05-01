package cef

import (
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/logging"
)

const (
	renderStallWatchdogInterval  = 1 * time.Second
	renderStallWarnAfter         = 3 * time.Second
	renderStallWarnRepeat        = 10 * time.Second
	renderStallCEFUIBlockedAfter = 3 * time.Second
)

type renderStallClassification struct {
	Category     string
	CEFUIBlocked bool
	CEFIOAlive   bool
}

type renderStallDiagnostics struct {
	browser      purecef.Browser
	uri          string
	title        string
	isLoading    bool
	pendingURI   string
	audioPlaying bool
	diag         cef2gtk.Diagnostics
	lastPaintAge time.Duration
	latestEvent  cef2gtk.DiagnosticEvent
}

func classifyRenderStall(uiHeartbeat, ioHeartbeat cefHeartbeatSnapshot) renderStallClassification {
	uiBlocked := uiHeartbeat.PostResult == 1 && uiHeartbeat.InFlight && uiHeartbeat.LastAckAge >= renderStallCEFUIBlockedAfter
	ioAlive := ioHeartbeat.PostResult == 1 && !ioHeartbeat.InFlight && ioHeartbeat.LastAckAge > 0 && ioHeartbeat.LastAckAge < renderStallCEFUIBlockedAfter
	category := "osr-paint-stalled"
	if uiBlocked {
		category = "cef-ui-task-runner-blocked"
		if ioAlive {
			category = "cef-ui-task-runner-blocked-io-alive"
		}
	}
	return renderStallClassification{Category: category, CEFUIBlocked: uiBlocked, CEFIOAlive: ioAlive}
}

func (wv *WebView) startRenderStallWatchdog() {
	if wv == nil || wv.ctx == nil || wv.viewBridge == nil {
		return
	}

	wv.renderStallMu.Lock()
	defer wv.renderStallMu.Unlock()
	if wv.renderStallStop != nil {
		return
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	bridge := wv.viewBridge
	wv.renderStallStop = stop
	wv.renderStallDone = done

	go wv.renderStallWatchdogLoop(bridge, stop, done)
}

func (wv *WebView) stopRenderStallWatchdog() {
	if wv == nil {
		return
	}
	wv.renderStallMu.Lock()
	stop := wv.renderStallStop
	done := wv.renderStallDone
	wv.renderStallStop = nil
	wv.renderStallDone = nil
	wv.renderStallMu.Unlock()

	if stop == nil || done == nil {
		return
	}
	close(stop)
	<-done
}

func (wv *WebView) renderStallWatchdogLoop(bridge *Cef2gtkAdapter, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(renderStallWatchdogInterval)
	defer ticker.Stop()

	var lastWarn time.Time
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			if wv.maybeLogRenderStall(bridge, now, lastWarn) {
				lastWarn = now
			}
		}
	}
}

func (wv *WebView) maybeLogRenderStall(bridge *Cef2gtkAdapter, now, lastWarn time.Time) bool {
	diag, ok := wv.shouldLogRenderStall(bridge, now, lastWarn)
	if !ok {
		return false
	}
	wv.logRenderStall(bridge, now, diag)
	return true
}

func (wv *WebView) shouldLogRenderStall(bridge *Cef2gtkAdapter, now, lastWarn time.Time) (renderStallDiagnostics, bool) {
	if wv == nil || bridge == nil || wv.destroyed.Load() || wv.ctx == nil {
		return renderStallDiagnostics{}, false
	}
	diag, ok := wv.collectStallDiagnostics(bridge, now)
	if !ok {
		return renderStallDiagnostics{}, false
	}
	if diag.lastPaintAge < renderStallWarnAfter {
		return renderStallDiagnostics{}, false
	}
	if !lastWarn.IsZero() && now.Sub(lastWarn) < renderStallWarnRepeat {
		return renderStallDiagnostics{}, false
	}
	return diag, true
}

func (wv *WebView) collectStallDiagnostics(bridge *Cef2gtkAdapter, now time.Time) (renderStallDiagnostics, bool) {
	wv.mu.RLock()
	browser := wv.browser
	uri := wv.uri
	title := wv.title
	isLoading := wv.isLoading
	pendingURI := wv.pendingURI
	wv.mu.RUnlock()
	if browser == nil {
		return renderStallDiagnostics{}, false
	}

	audioPlaying := wv.audioPlaying.Load()
	if !audioPlaying && !isLoading && pendingURI == "" {
		return renderStallDiagnostics{}, false
	}

	diag := bridge.Diagnostics()
	lastPaint := latestDiagnosticEventTime(diag, "accelerated-paint")
	if lastPaint.IsZero() {
		return renderStallDiagnostics{}, false
	}
	return renderStallDiagnostics{
		browser:      browser,
		uri:          uri,
		title:        title,
		isLoading:    isLoading,
		pendingURI:   pendingURI,
		audioPlaying: audioPlaying,
		diag:         diag,
		lastPaintAge: now.Sub(lastPaint),
		latestEvent:  latestDiagnosticEvent(diag),
	}, true
}

func (wv *WebView) logRenderStall(bridge *Cef2gtkAdapter, now time.Time, diag renderStallDiagnostics) {
	uiHeartbeat, ioHeartbeat := wv.cefHeartbeatSnapshots(now)
	classification := classifyRenderStall(uiHeartbeat, ioHeartbeat)
	wv.logRenderDiagnosticSnapshot(now, "stall", classification, bridge, diag.diag, diag.lastPaintAge, diag.browser, diag.uri, diag.title, diag.isLoading, diag.pendingURI, diag.audioPlaying)
	wv.logCEFProcessDiagnostics("stall", classification)
	if wv.engine != nil {
		wv.engine.logAllWebViewRenderSnapshots(now, wv.id, "stall-global", classification)
	}
	wv.maybeRunRenderStallRecovery(now, bridge, diag.diag.AcceleratedPaints)
	profile, profileAge, hasProfile := wv.latestCEF2GTKProfileSnapshot(now)
	logging.FromContext(wv.ctx).Warn().
		Str("classification", classification.Category).
		Bool("cef_ui_blocked", classification.CEFUIBlocked).
		Bool("cef_io_alive", classification.CEFIOAlive).
		Uint64("webview_id", uint64(wv.id)).
		Int32("browser_id", diag.browser.GetIdentifier()).
		Str("uri", logging.TruncateURL(diag.uri, logging.PermissionLogURLMaxLen)).
		Str("title", diag.title).
		Bool("is_loading", diag.isLoading).
		Str("pending_uri", logging.TruncateURL(diag.pendingURI, logging.PermissionLogURLMaxLen)).
		Bool("audio_playing", diag.audioPlaying).
		Int64("cef_ui_heartbeat_ack_age_ms", durationMillis(uiHeartbeat.LastAckAge)).
		Int64("cef_ui_heartbeat_latency_ms", durationMillis(uiHeartbeat.Latency)).
		Int32("cef_ui_heartbeat_post_result", uiHeartbeat.PostResult).
		Bool("cef_ui_heartbeat_in_flight", uiHeartbeat.InFlight).
		Int64("cef_io_heartbeat_ack_age_ms", durationMillis(ioHeartbeat.LastAckAge)).
		Int64("cef_io_heartbeat_latency_ms", durationMillis(ioHeartbeat.Latency)).
		Int32("cef_io_heartbeat_post_result", ioHeartbeat.PostResult).
		Bool("cef_io_heartbeat_in_flight", ioHeartbeat.InFlight).
		Uint64("audio_packets", wv.audioPacketCount.Load()).
		Uint64("audio_writes", wv.audioWriteCount.Load()).
		Int64("last_accelerated_paint_age_ms", diag.lastPaintAge.Milliseconds()).
		Bool("profile_snapshot_available", hasProfile).
		Int64("profile_snapshot_age_ms", durationMillis(profileAge)).
		Uint64("profile_frames_received", profile.FramesReceived).
		Uint64("profile_frames_queued", profile.FramesQueued).
		Uint64("profile_frames_rendered", profile.FramesRendered).
		Uint32("profile_gc_delta", profile.GC.NumGCDelta).
		Str("render_backend", diag.diag.Backend).
		Int("accelerated_paints", diag.diag.AcceleratedPaints).
		Int("unsupported_paints", diag.diag.UnsupportedPaints).
		Int("accelerated_paint_errors", diag.diag.AcceleratedPaintErrors).
		Int("import_failures", diag.diag.ImportFailures).
		Int("render_failures", diag.diag.RenderFailures).
		Int("textures_built", diag.diag.TexturesBuilt).
		Int("texture_build_failures", diag.diag.TextureBuildFailures).
		Int("fd_dup_failures", diag.diag.FDDupFailures).
		Int("unsupported_formats", diag.diag.UnsupportedFormats).
		Int("paintable_swaps", diag.diag.PaintableSwaps).
		Bool("pending_frame", diag.diag.PendingFrame).
		Bool("pending_scheduled", diag.diag.PendingScheduled).
		Int64("pending_age_ms", durationMillis(diag.diag.PendingAge)).
		Uint("pending_source_id", diag.diag.PendingSourceID).
		Int("pending_reschedules", diag.diag.PendingReschedules).
		Int("pending_schedule_failures", diag.diag.PendingScheduleFailures).
		Int("pending_idle_callbacks", diag.diag.PendingIdleCallbacks).
		Str("latest_render_event_kind", diag.latestEvent.Kind).
		Str("latest_render_event_message", diag.latestEvent.Message).
		Msg("cef: accelerated rendering appears stalled")
}

func latestDiagnosticEventTime(diag cef2gtk.Diagnostics, kind string) time.Time {
	return latestDiagnosticEventOfKind(diag, kind).Time
}

func latestDiagnosticEventOfKind(diag cef2gtk.Diagnostics, kind string) cef2gtk.DiagnosticEvent {
	var latest cef2gtk.DiagnosticEvent
	for _, event := range diag.Events {
		if event.Kind != kind {
			continue
		}
		if latest.Time.IsZero() || event.Time.After(latest.Time) {
			latest = event
		}
	}
	return latest
}

func latestDiagnosticEvent(diag cef2gtk.Diagnostics) cef2gtk.DiagnosticEvent {
	var latest cef2gtk.DiagnosticEvent
	for _, event := range diag.Events {
		if latest.Time.IsZero() || event.Time.After(latest.Time) {
			latest = event
		}
	}
	return latest
}

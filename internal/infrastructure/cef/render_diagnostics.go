package cef

import (
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const renderStallRecoveryCooldown = 10 * time.Second

func (e *Engine) logAllWebViewRenderSnapshots(now time.Time, _ port.WebViewID, reason string, classification renderStallClassification) {
	if e == nil {
		return
	}
	e.activeWebViews.Range(func(_, value any) bool {
		wv, ok := value.(*WebView)
		if !ok || wv == nil || wv.viewBridge == nil || wv.destroyed.Load() {
			return true
		}
		diag := wv.viewBridge.Diagnostics()
		lastPaintAge := time.Duration(0)
		if lastPaint := latestDiagnosticEventTime(diag, "accelerated-paint"); !lastPaint.IsZero() {
			lastPaintAge = now.Sub(lastPaint)
		}
		wv.mu.RLock()
		browser := wv.browser
		uri := wv.uri
		title := wv.title
		isLoading := wv.isLoading
		pendingURI := wv.pendingURI
		wv.mu.RUnlock()
		wv.logRenderDiagnosticSnapshot(
			now, reason, classification, wv.viewBridge, diag, lastPaintAge,
			browser, uri, title, isLoading, pendingURI, wv.audioPlaying.Load(),
		)
		return true
	})
}

func (wv *WebView) logRenderDiagnosticSnapshot(
	now time.Time,
	reason string,
	classification renderStallClassification,
	bridge *Cef2gtkAdapter,
	diag cef2gtk.Diagnostics,
	lastPaintAge time.Duration,
	browser purecef.Browser,
	uri, title string,
	isLoading bool,
	pendingURI string,
	audioPlaying bool,
) {
	if wv == nil || wv.ctx == nil {
		return
	}
	var browserID int32
	if browser != nil {
		browserID = browser.GetIdentifier()
	}
	width, height := int32(0), int32(0)
	scale := float32(0)
	if bridge != nil {
		width, height = bridge.Size()
		scale = bridge.DeviceScaleFactor()
	}
	uiHeartbeat, ioHeartbeat := wv.cefHeartbeatSnapshots(now)
	profile, profileAge, hasProfile := wv.latestCEF2GTKProfileSnapshot(now)
	logging.FromContext(wv.ctx).Warn().
		Str("reason", reason).
		Str("classification", classification.Category).
		Bool("cef_ui_blocked", classification.CEFUIBlocked).
		Bool("cef_io_alive", classification.CEFIOAlive).
		Uint64("webview_id", uint64(wv.id)).
		Int32("browser_id", browserID).
		Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
		Str("title", title).
		Bool("is_loading", isLoading).
		Str("pending_uri", logging.TruncateURL(pendingURI, logging.PermissionLogURLMaxLen)).
		Bool("audio_playing", audioPlaying).
		Uint64("audio_packets", wv.audioPacketCount.Load()).
		Uint64("audio_writes", wv.audioWriteCount.Load()).
		Int64("last_accelerated_paint_age_ms", durationMillis(lastPaintAge)).
		Str("render_backend", diag.Backend).
		Int("accelerated_paints", diag.AcceleratedPaints).
		Int("unsupported_paints", diag.UnsupportedPaints).
		Int("accelerated_paint_errors", diag.AcceleratedPaintErrors).
		Int("import_failures", diag.ImportFailures).
		Int("render_failures", diag.RenderFailures).
		Int("textures_built", diag.TexturesBuilt).
		Int("texture_build_failures", diag.TextureBuildFailures).
		Int("fd_dup_failures", diag.FDDupFailures).
		Int("unsupported_formats", diag.UnsupportedFormats).
		Int("paintable_swaps", diag.PaintableSwaps).
		Bool("pending_frame", diag.PendingFrame).
		Bool("pending_scheduled", diag.PendingScheduled).
		Int64("pending_age_ms", durationMillis(diag.PendingAge)).
		Uint("pending_source_id", diag.PendingSourceID).
		Int("pending_reschedules", diag.PendingReschedules).
		Int("pending_schedule_failures", diag.PendingScheduleFailures).
		Int("pending_idle_callbacks", diag.PendingIdleCallbacks).
		Int32("surface_width", width).
		Int32("surface_height", height).
		Float32("surface_scale", scale).
		Bool("external_begin_frame", externalBeginFrameEnabled()).
		Bool("profile_snapshot_available", hasProfile).
		Int64("profile_snapshot_age_ms", durationMillis(profileAge)).
		Uint64("profile_frames_received", profile.FramesReceived).
		Uint64("profile_frames_queued", profile.FramesQueued).
		Uint64("profile_frames_rendered", profile.FramesRendered).
		Uint64("profile_import_failures", profile.ImportFailures).
		Uint64("profile_render_failures", profile.RenderFailures).
		Uint32("profile_gc_delta", profile.GC.NumGCDelta).
		Int64("cef_ui_heartbeat_ack_age_ms", durationMillis(uiHeartbeat.LastAckAge)).
		Int64("cef_ui_heartbeat_latency_ms", durationMillis(uiHeartbeat.Latency)).
		Int32("cef_ui_heartbeat_post_result", uiHeartbeat.PostResult).
		Bool("cef_ui_heartbeat_in_flight", uiHeartbeat.InFlight).
		Int64("cef_io_heartbeat_ack_age_ms", durationMillis(ioHeartbeat.LastAckAge)).
		Int64("cef_io_heartbeat_latency_ms", durationMillis(ioHeartbeat.Latency)).
		Int32("cef_io_heartbeat_post_result", ioHeartbeat.PostResult).
		Bool("cef_io_heartbeat_in_flight", ioHeartbeat.InFlight).
		Msg("cef: render diagnostic snapshot")
}

func (wv *WebView) maybeRunRenderStallRecovery(now time.Time, _ *Cef2gtkAdapter, paintsBefore int) {
	if wv == nil || wv.ctx == nil || wv.destroyed.Load() || !renderStallRecoveryEnabled() {
		return
	}
	wv.renderStallMu.Lock()
	last := wv.renderStallRecoveryLastAt
	if !last.IsZero() && now.Sub(last) < renderStallRecoveryCooldown {
		wv.renderStallMu.Unlock()
		return
	}
	wv.renderStallMu.Unlock()

	wv.mu.RLock()
	host := wv.host
	browser := wv.browser
	wv.mu.RUnlock()
	if host == nil {
		return
	}
	browserID := int32(0)
	if browser != nil {
		browserID = browser.GetIdentifier()
	}
	logging.FromContext(wv.ctx).Warn().
		Uint64("webview_id", uint64(wv.id)).
		Int32("browser_id", browserID).
		Int("accelerated_paints_before", paintsBefore).
		Msg("cef: render stall recovery pulse scheduled")
	task := cefNewTask(cefTaskFunc(func() {
		if wv == nil || wv.viewBridge == nil || wv.destroyed.Load() || wv.ctx == nil || host == nil {
			return
		}
		host.Invalidate(purecef.PaintElementTypePetView)
		host.WasHidden(1)
		host.WasHidden(0)
		host.NotifyScreenInfoChanged()
		host.WasResized()
		host.Invalidate(purecef.PaintElementTypePetView)
		logging.FromContext(wv.ctx).Warn().
			Uint64("webview_id", uint64(wv.id)).
			Int32("browser_id", browserID).
			Msg("cef: render stall recovery pulse executed")
	}))
	if task == nil {
		return
	}
	result := cefPostTask(purecef.ThreadIDTidUi, task)
	logging.FromContext(wv.ctx).Warn().
		Uint64("webview_id", uint64(wv.id)).
		Int32("browser_id", browserID).
		Int32("post_result", result).
		Msg("cef: render stall recovery pulse posted")
	if result != 1 {
		return
	}
	wv.renderStallMu.Lock()
	wv.renderStallRecoveryLastAt = now
	wv.renderStallMu.Unlock()
	cefScheduleAfter(2*time.Second, func() {
		if wv == nil || wv.viewBridge == nil || wv.destroyed.Load() || wv.ctx == nil {
			return
		}
		after := wv.viewBridge.Diagnostics().AcceleratedPaints
		logging.FromContext(wv.ctx).Warn().
			Uint64("webview_id", uint64(wv.id)).
			Int32("browser_id", browserID).
			Int("accelerated_paints_before", paintsBefore).
			Int("accelerated_paints_after", after).
			Bool("recovered", after > paintsBefore).
			Msg("cef: render stall recovery pulse result")
	})
}

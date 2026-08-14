package cef

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

func logAccessibilityCaptureEnabled(ctx context.Context, webViewID port.WebViewID, captureDir string) {
	logging.FromContext(ctx).Info().
		Uint64("webview_id", uint64(webViewID)).
		Str("capture_dir", captureDir).
		Msg("cef: accessibility capture enabled")
}

func logAccessibilityUpdate(ctx context.Context, webViewID port.WebViewID, p accessibilityPayload) {
	logging.FromContext(ctx).Info().
		Uint64("webview_id", uint64(webViewID)).
		Str("kind", p.Kind).
		Int("bytes", p.Bytes).
		Int64("serialize_ns", p.SerializeNanos).
		Msg("cef: accessibility update")
}

func logAccessibilitySummary(ctx context.Context, webViewID port.WebViewID, snap accessibilityStatsSnapshot, dropped uint64) {
	logging.FromContext(ctx).Info().
		Uint64("webview_id", uint64(webViewID)).
		Int("count", snap.Count).
		Int("tree_count", snap.TreeCount).
		Int("location_count", snap.LocationCount).
		Int64("total_bytes", snap.TotalBytes).
		Int("min_bytes", snap.MinBytes).
		Int("max_bytes", snap.MaxBytes).
		Float64("average_bytes", snap.AverageBytes).
		Int64("total_serialize_ns", snap.TotalSerializeNanos).
		Float64("average_serialize_ns", snap.AverageSerializeNanos).
		Int64("max_serialize_ns", snap.MaxSerializeNanos).
		Uint64("dropped", dropped).
		Msg("cef: accessibility summary")
}

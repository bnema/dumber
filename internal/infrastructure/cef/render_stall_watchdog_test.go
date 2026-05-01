package cef

import (
	"context"
	"testing"
	"time"

	cef2gtk "github.com/bnema/purego-cef2gtk"
)

func TestRenderStallWatchdogStartStopIdempotent(t *testing.T) {
	wv := &WebView{ctx: context.Background(), viewBridge: &Cef2gtkAdapter{}}

	wv.startRenderStallWatchdog()
	firstStop := wv.renderStallStop
	firstDone := wv.renderStallDone
	if firstStop == nil || firstDone == nil {
		t.Fatal("startRenderStallWatchdog did not initialize stop/done channels")
	}

	wv.startRenderStallWatchdog()
	if wv.renderStallStop != firstStop || wv.renderStallDone != firstDone {
		t.Fatal("second start should not replace active watchdog channels")
	}

	wv.stopRenderStallWatchdog()
	if wv.renderStallStop != nil || wv.renderStallDone != nil {
		t.Fatal("stopRenderStallWatchdog should clear watchdog channels")
	}

	// A second stop should be a no-op, not a double close panic.
	wv.stopRenderStallWatchdog()
}

func TestClassifyRenderStallDetectsCEFUIBlockedWithIOAlive(t *testing.T) {
	classification := classifyRenderStall(
		cefHeartbeatSnapshot{PostResult: 1, InFlight: true, LastAckAge: 5 * time.Second},
		cefHeartbeatSnapshot{PostResult: 1, InFlight: false, LastAckAge: 100 * time.Millisecond},
	)
	if classification.Category != "cef-ui-task-runner-blocked-io-alive" {
		t.Fatalf("category = %q", classification.Category)
	}
	if !classification.CEFUIBlocked {
		t.Fatal("CEF UI should be classified as blocked")
	}
	if !classification.CEFIOAlive {
		t.Fatal("CEF IO should be classified as alive")
	}
}

func TestClassifyRenderStallKeepsOSRCategoryWhenUIHeartbeatHealthy(t *testing.T) {
	classification := classifyRenderStall(
		cefHeartbeatSnapshot{PostResult: 1, InFlight: false, LastAckAge: 100 * time.Millisecond},
		cefHeartbeatSnapshot{PostResult: 1, InFlight: false, LastAckAge: 100 * time.Millisecond},
	)
	if classification.Category != "osr-paint-stalled" {
		t.Fatalf("category = %q", classification.Category)
	}
	if classification.CEFUIBlocked {
		t.Fatal("CEF UI should not be classified as blocked")
	}
}

func TestLatestDiagnosticEventHelpers(t *testing.T) {
	now := time.Now()
	diag := cef2gtk.Diagnostics{Events: []cef2gtk.DiagnosticEvent{
		{Time: now.Add(-3 * time.Second), Kind: "accelerated-paint"},
		{Time: now.Add(-2 * time.Second), Kind: "render-failure", Message: "old"},
		{Time: now.Add(-1 * time.Second), Kind: "render-failure", Message: "new"},
	}}

	if got := latestDiagnosticEventOfKind(diag, "render-failure"); got.Message != "new" {
		t.Fatalf("latest render-failure message = %q, want new", got.Message)
	}
	if got := latestDiagnosticEvent(diag); got.Kind != "render-failure" || got.Message != "new" {
		t.Fatalf("latest event = (%q, %q), want (render-failure, new)", got.Kind, got.Message)
	}
	if got := latestDiagnosticEventTime(diag, "accelerated-paint"); !got.Equal(now.Add(-3 * time.Second)) {
		t.Fatalf("latest accelerated paint time = %v, want %v", got, now.Add(-3*time.Second))
	}
}

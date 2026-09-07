package cef

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

type recordingInjectBridge struct {
	accepted bool
	calls    [][2]float64
}

func (f *recordingInjectBridge) InjectScroll(dx, dy float64) bool {
	f.calls = append(f.calls, [2]float64{dx, dy})
	return f.accepted
}

func TestScrollPage_SmoothingOn_InjectsImpulse(t *testing.T) {
	host := &scrollRecorderHost{}
	fake := &recordingInjectBridge{accepted: true}
	wv := &WebView{
		host:             host,
		inputConfig:      RuntimeInputConfig{ScrollWheelSmoothing: true},
		scrollInjectSeam: fake,
	}

	req := port.PageScrollRequest{
		Command:    port.PageScrollCommandDown,
		FallbackDX: 0,
		FallbackDY: 80,
	}
	if err := wv.ScrollPage(context.Background(), req); err != nil {
		t.Fatalf("ScrollPage() unexpected error: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("inject calls = %d, want 1", len(fake.calls))
	}
	if fake.calls[0][0] != 0 || fake.calls[0][1] != -80 {
		t.Fatalf("inject deltas = %v, want (0,-80) CEF wheel sign", fake.calls[0])
	}
	if len(host.recordedEvents()) != 0 {
		t.Fatal("injected scroll leaked into the legacy native path")
	}
	if q := &wv.pageScrollQueue; q.tapDX != 0 || q.tapDY != 0 || q.heldDX != 0 || q.heldDY != 0 {
		t.Fatal("injected scroll leaked into the legacy JS queue")
	}
}

func TestScrollPage_SmoothingOn_InjectRejectedFallsBack(t *testing.T) {
	host := &scrollRecorderHost{}
	fake := &recordingInjectBridge{accepted: false}
	wv := &WebView{
		host:             host,
		inputConfig:      RuntimeInputConfig{ScrollWheelSmoothing: true},
		scrollInjectSeam: fake,
	}

	req := port.PageScrollRequest{
		Command:    port.PageScrollCommandDown,
		FallbackDX: 0,
		FallbackDY: 80,
	}
	if err := wv.ScrollPage(context.Background(), req); err != nil {
		t.Fatalf("ScrollPage() unexpected error: %v", err)
	}
	if len(host.recordedEvents()) != 1 {
		t.Fatalf("expected legacy fallback wheel event, got %d", len(host.recordedEvents()))
	}
}

func TestScrollPage_SmoothingOff_SkipsInjection(t *testing.T) {
	host := &scrollRecorderHost{}
	fake := &recordingInjectBridge{accepted: true}
	wv := &WebView{
		host:             host,
		scrollInjectSeam: fake,
	}

	req := port.PageScrollRequest{
		Command:    port.PageScrollCommandDown,
		FallbackDX: 0,
		FallbackDY: 80,
	}
	if err := wv.ScrollPage(context.Background(), req); err != nil {
		t.Fatalf("ScrollPage() unexpected error: %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatal("injection ran with smoothing disabled")
	}
	if len(host.recordedEvents()) != 1 {
		t.Fatalf("expected legacy wheel event, got %d", len(host.recordedEvents()))
	}
}

func TestScrollPage_SmoothingOn_NoBridgeFallsBack(t *testing.T) {
	host := &scrollRecorderHost{}
	wv := &WebView{
		host:        host,
		inputConfig: RuntimeInputConfig{ScrollWheelSmoothing: true},
	}

	req := port.PageScrollRequest{
		Command:    port.PageScrollCommandUpFast,
		FallbackDY: -320,
	}
	if err := wv.ScrollPage(context.Background(), req); err != nil {
		t.Fatalf("ScrollPage() unexpected error: %v", err)
	}
	events := host.recordedEvents()
	if len(events) != 1 || events[0].deltaY != 320 {
		t.Fatalf("expected legacy fallback (0,320), got %+v", events)
	}
}

package cef

import (
	"context"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

type viewportSyncOrderHost struct {
	purecef.BrowserHost
	calls     []string
	zoomLevel float64
}

type pendingBrowserCreateObservedSizeBridgeStub struct {
	sizeWidth       int32
	sizeHeight      int32
	refreshWidth    int32
	refreshHeight   int32
	refreshCalls    int
	refreshProvided bool
}

func (s *pendingBrowserCreateObservedSizeBridgeStub) Size() (int32, int32) {
	return s.sizeWidth, s.sizeHeight
}

func (s *pendingBrowserCreateObservedSizeBridgeStub) RefreshObservedSizeOnGTKThread() (int32, int32) {
	s.refreshCalls++
	if s.refreshProvided {
		return s.refreshWidth, s.refreshHeight
	}
	return s.sizeWidth, s.sizeHeight
}

func (h *viewportSyncOrderHost) WasHidden(state int32) {
	h.calls = append(h.calls, "WasHidden")
	if state != 0 {
		h.calls = append(h.calls, "WasHidden(nonzero)")
	}
}

func (h *viewportSyncOrderHost) NotifyScreenInfoChanged() {
	h.calls = append(h.calls, "NotifyScreenInfoChanged")
}

func (h *viewportSyncOrderHost) WasResized() {
	h.calls = append(h.calls, "WasResized")
}

func (h *viewportSyncOrderHost) Invalidate(_ purecef.PaintElementType) {
	h.calls = append(h.calls, "Invalidate")
}

func (h *viewportSyncOrderHost) SetZoomLevel(level float64) {
	h.calls = append(h.calls, "SetZoomLevel")
	h.zoomLevel = level
}

func (h *viewportSyncOrderHost) GetZoomLevel() float64 {
	return h.zoomLevel
}

func TestNotifyBrowserViewportSync_VisibleCallsFullSequence(t *testing.T) {
	host := &viewportSyncOrderHost{}

	notifyBrowserViewportSync(host, true)

	require.Equal(t, []string{"WasHidden", "NotifyScreenInfoChanged", "WasResized", "Invalidate"}, host.calls)
}

func TestNotifyBrowserViewportSync_HiddenSkipsWasHidden(t *testing.T) {
	host := &viewportSyncOrderHost{}

	notifyBrowserViewportSync(host, false)

	require.Equal(t, []string{"NotifyScreenInfoChanged", "WasResized", "Invalidate"}, host.calls)
}

func TestNotifyViewportSyncOnCEFUIThread_PostsCEFWork(t *testing.T) {
	oldNewTask := cefNewTask
	oldPostDelayedTask := cefPostDelayedTask
	defer func() {
		cefNewTask = oldNewTask
		cefPostDelayedTask = oldPostDelayedTask
	}()

	cefNewTask = func(task purecef.Task) purecef.Task { return task }

	var scheduled purecef.Task
	cefPostDelayedTask = func(threadID purecef.ThreadID, task purecef.Task, delayMs int64) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		if delayMs == 0 && scheduled == nil {
			scheduled = task
		}
		return 1
	}

	host := &viewportSyncOrderHost{}
	wv := &WebView{ctx: context.Background(), host: host}
	wv.notifyViewportSyncOnCEFUIThread(host, true)

	require.Empty(t, host.calls)
	require.NotNil(t, scheduled)

	scheduled.Execute()
	require.Equal(t, []string{
		"WasHidden",
		"NotifyScreenInfoChanged",
		"WasResized",
		"Invalidate",
		"SetZoomLevel",
		"NotifyScreenInfoChanged",
	}, host.calls)
}

func TestNotifyViewportSyncOnCEFUIThread_SkipsStaleHost(t *testing.T) {
	oldNewTask := cefNewTask
	oldPostDelayedTask := cefPostDelayedTask
	defer func() {
		cefNewTask = oldNewTask
		cefPostDelayedTask = oldPostDelayedTask
	}()

	cefNewTask = func(task purecef.Task) purecef.Task { return task }

	var scheduled purecef.Task
	cefPostDelayedTask = func(_ purecef.ThreadID, task purecef.Task, _ int64) int32 {
		scheduled = task
		return 1
	}

	capturedHost := &viewportSyncOrderHost{}
	currentHost := &viewportSyncOrderHost{}
	wv := &WebView{ctx: context.Background(), host: currentHost}
	wv.notifyViewportSyncOnCEFUIThread(capturedHost, true)

	require.NotNil(t, scheduled)
	scheduled.Execute()
	require.Empty(t, capturedHost.calls)
	require.Empty(t, currentHost.calls)
}

func TestNotifyViewportSyncOnCEFUIThread_SkipsDestroyedWebView(t *testing.T) {
	oldNewTask := cefNewTask
	oldPostDelayedTask := cefPostDelayedTask
	defer func() {
		cefNewTask = oldNewTask
		cefPostDelayedTask = oldPostDelayedTask
	}()

	cefNewTask = func(task purecef.Task) purecef.Task { return task }

	var scheduled purecef.Task
	cefPostDelayedTask = func(_ purecef.ThreadID, task purecef.Task, _ int64) int32 {
		scheduled = task
		return 1
	}

	host := &viewportSyncOrderHost{}
	wv := &WebView{ctx: context.Background(), host: host}
	wv.destroyed.Store(true)
	wv.notifyViewportSyncOnCEFUIThread(host, true)

	require.NotNil(t, scheduled)
	scheduled.Execute()
	require.Empty(t, host.calls)
}

func TestObservedViewportSizeReady(t *testing.T) {
	tests := []struct {
		name            string
		observedWidth   int32
		observedHeight  int32
		allocatedWidth  int32
		allocatedHeight int32
		want            bool
	}{
		{
			name:            "exact match",
			observedWidth:   793,
			observedHeight:  1753,
			allocatedWidth:  793,
			allocatedHeight: 1753,
			want:            true,
		},
		{
			name:            "one pixel drift is accepted",
			observedWidth:   792,
			observedHeight:  1754,
			allocatedWidth:  793,
			allocatedHeight: 1753,
			want:            true,
		},
		{
			name:            "fallback observed size is rejected",
			observedWidth:   1,
			observedHeight:  1,
			allocatedWidth:  793,
			allocatedHeight: 1753,
			want:            false,
		},
		{
			name:            "missing allocation is rejected",
			observedWidth:   793,
			observedHeight:  1753,
			allocatedWidth:  0,
			allocatedHeight: 1753,
			want:            false,
		},
		{
			name:            "large mismatch is rejected",
			observedWidth:   1200,
			observedHeight:  1400,
			allocatedWidth:  1587,
			allocatedHeight: 1679,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, observedViewportSizeReady(tt.observedWidth, tt.observedHeight, tt.allocatedWidth, tt.allocatedHeight))
		})
	}
}

func TestPendingBrowserCreateObservedSize_RefreshesStaleFallbackSize(t *testing.T) {
	bridge := &pendingBrowserCreateObservedSizeBridgeStub{
		sizeWidth:       1,
		sizeHeight:      1,
		refreshWidth:    1055,
		refreshHeight:   1719,
		refreshProvided: true,
	}

	observedWidth, observedHeight, ready := pendingBrowserCreateObservedSize(bridge, 1055, 1719)

	require.True(t, ready)
	require.Equal(t, int32(1055), observedWidth)
	require.Equal(t, int32(1719), observedHeight)
	require.Equal(t, 1, bridge.refreshCalls)
}

func TestPendingBrowserCreateObservedSize_SkipsRefreshWhenObservedSizeAlreadyMatches(t *testing.T) {
	bridge := &pendingBrowserCreateObservedSizeBridgeStub{sizeWidth: 1055, sizeHeight: 1719}

	observedWidth, observedHeight, ready := pendingBrowserCreateObservedSize(bridge, 1055, 1719)

	require.True(t, ready)
	require.Equal(t, int32(1055), observedWidth)
	require.Equal(t, int32(1719), observedHeight)
	require.Equal(t, 0, bridge.refreshCalls)
}

func TestShouldDeferPendingBrowserCreateFromViewportSync_WaitsForNativePopupAttach(t *testing.T) {
	wv := &WebView{pendingCreate: &pendingBrowserCreate{}, nativePopupCandidate: true}

	require.True(t, wv.shouldDeferPendingBrowserCreateFromViewportSync())

	wv.nativePopupFallbackStarted = true
	require.False(t, wv.shouldDeferPendingBrowserCreateFromViewportSync())
}

func TestPreparePendingBrowserCreateObservedSizeRetry_CoalescesScheduling(t *testing.T) {
	oldScheduleAfter := cefScheduleAfter
	defer func() { cefScheduleAfter = oldScheduleAfter }()

	scheduleCalls := 0
	cefScheduleAfter = func(delay time.Duration, fn func()) {
		require.Equal(t, pendingBrowserCreateObservedSizeRetryDelay, delay)
		require.NotNil(t, fn)
		scheduleCalls++
	}

	wv := &WebView{pendingCreate: &pendingBrowserCreate{}}

	attempt, action := wv.preparePendingBrowserCreateObservedSizeRetry(context.Background(), "test")
	require.Equal(t, 1, attempt)
	require.Equal(t, pendingBrowserCreateObservedSizeRetryScheduled, action)
	require.Equal(t, 1, scheduleCalls)

	attempt, action = wv.preparePendingBrowserCreateObservedSizeRetry(context.Background(), "test")
	require.Equal(t, 1, attempt)
	require.Equal(t, pendingBrowserCreateObservedSizeRetryAlreadyScheduled, action)
	require.Equal(t, 1, scheduleCalls)
}

func TestPreparePendingBrowserCreateObservedSizeRetry_MaxRetriesProceedsWithoutScheduling(t *testing.T) {
	oldScheduleAfter := cefScheduleAfter
	defer func() { cefScheduleAfter = oldScheduleAfter }()

	scheduleCalls := 0
	cefScheduleAfter = func(delay time.Duration, fn func()) {
		require.Equal(t, pendingBrowserCreateObservedSizeRetryDelay, delay)
		require.NotNil(t, fn)
		scheduleCalls++
	}

	wv := &WebView{pendingCreate: &pendingBrowserCreate{observedSizeRetries: pendingBrowserCreateObservedSizeMaxRetries}}

	attempt, action := wv.preparePendingBrowserCreateObservedSizeRetry(context.Background(), "test")
	require.Equal(t, pendingBrowserCreateObservedSizeMaxRetries+1, attempt)
	require.Equal(t, pendingBrowserCreateObservedSizeRetryProceedWithoutDelay, action)
	require.Equal(t, 0, scheduleCalls)
	require.False(t, wv.pendingCreate.observedSizeRetryScheduled)
}

func TestScheduleResizeRepaintPulse_CoalescesToLatestSequence(t *testing.T) {
	oldNewTask := cefNewTask
	oldPostDelayedTask := cefPostDelayedTask
	defer func() {
		cefNewTask = oldNewTask
		cefPostDelayedTask = oldPostDelayedTask
	}()

	cefNewTask = func(task purecef.Task) purecef.Task { return task }

	var scheduled []purecef.Task
	var delays []int64
	cefPostDelayedTask = func(threadID purecef.ThreadID, task purecef.Task, delayMs int64) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		scheduled = append(scheduled, task)
		delays = append(delays, delayMs)
		return 1
	}

	host := &viewportSyncOrderHost{}
	wv := &WebView{ctx: context.Background(), host: host}

	wv.scheduleResizeRepaintPulse(context.Background(), "first")
	wv.scheduleResizeRepaintPulse(context.Background(), "second")

	require.Equal(t, []int64{16, 48, 16, 48}, delays)
	require.Len(t, scheduled, 4)

	scheduled[0].Execute()
	scheduled[1].Execute()
	require.Empty(t, host.calls)

	scheduled[2].Execute()
	scheduled[3].Execute()
	require.Equal(t, []string{"Invalidate", "Invalidate"}, host.calls)
}

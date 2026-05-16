package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

type viewportSyncOrderHost struct {
	purecef.BrowserHost
	calls     []string
	zoomLevel float64
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

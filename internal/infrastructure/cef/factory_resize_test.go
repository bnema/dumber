package cef

import (
	"context"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

type resizeOrderHost struct {
	calls []string
}

func (h *resizeOrderHost) WasResized() {
	h.calls = append(h.calls, "WasResized")
}

func (h *resizeOrderHost) Invalidate(_ purecef.PaintElementType) {
	h.calls = append(h.calls, "Invalidate")
}

func TestNotifyBrowserResize_CallsWasResizedThenInvalidate(t *testing.T) {
	host := &resizeOrderHost{}

	notifyBrowserResize(host)

	require.Equal(t, []string{"WasResized", "Invalidate"}, host.calls)
}

func TestPostPendingBrowserCreate_RetriesWhenPostingTaskFails(t *testing.T) {
	oldNewTask := cefNewTask
	oldPostTask := cefPostTask
	oldScheduleAfter := cefScheduleAfter
	oldCreateBrowser := cefBrowserHostCreateBrowser
	defer func() {
		cefNewTask = oldNewTask
		cefPostTask = oldPostTask
		cefScheduleAfter = oldScheduleAfter
		cefBrowserHostCreateBrowser = oldCreateBrowser
	}()

	cefNewTask = func(task purecef.Task) purecef.Task { return task }

	postCalls := 0
	cefPostTask = func(threadID purecef.ThreadID, task purecef.Task) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		postCalls++
		if postCalls == 1 {
			return 0
		}
		task.Execute()
		return 1
	}

	retried := false
	cefScheduleAfter = func(delay time.Duration, fn func()) {
		require.Equal(t, pendingBrowserCreateRetryDelay, delay)
		retried = true
		fn()
	}

	createCalls := 0
	cefBrowserHostCreateBrowser = func(
		windowInfo *purecef.WindowInfo,
		_ purecef.RawClient,
		url string,
		settings *purecef.BrowserSettings,
		_ purecef.DictionaryValue,
		_ purecef.RequestContext,
	) int32 {
		createCalls++
		require.NotNil(t, windowInfo)
		require.NotNil(t, settings)
		require.Equal(t, "about:blank", url)
		return 1
	}

	windowInfo := purecef.NewWindowInfo()
	settings := purecef.NewBrowserSettings()
	wv := &WebView{
		ctx: context.Background(),
		pendingCreate: &pendingBrowserCreate{
			windowInfo: &windowInfo,
			settings:   &settings,
		},
	}

	factory := &WebViewFactory{}
	factory.postPendingBrowserCreate(context.Background(), wv, 640, 480)

	require.True(t, retried)
	require.Equal(t, 2, postCalls)
	require.Equal(t, 1, createCalls)
	require.Nil(t, wv.takePendingCreate())
}

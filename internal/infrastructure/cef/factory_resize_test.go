package cef

import (
	"context"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"
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

func TestConfigureWindowInfo_SetsAcceleratedWindowlessAndSharedTexture(t *testing.T) {
	info := purecef.NewWindowInfo()

	cef2gtk.ConfigureWindowInfo(&info, cef2gtk.WindowInfoOptions{})

	require.Equal(t, int32(1), info.WindowlessRenderingEnabled)
	require.Equal(t, int32(1), info.SharedTextureEnabled)
}

func TestShouldStartBrowserCreateFromSizeObserver_FalseAfterInitialResizeHandled(t *testing.T) {
	wv := &WebView{pendingCreate: &pendingBrowserCreate{}}

	require.True(t, wv.shouldStartBrowserCreateFromSizeObserver())
	wv.markInitialBrowserCreateResizeHandled()
	require.False(t, wv.shouldStartBrowserCreateFromSizeObserver())
}

func TestShouldStartBrowserCreateFromSizeObserver_FalseAfterPendingCreateConsumed(t *testing.T) {
	wv := &WebView{pendingCreate: &pendingBrowserCreate{}}

	require.True(t, wv.shouldStartBrowserCreateFromSizeObserver())
	require.NotNil(t, wv.takePendingCreate())
	require.False(t, wv.shouldStartBrowserCreateFromSizeObserver())
}

func TestInitialBrowserCreateSizeReadyFromObserver(t *testing.T) {
	tests := []struct {
		name   string
		width  int32
		height int32
		want   bool
	}{
		{name: "rejects bootstrap one by one", width: 1, height: 1, want: false},
		{name: "rejects zero width", width: 0, height: 480, want: false},
		{name: "rejects one pixel height", width: 640, height: 1, want: false},
		{name: "accepts real positive size", width: 640, height: 480, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, initialBrowserCreateSizeReadyFromObserver(tt.width, tt.height))
		})
	}
}

func TestHandleInitialBrowserCreateSizeObserver_IgnoresBootstrapForNormalWebView(t *testing.T) {
	factory := &WebViewFactory{}
	wv := &WebView{pendingCreate: &pendingBrowserCreate{}}
	prepared := false
	resized := false

	handled := factory.handleInitialBrowserCreateSizeObserver(
		context.Background(),
		wv,
		func(int32, int32) { resized = true },
		func() error {
			prepared = true
			return nil
		},
		1,
		1,
	)

	require.True(t, handled)
	require.False(t, prepared)
	require.False(t, resized)
	require.True(t, wv.shouldStartBrowserCreateFromSizeObserver())
}

func TestHandleInitialBrowserCreateSizeObserver_BootstrapPopupArmsFallbackWithoutPreparing(t *testing.T) {
	factory := &WebViewFactory{}
	wv := &WebView{pendingCreate: &pendingBrowserCreate{}, nativePopupCandidate: true}
	prepared := false
	var scheduled func()
	wv.nativePopupFallbackSchedule = func(_ time.Duration, fn func()) stoppableTimer {
		scheduled = fn
		return stubStoppableTimer{}
	}

	handled := factory.handleInitialBrowserCreateSizeObserver(
		context.Background(),
		wv,
		func(w, h int32) {
			factory.handlePopupShellInitialResize(context.Background(), wv, nil, w, h)
		},
		func() error {
			prepared = true
			return nil
		},
		1,
		1,
	)

	require.True(t, handled)
	require.False(t, prepared)
	require.NotNil(t, scheduled)
	require.True(t, wv.shouldStartBrowserCreateFromSizeObserver())
}

func TestHandleInitialBrowserCreateSizeObserver_ReadySizePreparesAndConsumesInitialResize(t *testing.T) {
	factory := &WebViewFactory{}
	wv := &WebView{pendingCreate: &pendingBrowserCreate{}}
	prepared := 0
	resizes := 0

	handled := factory.handleInitialBrowserCreateSizeObserver(
		context.Background(),
		wv,
		func(int32, int32) { resizes++ },
		func() error {
			prepared++
			return nil
		},
		640,
		480,
	)

	require.True(t, handled)
	require.Equal(t, 1, prepared)
	require.Equal(t, 1, resizes)
	require.False(t, wv.shouldStartBrowserCreateFromSizeObserver())
}

func TestHandleInitialBrowserCreateSizeObserver_ReadySizeAfterPopupFallbackPostsCreate(t *testing.T) {
	factory := &WebViewFactory{}
	wv := &WebView{pendingCreate: &pendingBrowserCreate{}, nativePopupCandidate: true}
	prepared := 0
	posted := 0
	var scheduled func()
	wv.nativePopupFallbackSchedule = func(_ time.Duration, fn func()) stoppableTimer {
		scheduled = fn
		return stubStoppableTimer{}
	}
	onFirstResize := func(w, h int32) {
		factory.handlePopupShellInitialResize(context.Background(), wv, func(int32, int32) {
			posted++
		}, w, h)
	}

	handled := factory.handleInitialBrowserCreateSizeObserver(
		context.Background(),
		wv,
		onFirstResize,
		func() error {
			prepared++
			return nil
		},
		0,
		480,
	)
	require.True(t, handled)
	require.Equal(t, 0, prepared)
	require.Equal(t, 0, posted)
	require.NotNil(t, scheduled)
	require.True(t, wv.shouldStartBrowserCreateFromSizeObserver())

	scheduled()
	require.True(t, wv.awaitsBrowserCreateFromNativePopupFallback())

	handled = factory.handleInitialBrowserCreateSizeObserver(
		context.Background(),
		wv,
		onFirstResize,
		func() error {
			prepared++
			return nil
		},
		640,
		480,
	)
	require.True(t, handled)
	require.Equal(t, 1, prepared)
	require.Equal(t, 1, posted)
	require.False(t, wv.shouldStartBrowserCreateFromSizeObserver())
}

func TestSchedulePopupShellNativeFallback_StartsFallbackOnTimeout(t *testing.T) {
	factory := &WebViewFactory{}
	wv := &WebView{pendingCreate: &pendingBrowserCreate{}, nativePopupCandidate: true}
	var scheduled func()
	wv.nativePopupFallbackSchedule = func(_ time.Duration, fn func()) stoppableTimer {
		scheduled = fn
		return stubStoppableTimer{}
	}

	factory.schedulePopupShellNativeFallback(context.Background(), wv)
	require.NotNil(t, scheduled)

	scheduled()

	require.True(t, wv.nativePopupFallbackStarted)
	require.False(t, wv.nativePopupCandidate)
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
		require.Equal(t, int32(1), windowInfo.WindowlessRenderingEnabled)
		require.Equal(t, int32(1), windowInfo.SharedTextureEnabled)
		return 1
	}

	windowInfo := purecef.NewWindowInfo()
	cef2gtk.ConfigureWindowInfo(&windowInfo, cef2gtk.WindowInfoOptions{})
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

package cef

import (
	"context"
	"sync"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/stretchr/testify/require"
)

// recordingScrollBridge mirrors the library retired-epoch contract: Invalidate
// retires the live gesture and returns its epoch while retaining it for
// epoch-scoped cleanup; cleanup only clears the matching live gesture,
// never a newer one, and reports whether it cleared anything.
type recordingScrollBridge struct {
	mu            sync.Mutex
	current       uint64
	live          uint64
	invalidations int
	cleanups      []uint64
}

func (f *recordingScrollBridge) startGesture() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current++
	f.live = f.current
	return f.live
}

func (f *recordingScrollBridge) InvalidateScroll() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidations++
	// Retain the retired gesture until matching cleanup clears it, like
	// the library session that stays stale until cleanupEpoch runs.
	return f.live
}

func (f *recordingScrollBridge) CancelScrollEpoch(epoch uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, epoch)
	if epoch == 0 || epoch != f.live {
		return false
	}
	f.live = 0
	return true
}

func (f *recordingScrollBridge) stats() (invalidations int, cleanups []uint64, live uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.invalidations, append([]uint64(nil), f.cleanups...), f.live
}

func TestScrollCancelOptionsMapping(t *testing.T) {
	wv := &WebView{
		inputConfig: RuntimeInputConfig{
			ScrollTouchpadInertia: true,
			ScrollWheelSmoothing:  true,
		},
	}
	opts := wv.bridgeInputOptions()
	require.True(t, opts.Scroll.TouchpadInertia)
	require.True(t, opts.Scroll.WheelSmoothing)

	wvDisabled := &WebView{inputConfig: RuntimeInputConfig{}}
	disabled := wvDisabled.bridgeInputOptions()
	require.False(t, disabled.Scroll.TouchpadInertia, "explicit false must survive mapping")
	require.False(t, disabled.Scroll.WheelSmoothing, "explicit false must survive mapping")
}

func TestNavCommandGTKInlineInvalidatesBeforeDispatch(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()
	var order []string
	browser := cefmocks.NewMockBrowser(t)
	browser.EXPECT().Reload().Run(func() { order = append(order, "dispatch") }).Return().Once()

	wv := &WebView{ctx: context.Background(), browser: browser, scrollCancelSeam: fakeWrap(fake, &order)}

	require.NoError(t, wv.Reload(context.Background()))

	invalidations, cleanups, live := fake.stats()
	require.Equal(t, 1, invalidations)
	require.Len(t, cleanups, 1)
	require.Zero(t, live)
	require.Equal(t, []string{"invalidate", "cleanup", "dispatch"}, order)
}

// fakeWrap records invalidate/cleanup ordering around the fake.
func fakeWrap(fake *recordingScrollBridge, order *[]string) *orderScrollBridge {
	return &orderScrollBridge{fake: fake, order: order}
}

type orderScrollBridge struct {
	fake  *recordingScrollBridge
	order *[]string
}

func (o *orderScrollBridge) InvalidateScroll() uint64 {
	*o.order = append(*o.order, "invalidate")
	return o.fake.InvalidateScroll()
}

func (o *orderScrollBridge) CancelScrollEpoch(epoch uint64) bool {
	*o.order = append(*o.order, "cleanup")
	return o.fake.CancelScrollEpoch(epoch)
}

func TestNavCommandOffThreadDispatchCompletes(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()
	browser := cefmocks.NewMockBrowser(t)
	browser.EXPECT().GoBack().Return().Once()

	wv := &WebView{
		ctx:             context.Background(),
		engine:          &Engine{},
		browser:         browser,
		scrollCancelSeam: fake,
		gtkSyncIsOwner:  func() bool { return false },
		gtkSyncDispatch: func(fn func()) { go fn() },
		gtkSyncTimeout:  2 * time.Second,
	}

	require.NoError(t, wv.GoBack(context.Background()))
	invalidations, cleanups, live := fake.stats()
	require.Equal(t, 1, invalidations)
	require.Len(t, cleanups, 1)
	require.Zero(t, live)
}

func TestNavCommandDispatchTimeoutSkipsNavigation(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()

	wv := &WebView{
		ctx:             context.Background(),
		engine:          &Engine{},
		scrollCancelSeam: fake,
		gtkSyncIsOwner:  func() bool { return false },
		gtkSyncDispatch: func(func()) {},
		gtkSyncTimeout:  20 * time.Millisecond,
	}

	err := wv.LoadURI(context.Background(), "https://example.com/")
	require.Error(t, err)
	require.Contains(t, err.Error(), "did not complete")
	require.Empty(t, wv.pendingNavigationURI(), "abandoned queued work must never navigate")
	invalidations, _, _ := fake.stats()
	require.Equal(t, 1, invalidations, "motion retires even when dispatch fails")
}

func TestNavCommandPreservesErrorContract(t *testing.T) {
	fake := &recordingScrollBridge{}
	destroyed := &WebView{}
	destroyed.destroyed.Store(true)
	require.ErrorIs(t, destroyed.Reload(context.Background()), errDestroyed)

	noBrowser := &WebView{ctx: context.Background(), scrollCancelSeam: fake}
	require.ErrorIs(t, noBrowser.Reload(context.Background()), errNoBrowser)
	require.ErrorIs(t, noBrowser.GoBack(context.Background()), errNoBrowser)
	require.ErrorIs(t, noBrowser.GoForward(context.Background()), errNoBrowser)
	require.ErrorIs(t, noBrowser.ReloadBypassCache(context.Background()), errNoBrowser)
	require.ErrorIs(t, noBrowser.LoadHTML(context.Background(), "<p>hi</p>", ""), errNoBrowser)
}

func TestQueuedCleanupAfterFreshGesture(t *testing.T) {
	fake := &recordingScrollBridge{}
	wv := &WebView{ctx: context.Background(), scrollCancelSeam: fake}

	first := fake.startGesture()
	retired := wv.scrollMotionRetired()
	require.Equal(t, first, retired)

	second := fake.startGesture()
	require.NotEqual(t, first, second)
	// The stale cleanup cannot cancel the fresh gesture.
	wv.cancelScrollEpochOnGTK(retired)
	_, _, live := fake.stats()
	require.Equal(t, second, live)
	require.True(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		for _, e := range fake.cleanups {
			if e == retired {
				return true
			}
		}
		return false
	}())
}

func TestInvalidateScrollMotionQueuesEpochCleanup(t *testing.T) {
	fake := &recordingScrollBridge{}
	wv := &WebView{ctx: context.Background(), scrollCancelSeam: fake}
	live := fake.startGesture()

	// engine==nil runs runOnGTK inline, so the queued cleanup applies now.
	require.Equal(t, live, wv.invalidateScrollMotion())
	_, cleanups, current := fake.stats()
	require.Equal(t, []uint64{live}, cleanups)
	require.Zero(t, current)
}

func TestPreBrowseInvalidationKeepsDecision(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().IsMain().Return(true)
	// downloadHandler is nil without an engine, so the request method is
	// never consulted; the request only needs to be non-nil to reach
	// pre-commit invalidation.
	request := cefmocks.NewMockRequest(t)
	h := &handlerSet{wv: &WebView{ctx: context.Background(), scrollCancelSeam: fake}}

	require.False(t, h.OnBeforeBrowse(nil, frame, request, 0, 0))
	invalidations, _, _ := fake.stats()
	require.Equal(t, 1, invalidations, "main-frame pre-commit invalidates")
}

func TestPreBrowseSubframeDoesNotCancel(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().IsMain().Return(false)
	h := &handlerSet{wv: &WebView{ctx: context.Background(), scrollCancelSeam: fake}}

	require.False(t, h.OnBeforeBrowse(nil, frame, nil, 0, 0))
	invalidations, _, live := fake.stats()
	require.Zero(t, invalidations, "subframe navigation must not cancel the main gesture")
	require.NotZero(t, live)
}

func TestLoadStartFallbackInvalidates(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().IsMain().Return(true).Once()
	frame.EXPECT().GetURL().Return("https://example.com/").Twice()
	h := &handlerSet{wv: &WebView{ctx: context.Background(), scrollCancelSeam: fake}}

	h.OnLoadStart(nil, frame, 0)
	invalidations, _, _ := fake.stats()
	require.Equal(t, 1, invalidations, "post-commit fallback invalidates")
}

func TestDestroyInvalidatesBeforeNativeClose(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()
	var order []string
	host := &orderBrowserHost{order: &order}
	wv := &WebView{
		viewBridge:       &Cef2gtkAdapter{},
		host:             host,
		scrollCancelSeam: &orderInvalidator{fake: fake, order: &order},
	}

	wv.Destroy()

	require.Equal(t, []string{"invalidate", "close"}, order)
	_, _, live := fake.stats()
	require.Zero(t, live)
}

type orderInvalidator struct {
	fake  *recordingScrollBridge
	order *[]string
}

func (o *orderInvalidator) InvalidateScroll() uint64 {
	*o.order = append(*o.order, "invalidate")
	return o.fake.InvalidateScroll()
}

func (o *orderInvalidator) CancelScrollEpoch(epoch uint64) bool {
	return o.fake.CancelScrollEpoch(epoch)
}

type orderBrowserHost struct {
	purecef.BrowserHost
	order *[]string
}

func (h *orderBrowserHost) CloseBrowser(_ int32) {
	*h.order = append(*h.order, "close")
}

func TestVisibilityHiddenTransitionInvalidates(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()
	host := &viewportSyncOrderHost{}
	wv := &WebView{scrollCancelSeam: fake}

	wv.applyEffectiveVisibility(host, false)
	invalidations, _, live := fake.stats()
	require.Equal(t, 1, invalidations)
	require.Zero(t, live)

	wv.applyEffectiveVisibility(host, true)
	invalidations, _, _ = fake.stats()
	require.Equal(t, 1, invalidations, "re-showing must not invalidate again")
}

func TestGestureHistoryCommitInvalidates(t *testing.T) {
	fake := &recordingScrollBridge{}
	fake.startGesture()
	browser := cefmocks.NewMockBrowser(t)
	browser.EXPECT().GoBack().Return().Once()
	wv := &WebView{
		ctx:              context.Background(),
		browser:          browser,
		canGoBack:        true,
		scrollCancelSeam: fake,
	}

	wv.handleNavigationSwipeAction(cef2gtk.NavigationSwipeBack)

	invalidations, _, _ := fake.stats()
	require.Equal(t, 1, invalidations, "gesture history commit invalidates")
}

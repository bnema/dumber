package cef

import (
	"context"
	"sync"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

type accessibilityStateHost struct {
	purecef.BrowserHost

	mu     sync.Mutex
	states []purecef.State
}

func (h *accessibilityStateHost) SetAccessibilityState(state purecef.State) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.states = append(h.states, state)
}

func (h *accessibilityStateHost) recordedStates() []purecef.State {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]purecef.State, len(h.states))
	copy(out, h.states)
	return out
}

func TestWebView_EnableAccessibility_NoOpWhenDestroyedOrHostNil(t *testing.T) {
	t.Parallel()

	destroyed := &WebView{}
	destroyed.destroyed.Store(true)
	host := &accessibilityStateHost{}
	destroyed.host = host
	destroyed.EnableAccessibility()
	assert.Empty(t, host.recordedStates())
	assert.False(t, destroyed.a11yRequested.Load())

	noHost := &WebView{}
	noHost.EnableAccessibility()
	assert.False(t, noHost.a11yEnabled.Load())
	assert.True(t, noHost.a11yRequested.Load(), "early request must persist while host is nil")
}

func TestWebView_EnableAccessibility_IdempotentOncePerWebView(t *testing.T) {
	t.Parallel()

	host := &accessibilityStateHost{}
	wv := &WebView{host: host}

	wv.EnableAccessibility()
	wv.EnableAccessibility()
	wv.EnableAccessibility()

	states := host.recordedStates()
	require.Len(t, states, 1)
	assert.Equal(t, purecef.StateStateEnabled, states[0])
	assert.True(t, wv.a11yEnabled.Load())
}

func TestWebView_EnableAccessibility_IndependentPerWebView(t *testing.T) {
	t.Parallel()

	hostA := &accessibilityStateHost{}
	hostB := &accessibilityStateHost{}
	wvA := &WebView{host: hostA}
	wvB := &WebView{host: hostB}

	wvA.EnableAccessibility()
	wvA.EnableAccessibility()
	wvB.EnableAccessibility()

	require.Len(t, hostA.recordedStates(), 1)
	require.Len(t, hostB.recordedStates(), 1)
	assert.Equal(t, purecef.StateStateEnabled, hostA.recordedStates()[0])
	assert.Equal(t, purecef.StateStateEnabled, hostB.recordedStates()[0])
}

func TestWebView_EnableAccessibility_PersistsUntilHostAttachment(t *testing.T) {
	t.Parallel()

	wv := &WebView{}
	wv.EnableAccessibility()
	assert.True(t, wv.a11yRequested.Load())
	assert.False(t, wv.a11yEnabled.Load())

	host := &accessibilityStateHost{}
	wv.host = host
	wv.applyAccessibilityIfReady()

	require.Len(t, host.recordedStates(), 1)
	assert.Equal(t, purecef.StateStateEnabled, host.recordedStates()[0])
	assert.True(t, wv.a11yEnabled.Load())
}

func TestWebView_EnableAccessibility_ConcurrentCallsEnableOnce(t *testing.T) {
	t.Parallel()

	host := &accessibilityStateHost{}
	wv := &WebView{host: host}

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wv.EnableAccessibility()
		}()
	}
	wg.Wait()

	require.Len(t, host.recordedStates(), 1)
	assert.Equal(t, purecef.StateStateEnabled, host.recordedStates()[0])
}

func TestWebView_EnableAccessibility_RequestThenDestroyBeforeAttachmentNeverEnables(t *testing.T) {
	t.Parallel()

	wv := &WebView{}
	wv.EnableAccessibility()
	assert.True(t, wv.a11yRequested.Load())

	wv.Destroy()

	host := &accessibilityStateHost{}
	wv.host = host
	wv.applyAccessibilityIfReady()
	wv.EnableAccessibility()

	assert.Empty(t, host.recordedStates())
	assert.False(t, wv.a11yEnabled.Load())
}

func TestFinishAfterCreated_AppliesPendingAccessibilityOnce(t *testing.T) {
	t.Parallel()

	wv := &WebView{ctx: context.Background()}
	wv.EnableAccessibility()
	assert.False(t, wv.a11yEnabled.Load())

	host := &accessibilityStateHost{}
	wv.host = host
	h := &handlerSet{wv: wv}
	h.finishAfterCreated(nil, host, afterCreatedState{})
	h.finishAfterCreated(nil, host, afterCreatedState{})

	require.Len(t, host.recordedStates(), 1)
	assert.Equal(t, purecef.StateStateEnabled, host.recordedStates()[0])
}

func TestFinishAfterCreated_SkipsWhenNoPendingRequest(t *testing.T) {
	t.Parallel()

	host := &accessibilityStateHost{}
	wv := &WebView{ctx: context.Background(), host: host}
	h := &handlerSet{wv: wv}
	h.finishAfterCreated(nil, host, afterCreatedState{})

	assert.Empty(t, host.recordedStates())
	assert.False(t, wv.a11yEnabled.Load())
}

func TestWebView_ImplementsAccessibilityEnabler(t *testing.T) {
	t.Parallel()
	var _ port.AccessibilityEnabler = (*WebView)(nil)
}
